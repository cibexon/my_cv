# 3. ArgoCD: как поднимается и как работает

## Идея GitOps в двух фразах

Git — единственный источник правды о том, что должно работать в кластере. ArgoCD — контроллер внутри кластера, который постоянно сравнивает «что в Git» с «что в кластере» и приводит кластер к Git.

## Из чего состоит ArgoCD

`kubectl -n argocd get pods` покажет:

| Под | Что делает |
|---|---|
| `argocd-server` | Web UI и API. Через него работают браузер (localhost:8081) и CLI `argocd` |
| `argocd-repo-server` | Клонирует Git-репозитории и Helm-чарты, **рендерит манифесты**: для Helm вызывает `helm template`, для папки с YAML просто читает файлы. Результат кеширует по коммиту |
| `argocd-application-controller` | Главный цикл: берёт отрендеренное у repo-server, сравнивает с живыми объектами в API кластера и применяет разницу. Считает статусы Sync и Health |
| `argocd-redis` | Кеш для repo-server и controller (манифесты, состояние) |
| `argocd-applicationset-controller` | Генерирует Application по шаблону (например, по одному на каждую папку или кластер). У нас не используется, но ставится чартом |

dex (SSO — вход через GitHub/Google) и notifications (уведомления в Slack/Telegram) выключены в `values/argocd.yaml`.

## Объект Application

Application — CRD (свой тип объекта в API Kubernetes), который ставит ArgoCD. Одно Application = «вот источник, вот куда, вот как синхронизировать»:

- `source` / `sources` — откуда: Git-репо + путь, или Helm-репо + чарт + версия;
- `destination` — куда: кластер (`https://kubernetes.default.svc` = тот же кластер, где ArgoCD) и namespace;
- `syncPolicy` — синхронизировать автоматически или по кнопке, удалять ли лишнее, исправлять ли ручные правки.

Два статуса, которые видно в UI и в `kubectl -n argocd get applications`:

| Статус | Значения | Смысл |
|---|---|---|
| **Sync** | `Synced` / `OutOfSync` | Совпадает ли кластер с Git |
| **Health** | `Healthy` / `Progressing` / `Degraded` / `Missing` | Работает ли то, что применено: Deployment раскатан, поды Ready и т. д. |

Может быть `Synced` + `Degraded`: применили ровно то, что в Git, но под в CrashLoopBackOff. Это сценарий `1-bad-image`, если бы он шёл через ArgoCD.

## Цикл работы (reconciliation)

```
каждые 60 с (+ до 10 с jitter)            при любом изменении объектов в кластере
            │                                           │
            ▼                                           ▼
 repo-server: git fetch → helm template     controller: watch через API
            │                                           │
            └──────────► controller: diff (Git vs кластер) ◄──┘
                                 │
                 есть разница? ──┴── automated? ── да ──► kubectl apply (server-side diff/apply)
                                                  нет ──► OutOfSync, ждёт кнопку Sync
```

- 60 с мы задали сами (`timeout.reconciliation` в `values/argocd.yaml`). В проде обычно ставят webhook из GitHub → ArgoCD, и синк идёт сразу после push. До ноутбука webhook не достучится, поэтому опрос.
- Кнопка **Refresh** в UI = «проверь Git прямо сейчас», не дожидаясь минуты.
- **Hard Refresh** — то же, но со сбросом кеша рендера.

## Последовательность запуска (что происходит после `scripts/bootstrap.sh`)

1. kind создаёт 3 контейнера-ноды, внутри каждой kubelet и containerd.
2. Helm ставит ArgoCD (релиз `argocd`, namespace `argocd`) и ждёт, пока поды готовы.
3. `kubectl apply -f deploy/root.yaml` — появляется Application `root`.
4. ArgoCD синкает `root` → читает `deploy/apps/` из GitHub → создаёт 3 Application: `argocd`, `monitoring`, `wiki`.
5. Волна 0: `argocd` и `monitoring`.
   - `argocd`: ArgoCD «усыновляет» свой же Helm-релиз — объекты уже есть, получается почти пустой diff.
   - `monitoring`: ставит CRD Prometheus Operator, Prometheus, Grafana, Alertmanager.
6. Волна 1: `wiki` — Deployment, Service, loadgen, ServiceMonitor, PrometheusRule, ConfigMap с дашбордом.
7. Prometheus Operator видит ServiceMonitor → дописывает конфиг Prometheus → тот начинает скрейпить поды вики. Видит PrometheusRule → загружает правила SLO. Grafana-sidecar видит ConfigMap → загружает дашборд.

---

## `deploy/kind-config.yaml`

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: lab
```
Конфиг для kind (не объект Kubernetes). `name: lab` → кластер `lab`, контекст kubectl `kind-lab`, Docker-контейнеры `lab-control-plane`, `lab-worker`, `lab-worker2`.

```yaml
nodes:
  - role: control-plane
    extraPortMappings:
      - {containerPort: 30000, hostPort: 8080, listenAddress: 127.0.0.1}  # wiki
      ...
  - role: worker
  - role: worker
```
- Каждая «нода» — Docker-контейнер.
- `extraPortMappings` = `docker run -p 127.0.0.1:8080:30000` для контейнера control-plane.
- NodePort (порт 30000–32767, который kube-proxy открывает на **каждой** ноде и направляет в Service) достаточно пробросить с одной ноды: трафик дойдёт до пода на любой ноде.
- `127.0.0.1` — порт доступен только с этого Mac, не из локальной сети.
- Два worker'а нужны, чтобы было куда раскидывать поды и чтобы сценарий Pending показывал «0/3 nodes».

## `scripts/bootstrap.sh` построчно

```bash
1  #!/usr/bin/env bash
```
Shebang: запустить bash, найденный через `PATH` (на Mac `/bin/bash` старый 3.2, через env берётся brew-версия, если она стоит раньше в PATH).

```bash
3  set -euo pipefail
```
- `-e` — выйти при первой ошибке.
- `-u` — ошибка при обращении к неустановленной переменной (защита от опечаток).
- `-o pipefail` — пайп считается упавшим, если упала любая команда в нём, а не только последняя.

```bash
4  cd "$(dirname "$0")/.."
```
`$0` — путь к скрипту. `dirname` → `scripts`, `/..` → корень репо. Скрипт работает из любой текущей папки, и относительные пути вроде `deploy/...` правильные.

```bash
5  ARGOCD_CHART_VERSION=$(awk '/chart: argo-cd/{f=1} f&&/targetRevision/{print $2; exit}' deploy/apps/argocd.yaml)
```
Берёт версию чарта из `apps/argocd.yaml`: после строки `chart: argo-cd` найти первую `targetRevision` и напечатать значение (`10.12.0`). Версия хранится в одном месте. bootstrap ставит ту же версию, которой потом будет управлять ArgoCD, иначе ArgoCD сразу начал бы апгрейд или даунгрейд.

```bash
7  kind get clusters | grep -qx lab || kind create cluster --config deploy/kind-config.yaml
```
Если кластера `lab` нет — создать. `grep -q` — тихо, только код возврата; `-x` — строка целиком (`lab`, а не `lab2`). `||` — выполнить правую часть, если левая неуспешна. Повторный запуск скрипта не пытается создать кластер ещё раз (идемпотентность — повторный запуск даёт тот же результат).

```bash
8  kubectl config use-context kind-lab
```
Переключить kubectl на этот кластер. Защита от того, чтобы поставить ArgoCD в рабочий кластер, если текущий контекст — он.

```bash
10 helm repo add argo https://argoproj.github.io/argo-helm >/dev/null
11 helm repo update argo >/dev/null
```
Добавить Helm-репозиторий проекта Argo под именем `argo` и обновить его индекс (список чартов и версий). Если репо уже добавлен — команда просто пройдёт.

```bash
12 helm upgrade --install argocd argo/argo-cd --version "$ARGOCD_CHART_VERSION" \
13   -n argocd --create-namespace -f deploy/values/argocd.yaml --wait --timeout 10m
```
- `upgrade --install` — установить, если релиза нет, иначе обновить. Тоже идемпотентно.
- `argocd` — имя релиза. Оно же в `apps/argocd.yaml` (`releaseName: argocd`), поэтому ArgoCD потом узнаёт свои объекты.
- `argo/argo-cd` — репо/чарт.
- `-n argocd --create-namespace` — namespace, создать при отсутствии.
- `-f` — наши values.
- `--wait` — вернуться, только когда Deployment'ы готовы, а не сразу после отправки манифестов в API. Иначе следующий шаг мог бы упасть: CRD Application ещё не зарегистрирован.
- `--timeout 10m` — сколько ждать.

```bash
15 kubectl apply -f deploy/root.yaml
```
Единственный ручной `kubectl apply`. С этого момента всем управляет Git.

```bash
17 password=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)
```
При первом старте ArgoCD генерирует пароль admin и кладёт в Secret. `jsonpath` достаёт поле. Данные в Secret хранятся в base64 (это кодирование, не шифрование), `base64 -d` декодирует. В проде этот Secret удаляют после первого входа и смены пароля или включают SSO.

```bash
18 cat <<MSG
...
28 MSG
```
Heredoc — многострочный текст в stdout. `$password` внутри подставляется, потому что `MSG` без кавычек.

## `deploy/values/argocd.yaml`

```yaml
configs:
  params:
    server.insecure: true
```
`configs.params` → ConfigMap `argocd-cmd-params-cm` (параметры запуска компонентов). `server.insecure` — argocd-server отдаёт HTTP без своего TLS. Иначе браузер ругался бы на самоподписанный сертификат. В проде TLS снимает Ingress или балансировщик.

```yaml
  cm:
    timeout.reconciliation: 60s
    timeout.reconciliation.jitter: 10s
```
`configs.cm` → ConfigMap `argocd-cm` (основные настройки). Опрос Git раз в 60 с (по умолчанию 120 с) плюс случайная добавка до 10 с, чтобы много Application не ходили в Git одновременно.

```yaml
server:
  service:
    type: NodePort
    nodePortHttp: 30001
```
Service argocd-server — NodePort 30001 → kind → http://localhost:8081.

```yaml
dex:
  enabled: false
notifications:
  enabled: false
```
Не ставить SSO и уведомления — меньше подов и памяти.

## `deploy/root.yaml` — app of apps

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: root
  namespace: argocd
```
Application всегда создаются в namespace самого ArgoCD (по умолчанию он смотрит только туда).

```yaml
spec:
  project: default
```
AppProject — группа Application с ограничениями: из каких репо, в какие namespace и кластеры можно деплоить. `default` разрешает всё. В проде делают проект на команду.

```yaml
  source:
    repoURL: https://github.com/cibexon/my_cv.git
    targetRevision: main
    path: deploy/apps
```
Источник — папка `deploy/apps` ветки `main`. Это не Helm-чарт (нет `Chart.yaml`), поэтому ArgoCD считает её папкой с обычными YAML и применяет все файлы. Репо публичный — доступ без кредов.

```yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
```
Куда: этот же кластер (внутренний адрес API-сервера), namespace `argocd` — сюда ложатся дочерние Application.

```yaml
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```
- `automated` — синкать без кнопки, как только Git изменился.
- `prune` — удалил файл из `deploy/apps` → удалится Application. Вместе с ним и всё, что он поставил: у дочерних Application стоит финализатор (метка, которая не даёт удалить объект, пока контроллер не выполнит очистку).
- `selfHeal` — кто-то поменял Application руками (`kubectl edit`) → вернуть как в Git.

## `deploy/apps/argocd.yaml` — ArgoCD управляет сам собой

```yaml
  sources:
    - repoURL: https://argoproj.github.io/argo-helm
      chart: argo-cd
      targetRevision: 10.12.0
      helm:
        releaseName: argocd
        valueFiles:
          - $values/deploy/values/argocd.yaml
    - repoURL: https://github.com/cibexon/my_cv.git
      targetRevision: main
      ref: values
```
Multi-source — несколько источников в одном Application:
- **Первый** — чарт `argo-cd` версии `10.12.0` из Helm-репозитория (`targetRevision` для Helm-репо = версия чарта, для Git = ветка, тег или коммит).
- **Второй** ничего не деплоит. `ref: values` даёт ему имя, и `$values/...` в первом источнике означает «файл из этого Git-репо».

Так чужой чарт берётся из его репозитория, а наши values лежат у нас. Копировать чарт к себе не нужно.

`releaseName: argocd` — то же имя, что в bootstrap, поэтому имена объектов совпадают, и ArgoCD подхватывает уже существующие.

```yaml
  syncPolicy:
    automated:
      prune: false
      selfHeal: true
```
`prune: false` — даже если из-за ошибки рендер окажется пустым, ArgoCD не удалит сам себя.

Как обновить ArgoCD: поменять `targetRevision: 10.12.0` → новую версию → push. Важно: `ARGOCD_CHART_VERSION` в bootstrap берётся из этого же файла.

## `deploy/apps/monitoring.yaml`

```yaml
  annotations:
    argocd.argoproj.io/sync-wave: "0"
```
Sync wave (волна синхронизации) — порядок применения внутри одного синка: сначала все объекты волны 0, ждём их Health, потом волна 1. Здесь объекты — это Application внутри `root`. Monitoring раньше wiki, потому что приносит CRD ServiceMonitor и PrometheusRule.

```yaml
  sources:
    - repoURL: https://prometheus-community.github.io/helm-charts
      chart: kube-prometheus-stack
      targetRevision: 92.18.0
      helm:
        releaseName: monitoring
        valueFiles:
          - $values/deploy/values/kube-prometheus-stack.yaml
    - repoURL: https://github.com/cibexon/my_cv.git
      targetRevision: main
      ref: values
```
Тот же приём multi-source. `releaseName: monitoring` влияет на имена объектов: `monitoring-grafana`, `monitoring-kube-prometheus-prometheus`.

```yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: monitoring
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
      - ServerSideApply=true
```
- `CreateNamespace=true` — создать `monitoring`, если его нет.
- `ServerSideApply=true` — применять через server-side apply: API-сервер сам сливает изменения и помнит, какое поле чьё. При обычном (client-side) apply весь объект пишется в аннотацию `last-applied-configuration`, а у CRD Prometheus она больше лимита 256 КБ → ошибка `metadata.annotations: Too long`.

## `deploy/apps/wiki.yaml`

```yaml
  annotations:
    argocd.argoproj.io/sync-wave: "1"
```
После monitoring.

```yaml
  source:
    repoURL: https://github.com/cibexon/my_cv.git
    targetRevision: main
    path: charts/wiki
    helm:
      releaseName: wiki
```
В `path` лежит `Chart.yaml` → ArgoCD понимает, что это Helm, и рендерит его с `values.yaml` из этой папки. Так CI и деплоит: меняет `values.yaml`, и следующий рендер содержит новый тег. `releaseName: wiki` + чарт `wiki` → объекты называются просто `wiki` (см. `wiki.fullname` в `_helpers.tpl`).

```yaml
    syncOptions:
      - CreateNamespace=true
      - SkipDryRunOnMissingResource=true
```
Перед применением ArgoCD делает dry-run (пробное применение без записи). Если CRD ServiceMonitor ещё нет, dry-run упал бы с `no matches for kind ServiceMonitor`. Опция говорит: такие объекты не проверять, а пробовать применить.

```yaml
    retry:
      limit: 10
      backoff:
        duration: 30s
        factor: 2
        maxDuration: 5m
```
Если синк упал (например, CRD ещё создаётся), повторить до 10 раз с паузой 30 с → 1 мин → 2 мин → … не больше 5 мин.

## `deploy/values/kube-prometheus-stack.yaml`, главное

- `grafana.sidecar.dashboards.searchNamespace: ALL` — sidecar ищет ConfigMap с дашбордами во всех namespace (наш — в `wiki`).
- `prometheus.prometheusSpec.*SelectorNilUsesHelmValues: false` — по умолчанию Prometheus Operator берёт только ServiceMonitor и PrometheusRule с меткой `release: monitoring`. `false` = брать все. Без этого Prometheus не увидел бы наш ServiceMonitor, и графики были бы пустыми — частая ошибка.
- `kubeEtcd/kubeControllerManager/kubeScheduler/kubeProxy.enabled: false` — в kind эти компоненты слушают только `127.0.0.1` внутри ноды. Prometheus до них не достанет, и алерты `TargetDown` горели бы постоянно.

## `scripts/break.sh` — почему вне ArgoCD

```bash
tag=$(awk '/^  tag:/{print $2}' charts/wiki/values.yaml)
```
Берёт тот же тег, что сейчас в Git: образ точно есть в GHCR. Исключение — сценарий 1, где тег намеренно ломается.

```bash
deploy() {
  helm upgrade --install lab charts/wiki -n lab --create-namespace \
    -f scenarios/_lab.yaml -f <(printf 'image:\n  tag: %s\n' "$tag") "$@" >/dev/null
}
```
Обычный Helm-релиз `lab` в namespace `lab`. Values складываются слоями, **каждый следующий `-f` перекрывает предыдущий**:

1. `_lab.yaml` — база (ClusterIP, без мониторинга);
2. `<(printf …)` — process substitution: bash подставляет вместо этого путь вида `/dev/fd/63`, из которого читается сгенерированный на лету YAML с рабочим тегом;
3. `"$@"` — аргументы функции, например `-f scenarios/1-bad-image.yaml`. Сценарий последний и перекрывает всё.

Почему не `--set image.tag=…`: `--set` в Helm всегда главнее любых `-f`, независимо от порядка. С ним сценарий `1-bad-image` подставлял бы рабочий тег вместо `v9.9.9` и ничего не ломал. Так и было в первой версии скрипта.

Почему не через ArgoCD: со `selfHeal: true` ArgoCD вернул бы рабочие values быстрее, чем ты начнёшь искать причину. Для Helm-релизов, поставленных руками, `helm list -n lab` и `helm history lab -n lab` работают как обычно. Можно потренировать `helm rollback lab 1 -n lab`.

## Полезные команды

```bash
kubectl -n argocd get applications                    # Sync / Health всех приложений
kubectl -n argocd describe application wiki           # события и ошибки синка
kubectl -n argocd get application wiki -o jsonpath='{.status.sync.revision}'   # какой коммит сейчас применён
kubectl -n argocd logs deploy/argocd-repo-server      # ошибки рендера Helm и доступа к Git
kubectl -n argocd logs statefulset/argocd-application-controller   # ошибки применения
helm list -A                                          # Helm-релизы: argocd (от bootstrap) и lab; wiki/monitoring тут нет
```

В UI: клик по Application → дерево объектов. **App Diff** — что поменяется, **History and Rollback** — прошлые синки по коммитам.
