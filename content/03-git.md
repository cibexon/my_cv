# Git

Подробно о Git: как он устроен внутри (это объясняет поведение всех команд), ежедневная работа, merge и rebase, анализ сильно разветвлённых репозиториев, перенос изменений между ветками, спасение потерянной работы и стратегии ветвления. В конце — кейсы, которые разбирают на собеседованиях.

## Как Git устроен внутри

Многие команды Git кажутся магией, пока не понимаешь модель данных. Она простая.

### Снапшоты, а не диффы
Git хранит не «изменения файлов», а **снимок (snapshot) всего проекта** на момент каждого коммита. Неизменившиеся файлы не дублируются: новый снимок ссылается на тот же объект, что и предыдущий. Диффы Git вычисляет на лету, когда ты просишь показать изменения.

### Четыре типа объектов
Всё хранится в папке `.git/objects` как объекты, а имя каждого объекта — **хеш его содержимого** (SHA-1 — алгоритм, который из любых данных вычисляет «отпечаток» фиксированной длины, 40 шестнадцатеричных символов; изменился хоть один байт — изменился весь хеш):
- **blob** — содержимое одного файла (без имени);
- **tree** — содержимое директории: список «имя → blob или вложенный tree»;
- **commit** — ссылка на корневой tree (снимок проекта), ссылки на **родительские** коммиты, автор, дата, сообщение;
- **tag** (аннотированный) — именованная ссылка на коммит с автором и сообщением.

```bash
git cat-file -p HEAD
# tree 9c4a1f...           ← снимок проекта
# parent 3b7e2d...         ← предыдущий коммит
# author Alex <alex@...> 1760000000 +0300
# committer Alex <...>
#
# Add healthcheck endpoint
git cat-file -p 9c4a1f     # содержимое tree: какие файлы и папки
```

Главные следствия:
1. **Коммит неизменяем.** Хеш вычисляется из содержимого, включая хеш родителя. Изменить сообщение, файл или родителя — значит получить **новый** коммит с новым хешем. Поэтому `--amend`, `rebase` и `cherry-pick` не меняют коммиты, а создают новые.
2. **История — цепочка ссылок на родителей.** Коммит знает своих родителей, но не потомков. У merge-коммита два родителя.
3. **Целостность.** Нельзя незаметно подменить старый коммит: изменится его хеш и хеши всех потомков.

### Ветки и HEAD — просто указатели
**Ветка** — это файл с хешем одного коммита (`.git/refs/heads/main`). Ничего больше. Создание ветки — запись 41 байта, поэтому в Git ветки дешёвые.

Когда ты делаешь коммит на ветке, новый коммит получает родителем текущий, а указатель ветки **сдвигается** на новый коммит.

**HEAD** — указатель на то, где ты сейчас. Обычно он указывает на ветку (`ref: refs/heads/main`), а ветка — на коммит.

```
            HEAD
             ↓
            main
             ↓
A ← B ← C ← D          (каждый коммит ссылается на родителя)
         ↑
       feature
```

**Detached HEAD** — HEAD указывает прямо на коммит, а не на ветку (после `git checkout <hash>` или checkout тега). Новые коммиты в таком состоянии ни на какую ветку не попадут, и после переключения их будет найти только через reflog. Чтобы сохранить работу — создать ветку: `git switch -c rescue`.

**Удалённые ветки** (`origin/main`) — локальные копии указателей с сервера на момент последнего `fetch`. Они не обновляются сами.

### Три зоны: рабочая копия, индекс, репозиторий
```
Рабочая копия          Индекс (staging area)        Репозиторий (.git)
(файлы на диске)  --git add-->  (что войдёт в       --git commit-->  (коммиты)
                                следующий коммит)
```
**Индекс** — промежуточная зона, где собирается следующий коммит. Зачем она: можно закоммитить только часть изменений (например, исправление бага отдельно от рефакторинга), даже части одного файла (`git add -p`).

```bash
git status            # что изменено в рабочей копии, что добавлено в индекс
git diff              # рабочая копия против индекса (что ещё НЕ добавлено)
git diff --staged     # индекс против последнего коммита (что ВОЙДЁТ в коммит)
```

### 🧪 Мини-задание

**Задача.** Загляни внутрь репозитория: посчитай хеш содержимого файла до коммита, закоммить два файла с одинаковым текстом и посмотри, какие объекты появились, что лежит в дереве коммита и чем на самом деле является `HEAD`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo hello > hello.txt
git hash-object hello.txt
cp hello.txt copy.txt
git add . && git commit -qm "two files"
git cat-file -t HEAD
git cat-file -p 'HEAD^{tree}'
git cat-file -t ce013625030ba8dba906f756967f9e9ca394464a
cat .git/HEAD
```

Ожидаемый результат:

```text
ce013625030ba8dba906f756967f9e9ca394464a
commit
100644 blob ce013625030ba8dba906f756967f9e9ca394464a	copy.txt
100644 blob ce013625030ba8dba906f756967f9e9ca394464a	hello.txt
blob
ref: refs/heads/main
```

Разбор: хеш blob зависит только от содержимого (`hello` + перевод строки), поэтому он одинаковый на любой машине и для обоих файлов — Git хранит такой текст один раз. Имена файлов живут не в blob, а в tree. Коммит указывает на tree, а `HEAD` — просто текстовый файл со ссылкой на текущую ветку. Сама ветка — файл `.git/refs/heads/main` с хешем коммита. Хеш коммита у каждого свой: в нём есть автор и время.

</details>

## Ежедневная работа

### Настройка
```bash
git config --global user.name "Alex Sukhanov"
git config --global user.email "alex@example.com"
git config --global pull.rebase true          # pull делает rebase вместо merge (линейная история)
git config --global rerere.enabled true       # запоминать разрешения конфликтов (см. ниже)
git config --global init.defaultBranch main
git config --global alias.lg "log --oneline --graph --decorate --all"
```

### Типичный цикл работы над задачей
```bash
git switch main && git pull                  # свежий main
git switch -c feature/JIRA-123-healthcheck   # новая ветка от текущего коммита

# ... правки ...
git status -sb                               # краткий статус и отставание от удалённой ветки
git add -p                                   # добавлять изменения по кускам, просматривая каждый
git commit -m "feat: add /health endpoint"

git push -u origin feature/JIRA-123-healthcheck   # -u: запомнить связь с удалённой веткой
# дальше просто git push / git pull
```
`git switch` и `git restore` (Git 2.23+) — более понятная замена перегруженной `git checkout`, которая умела и переключать ветки, и откатывать файлы.

### Отмена изменений до коммита
```bash
git restore app.py              # отменить изменения в файле (вернуть как в индексе) — НЕОБРАТИМО
git restore --staged app.py     # убрать файл из индекса (изменения в рабочей копии остаются)
git restore --source=HEAD~2 app.py   # вернуть файл в состояние двух коммитов назад
git clean -nd                   # показать неотслеживаемые файлы, которые будут удалены (dry-run)
git clean -fd                   # удалить их
```

### stash — отложить незаконченную работу
Ситуация: работаешь над фичей, и срочно нужно переключиться на хотфикс, а коммитить полуготовое не хочется.
```bash
git stash push -m "wip: refactor config"     # спрятать изменения, рабочая копия чистая
git switch main                              # ... делаем хотфикс ...
git switch feature/x
git stash list                               # stash@{0}: On feature/x: wip: refactor config
git stash pop                                # вернуть последний и удалить из списка
git stash apply stash@{1}                    # применить конкретный, не удаляя
git stash push -u                            # включая новые (неотслеживаемые) файлы
```
Альтернатива для долгих переключений — `git worktree` (см. ниже).

### Исправить последний коммит
```bash
git commit --amend -m "лучшее сообщение"     # переписать сообщение последнего коммита
git add forgotten.py && git commit --amend --no-edit   # добавить забытый файл в последний коммит
```
`--amend` создаёт **новый** коммит вместо последнего. Если старый уже запушен, понадобится `push --force-with-lease` — не делай так в общих ветках.

### fetch и pull
- **`git fetch`** — скачать новые коммиты и обновить `origin/*`. Твои локальные ветки и файлы **не меняются**. Безопасная операция: можно посмотреть, что пришло, прежде чем вливать.
- **`git pull`** = `fetch` + `merge` (или `rebase` при `pull.rebase=true`) в текущую ветку.

```bash
git fetch --prune                  # --prune: удалить origin/* для веток, удалённых на сервере
git log --oneline HEAD..origin/main   # что пришло в origin/main и ещё нет у меня
```

### push --force и --force-with-lease
После rebase или amend история ветки переписана, и обычный push отклоняется (удалённая ветка не является предком новой). Нужен принудительный push:
- `git push --force` — перезапишет удалённую ветку **в любом случае**, даже если коллега успел туда что-то запушить. Его коммиты пропадут.
- `git push --force-with-lease` — перезапишет, **только если** удалённая ветка находится там, где ты её последний раз видел. Если кто-то запушил новое, push откажет. **Всегда используй его вместо `--force`.**

### Теги
```bash
git tag -a v1.4.0 -m "Release 1.4.0"   # аннотированный тег (с автором и сообщением) — для релизов
git push origin v1.4.0                 # теги не пушатся автоматически
git tag --sort=-v:refname | head       # последние теги по версии
git describe --tags                    # → v1.4.0-3-g9c4a1f2 — 3 коммита после v1.4.0
```

### 🧪 Мини-задание

**Задача.** Пройди по трём зонам Git: измени закоммиченный файл и создай новый, посмотри `status --short` до и после `add`, убери файл из индекса, откати изменения в рабочей копии. Затем отложи незаконченную работу вместе с новым файлом в `stash` и верни её.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo v1 > app.conf && git add app.conf && git commit -qm "add config"
echo v2 > app.conf; echo draft > notes.txt
git status --short
git add app.conf; git status --short
git restore --staged app.conf; git status --short
git restore app.conf; cat app.conf
echo v3 > app.conf
git stash push -q -u -m wip; git status --short; git stash list
git stash pop -q; git status --short
```

Ожидаемый результат:

```text
 M app.conf
?? notes.txt
M  app.conf
?? notes.txt
 M app.conf
?? notes.txt
v1
stash@{0}: On main: wip
 M app.conf
?? notes.txt
```

Разбор: в `status --short` две колонки: левая — индекс (staged), правая — рабочая копия. ` M` — изменён, но не добавлен, `M ` — добавлен в индекс, `??` — неотслеживаемый файл. `restore --staged` убирает из индекса, не трогая файл, а `restore` без флага откатывает сам файл к версии из коммита — `v2` потерян безвозвратно. После `stash -u` статус пустой (строки нет), потому что `-u` забрал и неотслеживаемый `notes.txt`. Без `-u` он остался бы в рабочей копии.

</details>

## Просмотр истории
```bash
git log --oneline --graph --decorate --all   # граф всех веток
git log -p -- path/to/file                   # история файла с изменениями
git log --follow -- file                     # история с учётом переименований
git log -S "connection_timeout"              # коммиты, где строка появилась или исчезла (pickaxe)
git log -G "timeout\s*="                     # где менялись строки по регулярному выражению
git log --author="alex" --since="2 weeks ago" --until="yesterday"
git log --grep="JIRA-123"                    # по тексту сообщения
git show <sha>                               # коммит с изменениями
git show <sha>:path/file                     # файл в состоянии на тот коммит
git blame -L 40,60 app.py                    # кто и в каком коммите менял строки 40–60
git blame -w -C app.py                       # игнорировать пробелы, отслеживать перенос кода из других файлов
```
`log -S` — один из самых полезных приёмов: «когда и кто удалил эту настройку?» находится за секунды.

### 🧪 Мини-задание

**Задача.** Сделай три коммита, которые меняют два файла. Найди: всю историю одними сообщениями, историю только одного файла, коммит, в котором появилась строка `timeout=30` (поиск по содержимому), и сводку изменений за последние два коммита.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo "timeout=10" > app.conf && git add . && git commit -qm "add app config"
echo "replicas=2" > deploy.yaml && git add . && git commit -qm "add deploy"
echo "timeout=30" > app.conf && git commit -qam "raise timeout"
git log --format='%s'
echo ---; git log --format='%s' -- app.conf
echo ---; git log -S'timeout=30' --format='%s'
echo ---; git diff HEAD~2 --stat
```

Ожидаемый результат:

```text
raise timeout
add deploy
add app config
---
raise timeout
add app config
---
raise timeout
---
 app.conf    | 2 +-
 deploy.yaml | 1 +
 2 files changed, 2 insertions(+), 1 deletion(-)
```

Разбор: `--format='%s'` оставляет только заголовки — удобно в скриптах и для changelog. `-- app.conf` ограничивает историю файлом. `-S` (pickaxe) ищет коммиты, где *число вхождений* строки изменилось, то есть где её добавили или удалили, — так находят, кто и когда поменял настройку. `--stat` показывает объём изменений: в `app.conf` одна строка заменена, в `deploy.yaml` одна добавлена.

</details>

## Merge и rebase

Обе команды решают одну задачу — **объединить работу из двух веток** — но по-разному.

Исходная ситуация: от `main` ответвилась `feature`, и в обеих появились новые коммиты:
```
      C---D   feature
     /
A---B---E---F   main
```

### merge
```bash
git switch main
git merge feature
```
```
      C---D
     /     \
A---B---E---F---M   main   (M — merge-коммит с двумя родителями: F и D)
```
- История сохраняется **как была**: видно, что работа шла параллельно.
- Коммиты C и D не меняются (те же хеши).
- Появляется merge-коммит.
- Безопасно для общих веток.

**Fast-forward.** Если `main` не ушла вперёд после ответвления (коммитов E и F нет), Git просто сдвигает указатель `main` на D, без merge-коммита. `git merge --no-ff feature` создаст merge-коммит всегда: так в истории видны границы фичи. `--ff-only` откажется сливать, если fast-forward невозможен.

### rebase
```bash
git switch feature
git rebase main
```
```
A---B---E---F   main
             \
              C'---D'   feature
```
- Коммиты C и D **переигрываются** поверх F. Получаются **новые** коммиты C' и D' (другой родитель — значит, другой хеш).
- История становится **линейной**, будто работа начиналась от свежего main.
- Старые C и D остаются в репозитории без ссылок (их видно в reflog).

После rebase `main` можно влить fast-forward'ом, и история будет прямой линией.

### Золотое правило rebase
**Не делай rebase коммитов, которые уже опубликованы и на которых другие основывают свою работу.** У коллег останутся старые C и D, у тебя — новые C' и D' с теми же изменениями. При следующем pull всё перемешается, появятся дубли коммитов и конфликты.

Свою личную feature-ветку ребейзить можно и нужно (подтянуть свежий main), после чего — `push --force-with-lease`.

### Как это выглядит на практике
```bash
git switch feature/x
git fetch
git rebase origin/main          # переиграть мои коммиты поверх свежего main

# при конфликте rebase останавливается на проблемном коммите:
git status                      # какие файлы в конфликте
# правим файлы, убираем маркеры <<<<<<< ======= >>>>>>>
git add app.py
git rebase --continue           # перейти к следующему коммиту
git rebase --abort              # передумал — вернуть всё как было до rebase
git rebase --skip               # выбросить текущий коммит (осторожно)

git push --force-with-lease
```

### Интерактивный rebase — навести порядок в истории
Перед merge request часто нужно превратить «wip», «fix typo», «ещё фикс» в аккуратные коммиты:
```bash
git rebase -i main              # или HEAD~5 — последние 5 коммитов
```
Откроется редактор со списком коммитов (сверху самый старый):
```
pick 1a2b3c4 feat: add healthcheck endpoint
pick 5d6e7f8 wip
pick 9a8b7c6 fix typo
pick 3c4d5e6 feat: add readiness probe
```
Меняешь команды и сохраняешь:
- `pick` — оставить как есть;
- `reword` — оставить, но изменить сообщение;
- `squash` — объединить с предыдущим, объединив сообщения;
- `fixup` — объединить с предыдущим, выбросив сообщение этого;
- `drop` — удалить коммит;
- `edit` — остановиться на коммите, чтобы изменить его (например, разбить на два);
- можно менять строки местами — изменится порядок коммитов.

Автоматизация: делаешь исправление к конкретному старому коммиту через `git commit --fixup 1a2b3c4`, а потом `git rebase -i --autosquash main` сам расставит `fixup` в нужные места.

### Squash merge
Кнопка «Squash and merge» в GitLab или GitHub: все коммиты ветки превращаются в **один** коммит в main. Чистая история main (один коммит на одну фичу), но детальная история ветки в main теряется.

### Что выбрать
| Ситуация | Подход |
|---|---|
| Подтянуть свежий main в свою ветку | `rebase` (линейно, без лишних merge-коммитов) |
| Влить фичу в main | merge request: `merge --no-ff` или squash — по договорённости команды |
| Общая долгоживущая ветка (develop, release) | только `merge` |
| Навести порядок перед ревью | `rebase -i` |

### rebase --onto — перенести часть ветки
Ситуация: ветку `feature-b` создали от `feature-a`, а не от `main`. Теперь `feature-a` не нужна, а `feature-b` надо перенести на `main` **без коммитов `feature-a`**:
```
A---B   main
     \
      C---D   feature-a
           \
            E---F   feature-b
```
```bash
git rebase --onto main feature-a feature-b
# «взять коммиты из feature-b, которых нет в feature-a (это E и F), и переиграть поверх main»
```
```
A---B---E'---F'   feature-b
```

### 🧪 Мини-задание

**Задача.** Создай расходящиеся ветки: в `main` коммит `main fix`, в `feature` коммит `feature work`. Сделай копию `feature` и перебазируй её на `main`. Затем смержи исходную `feature` в `main`. Сравни историю обеих веток и найди merge-коммит.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
git commit -q --allow-empty -m init
git switch -qc feature && git commit -q --allow-empty -m "feature work"
git switch -q main && git commit -q --allow-empty -m "main fix"
git switch -qc rebased feature && git rebase -q main
git log --format='%s' rebased
echo ---; git switch -q main && git merge -q --no-edit feature
git log --first-parent --format='%s' main
echo ---; git log --merges --format='%s' main
```

Ожидаемый результат:

```text
feature work
main fix
init
---
Merge branch 'feature'
main fix
init
---
Merge branch 'feature'
```

Разбор: rebase переписал `feature work` поверх `main fix` — история линейная, но это *новый* коммит с другим хешем, поэтому ветку, которую уже запушили и с которой работают другие, ребейзить нельзя. Merge сохранил обе линии и добавил merge-коммит с двумя родителями. `--first-parent` показывает историю «глазами main» — только то, что в неё вливали, это удобно для changelog. `--merges` находит все слияния.

</details>

## Конфликты

Конфликт возникает, когда обе ветки изменили **одни и те же строки** (или одна изменила файл, а другая удалила). Git не может решить за тебя и помечает место:
```
<<<<<<< HEAD
timeout: 30
=======
timeout: 60
>>>>>>> feature/x
```
Между `<<<<<<<` и `=======` — версия текущей ветки, между `=======` и `>>>>>>>` — вливаемой. Нужно оставить правильный вариант (или объединить оба), удалить маркеры, затем `git add` и продолжить (`git merge --continue` или `git rebase --continue`).

```bash
git diff --name-only --diff-filter=U    # список файлов с конфликтами
git checkout --ours config.yaml         # взять целиком версию «нашей» стороны
git checkout --theirs config.yaml       # взять версию «их» стороны
git mergetool                           # открыть графический инструмент слияния
git merge --abort                       # отменить слияние целиком
git log --merge -p -- config.yaml       # коммиты обеих сторон, которые трогали конфликтный файл
```

**Ловушка: при rebase `ours` и `theirs` меняются местами.** При rebase Git переключается на ветку, на которую переносит (main), и по одному применяет твои коммиты. Поэтому «ours» — это main, а «theirs» — твои коммиты. При merge наоборот.

**rerere** (reuse recorded resolution) — Git запоминает, как ты разрешил конфликт, и при следующем таком же конфликте применяет решение сам. Незаменим, когда приходится многократно ребейзить долгоживущую ветку. Включается `git config --global rerere.enabled true`.

### 🧪 Мини-задание

**Задача.** Устрой конфликт: в `main` поменяй `timeout` на 30, а в `feature` — на 60, и смержи. Посмотри статус и маркеры конфликта в файле, разреши конфликт значением 45 и заверши merge.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo "timeout=10" > app.conf && git add . && git commit -qm init
git switch -qc feature && echo "timeout=60" > app.conf && git commit -qam "timeout 60"
git switch -q main && echo "timeout=30" > app.conf && git commit -qam "timeout 30"
git merge feature
git status --short
cat app.conf
echo "timeout=45" > app.conf && git add app.conf && git commit -q --no-edit
git log -1 --format='%s'; cat app.conf
```

Ожидаемый результат:

```text
Auto-merging app.conf
CONFLICT (content): Merge conflict in app.conf
Automatic merge failed; fix conflicts and then commit the result.
UU app.conf
<<<<<<< HEAD
timeout=30
=======
timeout=60
>>>>>>> feature
Merge branch 'feature'
timeout=45
```

Разбор: Git не может сам выбрать между 30 и 60, потому что обе ветки поменяли одну и ту же строку относительно общего предка. `UU` — «изменён в обеих ветках, не слит». Между `<<<<<<< HEAD` и `=======` — версия текущей ветки, ниже — сливаемой. Разрешить — значит оставить в файле нужный текст без маркеров, сделать `git add` и закоммитить. Если что-то пошло не так, `git merge --abort` вернёт всё как было до merge.

</details>

## Анализ сильно разветвлённых репозиториев

Когда в репозитории десятки веток, релизные ветки и хотфиксы, основные вопросы такие: «что есть в этой ветке, чего нет в той?», «попал ли фикс в релиз?», «какие ветки можно удалить?».

### Синтаксис диапазонов: две и три точки
```bash
git log main..feature       # коммиты, которые есть в feature, но НЕТ в main
                            # = «что принесёт merge feature в main»
git log feature..main       # коммиты, которые есть в main, но нет в feature
                            # = «насколько feature отстала от main»
git log main...feature      # коммиты, которые есть только в одной из веток (симметричная разность)
git log --left-right --oneline main...feature
# < a1b2c3 hotfix в main        ← '<' — только в левой (main)
# > d4e5f6 новая фича           ← '>' — только в правой (feature)
```
С `git diff` смысл точек **другой** (это частая путаница):
```bash
git diff main..feature      # разница между вершинами веток (то же, что git diff main feature)
git diff main...feature     # что изменила feature с момента ответвления от main
                            # = то, что показывает merge request. Изменения в main не учитываются
```

### Попал ли коммит в ветку или релиз
```bash
git branch -a --contains a1b2c3d     # в каких ветках (локальных и удалённых) есть этот коммит
git tag --contains a1b2c3d           # в каких релизах (тегах) он есть
git merge-base --is-ancestor a1b2c3d release/2.3 && echo "есть в release/2.3"
```
Проблема: после cherry-pick коммит в другой ветке имеет **другой хеш**, и `--contains` его не увидит. Тогда искать по содержимому изменений:
```bash
git cherry -v release/2.3 main
# + a1b2c3 fix: null pointer in auth     ← '+': такого изменения в release/2.3 нет
# - d4e5f6 fix: timeout                  ← '-': эквивалентный патч уже есть (перенесли cherry-pick'ом)
git log --cherry-pick --right-only --oneline release/2.3...main   # то же, только недостающие
git log --all --grep="JIRA-123" --oneline                          # или по номеру задачи в сообщениях
```

### Общий предок и карта веток
```bash
git merge-base main feature                       # коммит, от которого разошлись ветки
git log --oneline --graph --all --simplify-by-decoration   # «карта»: только коммиты с ветками и тегами
git show-branch main release/2.3 feature/x        # таблица: какие коммиты в каких ветках
```

### Уборка веток
```bash
git branch --merged main                 # локальные ветки, уже влитые в main — можно удалять
git branch -r --merged origin/main       # то же для удалённых
git branch --no-merged main              # ещё не влитые — осторожно
git branch -d feature/old                # удалить (откажет, если не влита)
git branch -D feature/old                # удалить принудительно
git push origin --delete feature/old     # удалить на сервере
# ветки, отсортированные по дате последнего коммита: какие давно заброшены
git for-each-ref --sort=-committerdate refs/remotes/origin \
  --format='%(committerdate:short) %(authorname) %(refname:short)'
```
Ограничение: ветки, влитые через **squash merge**, `--merged` не покажет: в main нет их коммитов, есть только один новый. Такие ветки ищут через `git cherry` или по закрытым merge request.

### Сравнить две версии ветки после rebase
Ревьюер хочет понять, что изменилось между прошлой и новой версией MR после rebase. Обычный diff покажет и все изменения main. `range-diff` сравнивает коммиты попарно:
```bash
git range-diff main origin/feature@{1} feature
```

### 🧪 Мини-задание

**Задача.** Ветка `done` уже влита в `main`, ветка `wip` — нет, а в `main` после её создания появился хотфикс. Найди влитые и невлитые ветки, коммиты, которых нет в `main`, и посчитай, на сколько коммитов ветки разошлись в обе стороны.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
git commit -q --allow-empty -m init
git switch -qc done && git commit -q --allow-empty -m "done feature"
git switch -q main && git merge -q --no-ff --no-edit done
git switch -qc wip && git commit -q --allow-empty -m "wip 1" && git commit -q --allow-empty -m "wip 2"
git switch -q main && git commit -q --allow-empty -m hotfix
git branch --merged main
echo ---; git branch --no-merged main
echo ---; git log --format='%s' main..wip
echo ---; git rev-list --left-right --count main...wip
git merge-base --is-ancestor done main && echo "done is in main"
```

Ожидаемый результат:

```text
  done
* main
---
  wip
---
wip 2
wip 1
---
1	2
done is in main
```

Разбор: `--merged` — ветки, все коммиты которых уже в `main`, их можно удалять; `--no-merged` — ветки с невлитой работой. Две точки `main..wip` — коммиты, которые есть в `wip`, но нет в `main`. Три точки с `--left-right --count` дают «1 слева, 2 справа»: в `main` один коммит, которого нет в `wip` (hotfix), а в `wip` два своих. `merge-base --is-ancestor` отвечает кодом возврата на вопрос «попал ли коммит или ветка в релиз» — удобно в скриптах.

</details>

## Перенос изменений между ветками

### cherry-pick — скопировать коммит
Типичная ситуация: исправление сделано в `main`, и его нужно донести в поддерживаемые релизные ветки.
```bash
git switch release/2.3
git cherry-pick -x a1b2c3d
# -x допишет в сообщение "(cherry picked from commit a1b2c3d)" — потом видно, откуда пришёл фикс
git cherry-pick A^..B            # диапазон коммитов от A до B включительно
git cherry-pick -m 1 <merge-sha> # перенести merge-коммит (указать, относительно какого родителя брать изменения)
git cherry-pick --continue / --abort   # при конфликте — как в rebase
```
cherry-pick создаёт **новый** коммит с теми же изменениями и другим хешем.

### Хотфикс в несколько релизов: варианты
1. **Сначала в main, потом cherry-pick вниз** в release-ветки. Просто и наглядно. Минус: одно изменение — несколько разных коммитов.
2. **Сначала в самую старую поддерживаемую ветку, потом merge вверх** (`release/2.2` → `release/2.3` → `main`). Коммит один, и `--contains` везде его покажет. Требует дисциплины.

### worktree — несколько веток одновременно
Вместо `stash` и переключения туда-обратно можно получить вторую рабочую папку с другой веткой того же репозитория:
```bash
git worktree add ../hotfix release/2.3   # в ../hotfix — рабочая копия ветки release/2.3
cd ../hotfix                             # ... делаем хотфикс, параллельно идёт сборка в основной папке ...
git worktree list
git worktree remove ../hotfix
```
Удобно, когда в основной папке идёт долгая сборка или тесты, или нужно сравнить поведение двух версий рядом.

### 🧪 Мини-задание

**Задача.** В `main` после релиза `release/1.0` сделано два коммита: новая фича и исправление бага. Перенеси в релизную ветку только исправление через `cherry-pick -x` и проверь, что фича туда не попала и что в сообщении осталась ссылка на исходный коммит.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo v1 > app.conf && git add . && git commit -qm "release 1.0"
git branch release/1.0
echo feature > feature.txt && git add . && git commit -qm "new feature"
echo fixed > app.conf && git commit -qam "fix crash on start"
git switch -q release/1.0
git cherry-pick -x main > /dev/null
git log --format='%s'
ls; cat app.conf
git log -1 --format='%b' | cut -c1-28
```

Ожидаемый результат:

```text
fix crash on start
release 1.0
app.conf
fixed
(cherry picked from commit
```

Разбор: `cherry-pick` скопировал в `release/1.0` только изменения коммита `main` (последнего — исправления), а `feature.txt` в релиз не попал. Это новый коммит с тем же сообщением, но другим хешем. Флаг `-x` дописал в тело строку «(cherry picked from commit <хеш>)» — по ней потом видно, откуда пришёл хотфикс. `cut` здесь только обрезает хеш, который у каждого свой.

</details>

## Спасение и откаты

### reflog — журнал всех перемещений
Git записывает каждое перемещение HEAD и веток: коммиты, переключения, reset, rebase, merge. Журнал хранится локально около 90 дней. **Почти всё, что было закоммичено, можно вернуть.**
```bash
git reflog
# 9c4a1f2 HEAD@{0}: reset: moving to HEAD~3
# 7f3e2d1 HEAD@{1}: commit: add retry logic        ← вот потерянный коммит
# 3b7e2d4 HEAD@{2}: commit: add healthcheck

git reset --hard 7f3e2d1            # вернуть ветку в то состояние
git branch rescue 7f3e2d1           # или безопасно: создать ветку на потерянном коммите
git reflog show feature/x           # журнал конкретной ветки
```
Что **нельзя** вернуть: изменения, которые никогда не были закоммичены или добавлены в stash (например, после `git restore` или `reset --hard` поверх незакоммиченной работы).

### reset — передвинуть ветку
`git reset` перемещает указатель **текущей ветки** на другой коммит. Три режима определяют, что станет с индексом и файлами:
```bash
git reset --soft HEAD~1    # ветка на коммит назад; изменения отменённого коммита ОСТАЮТСЯ в индексе
                           # → можно сразу закоммитить иначе (например, объединить несколько коммитов)
git reset --mixed HEAD~1   # (по умолчанию) изменения остаются в рабочей копии, но не в индексе
git reset --hard HEAD~1    # всё выбрасывается: и коммит, и изменения в файлах
git reset --hard origin/main   # сделать локальную ветку точной копией удалённой
```
reset **переписывает историю** ветки. Для уже запушенных общих веток он не подходит.

### revert — отменить коммит новым коммитом
```bash
git revert a1b2c3d             # создаёт новый коммит с обратными изменениями
git revert -m 1 <merge-sha>    # отменить merge (-m 1: относительно первого родителя — основной ветки)
git revert --no-commit A..B    # отменить диапазон одним коммитом
```
История не переписывается, поэтому это **правильный способ откатить изменение в общей ветке** (main, release).

Нюанс отката merge: если отменили merge фичи через revert, а потом исправили фичу и хотите влить снова, Git решит, что коммиты фичи уже в main (они там есть, просто их изменения отменены). Нужно сначала сделать **revert самого revert-коммита**, а потом влить исправления.

### bisect — найти коммит, который всё сломал
Бинарный поиск по истории. Между «хорошей» и «плохой» версией 1000 коммитов — найдёшь виновника за ~10 шагов.
```bash
git bisect start
git bisect bad                 # текущая версия сломана
git bisect good v2.3.0         # а в этом теге всё работало
# Git переключит на коммит посередине. Проверяешь и отвечаешь:
git bisect good                # или git bisect bad
# ... повторять, пока Git не скажет "a1b2c3d is the first bad commit"
git bisect reset               # вернуться туда, откуда начали

# автоматически: скрипт должен вернуть 0 для «хорошо», 1–127 (кроме 125) для «плохо»
git bisect run ./scripts/test-health.sh
```

### 🧪 Мини-задание

**Задача.** Сделай три коммита и «потеряй» два последних через `reset --hard HEAD~2`. Найди их в `reflog` и верни ветку обратно. Затем отмени последний коммит безопасно для общей ветки — через `revert`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
for c in c1 c2 c3; do echo $c > $c.txt && git add . && git commit -qm $c; done
git reset -q --hard HEAD~2
git log --format='%s'
echo ---; git reflog --format='%gs' | head -3
git reset -q --hard 'HEAD@{1}'
echo ---; git log --format='%s'
git revert -q --no-edit HEAD
echo ---; git log --format='%s'; ls -1
```

Ожидаемый результат:

```text
c1
---
reset: moving to HEAD~2
commit: c3
commit: c2
---
c3
c2
c1
---
Revert "c3"
c3
c2
c1
c1.txt
c2.txt
```

Разбор: `reset --hard` передвинул ветку назад, коммиты c2 и c3 пропали из `log`, но не из репозитория. `reflog` — локальный журнал всех перемещений `HEAD`: `HEAD@{1}` — позиция до последнего действия, то есть до reset. Коммиты хранятся минимум 30 дней, пока их не удалит сборка мусора. `revert` не переписывает историю, а добавляет новый коммит с обратными изменениями: `c3.txt` исчез, а коммит c3 остался. Поэтому в общих ветках откатывают через `revert`, а не через `reset` и `push --force`.

</details>

## Стратегии ветвления

### GitFlow
Ветки: `main` (только релизы), `develop` (интеграция), `feature/*` (от develop), `release/*` (стабилизация перед релизом), `hotfix/*` (от main, вливаются в main и develop).
- Подходит для продуктов с плановыми релизами и поддержкой нескольких версий (коробочное ПО, мобильные приложения).
- Минусы: тяжёлый процесс, долгоживущие ветки, сложные слияния, плохо сочетается с непрерывной доставкой.

### GitHub Flow
`main` всегда готов к деплою. На каждую задачу — короткая ветка, merge request, ревью, CI, merge в main, деплой. Просто и подходит для веб-сервисов с непрерывной доставкой.

### GitLab Flow
GitHub Flow плюс ветки окружений (`main` → `staging` → `production`, изменения продвигаются слиянием) или релизные ветки для поддерживаемых версий.

### Trunk-based development
Все интегрируются в `main` (trunk) очень часто — минимум раз в день. Ветки живут часы, а не недели. Незаконченные фичи прячутся за **feature flags**. Лучше всего подходит для CI/CD: меньше конфликтов, быстрее обратная связь. Требует хорошего покрытия тестами и культуры маленьких изменений.

### Сопутствующие практики
- **Protected branches** — в main нельзя пушить напрямую, только через MR с успешным CI и одобрением.
- **CODEOWNERS** — обязательные ревьюеры для путей (например, `/terraform/prod/ @platform-team`).
- **Conventional Commits** (`feat:`, `fix:`, `chore:`, `feat!:` для ломающих изменений) — соглашение о формате сообщений коммитов; позволяют автоматически генерировать CHANGELOG и версию (semantic-release — инструмент, который по типам коммитов сам решает: `fix` → патч-версия, `feat` → минорная, `!` → мажорная).
- **Подписанные коммиты** (GPG или SSH — коммит подписывается личным ключом, и платформа показывает отметку Verified) — подтверждение авторства: имя и email в коммите можно указать любые, а подделать подпись нельзя.
- **pre-commit хуки** — линтеры, форматирование, поиск секретов (gitleaks) до коммита.

### 🧪 Мини-задание

**Задача.** Проставь релизные аннотированные теги `v1.2.0`, `v1.9.0`, `v1.10.0` в порядке выхода, создай релизную ветку `release/1.10` от последнего тега и найди последний тег от текущего коммита. Сравни обычную и «версионную» сортировку тегов.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
for v in 1.2.0 1.9.0 1.10.0; do
  git commit -q --allow-empty -m "release $v" && git tag -a "v$v" -m "release $v"
done
git branch release/1.10 v1.10.0
git commit -q --allow-empty -m "next work"
git describe --tags --abbrev=0
echo ---; git tag --list
echo ---; git tag --sort=-v:refname
echo ---; git branch --list 'release/*'
```

Ожидаемый результат:

```text
v1.10.0
---
v1.10.0
v1.2.0
v1.9.0
---
v1.10.0
v1.9.0
v1.2.0
---
  release/1.10
```

Разбор: `describe --abbrev=0` нашёл ближайший тег в истории текущего коммита — так CI вычисляет версию сборки. Обычная сортировка строковая, поэтому `v1.10.0` оказался раньше `v1.2.0`. `--sort=-v:refname` сравнивает как версии, по убыванию, — именно так ищут «последний релиз». Релизная ветка от тега — основа GitLab Flow и GitFlow: в неё cherry-pick'ают хотфиксы, а разработка в `main` идёт дальше.

</details>

## Большие репозитории и монорепы
```bash
git clone --depth 1 <url>             # shallow: только последний коммит (быстро в CI)
git clone --filter=blob:none <url>    # partial: история есть, содержимое файлов докачивается по требованию
git sparse-checkout set services/api  # в рабочей копии только нужные папки
git submodule update --init --recursive   # подтянуть сабмодули
```
**Сабмодули** — репозиторий внутри репозитория, зафиксированный на конкретном коммите. Их не любят: легко забыть обновить, сложно в CI, путают новичков. Альтернативы — пакеты и артефакты, версионированные модули (Terraform-модули по тегу), монорепозиторий.

**Git LFS** — для больших бинарных файлов: в репозитории лежит ссылка, а сам файл — в отдельном хранилище.

### 🧪 Мини-задание

**Задача.** Сделай монорепо с тремя коммитами и папками `services/api` и `services/web`. Склонируй его двумя способами: shallow (`--depth 1`) и частично (`--filter=blob:none --sparse`, только `services/api`). Сравни, сколько истории и файлов получилось.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)" && git init -q -b main src && cd src
git config user.name dev && git config user.email dev@example.com
mkdir -p services/api services/web
echo api > services/api/main.go; echo web > services/web/index.js; echo docs > README.md
git add . && git commit -qm one
git commit -q --allow-empty -m two && git commit -q --allow-empty -m three
cd ..
git clone -q --depth 1 "file://$PWD/src" shallow
git -C src rev-list --count HEAD; git -C shallow rev-list --count HEAD
git clone -q --filter=blob:none --sparse "file://$PWD/src" sparse
git -C sparse sparse-checkout set services/api
ls -1 sparse; ls -1 sparse/services
```

Ожидаемый результат:

```text
3
1
README.md
services
api
```

Разбор: shallow-клон скачал только последний коммит — так ускоряют CI-джобы, которым не нужна история (но `git describe` и `git log` в нём неполные). Partial clone с `--filter=blob:none` не качает содержимое файлов заранее, а `sparse-checkout` оставляет в рабочей копии только нужные папки: из `services` есть только `api`, файлы в корне (`README.md`) sparse-режим по умолчанию тоже оставляет. Протокол `file://` нужен, чтобы `--depth` и `--filter` работали и на локальном пути.

</details>

## Кейсы с собеседований

**Закоммитил в main вместо feature-ветки, ещё не запушил. Что делать?**
```bash
git branch feature/x            # новая ветка указывает на текущий коммит — работа сохранена
git reset --hard origin/main    # вернуть main туда, где он на сервере
git switch feature/x            # продолжить работу в правильной ветке
```

**Сделал `reset --hard` и потерял коммиты.**
`git reflog`, найти хеш нужного состояния, `git reset --hard <sha>` или `git branch rescue <sha>`. Работает для всего, что было закоммичено.

**Случайно закоммитили пароль и запушили. Что делать?**
1. **Сначала ротировать секрет.** Он уже скомпрометирован: репозиторий могли склонировать, CI мог его залогировать, на GitHub его находят боты за минуты. Удаление из истории это не отменяет.
2. Проверить логи доступа: не использовался ли секрет.
3. Удалить из истории: `git filter-repo --path config/secrets.env --invert-paths` (filter-repo и BFG Repo-Cleaner — инструменты, которые переписывают всю историю репозитория, вырезая файл или строку из каждого коммита), затем `push --force` во все ветки и теги; попросить всех переклонировать; на GitHub/GitLab почистить кэши и ссылки из MR.
4. Профилактика: pre-commit с gitleaks, secret scanning на платформе (GitHub и GitLab сами проверяют каждый push на известные форматы ключей и могут заблокировать его), секреты в Vault, Secrets Manager или CI-переменных.

**Как отменить уже запушенный коммит в общей ветке?**
`git revert <sha>` и обычный push. Не `reset` + `push --force`: это перепишет историю у всех.

**Чем `git fetch` отличается от `git pull`?**
`fetch` только скачивает новые коммиты и обновляет `origin/*`, ничего не меняя локально. `pull` = `fetch` + `merge` (или `rebase`) в текущую ветку.

**Чем отличаются `reset`, `revert` и `restore`?**
`reset` перемещает указатель ветки (переписывает историю; режимы soft, mixed, hard определяют, что станет с индексом и файлами). `revert` создаёт новый коммит, отменяющий изменения (история не переписывается). `restore` откатывает файлы в рабочей копии или индексе, не трогая коммиты.

**Merge или rebase — что и когда?**
Rebase — подтянуть свежий main в свою личную ветку, получить линейную историю, причесать коммиты перед ревью. Merge — объединять общие ветки и вливать фичу в main. Rebase переписывает хеши, поэтому для опубликованных общих веток не подходит.

**Чем `--force` отличается от `--force-with-lease`?**
`--force-with-lease` откажется перезаписывать удалённую ветку, если в ней появились коммиты, которых ты не видел при последнем fetch. Не уничтожит чужую работу.

**Хотфикс нужно донести в три релизные ветки. Как?**
Фикс в main (или в самую старую поддерживаемую ветку) и `git cherry-pick -x` в остальные. Проверить: `git branch --contains` или, для перенесённых cherry-pick'ом, `git cherry -v release/2.3 main`. Альтернатива — фикс в самую старую ветку и merge вверх по цепочке.

**Как найти коммит, после которого сломалась сборка?**
`git bisect`: отметить хорошую и плохую версию, Git бинарным поиском переключает коммиты. Автоматизировать через `git bisect run ./test.sh`.

**Как понять, попал ли фикс в релиз?**
`git tag --contains <sha>` или `git branch -a --contains <sha>`. Если фикс переносили cherry-pick'ом (другой хеш) — `git cherry` или поиск по номеру задачи `git log --all --grep=JIRA-123`.

**Как посмотреть, что изменится при слиянии feature в main?**
`git log main..feature` — какие коммиты придут. `git diff main...feature` (три точки) — изменения feature относительно точки ответвления, то есть ровно то, что показывает merge request.

**Что такое detached HEAD и чем опасен?**
HEAD указывает прямо на коммит, а не на ветку (после checkout хеша или тега). Новые коммиты не принадлежат никакой ветке, и после переключения их легко потерять. Чтобы сохранить — `git switch -c new-branch`.

**Что такое fast-forward merge?**
Слияние, при котором целевая ветка не ушла вперёд: указатель просто сдвигается на последний коммит вливаемой ветки, merge-коммит не создаётся. `--no-ff` создаёт merge-коммит всегда.

**Как причесать историю перед merge request?**
`git rebase -i main`: объединить мелкие коммиты (`squash`, `fixup`), переписать сообщения (`reword`), удалить лишнее (`drop`). Затем `git push --force-with-lease`.

**Добавил файл в `.gitignore`, а Git продолжает его отслеживать.**
`.gitignore` действует только на неотслеживаемые файлы. Нужно убрать файл из индекса, оставив на диске: `git rm --cached file`, затем закоммитить.

**Во время rebase возник конфликт, а `--ours` взял не то. Почему?**
При rebase стороны меняются местами: «ours» — ветка, на которую переносим (main), «theirs» — твои переносимые коммиты.

**Отменили merge фичи через revert, исправили фичу, а повторный merge ничего не приносит. Почему?**
Коммиты фичи уже есть в истории main, просто их изменения отменены revert-коммитом. Нужно сначала сделать revert этого revert-коммита, а потом влить исправления.

**Почему удалённые ветки не видны в `git branch --merged`, хотя их влили?**
Их вливали squash merge: в main появился один новый коммит, а коммитов ветки там нет. Такие ветки ищут через `git cherry` или по закрытым merge request.

**Удалили ветку. Как восстановить?**
Зависит от того, где её удалили:
- **Удалили локально** (`git branch -D feature/x`), а коммиты не запушены. Ветка — просто указатель, сами коммиты остались в репозитории. Найти последний коммит ветки в reflog и создать ветку заново:
  ```bash
  git reflog | grep feature/x        # строка вида "checkout: moving from feature/x to main" — хеш перед ней
  git branch feature/x a1b2c3d       # или: git switch -c feature/x a1b2c3d
  ```
  При удалении Git сам печатает `Deleted branch feature/x (was a1b2c3d)` — этот хеш тоже подходит.
- **Удалили локально, но ветка есть на сервере** — просто создать локальную копию:
  ```bash
  git fetch --all --prune
  git ls-remote --heads origin feature/x       # убедиться, что ветка есть на сервере
  git switch -c feature/x origin/feature/x     # или просто: git switch feature/x
  ```
- **Удалили на сервере** (`git push origin --delete` или после merge), а у тебя или коллеги есть локальная копия — запушить её обратно: `git push -u origin feature/x`. Если ни у кого нет копии — в GitLab и GitHub коммиты влитой ветки остаются в истории merge request и в целевой ветке; хеш последнего коммита можно найти в MR и создать ветку от него.
