# my_cv — SRE-лаб на своей вики

Учебный стенд под собеседование на SRE (Site Reliability Engineer): вики подготовки к интервью ([cibexon.fyi/interview.html](https://cibexon.fyi/interview.html)) переписана сервисом на Go, собирается в GitHub Actions, раскатывается ArgoCD через Helm в локальный kind-кластер, а Prometheus считает по ней SLO (Service Level Objective — целевой уровень надёжности, например «99.9% запросов без ошибки за 30 дней»).

```
git push ──► GitHub Actions ──► ghcr.io/cibexon/my_cv:sha-xxxx
                 │                         ▲
                 └─ commit image.tag ──► charts/wiki/values.yaml
                                           │  (ArgoCD опрашивает Git раз в минуту)
kind "lab" ◄── ArgoCD ◄────────────────────┘
   ├─ wiki (Helm chart из этого репо) ──/metrics──► Prometheus ──► Grafana «Wiki SLO»
   ├─ monitoring (kube-prometheus-stack)               └─ правила SLO ──► Alertmanager
   └─ argocd (ArgoCD управляет своим же Helm-релизом)
```

Почему так, а не «CI делает `kubectl apply`»: это **pull-модель GitOps** — кластер сам тянет желаемое состояние из Git. Ноутбук недоступен из интернета, GitHub не может достучаться до kind, а вот kind до GitHub — может. В проде то же самое даёт безопасность: у CI нет доступа к кластеру вообще, только право на коммит.

| Что | Где |
|---|---|
| Сервис на Go: рендер markdown → HTML, `/metrics`, встроенные поломки | [app/main.go](app/main.go) |
| Образ: multi-stage сборка, distroless | [Dockerfile](Dockerfile) |
| Helm chart: Deployment, Service, ServiceMonitor, PrometheusRule (SLO), дашборд | [charts/wiki](charts/wiki) |
| ArgoCD: app-of-apps, values для ArgoCD и kube-prometheus-stack | [deploy](deploy) |
| CI/CD | [.github/workflows](.github/workflows) |
| Учебные поломки для траблшутинга | [scenarios](scenarios), [scripts/break.sh](scripts/break.sh) |

Подробные разборы: [устройство репозитория](docs/01-repo.md) · [GitHub Actions построчно](docs/02-github-actions.md) · [ArgoCD: установка и работа](docs/03-argocd.md).

## Запуск

Нужны Docker Desktop, kind, kubectl, helm. Репозиторий уже на GitHub и образ уже собран CI (иначе поды будут в `ImagePullBackOff` до первой сборки).

```bash
scripts/bootstrap.sh                       # kind + ArgoCD через Helm + root Application
kubectl -n argocd get applications -w      # ждём, пока root, argocd, monitoring, wiki станут Synced/Healthy
```

| UI | Адрес | Логин |
|---|---|---|
| Вики | http://localhost:8080 | — |
| ArgoCD | http://localhost:8081 | admin / пароль печатает bootstrap |
| Grafana | http://localhost:3000 | admin / admin, дашборд «Wiki SLO» |
| Prometheus | http://localhost:9090 | — |
| Alertmanager | http://localhost:9093 | — |

Удалить всё: `kind delete cluster --name lab`.

Обновить контент вики: `scripts/sync-content.sh` (копирует `~/work/GitHubRepos/interview-prep/topics`) → commit → push. CI соберёт образ и сам закоммитит новый тег, поэтому **перед следующим push делай `git pull --rebase`**.

## 1. CI/CD: GitHub Actions

Два workflow, каждый срабатывает только на свои пути (`paths:`), чтобы правка README не пересобирала образ:

- [image.yaml](.github/workflows/image.yaml) — `app/**`, `content/**`, `Dockerfile`:
  1. `test` — `go vet` (статический анализ: подозрительные конструкции, которые компилируются, но скорее всего ошибка) и `go test -race` (тесты с детектором гонок — одновременный доступ горутин к одной памяти).
  2. `build` — образ под `linux/amd64` и `linux/arm64` сразу. Раннер GitHub — amd64, kind на Mac M-серии — arm64. Go кросс-компилирует сам (`--platform=$BUILDPLATFORM` в Dockerfile), поэтому эмуляция QEMU не нужна и сборка быстрая. Тег `sha-<коммит>` — неизменяемый: по тегу всегда понятно, какой код запущен, а откат = вернуть старый тег. `latest` в деплое не используем: он «плавает», и два пода могут оказаться на разных версиях. Кеш слоёв — `type=gha` (хранилище кеша GitHub Actions).
  3. `deploy` — `sed` пишет новый тег в `charts/wiki/values.yaml` и коммитит. Это и есть весь CD: дальше работает ArgoCD.
  - На pull request выполняются только тест и сборка без push — проверка, что собирается.
  - Логин в GHCR (GitHub Container Registry) — встроенным `GITHUB_TOKEN` с `permissions: packages: write`; личных токенов в секретах нет.
  - `concurrency` — два быстрых push не будут одновременно коммитить values.yaml.
  - Коммит от `GITHUB_TOKEN` не запускает другие workflow — защита GitHub от бесконечных циклов «CI коммитит → CI запускается».
- [chart.yaml](.github/workflows/chart.yaml) — `charts/**`, `deploy/**`, `scenarios/**`: `helm lint`, `helm template` для дефолтных values и каждого сценария, проверка схемы через kubeconform (сверяет YAML со схемой API Kubernetes; для CRD — схемы из CRDs-catalog).

Упражнения:
- Сделай PR с изменением в `app/` — посмотри, что `build` идёт без push, а `deploy` пропускается.
- Сломай тест намеренно → убедись, что образ не собирается (`needs: test`).
- Вопрос с собеседования: «как откатить релиз?» → `git revert` коммита `deploy: wiki sha-…`, ArgoCD вернёт старый образ. Без `kubectl` и без пересборки.

## 2. GitOps: ArgoCD и Helm

- **App of apps**: руками применяется только [deploy/root.yaml](deploy/root.yaml). Это Application, которое смотрит в `deploy/apps/` и создаёт остальные Application. Новый сервис = новый файл в Git.
- **ArgoCD ставится Helm'ом дважды**: первый раз `helm install` в bootstrap (курица и яйцо: некому ещё синкать), затем [apps/argocd.yaml](deploy/apps/argocd.yaml) забирает тот же релиз под управление ArgoCD. Обновить ArgoCD = поменять `targetRevision` в Git.
- **Multi-source** (`sources:` в monitoring и argocd): чарт берётся из публичного Helm-репозитория, а values — из этого Git-репо через `ref: values` → `$values/deploy/values/...`. Не надо копировать чужой чарт к себе.
- **sync-wave**: monitoring (волна 0) приносит CRD (Custom Resource Definition — новые типы объектов в API, тут ServiceMonitor и PrometheusRule), wiki (волна 1) их использует. `SkipDryRunOnMissingResource` + `retry` страхуют первый запуск, когда CRD ещё нет.
- **selfHeal** и **prune**: ручная правка в кластере откатывается к Git; объект, удалённый из Git, удаляется из кластера.

Упражнения:
1. `kubectl -n wiki scale deploy wiki --replicas=5` → через несколько секунд снова 2 (selfHeal). Где это видно в UI ArgoCD?
2. Поменяй `replicaCount` в values.yaml через Git → push → смотри sync (или кнопка Refresh в UI, чтобы не ждать минуту).
3. `helm template wiki charts/wiki -n wiki | less` — что именно ArgoCD применяет. ArgoCD не вызывает `helm install`: он рендерит шаблоны и применяет как обычные манифесты, поэтому `helm list -n wiki` пустой. Частый вопрос на интервью.
4. `helm history argocd -n argocd` — одна ревизия от bootstrap, и новых не появится даже после апгрейда ArgoCD через Git. Почему? (Дальше манифесты применяет ArgoCD, Helm-релиз остался «замороженным» следом первой установки.)
5. Поменяй `targetRevision` у kube-prometheus-stack на старую версию и посмотри diff в UI до синка.

## 3. SLO, error budget, burn rate

**Термины**:
- **SLI** (Service Level Indicator) — измеримая доля «хороших» событий. Здесь два:
  - availability — доля запросов без 5xx;
  - latency — доля запросов быстрее 300 мс.
- **SLO** — цель для SLI за период: 99.9% availability и 99% latency за 30 дней ([values.yaml](charts/wiki/values.yaml), блок `slo`).
- **Error budget** (бюджет ошибок) = 1 − SLO. При 99.9% это 0.1% запросов. При 5 rps за 30 дней это ~13 000 ошибок из ~13 млн, или ~43 минуты полного простоя. Пока бюджет есть — можно рисковать: релизить чаще, экспериментировать. Кончился — фризим фичи и чиним надёжность. Это и есть договорённость SRE с разработкой.
- **Burn rate** (скорость сжигания) = текущая доля ошибок / бюджет. 1 — бюджет закончится ровно к концу 30 дней. 14.4 — за 30/14.4 ≈ 2 дня; за 1 час уходит 2% бюджета.

**Как устроено в chart** ([prometheusrule.yaml](charts/wiki/templates/prometheusrule.yaml)):
- **Recording rules** `slo:error_ratio:rate<окно>` — Prometheus раз в 30 с считает долю ошибок за 5m, 30m, 1h, 2h, 6h, 1d, 3d, 30d и хранит как отдельную серию. Алерты и дашборд читают готовое число, а не гоняют `rate()` за 3 дня на каждом обновлении.
- `slo:error_budget_remaining:ratio` — сколько бюджета осталось за период (1 — весь, 0 — кончился, меньше 0 — SLO нарушен).
- **Multi-window multi-burn-rate алерты** (Google SRE Workbook, гл. 5):

  | Окна | Burn rate | Сколько бюджета сгорит | Severity |
  |---|---|---|---|
  | 1h и 5m | 14.4 | 2% за час | page — будить дежурного |
  | 6h и 30m | 6 | 5% за 6 часов | page |
  | 1d и 2h | 3 | 10% за сутки | ticket — разобрать в рабочее время |
  | 3d и 6h | 1 | 10% за 3 дня | ticket |

  Длинное окно доказывает, что проблема значимая (не один всплеск). Короткое — что она ещё идёт: после починки алерт гаснет за минуты, а не через час.

**Почему не «алерт на CPU > 80%»**: CPU — причина, а не симптом. Пользователю всё равно, сколько CPU, пока страницы отдаются быстро. SLO-алерт срабатывает ровно тогда, когда страдают пользователи, и его порог объясним бизнесу.

Упражнения (Grafana → «Wiki SLO», Prometheus → Alerts):
1. Посмотри сырые данные: в Prometheus `rate(http_requests_total{namespace="wiki"}[1m])`, затем `slo:error_ratio:rate5m`.
2. **Быстрое горение**: в `charts/wiki/values.yaml` поставь `faults.errorRate: 0.05` (5% ошибок = burn rate 50) → push. Через ~5 минут `WikiAvailabilityBudgetBurn` (window=1h) в состоянии firing — почему так быстро, если окно час? (5% за 5 минут дают в часовом окне > 1.44%.) Верни 0 и засеки, когда алерт погаснет (по короткому окну 5m).
3. **Латентность**: `faults.slowRate: 0.05`, `faults.latency: 500ms`. Availability SLI не меняется, а latency-бюджет горит. Посмотри p99 на графике.
4. **Медленное горение**: `errorRate: 0.004` (burn rate 4). Page-алерты молчат, сработает только ticket через 2 часа. Это и задумано: такое чинят днём, а не ночью.
5. **Быстрый эксперимент без Git** (только один под, ArgoCD про это не знает):
   ```bash
   kubectl -n wiki port-forward deploy/wiki 18080:8080 &
   curl -X POST 'localhost:18080/admin/fault?error_rate=0.3'
   ```
   Почему на графике ошибок меньше 30%? (Подов два, сломан один.)
6. Посчитай руками: SLO 99.9%, 5 rps, 30 дней — сколько запросов можно потерять? Сколько минут полного простоя? А при 99.99%?
7. Вопрос «как выбрать SLO»: не 100% (бесконечно дорого, и пользователь всё равно не заметит разницы с 99.99% из-за своего Wi-Fi). Берут по истории SLI за прошлые месяцы, чуть ниже фактического, и согласуют с продуктом.

## 4. Troubleshooting drills

`scripts/break.sh <сценарий>` ставит сломанную копию вики Helm'ом в namespace `lab`, вне ArgoCD, чтобы selfHeal её не чинил. Задача — найти причину только по симптомам и **проговорить вслух на английском**, как на интервью. Сброс — `scripts/break.sh fix`.

Базовая последовательность для любого сценария:
`kubectl get pods` → `describe pod` (Events внизу) → `logs` / `logs --previous` → `get endpointslices` → `kubectl get events --sort-by=.lastTimestamp`.

```bash
scripts/break.sh list
scripts/break.sh 1-bad-image
```

<details><summary>1-bad-image — ответ</summary>

`ImagePullBackOff` / `ErrImagePull`. В `describe pod` → Events: `manifest unknown` / `not found` для тега `v9.9.9`. Причина: такого тега нет в реестре. Другие варианты того же симптома: приватный реестр без `imagePullSecrets` (`401 Unauthorized`), опечатка в имени, rate limit Docker Hub (`429`). Back-off — kubelet повторяет с растущей паузой до 5 минут.
</details>

<details><summary>2-crashloop — ответ</summary>

`CrashLoopBackOff`. `kubectl -n lab logs <pod> --previous` (логи упавшего контейнера, а не текущего): `cannot load content dir=/data/content`. Причина: неправильный `CONTENT_DIR`. Exit code в `describe` → Last State: `1` — приложение вышло само. Сравни: `137` = убит SIGKILL (часто OOM), `143` = SIGTERM.
</details>

<details><summary>3-oom — ответ</summary>

Под периодически рестартует. `describe pod` → Last State: `Terminated, Reason: OOMKilled, Exit Code: 137`. Контейнер упёрся в `limits.memory: 48Mi`, ядро (cgroup — механизм ядра, ограничивающий ресурсы группы процессов) убило процесс. Утечка: метрика `wiki_leaked_bytes` растёт. Лечение — найти утечку (pprof), а не только поднять лимит. `kubectl top pod` покажет рост памяти, если стоит metrics-server.
</details>

<details><summary>4-pending — ответ</summary>

Поды `Pending`. Events: `0/3 nodes are available: 3 Insufficient cpu`. Scheduler (планировщик) размещает под по `requests`, а не по реальному потреблению; 16 CPU не влезают ни в одну ноду. Проверить: `kubectl describe node | grep -A5 Allocated`. Другие причины Pending: taints без tolerations, nodeSelector/affinity, PVC не привязан.
</details>

<details><summary>5-not-ready — ответ</summary>

Поды `Running`, но `0/1 READY`. Events: `Readiness probe failed: HTTP probe failed with statuscode: 404`. Проба смотрит на `/ready`, а приложение отвечает на `/readyz`. Неготовый под исключается из endpoints Service: `kubectl -n lab get endpointslices` — адресов нет, curl на сервис падает. Рестартов нет: readiness не перезапускает, только убирает из балансировки.
</details>

<details><summary>6-wrong-port — ответ</summary>

Поды Ready, endpoints есть, но curl: `Connection refused`. В `get endpointslices` порт 8000, а контейнер слушает 8080. Service `targetPort` не совпадает с `containerPort`. Проверка изнутри пода-клиента: `curl <IP пода>:8080` работает, `:8000` — нет. Хорошая практика — именованный порт (`targetPort: http`), тогда ошибиться сложнее.
</details>

<details><summary>7-liveness — ответ</summary>

Под Running, но счётчик RESTARTS растёт каждые ~30 с. Events: `Liveness probe failed: ... 404`, затем `Container wiki failed liveness probe, will be restarted`. Проба смотрит на `/health`, а есть `/healthz`. Отличие от readiness: liveness убивает контейнер. Поэтому liveness должна быть простой («процесс жив») и не проверять зависимости (БД), иначе падение БД перезапустит все поды разом.
</details>

<details><summary>8-dns — ответ</summary>

Поды Ready, но `curl http://lab-wiki` из пода: `Could not resolve host`, а `curl <ClusterIP>` работает. `kubectl -n kube-system get deploy coredns` → `0/0`. CoreDNS (DNS-сервер кластера) выключен, имена сервисов не резолвятся. Проверка: `kubectl run -it --rm dns --image=busybox:1.36 -- nslookup kubernetes.default`. В проде так же выглядит перегруженный CoreDNS или сломанный NetworkPolicy, блокирующий UDP/53.
</details>
