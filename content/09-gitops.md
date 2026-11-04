# Helm, Kustomize, ArgoCD (GitOps)

Подробно о том, как управлять манифестами Kubernetes для многих сервисов и окружений и как их доставлять в кластер: Helm (шаблоны и пакеты), Kustomize (патчи без шаблонов), GitOps и ArgoCD. Каждая тема: что это, зачем, как работает, примеры с объяснением и типичные проблемы.

## Какую проблему решают эти инструменты

Один сервис в Kubernetes — это Deployment, Service, Ingress, ConfigMap, HPA, PDB, ServiceAccount: 200–300 строк YAML. В трёх окружениях (dev, stage, prod) они отличаются немногим: число реплик, ресурсы, домен, тег образа. А сервисов — 30.

Копировать YAML для каждого сервиса и окружения нельзя: изменение стандарта (например, добавить securityContext) придётся вносить в 90 местах, и копии неизбежно разъедутся. Нужен способ описать общее один раз, а различия — отдельно. Два основных подхода:
- **Helm** — шаблоны с параметрами: один шаблон и разные значения параметров;
- **Kustomize** — базовые манифесты и патчи поверх них для каждого окружения.

### 🧪 Мини-задание

**Задача.** Сымитируй жизнь без шаблонов: манифест для stage скопировали в prod и поправили руками. Сравни два файла через `diff` и найди, какие отличия задуманы, а какие — случайный дрейф.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > stage.yaml <<'EOF'
replicas: 1
image: shop/api:1.4.0
env: stage
readinessProbe: /healthz
memoryLimit: 256Mi
EOF
cat > prod.yaml <<'EOF'
replicas: 3
image: shop/api:1.3.2
env: prod
readinessProbe: /health
memoryLimit: 256Mi
EOF
diff stage.yaml prod.yaml
```

Ожидаемый результат:

```text
1,4c1,4
< replicas: 1
< image: shop/api:1.4.0
< env: stage
< readinessProbe: /healthz
---
> replicas: 3
> image: shop/api:1.3.2
> env: prod
> readinessProbe: /health
```

Разбор: `replicas` и `env` должны отличаться — это параметры окружения. А вот другой путь readinessProbe и отставшая версия образа — дрейф: кто-то правил prod руками, и теперь stage больше не проверяет то, что поедет в прод. На десятках сервисов такие копии расходятся незаметно. Helm и Kustomize решают это так: общая часть описана один раз, а окружения задают только явные отличия (values-файлы или overlay), которые видно на ревью.

</details>

## Helm

### Основные понятия
- **Chart (чарт)** — пакет: шаблоны манифестов, значения по умолчанию (`values.yaml`) и метаданные.
- **Values** — параметры, подставляемые в шаблоны. Задаются файлами (`-f values-prod.yaml`) и флагами (`--set image.tag=1.4.2`).
- **Release (релиз)** — установленный в кластер экземпляр чарта с конкретным именем. Один чарт можно установить много раз под разными именами.
- **Revision (ревизия)** — каждая установка или обновление релиза создаёт новую ревизию. История ревизий хранится в кластере (в Secret'ах в namespace релиза), поэтому возможен **откат** на любую предыдущую.
- **Repository** — место публикации чартов: HTTP-репозиторий или OCI-registry (registry, хранящий артефакты в том же формате OCI, что и образы контейнеров, — например ECR или GitLab registry, где лежат образы).

### Структура чарта
```
mychart/
  Chart.yaml            # имя, версия чарта, appVersion, зависимости от других чартов
  values.yaml           # значения по умолчанию
  values.schema.json    # JSON-схема для проверки values (необязательно, но полезно)
  templates/
    _helpers.tpl        # именованные шаблоны: имена, метки — переиспользуются в других файлах
    deployment.yaml
    service.yaml
    ingress.yaml
    hpa.yaml
    NOTES.txt           # текст, который выводится после установки
  charts/               # скачанные зависимости
```
```yaml
# Chart.yaml
apiVersion: v2
name: api
version: 1.3.0          # версия ЧАРТА (меняется при изменении шаблонов)
appVersion: "2.7.1"     # версия ПРИЛОЖЕНИЯ (тег образа по умолчанию)
dependencies:
  - name: redis
    version: "~19.0"
    repository: oci://registry-1.docker.io/bitnamicharts
    condition: redis.enabled   # ставить зависимость, только если в values redis.enabled = true
```

### Шаблоны
Шаблоны пишутся на языке **Go templates** (встроенный в Go шаблонизатор) с библиотекой функций Sprig (около 100 готовых функций: строки, списки, хеши, даты). Внутри `{{ }}` — выражения.
```yaml
# values.yaml
replicaCount: 2
image:
  repository: registry.example.com/api
  tag: ""                 # пусто — возьмём appVersion из Chart.yaml
resources:
  requests: { cpu: 100m, memory: 128Mi }
env:
  LOG_LEVEL: info
ingress:
  enabled: false
  host: ""
```
```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "api.fullname" . }}          # именованный шаблон из _helpers.tpl
  labels:
    {{- include "api.labels" . | nindent 4 }}   # вставить блок меток с отступом 4 пробела
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "api.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "api.selectorLabels" . | nindent 8 }}
      annotations:
        # хеш ConfigMap: при изменении конфига меняется шаблон пода → Deployment перезапустит поды
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
    spec:
      containers:
        - name: api
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          {{- with .Values.resources }}          # with: если значение не пустое — войти в блок, точка = это значение
          resources:
            {{- toYaml . | nindent 12 }}         # превратить структуру из values в YAML с отступом 12
          {{- end }}
          {{- if .Values.env }}
          env:
            {{- range $key, $value := .Values.env }}   # цикл по словарю
            - name: {{ $key }}
              value: {{ $value | quote }}               # quote — в кавычках, чтобы "true" и "123" остались строками
            {{- end }}
          {{- end }}
```
Что важно понимать:
- **`.Values`** — значения, **`.Chart`** — данные из Chart.yaml, **`.Release.Name`**, **`.Release.Namespace`** — имя и namespace релиза.
- **`{{-` и `-}}`** — удалить пробелы и переводы строк слева или справа от выражения. Главный источник ошибок в чартах — сломанные отступы YAML. Поэтому используют `nindent` (перевод строки плюс отступ) и `toYaml`.
- **`include`** вызывает именованный шаблон из `_helpers.tpl` (имена и метки описывают один раз).
- **`required`** — заставить задать значение: `{{ required "ingress.host обязателен" .Values.ingress.host }}`.
- Целые объекты можно включать по условию: весь файл `ingress.yaml` обёрнут в `{{- if .Values.ingress.enabled }} ... {{- end }}`.

**Аннотация `checksum/config`** — решение частой проблемы: поды не перезапускаются при изменении ConfigMap (переменные окружения читаются только при старте). Хеш содержимого ConfigMap в аннотации шаблона пода меняется вместе с конфигом, Deployment видит изменение шаблона и перекатывает поды.

### Окружения через values-файлы
```
values.yaml          # общие значения по умолчанию
values-dev.yaml      # replicaCount: 1, маленькие ресурсы
values-prod.yaml     # replicaCount: 4, большие ресурсы, ingress.host: api.example.com
```
```bash
helm upgrade --install api ./mychart -n prod -f values-prod.yaml --set image.tag=2.7.2
# порядок приоритета: values.yaml < -f файлы (в порядке указания) < --set
```

### Команды
```bash
helm repo add bitnami https://charts.bitnami.com/bitnami && helm repo update   # Bitnami — популярный каталог готовых чартов (PostgreSQL, Redis, Kafka)
helm search repo postgresql
helm show values bitnami/postgresql > pg-values.yaml   # посмотреть все параметры чарта

helm upgrade --install api ./mychart \      # установить, если нет, или обновить, если есть
  -n prod --create-namespace \
  -f values-prod.yaml \
  --set image.tag=2.7.2 \
  --atomic --wait --timeout 5m              # ждать готовности; при неудаче — автоматический откат

helm template api ./mychart -f values-prod.yaml   # отрендерить манифесты локально, ничего не применяя
helm lint ./mychart                                # проверка чарта
helm diff upgrade api ./mychart -f values-prod.yaml   # плагин helm-diff: что изменится в кластере

helm list -A                                # все релизы
helm history api -n prod                    # ревизии релиза
helm rollback api 3 -n prod                 # откат на ревизию 3
helm get values api -n prod                 # с какими values установлен релиз
helm get manifest api -n prod               # какие манифесты реально применены
helm uninstall api -n prod

helm package ./mychart && helm push api-1.3.0.tgz oci://registry.example.com/charts
```
**`--atomic`** важен в CI: без него при неудачной раскатке релиз остаётся в состоянии `failed`, а с ним Helm сам откатывает на предыдущую рабочую ревизию.

### Хуки
**Хук** — ресурс чарта (обычно Job), который Helm создаёт не вместе с остальными, а в определённый момент жизненного цикла релиза. Типичные задачи: миграция схемы БД перед новой версией приложения, бэкап перед обновлением, прогрев кэша или уведомление после установки, проверочный тест.

Все точки:
| Хук | Когда выполняется |
|---|---|
| `pre-install` | после рендера шаблонов, до создания любых ресурсов при `helm install` |
| `post-install` | после создания всех ресурсов при установке |
| `pre-upgrade` | перед обновлением ресурсов при `helm upgrade` (классика — **миграции БД**) |
| `post-upgrade` | после обновления ресурсов |
| `pre-rollback`, `post-rollback` | до и после `helm rollback` |
| `pre-delete`, `post-delete` | до и после удаления ресурсов при `helm uninstall` |
| `test` | только по команде `helm test <релиз>` (проверка, что релиз работает) |

```yaml
# templates/db-migrate-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .Release.Name }}-db-migrate
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "-5"                     # порядок среди хуков одной фазы: меньше — раньше
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 1                                   # не повторять миграцию много раз
  activeDeadlineSeconds: 600                        # убить, если идёт дольше 10 минут
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: migrate
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"   # та же версия, что и приложение
          command: ["/app/migrate", "up"]
          envFrom:
            - secretRef:
                name: {{ .Release.Name }}-db
```
Как это работает:
- Helm создаёт хук и **ждёт**, пока Job завершится (для Job и Pod — до успешного завершения или ошибки, для остальных типов — только создания). Если Job упал — релиз помечается `failed`, основные ресурсы `pre-upgrade` не обновляются. Время ожидания — `--timeout` (по умолчанию 5 минут);
- `hook-weight` — строка с числом; хуки одной фазы выполняются по возрастанию веса, при равном весе — по типу и имени;
- `hook-delete-policy`: `before-hook-creation` (удалить старый Job перед созданием нового — иначе второй `upgrade` упадёт с «already exists»; это поведение по умолчанию, если политика не указана), `hook-succeeded` (удалить после успеха), `hook-failed` (удалить после ошибки — тогда логи упавшей миграции пропадут, поэтому обычно не используют);
- хуки **не входят в релиз**: `helm uninstall` их не удаляет, `helm rollback` не откатывает их действие — миграция, которую выполнил `pre-upgrade`, останется применённой.

Подводные камни, о которых спрашивают:
- **Миграция и откат.** Helm откатит Deployment, но не схему БД. Поэтому миграции пишут **обратно совместимыми** (expand/contract: сначала добавить колонку, выкатить код, который пишет в обе, и только в следующем релизе удалить старую), чтобы старая версия приложения работала на новой схеме.
- **Курица и яйцо при `pre-install`.** Хук выполняется до создания ресурсов чарта, поэтому Secret или ConfigMap из того же чарта ещё не существует — Job не стартует. Решения: сделать нужный Secret тоже хуком с меньшим весом или брать секрет из внешнего источника (External Secrets, уже созданный секрет).
- **Несколько реплик и параллельные релизы.** Хук выполняется один раз на релиз, но две параллельные выкатки (два пайплайна) запустят две миграции. Защита — блокировка в самом инструменте миграций (Flyway, Liquibase, golang-migrate держат lock в БД) и запрет параллельных деплоев в CI.
- **`--wait` и хуки.** `post-install` выполняется после создания ресурсов, но без `--wait` — не дожидаясь, пока поды станут Ready.
- **Долгие миграции** блокируют деплой и упираются в `--timeout`. Тяжёлые изменения данных выносят из хуков в отдельные задачи.

**Хуки и ArgoCD.** ArgoCD не запускает `helm install`, а рендерит шаблоны и применяет их сам, поэтому Helm-хуки он переводит в свои: `pre-install` и `pre-upgrade` → `PreSync`, `post-install` и `post-upgrade` → `PostSync`, `post-delete` → `PostDelete` (в новых версиях ArgoCD), `hook-delete-policy` → соответствующие политики удаления ArgoCD. Различий между установкой и обновлением у ArgoCD нет: хук `pre-install` выполнится **при каждой синхронизации**, а `pre-rollback`, `post-rollback`, `test` и `pre-delete` как Helm-хуки не поддерживаются. Поэтому для чартов под ArgoCD хуки пишут сразу с аннотациями ArgoCD (`argocd.argoproj.io/hook: PreSync`) и делают их идемпотентными. Порядок внутри фазы — `argocd.argoproj.io/sync-wave`.

**`helm test`** — хук `test`: под или Job, который проверяет установленный релиз (например, `curl` к сервису). Удобно запускать в CI сразу после `helm upgrade`:
```bash
helm upgrade --install shop ./chart -n prod --wait --timeout 10m
helm test shop -n prod --logs          # запустить тестовые хуки и показать их логи
```

### Helmfile
Декларативное описание набора релизов: какие чарты, с какими values, в каких окружениях. `helmfile apply` приводит кластер к описанному состоянию. Удобно для инфраструктурных аддонов (ingress, cert-manager, мониторинг) без GitOps.

### 🧪 Мини-задание

**Задача.** Создай учебный чарт через `helm create`, проверь его линтером и отрендери без кластера: с значениями по умолчанию и с `prod`-values (3 реплики, свой тег образа). Сравни число реплик и образ в Deployment.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
helm create shop > /dev/null
helm lint shop | tail -1
printf 'replicaCount: 3\nimage:\n  tag: "1.27"\n' > values-prod.yaml
helm template web ./shop | yq 'select(.kind == "Deployment") | [.spec.replicas, .spec.template.spec.containers[0].image] | join(" ")'
helm template web ./shop -f values-prod.yaml | yq 'select(.kind == "Deployment") | [.spec.replicas, .spec.template.spec.containers[0].image] | join(" ")'
```

Ожидаемый результат:

```text
1 chart(s) linted, 0 chart(s) failed
1 nginx:1.16.0
3 nginx:1.27
```

Разбор: `helm template` рендерит шаблоны локально, без кластера, — так проверяют чарт в CI и смотрят, что именно уйдёт в Kubernetes. Значения по умолчанию лежат в `values.yaml` чарта, а `-f values-prod.yaml` перекрывает только указанные ключи, остальное берётся из чарта. Если тег не задан, шаблон подставляет `appVersion` из `Chart.yaml` (`1.16.0`).

</details>

## Kustomize

### Идея
Без шаблонов и своего языка: обычные валидные манифесты Kubernetes (**base**) плюс **overlays** для окружений, которые **патчат** base. Встроен в kubectl: `kubectl apply -k <папка>`.

```
k8s/
  base/
    kustomization.yaml
    deployment.yaml
    service.yaml
  overlays/
    dev/
      kustomization.yaml
    prod/
      kustomization.yaml
      replicas-patch.yaml
```
```yaml
# base/kustomization.yaml
resources:
  - deployment.yaml
  - service.yaml
labels:
  - pairs:
      app.kubernetes.io/part-of: shop     # добавить метку во все ресурсы
```
```yaml
# overlays/prod/kustomization.yaml
resources:
  - ../../base                            # взять базу
namespace: prod                           # проставить namespace во все ресурсы
namePrefix: prod-                         # добавить префикс к именам
images:
  - name: registry.example.com/api        # заменить тег образа без правки манифестов
    newTag: "2.7.2"
patches:
  - path: replicas-patch.yaml             # патч: изменить часть объекта
configMapGenerator:
  - name: api-config
    literals:
      - LOG_LEVEL=warn
```
```yaml
# overlays/prod/replicas-patch.yaml — strategic merge patch: частичный объект, сливается с базой
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 4
  template:
    spec:
      containers:
        - name: api                       # контейнер находится по имени
          resources:
            limits: { memory: 1Gi }
```
```bash
kubectl kustomize overlays/prod          # посмотреть итоговые манифесты
kubectl apply -k overlays/prod           # применить
```
**`configMapGenerator`** создаёт ConfigMap с **хешем содержимого в имени** (`api-config-7g2k9h`) и сам подставляет это имя в Deployment. Изменился конфиг — изменилось имя — изменился шаблон пода — поды перекатились. Тот же эффект, что `checksum/config` в Helm.

### Helm или Kustomize
| | Helm | Kustomize |
|---|---|---|
| Подход | Шаблоны с параметрами | Патчи поверх обычных манифестов |
| Сильные стороны | Распространение готовых пакетов (сторонние чарты), сложная параметризация, релизы с историей и откатом, хуки | Простота, нет шаблонного языка, манифесты остаются читаемым YAML |
| Слабые стороны | Шаблоны трудно читать и отлаживать (отступы, условия) | Сложную логику и условия выражать неудобно, нет релизов и отката |
| Типичное применение | Сторонние компоненты (ingress-nginx, cert-manager, мониторинг), общий чарт компании для сервисов | Свои сервисы с небольшими различиями между окружениями |

Часто их **комбинируют**: Kustomize умеет рендерить Helm-чарт и патчить результат (`helmCharts` в kustomization), а ArgoCD поддерживает оба.

### 🧪 Мини-задание

**Задача.** Сделай `base` с Deployment и overlay `prod`, который добавляет префикс имени, метку окружения, 3 реплики через патч и новый тег образа. Отрендери overlay и проверь результат.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && mkdir -p base overlays/prod
cat > base/deployment.yaml <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      containers:
        - name: web
          image: nginx:1.25
EOF
echo 'resources: [deployment.yaml]' > base/kustomization.yaml
cat > overlays/prod/kustomization.yaml <<'EOF'
resources: [../../base]
namePrefix: prod-
labels:
  - pairs: {env: prod}
images:
  - name: nginx
    newTag: "1.27"
patches:
  - target: {kind: Deployment, name: web}
    patch: |
      - op: replace
        path: /spec/replicas
        value: 3
EOF
kustomize build overlays/prod | yq '[.metadata.name, .metadata.labels.env, .spec.replicas, .spec.template.spec.containers[0].image] | join(" ")'
```

Ожидаемый результат:

```text
prod-web prod 3 nginx:1.27
```

Разбор: Kustomize не использует шаблоны: base — обычный валидный YAML, а overlay описывает, *что изменить*. `namePrefix` поменял имя, `labels` добавил метку, `images` заменил тег, не трогая YAML Deployment, а JSON-патч поменял `replicas`. Базу можно применить и без overlay. Тот же результат даёт `kubectl kustomize overlays/prod` — Kustomize встроен в kubectl. По умолчанию `labels` не добавляет метку в селектор, и это правильно: селектор Deployment менять нельзя.

</details>

## GitOps

### Принципы
GitOps — способ доставки, при котором **git-репозиторий является единственным источником правды** о желаемом состоянии кластера.
1. **Декларативность** — всё состояние описано манифестами.
2. **Версионирование** — описание лежит в git: история, ревью, откат через `git revert`.
3. **Pull** — **агент внутри кластера** сам забирает изменения из git и применяет. CI в кластер не ходит.
4. **Непрерывная сверка (reconciliation)** — агент постоянно сравнивает кластер с git и исправляет расхождения (drift).

### Push против pull
**Push-модель** (классический CI/CD): пайплайн выполняет `helm upgrade` или `kubectl apply`.
- CI нужен доступ на запись в кластер — его компрометация означает компрометацию кластера.
- Ручные изменения в кластере никто не замечает.
- Чтобы узнать, что развёрнуто, надо смотреть в кластер.

**Pull-модель** (GitOps): ArgoCD или Flux (два самых популярных GitOps-агента для Kubernetes, оба проекты CNCF) в кластере следит за git.
- CI не нужны креды кластера: он только собирает образ и обновляет тег в git.
- **Видно расхождение**: если кто-то поменял ресурс руками, агент покажет OutOfSync и может вернуть состояние.
- **Аудит и откат** — через историю git.
- **Восстановление**: новый кластер приводится к нужному состоянию просто подключением к тому же репозиторию.

Минусы GitOps: императивные шаги (миграции, разовые операции) выражать сложнее; нужен отдельный механизм для секретов (их нельзя класть в git открытым текстом); дополнительный компонент в кластере.

### Типичная схема репозиториев
- **Репозиторий приложения** — код и Dockerfile. CI: тесты → сборка образа с тегом SHA → публикация в registry → **коммит нового тега в репозиторий конфигурации** (для prod — merge request, который подтверждает человек).
- **Репозиторий конфигурации (deploy/gitops)** — Helm values или Kustomize overlays для всех сервисов и окружений. За ним следит ArgoCD.

Разделение нужно, чтобы изменения конфигурации не запускали сборку кода, а права на прод-конфигурацию можно было ограничить отдельно.

### 🧪 Мини-задание

**Задача.** Заведи «GitOps-репозиторий» с одним параметром — числом реплик. Увеличь его коммитом, затем откати через `git revert` и после каждого шага покажи, какое состояние сейчас описано в git и что об этом говорит история.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo 'replicas: 2' > values-prod.yaml && git add . && git commit -qm "prod: initial 2 replicas"
state() { echo "git says: $(yq .replicas values-prod.yaml) replicas"; }
state
yq -i '.replicas = 5' values-prod.yaml && git commit -qam "prod: scale to 5 for sale"
state
git revert -q --no-edit HEAD
state
git log --format='%s'
```

Ожидаемый результат:

```text
git says: 2 replicas
git says: 5 replicas
git says: 2 replicas
Revert "prod: scale to 5 for sale"
prod: scale to 5 for sale
prod: initial 2 replicas
```

Разбор: в GitOps желаемое состояние кластера — это содержимое git, а агент в кластере (ArgoCD, Flux) постоянно приводит кластер к нему. Поэтому любое изменение — это коммит (ревью через merge request, автор и время видны), а откат — `git revert`, тоже коммит: история не переписывается, и видно, кто, что и когда откатил. Если кто-то поменяет replicas через `kubectl scale` в обход git, агент покажет OutOfSync, а с self-heal вернёт значение из git.

</details>

## ArgoCD

### Основной объект — Application
Application связывает **источник** (репозиторий, путь, ветка или Helm-чарт) с **назначением** (кластер и namespace) и задаёт политику синхронизации.
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: api-prod
  namespace: argocd
spec:
  project: prod                                     # AppProject — ограничения прав (см. ниже)
  source:
    repoURL: https://gitlab.com/mycompany/deploy.git
    targetRevision: main                            # ветка, тег или коммит
    path: apps/api/overlays/prod                    # Kustomize-оверлей (ArgoCD сам определит тип)
  destination:
    server: https://kubernetes.default.svc          # этот же кластер
    namespace: api
  syncPolicy:
    automated:
      prune: true          # удалять из кластера ресурсы, которых больше нет в git
      selfHeal: true       # откатывать ручные изменения в кластере к состоянию из git
    syncOptions:
      - CreateNamespace=true
```

### Два статуса приложения
- **Sync status** — совпадает ли кластер с git: **Synced** или **OutOfSync** (в git есть изменения, ещё не применённые, или кто-то поменял ресурс руками).
- **Health status** — здоровы ли ресурсы: **Healthy**, **Progressing** (Deployment раскатывается), **Degraded** (поды падают, раскатка не удалась), **Missing**.

Приложение может быть Synced, но Degraded: всё применено как в git, но новая версия не запускается.

### Синхронизация
- **Ручная** — ArgoCD показывает OutOfSync, а применяет человек кнопкой Sync. Часто так делают для prod.
- **Автоматическая** (`automated`) — изменения применяются сами в течение нескольких минут после коммита (или сразу при вебхуке из git).
- **`prune`** — без него удалённый из git ресурс останется в кластере.
- **`selfHeal`** — без него ручные изменения в кластере только подсвечиваются, но не откатываются.

### Порядок применения: sync waves и hooks
- **Sync waves** — аннотация `argocd.argoproj.io/sync-wave: "-1"`: ресурсы применяются волнами по возрастанию номера. Например, сначала CRD и namespace, затем приложение.
- **Hooks** — `argocd.argoproj.io/hook: PreSync` (выполнить до синхронизации — миграции БД), `PostSync` (после — smoke-тесты), `SyncFail`.

### ignoreDifferences
Некоторые поля меняет не git, а кто-то в кластере. Пример: число реплик меняет HPA. ArgoCD видит, что `replicas` в кластере не совпадает с git, и постоянно показывает OutOfSync (а с selfHeal — откатывает реплики, воюя с HPA). Решения: убрать `replicas` из манифеста Deployment вовсе или настроить игнорирование:
```yaml
spec:
  ignoreDifferences:
    - group: apps
      kind: Deployment
      jsonPointers:
        - /spec/replicas
```

### Масштабирование на много приложений и кластеров
- **App of Apps** — одно «корневое» Application указывает на папку, где лежат манифесты других Application. Добавил файл в папку — появилось новое приложение.
- **ApplicationSet** — генерирует Application по шаблону из **генераторов**:
  - `list` — из явного списка;
  - `git` (directories или files) — по папкам в репозитории: на каждую папку `apps/*` своё приложение;
  - `cluster` — на каждый подключённый к ArgoCD кластер (раскатить мониторинг на все 20 кластеров);
  - `matrix` — комбинация двух генераторов (каждое приложение × каждый кластер).
```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: services-prod
  namespace: argocd
spec:
  generators:
    - git:
        repoURL: https://gitlab.com/mycompany/deploy.git
        revision: main
        directories:
          - path: apps/*/overlays/prod            # каждая папка сервиса → отдельное приложение
  template:
    metadata:
      name: '{{path[1]}}-prod'                    # path[1] — имя сервиса из пути apps/<сервис>/...
    spec:
      project: prod
      source:
        repoURL: https://gitlab.com/mycompany/deploy.git
        targetRevision: main
        path: '{{path}}'
      destination:
        server: https://kubernetes.default.svc
        namespace: '{{path[1]}}'
      syncPolicy:
        automated: { prune: true, selfHeal: true }
```

### Права: AppProject
**AppProject** ограничивает, из каких репозиториев, в какие кластеры и namespace, и какие типы ресурсов может разворачивать группа приложений. Например, проект команды `payments` может деплоить только из своего репозитория и только в namespace `payments-*`, без права создавать ClusterRole. Плюс RBAC ArgoCD для пользователей (кто может нажимать Sync в prod).

### Как ArgoCD работает с Helm
ArgoCD **сам рендерит** чарт (`helm template`) и применяет полученные манифесты. Следствия:
- `helm list` такой релиз **не покажет**: Helm-релиза в кластере нет, управляет ArgoCD;
- откат делается через ArgoCD или `git revert`, а не `helm rollback`;
- Helm-хуки ArgoCD преобразует в свои hooks.

### Обновление образов
Варианты, как новый тег образа попадает в репозиторий конфигурации:
1. **CI коммитит** новый тег (`yq` или `kustomize edit set image`) в deploy-репозиторий — самый прозрачный вариант. Для prod — через merge request.
2. **ArgoCD Image Updater** — следит за registry и сам обновляет тег (в git или в параметрах приложения).

### Прогрессивная доставка
**Argo Rollouts** и **Flagger** заменяют обычный Deployment на ресурс с canary или blue/green стратегией: трафик переводится на новую версию постепенно (5% → 25% → 100%), между шагами автоматически анализируются метрики из Prometheus (доля ошибок, задержка). При ухудшении — автоматический откат.

### Flux — альтернатива ArgoCD
Набор отдельных контроллеров: source-controller (следит за git, Helm-репозиториями, OCI), kustomize-controller, helm-controller (создаёт настоящие Helm-релизы), image-automation (обновление тегов). Без веб-интерфейса по умолчанию, всё настраивается CRD. Нативно поддерживает SOPS для секретов. ArgoCD популярнее благодаря интерфейсу и ApplicationSet.

### 🧪 Мини-задание

**Задача.** Без доступа к серверу ArgoCD сгенерируй манифест `Application` для overlay `overlays/prod` из git-репозитория с автоматической синхронизацией, удалением лишних ресурсов и self-heal. Посмотри источник, назначение и политику синхронизации.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
argocd admin app generate-spec shop-prod \
  --repo https://gitlab.com/mygroup/shop-deploy.git --path overlays/prod --revision main \
  --dest-server https://kubernetes.default.svc --dest-namespace shop \
  --sync-policy automated --auto-prune --self-heal --sync-option CreateNamespace=true |
yq '{"source": .spec.source, "destination": .spec.destination, "syncPolicy": .spec.syncPolicy}'
```

Ожидаемый результат:

```text
source:
  path: overlays/prod
  repoURL: https://gitlab.com/mygroup/shop-deploy.git
  targetRevision: main
destination:
  namespace: shop
  server: https://kubernetes.default.svc
syncPolicy:
  automated:
    prune: true
    selfHeal: true
  syncOptions:
    - CreateNamespace=true
```

Разбор: `Application` связывает источник (репозиторий, ветка, путь) с назначением (кластер и namespace). `automated` — синхронизировать сам при изменении git. `prune: true` — удалять из кластера ресурсы, которые убрали из git (без него они останутся «сиротами»). `selfHeal: true` — откатывать ручные изменения в кластере к состоянию из git. `kubernetes.default.svc` означает кластер, в котором работает сам ArgoCD. Этот YAML кладут в git и применяют (`kubectl apply -n argocd`), а для множества приложений генерируют через ApplicationSet.

</details>

## Секреты в GitOps
В git нельзя хранить секреты открытым текстом, а GitOps требует, чтобы всё было в git. Решения:
- **External Secrets Operator (ESO)** — в git лежит объект `ExternalSecret` со **ссылкой** на секрет в AWS Secrets Manager, SSM, Vault. Оператор сам создаёт и обновляет Kubernetes Secret. Самый распространённый вариант в AWS.
  ```yaml
  apiVersion: external-secrets.io/v1beta1
  kind: ExternalSecret
  metadata:
    name: api-db
  spec:
    refreshInterval: 1h
    secretStoreRef:
      name: aws-secrets-manager
      kind: ClusterSecretStore
    target:
      name: api-db                 # какой Kubernetes Secret создать
    data:
      - secretKey: DB_PASSWORD
        remoteRef:
          key: prod/api/db         # имя секрета в Secrets Manager
          property: password
  ```
- **Sealed Secrets** — секрет шифруется публичным ключом контроллера в кластере (`kubeseal`), в git лежит зашифрованный `SealedSecret`, расшифровать может только контроллер.
- **SOPS** — шифрование значений в YAML-файлах ключом KMS или age (простой современный инструмент шифрования файлов с короткими ключами — замена PGP); Flux расшифровывает нативно, ArgoCD — через плагин.

### 🧪 Мини-задание

**Задача.** Зашифруй Kubernetes Secret через SOPS ключом age так, чтобы были зашифрованы только данные, а метаданные остались читаемыми. Убедись, что пароля в открытом виде в файле нет, и расшифруй его обратно. Нужно: `brew install sops age`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
age-keygen -o key.txt 2> /dev/null
cat > secret.yaml <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: db-credentials
stringData:
  password: hunter2
EOF
sops --encrypt --age "$(grep -o 'age1.*' key.txt)" --encrypted-regex '^(data|stringData)$' secret.yaml > secret.enc.yaml
yq '.metadata.name' secret.enc.yaml
yq '.stringData.password' secret.enc.yaml | cut -c1-20
grep -c hunter2 secret.enc.yaml
SOPS_AGE_KEY_FILE=key.txt sops --decrypt secret.enc.yaml | yq '.stringData.password'
```

Ожидаемый результат:

```text
db-credentials
ENC[AES256_GCM,data:
0
hunter2
```

Разбор: в git можно коммитить `secret.enc.yaml`: значение зашифровано, а имя и тип читаются, поэтому diff на ревью понятен. Расшифровать может только владелец приватного ключа age — в кластере это делает ArgoCD-плагин или Flux с ключом в Secret, а в облаке ключом часто служит KMS. Альтернативы: Sealed Secrets (шифрование ключом контроллера в кластере) и External Secrets Operator (в git только ссылка, значение лежит в Secrets Manager или Vault). Обычный base64-Secret в git — это открытый текст.

</details>

## Вопросы с собеседований

**Чем push-деплой отличается от GitOps (pull)?**
В push CI сам применяет изменения в кластер и должен иметь туда доступ на запись. В pull агент в кластере (ArgoCD, Flux) сам забирает желаемое состояние из git и постоянно сверяет его с кластером. Плюсы pull: CI не нужны креды кластера, видно и исправляется ручное расхождение, аудит и откат через git. Минусы: сложнее императивные шаги, нужен механизм для секретов.

**Как хранить секреты при GitOps?**
External Secrets Operator (в git — ссылка на Secrets Manager или Vault), Sealed Secrets (в git — зашифрованный объект, расшифровывает контроллер), SOPS с KMS.

**Кто-то вручную поменял число реплик. Что сделает ArgoCD?**
С `selfHeal: true` — вернёт значение из git. Без него — покажет OutOfSync. Если репликами управляет HPA, нужно убрать `replicas` из манифеста или добавить `ignoreDifferences`, иначе ArgoCD и HPA будут бесконечно перезаписывать значение.

**Как раскатить приложение на 20 кластеров?**
ApplicationSet с генератором `cluster`: Application создаётся для каждого зарегистрированного кластера, различия задаются через метки кластеров и разные values.

**Как выполнить миграцию БД перед деплоем?**
Job с Helm-хуком `pre-upgrade` или ArgoCD-хуком `PreSync`. Миграции должны быть обратно совместимыми (сначала расширить схему, потом выкатить код, потом убрать старое), чтобы старая версия приложения продолжала работать во время раскатки и при откате.

**Как ArgoCD работает с Helm-чартами?**
Рендерит их сам (`helm template`) и применяет манифесты. Helm-релиза в кластере нет, поэтому `helm list` его не покажет, и откат делается через ArgoCD или git, а не `helm rollback`. Хуки Helm превращаются в хуки ArgoCD.

**Helm-релиз завис в состоянии `pending-upgrade`. Что делать?**
Предыдущая операция была прервана (упал CI, таймаут). `helm history` покажет состояние ревизий; `helm rollback` на последнюю успешную ревизию снимает блокировку. Профилактика — `--atomic` и `--timeout` в CI.

**Чем Helm отличается от Kustomize? Когда что использовать?**
Helm — шаблоны с параметрами, пакеты для распространения, релизы с историей и откатом, хуки. Kustomize — обычные манифесты и патчи для окружений, без шаблонного языка. Helm — для сторонних компонентов и общего чарта компании, Kustomize — для своих сервисов с небольшими различиями. Часто комбинируют.

**Поды не перезапустились после изменения ConfigMap. Почему и как исправить?**
Переменные окружения читаются только при старте контейнера, а изменение ConfigMap не меняет шаблон пода. Решения: аннотация с хешем конфига в шаблоне пода (`checksum/config` в Helm), `configMapGenerator` в Kustomize (хеш в имени ConfigMap), Reloader (контроллер, который сам перезапускает Deployment при изменении связанного ConfigMap или Secret), или `kubectl rollout restart`.

**Чем Sync status отличается от Health status в ArgoCD?**
Sync — совпадает ли кластер с git (Synced или OutOfSync). Health — работают ли ресурсы (Healthy, Progressing, Degraded). Можно быть Synced и Degraded одновременно: всё применено, но новая версия падает.

**Что такое App of Apps и ApplicationSet?**
App of Apps — корневое Application, которое разворачивает другие Application из папки в git. ApplicationSet — генерирует Application по шаблону из списка, папок репозитория или списка кластеров. ApplicationSet гибче и сейчас предпочтительнее.

**Как организовать репозитории при GitOps?**
Код приложения и конфигурация развёртывания — в разных репозиториях. CI приложения собирает образ и обновляет тег в репозитории конфигурации (для prod — через merge request). ArgoCD следит только за репозиторием конфигурации.

**Что такое canary-деплой в GitOps?**
Argo Rollouts или Flagger переводят трафик на новую версию постепенно и между шагами анализируют метрики (ошибки, задержки) из Prometheus. При ухудшении — автоматический откат на стабильную версию.

**Что такое Helm hooks и какие с ними подводные камни?**
Ресурсы чарта (обычно Job), которые выполняются в определённой фазе релиза: `pre-install`, `pre-upgrade`, `post-upgrade`, `pre-delete`, `test` и другие, порядок — через `hook-weight`, уборка — через `hook-delete-policy`. Типичное применение — миграции БД в `pre-upgrade`. Подводные камни: хуки не входят в релиз и не откатываются при `helm rollback` (поэтому миграции должны быть обратно совместимыми), при `pre-install` ещё нет Secret и ConfigMap из того же чарта, без `before-hook-creation` повторный upgrade упадёт на уже существующем Job, долгие миграции упираются в `--timeout`, а параллельные деплои запустят миграцию дважды.

**Как ArgoCD обрабатывает Helm hooks?**
ArgoCD сам рендерит чарт и переводит Helm-хуки в свои: pre-install и pre-upgrade — в PreSync, post-install и post-upgrade — в PostSync. Разницы между установкой и обновлением нет, поэтому хук установки выполняется при каждой синхронизации и должен быть идемпотентным. Часть хуков (rollback, test, pre-delete) не поддерживается. Для чартов под ArgoCD лучше сразу использовать аннотации `argocd.argoproj.io/hook` и `sync-wave`.
