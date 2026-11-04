# Puppet

Подробный разбор Puppet: как он устроен (агент, сервер, каталог), чем отличается от Ansible, Chef, Salt и Terraform, язык манифестов, ресурсы и зависимости между ними, модули, Hiera, паттерн roles и profiles, секреты, exported resources, тестирование, CI и эксплуатация на сотнях серверов. Каждая тема: что это, зачем, как работает, пример с объяснением и типичные ошибки. В конце — вопросы с собеседований с ответами.

Как читать: начни со стенда из первого раздела и запускай примеры через `puppet apply`. Вывод Puppet показан в комментариях после `# →`. Если ты уже знаешь Ansible, держи в голове сравнение из раздела «Puppet среди других инструментов» — на собеседовании почти всегда спрашивают именно разницу.

## Как учить без курса

Puppet можно учить без сервера: команда `puppet apply` берёт файл с описанием и применяет его к той машине, где запущена. Сервер понадобится позже, когда дойдёшь до каталогов, сертификатов и PuppetDB.

**Стенд вариант 1 — Docker-контейнер (быстро, но без systemd):**
```bash
docker run -it --name puppet-lab ubuntu:24.04 bash
apt update && apt install -y puppet-agent      # пакет из репозитория Ubuntu: агент Puppet 8 и Facter
puppet --version
# → 8.4.0
```
Ограничение: в контейнере нет systemd, поэтому ресурс `service` работать не будет — для пакетов, файлов и пользователей хватит.

**Стенд вариант 2 — виртуальные машины (как в жизни):** `brew install multipass`, затем `multipass launch -n puppet-server -m 4G` и `multipass launch -n node1`. На сервер ставится **puppetserver** (сервер на JVM — виртуальной машине Java, поэтому ему нужно минимум 2–4 ГБ памяти), на ноды — агент. Пакеты берут из официального репозитория Puppet или из **OpenVox** (сообщество-форк Puppet, см. раздел «Версии и OpenVox»). Команды установки меняются между версиями — сверяйся с инструкцией на сайте выбранного проекта.

Разница путей: в пакетах из репозитория Ubuntu/Debian конфигурация лежит в `/etc/puppet`, в официальных пакетах Puppet и OpenVox — в `/etc/puppetlabs/puppet`, а код — в `/etc/puppetlabs/code/environments/<окружение>`. Ниже используются пути официальных пакетов.

План: `puppet resource` → первый манифест → класс → зависимости → шаблон → модуль → Hiera → roles и profiles → сервер и агент → PuppetDB и exported resources. После каждого шага — повторный запуск и проверка, что изменений нет.

## Что такое Puppet и как он работает

**Puppet** — инструмент управления конфигурацией: описываешь, в каком состоянии должны быть серверы (какие пакеты стоят, какие файлы с каким содержимым лежат, какие сервисы запущены), а Puppet приводит их к этому состоянию и **постоянно держит** в нём.

Главные свойства:
- **Декларативный язык** — ты пишешь *что* должно быть, а не *как* это сделать: «пакет nginx установлен», а не «выполни apt-get install».
- **Pull-модель с агентом** — на каждом сервере работает агент, который сам раз в 30 минут (параметр `runinterval`) идёт на сервер за конфигурацией. Ansible наоборот: ты сам запускаешь изменения с управляющей машины (push).
- **Постоянное исправление дрейфа** (drift — расхождение реального состояния с описанным, например кто-то руками поправил конфиг). Агент каждые 30 минут вернёт файл к описанному виду. В этом главное отличие от Ansible, который меняет что-то только когда его запустили.
- **Модель ресурсов** — всё описывается ресурсами (`package`, `file`, `service`, `user`), а Puppet сам строит граф их зависимостей.

**Компоненты:**
- **Puppet Server** (раньше назывался puppet master) — компилирует конфигурации для нод, раздаёт файлы, работает центром сертификации. Слушает порт **8140** (HTTPS).
- **Puppet Agent** — служба на каждой управляемой машине (ноде).
- **Facter** — утилита, которая собирает **факты** о машине: ОС, IP, память, число CPU. Команда `facter os.family` → `Debian`.
- **PuppetDB** — база (на PostgreSQL), где сервер хранит факты, последние каталоги и отчёты всех нод. Нужна для exported resources и запросов вида «на каких серверах стоит openssl 3.0.2».
- **CA (Certificate Authority, центр сертификации)** — встроен в Puppet Server. Агент и сервер общаются по **mTLS** (взаимный TLS: не только клиент проверяет сервер, но и сервер проверяет сертификат клиента), поэтому каждая нода должна получить подписанный сертификат.

**Что происходит при одном запуске агента** (Puppet run):
1. Агент собирает факты через Facter.
2. Отправляет факты на сервер и просит **каталог** для своего имени (certname — имя ноды из её сертификата).
3. Сервер определяет, какие классы нужны этой ноде (классификация), берёт данные из Hiera и **компилирует каталог** — итоговый список ресурсов с конкретными значениями и графом зависимостей. Код Puppet выполняется на сервере, а не на ноде.
4. Агент получает каталог и **применяет** его: для каждого ресурса сравнивает текущее состояние с нужным и меняет только разницу.
5. Агент отправляет **отчёт** (что изменилось, что упало, сколько времени заняло) — на сервер и в PuppetDB.

Ситуация из жизни: коллега на проде руками поменял `worker_connections` в `/etc/nginx/nginx.conf`. Через 30 минут или меньше агент заметит отличие от каталога, вернёт файл, перезапустит nginx и запишет в отчёт `changed`. Если правка была нужна — её надо вносить в код Puppet, а не на сервер.

## Puppet среди других инструментов

«IaC» (Infrastructure as Code — инфраструктура описывается кодом в git, а не настраивается руками) — общее название. Внутри две разные задачи: **создать инфраструктуру** (сети, виртуалки, балансировщики) и **настроить то, что внутри ОС**. Puppet, Ansible, Chef и Salt решают вторую, Terraform — первую.

| | Puppet | Ansible | Chef | Salt | Terraform |
|---|---|---|---|---|---|
| Задача | настройка ОС | настройка ОС, деплой, оркестрация | настройка ОС | настройка ОС, удалённое выполнение | создание облачной инфраструктуры |
| Модель | pull: агент сам ходит к серверу | push: запускаешь с control node | pull: агент chef-client | push и pull: master + minion | push: запускаешь plan/apply |
| Агент на сервере | да | нет (SSH + Python) | да | да (minion), есть режим без агента | нет |
| Язык | свой декларативный DSL | YAML, задачи по порядку | Ruby DSL | YAML + Jinja | HCL |
| Порядок выполнения | граф зависимостей | сверху вниз | сверху вниз в рецепте | граф с `require` | граф зависимостей |
| Состояние | не хранит, сравнивает с реальностью при каждом запуске | не хранит | не хранит | не хранит | state-файл |
| Исправление дрейфа | автоматически, каждые 30 мин | только при запуске | автоматически по расписанию | по расписанию или событиям | только при `apply` |

Когда что выбирают:
- **Puppet** — тысячи долгоживущих серверов (on-prem, банки, телеком, госсектор), где важны постоянное соответствие стандарту, аудит и отчёты о каждом изменении.
- **Ansible** — быстрый старт, разовые операции, деплой приложений, оркестрация последовательных шагов (вывести из балансировки → обновить → вернуть), облачные эфемерные машины.
- **Terraform** — создание облачных ресурсов. Часто связка: Terraform создаёт VM → Puppet-агент из user data регистрируется на сервере и настраивает её.
- В Kubernetes-мире Puppet почти не нужен внутри кластера: конфигурацию приложений ведут через GitOps (ArgoCD, Flux). Puppet остаётся для нод, баз данных, on-prem железа.

Ответ на собеседовании «чем Puppet лучше Ansible» лучше строить как компромисс: Puppet сильнее в постоянном контроле состояния и масштабе (агенты сами приходят за каталогом, сервер не держит тысячи SSH-сессий), Ansible — в простоте, отсутствии агентов и оркестрации шагов по порядку.

## Ресурсы: из чего состоит любая конфигурация

**Ресурс** — одна единица состояния системы: пакет, файл, сервис, пользователь, cron-задача. Синтаксис:
```puppet
package { 'nginx':          # тип ресурса и его title (заголовок — уникальное имя внутри каталога)
  ensure => installed,      # атрибуты: желаемое состояние
}
```

Puppet умеет **показать текущее состояние** любого ресурса в виде кода — лучший способ понять синтаксис:
```bash
puppet resource user root
# → user { 'root':
# →   ensure => 'present',
# →   home   => '/root',
# →   shell  => '/bin/bash',
# →   uid    => 0,
# →   ...
# → }
puppet resource package openssl
puppet resource service ssh
puppet describe file            # документация по типу ресурса и всем его атрибутам
```

**RAL (Resource Abstraction Layer, слой абстракции ресурсов)** — почему один и тот же `package { 'nginx': }` работает на Ubuntu и RHEL. У каждого типа есть **провайдеры** — реализации под конкретную систему: для `package` это `apt`, `dnf`, `yum`, `pip`, `gem`; для `service` — `systemd`, `init`. Puppet сам выбирает провайдер по фактам, при необходимости его задают явно: `provider => pip`.

Основные типы:

| Тип | Пример | Что важно |
|---|---|---|
| `package` | `ensure => installed` / `'1.24.0-1'` / `latest` / `absent` | `latest` обновит пакет на каждом запуске, когда выйдет новая версия, — в проде лучше фиксированная версия |
| `file` | `ensure => file`, `content`, `source`, `mode => '0644'` | также папки (`ensure => directory`) и ссылки (`ensure => link`) |
| `service` | `ensure => running`, `enable => true` | `enable` — автозапуск при загрузке |
| `user`, `group` | `ensure => present`, `uid`, `groups`, `managehome => true` | |
| `exec` | запуск команды | не идемпотентен сам по себе, см. ниже |
| `cron` | `command`, `hour`, `minute` | |
| `host` | запись в `/etc/hosts` | |
| `mount` | точка монтирования и строка в fstab | |

**Первый манифест.** **Манифест** — файл с кодом Puppet, расширение `.pp`:
```puppet
# hello.pp
file { '/etc/motd':
  ensure  => file,
  content => "Сервер управляется Puppet. Ручные правки будут перезаписаны.\n",
  owner   => 'root',
  mode    => '0644',
}
```
```bash
puppet apply hello.pp
# → Notice: Compiled catalog for node1 in environment production in 0.01 seconds
# → Notice: /Stage[main]/Main/File[/etc/motd]/ensure: defined content as '{sha256}f38fc8d5...'
# → Notice: Applied catalog in 0.03 seconds
puppet apply hello.pp       # второй запуск
# → Notice: Applied catalog in 0.01 seconds    — строк про ресурсы нет: изменений нет, так и должно быть
```

## Идемпотентность и exec

**Идемпотентность** — повторный запуск не меняет систему, если она уже в нужном состоянии. Для Puppet это критично: агент применяет каталог каждые 30 минут, и неидемпотентный код будет что-то менять (или перезапускать сервисы) 48 раз в сутки.

Встроенные типы идемпотентны: `file` сравнивает контрольную сумму, `package` проверяет установленную версию. Исключение — **`exec`**: Puppet не знает, что делает произвольная команда, и без условий запускает её при каждом применении каталога.

Как сделать `exec` идемпотентным:
```puppet
exec { 'init-swarm':
  command => '/usr/bin/docker swarm init',
  creates => '/var/lib/docker/swarm/state.json',     # пропустить, если файл уже существует
}

exec { 'set-hostname':
  command => "/usr/bin/hostnamectl set-hostname ${facts['networking']['fqdn']}",
  unless  => "/usr/bin/test \"$(hostname -f)\" = '${facts['networking']['fqdn']}'",   # пропустить, если команда вернула 0
}

exec { 'reload-sysctl':
  command     => '/usr/sbin/sysctl --system',
  refreshonly => true,          # запускать ТОЛЬКО по уведомлению от другого ресурса (notify/subscribe)
}
```
- `creates` — путь к файлу: если он есть, команда не выполняется;
- `onlyif` — выполнить, только если проверочная команда вернула 0;
- `unless` — выполнить, только если проверочная команда вернула НЕ 0;
- `refreshonly => true` — запускать только когда пришло уведомление (аналог handler в Ansible).

Команды в `exec` пишут **с полным путём** (`/usr/bin/docker`) или задают `path => ['/usr/bin', '/usr/sbin']`: у агента своё окружение, и `PATH` там может отличаться от твоей консоли.

Правило то же, что в Ansible: сначала ищи готовый тип или модуль на Forge, `exec` — последнее средство.

## Язык Puppet: переменные, факты, условия, циклы

**Переменные** начинаются с `$` и **присваиваются один раз** в своей области видимости — переназначить нельзя (это декларативный язык, а не скрипт):
```puppet
$app_port = 8080
$app_port = 9090          # ошибка: Cannot reassign variable '$app_port'
```

**Строки:** в одинарных кавычках — как есть, в двойных — с подстановкой:
```puppet
$srv = 'web'
notice('Имя: $srv')              # → Имя: $srv
notice("Имя: ${srv}-01")         # → Имя: web-01
```
`notice()` печатает сообщение при компиляции — удобно для отладки. Имя `$name` для своих переменных не подходит: это встроенная переменная (в классах и defined types — имя ресурса, как `$title`), и присвоить её нельзя.

**Факты** доступны через хэш `$facts`:
```puppet
notice($facts['os']['family'])                  # → Debian
notice($facts['os']['release']['major'])        # → 24.04
notice($facts['networking']['ip'])              # → 10.0.1.15
notice($facts['processors']['count'])           # → 4
notice($facts['memory']['system']['total'])     # → 7.75 GiB
```
Старый стиль `$::osfamily` или `$::ipaddress` (legacy facts — «плоские» факты из Puppet 3) в Puppet 8 по умолчанию отключён. Если видишь его в чужом коде — это признак старого модуля.

**Trusted facts** (`$trusted`) — данные из сертификата ноды: `$trusted['certname']`, `$trusted['extensions']['pp_role']`. Обычные факты нода присылает сама, и взломанная нода может подделать, например, факт `role`, чтобы получить чужие секреты. Trusted facts подписаны CA и подделать их нельзя, поэтому решения о том, что выдать ноде (роль, секреты), принимают по ним.

**Типы данных** — параметры классов проверяются при компиляции:
```puppet
String, Integer, Boolean, Array[String], Hash[String, Integer]
Enum['present', 'absent']                # одно из значений
Optional[String]                         # строка или undef
Stdlib::Port, Stdlib::Absolutepath       # готовые типы из модуля puppetlabs-stdlib
Sensitive[String]                        # секрет: не печатается в логах и отчётах
```

**Условия:**
```puppet
if $facts['os']['family'] == 'Debian' {
  $pkg = 'apache2'
} elsif $facts['os']['family'] == 'RedHat' {
  $pkg = 'httpd'
} else {
  fail("ОС ${facts['os']['family']} не поддерживается")    # прервать компиляцию с понятной ошибкой
}

case $facts['os']['family'] {
  'Debian': { $pkg = 'apache2' }
  'RedHat': { $pkg = 'httpd' }
  default:  { fail('не поддерживается') }
}

# selector — выбор значения, короче case
$pkg = $facts['os']['family'] ? {
  'Debian' => 'apache2',
  'RedHat' => 'httpd',
}
```

**Циклы** (итерация по массиву или хэшу):
```puppet
$users = {
  'alex' => { 'groups' => ['sudo', 'docker'] },
  'ci'   => { 'groups' => ['docker'] },
}
$users.each |String $name, Hash $opts| {
  user { $name:
    ensure     => present,
    groups     => $opts['groups'],
    shell      => '/bin/bash',
    managehome => true,
  }
}

['curl', 'jq', 'htop'].each |$p| {
  package { $p: ensure => installed }
}
```
Для пакетов цикл не обязателен: `package { ['curl', 'jq', 'htop']: ensure => installed }` — массив в title создаёт три ресурса.

## Порядок и зависимости

Puppet строит из ресурсов **граф зависимостей** и применяет его. С Puppet 4 ресурсы без явных связей выполняются в порядке записи в манифесте, но полагаться на это нельзя: между классами и модулями порядок не очевиден. Зависимости пишут явно.

**Метапараметры отношений** (работают у любого ресурса):
- `require => Package['nginx']` — этот ресурс применить **после** указанного;
- `before => File['/etc/nginx/nginx.conf']` — этот ресурс применить **до** указанного;
- `notify => Service['nginx']` — после меня, и если я изменился — отправить **обновление** (refresh) указанному ресурсу. Для `service` refresh означает перезапуск;
- `subscribe => File['/etc/nginx/nginx.conf']` — то же, но со стороны получателя.

Обрати внимание: при ссылке на ресурс тип пишется **с заглавной буквы** (`Package['nginx']`), при объявлении — со строчной (`package { 'nginx': }`).

**Классический паттерн «пакет → файл → сервис»:**
```puppet
package { 'nginx':
  ensure => installed,
}

file { '/etc/nginx/nginx.conf':
  ensure  => file,
  content => epp('profile/nginx/nginx.conf.epp', { 'workers' => $facts['processors']['count'] }),
  require => Package['nginx'],          # конфиг кладём после установки пакета
  notify  => Service['nginx'],          # конфиг изменился → перезапустить nginx
}

service { 'nginx':
  ensure => running,
  enable => true,
}
```

**То же стрелками** (chaining arrows):
```puppet
Package['nginx'] -> File['/etc/nginx/nginx.conf'] ~> Service['nginx']
# ->  «до» (как before/require)
# ~>  «до и уведомить» (как notify/subscribe)
```

Чем refresh в Puppet отличается от handler в Ansible: уведомление срабатывает только если ресурс-источник реально изменился, и сервис перезапустится **один раз**, даже если поменялись три файла. Перезапуск выполняется сразу в рамках того же запуска, в порядке графа — не нужно ждать конца плея.

**Циклические зависимости** — частая ошибка: A требует B, B требует A. Компиляция пройдёт, но агент упадёт с `Found 1 dependency cycle`. Найти цикл помогает `puppet agent -t --graph`: Puppet запишет граф в `.dot`-файлы (`/opt/puppetlabs/puppet/cache/state/graphs/`), которые можно открыть через Graphviz.

**Отношения между классами** — `include` не задаёт порядок. Чтобы ресурсы класса оказались «внутри» другого класса и подчинялись его зависимостям, используют `contain`:
```puppet
class profile::app {
  contain profile::app::install      # contain: ресурсы класса считаются частью profile::app
  contain profile::app::config
  contain profile::app::service

  Class['profile::app::install']
  -> Class['profile::app::config']
  ~> Class['profile::app::service']
}
```
Без `contain` выражение `Class['profile::app'] -> Class['other']` не гарантирует, что внутренние классы выполнятся до `other`.

## Классы и defined types

**Класс** — именованный блок ресурсов, который применяется к ноде **один раз**: «настройка nginx», «базовая настройка ОС».
```puppet
class profile::nginx (
  String                $version            = 'installed',
  Integer[1]            $worker_processes   = $facts['processors']['count'],
  Array[Stdlib::Port]   $listen_ports       = [80],
) {
  package { 'nginx':
    ensure => $version,
  }
  file { '/etc/nginx/nginx.conf':
    ensure  => file,
    content => epp('profile/nginx/nginx.conf.epp', {
      'worker_processes' => $worker_processes,
    }),
    require => Package['nginx'],
    notify  => Service['nginx'],
  }
  service { 'nginx':
    ensure => running,
    enable => true,
  }
}
```

**Два способа подключить класс:**
```puppet
include profile::nginx              # include-подобно: можно вызывать сколько угодно раз и из разных мест

class { 'profile::nginx':           # resource-like: с параметрами прямо в коде
  version => '1.24.0-2ubuntu7',
}
```
Resource-like объявление можно сделать **только один раз** за каталог, и оно конфликтует с `include` того же класса, если тот уже был объявлен раньше (`Duplicate declaration: Class[Profile::Nginx] is already declared`). Поэтому параметры классов почти всегда задают через Hiera (см. ниже), а в коде пишут только `include`.

**Defined type** — шаблон ресурса, который можно объявить **много раз** с разными параметрами. Нужен для повторяющихся сущностей: виртуальные хосты, пользователи приложения, конфиги сайтов.
```puppet
define profile::nginx::vhost (
  String               $server_name,
  Stdlib::Port         $port = 80,
  Stdlib::Absolutepath $root = "/var/www/${title}",    # $title — имя, с которым объявили ресурс
) {
  file { $root:
    ensure => directory,
  }
  file { "/etc/nginx/conf.d/${title}.conf":
    ensure  => file,
    content => epp('profile/nginx/vhost.conf.epp', {
      'server_name' => $server_name,
      'port'        => $port,
      'root'        => $root,
    }),
    notify  => Service['nginx'],
  }
}

profile::nginx::vhost { 'shop':
  server_name => 'shop.example.com',
}
profile::nginx::vhost { 'api':
  server_name => 'api.example.com',
  port        => 8080,
}
```
Разница одной фразой: класс — синглтон («nginx на этой машине»), defined type — может быть много экземпляров («каждый сайт»).

## Шаблоны: EPP и ERB

Шаблон генерирует содержимое файла из переменных. Два формата:
- **EPP** (Embedded Puppet) — синтаксис Puppet внутри шаблона, параметры объявляются явно. Рекомендуемый.
- **ERB** (Embedded Ruby) — Ruby внутри шаблона, встречается в старых модулях.

```epp
<%- | Integer $worker_processes | -%>
# Управляется Puppet (модуль profile). Ручные правки будут перезаписаны.
user www-data;
worker_processes <%= $worker_processes %>;

events {
    worker_connections 4096;
}

http {
    include /etc/nginx/conf.d/*.conf;
}
```
- `<%- | ... | -%>` — список параметров шаблона с типами: если забыть передать параметр, компиляция упадёт с понятной ошибкой;
- `<%= выражение %>` — вставить значение;
- `<% код %>` — выполнить без вывода (условия, циклы);
- `-` у тегов убирает лишние пустые строки.

Шаблоны лежат в папке `templates/` модуля, а функция `epp('модуль/путь.epp', {...})` ищет их там: `epp('profile/nginx/nginx.conf.epp')` → `profile/templates/nginx/nginx.conf.epp`.

Статические файлы кладут в `files/` модуля и подключают через `source`:
```puppet
file { '/etc/logrotate.d/app':
  ensure => file,
  source => 'puppet:///modules/profile/logrotate-app',   # → profile/files/logrotate-app на сервере
}
```

## Модули и Forge

**Модуль** — папка со стандартной структурой, единица переиспользования (как роль в Ansible):
```text
nginx/
├── manifests/
│   ├── init.pp          # class nginx — главный класс модуля
│   ├── install.pp       # class nginx::install
│   └── vhost.pp         # define nginx::vhost
├── templates/           # *.epp, *.erb
├── files/               # статические файлы для source => 'puppet:///modules/nginx/...'
├── lib/facter/          # свои факты на Ruby
├── data/                # данные модуля для Hiera (значения по умолчанию)
├── hiera.yaml
├── spec/                # тесты rspec-puppet
└── metadata.json        # имя, версия, зависимости, поддерживаемые ОС
```
Имя класса отражает путь: `nginx::vhost` ↔ `manifests/vhost.pp`. Puppet находит классы автоматически (autoloading), поэтому имя и путь обязаны совпадать.

**PDK (Puppet Development Kit)** — генерирует скелет модуля, классы и тесты, запускает проверки:
```bash
pdk new module profile
pdk new class profile::nginx
pdk validate            # синтаксис, puppet-lint, metadata
pdk test unit           # тесты rspec-puppet
```

**Puppet Forge** (forge.puppet.com) — публичный каталог модулей. Самые нужные: `puppetlabs-stdlib` (функции и типы данных), `puppetlabs-concat` (собрать файл из кусков от разных ресурсов), `puppetlabs-apt`, `puppetlabs-firewall`, `puppet-nginx`, `puppetlabs-haproxy`, `puppetlabs-postgresql`. Модули с префиксом `puppet-` поддерживает сообщество **Vox Pupuli**, у них хорошее качество и тесты.

## Окружения, control repo и r10k

**Окружение** (environment) — отдельный набор кода на сервере: `production`, `staging`, `feature_x`. Нода работает в одном окружении, и можно проверить изменения на части нод, не трогая прод.

**Control repo** — git-репозиторий, где **каждая ветка = окружение**. В нём:
```text
control-repo/
├── Puppetfile            # какие модули и каких версий скачать
├── environment.conf
├── hiera.yaml            # иерархия Hiera окружения
├── data/                 # данные Hiera
├── manifests/site.pp     # классификация нод
└── site-modules/         # свои модули: role/, profile/
    ├── role/
    └── profile/
```

**Puppetfile** — список внешних модулей с фиксированными версиями (как `requirements.txt` в Python):
```ruby
forge 'https://forge.puppet.com'

mod 'puppetlabs-stdlib',     '9.6.0'
mod 'puppetlabs-concat',     '9.0.2'
mod 'puppetlabs-postgresql', '10.3.0'

mod 'internal_app',
  git: 'git@gitlab.example.com:puppet/internal_app.git',
  tag: 'v2.4.1'
```

**r10k** — утилита, которая разворачивает control repo на сервере: для каждой ветки создаёт папку окружения и скачивает модули из Puppetfile:
```bash
r10k deploy environment production --modules   # обновить окружение production вместе с модулями
r10k deploy environment --modules              # все ветки
# → /etc/puppetlabs/code/environments/production
# → /etc/puppetlabs/code/environments/staging
# → /etc/puppetlabs/code/environments/feature_nginx_tuning
```
Обычно r10k запускает CI после мержа или вебхук из GitLab. В Puppet Enterprise то же делает встроенный **Code Manager**. **g10k** — более быстрый аналог r10k на Go.

Проверить изменение на одной ноде, не трогая остальные:
```bash
git checkout -b feature_nginx_tuning && git push          # → r10k создал окружение feature_nginx_tuning
sudo puppet agent -t --environment feature_nginx_tuning --noop   # что поменялось бы на этой ноде
```

## Hiera: данные отдельно от кода

**Hiera** — встроенная в Puppet иерархическая база «ключ → значение» в YAML. Идея: код модулей одинаковый для всех, а различия между окружениями, ролями и конкретными нодами (версии, порты, размеры пулов, адреса) лежат в данных.

**Иерархия** описывается в `hiera.yaml` — от самого конкретного уровня к самому общему. Hiera ищет ключ сверху вниз и берёт **первое найденное** значение:
```yaml
# control-repo/hiera.yaml
---
version: 5
defaults:
  datadir: data
  data_hash: yaml_data
hierarchy:
  - name: "Конкретная нода"
    path: "nodes/%{trusted.certname}.yaml"
  - name: "Роль в окружении"
    path: "roles/%{trusted.extensions.pp_role}/%{server_facts.environment}.yaml"
  - name: "Роль"
    path: "roles/%{trusted.extensions.pp_role}.yaml"
  - name: "Семейство ОС"
    path: "os/%{facts.os.family}.yaml"
  - name: "Общее"
    path: "common.yaml"
```
`%{...}` — подстановка фактов: для ноды `web01.example.com` с ролью `web` в окружении `production` Hiera по очереди посмотрит `nodes/web01.example.com.yaml` → `roles/web/production.yaml` → `roles/web.yaml` → `os/Debian.yaml` → `common.yaml`.

**Automatic parameter lookup** — главная магия: параметр класса `profile::nginx::worker_processes` Puppet сам ищет в Hiera по ключу `profile::nginx::worker_processes`. Не нужно ничего писать в коде:
```yaml
# data/common.yaml
profile::nginx::worker_processes: 2
profile::nginx::listen_ports: [80]

# data/roles/web/production.yaml
profile::nginx::worker_processes: 8
profile::nginx::listen_ports: [80, 443]
```
Порядок, откуда берётся значение параметра: явно в `class { ... }` → Hiera → значение по умолчанию в коде класса.

**Функция lookup** — когда значение нужно явно:
```puppet
$ntp_servers = lookup('profile::base::ntp_servers', Array[String], 'first', ['169.254.169.123'])
#                      ключ                          тип            стратегия  значение по умолчанию
```
**Стратегии слияния** — что делать, если ключ найден на нескольких уровнях:
- `first` (по умолчанию) — первое найденное значение;
- `unique` — объединить массивы со всех уровней без повторов (например, список пакетов: общие + для роли);
- `hash` — объединить хэши, верхний уровень побеждает при совпадении ключей;
- `deep` — рекурсивное объединение вложенных хэшей.

Стратегию для конкретного ключа можно задать в данных:
```yaml
lookup_options:
  profile::base::packages:
    merge: unique
```

Отладить, откуда пришло значение:
```bash
puppet lookup profile::nginx::worker_processes --node web01.example.com --explain
# → Searching for "profile::nginx::worker_processes"
# →   Path "data/nodes/web01.example.com.yaml"  — not found
# →   Path "data/roles/web/production.yaml"     — Found key: value: 8
```

## Roles и profiles

**Roles and profiles** — стандартный паттерн организации кода Puppet. О нём спрашивают почти на каждом собеседовании, где упоминают Puppet.

Три слоя:
1. **Компонентные модули** — универсальные модули с Forge или свои (`nginx`, `postgresql`, `docker`). Ничего не знают о твоей компании.
2. **Profile** — обёртка «как мы используем технологию у себя»: берёт компонентный модуль, передаёт ему наши параметры из Hiera, добавляет нашу специфику (мониторинг, логи, firewall). Пример: `profile::nginx`, `profile::base`, `profile::postgresql`.
3. **Role** — бизнес-роль сервера, **только** список профилей и ничего больше. Пример: `role::web`, `role::db`.

```puppet
# site-modules/role/manifests/web.pp
class role::web {
  include profile::base          # пользователи, SSH, NTP, мониторинг — у всех серверов
  include profile::nginx
  include profile::app
}

# site-modules/profile/manifests/nginx.pp
class profile::nginx (
  Integer $worker_processes,     # значение — из Hiera
) {
  class { 'nginx':               # компонентный модуль с Forge
    worker_processes => $worker_processes,
    server_tokens    => 'off',
  }
  include profile::monitoring::nginx_exporter
}
```
Правила:
- **У каждой ноды ровно одна роль.** Если нужно «web + немного кэша», создаётся новая роль `role::web_with_cache`, а не две роли на ноде.
- В роли нет ресурсов и параметров — только `include profile::...`.
- Resource-like `class { 'nginx': ... }` пишут только в профилях, данные профиль берёт из Hiera.

Зачем: роль читается как документация («что это за сервер»), профиль — единственное место, где связываются технология и наши настройки, а компонентные модули можно обновлять с Forge, не переписывая свой код.

## Классификация нод: какой ноде что применить

**Классификация** — как сервер решает, какую роль получит нода. Варианты:

**1. site.pp** — по имени ноды прямо в коде:
```puppet
# manifests/site.pp
node 'db01.example.com' {
  include role::db
}
node /^web\d+\.example\.com$/ {           # регулярное выражение по certname
  include role::web
}
node default {
  include role::base
}
```
Минус: на тысяче серверов файл превращается в длинный список, а имена не всегда отражают роль.

**2. Роль из trusted facts** — роль зашивается в сертификат при первой регистрации ноды, и `site.pp` становится одной строкой:
```yaml
# /etc/puppetlabs/puppet/csr_attributes.yaml на ноде — ДО первого запуска агента
extension_requests:
  pp_role: web
  pp_environment: production
```
```puppet
# manifests/site.pp
node default {
  include "role::${trusted['extensions']['pp_role']}"
}
```
Этот файл удобно создавать из user data при запуске виртуалки (Terraform → cloud-init → csr_attributes.yaml → агент). Поскольку роль подписана CA, нода не может сама «повысить» себе роль.

**3. ENC (External Node Classifier)** — внешний скрипт или сервис, который по имени ноды возвращает YAML с классами и окружением. Так работают консоль Puppet Enterprise, Foreman, свои CMDB (базы учёта серверов).

## Секреты

Пароли, ключи и токены нельзя класть в Hiera открытым текстом: репозиторий видят все, а значения попадают в каталоги и отчёты.

**hiera-eyaml** — шифрует отдельные значения в YAML-файлах Hiera (по умолчанию PKCS7: открытым ключом шифрует кто угодно, закрытый лежит только на сервере Puppet):
```bash
eyaml createkeys                                    # пара ключей, закрытый — только на сервер Puppet
eyaml encrypt -l 'profile::db::password' -s 'S3cr3t!'
# → profile::db::password: ENC[PKCS7,MIIBiQYJKoZIhvcNAQcDoIIBejCCAXYCAQAx...]
```
```yaml
# hiera.yaml — уровень с секретами
  - name: "Секреты"
    lookup_key: eyaml_lookup_key
    paths:
      - "secrets/nodes/%{trusted.certname}.eyaml"
      - "secrets/common.eyaml"
    options:
      pkcs7_private_key: /etc/puppetlabs/puppet/eyaml/private_key.pkcs7.pem
      pkcs7_public_key: /etc/puppetlabs/puppet/eyaml/public_key.pkcs7.pem
```

**Тип Sensitive** — значение помечается секретом и не печатается в логах, `--noop`-диффах и отчётах:
```puppet
class profile::db (
  Sensitive[String] $password,          # из Hiera придёт уже как Sensitive (если в lookup_options указано convert_to: Sensitive)
) {
  postgresql::server::role { 'app':
    password_hash => postgresql::postgresql_password('app', $password),
  }
  notice($password)                     # → Sensitive [value redacted]
  notice($password.unwrap)              # → S3cr3t!  — unwrap достаёт значение; в логи так писать нельзя
}
```
```yaml
lookup_options:
  profile::db::password:
    convert_to: Sensitive
```

**Внешние хранилища** — функции Deferred: значение берётся **на ноде во время применения**, а не на сервере при компиляции, поэтому секрет вообще не попадает в каталог:
```puppet
$token = Deferred('vault_lookup::lookup', ['secret/data/app/api', 'https://vault.example.com:8200'])
```
(функция из модуля `puppet-vault_lookup`, нода авторизуется в HashiCorp Vault своим сертификатом Puppet).

## Facter и свои факты

Факты — то, на основе чего принимаются решения в коде и в иерархии Hiera.
```bash
facter                         # все факты
facter os                      # вложенный хэш
facter networking.ip
facter -p                      # плюс факты, которые модули Puppet добавили на ноду (custom facts)
puppet facts show os.family    # то же через puppet
```

**Внешние факты** (external facts) — простой файл на ноде, без программирования:
```yaml
# /etc/puppetlabs/facter/facts.d/datacenter.yaml
datacenter: fra1
rack: r12
```
Можно и исполняемый скрипт в той же папке, который печатает `key=value`.

**Свои факты на Ruby** — в модуле, `lib/facter/*.rb`, сервер сам раздаст их нодам (pluginsync):
```ruby
# site-modules/profile/lib/facter/app_version.rb
Facter.add(:app_version) do
  confine kernel: 'Linux'                          # только на Linux
  setcode do
    path = '/opt/app/VERSION'
    File.read(path).strip if File.exist?(path)     # nil → факт не появится
  end
end
```
```puppet
if $facts['app_version'] and versioncmp($facts['app_version'], '2.0.0') < 0 {
  notice('старая версия приложения, нужна миграция')
}
```
Помни: обычные факты нода присылает сама, поэтому для безопасности (какую роль и какие секреты выдать) используют trusted facts.

## Виртуальные и экспортируемые ресурсы

**Виртуальный ресурс** — объявлен, но не применяется, пока его явно не «реализовали». Решает проблему дубликатов: пользователя `alex` хотят создать и профиль админов, и профиль разработчиков, а объявить ресурс дважды нельзя.
```puppet
# объявляем всех пользователей один раз, виртуально (символ @)
@user { 'alex': ensure => present, groups => ['sudo'], tag => 'admins' }
@user { 'olga': ensure => present, groups => ['dev'],  tag => 'developers' }

# реализуем там, где нужно
realize(User['alex'])
User <| tag == 'developers' |>        # collector: реализовать все виртуальные User с тегом developers
```

**Экспортируемый ресурс** (exported resource) — нода объявляет ресурс «для других», он сохраняется в PuppetDB, а другие ноды его собирают. Так сервисы находят друг друга без ручных списков.

Классический пример: веб-серверы сами регистрируются в балансировщике HAProxy:
```puppet
# profile::app — на каждой веб-ноде
@@haproxy::balancermember { $facts['networking']['fqdn']:    # @@ — экспортировать
  listening_service => 'web',
  server_names      => $facts['networking']['hostname'],
  ipaddresses       => $facts['networking']['ip'],
  ports             => '8080',
  options           => 'check',
}

# profile::loadbalancer — на балансировщике
haproxy::listen { 'web':
  ipaddress => '0.0.0.0',
  ports     => '80',
}
Haproxy::Balancermember <<| listening_service == 'web' |>>    # собрать все экспортированные из PuppetDB
```
Как это работает во времени: новая веб-нода при своём запуске агента отправляет ресурс в PuppetDB → при следующем запуске агента на балансировщике (до 30 минут) он попадёт в конфиг HAProxy. Это **не мгновенно**, и удалённую ноду из PuppetDB нужно вычищать (`puppet node deactivate web07.example.com`), иначе она останется в балансировке. Для быстро меняющихся облачных групп лучше service discovery (Consul, облачные target groups).

## Запуск, noop, отчёты и отладка

```bash
sudo puppet agent -t                 # --test: запустить сейчас, подробный вывод, один раз, без демона
sudo puppet agent -t --noop          # «сухой прогон»: показать, что изменилось бы, ничего не меняя
sudo puppet agent -t --debug         # максимально подробно
sudo puppet agent -t --tags nginx    # применить только ресурсы с тегом (у класса тег = его имя)
sudo puppet agent --disable "INC-1234: ручная диагностика, alex"   # временно выключить агента с причиной
sudo puppet agent --enable
sudo puppet apply --noop site.pp     # то же для локального манифеста
```

**Коды выхода** с `--detailed-exitcodes` (в `-t` включены автоматически) — их проверяют в CI и скриптах:
- `0` — успех, изменений нет;
- `2` — успех, были изменения;
- `4` — были ошибки;
- `6` — были и изменения, и ошибки.

Типичная ловушка: скрипт считает любой ненулевой код ошибкой и падает на `2`, хотя это нормальный успешный запуск с изменениями.

**Где смотреть:**
- `/opt/puppetlabs/puppet/cache/state/last_run_summary.yaml` — итог последнего запуска на ноде (время, число изменённых и упавших ресурсов). Удобно для мониторинга: алерт, если последний успешный запуск был больше 2 часов назад;
- `journalctl -u puppet` — лог агента;
- отчёты в PuppetDB и дашборды (Puppetboard, консоль Puppet Enterprise) — изменения по всем нодам;
- `/var/log/puppetlabs/puppetserver/puppetserver.log` — лог сервера.

**Проверка кода до коммита:**
```bash
puppet parser validate manifests/      # синтаксис
puppet-lint site-modules/              # стиль: отступы, кавычки, длина строк, документация
puppet epp validate templates/         # шаблоны EPP
```

## Тестирование и CI

Уровни проверки, от дешёвого к дорогому:
1. **puppet parser validate, puppet-lint, yamllint** — синтаксис и стиль, секунды.
2. **rspec-puppet** — юнит-тесты: компилирует каталог класса с заданными фактами и проверяет, что в нём есть нужные ресурсы. Машина не нужна.
3. **Onceover** — компилирует каталоги **всех ролей** control repo для набора типовых нод: ловит ошибки Hiera и конфликты между профилями.
4. **Litmus** (или устаревший Beaker) — приёмочные тесты: поднимает контейнер или VM, применяет класс по-настоящему, проверяет идемпотентность (второй запуск без изменений) и состояние сервера.
5. **noop на реальных нодах** — CI запускает `puppet agent -t --noop --environment <ветка>` на нескольких нодах и показывает дифф в merge request.

Пример rspec-puppet:
```ruby
# site-modules/profile/spec/classes/nginx_spec.rb
require 'spec_helper'

describe 'profile::nginx' do
  on_supported_os.each do |os, os_facts|          # для каждой ОС из metadata.json
    context "on #{os}" do
      let(:facts) { os_facts }
      let(:params) { { 'worker_processes' => 4 } }

      it { is_expected.to compile.with_all_deps }                      # каталог собирается, все зависимости есть
      it { is_expected.to contain_package('nginx') }
      it { is_expected.to contain_service('nginx').with_ensure('running') }
      it { is_expected.to contain_file('/etc/nginx/nginx.conf').with_content(%r{worker_processes 4;}) }
    end
  end
end
```

Пайплайн control repo в GitLab CI:
```yaml
stages: [lint, test, deploy]

lint:
  stage: lint
  image: registry.example.com/tools/pdk:3
  script:
    - puppet parser validate site-modules/ manifests/
    - puppet-lint --fail-on-warnings site-modules/
    - yamllint data/

unit:
  stage: test
  image: registry.example.com/tools/pdk:3
  script:
    - bundle exec onceover run spec        # каталоги всех ролей

deploy:
  stage: deploy
  script:
    - ssh puppet@puppet.example.com "r10k deploy environment ${CI_COMMIT_REF_NAME} --modules"
  rules:
    - if: $CI_COMMIT_BRANCH
```
Каждая ветка становится окружением, после мержа в `production` изменения подхватят все ноды в течение `runinterval`. Чтобы не выкатывать изменение на тысячу серверов разом, используют канареечное окружение: сначала ветка применяется на группе нод, потом мерж.

## Масштаб и эксплуатация

**Нагрузка на сервер.** Компиляция каталога — самая дорогая операция. Каждая нода приходит раз в 30 минут: 3000 нод ≈ 100 компиляций в минуту. Что помогает:
- **compilers** (компилирующие серверы) за балансировщиком: основной сервер оставляет себе CA, а каталоги компилируют несколько дополнительных;
- настройки JVM и числа JRuby-интерпретаторов Puppet Server (`max-active-instances` — сколько каталогов компилируется параллельно);
- **splay** (`splay = true` в `puppet.conf`) — случайная задержка перед запуском, чтобы все агенты не пришли одновременно после перезапуска или сбоя сети;
- меньше `exec`, тяжёлых функций и запросов к PuppetDB в коде.

**Сертификаты.**
```bash
puppetserver ca list                                  # ожидающие подписи запросы
puppetserver ca sign --certname web01.example.com
puppetserver ca clean --certname web01.example.com    # отозвать при удалении ноды
sudo puppet ssl clean                                 # на ноде: удалить свои сертификаты (при пересоздании)
```
Автоподпись (`autosign`) удобна в облаке, но подписывать всё подряд опасно: любой, кто достучится до порта 8140, получит каталог, а с ним и секреты. Безопасный вариант — **policy-based autosign**: скрипт проверяет общий секрет из `csr_attributes.yaml` (`challengePassword`) или что инстанс действительно существует в облаке.

Частая проблема: VM пересоздали с тем же именем, а старый сертификат остался на сервере → агент падает с ошибкой проверки сертификата. Решение — `puppetserver ca clean` на сервере и `puppet ssl clean` на ноде.

**Bolt** — инструмент от создателей Puppet для разовых операций и оркестрации **без агента**, по SSH или WinRM (по сути то, что делает Ansible):
```bash
bolt command run 'systemctl restart nginx' --targets web01,web02
bolt plan run profile::rolling_restart --targets web    # план: шаги по порядку, например по одному хосту
```
Классическая связка: Puppet держит постоянное состояние, Bolt (или Ansible) — для разовых действий, где важен порядок шагов.

## Версии и OpenVox

- **Puppet 8** (2023) — текущая основная версия: Ruby 3.2, `strict` режим включён по умолчанию (обращение к неизвестной переменной — ошибка, а не предупреждение), legacy facts отключены по умолчанию.
- Компания Puppet в 2022 году куплена **Perforce**. С конца 2024 года Perforce публикует новые бинарные сборки Puppet только для коммерческих клиентов (бесплатно — с ограничением по числу нод), исходный код при этом открыт.
- В ответ сообщество (Vox Pupuli) сделало форк **OpenVox** — совместимые по коду агент, сервер и PuppetDB с открытыми пакетами (`openvox-agent`, `openvox-server`). Код модулей и манифестов не меняется.
- **Puppet Enterprise** — коммерческая версия: веб-консоль, классификация нод мышью, оркестратор, Code Manager, отчёты о соответствии.

На собеседовании это знание показывает, что ты следишь за экосистемой: если компания сейчас на open source Puppet, вопрос «будете ли вы переходить на OpenVox или покупать лицензию» — уместный.

## Если опыт в Ansible, а спрашивают Puppet

Что сказать честно и с пользой:
- концепции те же: идемпотентность, желаемое состояние, разделение кода и данных (group_vars ↔ Hiera), переиспользуемые блоки (роли Ansible ↔ модули и профили), шифрованные секреты (Vault ↔ eyaml);
- главные отличия, которые ты понимаешь: агент и pull-модель с постоянным исправлением дрейфа, компиляция каталога на сервере, граф зависимостей вместо порядка задач, сертификаты и mTLS;
- готов показать на примере: «nginx: пакет → конфиг из шаблона → сервис с перезапуском при изменении» — один и тот же сценарий в Ansible и в Puppet.

Соответствие понятий:

| Ansible | Puppet |
|---|---|
| playbook | манифест (`.pp`), каталог |
| task + модуль | ресурс + тип и провайдер |
| роль | модуль; паттерн roles and profiles |
| inventory, группы | классификация нод (site.pp, trusted facts, ENC) |
| group_vars / host_vars | Hiera с иерархией |
| handler + notify | `notify` / `subscribe`, `~>` |
| `when` | `if`, `case`, selector |
| Ansible Vault | hiera-eyaml, Sensitive, Deferred + HashiCorp Vault |
| `--check --diff` | `--noop` |
| Molecule | rspec-puppet, Litmus, Onceover |
| ansible-galaxy, requirements.yml | Puppet Forge, Puppetfile, r10k |

## Типичные ошибки

- **`exec` без `creates`/`onlyif`/`unless`/`refreshonly`** — команда выполняется при каждом запуске агента, то есть 48 раз в сутки.
- **`ensure => latest` в проде** — пакет обновится на всех серверах в течение 30 минут после выхода новой версии в репозитории, без тестирования. Нужна фиксированная версия в Hiera.
- **Дубликат ресурса** — один и тот же `package { 'git': }` объявлен в двух модулях → `Duplicate declaration`. Решения: один профиль отвечает за ресурс, `ensure_packages` из stdlib или виртуальные ресурсы.
- **Resource-like объявление класса в нескольких местах** — тот же `Duplicate declaration`. В коде — только `include`, параметры — в Hiera.
- **Логика в Hiera, данные в коде** — версии, порты и адреса захардкожены в манифестах, а в Hiera пишут условия. Должно быть наоборот.
- **Решения о секретах по обычным фактам** — нода может подменить факт `role`. Для безопасности — `$trusted`.
- **Ручные правки на сервере** — Puppet вернёт всё через 30 минут. Если нужно срочно — `puppet agent --disable "причина"`, правка, потом коммит в код и `--enable`.
- **Забытый `puppet agent --disable`** — агент выключен месяцами, сервер тихо расходится с кодом. Мониторь `last_run_summary.yaml`.
- **Циклические зависимости** из-за `require` в обе стороны — `Found 1 dependency cycle`.
- **`include` вместо `contain` там, где нужен порядок** — зависимость `Class['a'] -> Class['b']` не распространяется на классы внутри `a`.
- **Мёртвые ноды в PuppetDB** — exported resources удалённых серверов остаются в конфигах балансировщиков и мониторинга. `puppet node deactivate` при удалении ноды.
- **Автоподпись всех сертификатов** — любой в сети получит каталог с секретами.

## Вопросы с собеседований

**Что такое Puppet и как работает один запуск агента?**
Инструмент управления конфигурацией с декларативным языком и агентом на каждой ноде. Агент собирает факты через Facter, отправляет их на Puppet Server, сервер классифицирует ноду, берёт данные из Hiera и компилирует каталог — список ресурсов с конкретными значениями и графом зависимостей. Агент применяет каталог, меняя только то, что отличается, и отправляет отчёт. По умолчанию это повторяется каждые 30 минут.

**Чем Puppet отличается от Ansible?**
Puppet — pull-модель с агентом: ноды сами раз в 30 минут приходят за каталогом и постоянно исправляют дрейф. Ansible — push без агентов по SSH, изменения происходят только когда его запустили. У Puppet свой декларативный язык и граф зависимостей, у Ansible — YAML с выполнением задач по порядку. Puppet сильнее в постоянном контроле состояния тысяч долгоживущих серверов и отчётности, Ansible — в простоте, разовых операциях и оркестрации последовательных шагов.

**Чем Puppet отличается от Terraform?**
Terraform создаёт инфраструктуру (VPC, виртуалки, базы как сервис) через API провайдеров и хранит state-файл. Puppet настраивает то, что внутри ОС, и state не хранит — каждый раз сравнивает с реальностью. Обычно они работают вместе: Terraform создаёт VM и через user data регистрирует её в Puppet с нужной ролью.

**Что такое каталог и где он компилируется?**
Итоговый документ для конкретной ноды: все ресурсы с вычисленными значениями и связями между ними. Компилируется на Puppet Server из кода, фактов ноды и данных Hiera. На ноду приходит уже готовый каталог, код Puppet на ней не выполняется (кроме фактов и Deferred-функций).

**Что такое идемпотентность и как сделать идемпотентным exec?**
Повторное применение не меняет систему, если она уже в нужном состоянии. Встроенные типы идемпотентны сами, а `exec` выполняется каждый раз. Для него используют `creates` (пропустить, если файл есть), `onlyif` и `unless` (проверочная команда), `refreshonly => true` (только по уведомлению). Лучше заменить `exec` готовым типом или модулем.

**Как задать порядок ресурсов? Чем require отличается от notify?**
Метапараметрами `before`, `require`, `notify`, `subscribe` или стрелками `->` и `~>`. `require` и `before` задают только порядок. `notify` и `subscribe` — порядок плюс refresh: если исходный ресурс изменился, получатель обновляется (сервис перезапускается). Перезапуск произойдёт один раз, даже если изменились несколько файлов.

**Чем класс отличается от defined type?**
Класс применяется к ноде один раз (синглтон): «настройка nginx». Defined type — шаблон ресурса, который можно объявить много раз с разными параметрами и уникальным title: «виртуальный хост», «пользователь приложения».

**Чем include отличается от resource-like объявления класса? А contain?**
`include` можно вызывать сколько угодно раз, параметры берутся из Hiera. Resource-like `class { 'x': param => ... }` задаёт параметры в коде, но допустим только один раз за каталог, иначе `Duplicate declaration`. `contain` работает как `include`, но делает ресурсы класса частью текущего класса, чтобы зависимости вида `Class['a'] -> Class['b']` распространялись на них.

**Что такое Hiera и как работает иерархия?**
Встроенное хранилище данных «ключ → значение» в YAML, отделяющее данные от кода. В `hiera.yaml` задаётся иерархия уровней от конкретного к общему (нода → роль в окружении → роль → ОС → common), с подстановкой фактов в пути. Hiera ищет ключ сверху вниз и по умолчанию берёт первое значение. Стратегии `unique`, `hash`, `deep` объединяют значения с нескольких уровней. Параметры классов ищутся автоматически по ключу `класс::параметр`.

**Что такое паттерн roles and profiles?**
Три слоя: компонентные модули (универсальные, с Forge), профили (как мы используем технологию: модуль плюс наши данные из Hiera и наша специфика) и роли (бизнес-роль сервера — только список профилей). У каждой ноды ровно одна роль. Роль читается как документация, профиль — единственное место связи технологии с нашими настройками.

**Как классифицировать ноды?**
Через `site.pp` с блоками `node` по имени или регулярному выражению, через роль в trusted facts (pp_role из csr_attributes.yaml, вшивается в сертификат при регистрации) или через ENC — внешний сервис, который по имени ноды отдаёт классы и окружение (консоль Puppet Enterprise, Foreman, CMDB).

**Чем facts отличаются от trusted facts?**
Обычные факты нода собирает и присылает сама, взломанная нода может их подделать. Trusted facts берутся из сертификата, подписанного CA (certname и расширения вроде pp_role), подделать их нельзя. Решения, влияющие на безопасность (роль, выдача секретов), принимают по trusted facts.

**Как хранить секреты в Puppet?**
hiera-eyaml: значения в Hiera шифруются открытым ключом, закрытый лежит только на сервере. Тип `Sensitive` не даёт значению попасть в логи, диффы и отчёты. Для внешних хранилищ — Deferred-функции (например, `vault_lookup`): секрет запрашивается на ноде при применении и не попадает в каталог. Плюс trusted facts в иерархии, чтобы нода получала только свои секреты.

**Что такое окружения и как работает r10k?**
Окружение — отдельный набор кода на сервере, нода работает в одном из них. Control repo хранит код в git, каждая ветка = окружение. r10k разворачивает ветки в папки окружений и скачивает модули по версиям из Puppetfile. Так можно проверить ветку на части нод через `--environment` и `--noop`, не трогая прод.

**Что такое exported resources и какие у них ограничения?**
Нода объявляет ресурс с `@@`, он сохраняется в PuppetDB, а другие ноды собирают его коллектором `<<| |>>`: например, веб-серверы регистрируются в HAProxy, а хосты — в мониторинге. Ограничения: нужен PuppetDB, изменения доходят до получателя только при его следующем запуске (до 30 минут), удалённые ноды надо деактивировать в PuppetDB, иначе их ресурсы останутся.

**Что такое виртуальные ресурсы и зачем они?**
Ресурс, объявленный с `@`, который не применяется, пока его не реализуют через `realize()` или коллектор `<| |>`. Решает проблему дубликатов, когда один и тот же ресурс (пользователь, пакет) нужен нескольким профилям, а объявить его можно только один раз.

**Как проверить изменение, не сломав прод?**
`puppet parser validate` и `puppet-lint` в CI, юнит-тесты rspec-puppet, Onceover для каталогов всех ролей. Затем ветка становится окружением, и на нескольких нодах запускается `puppet agent -t --noop --environment <ветка>`. После мержа — сначала на канареечную группу нод. В эксплуатации — мониторинг ошибок в отчётах PuppetDB.

**Что означают коды выхода puppet agent?**
С `--detailed-exitcodes` (в `-t` включено): 0 — успех без изменений, 2 — успех с изменениями, 4 — ошибки, 6 — изменения и ошибки. Код 2 — не ошибка, скрипты и CI должны это учитывать.

**Агент на ноде падает с ошибкой сертификата. Что делать?**
Чаще всего ноду пересоздали с тем же именем, а на сервере остался старый сертификат, или на ноде старый. На сервере `puppetserver ca clean --certname <имя>`, на ноде `puppet ssl clean`, затем `puppet agent -t` и подписать новый запрос. Также проверить время на ноде (сертификат «из будущего» не пройдёт проверку) и что certname совпадает с ожидаемым.

**Puppet Server не справляется с нагрузкой. Что делать?**
Добавить compilers за балансировщиком, оставив CA на основном сервере. Настроить JVM и число JRuby-экземпляров. Включить `splay`, чтобы агенты не приходили одновременно. Найти дорогие классы по метрикам времени компиляции и упростить их: меньше тяжёлых функций и запросов к PuppetDB. При необходимости увеличить `runinterval`.

**Кто-то поправил конфиг на сервере руками. Что произойдёт и как правильно делать срочные изменения?**
При следующем запуске агента (до 30 минут) Puppet вернёт файл к описанному виду и перезапустит сервис, если это прописано через notify. Правильно: для срочной диагностики — `puppet agent --disable "причина"`, затем изменение в коде Puppet, проверка через noop и `--enable`. Ручная правка, не перенесённая в код, потеряется.

**Что изменилось вокруг Puppet в последние годы?**
Puppet 8: Ruby 3.2, strict-режим и отключённые по умолчанию legacy facts. Компанию купил Perforce, и с конца 2024 года новые бинарные сборки доступны только по коммерческой лицензии (бесплатно с ограничением по числу нод). Сообщество Vox Pupuli выпустило совместимый открытый форк OpenVox (агент, сервер, PuppetDB), код модулей и манифестов при переходе не меняется.
