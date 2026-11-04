# Terraform

Подробно о Terraform: как он работает, язык HCL, state и командная работа, модули и как их правильно строить, рефакторинг без пересоздания ресурсов, CI/CD для инфраструктуры. Каждая тема: что это, зачем, как работает, пример с объяснением и типичные ошибки. В конце — вопросы и кейсы с собеседований.

## Как учить без курса

Курс HashiCorp и сертификация полезны, но основу можно быстро набрать самостоятельно:
- **Официальные туториалы**: https://developer.hashicorp.com/terraform/tutorials — треки «Get Started — AWS» и «Terraform Associate prep» (Terraform Associate — базовая сертификация HashiCorp по Terraform). Бесплатно, каждый урок 10–20 минут.
- **Список тем экзамена Terraform Associate**: https://developer.hashicorp.com/terraform/tutorials/certification-003/associate-review-003 — готовый чек-лист «что должен знать инженер», даже если не сдавать экзамен.
- **Книга** «Terraform: Up & Running» (Yevgeniy Brikman) — лучшая по паттернам, модулям и организации кода.
- **Практика** на своём репозитории `~/work/GitHubRepos/tf-aws-eks-cluster`: поднять VPC и EKS из модулей, специально сломать state, сделать import существующего ресурса (взять под управление Terraform ресурс, созданный вручную), переименовать ресурс через `moved` (обе операции разобраны в разделе про рефакторинг).

## Что такое Terraform и как он работает

**Infrastructure as Code (IaC)** — описание инфраструктуры (сетей, серверов, баз, DNS-записей, прав) в виде кода, который хранится в git, проходит ревью и применяется автоматически. Вместо кликов в консоли AWS:
- инфраструктура **воспроизводима** (поднять ещё одно окружение — запустить тот же код);
- изменения **видны и проверяются** (merge request, план изменений);
- есть **история** (кто, когда и зачем поменял);
- нет **дрейфа** между окружениями.

**Terraform декларативен.** Ты описываешь, *что* должно существовать («VPC с таким CIDR, три подсети»), а не *как* это создать по шагам. Terraform сам определяет, что нужно создать, изменить или удалить, чтобы реальность совпала с описанием.

### Из чего состоит
- **Terraform CLI** — читает `.tf`-файлы, строит граф зависимостей, сравнивает с текущим состоянием и вызывает провайдеры.
- **Провайдеры** — плагины, которые знают API конкретной системы: `aws`, `kubernetes`, `helm`, `cloudflare`, `gitlab`, `datadog`. Сам Terraform ничего не знает об AWS — всё через провайдер.
- **State** — файл, где Terraform запоминает, какие реальные объекты соответствуют ресурсам в коде.

### Рабочий цикл
```bash
terraform init        # скачать провайдеры и модули, настроить backend для state
terraform fmt         # отформатировать код
terraform validate    # проверить синтаксис и типы (без обращения к облаку)
terraform plan        # показать, что будет изменено
terraform apply       # применить изменения (покажет план и спросит подтверждения)
terraform destroy     # удалить всё, что описано в этой конфигурации
```

### Что делает plan
1. Читает код и строит **граф зависимостей** ресурсов.
2. **Refresh**: для каждого ресурса из state запрашивает у API его текущее реальное состояние.
3. Сравнивает три картины: **код** (желаемое), **state** (что Terraform создал раньше) и **реальность**.
4. Показывает план действий с символами:
   - `+` создать;
   - `~` изменить на месте (update in-place);
   - `-` удалить;
   - `-/+` **удалить и создать заново** (replace) — потому что изменился атрибут, который нельзя поменять у существующего объекта. **Самое важное, на что смотреть в плане**: replace базы данных означает потерю данных.

```
  # aws_db_instance.main must be replaced
-/+ resource "aws_db_instance" "main" {
      ~ identifier = "app-db" -> "app-db-prod" # forces replacement
```
Фраза `forces replacement` указывает, какой именно атрибут вызывает пересоздание.

Хорошая практика — сохранять план и применять **ровно его**:
```bash
terraform plan -out=tfplan     # план записан в файл
terraform apply tfplan         # применится именно то, что было в плане, без повторного расчёта
```
Так между ревью плана и применением ничего не изменится.

### 🧪 Мини-задание

**Задача.** Пройди полный рабочий цикл на провайдере `local`, без облака: `init` → `plan` → `apply` → повторный `plan` → `destroy`. Ресурс — файл `motd.txt` с текстом.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > main.tf <<'EOF'
terraform {
  required_providers {
    local = { source = "hashicorp/local", version = "~> 2.5" }
  }
}

resource "local_file" "motd" {
  filename = "${path.module}/motd.txt"
  content  = "hello from terraform\n"
}
EOF
terraform init -no-color | grep initialized
terraform plan -no-color | grep -E 'will be created|^Plan'
terraform apply -auto-approve -no-color | tail -1
cat motd.txt
terraform plan -no-color | grep 'No changes'
terraform destroy -auto-approve -no-color | tail -1
```

Ожидаемый результат:

```text
Terraform has been successfully initialized!
  # local_file.motd will be created
Plan: 1 to add, 0 to change, 0 to destroy.
Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
hello from terraform
No changes. Your infrastructure matches the configuration.
Destroy complete! Resources: 1 destroyed.
```

Разбор: `init` скачал провайдер в `.terraform/` и создал `.terraform.lock.hcl` с точной версией. `plan` сравнил код с state и реальностью и показал «будет создан». `apply` создал файл и записал его в `terraform.tfstate`. Повторный `plan` показал «No changes» — это и есть идемпотентность: код, state и реальность совпадают. `grep` и `tail` здесь только убирают лишний вывод — запусти команды без них, чтобы увидеть полный план.

</details>

## Язык HCL

### Провайдер и версии
```hcl
terraform {
  required_version = ">= 1.6"          # минимальная версия Terraform CLI
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"               # любая 5.x, но не 6.0 — защита от ломающих изменений
    }
  }
}

provider "aws" {
  region = var.region
  default_tags {                        # теги, которые получат ВСЕ ресурсы этого провайдера
    tags = {
      ManagedBy   = "terraform"
      Environment = var.env
    }
  }
}

provider "aws" {                        # второй экземпляр провайдера — для другого региона
  alias  = "us_east_1"
  region = "us-east-1"                  # например, сертификаты ACM для CloudFront обязаны быть в us-east-1
}
```
**Операторы версий:** `= 5.31.0` — ровно эта; `>= 5.0` — не ниже; `~> 5.31` — 5.31 и выше, но ниже 6.0; `~> 5.31.0` — только патчи 5.31.x.

**Файл `.terraform.lock.hcl`** создаётся при `init` и фиксирует точные версии провайдеров и их хеши. **Его коммитят в git**: так у всех в команде и в CI будет одна и та же версия провайдера. Обновить провайдеры: `terraform init -upgrade`.

### Ресурсы и источники данных
```hcl
resource "aws_vpc" "main" {             # resource "<тип>" "<локальное имя>"
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  tags = { Name = "main" }
}

resource "aws_subnet" "private_a" {
  vpc_id            = aws_vpc.main.id   # ссылка на атрибут другого ресурса: <тип>.<имя>.<атрибут>
  cidr_block        = "10.0.1.0/24"
  availability_zone = "us-east-2a"
}

data "aws_availability_zones" "available" {   # data — ЧИТАЕТ существующее, ничего не создаёт
  state = "available"
}

data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]
  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }
}
```
**Адрес ресурса** — `aws_vpc.main`. По нему ресурс находится в state, на него ссылаются другие ресурсы и команды (`terraform state show aws_vpc.main`).

**Зависимости.** Ссылка `aws_vpc.main.id` создаёт **неявную зависимость**: Terraform знает, что подсеть нужно создавать после VPC, а удалять до неё. Независимые ресурсы создаются параллельно. Явный `depends_on` нужен редко — когда зависимость есть, но не видна через ссылки (например, IAM-политика должна быть прикреплена до того, как приложение начнёт использовать роль).

### Переменные, locals, outputs
**Переменные (input variables)** — параметры конфигурации или модуля:
```hcl
variable "env" {
  type        = string
  description = "Окружение: dev, stage или prod"
  validation {
    condition     = contains(["dev", "stage", "prod"], var.env)
    error_message = "env должен быть dev, stage или prod."
  }
}

variable "instance_count" {
  type    = number
  default = 2                            # есть default — переменная необязательная
}

variable "db_password" {
  type      = string
  sensitive = true                       # не показывать значение в выводе plan и apply
}

variable "subnets" {
  type = map(object({                    # сложный тип: словарь объектов
    cidr = string
    az   = string
  }))
}
```
Откуда берутся значения (от низшего приоритета к высшему):
1. `default` в объявлении;
2. переменные окружения `TF_VAR_env=prod`;
3. файл `terraform.tfvars`;
4. файлы `*.auto.tfvars`;
5. флаги `-var-file=prod.tfvars` и `-var="env=prod"`.

**Locals** — вычисляемые значения внутри конфигурации, чтобы не повторять выражения:
```hcl
locals {
  name_prefix = "${var.project}-${var.env}"               # "shop-prod"
  azs         = slice(data.aws_availability_zones.available.names, 0, 3)   # первые три зоны
  common_tags = { Project = var.project, Owner = "platform" }
}
```

**Outputs** — значения, которые конфигурация отдаёт наружу: показать после apply, передать из модуля в вызывающий код, прочитать из другой конфигурации.
```hcl
output "vpc_id" {
  value       = aws_vpc.main.id
  description = "ID созданной VPC"
}
```

### count и for_each — несколько экземпляров ресурса
**count** — создать N одинаковых экземпляров, адресуемых по **индексу**:
```hcl
resource "aws_instance" "web" {
  count         = 3
  ami           = data.aws_ami.al2023.id
  instance_type = "t3.micro"
  tags          = { Name = "web-${count.index}" }   # web-0, web-1, web-2
}
# адреса: aws_instance.web[0], aws_instance.web[1], aws_instance.web[2]
```
**Проблема count со списками.** Если создавать по списку и удалить элемент из **середины**, индексы сдвинутся:
```hcl
variable "users" { default = ["alice", "bob", "carol"] }
resource "aws_iam_user" "u" {
  count = length(var.users)
  name  = var.users[count.index]
}
# удалили "bob" → список ["alice", "carol"]
# Terraform видит: [1] был bob, стал carol → ПЕРЕИМЕНОВАТЬ; [2] был carol → УДАЛИТЬ
# вместо простого «удалить bob» — пересоздание carol
```

**for_each** — экземпляры адресуются по **ключу**, а не по позиции:
```hcl
resource "aws_iam_user" "u" {
  for_each = toset(["alice", "bob", "carol"])
  name     = each.key
}
# адреса: aws_iam_user.u["alice"], aws_iam_user.u["bob"], aws_iam_user.u["carol"]
# удаление "bob" затронет только aws_iam_user.u["bob"]
```
С map можно передать параметры каждого экземпляра:
```hcl
variable "subnets" {
  default = {
    "private-a" = { cidr = "10.0.1.0/24", az = "us-east-2a" }
    "private-b" = { cidr = "10.0.2.0/24", az = "us-east-2b" }
  }
}
resource "aws_subnet" "this" {
  for_each          = var.subnets
  vpc_id            = aws_vpc.main.id
  cidr_block        = each.value.cidr       # each.key — ключ, each.value — значение
  availability_zone = each.value.az
  tags              = { Name = each.key }
}
```
**Правило:** `for_each` — для коллекций объектов; `count` — для «создать или нет» (`count = var.enable_bastion ? 1 : 0`) и действительно одинаковых экземпляров.

Ограничение: ключи `for_each` должны быть известны на этапе plan. Нельзя использовать в ключах ID ресурса, который ещё не создан.

### Выражения и функции
```hcl
# условие
instance_type = var.env == "prod" ? "m6i.large" : "t3.small"

# for-выражения — как list/dict comprehension в Python
private_subnet_ids = [for s in aws_subnet.this : s.id]
subnet_by_az       = { for k, s in aws_subnet.this : s.availability_zone => s.id }
prod_only          = [for u in var.users : u if u.env == "prod"]

# splat — короткая запись для списка атрибутов (для count)
instance_ids = aws_instance.web[*].id
```
Функции, которые нужны постоянно:
- `cidrsubnet("10.0.0.0/16", 8, 1)` → `10.0.1.0/24` — нарезка подсетей (добавить 8 бит к префиксу, взять подсеть номер 1);
- `merge(local.common_tags, { Name = "x" })` — слияние словарей;
- `lookup(var.sizes, var.env, "t3.small")` — значение из словаря с дефолтом;
- `try(var.config.timeout, 30)` — значение или запасное, если выражение упало;
- `coalesce(var.name, local.default_name)` — первое непустое;
- `flatten([[1, 2], [3]])` → `[1, 2, 3]`;
- `templatefile("user-data.sh.tpl", { cluster = var.name })` — файл-шаблон с подстановкой;
- `jsonencode({...})` — HCL-объект в JSON (IAM-политики);
- `yamldecode(file("config.yaml"))` — прочитать YAML-файл как структуру.

Поэкспериментировать с выражениями: `terraform console`.

### dynamic — повторяющиеся вложенные блоки
Некоторые ресурсы содержат повторяющиеся вложенные блоки (правила Security Group). `dynamic` генерирует их из коллекции:
```hcl
resource "aws_security_group" "web" {
  name   = "web"
  vpc_id = aws_vpc.main.id

  dynamic "ingress" {
    for_each = var.allowed_ports          # например [80, 443]
    content {
      from_port   = ingress.value
      to_port     = ingress.value
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }
  }
}
```
Не злоупотребляй: код с вложенными `dynamic` трудно читать.

### lifecycle — управление поведением ресурса
```hcl
resource "aws_launch_template" "web" {
  # ...
  lifecycle {
    create_before_destroy = true          # при замене сначала создать новый, потом удалить старый (без простоя)
    prevent_destroy       = true          # plan с удалением этого ресурса упадёт с ошибкой — защита БД, бакетов
    ignore_changes        = [tags["LastScanned"], desired_capacity]   # не трогать атрибуты, которые меняет кто-то другой
    replace_triggered_by  = [aws_ssm_parameter.ami_id]   # пересоздать, если изменился другой ресурс
    precondition {
      condition     = var.instance_type != "t2.micro"
      error_message = "t2.micro слишком мал для этого сервиса."
    }
  }
}
```
Типичный пример `ignore_changes`: `desired_capacity` у Auto Scaling Group меняет автоскейлер. Без `ignore_changes` каждый `apply` возвращал бы его к значению из кода.

### Секреты в Terraform
- `sensitive = true` **только скрывает значение в выводе** plan и apply. **В state значение хранится открытым текстом.** Поэтому state шифруют и ограничивают доступ к нему.
- Не хранить секреты в `.tfvars` в git. Варианты: читать из AWS Secrets Manager или SSM Parameter Store через data source; передавать через `TF_VAR_*` из защищённых переменных CI.
- Лучше, чтобы секрет вообще не попадал в state: пусть AWS сам генерирует и хранит пароль (`manage_master_user_password = true` у RDS — пароль создаётся и ротируется в Secrets Manager, Terraform его не видит), или используй ephemeral-ресурсы (значения, которые существуют только во время plan/apply и не сохраняются) и write-only атрибуты (атрибуты, которые передаются в API, но не записываются в state) — они появились в Terraform 1.10–1.11.

### 🧪 Мини-задание

**Задача.** Без единого ресурса поэкспериментируй с выражениями HCL в `terraform console`: собери префикс имени из переменной, отфильтруй сервисы с портом через `for … if`, посчитай сумму реплик, сгенерируй список имён и достань необязательный атрибут через `try`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > main.tf <<'EOF'
variable "env" {
  default = "prod"
}

variable "services" {
  default = {
    api  = { port = 8080, replicas = 3 }
    web  = { port = 80, replicas = 2 }
    cron = { port = 0, replicas = 1 }
  }
}

locals {
  name_prefix = "${var.env}-app"
  exposed     = { for name, s in var.services : name => s.port if s.port > 0 }
  total       = sum([for s in values(var.services) : s.replicas])
}
EOF
terraform init -no-color > /dev/null
terraform console <<'EOF'
local.name_prefix
local.exposed
local.total
upper(var.env)
[for name in sort(keys(var.services)) : "${local.name_prefix}-${name}"]
try(var.services["api"].cpu, "500m")
EOF
```

Ожидаемый результат:

```text
"prod-app"
{
  "api" = 8080
  "web" = 80
}
6
"PROD"
[
  "prod-app-api",
  "prod-app-cron",
  "prod-app-web",
]
"500m"
```

Разбор: `terraform console` вычисляет выражения в контексте конфигурации — лучший способ отладить сложный `for` или функцию, ничего не применяя. `for … if` вернул словарь только с сервисами, у которых порт больше 0. `values()` + `for` + `sum()` посчитали реплики. `keys()` у map возвращает ключи, а `sort()` делает порядок предсказуемым. `try()` вернул значение по умолчанию, потому что атрибута `cpu` нет — без `try` была бы ошибка.

</details>

## State

### Зачем нужен state
State — файл `terraform.tfstate` (JSON), где для каждого ресурса из кода записано, какому реальному объекту он соответствует (ID в AWS), и последние известные атрибуты.

Почему нельзя обойтись без него:
- **Сопоставление.** В коде `aws_instance.web`, в AWS — `i-0abc123`. Только state связывает их. Без state Terraform не знает, что этот инстанс он уже создал, и попытается создать ещё один.
- **Удаление.** Если ресурс убрали из кода, Terraform узнаёт о необходимости удалить его только из state.
- **Зависимости при удалении** и **производительность** (кэш атрибутов).

### Remote backend
По умолчанию state лежит локально. Для работы команды это не годится: у каждого своя копия, нет защиты от одновременного запуска, ноутбук может потеряться. Поэтому state хранят удалённо:
```hcl
terraform {
  backend "s3" {
    bucket       = "mycompany-terraform-state"
    key          = "prod/network/terraform.tfstate"   # путь — отдельный для каждого стека
    region       = "us-east-2"
    encrypt      = true                                # шифрование на стороне S3
    use_lockfile = true                                # блокировка через файл в S3 (Terraform 1.10+)
    # в старых версиях блокировка через DynamoDB: dynamodb_table = "terraform-locks"
  }
}
```
Настройки бакета для state: **versioning** (можно откатить испорченный state на предыдущую версию), шифрование, Block Public Access, доступ только ролям CI и администраторов.

### Блокировка (locking)
Если два человека одновременно запустят `apply`, оба прочитают один state, внесут разные изменения и перезапишут state друг друга. Ресурсы «потеряются». **Lock** гарантирует, что в каждый момент state меняет только один процесс: второй получит ошибку «state locked».

Если процесс упал и не снял блокировку (убили задачу в CI):
```bash
terraform force-unlock <LOCK_ID>   # только убедившись, что никто действительно не выполняет apply
```

### Команды для работы со state
```bash
terraform state list                         # все ресурсы в state
terraform state show aws_vpc.main            # атрибуты ресурса
terraform state mv aws_instance.a aws_instance.b   # переименовать адрес (лучше блок moved, см. ниже)
terraform state rm aws_s3_bucket.logs        # «забыть» ресурс: удалить из state, НЕ удаляя в облаке
terraform state pull > backup.tfstate        # скачать state (перед рискованными операциями)
terraform plan -refresh-only                 # показать, что изменилось в облаке вне Terraform (drift)
terraform apply -replace=aws_instance.web    # принудительно пересоздать ресурс (замена устаревшего taint)
AWS_PROFILE=test terraform state rm 'module.opensearch.opensearch_roles_mapping.all_access'
# адреса с индексами и модулями берут в одинарные кавычки, иначе shell съест [ ] и "
```

### Опасные флаги: -lock=false и -target
- **`-lock=false`** отключает блокировку state. Допустимо для чтения (`plan` при отладке, когда точно никто не применяет), но **`apply -lock=false` в проде** — риск: два параллельных apply перезапишут state друг друга, и ресурсы «потеряются». Если state заблокирован — разобраться, кто его держит, а не отключать блокировку.
- **`-target=адрес`** применяет изменения только к указанному ресурсу (например, `-target='helm_release.nginx[0]'`). Нужен в авариях: срочно поправить один ресурс, не трогая остальное. Но Terraform при этом **не учитывает всё, что зависит от этого ресурса**, и state может разойтись с кодом. После `-target` обязательно выполнить полный `plan` и `apply`. Регулярно пользоваться `-target` — признак того, что state слишком большой и его пора делить.

### Drift — изменения в обход Terraform
Кто-то поменял Security Group вручную в консоли. Следующий `plan` покажет разницу и предложит вернуть всё как в коде. Варианты действий:
- изменение было ошибкой — применить, вернув состояние из кода;
- изменение нужно — **перенести его в код**, чтобы plan стал пустым.

Обнаруживать drift заранее: регулярный `plan` по расписанию в CI с уведомлением при непустом результате.

### 🧪 Мини-задание

**Задача.** Создай два файла через `local_file`, посмотри, что лежит в state. Затем «забудь» один ресурс через `terraform state rm`, а второй файл удали руками в обход Terraform (drift). Что покажет `plan`?

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > main.tf <<'EOF'
resource "local_file" "a" {
  filename = "${path.module}/a.txt"
  content  = "a\n"
}

resource "local_file" "b" {
  filename = "${path.module}/b.txt"
  content  = "b\n"
}
EOF
terraform init -no-color > /dev/null
terraform apply -auto-approve -no-color > /dev/null
terraform state list
terraform state rm local_file.b
ls *.txt
rm a.txt
terraform plan -no-color | grep -E 'will be created|^Plan'
```

Ожидаемый результат:

```text
local_file.a
local_file.b
Removed local_file.b
Successfully removed 1 resource instance(s).
a.txt
b.txt
  # local_file.a will be created
  # local_file.b will be created
Plan: 2 to add, 0 to change, 0 to destroy.
```

Разбор: `state rm` удалил запись только из state — файл `b.txt` остался на диске, но Terraform о нём больше не знает и хочет создать его заново (в облаке это был бы дубль или конфликт имён; правильный путь — `import`). `a.txt` удалили в обход Terraform: при `plan` провайдер обновил state по реальности, увидел, что файла нет, и запланировал создание. Так выглядит drift: реальность разошлась с state, и `plan` это показывает.

</details>

## Структура проекта и окружения

### Типичная структура
```
modules/                  ← переиспользуемые модули
  vpc/
    main.tf
    variables.tf
    outputs.tf
    versions.tf
    README.md
  eks/
envs/                     ← корневые конфигурации, по одной на окружение и стек
  dev/
    network/
      main.tf             ← вызывает modules/vpc с параметрами dev
      backend.tf          ← key = "dev/network/terraform.tfstate"
      terraform.tfvars
    eks/
  prod/
    network/
    eks/
```
**Root module** — папка, где запускаешь `terraform`. **Child modules** вызываются из неё через блок `module`.

### Как разделять state
Один огромный state на всё — плохо:
- `plan` идёт долго (refresh тысяч ресурсов);
- **большой радиус поражения**: ошибка в изменении DNS-записи может задеть базу данных;
- блокировка мешает параллельной работе команд.

Разбивать по **окружениям** (dev, stage, prod — всегда отдельно) и по **слоям**, которые меняются с разной частотой и имеют разную критичность: сеть → данные (RDS, S3) → вычисления (EKS) → приложения. Чем реже меняется слой и чем он критичнее, тем отдельнее.

### Как связать стеки между собой
EKS-стеку нужны ID подсетей из сетевого стека. Варианты:
- `data "terraform_remote_state"` — прочитать outputs другого state. Просто, но жёстко связывает стеки и требует доступа на чтение чужого state целиком (включая секреты).
- **Data sources по тегам**: `data "aws_subnets"` с фильтром по тегу. Связь через реальную инфраструктуру, без доступа к state.
- **SSM Parameter Store**: сетевой стек записывает ID в параметры, EKS-стек читает. Явный контракт между командами.

### Окружения: директории, workspaces или Terragrunt
- **Отдельные директории** на окружение (как выше) — самый явный и распространённый способ. Окружения могут отличаться, у каждого свой backend и свои права.
- **Workspaces** (`terraform workspace new prod`) — один код, несколько state. Подходит только для **одинаковых** окружений. Легко ошибиться и применить в чужом workspace, а различия между окружениями превращаются в `var.env == "prod" ? ... : ...` по всему коду.
- **Terragrunt** — обёртка над Terraform: убирает дублирование настроек backend и провайдеров, задаёт зависимости между стеками (`dependency`) и запускает их в правильном порядке.
- **Один код и файлы параметров на окружение** (частичная конфигурация backend) — см. ниже.

### Один код — много окружений: -backend-config и -var-file
Частый на практике вариант, когда окружения устроены одинаково, но должны иметь **отдельные state** и значения. Код один, а для каждого окружения и региона — папка с двумя файлами:
```
main.tf  variables.tf  versions.tf  backend.tf
env/
  test/eu-central-1/
    backend.tfvars      ← где лежит state этого окружения
    terraform.tfvars    ← значения переменных этого окружения
  prod/eu-central-1/
    backend.tfvars
    terraform.tfvars
```
В коде backend описан **частично** — без конкретных значений:
```hcl
# backend.tf
terraform {
  backend "s3" {}       # параметры придут из -backend-config при init
}
```
```hcl
# env/prod/eu-central-1/backend.tfvars
bucket       = "mycompany-tf-state-prod"
key          = "platform/eu-central-1/terraform.tfstate"
region       = "eu-central-1"
use_lockfile = true
```
```bash
terraform init -backend-config=./env/prod/eu-central-1/backend.tfvars -reconfigure
terraform plan  -var-file=./env/prod/eu-central-1/terraform.tfvars -out=tfplan
terraform apply tfplan
```
**`-reconfigure`** обязателен при переключении между окружениями в одной папке: без него Terraform помнит предыдущий backend (в `.terraform/`) и предложит **перенести state** из test в prod — так легко перезаписать прод-state тестовым. Ещё надёжнее — отдельная рабочая копия или чистая папка `.terraform` на каждое окружение (в CI это получается само собой).

Плюс подхода — нет дублирования кода и окружения гарантированно одинаковы. Минус — все окружения всегда на одной версии кода: чтобы выкатить изменение сначала в test, а потом в prod, нужны ветки, теги или флаги в переменных.

### Раскладка файлов в конфигурации
Terraform читает все `.tf`-файлы папки как один, имена файлов для него не важны. Но принятая раскладка помогает читать чужой код:
- `versions.tf` — `required_version` и `required_providers`;
- `providers.tf` — настройки провайдеров (регион, профиль, `default_tags`, alias);
- `backend.tf` — где хранится state;
- `variables.tf` — входные переменные с типами, описаниями и `validation`;
- `locals.tf` — вычисляемые значения (общие теги, префиксы имён);
- `data.tf` — источники данных (AMI, существующая VPC, зоны);
- `main.tf` или файлы по смыслу (`network.tf`, `eks.tf`, `iam.tf`) — ресурсы и модули;
- `outputs.tf` — выходные значения.

### Секреты окружений: только не в заметках и не в tfvars
Секретные переменные (client secret для SSO в ArgoCD и Grafana, токены, API-ключи) часто передают через `export TF_VAR_имя=...` перед запуском. Это рабочий механизм, но **значения нужно брать из менеджера секретов**, а не хранить в заметках, tfvars или истории shell:
```bash
export TF_VAR_grafana_client_secret="$(aws secretsmanager get-secret-value \
  --secret-id prod/grafana/sso --query SecretString --output text)"
```
Ещё лучше — читать секрет прямо в коде через `data "aws_secretsmanager_secret_version"` или `ephemeral`-ресурс, а в CI передавать через защищённые переменные. Помнить: значение всё равно окажется в state, если его записывают в атрибуты ресурса.

### 🧪 Мини-задание

**Задача.** Одна конфигурация — два окружения. Создай `dev.tfvars` (1 реплика) и `prod.tfvars` (3 реплики), примени dev в workspace `default`, затем создай workspace `prod` и примени prod. Посмотри, какие файлы получились и где лежит state каждого окружения.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > main.tf <<'EOF'
variable "replicas" {
  type = number
}

resource "local_file" "cfg" {
  filename = "${path.module}/${terraform.workspace}.conf"
  content  = "replicas=${var.replicas}\n"
}
EOF
echo 'replicas = 1' > dev.tfvars
echo 'replicas = 3' > prod.tfvars
terraform init -no-color > /dev/null
terraform apply -auto-approve -no-color -var-file=dev.tfvars | tail -1
terraform workspace new prod > /dev/null
terraform apply -auto-approve -no-color -var-file=prod.tfvars | tail -1
terraform workspace list
head *.conf
find . -name terraform.tfstate | sort
```

Ожидаемый результат:

```text
Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
  default
* prod

==> default.conf <==
replicas=1

==> prod.conf <==
replicas=3
./terraform.tfstate
./terraform.tfstate.d/prod/terraform.tfstate
```

Разбор: код один, а state у каждого workspace свой: `default` — в `terraform.tfstate`, остальные — в `terraform.tfstate.d/<имя>/` (в S3-бэкенде — под префиксом `env:/`). Поэтому второй `apply` не тронул dev, а создал новый ресурс. Звёздочка показывает текущий workspace — его легко перепутать, и `apply` уйдёт не туда. Поэтому для настоящих dev/prod чаще используют отдельные директории или `-backend-config` с разными ключами state, а workspaces — для временных копий.

</details>

## Модули

### Что такое модуль и зачем он нужен
Модуль — любая папка с `.tf`-файлами. Вызов модуля — способ переиспользовать код: «создай мне VPC по нашему стандарту» вместо копирования 200 строк в каждое окружение. Модули дают:
- **переиспользование** — одинаковая VPC в dev, stage и prod;
- **стандарты** — теги, шифрование, логирование включены в модуль, и их не забудут;
- **абстракцию** — команда приложения задаёт 5 параметров, а не разбирается в 30 ресурсах.

### Вызов модуля
```hcl
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"    # модуль из публичного Terraform Registry
  version = "~> 5.13"                           # ОБЯЗАТЕЛЬНО фиксировать версию

  name = local.name_prefix
  cidr = "10.0.0.0/16"
  azs  = local.azs
  private_subnets = [for i, az in local.azs : cidrsubnet("10.0.0.0/16", 4, i)]
  public_subnets  = [for i, az in local.azs : cidrsubnet("10.0.0.0/16", 8, i + 48)]

  enable_nat_gateway = true
  single_nat_gateway = var.env != "prod"       # в dev один NAT (дешевле), в prod — по одному на зону
}

module "eks" {
  source = "git::https://gitlab.com/mycompany/tf-modules.git//eks?ref=v2.3.0"
  # git-источник: // отделяет путь внутри репозитория, ?ref= — тег или коммит

  cluster_name = local.name_prefix
  vpc_id       = module.vpc.vpc_id              # outputs модуля: module.<имя>.<output>
  subnet_ids   = module.vpc.private_subnets

  providers = {
    aws = aws.us_east_1                         # передать модулю конкретный экземпляр провайдера
  }
}
```
Источники модулей: локальная папка (`./modules/vpc`), Git (с `?ref=`), Terraform Registry (публичный или приватный — в GitLab есть встроенный registry модулей), S3.

**Почему обязательно фиксировать версию.** Без `version` или `?ref=` при следующем `init` подтянется новая версия модуля, и plan вдруг захочет пересоздать половину инфраструктуры.

### Как правильно писать свои модули
**1. Модуль — это логическая единица, а не обёртка над одним ресурсом.** Модуль «s3-bucket», который просто пробрасывает все аргументы `aws_s3_bucket`, ничего не даёт — только лишний слой. Хороший модуль: «бакет для логов по стандарту компании» (шифрование, versioning, lifecycle-правила, блок публичного доступа, политика) или «сеть окружения».

**2. Продуманный интерфейс.**
```hcl
# modules/app-bucket/variables.tf
variable "name" {
  type        = string
  description = "Имя бакета (будет добавлен префикс окружения)"
}
variable "retention_days" {
  type        = number
  default     = 90
  description = "Через сколько дней удалять объекты"
  validation {
    condition     = var.retention_days >= 1
    error_message = "retention_days должен быть положительным."
  }
}
```
У каждой переменной есть тип, описание, валидация где нужно и разумный default. Outputs — всё, что может понадобиться снаружи (ID, ARN, имена).

**3. Не объявляй `provider` внутри модуля.** Только `required_providers` с версиями в `versions.tf`. Провайдеры передаются из корневого модуля. Иначе модуль нельзя использовать в другом регионе или аккаунте, а удаление модуля ломается (провайдер удаляется вместе с ресурсами, и Terraform не может их удалить).

**4. Не делай модуль «умным» сверх меры.** Десятки флагов `enable_this`, `create_that` и условий превращают модуль в нечитаемый комбайн. Лучше несколько маленьких модулей, которые собираются вместе (композиция).

**5. Версионирование.** Семантические версии в git-тегах: патч — исправления, минор — новые возможности без ломающих изменений, мажор — ломающие (переименование переменных, изменения, ведущие к пересозданию). CHANGELOG. README генерирует `terraform-docs`.

**6. Примеры и тесты.** Папка `examples/` с рабочими вызовами модуля. Тесты: встроенный `terraform test` (файлы `*.tftest.hcl`, с версии 1.6: описываешь входы и проверки, Terraform делает plan или apply и проверяет условия) или Terratest (библиотека на Go, которая реально создаёт ресурсы в облаке, проверяет их и удаляет).
```hcl
# tests/bucket.tftest.hcl
run "creates_bucket_with_versioning" {
  command = plan
  variables {
    name = "test"
  }
  assert {
    condition     = aws_s3_bucket_versioning.this.versioning_configuration[0].status == "Enabled"
    error_message = "versioning должен быть включён"
  }
}
```

### 🧪 Мини-задание

**Задача.** Напиши локальный модуль `modules/svc`, который создаёт конфиг сервиса из входов `name` и `port` и отдаёт путь к файлу через output. Вызови его для двух сервисов через `for_each` и посмотри адреса ресурсов в state и итоговый output.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
mkdir -p modules/svc
cat > modules/svc/main.tf <<'EOF'
variable "name" {
  type = string
}

variable "port" {
  type = number
}

resource "local_file" "this" {
  filename = "${path.root}/${var.name}.conf"
  content  = "name=${var.name}\nport=${var.port}\n"
}

output "path" {
  value = local_file.this.filename
}
EOF
cat > main.tf <<'EOF'
module "svc" {
  source   = "./modules/svc"
  for_each = { api = 8080, web = 80 }
  name     = each.key
  port     = each.value
}

output "configs" {
  value = { for k, m in module.svc : k => m.path }
}
EOF
terraform init -no-color > /dev/null
terraform apply -auto-approve -no-color > /dev/null
terraform state list
terraform output configs
```

Ожидаемый результат:

```text
module.svc["api"].local_file.this
module.svc["web"].local_file.this
{
  "api" = "./api.conf"
  "web" = "./web.conf"
}
```

Разбор: у ресурсов внутри модуля адрес `module.<имя>[ключ].<ресурс>` — именно его указывают в `state mv`, `import` и `-target`. `for_each` по map дал стабильные ключи `api` и `web`: если удалить `api`, `web` не пересоздастся (с `count` индексы сдвинулись бы). Модуль отдаёт наружу только то, что объявлено в `output`, а вызывающий код собирает выходы всех экземпляров в один map. `path.root` указывает на корень конфигурации, а не на папку модуля.

</details>

## Рефакторинг и импорт без пересоздания

### moved — переименовать или перенести ресурс
Переименовали ресурс в коде или перенесли его в модуль — Terraform видит «старый адрес исчез, новый появился» и планирует **удалить и создать заново**. Для базы данных это катастрофа. Блок `moved` говорит: «это тот же объект, просто другой адрес»:
```hcl
moved {
  from = aws_instance.web
  to   = aws_instance.frontend
}

moved {
  from = aws_db_instance.main
  to   = module.database.aws_db_instance.this    # перенос в модуль
}

moved {
  from = aws_iam_user.u[1]                        # переход с count на for_each
  to   = aws_iam_user.u["bob"]
}
```
После `apply` план покажет перемещение без изменений. Блок оставляют в коде на время (чтобы все окружения и копии state применили его), потом удаляют. Раньше то же делали командой `terraform state mv`, но блок `moved` лучше: изменение проходит через ревью и применяется во всех окружениях одинаково.

### import — взять под управление существующий ресурс
Ресурс создан вручную или другим инструментом, и его нужно перевести в Terraform:
```hcl
import {
  to = aws_s3_bucket.legacy
  id = "my-old-bucket"                 # идентификатор в облаке (у каждого типа ресурса свой формат)
}
```
```bash
terraform plan -generate-config-out=generated.tf   # Terraform сам сгенерирует код ресурса по реальному объекту
```
Затем почистить сгенерированный код, добиться **пустого plan** (код полностью совпадает с реальностью) и применить. Старая команда `terraform import <адрес> <id>` делает то же без ревью.

### removed — перестать управлять ресурсом, не удаляя
```hcl
removed {
  from = aws_s3_bucket.archive
  lifecycle {
    destroy = false                    # убрать из state, но объект в AWS оставить
  }
}
```

### 🧪 Мини-задание

**Задача.** Переименуй ресурс `local_file.old` в `local_file.app`. Сначала просто поменяй имя и посмотри план — Terraform захочет удалить и создать. Потом добавь блок `moved` и убедись, что план пустой, а адрес в state поменялся.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
cat > main.tf <<'EOF'
resource "local_file" "old" {
  filename = "${path.module}/app.conf"
  content  = "v1\n"
}
EOF
terraform init -no-color > /dev/null
terraform apply -auto-approve -no-color > /dev/null
sed -i.bak 's/"old"/"app"/' main.tf
terraform plan -no-color | grep '^Plan'
cat >> main.tf <<'EOF'

moved {
  from = local_file.old
  to   = local_file.app
}
EOF
terraform plan -no-color | grep -E 'has moved|^Plan'
terraform apply -auto-approve -no-color | tail -1
terraform state list
```

Ожидаемый результат:

```text
Plan: 1 to add, 0 to change, 1 to destroy.
  # local_file.old has moved to local_file.app
Plan: 0 to add, 0 to change, 0 to destroy.
Apply complete! Resources: 0 added, 0 changed, 0 destroyed.
local_file.app
```

Разбор: для Terraform ресурс определяется адресом. Без `moved` новое имя — новый ресурс, старый нужно удалить: в облаке это пересоздание базы или кластера. Блок `moved` говорит «это тот же объект под новым адресом», и план пустой, меняется только запись в state. В отличие от `terraform state mv`, `moved` лежит в коде, проходит ревью и применяется в CI для всех окружений. После того как изменение применено везде, блок можно удалить.

</details>

## CI/CD для Terraform

### Типичный пайплайн
1. **Merge request:**
   - `terraform fmt -check` — код отформатирован;
   - `terraform validate`;
   - **tflint** — линтер: неверные типы инстансов, устаревший синтаксис, соглашения об именах;
   - **trivy config** / **checkov** — проверка безопасности: открытые Security Group, нешифрованные диски, публичные бакеты;
   - `terraform plan -out=tfplan` — **план публикуется комментарием в MR**, ревьюер видит, что изменится в инфраструктуре, а не только код.
2. **После merge в main:** `terraform apply tfplan`. Для prod — с ручным подтверждением (protected environment — окружение в GitLab или GitHub, деплой в которое разрешён только определённым людям и требует нажать кнопку).

Готовые решения: Atlantis (бот, который запускает plan и apply по командам в комментариях MR), Spacelift, env0, HCP Terraform (облачные платформы для запуска Terraform с политиками, историей и хранением state).

### pre-commit
Проверки до коммита на машине разработчика (pre-commit — фреймворк git-хуков: перед каждым `git commit` автоматически запускает заданные проверки и не даёт закоммитить, если они упали): `terraform_fmt`, `terraform_validate`, `terraform_tflint`, `terraform_docs` (обновить README модуля), `terraform_trivy`. Ловят ошибки до того, как они попадут в MR.

### Доступ CI в AWS без ключей
Не хранить `AWS_ACCESS_KEY_ID` в переменных CI. Использовать **OIDC**: GitLab или GitHub выдаёт задаче подписанный токен, AWS проверяет его и выдаёт временные креды роли. В trust policy роли ограничивают, какой проект, ветка или окружение могут её получить. Разделение прав: роль для `plan` (только чтение) доступна во всех MR, роль для `apply` (запись) — только для main и protected environment.

### 🧪 Мини-задание

**Задача.** Повтори шаги типичного пайплайна и посмотри на коды возврата, по которым CI принимает решения: `fmt -check` на плохо отформатированном файле и после `fmt`, `validate`, затем `plan -detailed-exitcode` до и после `apply` сохранённого плана.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
printf 'resource "local_file" "x" {\nfilename="x.txt"\n    content = "x"\n}\n' > main.tf
terraform fmt -check; echo "fmt -check exit=$?"
terraform fmt; terraform fmt -check; echo "after fmt exit=$?"
terraform init -no-color > /dev/null
terraform validate -no-color
terraform plan -detailed-exitcode -no-color -out=tfplan > /dev/null; echo "plan exit=$?"
terraform apply -no-color tfplan > /dev/null
terraform plan -detailed-exitcode -no-color > /dev/null; echo "plan exit=$?"
```

Ожидаемый результат:

```text
main.tf
fmt -check exit=3
main.tf
after fmt exit=0
Success! The configuration is valid.
plan exit=2
plan exit=0
```

Разбор: `fmt -check` ничего не меняет, а печатает файлы с неправильным форматированием и возвращает ненулевой код — job линтера падает. `fmt` исправил файл (и тоже напечатал его имя). У `plan -detailed-exitcode` три кода: 0 — изменений нет, 1 — ошибка, 2 — есть изменения. По коду 2 пайплайн решает, нужен ли шаг apply, а ночной job ловит drift. `-out=tfplan` сохраняет план, и `apply tfplan` применяет ровно то, что прошло ревью, а не новый план.

</details>

## Terraform и OpenTofu
В 2023 году HashiCorp сменила лицензию Terraform с открытой MPL (Mozilla Public License — свободная лицензия) на BSL (ограничение на конкурирующие коммерческие продукты). Сообщество сделало форк **OpenTofu** под Linux Foundation с открытой лицензией. Синтаксис и провайдеры совместимы, у OpenTofu есть свои возможности (например, встроенное шифрование state). Для большинства компаний разница пока небольшая, но вопрос задают, чтобы проверить, что ты следишь за экосистемой.

## Вопросы с собеседований

**Что такое state и почему его нельзя терять и хранить в git?**
State связывает ресурсы в коде с реальными объектами в облаке. Без него Terraform не знает, что уже создано, и попытается создать всё заново, а удалённые из кода ресурсы не удалит. В git его не хранят: внутри секреты открытым текстом, нет блокировки от одновременных изменений, постоянные конфликты слияния. Правильно — remote backend с шифрованием, versioning и блокировкой.

**Кто-то удалил ресурс руками в консоли. Что покажет plan?**
При refresh Terraform обнаружит, что объекта нет, уберёт его из state и предложит создать заново.

**Кто-то изменил ресурс руками. Что делать?**
`plan` (или `plan -refresh-only`) покажет drift. Если изменение было ошибкой — `apply` вернёт состояние из кода. Если оно нужно — перенести изменение в код, чтобы план стал пустым. Профилактика — ограничить права на ручные изменения и запускать plan по расписанию.

**Как перевести существующий ресурс под управление Terraform?**
Блок `import` с адресом и ID ресурса, `terraform plan -generate-config-out` для генерации кода, доработать код до пустого плана, `apply`.

**Как переименовать ресурс или перенести его в модуль, не пересоздавая?**
Блок `moved { from = ... to = ... }` (или `terraform state mv`). Без этого Terraform удалит старый и создаст новый.

**State заблокирован, а пайплайн упал. Что делать?**
Убедиться, что никто действительно не выполняет apply (проверить запущенные задачи CI), и снять блокировку: `terraform force-unlock <LOCK_ID>`.

**Как работать с секретами в Terraform?**
Не хранить в tfvars в git. Читать из Secrets Manager или SSM через data source, передавать через защищённые переменные CI (`TF_VAR_*`). Помнить, что `sensitive` только скрывает вывод, а в state значение открыто: шифровать state и ограничивать к нему доступ. Лучше, чтобы секрет не попадал в state вовсе (пароль генерирует и хранит AWS, ephemeral-ресурсы).

**Чем count отличается от for_each?**
count адресует экземпляры по индексу: удаление элемента из середины списка сдвигает индексы и вызывает пересоздание следующих ресурсов. for_each адресует по ключу: изменения затрагивают только нужный экземпляр. count — для «создать или нет», for_each — для коллекций.

**Как организовать несколько окружений?**
Отдельные директории с отдельными state и backend на каждое окружение, общий код — в версионированных модулях. Workspaces — только для действительно одинаковых окружений. Terragrunt — чтобы убрать дублирование и связать стеки.

**Plan хочет пересоздать ресурс. Почему и что делать?**
Изменился атрибут, который нельзя менять на месте (в плане он помечен `forces replacement`: имя, зона, движок БД); изменился ключ for_each или индекс count; изменилось поведение после обновления провайдера. Решения: вернуть значение, `moved` при переименовании, `ignore_changes` если атрибут меняется извне, `create_before_destroy` чтобы замена прошла без простоя, `prevent_destroy` как защита.

**Что такое provisioners и почему их стараются не использовать?**
`local-exec` и `remote-exec` выполняют команды при создании ресурса. Их результат не отслеживается в state, они не идемпотентны, при ошибке ресурс помечается tainted, и о повторном выполнении Terraform не знает. Лучше: `user_data` или cloud-init, готовые образы (Packer), Ansible после создания.

**Чем Terraform отличается от CloudFormation, CDK, Pulumi и Ansible?**
Terraform — множество провайдеров для любых систем, явный state, язык HCL. CloudFormation — только AWS, state хранит AWS, автоматический откат при ошибке. CDK и Pulumi — инфраструктура на обычных языках программирования (CDK генерирует CloudFormation). Ansible — в первую очередь настройка операционных систем и приложений (последовательные задачи), а не управление жизненным циклом облачных ресурсов.

**Что такое OpenTofu?**
Форк Terraform под открытой лицензией после перехода Terraform на BSL в 2023 году. Совместим по синтаксису и провайдерам.

**Как ускорить plan на большом проекте?**
Разделить state на меньшие стеки, уменьшить количество data sources, использовать кэш провайдеров (`TF_PLUGIN_CACHE_DIR`), увеличить `-parallelism`. В исключительных случаях — `-refresh=false` или `-target` (только для аварийных ситуаций: они нарушают целостность плана).

**Зачем нужен `.terraform.lock.hcl` и нужно ли его коммитить?**
Фиксирует точные версии и хеши провайдеров, чтобы у всех и в CI использовалась одна версия. Да, коммитить. Обновлять осознанно через `terraform init -upgrade`.

**Как устроить CI/CD для Terraform?**
В MR: fmt, validate, tflint, сканер безопасности, plan с публикацией результата в MR. После merge — apply сохранённого плана, prod с ручным подтверждением. Доступ в AWS через OIDC с разными ролями для plan и apply. Блокировка state, регулярный plan для обнаружения drift.

**Что делать, если apply упал посередине?**
Ресурсы, которые успели создаться, уже записаны в state. Нужно исправить причину ошибки (лимиты, права, неверный параметр) и снова выполнить plan и apply: Terraform продолжит с того места. Если ресурс создался в облаке, но не попал в state (редкий случай при обрыве) — импортировать его.

**Зачем `-reconfigure` при `terraform init` и что будет без него?**
При частичной конфигурации backend (`backend "s3" {}` и разные `-backend-config` на окружение) Terraform запоминает последний backend в папке `.terraform`. Если переключиться на другое окружение без `-reconfigure`, он решит, что backend «переехал», и предложит скопировать state — так можно перезаписать state prod тестовым. `-reconfigure` говорит: забыть предыдущий backend и подключиться к новому, ничего не перенося.

**Когда допустимы `-target` и `-lock=false`?**
Только в исключительных ситуациях. `-target` — аварийно поправить один ресурс, после чего обязательно полный plan и apply. `-lock=false` — только для чтения (plan), когда точно никто не применяет изменения; apply без блокировки грозит испорченным state.
