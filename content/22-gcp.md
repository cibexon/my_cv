# GCP (Google Cloud)

Основы Google Cloud Platform для DevOps/SRE, который уже знает AWS: чем GCP устроен иначе, какие сервисы чему соответствуют и что спрашивают на собеседованиях. Каждая тема: что это, зачем, как работает, пример и типичные ловушки. Мини-задания требуют проекта GCP: при регистрации Google даёт пробный период — $300 кредитов на 90 дней, а некоторые ресурсы (например, одна маленькая VM e2-micro в регионах США) бесплатны постоянно (Free Tier).

## GCP глазами AWS-инженера: карта сервисов

Концепции у облаков общие, отличаются названия и несколько принципиальных деталей. Таблица соответствий — первое, что стоит выучить: на собеседовании нормально сказать «я работал с AWS ALB, в GCP аналог — Application Load Balancer, отличие в том, что…».

| Задача | AWS | GCP |
|---|---|---|
| Изоляция ресурсов и счёта | Account, AWS Organizations, OU (Organizational Unit, папка аккаунтов) | **Project**, Organization, Folder |
| Управление доступом | IAM users, roles, policies | IAM: principals, roles, **allow policies на ресурсе**, service accounts |
| Виртуальные машины | EC2 | **Compute Engine** (GCE) |
| Группа VM с автомасштабированием | Auto Scaling Group + Launch Template | **Managed Instance Group (MIG)** + Instance Template |
| Kubernetes | EKS | **GKE** (Google Kubernetes Engine) |
| Контейнеры без серверов | Fargate, App Runner | **Cloud Run** |
| Функции | Lambda | **Cloud Run functions** (раньше Cloud Functions) |
| Сеть | VPC (региональная) | **VPC (глобальная)** |
| Правила доступа к VM | Security Group | **VPC firewall rules** (по сетевым тегам или сервисным аккаунтам) |
| NAT для приватных VM | NAT Gateway | **Cloud NAT** |
| Балансировщики | ALB, NLB | **Application LB, Network LB** |
| DNS | Route 53 | **Cloud DNS** |
| CDN (сеть кэширующих серверов рядом с пользователями) и WAF (фильтр вредных HTTP-запросов) | CloudFront, AWS WAF, Shield | **Cloud CDN, Cloud Armor** |
| Объектное хранилище | S3 | **Cloud Storage** (GCS, Google Cloud Storage) |
| Диски VM | EBS | **Persistent Disk, Hyperdisk** |
| Управляемая реляционная БД | RDS, Aurora | **Cloud SQL, AlloyDB** |
| NoSQL | DynamoDB | **Firestore, Bigtable** |
| Кэш | ElastiCache | **Memorystore** |
| Хранилище данных для аналитики | Redshift, Athena | **BigQuery** |
| Очереди и события | SQS, SNS, EventBridge | **Pub/Sub, Eventarc** |
| Секреты и ключи | Secrets Manager, KMS | **Secret Manager, Cloud KMS** |
| Логи и метрики | CloudWatch | **Cloud Logging, Cloud Monitoring** |
| Аудит действий | CloudTrail | **Cloud Audit Logs** |
| Реестр образов | ECR | **Artifact Registry** |
| CI/CD | CodeBuild, CodePipeline | **Cloud Build, Cloud Deploy** |
| Ограничения на уровне организации | SCP (Service Control Policies) | **Organization Policies** |

Главные отличия, которые стоит назвать на собеседовании:
- **VPC глобальная**, а подсети региональные и охватывают все зоны региона. В AWS VPC живёт в одном регионе, а подсеть — в одной зоне доступности.
- **Проект — основная единица изоляции**, как аккаунт в AWS, но создаётся за секунды, и их обычно много: по проекту на окружение или команду.
- **Права выдаются на ресурс** (проект, бакет, топик), а не описываются политикой у пользователя. В AWS чаще пишут политику и вешают её на роль.
- **Firewall rules вместо Security Groups**: правила задаются на уровне сети и применяются к VM по сетевым тегам или сервисному аккаунту.
- **Глобальный балансировщик с одним anycast IP** (один адрес анонсируется из многих точек мира, и пользователь попадает в ближайшую) — в AWS для этого нужны Global Accelerator или CloudFront.
- **Live migration**: при обслуживании железа Google переносит работающую VM на другой хост без перезагрузки. В AWS приходят письма «scheduled retirement», и инстанс нужно перезапустить.

### 🧪 Мини-задание

**Задача.** Установи gcloud CLI (утилита командной строки GCP) и выведи список регионов вместе с числом зон в каждом, чтобы увидеть, как называются регионы и зоны.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
brew install --cask google-cloud-sdk
gcloud init
gcloud compute zones list --format="value(region.basename())" | sort | uniq -c | head -5
```

Ожидаемый результат:

```text
   3 africa-south1
   3 asia-east1
   3 asia-east2
   3 asia-northeast1
   3 asia-northeast2
```

Разбор: `gcloud init` открывает браузер для входа в аккаунт Google и спрашивает проект и регион по умолчанию. Регионы называются `europe-west3` (Франкфурт), `us-central1` (Айова), а зоны — регион плюс буква: `europe-west3-a`. В большинстве регионов три зоны, как и у AWS. `--format="value(...)"` — встроенное форматирование gcloud: выводит только нужные поля без заголовков, `basename()` отрезает от ссылки на регион всё, кроме имени. Нужен проект с включённым Compute Engine API, иначе gcloud предложит его включить. Список регионов у тебя будет актуальным на момент запуска.

</details>

## Иерархия ресурсов: организация, папки, проекты

```
Organization (example.com)            ← привязана к домену Google Workspace или Cloud Identity
├── Folder: prod
│   ├── Project: shop-prod            ← ресурсы: VM, бакеты, кластеры
│   └── Project: shop-prod-data
├── Folder: nonprod
│   ├── Project: shop-dev
│   └── Project: shop-stage
└── Project: shared-network           ← общая сеть (Shared VPC) для всех проектов
Billing Account ── оплачивает ── проекты (один счёт может платить за много проектов)
```

- **Organization (организация)** — корень иерархии. Появляется, когда компания подключает свой домен через Google Workspace (почта, документы) или Cloud Identity (бесплатный каталог пользователей без почты). Без организации проекты существуют сами по себе — так обычно у личных аккаунтов.
- **Folder (папка)** — группировка проектов по окружению, отделу или продукту. Папки могут быть вложенными.
- **Project (проект)** — контейнер для ресурсов, граница для API, квот и счёта. У проекта три идентификатора:
  - **Project ID** (`shop-prod-4821`) — глобально уникальный, задаётся при создании и **больше не меняется**; его пишут в командах и Terraform;
  - **Project name** — человекочитаемое имя, можно менять;
  - **Project number** (`123456789012`) — числовой, выдаёт Google; встречается в именах сервисных аккаунтов и в ошибках.
- **Billing Account (платёжный аккаунт)** — отдельная сущность с картой или договором; к нему привязывают проекты. Если отвязать — ресурсы в проекте остановятся.

Почему иерархия важна: **права и политики наследуются вниз**. Роль, выданная на папку `prod`, действует во всех её проектах. Так же наследуются **Organization Policies** — ограничения, которые нельзя обойти даже владельцем проекта, аналог SCP в AWS. Примеры:
- `constraints/gcp.resourceLocations` — разрешить ресурсы только в регионах ЕС (требования GDPR — европейского закона о защите персональных данных);
- `constraints/iam.disableServiceAccountKeyCreation` — запретить создавать ключи сервисных аккаунтов;
- `constraints/compute.vmExternalIpAccess` — запретить VM с публичными IP.

**API нужно включать в каждом проекте.** В новом проекте почти все сервисы выключены: прежде чем создать VM, включают Compute Engine API (`gcloud services enable compute.googleapis.com`). Типичная ошибка новичка в Terraform — `Error 403: Compute Engine API has not been used in project … before or it is disabled`.

### 🧪 Мини-задание

**Задача.** Создай проект для экспериментов, сделай его проектом по умолчанию, включи в нём Compute Engine API и посмотри, какие API включены.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком (ID проекта должен быть глобально уникальным, поэтому добавляем случайный суффикс):

```bash
PROJECT_ID="sandbox-$RANDOM$RANDOM"
gcloud projects create "$PROJECT_ID" --name="Sandbox"
gcloud config set project "$PROJECT_ID"
gcloud billing accounts list --format="value(name)" | head -1 | xargs -I{} gcloud billing projects link "$PROJECT_ID" --billing-account={}
gcloud services enable compute.googleapis.com
gcloud services list --enabled --format="value(config.name)" | grep -E 'compute|oslogin'
```

Ожидаемый результат:

```text
Create in progress for [https://cloudresourcemanager.googleapis.com/v1/projects/sandbox-1234567890].
Waiting for [operations/cp.1234567890] to finish...done.
Updated property [core/project].
billingAccountName: billingAccounts/0X0X0X-0X0X0X-0X0X0X
billingEnabled: true
compute.googleapis.com
oslogin.googleapis.com
```

Разбор: проект создаётся за секунды — в отличие от аккаунта AWS, его не жалко создать под эксперимент и удалить (`gcloud projects delete`, после этого он 30 дней лежит в корзине). Без привязки к платёжному аккаунту платные API не включатся. Вместе с Compute Engine автоматически включаются зависимые API — здесь OS Login (вход на VM по IAM-правам). ID и номера у тебя будут свои.

</details>

## IAM: кто и что может делать

### Основные понятия

- **Principal (принципал)** — тот, кому выдают доступ: аккаунт Google (`user:anna@example.com`), группа (`group:devops@example.com`), сервисный аккаунт, домен целиком или внешняя идентичность через Workload Identity Federation (о ней ниже).
- **Role (роль)** — набор разрешений (permissions) вида `compute.instances.start`. Отдельные разрешения напрямую выдать нельзя — только в составе роли. Три вида ролей:
  - **Basic** (базовые): `roles/owner`, `roles/editor`, `roles/viewer`. Слишком широкие — `editor` может менять почти всё в проекте. В проде их избегают;
  - **Predefined** (готовые от Google): `roles/compute.instanceAdmin.v1`, `roles/storage.objectViewer`, `roles/container.developer`. Используются чаще всего;
  - **Custom** (свои): собранные из нужных разрешений, если готовой роли не хватает или она слишком широкая.
- **Allow policy (политика доступа)** — привязана к **ресурсу** (организации, папке, проекту, бакету, топику) и состоит из **bindings** (привязок): «роль X — таким-то принципалам». Это главное отличие от AWS: там политика обычно висит на пользователе или роли и перечисляет ресурсы, а в GCP ресурс сам хранит, кто к нему имеет доступ.
- **Наследование**: итоговые права = объединение привязок на ресурсе и на всех его родителях. Роль на проекте нельзя «отнять» на бакете внутри него — для этого есть отдельные **deny policies** (запрещающие политики, они важнее разрешающих), но используют их редко.
- **IAM Conditions** — условие у привязки: например, доступ только до определённой даты или только к ресурсам с определённым именем.

```bash
# выдать группе право читать объекты в одном бакете
gcloud storage buckets add-iam-policy-binding gs://shop-prod-assets \
  --member="group:frontend@example.com" --role="roles/storage.objectViewer"
# кто имеет доступ к проекту
gcloud projects get-iam-policy shop-prod --format=json
```

### Сервисные аккаунты

**Service account (сервисный аккаунт, SA)** — идентичность для программ, а не людей: `deployer@shop-prod.iam.gserviceaccount.com`. Аналог IAM-роли в AWS, которую принимает EC2 или под.

Как программа получает права SA, от лучшего к худшему:
1. **Привязать SA к ресурсу**: VM, Cloud Run, Cloud Build запускаются «от имени» SA. Код получает короткоживущие токены у **metadata server** (служебный адрес `169.254.169.254` внутри VM, как IMDS — Instance Metadata Service — в AWS) — ключей нет вообще.
2. **Workload Identity Federation (федерация идентичностей)** — для внешних систем: GitHub Actions, GitLab CI, AWS. Внешняя система предъявляет свой OIDC-токен (OpenID Connect — стандарт, по которому сервис выдаёт подписанный токен «я — пайплайн такого-то репозитория»), GCP обменивает его на временный токен GCP. Ключей тоже нет. Аналог в AWS — IAM-роль с OIDC-провайдером для GitHub.
3. **Impersonation (олицетворение)**: человек с ролью `roles/iam.serviceAccountTokenCreator` временно действует от имени SA: `gcloud ... --impersonate-service-account=deployer@...`. Удобно, чтобы проверить права SA или запускать Terraform без личных широких прав.
4. **JSON-ключ SA** — файл с приватным ключом, который действует, пока его не удалят. Его можно украсть из репозитория или CI. Это **худший вариант**: в организациях создание ключей часто запрещено Organization Policy.

Ловушка: **Compute Engine default service account** (`PROJECT_NUMBER-compute@developer.gserviceaccount.com`) — его автоматически получают VM, если не указать другой SA. В старых проектах у него роль `editor` на весь проект: взломали одну VM — получили права менять почти всё. Правильно — для каждого приложения отдельный SA с минимальными ролями.

### Как войти самому: два вида учётных данных

- `gcloud auth login` — учётные данные для **самой утилиты gcloud**.
- `gcloud auth application-default login` — **ADC** (Application Default Credentials, учётные данные по умолчанию для приложений): их подхватывают Terraform, SDK на Python или Go. На VM и в Cloud Run ADC берутся из metadata server автоматически.

Частая путаница: gcloud работает, а Terraform пишет «could not find default credentials» — значит, выполнили только первую команду.

### 🧪 Мини-задание

**Задача.** Создай сервисный аккаунт `reader`, выдай ему роль только на просмотр VM в проекте и проверь через impersonation, что он может читать список VM, но не может создавать бакеты.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
PROJECT_ID=$(gcloud config get-value project)
ME=$(gcloud config get-value account)
gcloud iam service-accounts create reader --display-name="Read-only VMs"
SA="reader@$PROJECT_ID.iam.gserviceaccount.com"
gcloud projects add-iam-policy-binding "$PROJECT_ID" --member="serviceAccount:$SA" --role="roles/compute.viewer" --condition=None >/dev/null
gcloud iam service-accounts add-iam-policy-binding "$SA" --member="user:$ME" --role="roles/iam.serviceAccountTokenCreator" >/dev/null
sleep 60
gcloud compute instances list --impersonate-service-account="$SA"
gcloud storage buckets create "gs://$PROJECT_ID-test" --impersonate-service-account="$SA" 2>&1 | tail -1
```

Ожидаемый результат:

```text
WARNING: This command is using service account impersonation. All API calls will be executed as [reader@sandbox-1234567890.iam.gserviceaccount.com].
Listed 0 items.
ERROR: (gcloud.storage.buckets.create) HTTPError 403: reader@sandbox-1234567890.iam.gserviceaccount.com does not have storage.buckets.create access to the Google Cloud project.
```

Разбор: роль `roles/compute.viewer` дала разрешение `compute.instances.list`, поэтому список прочитан (VM пока нет). Разрешения `storage.buckets.create` в ней нет — получаем 403, и в тексте ошибки GCP прямо пишет, какого разрешения не хватает: это удобно при отладке. Чтобы действовать от имени SA, твоему пользователю нужна роль `serviceAccountTokenCreator` на этом SA. `sleep 60` — изменения IAM применяются не мгновенно, обычно за минуту. Ключ SA ни разу не создавался. Имена у тебя будут свои.

</details>

## Сеть: VPC

### Устройство VPC

- **VPC (Virtual Private Cloud, виртуальная частная сеть) — глобальный ресурс.** Одна сеть может иметь подсети в Европе и США, и VM в них общаются по внутренним IP через сеть Google без VPN и пиринга. В AWS для этого понадобились бы две VPC и peering или Transit Gateway.
- **Подсеть (subnet) — региональная** и охватывает все зоны региона. Не нужно делать «подсеть на каждую AZ», как в AWS: одна подсеть `europe-west3` — и VM в зонах `-a`, `-b`, `-c`.
- **Режимы VPC:**
  - **auto mode** — GCP сам создаёт по подсети `/20` в каждом регионе из диапазона `10.128.0.0/9`. Так устроена сеть `default`, которая появляется в новом проекте;
  - **custom mode** — подсети создаёшь сам. В проде используют только его: диапазоны auto mode пересекаются между проектами, и их потом не соединить пирингом или VPN.
- **Secondary ranges (дополнительные диапазоны подсети)** — для IP подов и сервисов GKE (alias IP, об этом в разделе GKE).
- **Маршруты:** в каждой VPC есть маршрут по умолчанию `0.0.0.0/0` на default internet gateway. Отдельного объекта Internet Gateway, как в AWS, нет — VM с внешним IP сразу ходит в интернет.
- **Публичных и приватных подсетей в смысле AWS тоже нет**: «публичность» определяется тем, есть ли у самой VM внешний IP.

### Firewall rules (правила файрвола)

Аналог Security Group, но устроены иначе:
- задаются **на уровне VPC**, а не на сетевом интерфейсе;
- применяются к VM по **target** (цели): ко всем VM сети, к VM с **сетевым тегом** (`network tag` — метка `web` или `ssh` на VM) или к VM с определённым **сервисным аккаунтом** (надёжнее: тег может поставить любой, у кого есть права на VM, а SA выдаётся через IAM);
- **stateful** (с отслеживанием состояния): ответный трафик на разрешённое соединение пропускается автоматически, как в Security Group;
- есть **разрешающие и запрещающие** правила, и у каждого **priority** (приоритет) от 0 до 65535 — **меньшее число важнее**. Правило с приоритетом 100 сработает раньше правила с 1000;
- **неявные правила** (implied rules) с самым низким приоритетом 65535: **весь входящий трафик запрещён, весь исходящий разрешён**.

```bash
# пустить HTTP/HTTPS только на VM с тегом web
gcloud compute firewall-rules create allow-web \
  --network=prod-vpc --direction=INGRESS --action=ALLOW \
  --rules=tcp:80,tcp:443 --source-ranges=0.0.0.0/0 --target-tags=web --priority=1000
```

В сети `default` уже есть правила `default-allow-ssh` и `default-allow-rdp` **из всего интернета** — одна из причин не использовать её в проде.

Кроме обычных правил есть **firewall policies** (политики файрвола) — наборы правил, привязанные к организации, папке или сети. Ими безопасники задают общие запреты сразу для всех проектов.

### Доступ в интернет и к API Google без внешнего IP

- **Cloud NAT** — управляемый NAT (Network Address Translation, подмена внутреннего адреса на внешний) для VM без внешнего IP, аналог NAT Gateway. Это не отдельная VM, а настройка, привязанная к **Cloud Router** (управляемый маршрутизатор GCP, который также обменивается маршрутами по BGP — протоколу обмена маршрутами между сетями — с VPN и Interconnect). Один Cloud NAT обслуживает весь регион, а не одну зону.
- **Private Google Access** — флаг подсети: VM без внешнего IP ходят к API Google (Cloud Storage, Artifact Registry, BigQuery) по внутренней сети. Аналог VPC endpoints в AWS, только включается одной галочкой.
- **IAP TCP forwarding** (Identity-Aware Proxy — прокси Google, который пускает к ресурсу только после проверки IAM): SSH на VM без внешнего IP и без бастиона: `gcloud compute ssh vm1 --tunnel-through-iap`. Нужно правило файрвола, пропускающее TCP 22 из диапазона IAP `35.235.240.0/20`. Аналог в AWS — SSM Session Manager.

### Связь VPC с другими сетями

- **VPC Network Peering** — соединяет две VPC (в том числе разных проектов и организаций). **Не транзитивен**: если A соединён с B, а B с C, то A не видит C. Диапазоны не должны пересекаться.
- **Shared VPC** — одна сеть в **host project** (проект-хозяин сети), которой пользуются **service projects** (проекты приложений). Сетевая команда управляет подсетями и файрволом централизованно, а команды приложений создают VM в «чужих» подсетях. Аналог — VPC sharing через AWS RAM (Resource Access Manager). Очень частая схема в компаниях.
- **Cloud VPN** — IPsec-туннели (зашифрованные туннели поверх интернета) в дата-центр или другое облако. **HA VPN** — два интерфейса с SLA (Service Level Agreement, гарантированная доступность) 99,99%.
- **Cloud Interconnect** — выделенный физический канал: **Dedicated** (свой кабель в точке присутствия Google, 10 или 100 Гбит/с) или **Partner** (через провайдера). Аналог — AWS Direct Connect.
- **Private Service Connect** — приватный доступ к сервису другого проекта или к API Google через внутренний IP в своей сети. Аналог — AWS PrivateLink.

### 🧪 Мини-задание

**Задача.** Создай custom-mode VPC с одной подсетью во Франкфурте, правило файрвола для SSH только через IAP и Cloud NAT для выхода в интернет. Выведи правила и подсети сети.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
gcloud compute networks create lab-vpc --subnet-mode=custom
gcloud compute networks subnets create lab-euw3 --network=lab-vpc \
  --region=europe-west3 --range=10.10.0.0/24 --enable-private-ip-google-access
gcloud compute firewall-rules create lab-allow-iap-ssh --network=lab-vpc \
  --direction=INGRESS --action=ALLOW --rules=tcp:22 --source-ranges=35.235.240.0/20
gcloud compute routers create lab-router --network=lab-vpc --region=europe-west3
gcloud compute routers nats create lab-nat --router=lab-router --region=europe-west3 \
  --auto-allocate-nat-external-ips --nat-all-subnet-ip-ranges
gcloud compute networks subnets list --network=lab-vpc --format="table(name, region.basename(), ipCidrRange, privateIpGoogleAccess)"
gcloud compute firewall-rules list --filter="network:lab-vpc" --format="table(name, direction, priority, sourceRanges.list(), allowed[].map().firewall_rule().list())"
```

Ожидаемый результат:

```text
NAME      REGION        RANGE         PRIVATE_IP_GOOGLE_ACCESS
lab-euw3  europe-west3  10.10.0.0/24  True
NAME               DIRECTION  PRIORITY  SRC_RANGES       ALLOW
lab-allow-iap-ssh  INGRESS    1000      35.235.240.0/20  tcp:22
```

Разбор: сеть глобальная, а подсеть привязана к региону и охватывает все три зоны Франкфурта. SSH открыт только для IAP — снаружи на 22 порт не попасть, войти можно `gcloud compute ssh ... --tunnel-through-iap`. Остальной входящий трафик закрыт неявным правилом с приоритетом 65535, которое в списке не показывается. Cloud NAT даёт VM без внешнего IP выход в интернет для `apt update`, а Private Google Access — доступ к API Google без NAT. Cloud NAT платный, поэтому после экспериментов удали его: `gcloud compute routers delete lab-router --region=europe-west3`.

</details>

## Балансировщики и DNS

### Виды балансировщиков

GCP называет балансировщики по уровню и охвату:

| Балансировщик | Уровень | Охват | Аналог в AWS | Когда брать |
|---|---|---|---|---|
| **Global external Application LB** | L7 (HTTP/HTTPS) | глобальный, один anycast IP | CloudFront + ALB / Global Accelerator | публичные сайты и API для пользователей по всему миру |
| **Regional external/internal Application LB** | L7 | один регион | ALB | когда данные не должны покидать регион или нужен внутренний HTTP-балансировщик |
| **Proxy Network LB** | L4 (TCP, с TLS-терминацией) | глобальный или региональный | NLB с TLS | TCP-сервисы, которым нужен глобальный IP |
| **Passthrough Network LB** | L4 (TCP/UDP) | региональный | NLB | когда VM должна видеть исходный IP клиента и сама обрабатывать соединение (игры, VPN, свой HAProxy) |

L7 и L4 — уровни модели OSI: L7 понимает HTTP (пути, заголовки, cookie), L4 видит только IP, порты и TCP/UDP. Proxy-балансировщик сам завершает соединение клиента и открывает новое к бэкенду; passthrough пропускает пакеты насквозь.

Из чего собран HTTP(S)-балансировщик — это спрашивают, потому что в консоли и Terraform компоненты создаются отдельно:

```
forwarding rule (IP:443) → target HTTPS proxy (+ SSL-сертификат) → URL map (правила: /api → backend A)
   → backend service (+ health check, Cloud CDN, Cloud Armor) → backends: MIG или NEG
```

- **Forwarding rule** — внешний IP и порт, на который приходят клиенты.
- **Target proxy** — завершает TLS-соединение (TLS — шифрование поверх TCP, на нём работает HTTPS) (здесь сертификат; Google-managed сертификаты выпускаются и продлеваются автоматически).
- **URL map** — маршрутизация по хосту и пути, как правила listener в ALB.
- **Backend service** — группа бэкендов с проверкой здоровья, алгоритмом балансировки, CDN и WAF.
- **Backends** — Managed Instance Group (группа VM) или **NEG** (Network Endpoint Group — набор конечных точек: поды GKE напрямую, Cloud Run, внешние адреса).

**Ловушка с health checks:** проверки здоровья идут не изнутри VPC, а с адресов Google `35.191.0.0/16` и `130.211.0.0/22`. Если файрвол их не пропускает, все бэкенды считаются нездоровыми и балансировщик отдаёт 502. Это классическая проблема при первом запуске.

### Cloud Armor и Cloud CDN

- **Cloud Armor** — WAF (Web Application Firewall, фильтр HTTP-запросов против SQL-инъекций, XSS — Cross-Site Scripting, внедрения чужого JavaScript в страницу, — и т. п.) и защита от DDoS (распределённой атаки отказа в обслуживании) на внешних Application LB. Правила по IP, странам, заголовкам, rate limiting, готовые наборы правил OWASP (Open Worldwide Application Security Project, список самых частых уязвимостей веб-приложений).
- **Cloud CDN** (Content Delivery Network) — кэширование ответов на точках присутствия Google; включается флагом на backend service.

### Cloud DNS

Управляемый DNS, аналог Route 53:
- **public zone** — зона, видимая из интернета;
- **private zone** — видна только из указанных VPC (внутренние имена вроде `db.internal.example.com`);
- **forwarding zone** — запросы к зоне пересылаются на другие DNS-серверы (в дата-центр или в AWS);
- внутри VPC у каждой VM есть автоматическое имя `VM.ZONE.c.PROJECT.internal`.

### 🧪 Мини-задание

**Задача.** Посмотри, какие готовые наборы правил WAF (preconfigured rules) есть в Cloud Armor, и создай правило файрвола, без которого health checks балансировщика не проходят.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
gcloud compute security-policies list-preconfigured-expression-sets | grep -E '^(sqli|xss|lfi|rce)-v33-stable'
gcloud compute firewall-rules create lab-allow-health-checks --network=lab-vpc \
  --direction=INGRESS --action=ALLOW --rules=tcp:80 \
  --source-ranges=35.191.0.0/16,130.211.0.0/22 --target-tags=web
gcloud compute firewall-rules describe lab-allow-health-checks --format="value(sourceRanges.list())"
```

Ожидаемый результат:

```text
lfi-v33-stable
rce-v33-stable
sqli-v33-stable
xss-v33-stable
35.191.0.0/16,130.211.0.0/22
```

Разбор: наборы `sqli`, `xss`, `lfi` (Local File Inclusion — чтение локальных файлов сервера через параметры запроса), `rce` (Remote Code Execution — выполнение кода на сервере) — правила на основе OWASP ModSecurity Core Rule Set, их подключают в политику Cloud Armor одной строкой `evaluatePreconfiguredWaf('sqli-v33-stable')`. Правило файрвола пропускает проверки здоровья Google к VM с тегом `web` — без него балансировщик считает бэкенды мёртвыми и отдаёт 502. Версии наборов правил (`v33`) со временем обновляются, у тебя список может быть другим.

</details>

## Вычисления: Compute Engine, MIG, Cloud Run

### Compute Engine (VM)

- **Семейства машин (machine families):**
  - **E2** — дешёвые VM общего назначения, ядро CPU может делиться с другими (аналог T3 в AWS); `e2-micro` входит в Free Tier;
  - **N2, N2D, N4** — общего назначения, стабильная производительность (как M6i/M7i);
  - **C3, C4** — оптимизированные под вычисления;
  - **M3** — с большим объёмом памяти для SAP (крупных корпоративных систем учёта) и in-memory баз (хранящих все данные в оперативной памяти);
  - **C4A, T2A** — на процессорах Arm (C4A — собственный CPU Google Axion), аналог Graviton.
- **Custom machine types** — можно задать любое число vCPU и объём памяти (например, 6 vCPU и 20 GB), а не выбирать из фиксированного списка, как в AWS.
- **Spot VM** — неиспользуемые мощности со скидкой 60–91%; Google может забрать VM, предупредив за 30 секунд. **Preemptible VM** — старая версия Spot, которая жила не больше 24 часов. Подходят для CI, пакетных задач, stateless-нод GKE.
- **Live migration** — при обслуживании железа VM переезжает на другой хост без перезагрузки (кроме VM с GPU и некоторых других типов).
- **Metadata и startup script** — ключ `startup-script` в метаданных VM выполняется при каждой загрузке (аналог user data в EC2, но user data в EC2 по умолчанию выполняется только при первом запуске).
- **OS Login** — вход по SSH через IAM-роли (`roles/compute.osLogin`, `roles/compute.osAdminLogin`) вместо раскладывания SSH-ключей по VM. Уволили человека, удалили из группы — доступ пропал на всех VM.

### Managed Instance Groups (MIG)

**Instance template** (шаблон VM, как Launch Template) + **MIG** (группа одинаковых VM, как Auto Scaling Group):
- **autoscaling** — по CPU, нагрузке балансировщика, метрике Cloud Monitoring или расписанию;
- **autohealing** — health check: VM, которая не отвечает, пересоздаётся;
- **zonal или regional MIG** — региональная группа распределяет VM по нескольким зонам и переживает отказ зоны;
- **rolling update** — постепенная замена VM на новый шаблон с параметрами `maxSurge` (сколько лишних VM можно создать во время обновления) и `maxUnavailable` (сколько VM может одновременно не работать) — те же понятия, что в Deployment в Kubernetes.

### Диски

- **Persistent Disk (PD)** — сетевые диски, аналог EBS: `pd-standard` (HDD), `pd-balanced` (SSD по умолчанию), `pd-ssd` (быстрый SSD). **Hyperdisk** — новое поколение, где производительность (IOPS — операций ввода-вывода в секунду, и пропускная способность) настраивается отдельно от размера, как gp3/io2.
- **Regional PD** — диск синхронно реплицируется в две зоны; если зона упала, VM поднимают во второй зоне с тем же диском. В AWS у EBS такого нет.
- **Local SSD** — физически подключённый к хосту диск, очень быстрый, но **данные теряются** при остановке VM. Для кэшей и временных файлов.
- **Snapshots** — инкрементальные снимки дисков, хранятся по умолчанию в мультирегиональном хранилище, и из них можно создать диск в любом регионе.

### Cloud Run и функции

- **Cloud Run** — запускает контейнер без управления серверами: даёшь образ, получаешь HTTPS-адрес. Масштабируется по числу запросов, **в том числе до нуля** (нет запросов — не платишь). Одна копия обрабатывает до 80 одновременных запросов по умолчанию (`concurrency`, максимум 1000) — в отличие от Lambda, где одна копия обрабатывает один запрос. Аналог в AWS — App Runner или Fargate, но проще.
  - **Services** — HTTP-сервисы;
  - **Jobs** — задачи, которые выполняются и завершаются (миграции, пакетная обработка).
- **Cloud Run functions** (раньше Cloud Functions) — функции на Python, Node.js, Go, которые запускаются на событие (HTTP-запрос, сообщение Pub/Sub, новый файл в бакете). Аналог Lambda; под капотом теперь тот же Cloud Run.
- **App Engine** — старая платформа (PaaS — Platform as a Service, платформа, куда загружаешь код, а не серверы), сейчас для новых проектов обычно берут Cloud Run.

Как выбрать: **Cloud Run** — для stateless HTTP-сервисов и API без Kubernetes; **GKE** — когда сервисов много, нужны свои операторы, stateful-нагрузки, service mesh; **Compute Engine** — для legacy, баз данных на своих VM и всего, что не влезает в контейнер.

### 🧪 Мини-задание

**Задача.** Задеплой публичный образ-пример в Cloud Run одной командой, получи URL и проверь, что сервис отвечает.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
gcloud services enable run.googleapis.com
gcloud run deploy hello --image=us-docker.pkg.dev/cloudrun/container/hello \
  --region=europe-west3 --allow-unauthenticated --max-instances=2 --quiet
URL=$(gcloud run services describe hello --region=europe-west3 --format="value(status.url)")
curl -s "$URL" | grep -o '<title>.*</title>'
gcloud run services describe hello --region=europe-west3 --format="value(spec.template.spec.containerConcurrency)"
```

Ожидаемый результат:

```text
Service [hello] revision [hello-00001-abc] has been deployed and is serving 100 percent of traffic.
Service URL: https://hello-123456789012.europe-west3.run.app
<title>Congratulations | Cloud Run</title>
80
```

Разбор: Cloud Run сам выпустил HTTPS-сертификат и выдал адрес. Каждый деплой создаёт **ревизию** (revision — неизменяемый снимок образа и настроек), между ревизиями можно делить трафик (`--tag`, `--no-traffic` для canary-релизов). `80` — `containerConcurrency`, сколько запросов одновременно обрабатывает одна копия. `--max-instances=2` — потолок копий, который защищает от неожиданного счёта. `--allow-unauthenticated` открывает сервис всему интернету; для внутренних сервисов его не ставят, и вызывать их можно только с IAM-ролью `roles/run.invoker`. Удалить: `gcloud run services delete hello --region=europe-west3`. URL у тебя будет свой.

</details>

## GKE — Kubernetes в GCP

Kubernetes родом из Google, и GKE считается самым зрелым управляемым Kubernetes. Главное, что нужно знать:

### Режимы: Autopilot и Standard

| | Autopilot | Standard |
|---|---|---|
| Ноды | управляет Google, ты их не видишь | твои **node pools** (группы нод одного типа), сам выбираешь машины |
| Оплата | за ресурсы, запрошенные подами (`requests`) | за VM нод, даже если они простаивают |
| Ограничения | нельзя privileged-поды, доступ к ноде по SSH, часть DaemonSet | полный контроль |
| Когда брать | по умолчанию для новых кластеров, когда не нужен тонкий тюнинг нод | GPU, специфичные ядра и настройки нод, свои агенты на нодах |

Аналог Autopilot в AWS — EKS Auto Mode или EKS на Fargate.

### Ключевые понятия

- **Zonal или regional cluster** — у зонального control plane (API-сервер, etcd — база состояния кластера) живёт в одной зоне: пока зону обслуживают или она упала, `kubectl` не работает (поды продолжают работать). У регионального — три реплики control plane в трёх зонах. Для прода — regional.
- **Release channels (каналы обновлений)**: Rapid, Regular, Stable, Extended. GKE **сам обновляет** control plane и ноды в выбранном канале; управлять этим можно через **maintenance windows** (окна, когда обновления разрешены) и **maintenance exclusions** (периоды запрета, например на распродажу). В EKS обновления запускаешь сам.
- **VPC-native cluster** — поды получают IP из secondary range подсети (alias IP): поды видны в VPC как обычные адреса, без оверлея. Диапазоны подов и сервисов планируют заранее — их трудно расширить, и при нехватке адресов новые ноды не создаются.
- **Private cluster** — у нод нет внешних IP, а API-сервер доступен только из разрешённых сетей.
- **Workload Identity Federation for GKE** — под получает права IAM без ключей: Kubernetes ServiceAccount связывается с IAM-ролями. Аналог IRSA (IAM Roles for Service Accounts) и EKS Pod Identity в AWS. Без неё поды получают права SA ноды — то есть все поды на ноде имеют одинаковые права.
- **Cluster autoscaler** (добавляет и убирает ноды в node pool) и **node auto-provisioning** (сам создаёт новые node pools подходящего типа, похоже на Karpenter в AWS).
- **Стоимость**: $0,10 в час за кластер (плата за управление), но один зональный или Autopilot-кластер на платёжный аккаунт фактически бесплатен — Google ежемесячно возвращает эту сумму кредитом.

```bash
# получить kubeconfig для кластера (нужен плагин gke-gcloud-auth-plugin)
gcloud container clusters get-credentials prod-gke --region=europe-west3
# выдать поду права через Workload Identity: Kubernetes ServiceAccount app в namespace default получает роль на бакет
gcloud storage buckets add-iam-policy-binding gs://shop-prod-assets \
  --role=roles/storage.objectViewer \
  --member="principal://iam.googleapis.com/projects/123456789012/locations/global/workloadIdentityPools/shop-prod.svc.id.goog/subject/ns/default/sa/app"
```

### 🧪 Мини-задание

**Задача.** Создай самый дешёвый вариант кластера — Autopilot, получи kubeconfig, запусти под и посмотри, какие ноды под ним появились.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком (создание кластера занимает 5–10 минут):

```bash
gcloud services enable container.googleapis.com
gcloud components install gke-gcloud-auth-plugin --quiet
gcloud container clusters create-auto lab-gke --region=europe-west3
gcloud container clusters get-credentials lab-gke --region=europe-west3
kubectl apply -f - <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: web
spec:
  containers:
  - name: nginx
    image: nginx
    resources:
      requests:
        cpu: 250m
        memory: 512Mi
EOF
kubectl wait --for=condition=Ready pod/web --timeout=300s
kubectl get nodes -o custom-columns=NAME:.metadata.name,TYPE:.metadata.labels.node\\.kubernetes\\.io/instance-type
gcloud container clusters describe lab-gke --region=europe-west3 --format="value(autopilot.enabled, releaseChannel.channel, currentMasterVersion)"
```

Ожидаемый результат:

```text
pod/web created
pod/web condition met
NAME                                     TYPE
gk3-lab-gke-pool-2-1a2b3c4d-x7kq         ek-standard-8
True	REGULAR	1.33.5-gke.1080000
```

Разбор: Autopilot сам создал ноду под запрошенные `requests` — сколько и каких нод, решает Google, а платишь ты за 250m CPU и 512Mi памяти пода. Kubeconfig получен через gcloud, а токен для `kubectl` выдаёт плагин `gke-gcloud-auth-plugin` по твоей учётной записи gcloud — как `aws eks update-kubeconfig`. Кластер подписан на канал Regular и будет обновляться сам. Удали кластер после эксперимента: `gcloud container clusters delete lab-gke --region=europe-west3 --quiet`. Имена нод и версии у тебя будут свои.

</details>

## Хранилища и базы данных

### Cloud Storage (GCS)

Объектное хранилище, аналог S3, адреса вида `gs://bucket/path/file`.
- **Имя бакета глобально уникально**, как в S3.
- **Location (размещение) бакета:** region (один регион), **dual-region** (два конкретных региона, синхронная запись), multi-region (`EU`, `US` — несколько регионов континента). В S3 бакет всегда в одном регионе, а репликацию настраивают отдельно.
- **Классы хранения:** Standard; Nearline (минимальный срок хранения 30 дней, для данных, которые читают раз в месяц); Coldline (90 дней); Archive (365 дней). В отличие от S3 Glacier, **данные во всех классах доступны сразу**, без ожидания восстановления — дороже только чтение. Если удалить объект раньше минимального срока, всё равно заплатишь за весь срок.
- **Autoclass** — сам переводит объекты между классами по частоте доступа, как S3 Intelligent-Tiering.
- **Uniform bucket-level access** — права только через IAM на бакет, без ACL (Access Control List — списков доступа на каждый объект). Включают всегда; аналог «Object Ownership: bucket owner enforced» в S3.
- **Public access prevention** — запрет делать бакет публичным, аналог S3 Block Public Access.
- **Object versioning, lifecycle rules, retention policy с bucket lock** (неизменяемое хранение: нельзя удалить объект раньше срока даже администратору — для бэкапов и требований регуляторов).
- **Signed URL** — временная ссылка на объект без прав IAM, аналог presigned URL в S3.
- CLI: `gcloud storage` (новый и быстрый); `gsutil` — старый инструмент, встречается в скриптах.

### Базы данных

| Сервис | Что это | Аналог AWS | Когда брать |
|---|---|---|---|
| **Cloud SQL** | управляемые MySQL, PostgreSQL, SQL Server | RDS | обычная реляционная база приложения |
| **AlloyDB** | совместимая с PostgreSQL, быстрее и дороже Cloud SQL | Aurora PostgreSQL | высокая нагрузка на Postgres, смешанная аналитика |
| **Spanner** | глобально распределённая SQL-база с горизонтальным масштабированием и строгой согласованностью | Aurora DSQL, DynamoDB Global Tables (частично) | финансы, глобальные системы, где одна база не помещается на сервер |
| **Firestore** | документная NoSQL-база, serverless | DynamoDB | мобильные и веб-приложения, простые ключ-документ запросы |
| **Bigtable** | широкостолбцовая NoSQL для огромных объёмов (совместима с HBase API) | Keyspaces, DynamoDB | временные ряды, IoT (данные с датчиков и устройств), аналитика с миллионами записей в секунду |
| **Memorystore** | управляемые Redis, Valkey, Memcached | ElastiCache | кэш, сессии |
| **BigQuery** | serverless хранилище данных для аналитики, SQL по петабайтам | Redshift, Athena | аналитика, отчёты, экспорт логов и биллинга |

Что спрашивают про **Cloud SQL**:
- **High availability (HA)**: основной инстанс и резервный в другой зоне, запись на диск реплицируется синхронно (regional PD); при отказе переключение за ~минуту на тот же IP. Аналог RDS Multi-AZ. Читать с резервного нельзя.
- **Read replicas** — асинхронные копии для чтения, в том числе в других регионах (для DR — Disaster Recovery, восстановления после катастрофы).
- **Бэкапы и PITR** (Point-In-Time Recovery — восстановление на любую секунду в пределах срока хранения логов транзакций).
- **Подключение:** через **private IP** в своей VPC (через Private Service Access — пиринг с сетью Google, где живёт база) или через **Cloud SQL Auth Proxy** — локальный прокси, который шифрует соединение и проверяет права по IAM, без открытия базы в интернет и без управления сертификатами.

### Секреты и ключи

- **Secret Manager** — хранение секретов с версиями, правами IAM на каждый секрет и аудитом доступа. Аналог AWS Secrets Manager, но без встроенной автоматической ротации паролей баз — ротацию делают через уведомления Pub/Sub и свою функцию.
- **Cloud KMS** (Key Management Service, сервис управления ключами шифрования) — ключи для шифрования. Все данные в GCP и так шифруются ключами Google; **CMEK** (Customer-Managed Encryption Keys, ключи под управлением клиента) нужны, когда регулятор требует, чтобы ключ контролировал ты: отключил ключ — данные стали недоступны.

### 🧪 Мини-задание

**Задача.** Создай бакет с uniform access и запретом публичного доступа, добавь правило lifecycle «удалять объекты старше 30 дней», загрузи файл и выдай временную ссылку на него.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
PROJECT_ID=$(gcloud config get-value project)
B="gs://$PROJECT_ID-lab"
gcloud storage buckets create "$B" --location=europe-west3 --uniform-bucket-level-access --public-access-prevention
echo '{"rule":[{"action":{"type":"Delete"},"condition":{"age":30}}]}' > lifecycle.json
gcloud storage buckets update "$B" --lifecycle-file=lifecycle.json
echo "hello gcs" > hello.txt && gcloud storage cp hello.txt "$B/"
gcloud storage buckets describe "$B" --format="yaml(location, storage_class, uniform_bucket_level_access, public_access_prevention, lifecycle_config)"
```

Ожидаемый результат:

```text
lifecycle_config:
  rule:
  - action:
      type: Delete
    condition:
      age: 30
location: EUROPE-WEST3
public_access_prevention: enforced
storage_class: STANDARD
uniform_bucket_level_access: true
```

Разбор: `uniform_bucket_level_access` отключает ACL на объектах — доступ управляется только IAM на бакет. `public_access_prevention: enforced` не даст выдать роль `allUsers`, даже если кто-то попробует. Правило lifecycle удалит объекты через 30 дней после создания — так чистят временные выгрузки и старые логи. Временная ссылка делается командой `gcloud storage sign-url "$B/hello.txt" --duration=10m --impersonate-service-account=SA` — подписывать ссылку должен сервисный аккаунт, поэтому для пользователя нужна impersonation. Имя бакета у тебя будет своё.

</details>

## Наблюдаемость и аудит

### Cloud Logging

- Логи VM (через **Ops Agent** — агент сбора логов и метрик, его ставят на VM), GKE, Cloud Run, балансировщиков и самих сервисов GCP собираются автоматически в Cloud Logging. Аналог CloudWatch Logs.
- **Log Router и sinks** (маршрутизатор логов и «стоки») — каждый лог проходит через правила: в какой **log bucket** его положить, куда экспортировать (Cloud Storage для дешёвого долгого хранения, BigQuery для анализа SQL-запросами, Pub/Sub для отправки во внешние системы вроде Splunk или Datadog), а что отбросить (exclusion filters — так экономят: платишь за объём принятых логов).
- Встроенные бакеты: **`_Required`** — аудит-логи администрирования, хранятся 400 дней бесплатно, удалить или изменить нельзя; **`_Default`** — всё остальное, 30 дней по умолчанию.
- Запросы пишут на **Logging query language**: `resource.type="k8s_container" AND severity>=ERROR AND resource.labels.namespace_name="prod"`.

### Cloud Monitoring

- Метрики всех сервисов GCP, дашборды, **alerting policies** (правила алертов) с каналами уведомлений (email, Slack, PagerDuty, Pub/Sub).
- **Uptime checks** — внешние проверки доступности сайта из нескольких точек мира.
- **Managed Service for Prometheus** — Prometheus как сервис: метрики приложений из GKE собираются в формате Prometheus и запрашиваются на PromQL (языке запросов Prometheus), а хранить их не нужно самому. Grafana подключается к нему как к обычному Prometheus.
- **SLO monitoring** — встроенное описание SLO (Service Level Objective, целевой уровень надёжности, например «99,9% запросов успешны») и алерты по скорости сжигания бюджета ошибок.
- **Cloud Trace** (распределённая трассировка запросов) и **Error Reporting** (группировка исключений из логов).

### Cloud Audit Logs — кто что сделал

Аналог CloudTrail, но разделён на типы:
- **Admin Activity** — изменения конфигурации и прав (создал VM, поменял IAM). **Включены всегда и бесплатны**, хранятся 400 дней.
- **Data Access** — чтение данных и метаданных (кто прочитал объект в бакете, кто запросил секрет). **По умолчанию выключены** (кроме BigQuery), потому что их очень много и они платные. Включают выборочно для чувствительных сервисов: Secret Manager, Cloud Storage с персональными данными.
- **System Event** — действия самого Google (live migration VM, автоматическое масштабирование).
- **Policy Denied** — запросы, отклонённые Organization Policy или VPC Service Controls (сервис, который ограничивает вынос данных из периметра проектов).

### 🧪 Мини-задание

**Задача.** Найди в аудит-логах, кто создавал сервисные аккаунты и правила файрвола в проекте за последние сутки.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
gcloud logging read '
  logName:"cloudaudit.googleapis.com%2Factivity"
  AND (protoPayload.methodName="google.iam.admin.v1.CreateServiceAccount"
       OR protoPayload.methodName:"compute.firewalls.insert")' \
  --freshness=1d --limit=5 \
  --format="table(timestamp.date('%Y-%m-%d %H:%M'), protoPayload.authenticationInfo.principalEmail, protoPayload.methodName, protoPayload.resourceName.basename())"
```

Ожидаемый результат:

```text
TIMESTAMP         PRINCIPAL_EMAIL      METHOD_NAME                                   RESOURCE_NAME
2026-10-30 14:12  anna@example.com     v1.compute.firewalls.insert                   lab-allow-health-checks
2026-10-30 13:58  anna@example.com     v1.compute.firewalls.insert                   lab-allow-iap-ssh
2026-10-30 13:41  anna@example.com     google.iam.admin.v1.CreateServiceAccount      reader@sandbox-1234567890.iam.gserviceaccount.com
```

Разбор: `cloudaudit.googleapis.com%2Factivity` — лог Admin Activity (`%2F` — закодированный символ `/` в имени лога), он включён всегда. `principalEmail` — кто сделал действие: человек или сервисный аккаунт (если через impersonation, в `serviceAccountDelegationInfo` будет виден и исходный пользователь). `methodName` — какой API вызван. Это ответ на классический вопрос «кто удалил ресурс в проде»: фильтр по `methodName` с `delete` и имени ресурса. Для поиска за больший период логи экспортируют sink'ом в BigQuery. Записи у тебя будут свои.

</details>

## Интеграция сервисов и CI/CD

### Pub/Sub и события

- **Pub/Sub** — управляемая очередь сообщений «издатель — подписчик». Издатель пишет в **topic** (топик), у топика может быть несколько **subscriptions** (подписок), и каждая получает свою копию сообщений. Одна сущность заменяет связку SNS + SQS. Подписки бывают **pull** (потребитель сам забирает сообщения) и **push** (Pub/Sub отправляет HTTP-запрос на твой endpoint, например в Cloud Run). Сообщение без подтверждения (ack) доставляется повторно, поэтому обработчик должен быть идемпотентным (повторная обработка не должна ломать данные). Есть dead-letter topic для сообщений, которые не удаётся обработать.
- **Eventarc** — маршрутизирует события («создан объект в бакете», «запись в аудит-логе») в Cloud Run и функции, аналог EventBridge.
- **Cloud Scheduler** (cron как сервис), **Cloud Tasks** (очередь отложенных HTTP-вызовов), **Workflows** (оркестрация шагов, аналог Step Functions).

### Artifact Registry, Cloud Build, Cloud Deploy

- **Artifact Registry** — реестр Docker-образов, а также пакетов npm, Maven, Python, apt. Адрес образа: `europe-west3-docker.pkg.dev/PROJECT/REPO/IMAGE:TAG`. Старый Container Registry (`gcr.io`) закрыт в 2025 году — если в манифестах встречается `gcr.io`, это повод для миграции. Умеет сканировать образы на уязвимости и работать как кэширующий прокси Docker Hub (remote repository — защищает от лимитов Docker Hub на скачивание).
- **Cloud Build** — сборки по файлу `cloudbuild.yaml`: каждый шаг — контейнер. Запускается по триггеру из GitHub, GitLab, Bitbucket. Аналог CodeBuild.
- **Cloud Deploy** — доставка релизов по цепочке окружений (dev → stage → prod) с ручным подтверждением, canary-релизами и откатом, для GKE и Cloud Run.

На практике CI часто остаётся в GitHub Actions или GitLab CI, а в GCP заходят через **Workload Identity Federation** без JSON-ключей:

```yaml
# .github/workflows/deploy.yml (фрагмент)
permissions:
  id-token: write
  contents: read
steps:
  - uses: google-github-actions/auth@v2
    with:
      workload_identity_provider: projects/123456789012/locations/global/workloadIdentityPools/github/providers/github
      service_account: deployer@shop-prod.iam.gserviceaccount.com
  - uses: google-github-actions/setup-gcloud@v2
  - run: gcloud run deploy api --image=europe-west3-docker.pkg.dev/shop-prod/apps/api:${{ github.sha }} --region=europe-west3
```

`id-token: write` разрешает GitHub выдать пайплайну OIDC-токен; GCP проверяет, что токен выдан именно этому репозиторию (условие задаётся в провайдере), и выдаёт временный токен сервисного аккаунта `deployer`.

### 🧪 Мини-задание

**Задача.** Создай топик Pub/Sub с pull-подпиской, опубликуй сообщение и прочитай его с подтверждением.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
gcloud services enable pubsub.googleapis.com
gcloud pubsub topics create lab-orders
gcloud pubsub subscriptions create lab-orders-worker --topic=lab-orders --ack-deadline=30
gcloud pubsub topics publish lab-orders --message='{"order_id": 42}' --attribute=source=shop
gcloud pubsub subscriptions pull lab-orders-worker --auto-ack --limit=1 --format="value(message.data, message.attributes.source)"
```

Ожидаемый результат:

```text
messageIds:
- '13579246801357924'
{"order_id": 42}	shop
```

Разбор: сообщение ушло в топик и сохранилось для каждой подписки, созданной **до** публикации (сообщения, отправленные раньше создания подписки, она не получит). `--auto-ack` сразу подтверждает получение; без подтверждения через `ack-deadline` (30 секунд) сообщение будет доставлено снова — так Pub/Sub гарантирует доставку хотя бы один раз (at-least-once). Если бы подписок было две, каждая получила бы свою копию — это и есть замена SNS + SQS одной сущностью. ID сообщения у тебя будет свой.

</details>

## Terraform для GCP

- Провайдеры: **`google`** и **`google-beta`** (для функций в бета-версии API).
- **Бэкенд состояния — `gcs`**: стейт в бакете Cloud Storage, **блокировка встроена** (в S3-бэкенде для этого раньше нужна была DynamoDB-таблица). Включают версионирование бакета, чтобы можно было откатиться к прошлому стейту.
- **Аутентификация**: локально — ADC (`gcloud auth application-default login`), в CI — Workload Identity Federation. Удобный приём — `impersonate_service_account` в провайдере: человек запускает Terraform со своими правами, но действует от имени SA с нужными ролями.
- **API нужно включать ресурсом** `google_project_service`, иначе первый `apply` в новом проекте падает с 403.
- **Три способа задать IAM — главная ловушка:**
  - `google_project_iam_policy` — **заменяет всю** политику проекта: удалит все привязки, которых нет в коде, включая служебные привязки Google. Так легко отрезать доступ себе и сервисам;
  - `google_project_iam_binding` — **авторитетен для одной роли**: удалит всех остальных участников этой роли;
  - `google_project_iam_member` — **добавляет одного** участника, не трогая остальных. Безопасный вариант по умолчанию.
- **Cloud Foundation Toolkit / terraform-google-modules** — официальные модули Google (network, project-factory, kubernetes-engine), аналог terraform-aws-modules.

```hcl
terraform {
  backend "gcs" {
    bucket = "shop-tfstate"
    prefix = "prod/network"
  }
}

provider "google" {
  project                     = "shop-prod"
  region                      = "europe-west3"
  impersonate_service_account = "terraform@shop-prod.iam.gserviceaccount.com"
}

resource "google_project_service" "compute" {
  service            = "compute.googleapis.com"
  disable_on_destroy = false
}

resource "google_compute_network" "main" {
  name                    = "prod-vpc"
  auto_create_subnetworks = false
  depends_on              = [google_project_service.compute]
}

resource "google_compute_subnetwork" "euw3" {
  name                     = "prod-euw3"
  network                  = google_compute_network.main.id
  region                   = "europe-west3"
  ip_cidr_range            = "10.20.0.0/20"
  private_ip_google_access = true

  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = "10.64.0.0/14"
  }
  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = "10.20.16.0/20"
  }
}

resource "google_project_iam_member" "ci_run_admin" {
  project = "shop-prod"
  role    = "roles/run.admin"
  member  = "serviceAccount:deployer@shop-prod.iam.gserviceaccount.com"
}
```

`disable_on_destroy = false` — чтобы `terraform destroy` не выключал API в проекте (иначе сломаются ресурсы, созданные не этим кодом). `auto_create_subnetworks = false` — custom-mode VPC. Secondary ranges `pods` и `services` заранее заложены под VPC-native кластер GKE: `/14` даёт около 260 тысяч адресов подов.

### 🧪 Мини-задание

**Задача.** Опиши в Terraform бакет для стейта и VPC из примера выше, проверь код без применения и посмотри план.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
mkdir -p ~/lab-gcp-tf && cd ~/lab-gcp-tf
gcloud auth application-default login
PROJECT_ID=$(gcloud config get-value project)
cat > main.tf <<EOF
provider "google" {
  project = "$PROJECT_ID"
  region  = "europe-west3"
}

resource "google_storage_bucket" "tfstate" {
  name                        = "$PROJECT_ID-tfstate"
  location                    = "EUROPE-WEST3"
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  versioning {
    enabled = true
  }
}

resource "google_compute_network" "main" {
  name                    = "tf-vpc"
  auto_create_subnetworks = false
}
EOF
terraform init -input=false >/dev/null && terraform validate && terraform plan -input=false | grep -E '^  # |^Plan:'
```

Ожидаемый результат:

```text
Success! The configuration is valid.

  # google_compute_network.main will be created
  # google_storage_bucket.tfstate will be created
Plan: 2 to add, 0 to change, 0 to destroy.
```

Разбор: `terraform init` скачал провайдер `google`, `validate` проверил синтаксис, `plan` сходил в API с учётными данными ADC — поэтому сначала нужна команда `application-default login`, а не просто `gcloud auth login`. Бакет с версионированием потом указывают в блоке `backend "gcs"` и переносят туда стейт командой `terraform init -migrate-state`. `apply` здесь не делаем — это только проверка кода.

</details>

## Стоимость (FinOps)

FinOps — практика управления облачными расходами: видеть, кто и на что тратит, и снижать счёт без потери надёжности.

- **Посекундная оплата** VM (минимум минута).
- **Sustained use discounts (SUD, скидки за постоянное использование)** — **автоматические**: если VM N1 или N2 работает больше четверти месяца, цена постепенно снижается, до 20–30% при работе весь месяц. Ничего покупать не нужно. В AWS такого нет. На E2 и новых семействах (N4, C3, C4) не действуют.
- **Committed use discounts (CUD, скидки за обязательство)** — обещаешь использовать ресурсы 1 или 3 года и получаешь до 55–70% скидки:
  - **resource-based** — на конкретные vCPU и память в регионе, как EC2 Reserved Instances;
  - **spend-based (flexible)** — на сумму расходов в час на разные сервисы, как Savings Plans.
- **Spot VM** — до 91% скидки для прерываемой нагрузки.
- **Платный исходящий трафик (egress)** — в интернет и между регионами, как и в AWS. **Network Service Tiers**: **Premium** (по умолчанию: трафик идёт по сети Google до ближайшей к пользователю точки, нужен для глобального балансировщика) и **Standard** (дешевле: трафик сразу уходит в обычный интернет из региона).
- **Labels (метки)** — пары ключ-значение на ресурсах (`env=prod`, `team=payments`), аналог тегов AWS; по ним делят счёт.
- **Budgets and alerts** — бюджеты с уведомлениями при 50%, 90%, 100%. **Важно: бюджет не останавливает расходы**, только уведомляет. Чтобы реально «выключить» проект при перерасходе, бюджет шлёт сообщение в Pub/Sub, а функция отвязывает проект от платёжного аккаунта.
- **Billing export в BigQuery** — детальные расходы по сервисам, проектам и меткам, на них строят дашборды в Looker Studio.
- **Recommender** — рекомендации: уменьшить VM, удалить простаивающие диски и IP, убрать лишние роли IAM.

### 🧪 Мини-задание

**Задача.** Создай бюджет на $10 с уведомлениями при 50% и 90% для своего проекта и найди неиспользуемые ресурсы, за которые идёт оплата: диски без VM и зарезервированные внешние IP без привязки.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
gcloud services enable billingbudgets.googleapis.com
BILLING=$(gcloud billing projects describe "$(gcloud config get-value project)" --format="value(billingAccountName.basename())")
gcloud billing budgets create --billing-account="$BILLING" --display-name="lab-10usd" \
  --budget-amount=10USD --threshold-rule=percent=0.5 --threshold-rule=percent=0.9
gcloud billing budgets list --billing-account="$BILLING" --format="table(displayName, amount.specifiedAmount.units, thresholdRules[].thresholdPercent.list())"
gcloud compute disks list --filter="-users:*" --format="table(name, zone.basename(), sizeGb)"
gcloud compute addresses list --filter="status=RESERVED" --format="table(name, region.basename(), address)"
```

Ожидаемый результат:

```text
DISPLAY_NAME  UNITS  THRESHOLD_PERCENT
lab-10usd     10     0.5,0.9
Listed 0 items.
Listed 0 items.
```

Разбор: бюджет создаётся на платёжном аккаунте и по умолчанию считает все его проекты (фильтр по проекту задаётся `--filter-projects`). Уведомления придут администраторам платёжного аккаунта на почту, но **расходы бюджет не остановит**. `-users:*` в фильтре — «у диска нет поля users», то есть он не подключён ни к одной VM, но место оплачивается. Внешний IP в статусе `RESERVED` (зарезервирован, но не привязан) тоже стоит денег. В чистом проекте оба списка пустые. Для лабораторных проще всего удалить весь проект: `gcloud projects delete PROJECT_ID`.

</details>

## gcloud CLI в повседневной работе

```bash
gcloud config list                              # текущие аккаунт, проект, регион
gcloud config configurations create prod       # именованный набор настроек, как профиль AWS CLI
gcloud config set project shop-prod
gcloud config set compute/region europe-west3
gcloud config configurations activate dev      # переключиться на другой набор
CLOUDSDK_ACTIVE_CONFIG_NAME=prod gcloud compute instances list   # разово для одной команды

gcloud compute instances list --filter="labels.env=prod AND status=RUNNING" \
  --format="table(name, zone.basename(), machineType.basename(), networkInterfaces[0].networkIP)"
gcloud compute ssh vm1 --zone=europe-west3-a --tunnel-through-iap
gcloud compute instances add-metadata vm1 --metadata=enable-oslogin=TRUE
gcloud asset search-all-resources --scope=projects/shop-prod --query="labels.env:prod"
gcloud logging read 'severity>=ERROR' --freshness=1h --limit=20
gcloud projects get-iam-policy shop-prod --flatten="bindings[].members" \
  --filter="bindings.members:deployer@" --format="value(bindings.role)"
```

- **`--filter`** — фильтрация на стороне gcloud (аналог `--query` в AWS CLI для отбора), **`--format`** — что и как выводить: `table(...)`, `value(...)` (для скриптов, без заголовков), `json`, `yaml`, `csv`. `--flatten` разворачивает вложенные списки в отдельные строки — так удобно искать, какие роли есть у конкретного участника.
- **`gcloud asset search-all-resources`** — поиск ресурсов по всей организации или проекту одной командой (Cloud Asset Inventory). Аналог AWS Resource Explorer.
- **Cloud Shell** — бесплатная VM с браузерным терминалом, где уже установлены gcloud, kubectl, terraform и выполнен вход. Удобно, если на рабочем компьютере нельзя ставить утилиты.

### 🧪 Мини-задание

**Задача.** Заведи две конфигурации gcloud — `lab` и `prod` — с разными регионами и выведи, какая активна и какой регион в каждой.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
gcloud config configurations create lab --no-activate
gcloud config configurations create prod --no-activate
gcloud config set compute/region europe-west3 --configuration=lab
gcloud config set compute/region us-central1 --configuration=prod
gcloud config configurations activate lab
gcloud config configurations list --format="table(name, is_active, properties.compute.region)"
CLOUDSDK_ACTIVE_CONFIG_NAME=prod gcloud config get-value compute/region
```

Ожидаемый результат:

```text
NAME     IS_ACTIVE  REGION
default  False
lab      True       europe-west3
prod     False      us-central1
Activated [lab].
us-central1
```

Разбор: конфигурации — это именованные наборы настроек (аккаунт, проект, регион), как профили в `~/.aws/config`. Переключаться можно командой `activate` или переменной `CLOUDSDK_ACTIVE_CONFIG_NAME` для одной команды — так не перепутаешь прод и стенд в соседних вкладках терминала. Строка `Activated [lab].` выводится в stderr, поэтому может появиться в другом месте вывода. Аккаунт и проект в каждой конфигурации задаются так же: `gcloud config set account ... --configuration=prod`.

</details>

## Вопросы с собеседований

**Чем сеть в GCP отличается от AWS?**
VPC в GCP глобальная, подсети региональные и охватывают все зоны региона. Нет отдельных Internet Gateway и публичных подсетей: выход в интернет — через внешний IP у VM или Cloud NAT. Вместо Security Group — firewall rules уровня VPC, которые применяются по сетевым тегам или сервисным аккаунтам, с приоритетами и запрещающими правилами. Глобальный HTTP-балансировщик даёт один anycast IP для всего мира.

**Что такое проект в GCP и зачем их несколько?**
Основная единица изоляции: ресурсы, включённые API, квоты, права и привязка к счёту. Аналог аккаунта AWS, но создаётся за секунды. Обычно отдельные проекты на окружения (dev, stage, prod) и команды, сгруппированные в папки; права и Organization Policies наследуются сверху вниз.

**Как выдать права приложению в GCP без ключей?**
На VM, в Cloud Run, Cloud Build — привязать к ресурсу сервисный аккаунт с минимальными ролями, токены выдаёт metadata server. В GKE — Workload Identity Federation for GKE, связывающая Kubernetes ServiceAccount с IAM. Для внешних систем (GitHub Actions, GitLab, AWS) — Workload Identity Federation через OIDC. JSON-ключи сервисных аккаунтов — крайний случай, их обычно запрещают Organization Policy.

**Почему опасен Compute Engine default service account?**
VM получают его автоматически, если не указать другой, а в старых проектах у него роль Editor на весь проект. Взломанная VM получает права менять почти всё. Правильно — отдельный SA на каждое приложение с минимальными ролями и запрет автоматической выдачи ролей через Organization Policy.

**Чем отличаются google_project_iam_policy, _binding и _member в Terraform?**
`_policy` заменяет всю IAM-политику проекта и удаляет всё, чего нет в коде, включая служебные привязки Google. `_binding` полностью управляет одной ролью и удаляет других её участников. `_member` добавляет одного участника и ничего не трогает — безопасный вариант по умолчанию. Смешивать `_binding` и `_member` для одной роли нельзя — они будут бесконечно перезаписывать друг друга.

**Балансировщик отдаёт 502, а приложение на VM работает. Что проверить?**
Health check бэкенд-сервиса: проверки идут с адресов Google `35.191.0.0/16` и `130.211.0.0/22`, и файрвол должен их пропускать на порт проверки. Затем — что путь и порт health check совпадают с тем, что слушает приложение, и что named port группы совпадает с портом backend service. Состояние бэкендов показывает `gcloud compute backend-services get-health`.

**GKE Autopilot или Standard?**
Autopilot — Google управляет нодами, оплата за ресурсы подов, меньше операционной работы и безопасные настройки по умолчанию, но нет privileged-подов и доступа к нодам. Standard — свои node pools, полный контроль, нужен для GPU, особых ядер, своих агентов на нодах. Для новых типовых кластеров по умолчанию берут Autopilot.

**Как устроены обновления GKE?**
Кластер подписан на release channel (Rapid, Regular, Stable, Extended), и GKE сам обновляет control plane и ноды. Управлять этим можно maintenance windows (когда обновлять) и maintenance exclusions (когда нельзя). Для прода — регулярный или стабильный канал, региональный кластер, PodDisruptionBudget у приложений и surge-обновление node pools.

**Чем Cloud Run отличается от Lambda?**
Cloud Run запускает любой контейнер, а не функцию в определённом рантайме, и одна копия обрабатывает много запросов одновременно (concurrency по умолчанию 80), тогда как Lambda — один запрос на копию. Масштабируется до нуля, платишь за время обработки запросов. Есть jobs для пакетных задач. Ближайший аналог в AWS — App Runner.

**Чем отличаются классы хранения Cloud Storage от S3 Glacier?**
Nearline, Coldline и Archive отличаются минимальным сроком хранения (30, 90, 365 дней) и ценой чтения, но данные во всех классах доступны сразу, без восстановления на часы, как в Glacier Flexible Retrieval и Deep Archive. При раннем удалении оплачивается весь минимальный срок.

**Что такое Shared VPC и зачем она?**
Сеть живёт в одном host project, а service projects создают ресурсы в её подсетях. Сетевая команда централизованно управляет адресами, файрволом, VPN и Interconnect, а команды приложений — своими ресурсами и счетами. Стандартная схема для компаний с несколькими командами.

**Как сэкономить в GCP?**
Rightsizing по рекомендациям Recommender, CUD на постоянную нагрузку (у старых семейств ещё и автоматические SUD), Spot VM для CI и stateless, Autopilot вместо простаивающих нод, Cloud Run со scale-to-zero, lifecycle и Autoclass в Cloud Storage, exclusion filters в Cloud Logging, удаление неиспользуемых дисков и IP, метки и экспорт биллинга в BigQuery. И помнить, что бюджет только уведомляет, а не останавливает расходы.

**Кто удалил ресурс в проде?**
Cloud Audit Logs, лог Admin Activity — включён всегда и хранится 400 дней: фильтр по `protoPayload.methodName` (например, `delete`) и имени ресурса, поле `principalEmail` покажет, кто это сделал. Для чтения данных (кто скачал файл) нужны Data Access логи, которые по умолчанию выключены.

**У тебя опыт с AWS, а у нас GCP. Как быстро разберёшься?**
Концепции одни и те же: изоляция через аккаунты и проекты, IAM с минимальными правами, VPC, балансировщики, управляемый Kubernetes, IaC на Terraform. Отличаются названия и несколько деталей — глобальная VPC, firewall rules с тегами, IAM на ресурсе, проекты вместо аккаунтов. Terraform, Kubernetes, мониторинг и CI переносятся почти без изменений, поэтому на GCP можно выйти на продуктивную работу за пару недель.
