# Ansible

Подробный разбор Ansible: как он устроен, инвентарь, плейбуки, модули и идемпотентность, переменные и их приоритеты, шаблоны, handlers, роли, Vault, rolling update без простоя, скорость, тестирование и CI. Каждая тема: что это, зачем, как работает, пример с объяснением и типичные ошибки. В конце — вопросы с собеседований с ответами.

Как читать: собери стенд из следующего раздела и запускай примеры по ходу. Вывод Ansible показан в комментариях после `# →`.

## Как учить без курса

Ansible лучше всего учится руками: написал задачу → запустил → запустил ещё раз и убедился, что второй раз ничего не изменилось. Для этого нужны «серверы», которые не жалко сломать.

**Установка на macOS:**
```bash
brew install ansible            # ставит пакет ansible: ansible-core + набор коллекций (community.*, amazon.aws и др.)
ansible --version               # версия, путь к ansible.cfg и к Python, на котором работает Ansible
```
`ansible-core` — это сам движок и встроенные модули `ansible.builtin.*`. Пакет `ansible` — движок плюс большой набор коллекций. На собеседовании это различие иногда спрашивают.

**Стенд вариант 1 — Docker-контейнеры (быстро, но без systemd):**
```bash
for n in node1 node2 node3; do
  docker run -d --name "$n" python:3.12-slim sleep infinity   # в образе уже есть Python — Ansible он нужен на цели
done
```
```ini
# inventory.ini
[lab]
node1
node2
node3

[lab:vars]
ansible_connection=community.docker.docker   # подключаться через docker exec, а не по SSH
```
```bash
ansible -i inventory.ini lab -m ansible.builtin.ping
# → node1 | SUCCESS => {"changed": false, "ping": "pong"}
```
Ограничение: внутри контейнера нет systemd, поэтому модули `service`/`systemd` не заработают.

**Стенд вариант 2 — виртуальные машины (как в жизни):** `brew install multipass`, затем `multipass launch -n node1` и так для 2–3 машин. Это полноценные Ubuntu с SSH и systemd. IP машин смотри через `multipass list`, ключ добавь в `~/.ssh/authorized_keys` пользователя `ubuntu` на каждой машине.

План: инвентарь → ad-hoc команды → первый плейбук → переменные и шаблоны → handlers → роль → Vault → rolling update. После каждого шага — повторный запуск и проверка, что `changed=0`.

## Что такое Ansible и как он работает

**Ansible** — инструмент управления конфигурацией и автоматизации: ставит пакеты, раскладывает конфиги, создаёт пользователей, перезапускает сервисы, выкатывает приложения — на десятки и сотни серверов одной командой.

Главные свойства:
- **Agentless** — на серверах не нужен агент. Нужны только SSH-доступ и Python (для Linux). Сравни с Puppet и Chef, где на каждом сервере работает агент, который периодически тянет конфигурацию с центрального сервера.
- **Push-модель** — изменения запускаешь ты (или CI) с управляющей машины (**control node**), а не серверы сами забирают их по расписанию.
- **Декларативные задачи** — описываешь желаемое состояние («пакет nginx установлен», «файл с таким содержимым лежит здесь»), а модуль сам проверяет текущее состояние и меняет только то, что отличается.
- **YAML** — плейбуки читаются почти как текст, порог входа низкий.

**Что происходит при запуске задачи** (упрощённо):
1. Ansible читает инвентарь и выбирает хосты.
2. Подключается к каждому хосту по SSH (параллельно, по умолчанию до 5 хостов одновременно — параметр `forks`).
3. Для каждой задачи берёт код модуля (Python), подставляет аргументы, копирует на хост во временную папку `~/.ansible/tmp/...`.
4. Запускает модуль на хосте через Python. Модуль проверяет состояние, при необходимости меняет его и возвращает JSON: `changed`, `failed`, данные.
5. Удаляет временные файлы и переходит к следующей задаче.

По умолчанию Ansible выполняет **задачу на всех хостах, затем следующую задачу на всех хостах** (стратегия `linear`). Если задача упала на хосте, этот хост выбывает из дальнейших задач плея, остальные продолжают.

**Ansible и Terraform — не конкуренты.** Terraform создаёт инфраструктуру (VPC, виртуалки, балансировщики) и хранит state. Ansible настраивает то, что уже создано (ОС, пакеты, конфиги, деплой), и state не хранит — каждый раз проверяет реальное состояние заново. Типичная связка: Terraform поднял EC2 → Ansible их настроил.

### 🧪 Мини-задание

**Задача.** Без агентов и без SSH поработай с локальной машиной как с управляемым хостом: проверь связь модулем `ping` и выполни команду модулем `command`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

ansible all -i hosts -m ping
ansible all -i hosts -m command -a 'uname -s'
```

Ожидаемый результат:

```text
localhost | SUCCESS => {
    "changed": false,
    "ping": "pong"
}
localhost | CHANGED | rc=0 >>
Darwin
```

Разбор: Ansible работает по push-модели и без агентов: управляющая машина подключается к хосту (обычно по SSH, здесь `ansible_connection=local`), копирует туда маленький Python-модуль, запускает его и забирает JSON-результат. `ping` — не ICMP, а проверка «могу подключиться и выполнить Python». `command` всегда отчитывается `CHANGED`, потому что Ansible не знает, что меняет произвольная команда. Поэтому для изменений берут специализированные модули, которые сами проверяют текущее состояние.

</details>

## Инвентарь

**Инвентарь** — список хостов, которыми управляет Ansible, разбитый на группы. Без инвентаря Ansible не знает, куда подключаться.

**INI-формат:**
```ini
# inventory/prod.ini
[web]
web1.example.com
web2.example.com ansible_host=10.0.1.12        # имя для Ansible одно, а подключаться по этому IP

[db]
db1 ansible_host=10.0.2.10 ansible_user=admin ansible_port=2222

[swarm_managers]
swarm-m[1:3].example.com                        # диапазон: swarm-m1, swarm-m2, swarm-m3

[prod:children]                                 # группа из групп
web
db
swarm_managers
```

**То же в YAML:**
```yaml
# inventory/prod.yml
all:
  children:
    web:
      hosts:
        web1.example.com:
        web2.example.com:
          ansible_host: 10.0.1.12
    db:
      hosts:
        db1:
          ansible_host: 10.0.2.10
          ansible_user: admin
          ansible_port: 2222
```

Есть две встроенные группы: `all` (все хосты) и `ungrouped` (хосты без группы). Хост может входить в несколько групп сразу.

**Служебные переменные подключения** (их понимает сам Ansible):
| Переменная | Смысл |
|---|---|
| `ansible_host` | адрес для подключения, если имя хоста не резолвится |
| `ansible_user` | пользователь SSH |
| `ansible_port` | порт SSH |
| `ansible_connection` | способ подключения: `ssh` (по умолчанию), `local`, `community.docker.docker`, `winrm` |
| `ansible_python_interpreter` | путь к Python на хосте, если автоопределение ошиблось |
| `ansible_become`, `ansible_become_user` | повышать ли права и до кого |

**Переменные групп и хостов выносят в папки**, а не пишут в инвентарь:
```text
inventory/
├── prod.ini
├── group_vars/
│   ├── all.yml          # для всех хостов
│   ├── web.yml          # для группы web
│   └── db/              # можно папкой: все файлы внутри читаются
│       ├── main.yml
│       └── vault.yml    # зашифрованные секреты группы db
└── host_vars/
    └── db1.yml          # только для хоста db1
```
Ansible ищет `group_vars/` и `host_vars/` рядом с файлом инвентаря и рядом с плейбуком.

**Проверить, как Ansible видит инвентарь:**
```bash
ansible-inventory -i inventory/prod.ini --graph            # дерево групп и хостов
ansible-inventory -i inventory/prod.ini --host db1         # все переменные хоста после слияния
ansible -i inventory/prod.ini web --list-hosts             # какие хосты попадут под шаблон
```

**Динамический инвентарь.** В облаке серверы появляются и исчезают, поэтому статический список быстро устаревает. Плагин инвентаря сам спрашивает API облака:
```yaml
# inventory/aws_ec2.yml — имя файла обязано заканчиваться на aws_ec2.yml
plugin: amazon.aws.aws_ec2
regions:
  - eu-central-1
filters:
  instance-state-name: running
  tag:Project: shop
keyed_groups:
  - key: tags.Role               # тег Role=web → группа role_web
    prefix: role
hostnames:
  - private-ip-address
compose:
  ansible_host: private_ip_address
```
```bash
ansible-inventory -i inventory/aws_ec2.yml --graph
# → @all:
# →   |--@role_web:
# →   |  |--10.0.1.15
# →   |  |--10.0.1.27
```
Плагину нужны AWS-креды (профиль, переменные окружения или роль EC2) и библиотеки `boto3`/`botocore`.

**Шаблоны выбора хостов** (pattern) в командах и в `hosts:` плейбука: `web` — группа; `web:db` — объединение; `web:&prod` — пересечение; `prod:!db` — prod без db; `web[0]` — первый хост группы.

### 🧪 Мини-задание

**Задача.** Опиши YAML-инвентарь: группа `prod` с подгруппами `web` (два хоста) и `db`, переменные на уровне группы и хоста. Выведи состав группы `web` и итоговые переменные хоста `web1`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > inventory.yml <<'EOF'
prod:
  vars:
    env: prod
  children:
    web:
      vars:
        http_port: 8080
      hosts:
        web1: {ansible_host: 10.0.1.11}
        web2: {ansible_host: 10.0.1.12}
    db:
      hosts:
        db1: {ansible_host: 10.0.2.21}
EOF
ansible-inventory -i inventory.yml --graph web
ansible-inventory -i inventory.yml --host web1
```

Ожидаемый результат:

```text
@web:
  |--web1
  |--web2
{
    "ansible_host": "10.0.1.11",
    "env": "prod",
    "http_port": 8080
}
```

Разбор: `web1` получил переменные со всех уровней: `env` от родительской группы `prod`, `http_port` от `web` и `ansible_host` от себя. `ansible-inventory --host` показывает итоговый результат слияния — первым делом смотрят именно его, когда «переменная не та». В проде переменные обычно выносят из инвентаря в `group_vars/<группа>.yml` и `host_vars/<хост>.yml`, а для облака используют динамический инвентарь (плагин `amazon.aws.aws_ec2` строит группы из тегов EC2).

</details>

## ansible.cfg

`ansible.cfg` — файл настроек. Ansible берёт **первый найденный** (не объединяет их):
1. путь из переменной окружения `ANSIBLE_CONFIG`;
2. `./ansible.cfg` в текущей папке;
3. `~/.ansible.cfg`;
4. `/etc/ansible/ansible.cfg`.

Важная деталь безопасности: `ansible.cfg` из текущей папки **игнорируется, если папка доступна на запись всем** (world-writable). Иначе любой пользователь мог бы подсунуть свой конфиг. Ansible предупредит об этом в выводе.

Типичный `ansible.cfg` в репозитории проекта:
```ini
[defaults]
inventory = inventory/prod.ini
remote_user = deploy
roles_path = roles
forks = 20                     ; хостов параллельно (по умолчанию 5)
host_key_checking = True       ; не отключай в проде: защита от подмены сервера
callback_result_format = yaml  ; читаемый вывод результатов и ошибок (ansible-core 2.13+)
interpreter_python = auto_silent

[ssh_connection]
pipelining = True              ; меньше SSH-операций на задачу, заметно быстрее
ssh_args = -o ControlMaster=auto -o ControlPersist=60s
```
Посмотреть, что реально применилось: `ansible-config dump --only-changed`.

### 🧪 Мини-задание

**Задача.** Положи в папку проекта `ansible.cfg` с числом потоков 20, таймаутом подключения 15 секунд и отключённой проверкой ключей хостов. Проверь через `ansible-config`, какие настройки отличаются от умолчаний и откуда они взялись.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > ansible.cfg <<'EOF'
[defaults]
forks = 20
timeout = 15
host_key_checking = False
EOF
ansible-config dump --only-changed | grep -E '^(DEFAULT_FORKS|DEFAULT_TIMEOUT|HOST_KEY_CHECKING)' | sed 's/(.*)//'
ansible --version | grep 'config file' | sed 's|=.*/|= .../|'
```

Ожидаемый результат:

```text
DEFAULT_FORKS = 20
DEFAULT_TIMEOUT = 15
HOST_KEY_CHECKING = False
  config file = .../ansible.cfg
```

Разбор: Ansible ищет конфиг в таком порядке: переменная `ANSIBLE_CONFIG`, `ansible.cfg` в текущей папке, `~/.ansible.cfg`, `/etc/ansible/ansible.cfg` — и берёт первый найденный, а не сливает их. Поэтому конфиг кладут в репозиторий рядом с плейбуками: у всех в команде и в CI одинаковое поведение. `--only-changed` показывает отличия от умолчаний, а `ansible --version` — какой файл реально используется. Это первое, что проверяют, если «у меня работает, а в CI нет». Отключать `host_key_checking` уместно в динамических облачных средах, но это компромисс безопасности.

</details>

## Ad-hoc команды

**Ad-hoc команда** — одна задача без плейбука, прямо из терминала. Удобно для разовых действий и диагностики.

```bash
ansible all -m ansible.builtin.ping                         # доступны ли хосты и есть ли Python
ansible web -m ansible.builtin.command -a "uptime"          # выполнить команду
ansible web -m ansible.builtin.shell -a "df -h | grep /var" # нужен пайп — значит shell
ansible web -b -m ansible.builtin.apt -a "name=htop state=present update_cache=true"   # -b = sudo
ansible db1 -m ansible.builtin.setup -a "filter=ansible_distribution*"                # факты о системе
ansible all -m ansible.builtin.reboot -b --limit 'web2'    # перезагрузить и дождаться возврата
ansible all -f 50 -m ansible.builtin.command -a "systemctl is-active nginx"           # 50 хостов параллельно
```

Ключи, которые нужны постоянно:
| Ключ | Что делает |
|---|---|
| `-i` | инвентарь |
| `-m` / `-a` | модуль и его аргументы |
| `-b` | become (по умолчанию через sudo) |
| `-K` | спросить пароль sudo |
| `-u` | пользователь SSH |
| `--limit` | сузить список хостов |
| `-f` | число параллельных хостов |
| `-v`…`-vvvv` | подробность вывода; `-vvvv` показывает и детали SSH |

**command против shell.** `command` запускает программу напрямую, без shell: не работают пайпы `|`, перенаправления `>`, `&&`, подстановка `$VAR`. Зато нет проблем с экранированием и инъекциями. `shell` запускает через `/bin/sh`, и всё это работает. Правило: по умолчанию `command`, `shell` — только когда действительно нужны возможности shell.

### 🧪 Мини-задание

**Задача.** Ad-hoc командой модуля `copy` создай файл с текстом и запусти ту же команду ещё раз. Сравни статусы двух запусков, а затем проверь содержимое модулем `command`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

ansible all -i hosts -m copy -a "content='maintenance\n' dest=$PWD/motd" | head -1
ansible all -i hosts -m copy -a "content='maintenance\n' dest=$PWD/motd" | head -1
ansible all -i hosts -m command -a "cat $PWD/motd" | tail -1
```

Ожидаемый результат:

```text
localhost | CHANGED => {
localhost | SUCCESS => {
maintenance
```

Разбор: первый запуск создал файл — `CHANGED`. Второй сравнил содержимое (по контрольной сумме) с желаемым и ничего не делал — `SUCCESS`, это и есть идемпотентность. Ad-hoc команды удобны для разовых действий на многих хостах: перезапустить сервис, проверить версию пакета, собрать факты (`-m setup`). Всё, что нужно повторять и ревьюить, оформляют плейбуком.

</details>

## Первый плейбук

**Плейбук** — YAML-файл со списком **плеев** (play). Плей связывает группу хостов с набором задач. **Задача** (task) — вызов одного модуля с аргументами.

```yaml
# site.yml
- name: Настроить веб-серверы            # плей
  hosts: web
  become: true                            # все задачи — через sudo

  tasks:
    - name: Установить nginx              # задача
      ansible.builtin.apt:
        name: nginx
        state: present
        update_cache: true
        cache_valid_time: 3600            # не обновлять кэш apt, если он свежее часа

    - name: Положить главную страницу
      ansible.builtin.copy:
        content: "Hello from {{ inventory_hostname }}\n"
        dest: /var/www/html/index.html
        owner: www-data
        group: www-data
        mode: "0644"

    - name: nginx запущен и в автозагрузке
      ansible.builtin.service:
        name: nginx
        state: started
        enabled: true
```

```bash
ansible-playbook -i inventory.ini site.yml
# → PLAY RECAP
# → web1 : ok=4  changed=3  unreachable=0  failed=0  skipped=0
ansible-playbook -i inventory.ini site.yml        # второй запуск
# → web1 : ok=4  changed=0  ...                    — ничего не изменилось: так и должно быть
```
Первая «лишняя» задача `ok` — это сбор фактов (`Gathering Facts`), он идёт в начале каждого плея.

**Что значат статусы:**
- `ok` — состояние уже было нужным, ничего не делали;
- `changed` — модуль что-то изменил;
- `failed` — ошибка, хост выбывает из плея;
- `skipped` — пропущено по условию `when`;
- `unreachable` — не смогли подключиться.

**Полезные ключи запуска:**
```bash
ansible-playbook site.yml --syntax-check          # проверка синтаксиса без подключения
ansible-playbook site.yml --check --diff          # «сухой прогон»: что изменилось бы, и дифф файлов
ansible-playbook site.yml --limit web1            # только один хост
ansible-playbook site.yml --tags nginx            # только задачи с тегом
ansible-playbook site.yml --list-tasks            # какие задачи будут выполнены
ansible-playbook site.yml --start-at-task "Положить главную страницу"
ansible-playbook site.yml --step                  # спрашивать подтверждение перед каждой задачей
ansible-playbook site.yml -e "app_version=1.4.2"  # передать переменную (высший приоритет)
```

### 🧪 Мини-задание

**Задача.** Напиши плейбук из двух задач: создать каталог `app` и положить в него `config.ini`. Запусти его дважды и сравни строки `PLAY RECAP`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

cat > site.yml <<'EOF'
- name: Configure app
  hosts: all
  gather_facts: false
  tasks:
    - name: Create app directory
      ansible.builtin.file:
        path: "{{ playbook_dir }}/app"
        state: directory
        mode: "0755"

    - name: Write config
      ansible.builtin.copy:
        dest: "{{ playbook_dir }}/app/config.ini"
        content: "[server]\nport = 8080\n"
        mode: "0644"
EOF
ansible-playbook -i hosts site.yml | grep '^localhost' | awk '{print $3, $4, $7}'
ansible-playbook -i hosts site.yml | grep '^localhost' | awk '{print $3, $4, $7}'
cat app/config.ini
```

Ожидаемый результат:

```text
ok=2 changed=2 failed=0
ok=2 changed=0 failed=0
[server]
port = 8080
```

Разбор: плейбук — список plays (на каких хостах), а в каждом — список tasks, выполняемых по порядку. Первый прогон изменил обе вещи, второй — ничего, потому что каталог и файл уже в нужном состоянии. Это главный критерий качества плейбука: повторный запуск даёт `changed=0`. Если на втором прогоне что-то снова `changed`, задача не идемпотентна, и её надо исправить. `gather_facts: false` пропускает сбор фактов о системе — быстрее, когда они не нужны. Полные имена модулей (`ansible.builtin.file`, FQCN) — современный стандарт.

</details>

## Модули и идемпотентность

**Идемпотентность** — свойство, при котором повторный запуск не меняет систему, если она уже в нужном состоянии. Это главная идея Ansible: плейбук можно запускать сколько угодно раз, и он только «дотягивает» серверы до описанного состояния. На собеседовании про это спрашивают почти всегда.

Идемпотентность обеспечивают **модули**: `apt` сначала проверяет, установлен ли пакет; `copy` сравнивает контрольную сумму файла; `user` смотрит, есть ли пользователь. Поэтому всегда ищи готовый модуль, а не пиши `shell`.

**Почему command/shell ломают идемпотентность.** Ansible не знает, что делает произвольная команда, поэтому всегда считает её `changed` и выполняет каждый раз. Как это исправить:
```yaml
- name: Инициализировать Swarm (только если ещё не инициализирован)
  ansible.builtin.command: docker swarm init --advertise-addr {{ ansible_default_ipv4.address }}
  args:
    creates: /var/lib/docker/swarm/state.json     # если файл есть — задача пропускается

- name: Узнать версию приложения
  ansible.builtin.command: /opt/app/bin/app --version
  register: app_version
  changed_when: false                              # чтение ничего не меняет — не пиши changed

- name: Применить миграции БД
  ansible.builtin.command: /opt/app/bin/migrate up
  register: migrate
  changed_when: "'no change' not in migrate.stdout"   # changed только если миграции реально были
```
Ещё лучше — модуль, который уже умеет это сам. Для примера выше: `community.docker.docker_swarm` с `state: present`.

**FQCN — полное имя модуля.** Пиши `ansible.builtin.copy`, а не просто `copy`. Короткие имена работают, но при установленных коллекциях может найтись модуль с тем же именем из другой коллекции. ansible-lint требует FQCN.

**Модули, которые нужны каждый день:**

| Задача | Модуль | Пример |
|---|---|---|
| Пакеты | `ansible.builtin.package` (универсальный), `apt`, `dnf` | `name: [curl, jq] state: present` |
| Сервисы | `ansible.builtin.systemd` / `service` | `name: nginx state: restarted daemon_reload: true` |
| Файл целиком | `ansible.builtin.copy` | статический файл или `content:` |
| Файл из шаблона | `ansible.builtin.template` | Jinja2-шаблон с переменными |
| Права, папки, ссылки | `ansible.builtin.file` | `state: directory mode: "0750"` |
| Строка в файле | `ansible.builtin.lineinfile` | поменять одну строку в чужом конфиге |
| Блок в файле | `ansible.builtin.blockinfile` | вставить блок между маркерами |
| Пользователи | `ansible.builtin.user`, `ansible.posix.authorized_key` | пользователь и его SSH-ключ |
| Скачать / распаковать | `get_url`, `unarchive` | бинарник с checksum, архив |
| HTTP-запрос | `ansible.builtin.uri` | healthcheck, вызов API |
| Ожидание | `wait_for`, `wait_for_connection` | порт открылся, хост вернулся после ребута |
| Docker | `community.docker.docker_container`, `docker_swarm`, `docker_node` | контейнеры и Swarm |
| Kubernetes | `kubernetes.core.k8s` | применить манифест |

Документация по любому модулю — прямо в терминале: `ansible-doc ansible.builtin.lineinfile` (раздел EXAMPLES в конце особенно полезен), список модулей — `ansible-doc -l`.

**lineinfile против template.** `lineinfile` правит одну строку и подходит для чужих конфигов, которые ты не хочешь контролировать целиком (например, одна опция в `/etc/sysctl.conf`). Если же файл твой — генерируй его целиком через `template`. Десяток `lineinfile` по одному файлу — признак, что пора переходить на шаблон: их порядок и взаимодействие быстро становятся непредсказуемыми.

### 🧪 Мини-задание

**Задача.** Добавь строку `timeout=30` в конфиг двумя способами — через `shell` с `>>` и через модуль `lineinfile` — и запусти каждый по три раза. Сравни итоговые файлы.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

echo "port=80" > shell.conf; echo "port=80" > module.conf
for i in 1 2 3; do
  ansible all -i hosts -m shell -a "echo timeout=30 >> $PWD/shell.conf" > /dev/null
  ansible all -i hosts -m lineinfile -a "path=$PWD/module.conf line=timeout=30" > /dev/null
done
echo "shell:";  cat shell.conf
echo "lineinfile:"; cat module.conf
```

Ожидаемый результат:

```text
shell:
port=80
timeout=30
timeout=30
timeout=30
lineinfile:
port=80
timeout=30
```

Разбор: `shell` просто выполняет команду — каждый запуск дописывает ещё одну строку, и конфиг портится. `lineinfile` описывает желаемое состояние («в файле есть такая строка») и сначала проверяет, выполнено ли оно. Отсюда правило: `shell` и `command` — последнее средство. Если без них не обойтись, добавляют `creates`/`removes` или `changed_when`, чтобы задача честно сообщала об изменениях и не повторяла действие. Для конфигов целиком лучше `template`, чем набор `lineinfile`.

</details>

## Переменные

Переменные делают один плейбук пригодным для разных окружений и хостов: версия приложения, порты, список пользователей.

**Где их определяют** (основные места):
- в инвентаре и в `group_vars/`, `host_vars/`;
- в плейбуке: `vars:`, `vars_files:`;
- в роли: `defaults/main.yml` (значения по умолчанию) и `vars/main.yml`;
- в задаче: `vars:` на уровне задачи или блока;
- динамически: `register` (результат задачи), `set_fact`;
- в командной строке: `-e` / `--extra-vars`.

**Приоритеты** — что победит, если одна переменная задана в нескольких местах. Полный список — 22 уровня, запоминать его целиком не нужно. Достаточно понимать порядок от слабого к сильному:
1. `defaults/` роли — самые слабые: «значение по умолчанию, переопредели меня»;
2. инвентарь, затем `group_vars/all`, затем `group_vars/<группа>`, затем `host_vars/<хост>` — **хост сильнее группы, конкретная группа сильнее all**;
3. факты хоста;
4. переменные плея (`vars`, `vars_files`);
5. `vars/` роли;
6. переменные блока и задачи;
7. `set_fact` и `register`;
8. `-e` в командной строке — **сильнее всего**, переопределить нельзя ничем.

Практические выводы:
- то, что пользователь роли должен иметь возможность поменять, клади в `defaults/`, а не в `vars/` роли — иначе его `group_vars` не сработают;
- `-e` удобен в CI (`-e app_version=$CI_COMMIT_TAG`), но злоупотреблять им не стоит: значение молча перебьёт всё остальное.

Если две группы одного уровня задают одну переменную (хост входит в обе), побеждает группа, которая идёт позже по алфавиту. Управлять этим можно через `ansible_group_priority`. Лучше не допускать таких пересечений вовсе.

**Факты** — переменные, которые Ansible собирает о хосте в начале плея (модуль `setup`): ОС, IP, память, диски.
```yaml
- name: Показать пару фактов
  ansible.builtin.debug:
    msg: "{{ ansible_facts['distribution'] }} {{ ansible_facts['distribution_version'] }}, IP {{ ansible_facts['default_ipv4']['address'] }}"
```
Если факты не нужны, `gather_facts: false` в плее экономит секунды на каждом хосте.

**register** сохраняет результат задачи:
```yaml
- name: Проверить, есть ли swap
  ansible.builtin.command: swapon --show --noheadings
  register: swap
  changed_when: false

- name: Отключить swap (нужно для Kubernetes)
  ansible.builtin.command: swapoff -a
  when: swap.stdout | length > 0
```
В зарегистрированной переменной есть `stdout`, `stderr`, `rc`, `changed`, `failed`, а у модулей — их собственные поля. Посмотреть структуру: `- ansible.builtin.debug: var=swap`.

**Магические переменные** — Ansible задаёт их сам:
- `inventory_hostname` — имя хоста из инвентаря;
- `groups` — словарь «группа → список хостов»: `groups['swarm_managers'][0]`;
- `hostvars` — переменные и факты других хостов: `hostvars['db1']['ansible_host']`;
- `play_hosts` / `ansible_play_hosts` — хосты текущего плея.

Пример: на воркерах Swarm нужен токен, который знает только первый менеджер:
```yaml
- name: Подключить воркеры к Swarm
  community.docker.docker_swarm:
    state: join
    join_token: "{{ hostvars[groups['swarm_managers'][0]]['swarm_info']['swarm_facts']['JoinTokens']['Worker'] }}"
    remote_addrs: ["{{ hostvars[groups['swarm_managers'][0]]['ansible_host'] }}:2377"]
```
Здесь `swarm_info` — результат `register` задачи, выполненной ранее на первом менеджере. Важно: переменная из `hostvars` появится, только если та задача уже выполнилась на том хосте в этом же запуске.

### 🧪 Мини-задание

**Задача.** Задай переменную `release` в трёх местах: `group_vars/all.yml`, `vars` плейбука и `-e` в командной строке. Запусти плейбук без `-e` и с `-e` и посмотри, какое значение победило.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

mkdir group_vars && echo 'release: from group_vars' > group_vars/all.yml
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars:
    release: from play vars
  tasks:
    - ansible.builtin.debug:
        msg: "{{ release }}"
EOF
ansible-playbook -i hosts site.yml | grep '"msg"'
ansible-playbook -i hosts site.yml -e release=from_extra_vars | grep '"msg"'
```

Ожидаемый результат:

```text
    "msg": "from play vars"
    "msg": "from_extra_vars"
```

Разбор: у Ansible больше 20 уровней приоритета переменных. Запомнить нужно главное: `defaults` роли — самый слабый уровень (значения по умолчанию, которые ожидают переопределения), затем инвентарь и `group_vars`/`host_vars`, затем `vars` плейбука и роли, а `-e` (extra vars) побеждает всё. Поэтому `-e` используют для разовых параметров запуска (версия, флаг), а не для постоянной настройки. Если одна и та же переменная задана в пяти местах, отладка превращается в квест — держи каждую переменную в одном очевидном месте.

</details>

## Jinja2 и шаблоны

Ansible подставляет значения через **Jinja2**: `{{ переменная }}`, фильтры `{{ x | filter }}`, условия и циклы `{% ... %}` в шаблонах.

**Частые фильтры:**
```yaml
port: "{{ app_port | default(8080) }}"                   # значение, если переменная не задана
name: "{{ service_name | mandatory }}"                   # упасть с понятной ошибкой, если не задана
hosts: "{{ groups['web'] | join(',') }}"                 # список → строка
upper: "{{ env | upper }}"
is_prod: "{{ (env == 'prod') | ternary('yes', 'no') }}"
cfg: "{{ app_config | to_nice_yaml }}"                   # словарь → YAML-текст
ips: "{{ groups['web'] | map('extract', hostvars, 'ansible_host') | list }}"   # список IP всех web
```

**Модуль template** рендерит файл на control node и кладёт результат на хост. Шаблоны лежат в `templates/` и имеют расширение `.j2`:
```jinja
{# templates/haproxy.cfg.j2 — комментарий Jinja, в итоговый файл не попадёт #}
# Управляется Ansible. Ручные правки будут перезаписаны.
global
    maxconn {{ haproxy_maxconn | default(4000) }}

frontend http
    bind *:80
    default_backend app

backend app
    balance roundrobin
{% for host in groups['app'] %}
    server {{ host }} {{ hostvars[host]['ansible_host'] }}:{{ app_port }} check
{% endfor %}
```
```yaml
- name: Конфиг HAProxy
  ansible.builtin.template:
    src: haproxy.cfg.j2
    dest: /etc/haproxy/haproxy.cfg
    mode: "0644"
    validate: haproxy -c -f %s          # проверить файл ДО того, как он заменит рабочий
  notify: Reload haproxy
```
`validate` — очень полезная опция: Ansible кладёт новый файл во временное место, запускает проверку (`%s` — путь к временному файлу), и только при успехе ставит его на место. Битый конфиг так не попадёт на сервер. Классика: `visudo -cf %s` для sudoers, `sshd -t -f %s` для sshd.

Строку «Управляется Ansible» в начале файла стоит писать всегда: коллеги поймут, что править руками бесполезно.

**Lookups** читают данные на control node: `{{ lookup('file', 'files/id_rsa.pub') }}`, `{{ lookup('env', 'HOME') }}`, `{{ lookup('pipe', 'git rev-parse --short HEAD') }}`.

### 🧪 Мини-задание

**Задача.** Напиши Jinja2-шаблон блока `upstream` для nginx, который строит строки `server` из списка бэкендов (у одного из них нет порта — подставь 80 по умолчанию) и пишет комментарий с числом серверов. Отрендери его модулем `template`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

cat > upstream.conf.j2 <<'EOF'
# {{ backends | length }} servers, managed by Ansible
upstream {{ app_name | lower }} {
{% for b in backends %}
    server {{ b.host }}:{{ b.port | default(80) }};
{% endfor %}
}
EOF
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars:
    app_name: Shop
    backends:
      - {host: 10.0.1.11, port: 8080}
      - {host: 10.0.1.12, port: 8080}
      - {host: 10.0.1.13}
  tasks:
    - ansible.builtin.template:
        src: upstream.conf.j2
        dest: "{{ playbook_dir }}/upstream.conf"
EOF
ansible-playbook -i hosts site.yml > /dev/null
cat upstream.conf
```

Ожидаемый результат:

```text
# 3 servers, managed by Ansible
upstream shop {
    server 10.0.1.11:8080;
    server 10.0.1.12:8080;
    server 10.0.1.13:80;
}
```

Разбор: шаблон рендерится на управляющей машине, а на хост попадает готовый файл. `{{ }}` подставляет значения, `{% %}` — управляющие конструкции, `|` — фильтры (`length`, `lower`, `default`). Ansible по умолчанию включает `trim_blocks`, поэтому строки с `{% for %}` не оставляют пустых строк. Фильтр `default` спасает от ошибки, когда у элемента нет ключа. Комментарий «managed by Ansible» предупреждает, что ручные правки затрутся при следующем запуске. Для проверки сгенерированного конфига у `template` есть параметр `validate: nginx -t -c %s`.

</details>

## Условия, циклы и обработка ошибок

**when** — выполнить задачу только при условии. Пишется без `{{ }}`:
```yaml
- name: Пакеты для Debian-семейства
  ansible.builtin.apt:
    name: chrony
  when: ansible_facts['os_family'] == "Debian"

- name: Пакеты для RHEL-семейства
  ansible.builtin.dnf:
    name: chrony
  when:
    - ansible_facts['os_family'] == "RedHat"
    - ansible_facts['distribution_major_version'] | int >= 8   # список условий = логическое И
```

**loop** — повторить задачу для каждого элемента:
```yaml
- name: Пользователи команды
  ansible.builtin.user:
    name: "{{ item.name }}"
    groups: "{{ item.groups }}"
    shell: /bin/bash
  loop:
    - { name: alex, groups: "sudo,docker" }
    - { name: ci,   groups: "docker" }
  loop_control:
    label: "{{ item.name }}"      # в выводе показывать только имя, а не весь словарь
```
Для пакетов цикл не нужен: `apt` и `dnf` принимают список в `name:` и ставят всё одной операцией — это намного быстрее.

**until** — повторять, пока условие не выполнится (ожидание):
```yaml
- name: Дождаться, пока приложение ответит 200
  ansible.builtin.uri:
    url: "http://{{ ansible_host }}:{{ app_port }}/health"
    status_code: 200
  register: health
  until: health.status == 200
  retries: 30
  delay: 5                          # до 30 попыток раз в 5 секунд
```

**Управление ошибками:**
- `failed_when` — своё правило, что считать ошибкой: `failed_when: "'ERROR' in result.stdout"`;
- `ignore_errors: true` — продолжить, даже если задача упала (используй редко: ошибки прячутся);
- `any_errors_fatal: true` в плее — упала задача на одном хосте → остановить плей на всех (нужно, когда кластеру опасна частичная выкатка);
- `max_fail_percentage` — допустимая доля упавших хостов в батче.

**block / rescue / always** — аналог try / except / finally:
```yaml
- name: Обновление с откатом
  block:
    - name: Выкатить новую версию
      ansible.builtin.copy:
        src: "app-{{ app_version }}.jar"
        dest: /opt/app/app.jar
      notify: Restart app

    - name: Применить перезапуск сейчас, чтобы проверить здоровье
      ansible.builtin.meta: flush_handlers

    - name: Проверить здоровье
      ansible.builtin.uri:
        url: "http://localhost:{{ app_port }}/health"
      register: health
      until: health.status == 200
      retries: 12
      delay: 5

  rescue:
    - name: Вернуть предыдущую версию
      ansible.builtin.copy:
        src: "app-{{ app_previous_version }}.jar"
        dest: /opt/app/app.jar
      notify: Restart app

    - name: Пометить хост как упавший после отката
      ansible.builtin.fail:
        msg: "Версия {{ app_version }} не поднялась, откатились на {{ app_previous_version }}"

  always:
    - name: Записать результат
      ansible.builtin.lineinfile:
        path: /var/log/deploy.log
        line: "{{ now(utc=true, fmt='%Y-%m-%dT%H:%M:%SZ') }} {{ app_version }} {{ 'ok' if (health is defined and health is succeeded) else 'rollback' }}"
        create: true
```
Задачи в `rescue` выполняются, только если что-то в `block` упало. `always` — в любом случае.

### 🧪 Мини-задание

**Задача.** Пройди циклом по списку сервисов и выведи только включённые (`when`). Затем в блоке `block` выполни заведомо падающую команду и обработай ошибку в `rescue`, а в `always` выведи сообщение, которое должно появиться в любом случае.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars:
    services:
      - {name: api, enabled: true}
      - {name: cron, enabled: false}
      - {name: web, enabled: true}
  tasks:
    - ansible.builtin.debug:
        msg: "deploy {{ item.name }}"
      loop: "{{ services }}"
      when: item.enabled

    - block:
        - ansible.builtin.command: /bin/false
      rescue:
        - ansible.builtin.debug:
            msg: "deploy failed, rolling back"
      always:
        - ansible.builtin.debug:
            msg: "notify chat"
EOF
ansible-playbook -i hosts site.yml | grep -E '"msg"|skipping'
ansible-playbook -i hosts site.yml | grep '^localhost' | awk '{print $3, $4, $7}'
```

Ожидаемый результат:

```text
skipping: [localhost] => (item={'name': 'cron', 'enabled': False})
    "msg": "deploy api"
    "msg": "deploy web"
    "msg": "deploy failed, rolling back"
    "msg": "notify chat"
ok=4 changed=0 failed=0
```

Разбор: `when` проверяется для каждого элемента `loop` отдельно, и `cron` пропущен. `block/rescue/always` — аналог try/except/finally: команда упала, но `rescue` перехватил ошибку и выполнил откат, а `always` отработал в любом случае. Поэтому в итоге `failed=0`: ошибка обработана, и play продолжается. Без `rescue` хост выбыл бы из дальнейших задач. Другие инструменты: `failed_when` (своё условие ошибки, например по тексту вывода), `ignore_errors` (используй редко) и `any_errors_fatal` (остановить всё при первой ошибке).

</details>

## Handlers

**Handler** — задача, которая выполняется только по уведомлению (`notify`) и только если уведомившая задача вернула `changed`. Классический пример: перезапустить nginx, только если поменялся его конфиг.

```yaml
- name: Веб-серверы
  hosts: web
  become: true

  tasks:
    - name: Конфиг nginx
      ansible.builtin.template:
        src: nginx.conf.j2
        dest: /etc/nginx/nginx.conf
        validate: nginx -t -c %s
      notify: Reload nginx

    - name: Конфиг сайта
      ansible.builtin.template:
        src: site.conf.j2
        dest: /etc/nginx/conf.d/site.conf
      notify: Reload nginx

  handlers:
    - name: Reload nginx
      ansible.builtin.systemd:
        name: nginx
        state: reloaded
```

Как работают handlers:
- уведомлений может быть много, а handler выполнится **один раз**;
- выполняется не сразу, а **в конце секции плея** (после `pre_tasks`, после `roles` + `tasks`, после `post_tasks`);
- handlers выполняются **в порядке, в котором они описаны в `handlers:`**, а не в порядке уведомлений;
- если плей упал до этого момента, handler **не выполнится**: конфиг уже новый, а сервис не перезапущен. Защита — `--force-handlers` или `force_handlers: true` в плее;
- выполнить накопленные handlers прямо сейчас — `- ansible.builtin.meta: flush_handlers`;
- `listen:` позволяет повесить несколько handlers на одно имя-событие: `notify: restart web stack` запустит все handlers с `listen: restart web stack`.

**reload против restart.** `reloaded` перечитывает конфиг без разрыва соединений (nginx, haproxy умеют), `restarted` перезапускает процесс. Для веб-серверов почти всегда нужен reload.

### 🧪 Мини-задание

**Задача.** Сделай плейбук, где задача записи конфига уведомляет handler «restart app». Запусти его трижды: с новым конфигом, без изменений и с изменённым значением. Сосчитай, сколько раз сработал handler.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars:
    port: 8080
  tasks:
    - ansible.builtin.copy:
        dest: "{{ playbook_dir }}/app.conf"
        content: "port={{ port }}\n"
      notify: restart app
  handlers:
    - name: restart app
      ansible.builtin.debug:
        msg: "app restarted"
EOF
for extra in "" "" "-e port=9090"; do
  ansible-playbook -i hosts site.yml $(echo $extra) | grep -c 'RUNNING HANDLER'
done
```

Ожидаемый результат:

```text
1
0
1
```

Разбор: handler выполняется только если уведомившая задача вернула `changed`, и только один раз в конце play, даже если его уведомили десять задач. Поэтому сервис перезапускается только при реальном изменении конфига, а не при каждом прогоне. Если play упадёт до конца, отложенные handlers не выполнятся. Для таких случаев есть `--force-handlers` или `meta: flush_handlers` посреди плейбука. `$(echo $extra)` разбивает строку на аргументы одинаково в bash и zsh.

</details>

## Роли

Когда плейбук разрастается, его разбивают на **роли** — переиспользуемые блоки со стандартной структурой. Роль `nginx` можно подключить в разные проекты, роль `docker` — на все серверы.

```bash
ansible-galaxy role init --init-path roles docker      # создать скелет роли в roles/docker
```
```text
roles/docker/
├── defaults/main.yml     # переменные по умолчанию (слабый приоритет) — «настройки роли»
├── vars/main.yml         # внутренние переменные роли (сильный приоритет), не для переопределения
├── tasks/main.yml        # задачи — точка входа
├── handlers/main.yml
├── templates/            # *.j2 для модуля template
├── files/                # статические файлы для copy
├── meta/main.yml         # зависимости от других ролей, метаданные
└── README.md             # какие переменные принимает роль
```
Модули `template` и `copy` внутри роли ищут файлы в `templates/` и `files/` этой роли — путь писать не нужно.

**Пример роли `docker`:**
```yaml
# roles/docker/defaults/main.yml
docker_version: "5:27.3.1-1~ubuntu.24.04~noble"
docker_users: []
docker_daemon_options:
  log-driver: json-file
  log-opts:
    max-size: "50m"
    max-file: "3"
```
```yaml
# roles/docker/tasks/main.yml
- name: Репозиторий Docker (формат deb822, ключ модуль скачает сам)
  ansible.builtin.deb822_repository:
    name: docker
    types: [deb]
    uris: https://download.docker.com/linux/ubuntu
    suites: ["{{ ansible_facts['distribution_release'] }}"]
    components: [stable]
    signed_by: https://download.docker.com/linux/ubuntu/gpg

- name: Пакеты Docker фиксированной версии
  ansible.builtin.apt:
    name:
      - "docker-ce={{ docker_version }}"
      - "docker-ce-cli={{ docker_version }}"
      - containerd.io
    update_cache: true

- name: daemon.json
  ansible.builtin.copy:
    content: "{{ docker_daemon_options | to_nice_json }}\n"
    dest: /etc/docker/daemon.json
    mode: "0644"
  notify: Restart docker

- name: Пользователи в группе docker
  ansible.builtin.user:
    name: "{{ item }}"
    groups: docker
    append: true             # без append пользователь потеряет остальные группы
  loop: "{{ docker_users }}"
```
```yaml
# roles/docker/handlers/main.yml
- name: Restart docker
  ansible.builtin.systemd:
    name: docker
    state: restarted
```
```yaml
# site.yml
- name: Docker-хосты
  hosts: swarm
  become: true
  roles:
    - role: docker
      vars:
        docker_users: [deploy]
```

**Версия пакета зафиксирована** не случайно: без этого на одном сервере окажется Docker из прошлого месяца, на новом — из сегодняшнего, и кластер станет неоднородным.

**Зависимости ролей** — в `meta/main.yml`:
```yaml
dependencies:
  - role: common          # выполнится перед этой ролью
```
С зависимостями осторожно: одна и та же роль выполнится один раз за плей (если параметры совпадают), и порядок становится неочевидным. Часто проще явно перечислить роли в плейбуке.

### 🧪 Мини-задание

**Задача.** Создай каркас роли `web` через `ansible-galaxy role init`, задай в её `defaults` порт 80 и выведи его в задаче. Подключи роль в плейбуке дважды: без параметров и с переопределённым портом.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

ansible-galaxy role init roles/web > /dev/null
ls roles/web
echo 'web_port: 80' > roles/web/defaults/main.yml
printf -- '- ansible.builtin.debug:\n    msg: "web listens on {{ web_port }}"\n' > roles/web/tasks/main.yml
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  roles:
    - role: web
    - role: web
      vars:
        web_port: 8443
EOF
ansible-playbook -i hosts site.yml | grep '"msg"'
```

Ожидаемый результат:

```text
README.md
defaults
files
handlers
meta
tasks
templates
tests
vars
    "msg": "web listens on 80"
    "msg": "web listens on 8443"
```

Разбор: роль — переиспользуемый пакет со стандартной структурой: `tasks` (что делать), `handlers`, `templates` и `files`, `defaults` (параметры по умолчанию с низшим приоритетом — их и переопределяют пользователи роли), `vars` (внутренние значения с высоким приоритетом), `meta` (зависимости). Одну роль подключили дважды с разными параметрами — так один код настраивает разные сервисы. Хорошая роль делает одно дело, все настраиваемые значения выносит в `defaults` и документирует их в README.

</details>

## import против include

Подключать задачи, роли и плейбуки можно двумя способами, и разницу любят спрашивать.

| | `import_tasks`, `import_role`, `import_playbook` | `include_tasks`, `include_role` |
|---|---|---|
| Когда обрабатывается | при разборе плейбука, до запуска (**статически**) | во время выполнения (**динамически**) |
| `when` и `tags` на этой строке | применяются **к каждой** подключённой задаче | применяются только к самому include |
| Видно в `--list-tasks` | да | нет |
| Можно в `loop` | нет | да |
| Имя файла из переменной | нет (нужно знать при разборе) | да: `include_tasks: "{{ ansible_facts['os_family'] }}.yml"` |

Правило: по умолчанию `import_*` (предсказуемо, работают теги и `--list-tasks`). `include_*` — когда нужен цикл или выбор файла по переменной, например задачи под разные ОС:
```yaml
- name: Задачи для конкретной ОС
  ansible.builtin.include_tasks: "{{ ansible_facts['os_family'] | lower }}.yml"   # debian.yml или redhat.yml
```
Если нужно, чтобы теги дошли до задач внутри `include_*`, используют `apply: { tags: [...] }`.

### 🧪 Мини-задание

**Задача.** Подключи один файл задач через `import_tasks`, а другой через `include_tasks` и посмотри на вывод `ansible-playbook --list-tasks`: какие задачи Ansible знает до запуска плейбука?

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

printf -- '- name: static task A\n  ansible.builtin.debug: {msg: a}\n- name: static task B\n  ansible.builtin.debug: {msg: b}\n' > static.yml
printf -- '- name: dynamic task C\n  ansible.builtin.debug: {msg: c}\n' > dynamic.yml
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  tasks:
    - ansible.builtin.import_tasks: static.yml
    - name: load dynamic tasks
      ansible.builtin.include_tasks: dynamic.yml
EOF
ansible-playbook -i hosts site.yml --list-tasks | grep -E 'task|load'
```

Ожидаемый результат:

```text
      static task A	TAGS: []
      static task B	TAGS: []
      load dynamic tasks	TAGS: []
```

Разбор: `import_*` статический: файл вставляется в плейбук при разборе, поэтому его задачи видны в `--list-tasks`, работают с `--tags` и `--start-at-task`, а `when` на импорте применяется к каждой задаче внутри. `include_*` динамический: файл читается только когда до него доходит выполнение, поэтому в списке видна лишь сама задача include. Зато его можно использовать в цикле или с именем файла из переменной (`include_tasks: "{{ ansible_os_family }}.yml"`). Правило: import — по умолчанию, include — когда нужна динамика.

</details>

## Коллекции и Galaxy

**Коллекция** — пакет с модулями, плагинами и ролями от одного автора: `community.docker`, `amazon.aws`, `kubernetes.core`. Модули из них вызывают по FQCN: `amazon.aws.ec2_instance`.

Зависимости проекта фиксируют в `requirements.yml`, как `requirements.txt` в Python:
```yaml
# collections/requirements.yml
collections:
  - name: community.docker
    version: ">=3.10.0,<4.0.0"
  - name: amazon.aws
    version: "8.2.1"
roles:
  - name: geerlingguy.node_exporter
    version: "2.1.0"
```
```bash
ansible-galaxy install -r collections/requirements.yml
```
Версии фиксируй обязательно: обновление коллекции может поменять поведение модуля, и плейбук, который вчера работал, сегодня упадёт в CI.

### 🧪 Мини-задание

**Задача.** Опиши зависимости проекта в `requirements.yml` с закреплёнными версиями коллекций `community.general` и `amazon.aws`, установи их в папку проекта и проверь, какие версии установились и откуда Ansible их берёт.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > requirements.yml <<'EOF'
collections:
  - name: community.general
    version: "11.4.0"
  - name: amazon.aws
    version: "10.1.2"
EOF
ansible-galaxy collection install -r requirements.yml -p ./collections > /dev/null
ANSIBLE_COLLECTIONS_PATH=./collections ansible-galaxy collection list -p ./collections 2>/dev/null | grep -E '^(community.general|amazon.aws) '
```

Ожидаемый результат:

```text
amazon.aws        10.1.2
community.general 11.4.0
```

Разбор: модули, которых нет в `ansible-core` (AWS, Kubernetes, Docker, большинство утилит), живут в коллекциях и вызываются по полному имени: `amazon.aws.ec2_instance`. Без закреплённых версий сегодня и через месяц устанавливаются разные коллекции, и плейбук может сломаться. Поэтому `requirements.yml` с версиями коммитят в репозиторий, а CI ставит из него зависимости перед запуском. Установка в папку проекта (`-p ./collections` и `collections_path` в `ansible.cfg`) изолирует проекты друг от друга, как venv в Python. Версии здесь — пример, бери актуальные.

</details>

## Секреты: Ansible Vault

**Ansible Vault** шифрует файлы или отдельные значения (AES-256 — стандартный симметричный алгоритм шифрования, ключ получается из пароля vault), чтобы пароли и ключи можно было хранить в git рядом с кодом.

```bash
ansible-vault create inventory/group_vars/db/vault.yml      # создать зашифрованный файл
ansible-vault edit inventory/group_vars/db/vault.yml        # отредактировать
ansible-vault view inventory/group_vars/db/vault.yml
ansible-vault encrypt secrets.yml                           # зашифровать существующий
ansible-vault rekey inventory/group_vars/db/vault.yml       # сменить пароль
ansible-vault encrypt_string 'S3cr3t!' --name 'db_password' # зашифровать одно значение для вставки в YAML
```
Запуск:
```bash
ansible-playbook site.yml --ask-vault-pass
ansible-playbook site.yml --vault-password-file ~/.vault_pass        # файл с паролем, не в git!
ansible-playbook site.yml --vault-id dev@prompt --vault-id prod@~/.vault_prod   # разные пароли для окружений
```

**Приём с двумя файлами**, чтобы секреты можно было найти grep-ом:
```yaml
# group_vars/db/main.yml — открытый, видно, какие переменные есть
db_user: app
db_password: "{{ vault_db_password }}"
```
```yaml
# group_vars/db/vault.yml — зашифрован
vault_db_password: "S3cr3t!"
```

**no_log** — не печатать значения в вывод и логи (иначе пароль окажется в логе CI):
```yaml
- name: Создать пользователя БД
  community.postgresql.postgresql_user:
    name: "{{ db_user }}"
    password: "{{ db_password }}"
  no_log: true
```

**Внешние хранилища.** Когда секреты уже живут в HashiCorp Vault или AWS Secrets Manager, их читают lookup-ом прямо во время выполнения, и в репозитории вообще ничего не хранится:
```yaml
db_password: "{{ lookup('community.hashi_vault.vault_kv2_get', 'apps/shop/db', engine_mount_point='secret').secret.password }}"
db_password_aws: "{{ lookup('amazon.aws.secretsmanager_secret', 'prod/shop/db') }}"
```

**Бинарные секреты** (keytab-файлы Kerberos — файлы с ключами учётной записи сервиса, по которым он сам, без пароля, входит в корпоративный домен; TLS-ключи) тоже можно зашифровать vault-ом целиком и раскладывать через `copy` — он расшифрует файл при копировании:
```yaml
- name: Keytab сервиса
  ansible.builtin.copy:
    src: files/app.keytab.vault       # зашифрован: ansible-vault encrypt files/app.keytab.vault
    dest: /etc/app/app.keytab
    owner: app
    mode: "0400"
  no_log: true
```

### 🧪 Мини-задание

**Задача.** Зашифруй файл с паролем базы через Ansible Vault, посмотри на зашифрованный файл и используй переменную из него в плейбуке, передав пароль от vault через файл.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

echo 'correct-horse-battery' > .vault_pass
echo 'db_password: hunter2' > secrets.yml
ansible-vault encrypt secrets.yml --vault-password-file .vault_pass
head -1 secrets.yml
grep -c hunter2 secrets.yml
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars_files: [secrets.yml]
  tasks:
    - ansible.builtin.debug:
        msg: "password length is {{ db_password | length }}"
EOF
ansible-playbook -i hosts site.yml --vault-password-file .vault_pass | grep '"msg"'
```

Ожидаемый результат:

```text
Encryption successful
$ANSIBLE_VAULT;1.1;AES256
0
    "msg": "password length is 7"
```

Разбор: файл зашифрован целиком (AES256), заголовок показывает формат, а пароля в открытом виде нет — такой файл можно коммитить. Расшифровка происходит в памяти при запуске. В CI пароль от vault передают через переменную окружения или скрипт в `--vault-password-file`, а сам `.vault_pass` добавляют в `.gitignore`. `no_log: true` в задачах не даёт секрету попасть в вывод. Минус vault: в diff не видно, что поменялось. Поэтому всё чаще секреты держат в Secrets Manager или HashiCorp Vault и читают lookup-плагином в момент запуска.

</details>

## Rolling update без простоя

Если обновить все серверы сразу, сервис ляжет. **Rolling update** — обновлять хосты небольшими партиями, выводя каждую из балансировки. В Ansible для этого есть всё нужное.

**serial** — сколько хостов обрабатывать за раз. Плей полностью проходит на одной партии, потом на следующей:
```yaml
serial: 1                 # по одному
serial: "25%"             # по четверти
serial: [1, 5, "30%"]     # сначала один (канарейка), потом 5, потом по 30%
```

Полный пример: обновление приложения за HAProxy:
```yaml
- name: Rolling update приложения
  hosts: app
  become: true
  serial: [1, "30%"]
  max_fail_percentage: 0          # любой сбой — остановить, не трогать следующие партии

  pre_tasks:
    - name: Вывести хост из балансировки
      community.general.haproxy:
        state: disabled
        host: "{{ inventory_hostname }}"
        backend: app
        socket: /run/haproxy/admin.sock
        wait: true                # дождаться, пока завершатся активные соединения
      delegate_to: "{{ item }}"
      loop: "{{ groups['lb'] }}"

  roles:
    - role: app                   # выкатка новой версии, перезапуск через handler

  post_tasks:
    - name: Применить handlers до проверки
      ansible.builtin.meta: flush_handlers

    - name: Дождаться здоровья
      ansible.builtin.uri:
        url: "http://{{ ansible_host }}:{{ app_port }}/health"
      register: health
      until: health.status == 200
      retries: 30
      delay: 5
      delegate_to: localhost
      become: false

    - name: Вернуть хост в балансировку
      community.general.haproxy:
        state: enabled
        host: "{{ inventory_hostname }}"
        backend: app
        socket: /run/haproxy/admin.sock
      delegate_to: "{{ item }}"
      loop: "{{ groups['lb'] }}"
```

Ключевые инструменты из примера:
- **delegate_to** — выполнить задачу на другом хосте, но в контексте текущего: команда идёт на балансировщик, а `inventory_hostname` — это обновляемый сервер;
- **run_once: true** — выполнить задачу один раз на весь батч (например, миграцию БД перед выкаткой);
- **pre_tasks / post_tasks** — задачи до и после ролей; handlers сбрасываются между секциями;
- **max_fail_percentage: 0** — если канарейка упала, остальные партии не трогаем.

**Тот же приём для Docker Swarm** — перед обслуживанием ноды (обновление Docker, ядра) вывести её из-под нагрузки:
```yaml
- name: Обслуживание нод Swarm по одной
  hosts: swarm_workers
  become: true
  serial: 1

  tasks:
    - name: Drain — сервисы уедут на другие ноды
      community.docker.docker_node:
        hostname: "{{ ansible_hostname }}"
        availability: drain
      delegate_to: "{{ groups['swarm_managers'][0] }}"

    - name: Обновить пакеты
      ansible.builtin.apt:
        upgrade: dist
        update_cache: true

    - name: Нужна ли перезагрузка (Ubuntu создаёт этот файл после обновления ядра и библиотек)
      ansible.builtin.stat:
        path: /var/run/reboot-required
      register: reboot_flag

    - name: Перезагрузить, если требуется
      ansible.builtin.reboot:
        reboot_timeout: 600
      when: reboot_flag.stat.exists

    - name: Вернуть ноду в работу
      community.docker.docker_node:
        hostname: "{{ ansible_hostname }}"
        availability: active
      delegate_to: "{{ groups['swarm_managers'][0] }}"
```
Для Kubernetes аналог — `kubectl drain` / `uncordon` (модуль `kubernetes.core.k8s_drain`), обычно с `serial: 1` и проверкой, что все поды снова Ready.

### 🧪 Мини-задание

**Задача.** Сымитируй выкатку на пять серверов партиями по два через `serial: 2`. Посмотри, в каком порядке и какими группами хосты проходят задачу.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
printf '[web]\n' > hosts
for i in 1 2 3 4 5; do echo "web$i ansible_connection=local ansible_python_interpreter=auto_silent" >> hosts; done
cat > site.yml <<'EOF'
- hosts: web
  gather_facts: false
  serial: 2
  max_fail_percentage: 0
  tasks:
    - name: deploy
      ansible.builtin.debug:
        msg: "deploying {{ inventory_hostname }}"
EOF
ansible-playbook -i hosts site.yml -f 1 | grep -E '^TASK \[deploy|^ok: \[' | sed -e 's/ \*.*//' -e 's/ =>.*//'
```

Ожидаемый результат:

```text
TASK [deploy]
ok: [web1]
ok: [web2]
TASK [deploy]
ok: [web3]
ok: [web4]
TASK [deploy]
ok: [web5]
```

Разбор: `serial: 2` прогоняет весь play на двух хостах, и только потом переходит к следующим — три партии. В настоящей выкатке задачи партии выглядят так: вывести хосты из балансировщика (HAProxy через сокет или drain в Swarm), обновить, проверить healthcheck и вернуть в балансировку (`delegate_to` на хост балансировщика). Остальные серверы всё это время обслуживают трафик. `max_fail_percentage: 0` останавливает выкатку при первой же ошибке, чтобы сломанная версия не разошлась по всем серверам. `serial` можно задать и в процентах или списком растущих партий: `[1, 5, "25%"]`.

</details>

## Скорость: как ускорить Ansible на сотнях хостов

Ansible по умолчанию неторопливый: 5 хостов параллельно, сбор фактов в каждом плее, несколько SSH-операций на каждую задачу. На 50+ кластерах это минуты и десятки минут. Что помогает:

1. **forks** — поднять параллельность: `forks = 30` в `ansible.cfg` или `-f 30`. Ограничение — CPU и память control node.
2. **pipelining = True** — модуль передаётся через уже открытое SSH-соединение без отдельного копирования файла. Обычно ускоряет в разы. Требует, чтобы в sudoers не было `requiretty` (настройка sudo «работать только из настоящего терминала», которая ломает неинтерактивный запуск).
3. **ControlPersist** — SSH-соединение переиспользуется между задачами, не нужно каждый раз заново устанавливать сессию.
4. **Факты:** `gather_facts: false`, если не нужны; `gather_subset: [network]`, если нужна только часть; кэш фактов между запусками:
   ```ini
   [defaults]
   gathering = smart
   fact_caching = jsonfile
   fact_caching_connection = /tmp/ansible_facts
   fact_caching_timeout = 3600
   ```
5. **Пакеты списком**, а не в `loop` — одна транзакция apt вместо десяти.
6. **strategy: free** — каждый хост идёт по задачам в своём темпе, быстрые не ждут медленных. Не подходит, когда задачи на разных хостах зависят друг от друга.
7. **async** — долгая задача запускается в фоне, Ansible не держит соединение:
   ```yaml
   - name: Долгая сборка индекса
     ansible.builtin.command: /opt/app/bin/reindex
     async: 3600        # максимум час
     poll: 0            # не ждать, сразу дальше
     register: reindex_job

   - name: Дождаться завершения
     ansible.builtin.async_status:
       jid: "{{ reindex_job.ansible_job_id }}"
     register: job
     until: job.finished
     retries: 120
     delay: 30
   ```
8. **throttle** на задаче — ограничить параллельность только для неё (например, не больше 2 хостов одновременно качают большой файл с внутреннего репозитория).

Чтобы найти медленные задачи, включи callback с таймингами: `callbacks_enabled = ansible.posix.profile_tasks` в `[defaults]` — в конце покажет топ задач по времени.

### 🧪 Мини-задание

**Задача.** Пять «хостов» выполняют задачу `sleep 1`. Замерь время прогона с `-f 1` и с `-f 5` и объясни разницу.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
for i in 1 2 3 4 5; do echo "h$i ansible_connection=local ansible_python_interpreter=auto_silent" >> hosts; done
printf -- '- hosts: all\n  gather_facts: false\n  tasks:\n    - ansible.builtin.command: sleep 1\n' > site.yml
for f in 1 5; do
  start=$(date +%s); ansible-playbook -i hosts site.yml -f $f > /dev/null; echo "forks=$f: $(( $(date +%s) - start ))s"
done
```

Ожидаемый результат:

```text
forks=1: 7s
forks=5: 3s
```

Разбор: `forks` — сколько хостов Ansible обрабатывает параллельно (по умолчанию всего 5). С одним потоком хосты идут по очереди, с пятью — одновременно; оставшиеся секунды — это запуск Ansible и Python. На сотнях хостов главные ускорители: больше forks (насколько тянет управляющая машина), `pipelining = True` и SSH ControlPersist (меньше SSH-соединений на задачу), `gather_facts: false` или кэш фактов, `strategy: free` (хосты не ждут друг друга между задачами) и `async` для долгих операций. Числа у тебя будут свои.

</details>

## Тестирование и качество

Плейбук — это код, и проверять его нужно так же.

**Уровни проверки, от дешёвого к дорогому:**
1. **yamllint** — синтаксис и стиль YAML.
2. **ansible-playbook --syntax-check** — Ansible понимает плейбук.
3. **ansible-lint** — лучшие практики: FQCN, имена задач, `command` вместо модуля, отсутствие `changed_when`, права файлов без кавычек:
   ```bash
   pip install ansible-lint
   ansible-lint site.yml roles/
   ```
4. **--check --diff** на реальных хостах — что изменится. Ограничение: модули `command`/`shell` в check mode пропускаются (Ansible не может предсказать их результат), поэтому задачи, зависящие от их `register`, могут вести себя иначе. Задаче, которая только читает данные, можно разрешить выполняться и в check mode: `check_mode: false`.
5. **Molecule** — тесты ролей: поднимает контейнер или ВМ, применяет роль, проверяет идемпотентность (второй прогон должен дать `changed=0`), запускает проверки, удаляет стенд:
   ```bash
   pip install molecule molecule-plugins[docker]
   cd roles/docker
   molecule init scenario -d docker
   molecule test         # create → converge → idempotence → verify → destroy
   ```
   Шаг `idempotence` в Molecule — автоматическая проверка того самого «второй запуск ничего не меняет».

### 🧪 Мини-задание

**Задача.** Проверь плейбук без изменения системы: сначала синтаксис, потом прогон в режиме `--check --diff`, который покажет, что поменялось бы в конфиге. Убедись, что сам файл остался прежним.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

echo "port=80" > app.conf
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  tasks:
    - ansible.builtin.lineinfile:
        path: "{{ playbook_dir }}/app.conf"
        line: timeout=30
EOF
ansible-playbook -i hosts site.yml --syntax-check
ansible-playbook -i hosts site.yml --check --diff | grep -E '^[-+@ ]' | grep -v '^ *$' | sed 's|: /.*/app.conf|: app.conf|'
cat app.conf
```

Ожидаемый результат:

```text
playbook: site.yml
--- before: app.conf (content)
+++ after: app.conf (content)
@@ -1 +1,2 @@
 port=80
+timeout=30
port=80
```

Разбор: `--syntax-check` ловит ошибки YAML и структуры плейбука за секунду — его ставят первым шагом в CI. `--check` выполняет «сухой прогон»: модули сообщают, что *изменили бы*, но ничего не меняют, а `--diff` показывает сами изменения. Это стандартный шаг ревью перед применением на прод: виден будущий diff конфигов на всех хостах. Ограничение: `command`/`shell` в check-режиме пропускаются, а задачи, зависящие от их результата, могут вести себя иначе. Следующие уровни: `ansible-lint` (стиль и ошибки) и Molecule (прогон роли в контейнере с проверками).

</details>

## Ansible в CI/CD

Запускать плейбуки с ноутбука в команде плохо: у всех разные версии Ansible и коллекций, нет истории, кто и что выкатил. Поэтому их запускают из CI.

Пример для GitLab CI:
```yaml
# .gitlab-ci.yml
stages: [lint, deploy]

variables:
  ANSIBLE_FORCE_COLOR: "true"
  ANSIBLE_HOST_KEY_CHECKING: "true"

.ansible:
  image: registry.example.com/tools/ansible:2.17      # свой образ: зафиксированные ansible-core, коллекции, lint
  before_script:
    - eval "$(ssh-agent -s)"
    - echo "$DEPLOY_SSH_KEY" | tr -d '\r' | ssh-add -          # SSH-ключ из защищённой переменной CI
    - mkdir -p ~/.ssh && echo "$SSH_KNOWN_HOSTS" > ~/.ssh/known_hosts
    - echo "$ANSIBLE_VAULT_PASSWORD" > .vault_pass && chmod 600 .vault_pass
    - ansible-galaxy install -r collections/requirements.yml

lint:
  extends: .ansible
  stage: lint
  script:
    - yamllint .
    - ansible-lint

deploy_staging:
  extends: .ansible
  stage: deploy
  script:
    - ansible-playbook -i inventory/staging.ini site.yml --vault-password-file .vault_pass --diff
  rules:
    - if: $CI_COMMIT_BRANCH == "main"

deploy_prod:
  extends: .ansible
  stage: deploy
  script:
    - ansible-playbook -i inventory/prod.ini site.yml --vault-password-file .vault_pass --diff -e "app_version=$CI_COMMIT_TAG"
  rules:
    - if: $CI_COMMIT_TAG
      when: manual                     # прод — по кнопке
  environment: production
```
Обрати внимание: `known_hosts` передаётся явно, а не отключается проверка ключей хостов, — иначе CI подключится к любому серверу, который ответит по этому адресу.

**AWX / Ansible Automation Platform** — веб-интерфейс и API над Ansible: запуск плейбуков по кнопке и по расписанию, права доступа, хранение кредов, история запусков. AWX — бесплатная upstream-версия (открытый проект, из которого собирают коммерческий продукт), AAP — коммерческая от Red Hat.

**ansible-pull** переворачивает модель: хост сам клонирует репозиторий и применяет плейбук к себе (например, по cron). Пригодится, когда control node не может достучаться до хостов (они за NAT), или для автонастройки при первой загрузке.

**Execution Environments** — контейнерный образ с ansible-core, коллекциями и Python-зависимостями. Запускаются через `ansible-navigator`. Решает проблему «у меня работает, а в CI нет».

### 🧪 Мини-задание

**Задача.** Запусти плейбук с JSON-выводом вместо текстового и достань через `jq` итоговую статистику по хосту — так CI может решать, что делать дальше. Затем посмотри на код выхода, когда задача падает.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

printf -- '- hosts: all\n  gather_facts: false\n  tasks:\n    - ansible.builtin.copy: {dest: "{{ playbook_dir }}/x", content: "x"}\n' > ok.yml
printf -- '- hosts: all\n  gather_facts: false\n  tasks:\n    - ansible.builtin.command: /bin/false\n' > fail.yml
ANSIBLE_STDOUT_CALLBACK=json ansible-playbook -i hosts ok.yml | jq -c '.stats.localhost | {ok, changed, failures}'
ansible-playbook -i hosts fail.yml > /dev/null; echo "exit=$?"
```

Ожидаемый результат:

```text
{"ok":1,"changed":1,"failures":0}
exit=2
```

Разбор: CI решает по коду выхода: 0 — успех, 2 — на каком-то хосте задача упала, 4 — хосты недоступны. Поэтому job с упавшим плейбуком краснеет сам. JSON-вывод удобен, когда нужно больше, чем «упало или нет»: сколько изменений, на каких хостах, выложить отчёт артефактом или в чат. Типичный пайплайн: lint и syntax-check → `--check --diff` с выводом на ревью → ручное подтверждение → применение. Креды (SSH-ключ, пароль от vault, AWS-роль через OIDC) приходят из защищённых переменных CI, а не из репозитория.

</details>

## Ansible вместе с Terraform

Типичная связка в облаке:
1. Terraform создаёт VPC, EC2, security groups и ставит теги (`Role=web`, `Project=shop`).
2. Динамический инвентарь `amazon.aws.aws_ec2` находит инстансы по тегам.
3. Ansible настраивает их.

Чего избегать: вызывать Ansible из Terraform через `local-exec` — Terraform не видит, что сделал Ansible, ошибки и повторы становятся непредсказуемыми. Лучше два отдельных шага в пайплайне.

Где граница: если сервис можно настроить через API провайдера (параметры RDS, настройки S3), это делает Terraform. Всё, что внутри ОС, — Ansible. А для Kubernetes-кластеров конфигурацию внутри кластера всё чаще ведут через GitOps (ArgoCD, Flux), а Ansible остаётся для подготовки нод и on-prem инфраструктуры.

**Kubespray** — набор плейбуков и ролей Ansible для установки production-кластера Kubernetes на свои серверы (etcd, control plane, CNI, обновления версий). Полезно знать, если речь заходит об on-prem Kubernetes.

### 🧪 Мини-задание

**Задача.** Сымитируй связку: Terraform отдаёт IP созданных серверов через output, а скрипт превращает его в инвентарь Ansible. Без облака используй `terraform_data` со списком адресов и проверь получившийся инвентарь.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > main.tf <<'EOF'
resource "terraform_data" "web" {
  count = 2
  input = "10.0.1.${count.index + 11}"
}

output "web_ips" {
  value = terraform_data.web[*].output
}
EOF
terraform init -no-color > /dev/null && terraform apply -auto-approve -no-color > /dev/null
{ echo "[web]"; terraform output -json web_ips | jq -r 'to_entries[] | "web\(.key + 1) ansible_host=\(.value)"'; } > hosts
cat hosts
ansible-inventory -i hosts --host web2
```

Ожидаемый результат:

```text
[web]
web1 ansible_host=10.0.1.11
web2 ansible_host=10.0.1.12
{
    "ansible_host": "10.0.1.12"
}
```

Разбор: разделение ролей: Terraform создаёт инфраструктуру и хранит её состояние, Ansible настраивает то, что внутри машин. Мост между ними — данные о созданных ресурсах. Генерация инвентаря из `terraform output -json` — самый простой вариант. В AWS чаще используют динамический инвентарь `amazon.aws.aws_ec2`: Terraform ставит теги, а Ansible сам находит хосты по тегам в момент запуска, и ничего генерировать не надо. Запускать Ansible из Terraform через `local-exec` можно, но так теряется идемпотентность и прозрачность обоих инструментов. Для образов машин связка другая: Packer + Ansible-провиженер.

</details>

## Кейсы из опыта: как рассказывать

На собеседовании про Ansible почти всегда просят рассказать реальный опыт. Ниже — каркасы под темы из резюме. Подставь свои цифры и детали, придумывать не нужно: интервьюер обычно копает в глубину.

**50+ кластеров Docker Swarm через Ansible.** О чём рассказать:
- как был устроен инвентарь: группы по кластерам и ролям (managers, workers, lb), где лежали переменные;
- какие роли были: базовая настройка ОС, Docker, HAProxy, Pacemaker/Corosync (кластерный менеджер: переносит плавающий IP и сервисы на живой узел при отказе), GlusterFS (распределённая файловая система: общий том на нескольких серверах), мониторинг;
- как обновляли Docker или ОС без простоя: `serial`, drain ноды, проверка кворума менеджеров;
- как ускоряли прогон на десятках кластеров (forks, pipelining, ограничение `--limit` по кластерам);
- что ломалось и как починили (дрейф конфигураций, ручные правки, разные версии пакетов).

**Kubernetes on-prem через Ansible.** Что использовали (Kubespray, kubeadm в своих ролях), как обновляли версии, как готовили ноды (swap, модули ядра, sysctl, container runtime).

**Секреты: Vault и keytab-файлы Kerberos.** Как секреты попадали на хосты: Ansible Vault, lookup из HashiCorp Vault, `no_log`, права `0400`, ротация.

Формат ответа (STAR): **ситуация** (что было и в чём проблема) → **задача** → **что сделал лично ты** → **результат в цифрах** (время выкатки было/стало, число ручных операций, инциденты).

## Типичные ошибки

- **shell/command там, где есть модуль** — теряется идемпотентность, задача «changed» при каждом запуске.
- **Нет `changed_when` у команд только для чтения** — отчёт полон ложных `changed`, настоящие изменения в нём не видны.
- **`mode: 644` без кавычек** — YAML прочитает как десятичное число 644, а не восьмеричное, права получатся странными. Пиши `mode: "0644"`.
- **Забытый `append: true` в модуле user** — пользователь лишится всех остальных групп.
- **Переменные роли в `vars/` вместо `defaults/`** — их нельзя переопределить из `group_vars`.
- **Jinja в `when` в двойных фигурных скобках** — `when: "{{ x }}"` не нужно, пиши `when: x`.
- **Переменная, начинающаяся с `{{`, без кавычек** — `name: {{ app }}` YAML воспримет как словарь. Нужно `name: "{{ app }}"`.
- **Handler не выполнился, потому что плей упал раньше** — конфиг новый, сервис старый. `force_handlers` или `flush_handlers` в нужных местах.
- **`host_key_checking = False` в проде** — удобно, но открывает подмену сервера.
- **Незафиксированные версии пакетов и коллекций** — серверы и прогоны CI со временем расходятся.
- **Секреты в выводе** — пароли попадают в лог CI без `no_log: true`.
- **Ручные правки на серверах «пока никто не видит»** — следующий прогон Ansible их перезапишет, или, хуже, они не описаны в коде и потеряются при пересоздании сервера.

### 🧪 Мини-задание

**Задача.** Попробуй посчитать строки через пайп модулем `command`, а потом тем же текстом через модуль `shell`. Сравни результаты и объясни, почему так.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

ansible all -i hosts -m command -a 'echo a | wc -l' | tail -1
ansible all -i hosts -m shell -a 'echo a | wc -l' | tail -1 | tr -d ' '
```

Ожидаемый результат:

```text
a | wc -l
1
```

Разбор: модуль `command` запускает программу напрямую, без shell, поэтому `|`, `>`, `&&`, `*` и переменные окружения не работают: `echo` просто получил аргументы `a`, `|`, `wc`, `-l` и напечатал их. Пайп выполнил только `shell`. Это безопаснее (нет инъекций через подстановку) и поэтому `command` — выбор по умолчанию. Другие частые ошибки: `shell` там, где есть модуль (неидемпотентно), секреты без `no_log`, переменная в пяти местах с разным приоритетом, отсутствие `--check` перед продом и handlers, которые не сработали из-за упавшего play.

</details>

## Вопросы с собеседований

**Что такое Ansible и чем он отличается от Puppet и Chef?**
Инструмент управления конфигурацией без агентов: подключается к хостам по SSH, копирует и запускает модули на Python, работает по push-модели. Puppet и Chef требуют агента на каждом сервере и центральный сервер, агенты сами периодически тянут конфигурацию (pull). У Ansible ниже порог входа и проще старт, у Puppet лучше встроенный контроль дрейфа за счёт постоянной работы агента.

**Что такое идемпотентность и как её обеспечить?**
Повторный запуск не меняет систему, если она уже в нужном состоянии. Обеспечивают модули: они проверяют текущее состояние и меняют только разницу. Для `command`/`shell` — `creates`/`removes`, `changed_when`, проверка через `register` + `when`, а лучше замена на подходящий модуль. Проверка — второй прогон должен дать `changed=0`, в Molecule это отдельный шаг.

**Чем command отличается от shell?**
`command` запускает программу напрямую без shell: нет пайпов, перенаправлений, переменных окружения в строке, зато безопаснее и нет проблем с экранированием. `shell` выполняет строку через `/bin/sh` со всеми возможностями shell. По умолчанию используют `command`.

**Что нужно на управляемом хосте, чтобы Ansible работал?**
SSH-доступ (для Linux) и Python. Без Python работают только модули `raw` и `script` — через `raw` обычно и ставят Python на «голые» хосты. Для Windows — WinRM или SSH и PowerShell.

**Как устроены приоритеты переменных? Что победит: group_vars или host_vars? defaults роли или group_vars?**
От слабого к сильному: defaults роли → инвентарь и group_vars (all слабее конкретной группы) → host_vars → факты → переменные плея → vars роли → переменные блока и задачи → set_fact и register → `-e`. host_vars сильнее group_vars. group_vars сильнее defaults роли, поэтому настройки роли кладут в defaults. `-e` перебивает всё.

**Что такое handlers и когда они выполняются?**
Задачи, которые запускаются по `notify` и только если уведомившая задача дала `changed`. Выполняются один раз, в конце секции плея, в порядке описания в `handlers`. Если плей упал раньше — не выполнятся; защита `force_handlers`. Выполнить немедленно — `meta: flush_handlers`.

**Чем import_tasks отличается от include_tasks?**
`import_*` обрабатывается статически при разборе: `when` и `tags` применяются к каждой задаче внутри, задачи видны в `--list-tasks`, но нельзя использовать `loop` и имя файла из переменной времени выполнения. `include_*` динамический: работает в цикле и с вычисляемым именем, но теги и условия относятся только к самому include.

**Как хранить секреты в Ansible?**
Ansible Vault для файлов и отдельных значений в репозитории, пароль от vault — в защищённой переменной CI или файле вне git. Приём `db_password: "{{ vault_db_password }}"` для читаемости. `no_log: true` на задачах с секретами. Для зрелых команд — lookup из HashiCorp Vault или AWS Secrets Manager, тогда секретов в репозитории нет вовсе.

**Как сделать rolling update без простоя?**
`serial` для обработки хостов партиями (часто `[1, "30%"]`: сначала канарейка), `max_fail_percentage: 0` чтобы остановиться при сбое. В `pre_tasks` вывести хост из балансировщика через `delegate_to` на LB, обновить, `flush_handlers`, дождаться health-check через `until`, в `post_tasks` вернуть в балансировку.

**Что делают delegate_to и run_once?**
`delegate_to` выполняет задачу на другом хосте, но в контексте текущего (переменные текущего хоста доступны): например, вывести сервер из HAProxy, выполнив команду на балансировщике. `run_once` выполняет задачу один раз на весь батч хостов — для миграции БД, создания токена, запроса к API.

**Плейбук на 200 хостов идёт 40 минут. Как ускорить?**
Найти медленные задачи (`profile_tasks`). Затем: поднять `forks`, включить `pipelining` и `ControlPersist`, отключить или кэшировать сбор фактов, ставить пакеты списком, `strategy: free` где задачи независимы, `async` для долгих операций, не выполнять лишнее (`creates`, условия). Если часть хостов меняется редко — ограничивать прогон через `--limit` и теги.

**Как работает check mode и какие у него ограничения?**
`--check` запускает модули в режиме «что бы изменилось» без изменений, `--diff` показывает разницу файлов. Модули `command` и `shell` в check mode пропускаются, поэтому зависящие от их результатов задачи могут вести себя иначе, чем в реальном прогоне. Модули без поддержки check mode тоже пропускаются. Задачу только для чтения можно выполнить и в check mode через `check_mode: false`.

**Как тестировать роли?**
yamllint и ansible-lint в CI на каждый коммит, `--syntax-check`, Molecule: поднимает контейнер или ВМ, применяет роль, повторно применяет и проверяет, что изменений нет (идемпотентность), выполняет проверки (verify), удаляет стенд.

**Что такое динамический инвентарь?**
Плагин, который получает список хостов из внешнего источника при каждом запуске: из AWS (`amazon.aws.aws_ec2`), GCP, Azure, VMware, NetBox (система учёта инфраструктуры: серверы, стойки, IP-адреса). Группы строятся по тегам и атрибутам (`keyed_groups`). Нужен в облаке, где серверы постоянно создаются и удаляются.

**Как Ansible и Terraform работают вместе?**
Terraform создаёт инфраструктуру и хранит state, Ansible настраивает ОС и приложения на созданных машинах. Связь через динамический инвентарь по тегам, которые ставит Terraform, или через сгенерированный Terraform файл инвентаря. Запускаются отдельными шагами пайплайна, а не через `local-exec`.

**Что такое факты и как с ними работать?**
Данные о хосте, которые собирает модуль `setup` в начале плея: ОС, версия, IP, память, диски. Доступны через `ansible_facts`. Отключаются `gather_facts: false`, частично собираются через `gather_subset`, кэшируются через `fact_caching`. Свои факты можно положить в `/etc/ansible/facts.d/*.fact` на хосте.

**В чём разница между defaults и vars в роли?**
`defaults/main.yml` — самый слабый приоритет: значения по умолчанию, которые пользователь роли переопределяет через group_vars или параметры роли. `vars/main.yml` — сильный приоритет, для внутренних значений роли, которые менять не предполагается (например, имена пакетов для разных ОС).

**Задача упала на одном из 10 хостов. Что будет с остальными?**
По умолчанию упавший хост выбывает из оставшихся задач плея, остальные 9 продолжают. В конце хост будет в `failed`. Изменить поведение: `any_errors_fatal: true` — остановить всех, `max_fail_percentage` — допустимая доля сбоев, `ignore_errors` на конкретной задаче, `block`/`rescue` для обработки и отката.

**Как сделать, чтобы битый конфиг не попал на сервер?**
Параметр `validate` у `template` и `copy`: файл проверяется командой во временном месте и только при успехе заменяет рабочий (`nginx -t -c %s`, `haproxy -c -f %s`, `visudo -cf %s`). Плюс reload вместо restart в handler и `--check --diff` перед выкаткой.

**Что такое ansible-pull и когда он нужен?**
Обратная модель: хост сам клонирует репозиторий с плейбуками и применяет их к себе, обычно по cron или при первой загрузке. Нужен, когда control node не может подключиться к хостам (NAT, тысячи эфемерных машин) или для самонастройки образов.

**Чем отличаются пакеты ansible и ansible-core?**
`ansible-core` — движок, CLI и встроенные модули `ansible.builtin`. `ansible` — ansible-core плюс большой набор коллекций сообщества. В проде часто ставят `ansible-core` и явно нужные коллекции из `requirements.yml` с фиксированными версиями, чтобы контролировать, что именно используется.
