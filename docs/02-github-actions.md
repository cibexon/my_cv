# 2. GitHub Actions построчно

## Термины

- **Workflow** — один YAML-файл в `.github/workflows/`. GitHub сам находит все файлы в этой папке. У нас их два: `image` и `chart`. Это два пункта слева во вкладке Actions.
- **Run** (запуск) — одно выполнение workflow. «image #1» на скриншоте = первый запуск workflow `image`.
- **Job** — группа шагов на **одной** виртуальной машине (runner). Разные jobs = разные чистые машины. Между ними не передаются ни файлы, ни переменные, только явные `outputs`.
- **Step** — один шаг внутри job: либо `uses:` (готовый action — чужой код из GitHub-репозитория), либо `run:` (команды shell).
- **Runner** — машина, где выполняется job. `ubuntu-latest` — виртуалка GitHub (бесплатно для публичных репо): 4 CPU, 16 ГБ, уже есть docker, git, curl, yq, Go и т. д. После job машина уничтожается.
- **`${{ … }}`** — выражение GitHub, подставляется **до** запуска shell. Это не переменная bash.
- **`GITHUB_TOKEN`** — временный токен, который GitHub выдаёт на каждый run. Живёт, пока идёт job. Права задаются `permissions:`.

## `image.yaml`

```yaml
1  # Code or content changed → test → build a multi-arch image to GHCR → write its tag into the Helm values.
2  # The last step is the whole "CD" part: ArgoCD notices the commit and rolls the new image out.
3  name: image
```
**3** — имя workflow в UI: левая колонка Actions, строка «image #1: Commit 6f3f789».

```yaml
5  on:
6    push:
7      branches: [main]
8      paths: [app/**, content/**, Dockerfile, .github/workflows/image.yaml]
```
**5–8** — триггер: push в ветку `main`, в котором изменился хотя бы один файл, подходящий под эти шаблоны. `**` — любая вложенность. Правка только README или `charts/` этот workflow **не** запускает. Поэтому коммит бота (он меняет только `charts/wiki/values.yaml`) не вызывает новую сборку, и цикла не будет.

```yaml
9    pull_request:
10     paths: [app/**, content/**, Dockerfile, .github/workflows/image.yaml]
```
**9–10** — то же для pull request (PR) в любую ветку. Проверка запускается на коде из PR до слияния, и результат виден в PR как зелёная или красная галочка.

```yaml
11   workflow_dispatch:
```
**11** — ручной запуск. В UI появляется кнопка **Run workflow** на странице workflow `image`. Нужно, например, пересобрать образ без изменений в коде.

```yaml
13 # One deploy at a time: two quick pushes must not race to commit values.yaml.
14 concurrency:
15   group: image-${{ github.ref }}
16   cancel-in-progress: false
```
**14–16** — в одной группе одновременно идёт только один run. Группа `image-refs/heads/main` — все запуски на main. Если сделать два push подряд, второй run покажется в UI как **Pending / Waiting** и стартует, когда закончится первый. `cancel-in-progress: false` — не отменять идущий run. Для деплоя так безопаснее: отменённый на середине run мог бы запушить образ, но не закоммитить тег.

```yaml
18 env:
19   IMAGE: ghcr.io/${{ github.repository }}
```
**18–19** — переменная для всех jobs. `github.repository` = `cibexon/my_cv` → `IMAGE=ghcr.io/cibexon/my_cv`. Форк репозитория соберёт образ в свой namespace без правок.

### Job `test`

```yaml
21 jobs:
22   test:
23     runs-on: ubuntu-latest
24     steps:
25       - uses: actions/checkout@v7
```
**22–23** — job `test` на новой Ubuntu-машине.
**25** — action `checkout`: клонирует репозиторий на коммит, который запустил run. Без него на машине пусто. `@v7` — мажорная версия action (git-тег в репо `actions/checkout`).

```yaml
26       - uses: actions/setup-go@v7
27         with:
28           go-version-file: app/go.mod
29           cache-dependency-path: app/go.sum
```
**26–29** — ставит Go той версии, что в `go.mod` (одна версия и локально, и в CI). Кеширует скачанные модули по хешу `go.sum`: второй run не качает зависимости заново. В логе шага видно `Cache restored` или `Cache saved`.

```yaml
30       - run: go vet ./...
31         working-directory: app
32       - run: go test -race ./...
33         working-directory: app
```
**30–31** — `go vet`: статический анализ, ловит код, который компилируется, но почти наверняка ошибочен (неправильный формат в `Printf`, копирование mutex). `./...` — все пакеты рекурсивно. `working-directory` — выполнить в `app/`, где лежит `go.mod`.
**32–33** — тесты с `-race` — детектором гонок данных (две горутины одновременно пишут в одну переменную). Ненулевой код выхода = красный шаг = красный job.

### Job `build`

```yaml
35   build:
36     needs: test
```
**36** — стартует, только если `test` прошёл. В UI это граф: стрелка `test → build → deploy`.

```yaml
37     runs-on: ubuntu-latest
38     permissions:
39       contents: read
40       packages: write
```
**38–40** — права `GITHUB_TOKEN` в этом job: читать код и писать в Packages (GHCR — GitHub Container Registry). Если `permissions` задан, все неуказанные права = `none`. Это принцип наименьших привилегий: build не может пушить в репозиторий.

```yaml
41     outputs:
42       tag: ${{ steps.meta.outputs.version }}
```
**41–42** — что job отдаёт наружу: тег образа из шага `meta`. Job `deploy` прочитает его как `needs.build.outputs.tag`. По-другому передать значение между машинами нельзя.

```yaml
44       - uses: actions/checkout@v7
45       - uses: docker/setup-buildx-action@v4
```
**44** — снова checkout: это другая машина.
**45** — включает buildx (расширенный сборщик Docker на BuildKit). Без него нельзя собирать под несколько платформ сразу и писать кеш в `gha`.

```yaml
46       - uses: docker/login-action@v4
47         if: github.event_name != 'pull_request'
48         with:
49           registry: ghcr.io
50           username: ${{ github.actor }}
51           password: ${{ secrets.GITHUB_TOKEN }}
```
**46–51** — `docker login ghcr.io` встроенным токеном. Личный токен в Secrets заводить не нужно. `github.actor` — кто запустил run (`cibexon`).
**47** — на PR не логинимся. PR из форка мог бы попытаться запушить что-то своё, а так у него нет даже входа в реестр. В UI шаг при этом серый (skipped).

```yaml
52       - id: meta
53         uses: docker/metadata-action@v6
54         with:
55           images: ${{ env.IMAGE }}
56           tags: type=sha,prefix=sha-
```
**52** — `id` — имя шага, чтобы ссылаться на его outputs (`steps.meta.outputs...`).
**53–56** — вычисляет теги и OCI-метки (OCI — стандарт формата образов). `type=sha` — короткий хеш коммита: `ghcr.io/cibexon/my_cv:sha-6f3f789`. Получаем:
- `outputs.tags` — полное имя (или несколько через перевод строки);
- `outputs.version` — только тег (`sha-6f3f789`);
- `outputs.labels` — метки `org.opencontainers.image.source=https://github.com/cibexon/my_cv`, `revision`, `created`.

Тег привязан к коммиту и не меняется: по `sha-6f3f789` всегда найдёшь код, из которого собран образ.

```yaml
57       - uses: docker/build-push-action@v7
58         with:
59           context: .
60           platforms: linux/amd64,linux/arm64
61           push: ${{ github.event_name != 'pull_request' }}
62           tags: ${{ steps.meta.outputs.tags }}
63           labels: ${{ steps.meta.outputs.labels }}
64           cache-from: type=gha
65           cache-to: type=gha,mode=max
```
- **59** — контекст сборки (что видят `COPY`): корень репо. Dockerfile по умолчанию `./Dockerfile`.
- **60** — две платформы. Получается один тег с manifest list внутри (список «для amd64 — этот образ, для arm64 — этот»). Docker и kind сами берут нужный. На Mac M-серии без arm64 под не запустился бы: `exec format error`.
- **61** — пушить только не на PR. Выражение даёт `true` или `false`.
- **63** — метка `org.opencontainers.image.source` связывает пакет GHCR с репозиторием. Поэтому пакет появился справа на главной странице репо в блоке **Packages**, а `GITHUB_TOKEN` этого репо имеет к нему доступ.
- **64–65** — кеш слоёв BuildKit в хранилище кеша Actions (то же, что у `setup-go`). `mode=max` — кешировать слои и промежуточной стадии `build`, а не только итоговой. Повторная сборка без изменений в `go.sum` не скачивает модули. Кеш виден в **Actions → Caches**.

### Job `deploy`

```yaml
67   deploy:
68     needs: build
69     if: github.event_name != 'pull_request' && github.ref == 'refs/heads/main'
```
**69** — только для push в main. На PR job серый (skipped). Деплой из чужой ветки не случится.

```yaml
70     runs-on: ubuntu-latest
71     permissions:
72       contents: write
```
**71–72** — право пушить в репозиторий. Только у этого job.

```yaml
74       - uses: actions/checkout@v7
75       - name: Write the new tag into charts/wiki/values.yaml
76         env:
77           TAG: ${{ needs.build.outputs.tag }}
```
**75** — `name` — заголовок шага в логе run.
**76–77** — тег из job `build` кладём в переменную окружения, а не подставляем `${{ }}` прямо в скрипт. Так безопаснее: значение не может стать частью bash-кода (защита от инъекций через выражения, по рекомендации GitHub).

```bash
78         run: |
79           sed -i "s/^  tag: .*/  tag: ${TAG}/" charts/wiki/values.yaml
```
**78** — `|` — многострочный блок YAML. Каждая строка выполняется в bash с `set -e` (GitHub запускает `bash -e`): любая ошибка = красный шаг.
**79** — заменить строку, начинающуюся ровно с двух пробелов и `tag:`. В values это только `image.tag`.

```bash
80           git diff --exit-code charts/wiki/values.yaml && { echo "tag unchanged"; exit 0; }
```
**80** — `git diff --exit-code` возвращает 0, если изменений нет. Тогда выходим успешно: это повторный запуск того же коммита (например, через Run workflow), коммитить нечего.

```bash
81           git config user.name "github-actions[bot]"
82           git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
83           git commit -am "deploy: wiki ${TAG}"
```
**81–82** — от чьего имени коммит. Этот email GitHub знает, поэтому в истории у коммита иконка бота.
**83** — тот самый коммит `deploy: wiki sha-6f3f789` в истории.

```bash
84           git pull --rebase origin main
85           git push origin HEAD:main
```
**84** — пока шла сборка (2–4 мин), ты мог запушить ещё что-то. Переносим наш коммит поверх свежего main, иначе push отклонят как non-fast-forward.
**85** — push `GITHUB_TOKEN`ом (checkout сохранил его в настройках git). Этот push **не запускает** новые workflow: GitHub так защищает от бесконечных циклов. Поэтому `chart.yaml` на коммит бота не сработал.

Что происходит дальше уже без GitHub Actions: ArgoCD в кластере в течение минуты видит новый коммит → рендерит чарт с новым тегом → Deployment меняет образ → rolling update.

## `chart.yaml`

```yaml
2  name: chart
4  on:
5    push:
6      branches: [main]
7      paths: [charts/**, deploy/**, scenarios/**, .github/workflows/chart.yaml]
8    pull_request:
9      paths: [charts/**, deploy/**, scenarios/**, .github/workflows/chart.yaml]
```
Запуск только при изменениях в чарте, ArgoCD-манифестах или сценариях. На скриншоте `chart #1` занял 12 с: ничего не собирается, только проверки.

```yaml
12   lint:
13     runs-on: ubuntu-latest
14     steps:
15       - uses: actions/checkout@v7
16       - uses: azure/setup-helm@v5
```
**16** — ставит Helm (последнюю стабильную версию).

```yaml
17       - name: Install kubeconform
18         run: |
19           curl -sSL https://github.com/yannh/kubeconform/releases/download/v0.8.0/kubeconform-linux-amd64.tar.gz \
20             | sudo tar -xz -C /usr/local/bin kubeconform
```
**17–20** — скачивает бинарник kubeconform зафиксированной версии и распаковывает сразу в `/usr/local/bin`. `-sSL`: тихо, но показать ошибку, следовать редиректам (GitHub отдаёт релизы через редирект).

```yaml
21       - run: helm lint charts/wiki --strict
```
**21** — проверка структуры чарта: синтаксис шаблонов, обязательные поля `Chart.yaml`, YAML после рендера. `--strict` — предупреждения тоже считаются ошибками.

```bash
25           check() { kubeconform -strict -summary \
26             -schema-location default \
27             -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'; }
```
**25–27** — функция bash `check`: читает YAML из stdin и сверяет каждый объект со схемой API.
- `-strict` — лишнее или опечатанное поле (`replica:` вместо `replicas:`) = ошибка. `kubectl apply` такое молча проигнорирует.
- `default` — схемы встроенных типов (Deployment, Service).
- Второй источник — каталог схем для CRD (ServiceMonitor, PrometheusRule, Application). `{{.Group}}` и т. п. — шаблон kubeconform, а не GitHub.

```bash
28           helm template wiki charts/wiki -n wiki | check
29           for s in scenarios/[0-9]*.yaml; do
30             echo "== $s"
31             helm template lab charts/wiki -n lab -f scenarios/_lab.yaml -f "$s" | check
32           done
```
**28** — рендер с дефолтными values (как у ArgoCD) → проверка.
**29–32** — то же для каждого сценария. `-f` можно указать несколько раз, и последующий перекрывает предыдущий. Так сценарии остаются валидными при правках чарта.

```bash
33           kubeconform -strict -summary -schema-location default \
34             -schema-location '…CRDs-catalog…' \
35             deploy/root.yaml deploy/apps/*.yaml
```
**33–35** — проверка Application-манифестов ArgoCD (файлы передаются по одному, а не через `cat`).

## Где что смотреть в UI GitHub

| Что | Где |
|---|---|
| Граф jobs и логи каждого шага | Actions → run → клик по job |
| Перезапустить упавший job | Run → **Re-run jobs** → Re-run failed jobs |
| Ручной запуск `image` | Actions → image → **Run workflow** |
| Образ и его теги | Профиль → Packages → `my_cv`, или справа на главной репо |
| Кеши | Actions → Management → **Caches** |
| Запретить merge без зелёного CI | Settings → Rules → Rulesets → Require status checks (`test`, `lint`) |
| Права `GITHUB_TOKEN` по умолчанию | Settings → Actions → General → Workflow permissions (у нас права заданы явно в файле) |

## Вопросы с собеседования по этому пайплайну

- **Почему не `latest`?** Не видно, какой код запущен. `IfNotPresent` на ноде может держать старый `latest`. Откат невозможен по тегу.
- **Как откатить?** `git revert` коммита `deploy: wiki sha-…` → ArgoCD вернёт прошлый тег. Пересборка не нужна.
- **Почему CI не делает `kubectl apply`?** Тогда CI нужен доступ к кластеру (kubeconfig в Secrets = большая цель для атаки), а состояние кластера расходится с Git. В GitOps кластер сам тянет из Git.
- **Как ускорить сборку?** Кеш модулей Go (`setup-go`), кеш слоёв (`type=gha`), порядок `COPY` в Dockerfile (сначала `go.mod`), кросс-компиляция вместо QEMU.
- **Секреты?** Тут их нет вообще. Для облака вместо ключей в Secrets используют OIDC (OpenID Connect): GitHub выдаёт короткоживущий токен, AWS/GCP меняют его на временные креды по доверенной роли, и постоянных ключей нет.
