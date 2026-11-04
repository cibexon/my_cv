# 1. Устройство репозитория

Как всё связано, одной картинкой:

```
 ты: git push
      │
      ▼
 GitHub ──► Actions: image.yaml ──► образ ghcr.io/cibexon/my_cv:sha-xxxxxxx
      ▲           │
      │           └─► коммит бота: charts/wiki/values.yaml  tag: sha-xxxxxxx
      │
      │ (раз в минуту: «что нового в Git?»)
 ArgoCD внутри kind ──► рендерит charts/wiki ──► Deployment/Service/... в namespace wiki
                                                        │
                                Prometheus ◄── /metrics ┘ ──► Grafana, Alertmanager
```

## Корень

| Файл | Что делает |
|---|---|
| `README.md` | Обзор и упражнения: CI/CD, GitOps, SLO, поломки |
| `Dockerfile` | Как собрать образ: компиляция Go + копирование `content/` (разбор ниже) |
| `.gitignore` | Что git не отслеживает: `.DS_Store` (служебный файл macOS Finder), настройки IDE |
| `docs/` | Эта документация |

## `app/` — сервис на Go

| Файл | Что делает |
|---|---|
| `go.mod` | Описание модуля: имя (`github.com/cibexon/my_cv/app`), версия Go, прямые зависимости — goldmark (markdown → HTML) и client_golang (метрики Prometheus). Аналог `requirements.txt` / `package.json` |
| `go.sum` | Контрольные суммы всех зависимостей, включая транзитивные (зависимости зависимостей). Если кто-то подменит библиотеку, сборка упадёт. Аналог lock-файла |
| `main.go` | Весь сервис, один бинарник с двумя режимами |
| `main_test.go` | Тесты: страница рендерится, пустая папка с контентом = ошибка, ошибка от fault injection считается в метрике |

Что внутри `main.go`, по блокам:

- **Метрики** (`requests`, `duration`, `leakedBytes`):
  - `http_requests_total{route, code}` — счётчик (counter, только растёт) запросов. По нему считается SLI доступности.
  - `http_request_duration_seconds` — гистограмма (histogram — счётчики «сколько запросов уложилось в 10 мс, 25 мс, … 5 с», их называют бакетами). По ней считается SLI задержки и p99. Бакет `0.3` стоит специально: порог latency SLO должен точно совпадать с границей бакета, иначе Prometheus не скажет, сколько запросов было быстрее 300 мс.
  - `wiki_leaked_bytes` — сколько памяти «утекло» намеренно.
- **`main()`**:
  1. Если первый аргумент `loadgen` — запускает генератор нагрузки, а не сервер. Один образ на два Deployment.
  2. Читает markdown из `CONTENT_DIR`. Если не удалось — выходит с кодом 1, отсюда сценарий CrashLoopBackOff.
  3. Берёт настройки поломок из переменных окружения (`FAULT_*`, `LEAK_MB_PER_SEC`).
  4. Регистрирует маршруты: `/` и `/en/` — списки тем, `/t/<файл>` — тема, `/healthz` — liveness (жив ли процесс), `/readyz` — readiness (готов ли принимать трафик), `/metrics` — для Prometheus, `/admin/fault` — включить поломку на лету.
  5. Ждёт SIGTERM (сигнал, который kubelet шлёт при удалении пода) и делает `Shutdown` — дообслуживает начатые запросы до 10 с. Без этого при каждом деплое часть запросов обрывалась бы с ошибкой и жгла error budget.
- **`instrument()`** — middleware (обёртка вокруг обработчика): с заданной вероятностью тормозит и/или отвечает 500, затем записывает код и время ответа в метрики.
- **`leak()`** — каждую секунду забирает N МБ и не отдаёт, пока контейнер не упрётся в лимит памяти и не получит OOMKilled.
- **`loadSite()`** — один раз при старте превращает все `.md` в HTML и держит в памяти. Дальше запросы обслуживаются без диска.
- **`loadgen()`** — ходит по 5 страницам с заданной частотой и раз в минуту пишет в лог, сколько запросов было успешных и неудачных.

## `content/` — тексты вики

Копия `~/work/GitHubRepos/interview-prep/topics`: `*.md` — русские, `en/*.md` — английские. Обновляется скриптом `scripts/sync-content.sh`. Изменение здесь = новый образ (`content/**` есть в триггерах `image.yaml`).

## `Dockerfile`

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
```
Первая стадия (multi-stage build — несколько FROM в одном файле; в итоговый образ попадает только последняя стадия). `$BUILDPLATFORM` — архитектура машины, на которой идёт сборка (amd64 у раннера GitHub). Компилятор запускается нативно, без эмуляции.

```dockerfile
ARG TARGETOS TARGETARCH
```
Какую платформу собираем сейчас (`linux` + `amd64` или `arm64`). buildx подставляет их сам для каждой платформы из `platforms:`.

```dockerfile
WORKDIR /src
COPY app/go.mod app/go.sum ./
RUN go mod download
```
Сначала копируем только файлы зависимостей и скачиваем их. Этот слой кешируется, пока не меняется `go.sum`: правка в `main.go` не перекачивает библиотеки.

```dockerfile
COPY app/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /wiki .
```
- `CGO_ENABLED=0` — без C-библиотек: статический бинарник, который не требует glibc и запускается в пустом образе.
- `GOOS/GOARCH` — кросс-компиляция под целевую платформу.
- `-trimpath` — убрать локальные пути из бинарника.
- `-s -w` — выкинуть отладочную информацию (бинарник меньше примерно на треть).

```dockerfile
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /wiki /wiki
COPY content/ /content/
EXPOSE 8080
ENTRYPOINT ["/wiki"]
```
- distroless — образ без shell и пакетного менеджера: только CA-сертификаты, tzdata и пользователь `nonroot` (uid 65532). Меньше размер (28 МБ всего) и меньше CVE (Common Vulnerabilities and Exposures — публичные уязвимости, которые находят сканеры вроде Trivy). Минус — нельзя `kubectl exec ... sh`; для отладки есть `kubectl debug`.
- `ENTRYPOINT` в exec-форме (JSON-массив): процесс сразу PID 1 и сам получает SIGTERM. Та же тема, что `exec "$@"` в entrypoint-скриптах.

## `charts/wiki/` — Helm chart

Helm — шаблонизатор для манифестов Kubernetes: `templates/*.yaml` + `values.yaml` → готовый YAML.

| Файл | Что делает |
|---|---|
| `Chart.yaml` | Паспорт чарта: имя, `version` (версия самого чарта), `appVersion` (версия приложения, только для информации) |
| `values.yaml` | Все настройки по умолчанию: образ и **тег (его меняет CI)**, реплики, поломки, ресурсы, пробы, Service, loadgen, SLO-цели и burn-rate алерты |
| `.helmignore` | Какие файлы не класть в пакет чарта |
| `files/dashboard.json` | Дашборд Grafana с плейсхолдерами `__SELECTOR__`, `__AVAIL_TARGET__` и т. п. |
| `templates/_helpers.tpl` | Именованные шаблоны (`define`), которые вызываются из остальных через `include`: имя ресурсов, общие метки, селектор для PromQL, формула доли ошибок для SLO |
| `templates/deployment.yaml` | Deployment сервиса: env из `faults`, пробы, ресурсы, securityContext (не root, read-only файловая система, без Linux capabilities — привилегий ядра вроде смены владельца файлов) |
| `templates/loadgen.yaml` | Deployment генератора нагрузки (тот же образ, аргумент `loadgen`), если `loadgen.enabled` |
| `templates/service.yaml` | Service: NodePort 30000 (через kind → localhost:8080) или ClusterIP в сценариях |
| `templates/servicemonitor.yaml` | ServiceMonitor — CRD (Custom Resource Definition — свой тип объекта в API) от Prometheus Operator: «скрейпь `/metrics` у этого Service каждые 15 с» |
| `templates/prometheusrule.yaml` | PrometheusRule — правила SLO: recording rules (доля ошибок по окнам 5m…30d, остаток бюджета) и 8 алертов burn rate |
| `templates/dashboard.yaml` | ConfigMap с дашбордом и меткой `grafana_dashboard: "1"` — sidecar Grafana (вспомогательный контейнер в поде Grafana) находит такие ConfigMap и загружает дашборд сам. В JSON подставляются селектор и цели SLO из values |

Посмотреть, что получится, без кластера:
```bash
helm template wiki charts/wiki -n wiki | less
helm template wiki charts/wiki -n wiki --set faults.errorRate=0.05 | grep -A1 FAULT_ERROR
```

## `deploy/` — всё для кластера и ArgoCD

| Файл | Что делает |
|---|---|
| `kind-config.yaml` | Кластер `lab`: 1 control-plane + 2 worker; пробрасывает NodePort'ы 30000–30004 на localhost |
| `root.yaml` | Корневое Application (app of apps); единственное, что применяется руками |
| `apps/argocd.yaml` | ArgoCD управляет своим Helm-релизом |
| `apps/monitoring.yaml` | kube-prometheus-stack (Prometheus, Grafana, Alertmanager, node-exporter, kube-state-metrics, Prometheus Operator) |
| `apps/wiki.yaml` | Наш сервис из `charts/wiki` |
| `values/argocd.yaml` | Настройки чарта ArgoCD |
| `values/kube-prometheus-stack.yaml` | Настройки мониторинга |

Подробно — в [03-argocd.md](03-argocd.md).

## `scenarios/` — поломки для траблшутинга

| Файл | Что ломает |
|---|---|
| `_lab.yaml` | База для всех: ClusterIP, без loadgen, без мониторинга |
| `1-bad-image.yaml` … `7-liveness.yaml` | По одному неправильному значению в values (тег, путь к контенту, лимит памяти, requests CPU, путь readiness, targetPort, путь liveness) |

Сценарий 8-dns — без файла: его делает `break.sh`, выключая CoreDNS.

## `scripts/`

| Скрипт | Что делает |
|---|---|
| `bootstrap.sh` | kind + ArgoCD через Helm + `root.yaml` (построчно — в [03-argocd.md](03-argocd.md)) |
| `break.sh` | `list` / `<сценарий>` / `fix` — Helm-релиз `lab` в namespace `lab` со сломанными values |
| `sync-content.sh` | Копирует вики из interview-prep в `content/` через `rsync --delete` (удалённые темы удаляются и здесь) |

## `.github/workflows/`

`image.yaml` и `chart.yaml` — построчно в [02-github-actions.md](02-github-actions.md).
