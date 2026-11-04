# Kubernetes

Подробно о Kubernetes: зачем он нужен, как устроен изнутри, основные объекты и когда какой использовать, планирование подов, ресурсы, probes, сеть, хранилище, безопасность и диагностика. Каждая тема: что это, зачем, как работает, манифест с объяснением и типичные ошибки.

## Практика: Kubernetes The Hard Way

**Репозиторий:** https://github.com/kelseyhightower/kubernetes-the-hard-way (автор Kelsey Hightower).

Это пошаговое руководство по ручной сборке кластера **без kubeadm** (официальная утилита, которая автоматически поднимает кластер одной командой `kubeadm init`) **и без облачных сервисов**: генерируешь собственный центр сертификации (CA) и сертификаты для каждого компонента, пишешь kubeconfig-файлы (файл с адресом API-сервера и учётными данными, по которому клиент подключается к кластеру), поднимаешь etcd (база, где хранится всё состояние кластера), запускаешь компоненты control plane (управляющая часть кластера: API-сервер, планировщик, контроллеры) как systemd-сервисы, настраиваешь kubelet (агент на каждой ноде) и containerd (среда запуска контейнеров) на воркерах, прописываешь маршруты для сети подов, ставишь CoreDNS (DNS внутри кластера) и проверяешь работу.

Зачем проходить, если в работе всё равно EKS: управляемый кластер скрывает внутренности, и при проблемах непонятно, где искать. После «The Hard Way» становится ясно:
- **каждый компонент — просто бинарник с флагами**, никакой магии;
- **все компоненты общаются только через kube-apiserver** (единственное исключение — apiserver с etcd);
- **зачем столько сертификатов**: все соединения взаимно аутентифицированы (mTLS), а kubelet представляется как `system:node:<имя ноды>` в группе `system:nodes`, и специальный Node authorizer даёт ему доступ только к подам своей ноды;
- **сеть подов без «магии» CNI** (CNI, Container Network Interface — плагин, который выдаёт подам IP и соединяет их между нодами): у каждой ноды свой диапазон адресов для подов, а между нодами — обычные статические маршруты;
- **как включается шифрование Secret в etcd** (EncryptionConfiguration — файл для apiserver, в котором указан ключ шифрования; без него Secret лежат в etcd в открытом виде, только в base64).

Дополнения и альтернативы:
- кластер на **kubeadm** на 2–3 виртуальных машинах (Multipass, Vagrant, Lima на Mac — утилиты, которые одной командой поднимают Linux-VM на ноутбуке) — ближе к реальной эксплуатации;
- **killercoda.com** и **killer.sh** — бесплатные интерактивные сценарии в стиле экзаменов CKA и CKAD (Certified Kubernetes Administrator / Application Developer — практические экзамены CNCF в терминале);
- **kind** (Kubernetes in Docker — ноды кластера это Docker-контейнеры), **k3d** (то же для облегчённого k3s), **minikube** — локальный кластер за минуту для экспериментов с манифестами;
- свои репозитории: `~/work/GitHubRepos/mastering-k8s`, `k8s-controller-tutorial`.

## Зачем нужен Kubernetes

Docker запускает контейнеры на **одной** машине. В проде нужно намного больше:
- запускать сотни контейнеров на десятках серверов и **решать, куда поставить каждый**, с учётом свободных ресурсов;
- **перезапускать упавшие** и переносить контейнеры с умершего сервера на живые;
- **масштабировать**: добавлять копии при нагрузке и убирать при спаде;
- **обновлять без простоя**: постепенно заменять старую версию новой и откатываться при проблемах;
- давать сервисам **стабильный адрес и балансировку**, хотя отдельные контейнеры постоянно пересоздаются с новыми IP;
- раздавать **конфигурацию и секреты**, подключать **постоянные диски**.

Kubernetes — **оркестратор**: делает всё это автоматически.

### Главная идея: декларативность и цикл согласования
Ты не говоришь «запусти 3 контейнера» (императивно). Ты описываешь **желаемое состояние** (desired state): «должно работать 3 реплики приложения версии 1.4». Kubernetes сохраняет это описание, а набор **контроллеров** в бесконечном цикле сравнивает желаемое состояние с фактическим (actual state) и устраняет разницу. Этот цикл называется **reconciliation loop** (цикл согласования).

Следствие: упал под — контроллер видит, что работает 2 реплики из 3, и создаёт новую. Ты ничего не делал. Поменял в манифесте версию — контроллер видит расхождение и раскатывает новую версию. Это и называется **self-healing**.

### 🧪 Мини-задание

**Задача.** Создай Deployment `web` на 3 реплики nginx, удали один под и посмотри, что сделает Kubernetes. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create deployment web --image=nginx:1.27 --replicas=3 > /dev/null
kubectl rollout status deployment/web > /dev/null
victim=$(kubectl get pods -l app=web -o name | head -1)
kubectl delete "$victim" --wait=false
sleep 5
kubectl get pods -l app=web --no-headers | awk '{print $1, $3}'
kubectl delete deployment web > /dev/null
```

Ожидаемый результат:

```text
pod "web-6d4cf56db6-4xk2p" deleted
web-6d4cf56db6-9tq7z Running
web-6d4cf56db6-hv5mc Running
web-6d4cf56db6-r2nwd Running
```

Разбор: вместо удалённого пода тут же появился новый с другим именем — ReplicaSet увидел 2 пода вместо 3 и создал недостающий. Это главная идея Kubernetes: ты описываешь желаемое состояние («3 реплики»), а контроллеры постоянно приводят реальность к нему. Так же переживаются падение процесса и потеря целой ноды. Поды одноразовые: их не чинят руками, а заменяют, поэтому нельзя полагаться на имя или IP конкретного пода. Суффиксы имён подов и IP у тебя будут свои.

</details>

## Архитектура кластера

```
           ┌──────────────────── Control Plane ─────────────────────┐
kubectl ─► │  kube-apiserver ◄────► etcd (всё состояние кластера)    │
           │     ▲    ▲    ▲                                         │
           │     │    │    └── kube-scheduler (куда поставить под)   │
           │     │    └─────── kube-controller-manager (контроллеры) │
           │     └──────────── cloud-controller-manager (облако)     │
           └─────▲───────────────────────────────────────────────────┘
                 │  watch: «есть ли для меня работа?»
   ┌─────────────┴────────── Worker node ───────────────────────┐
   │  kubelet ──CRI──► containerd ──► runc ──► контейнеры подов │
   │  kube-proxy (правила для Service)    CNI-плагин (сеть)      │
   └─────────────────────────────────────────────────────────────┘
```

### Control plane (управляющий уровень)
**kube-apiserver** — единственная точка входа в кластер. Через него работают и `kubectl`, и все остальные компоненты. Каждый запрос проходит этапы:
1. **Аутентификация** — кто ты (сертификат, токен, IAM в EKS — в AWS пользователь или роль IAM сопоставляется с пользователем Kubernetes);
2. **Авторизация** — что тебе можно (RBAC, Role-Based Access Control — правила вида «пользователь X может читать поды в namespace Y», см. раздел «Безопасность»);
3. **Admission controllers** (плагины, которые проверяют и правят объект перед сохранением) — сначала mutating (могут изменить объект: добавить sidecar — вспомогательный контейнер, значения по умолчанию), затем validating (могут отклонить: политики безопасности, Kyverno, OPA Gatekeeper — движки политик вида «запрещено запускать образы не из нашего registry»);
4. **Запись в etcd.**

apiserver не хранит состояние сам, поэтому его легко масштабировать горизонтально.

**etcd** — распределённое хранилище «ключ — значение», где лежит **всё** состояние кластера: каждый объект, каждый под, каждый Secret. Для согласованности между репликами использует алгоритм **Raft**, которому нужен **кворум** — большинство узлов:
- 3 узла переживают потерю 1;
- 5 узлов переживают потерю 2;
- поэтому число узлов нечётное: 4 узла переживают ту же одну потерю, что и 3, но медленнее.

Потеря кворума — кластер не может ничего записать. Потеря данных etcd без бэкапа — потеря кластера. Бэкап: `etcdctl snapshot save`. В EKS etcd управляет AWS.

**kube-scheduler** — выбирает ноду для каждого нового пода, у которого ещё не указана нода. Два этапа:
1. **Фильтрация** — отбросить ноды, где под не может работать: не хватает ресурсов (по requests — заявленным подом CPU и памяти), есть taint без toleration (taint — «метка-запрет» на ноде, toleration — разрешение пода её терпеть; см. раздел о планировании), не подходит nodeAffinity (требование пода к меткам ноды), диск в другой зоне;
2. **Оценка (scoring)** — ранжировать оставшиеся: равномерность загрузки, разнесение реплик по нодам и зонам, наличие образа на ноде (не нужно скачивать).

Затем записывает выбранную ноду в поле `spec.nodeName` пода (binding). Сам scheduler ничего не запускает.

**kube-controller-manager** — один процесс, внутри которого работают десятки контроллеров, каждый со своим циклом согласования: Deployment-контроллер, ReplicaSet-контроллер, Node-контроллер (следит за доступностью нод), Job-контроллер, EndpointSlice-контроллер (связывает Service с подами) и другие.

**cloud-controller-manager** — интеграция с облаком: создаёт балансировщики для Service типа LoadBalancer, получает метаданные нод (зона, тип инстанса).

### Компоненты рабочей ноды
**kubelet** — агент на каждой ноде. Следит через apiserver за подами, назначенными на его ноду; через **CRI** (Container Runtime Interface — стандартный интерфейс, через который kubelet управляет любой средой запуска контейнеров) просит containerd скачать образы и запустить контейнеры; выполняет probes (проверки живости и готовности контейнера, см. раздел «Probes»); монтирует volumes; отчитывается о состоянии подов и ноды.

**kube-proxy** — реализует Service: на каждой ноде создаёт правила iptables или IPVS (балансировщик в ядре Linux), которые перенаправляют трафик с виртуального IP сервиса на реальные IP подов. В Cilium его можно заменить на eBPF (программы, работающие прямо в ядре, — быстрее длинных цепочек iptables).

**CNI-плагин** (Container Network Interface) — выдаёт поду IP-адрес и обеспечивает связность подов между нодами. Примеры: Calico, Cilium, Flannel, **AWS VPC CNI** (под получает настоящий IP из подсети VPC).

**CoreDNS** — DNS-сервер внутри кластера: сервис `api` в namespace `prod` доступен по имени `api.prod.svc.cluster.local`.

### Что происходит при `kubectl apply -f deployment.yaml`
Это классический вопрос. Ответ показывает понимание архитектуры:
1. **kubectl** читает kubeconfig, отправляет манифест в apiserver.
2. **apiserver**: аутентификация, авторизация RBAC, admission controllers, сохранение Deployment в **etcd**.
3. **Deployment-контроллер** (через watch на apiserver) видит новый Deployment и создаёт **ReplicaSet**.
4. **ReplicaSet-контроллер** видит ReplicaSet, которому нужно 3 пода, и создаёт 3 объекта **Pod** без назначенной ноды.
5. **Scheduler** видит неназначенные поды, выбирает ноды и записывает `nodeName`.
6. **kubelet** на выбранной ноде видит, что ему назначен под: containerd скачивает образ, CNI выдаёт IP, монтируются volumes, запускаются init-контейнеры (подготовительные, выполняются по очереди до основных), потом основные.
7. kubelet выполняет probes и обновляет статус пода. Когда **readiness probe** (проверка «готов принимать трафик») проходит, EndpointSlice-контроллер добавляет IP пода в EndpointSlice сервиса (список адресов готовых подов), и **kube-proxy** на всех нодах обновляет правила. Трафик пошёл на новый под.

Заметь: компоненты не вызывают друг друга напрямую. Каждый только **наблюдает (watch)** изменения через apiserver и записывает свой результат туда же.

### 🧪 Мини-задание

**Задача.** Найди в кластере все компоненты control plane и ноды: на какой ноде они работают и какие из них запущены как static pod'ы (поды, которые kubelet запускает сам из файлов в `/etc/kubernetes/manifests`, без участия apiserver), а какие — через DaemonSet (контроллер «по одному поду на каждой ноде»). Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl get pods -n kube-system -o custom-columns=POD:.metadata.name,NODE:.spec.nodeName,OWNER:.metadata.ownerReferences[0].kind --no-headers | sort
```

Ожидаемый результат:

```text
coredns-6f6b679f8f-7lcmh                     lab-control-plane   ReplicaSet
coredns-6f6b679f8f-xq2bz                     lab-control-plane   ReplicaSet
etcd-lab-control-plane                       lab-control-plane   Node
kindnet-4kq8r                                lab-worker          DaemonSet
kindnet-9wz5c                                lab-control-plane   DaemonSet
kindnet-p7tmx                                lab-worker2         DaemonSet
kube-apiserver-lab-control-plane             lab-control-plane   Node
kube-controller-manager-lab-control-plane    lab-control-plane   Node
kube-proxy-5hdm2                             lab-worker2         DaemonSet
kube-proxy-g8wqv                             lab-worker          DaemonSet
kube-proxy-r4nkt                             lab-control-plane   DaemonSet
kube-scheduler-lab-control-plane             lab-control-plane   Node
```

Разбор: владелец `Node` значит static pod: etcd, apiserver, controller-manager и scheduler kubelet запускает сам из манифестов на диске control plane ноды (поэтому у них в имени имя ноды). `kube-proxy` и CNI (`kindnet`) — DaemonSet'ы, по одному на каждую ноду. CoreDNS — обычный Deployment. В EKS control plane скрыт и управляется AWS, а в `kube-system` видны только аддоны: CoreDNS, kube-proxy и VPC CNI (`aws-node`). Суффиксы имён подов и IP у тебя будут свои.

</details>

## Основные объекты

Все объекты описываются YAML-манифестами с общей структурой:
```yaml
apiVersion: apps/v1         # группа и версия API объекта
kind: Deployment            # тип объекта
metadata:
  name: api                 # имя, уникальное в namespace
  namespace: prod
  labels:                   # метки — пары ключ-значение для группировки и выбора
    app: api
spec:                       # желаемое состояние — что ты хочешь
  ...
status:                     # фактическое состояние — заполняет Kubernetes, руками не пишут
  ...
```
**Labels и selectors** — основной механизм связей между объектами. Deployment находит свои поды, Service — свои бэкенды, NetworkPolicy — свои цели не по именам, а по меткам: «все поды с `app: api`».

### Pod
Минимальная единица запуска: **один или несколько контейнеров**, которые всегда размещаются на одной ноде и делят:
- **сеть** — один IP на под, контейнеры общаются через `localhost`;
- **volumes** — могут монтировать одни и те же тома.

Зачем несколько контейнеров в поде: вспомогательный контейнер (**sidecar**) рядом с основным — прокси service mesh (Envoy), агент сбора логов, процесс, обновляющий конфиг. **Init-контейнеры** выполняются по очереди **до** основных и должны успешно завершиться: подготовить конфиг, дождаться БД, выполнить миграцию. С версии 1.29 есть «нативные sidecar»: init-контейнер с `restartPolicy: Always` запускается до основного и работает всё время жизни пода.

Поды **эфемерны**: при пересоздании под получает новое имя и новый IP. Поэтому поды почти никогда не создают напрямую, а используют контроллеры (Deployment и другие), и обращаются к ним через Service (объект со стабильным именем и IP перед группой подов, см. ниже).

### Deployment и ReplicaSet — для stateless-приложений
**ReplicaSet** поддерживает заданное число одинаковых подов. **Deployment** управляет ReplicaSet'ами и добавляет обновление версий и откат. Напрямую ReplicaSet почти не создают. Stateless-приложение — такое, которое не хранит важных данных у себя на диске (веб-API, фронтенд, воркеры очереди): любую копию можно убить и запустить заново где угодно.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api                 # «мои поды — те, у кого метка app=api»
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1              # сколько подов можно создать СВЕРХ replicas во время обновления
      maxUnavailable: 0        # сколько может быть недоступно — 0 значит без снижения ёмкости
  template:                    # шаблон пода
    metadata:
      labels:
        app: api               # должна совпадать с selector
    spec:
      containers:
        - name: api
          image: registry.example.com/api:1.4.2
          ports:
            - containerPort: 8080
```
**Как идёт обновление.** При изменении шаблона (например, тега образа) Deployment создаёт **новый ReplicaSet** и постепенно увеличивает его, одновременно уменьшая старый, с учётом `maxSurge` и `maxUnavailable`. Новый под считается доступным, когда проходит readiness probe. Старый ReplicaSet остаётся с 0 реплик — к нему можно откатиться:
```bash
kubectl rollout status deploy/api      # следить за раскаткой
kubectl rollout history deploy/api     # история ревизий
kubectl rollout undo deploy/api        # откатиться на предыдущую ревизию
kubectl rollout restart deploy/api     # пересоздать все поды (например, чтобы подхватить новый Secret)
```
Стратегия `Recreate` — сначала удалить все старые поды, потом создать новые. Будет простой, но нужна, когда две версии не могут работать одновременно.

### StatefulSet — для приложений с состоянием
Для баз данных, Kafka (распределённая очередь сообщений), ZooKeeper (сервис координации кластеров), Elasticsearch (поисковая база): им важна идентичность каждой реплики — у каждой свои данные, и реплики знают друг друга по именам.

Чем отличается от Deployment:
- **стабильные имена**: `kafka-0`, `kafka-1`, `kafka-2` — после пересоздания под получает **то же** имя;
- **стабильный DNS** для каждого пода через headless Service (Service без виртуального IP, см. ниже): `kafka-0.kafka.prod.svc.cluster.local` (по нему реплики находят друг друга);
- **свой постоянный диск у каждой реплики** через `volumeClaimTemplates` (шаблон заявки на диск — PVC, см. раздел «Хранилище»): `kafka-0` после пересоздания подключится к **своему** диску `data-kafka-0`;
- **упорядоченный** запуск (0, затем 1, затем 2 — каждый после готовности предыдущего) и остановка в обратном порядке.

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
spec:
  serviceName: postgres          # headless Service для DNS-имён подов
  replicas: 3
  selector:
    matchLabels: { app: postgres }
  template:
    metadata:
      labels: { app: postgres }
    spec:
      containers:
        - name: postgres
          image: postgres:16
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:          # для каждой реплики создаётся свой PVC: data-postgres-0, data-postgres-1...
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: gp3
        resources:
          requests:
            storage: 50Gi
```
При удалении StatefulSet его PVC **не удаляются** — защита от потери данных.

### DaemonSet — по одному поду на каждой ноде
Для агентов, которые должны работать на каждой ноде: сбор логов (Fluent Bit — лёгкий агент, читающий логи контейнеров с диска ноды и отправляющий их в хранилище), метрики ноды (node-exporter — отдаёт Prometheus метрики CPU, памяти, дисков ноды), CNI-плагин, kube-proxy. При добавлении ноды под появляется на ней автоматически. С помощью nodeSelector (выбор нод по меткам) и tolerations можно ограничить набор нод или, наоборот, разрешить запуск на нодах с taint.

### Job и CronJob — разовые и периодические задачи
**Job** запускает под (или несколько) до **успешного завершения**: миграция БД, разовая обработка данных. `backoffLimit` — сколько раз повторять при неудаче (паузы между попытками растут экспоненциально), `activeDeadlineSeconds` — общий таймаут, после которого Job считается проваленным.

**CronJob** создаёт Job по расписанию (формат cron):
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: backup
spec:
  schedule: "0 3 * * *"            # каждый день в 03:00
  concurrencyPolicy: Forbid         # не запускать новую, если предыдущая ещё работает
  successfulJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 2
      template:
        spec:
          restartPolicy: OnFailure  # для Job: OnFailure или Never (Always нельзя)
          containers:
            - name: backup
              image: registry.example.com/backup:1.0
```

### Service — стабильный адрес для набора подов
**Проблема.** Поды пересоздаются и получают новые IP, их несколько. Как другим сервисам к ним обращаться?

**Service** даёт **постоянный виртуальный IP (ClusterIP) и DNS-имя** для всех подов, выбранных по меткам, и балансирует трафик между ними.
```yaml
apiVersion: v1
kind: Service
metadata:
  name: api
spec:
  selector:
    app: api              # трафик идёт на поды с этой меткой, которые готовы (Ready)
  ports:
    - port: 80            # порт сервиса — на него обращаются клиенты
      targetPort: 8080    # порт контейнера, куда реально пойдёт трафик
```
Теперь другие поды обращаются к `http://api` (в том же namespace) или `http://api.prod.svc.cluster.local`.

**Как это работает.** У сервиса нет процесса. Его IP **виртуальный**: никакой интерфейс его не имеет. EndpointSlice-контроллер ведёт список IP готовых подов, а kube-proxy на **каждой** ноде пишет правила iptables или IPVS: «пакет на ClusterIP:80 подменить (DNAT) на IP одного из подов:8080». Балансировка происходит прямо на ноде клиента.

**Типы Service:**
- **ClusterIP** (по умолчанию) — доступен только внутри кластера.
- **NodePort** — дополнительно открывает порт (30000–32767) на **каждой** ноде; трафик на `любая_нода:порт` попадает в сервис.
- **LoadBalancer** — дополнительно облако создаёт внешний балансировщик (в AWS с AWS Load Balancer Controller — контроллером, который по объектам Kubernetes создаёт и настраивает балансировщики AWS, — это NLB), направленный на сервис.
- **ExternalName** — DNS-псевдоним (CNAME) на внешнее имя, например на RDS (управляемая база данных AWS): приложение обращается к `db`, а адрес базы можно поменять в одном месте.
- **Headless** (`clusterIP: None`) — без виртуального IP; DNS-запрос возвращает IP всех подов напрямую. Нужен для StatefulSet и клиентской балансировки.

### Ingress и Gateway API — HTTP-маршрутизация снаружи
Создавать отдельный балансировщик (`type: LoadBalancer`) на каждый сервис дорого. **Ingress** описывает L7-правила: один вход для многих сервисов, маршрутизация по домену и пути, TLS.
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: main
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing   # параметры конкретного контроллера
spec:
  ingressClassName: alb
  tls:
    - hosts: [api.example.com]
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /v1
            pathType: Prefix
            backend:
              service:
                name: api
                port:
                  number: 80
```
Сам по себе объект Ingress ничего не делает: нужен **Ingress Controller**, который его читает и настраивает реальный балансировщик (ingress-nginx, AWS Load Balancer Controller — создаёт ALB, Traefik). **Gateway API** — новый стандарт на замену Ingress: более выразительный, с разделением ролей (инфраструктурная команда управляет Gateway, команда приложения — маршрутами HTTPRoute).

### ConfigMap и Secret — конфигурация
Выносят настройки из образа, чтобы один образ работал во всех окружениях.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: api-config
data:
  LOG_LEVEL: info
  appsettings.json: |
    { "Feature": { "NewCheckout": true } }
---
apiVersion: v1
kind: Secret
metadata:
  name: api-secrets
type: Opaque
stringData:                     # stringData — открытым текстом, Kubernetes сам закодирует в base64
  DB_PASSWORD: s3cr3t
```
Подключение к поду — как переменные окружения или как файлы:
```yaml
containers:
  - name: api
    envFrom:
      - configMapRef: { name: api-config }     # все ключи как переменные окружения
    env:
      - name: DB_PASSWORD
        valueFrom:
          secretKeyRef: { name: api-secrets, key: DB_PASSWORD }
    volumeMounts:
      - name: config
        mountPath: /app/config                 # ключи ConfigMap как файлы в этой папке
volumes:
  - name: config
    configMap: { name: api-config }
```
Особенности:
- **Secret — это base64, а не шифрование.** Любой с правом читать Secret увидит значение. Защита: RBAC на Secret, шифрование в etcd (в EKS — через KMS, Key Management Service — сервис ключей шифрования AWS), а значения в git не хранят — используют External Secrets Operator (контроллер, который сам тянет секреты из AWS Secrets Manager или Vault и создаёт из них Secret), Sealed Secrets (секрет шифруется публичным ключом кластера, и зашифрованную версию можно безопасно коммитить) или SOPS (шифрование значений в YAML-файлах ключом из KMS).
- **Переменные окружения не обновляются** при изменении ConfigMap — нужен перезапуск пода. Файлы из volume обновляются (с задержкой до минуты), но приложение должно уметь перечитать конфиг. Распространённый приём — хеш конфига в аннотации шаблона пода (Helm `checksum/config` — в аннотацию кладётся sha256 содержимого ConfigMap) или `configMapGenerator` в Kustomize (добавляет хеш к имени ConfigMap): изменение конфига меняет шаблон, и Deployment автоматически перекатывает поды.

### Хранилище: PersistentVolume, PersistentVolumeClaim, StorageClass
**Проблема.** Файловая система контейнера исчезает вместе с ним. Базе данных нужен диск, который переживёт пересоздание пода и переезд на другую ноду.

Три объекта разделяют ответственность:
- **PersistentVolumeClaim (PVC)** — **заявка** приложения: «нужен диск 50 GB с режимом ReadWriteOnce класса gp3». Пишет разработчик.
- **StorageClass** — **тип** хранилища и способ его создания: какой драйвер (EBS CSI — CSI, Container Storage Interface, стандартный плагин, через который Kubernetes создаёт и подключает диски конкретного хранилища), какие параметры (gp3, шифрование), что делать с диском после удаления заявки.
- **PersistentVolume (PV)** — **конкретный** диск. При динамическом выделении создаётся автоматически по PVC.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  encrypted: "true"
reclaimPolicy: Delete                     # удалить EBS-том при удалении PVC (Retain — оставить)
volumeBindingMode: WaitForFirstConsumer   # создавать диск, когда под уже назначен на ноду
allowVolumeExpansion: true
```
**Почему важен `WaitForFirstConsumer`.** EBS-том существует в **одной** зоне доступности. Если создать диск сразу (`Immediate`), он может оказаться в зоне `us-east-2a`, а scheduler поставит под на ноду в `us-east-2b` — под не сможет подключить диск и зависнет в Pending. С `WaitForFirstConsumer` диск создаётся в зоне ноды, на которую уже назначен под.

**Режимы доступа:**
- `ReadWriteOnce` (RWO) — чтение и запись с одной ноды (EBS);
- `ReadOnlyMany` (ROX) — только чтение со многих нод;
- `ReadWriteMany` (RWX) — чтение и запись со многих нод (EFS — управляемый NFS в AWS, NFS): например, общая папка загрузок для нескольких реплик веб-приложения;
- `ReadWriteOncePod` (RWOP) — только один под.

### Namespace, ResourceQuota, LimitRange
**Namespace** — логическое разделение кластера: по командам, окружениям, приложениям. Имена объектов уникальны внутри namespace. На namespace назначают права (RBAC), квоты и сетевые политики. Не являются границей безопасности сами по себе: поды из разных namespace по умолчанию видят друг друга по сети.

**ResourceQuota** — лимит на суммарное потребление в namespace (не больше 20 CPU в requests, не больше 50 подов). **LimitRange** — значения requests и limits по умолчанию для контейнеров, где они не указаны.

### 🧪 Мини-задание

**Задача.** Создай Deployment и пройди по цепочке владельцев вверх: от пода к ReplicaSet и от ReplicaSet к Deployment. Затем удали Deployment и проверь, что стало с остальными объектами. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create deployment api --image=nginx:1.27 > /dev/null
kubectl rollout status deployment/api > /dev/null
pod=$(kubectl get pods -l app=api -o jsonpath='{.items[0].metadata.name}')
rs=$(kubectl get pod $pod -o jsonpath='{.metadata.ownerReferences[0].name}')
echo "pod $pod -> $(kubectl get pod $pod -o jsonpath='{.metadata.ownerReferences[0].kind}') $rs"
echo "rs $rs -> $(kubectl get rs $rs -o jsonpath='{.metadata.ownerReferences[0].kind}/{.metadata.ownerReferences[0].name}')"
kubectl delete deployment api > /dev/null; sleep 5
kubectl get rs,pods -l app=api 2>&1
```

Ожидаемый результат:

```text
pod api-5d8c6b7f9c-qw4zr -> ReplicaSet api-5d8c6b7f9c
rs api-5d8c6b7f9c -> Deployment/api
No resources found in default namespace.
```

Разбор: Deployment не управляет подами напрямую — он создаёт ReplicaSet (по одному на каждую версию шаблона пода), а ReplicaSet создаёт поды. Связь хранится в `ownerReferences`. Суффикс `5d8c6b7f9c` — хеш шаблона пода: при смене образа появится новый ReplicaSet с другим хешем, а старый останется для отката. При удалении Deployment сборщик мусора по `ownerReferences` удалил и ReplicaSet, и поды (каскадное удаление; `--cascade=orphan` оставил бы их). Суффиксы имён подов и IP у тебя будут свои.

</details>

## Ресурсы: requests и limits

```yaml
resources:
  requests:
    cpu: 250m          # 250 милликор = четверть ядра
    memory: 256Mi
  limits:
    cpu: "1"
    memory: 512Mi
```
**requests** — гарантированный минимум. **Используется scheduler'ом**: под ставится только на ноду, где сумма requests всех подов плюс этого не превышает доступные ресурсы. Фактическое потребление scheduler не смотрит.

**limits** — потолок. Поведение при превышении разное:
- **память**: процесс убивается OOM killer'ом (механизм ядра Linux, убивающий процесс при нехватке памяти; **OOMKilled**, exit code 137), контейнер перезапускается. Память нельзя «отнять» у процесса.
- **CPU**: процесс не убивают, его **притормаживают (throttling)**: в каждом периоде 100 мс он получает не больше квоты (limit `1` = 100 мс CPU-времени за каждые 100 мс). Приложение становится медленным, растёт задержка. Типичная картина: средняя загрузка CPU пода 30%, а ответы тормозят — в коротких всплесках он выбирает квоту и ждёт следующего периода (видно по метрике `container_cpu_cfs_throttled_periods_total`).

### Классы QoS
По сочетанию requests и limits под получает класс QoS (Quality of Service), определяющий, кого выселять (eviction — kubelet сам удаляет поды с ноды, чтобы её спасти) первым, когда на ноде не хватает памяти:
- **Guaranteed** — у всех контейнеров requests равны limits для CPU и памяти. Выселяется последним.
- **Burstable** — requests заданы, но меньше limits (или limits не заданы).
- **BestEffort** — ничего не задано. Выселяется **первым**.

### Практические рекомендации
- **Всегда задавай requests.** Без них scheduler считает под «бесплатным», ноды переполняются, HPA (Horizontal Pod Autoscaler — автоматически меняет число реплик по загрузке) не может считать загрузку в процентах, ведь процент считается от requests.
- **Memory limit задавай** (часто равным request), чтобы утечка памяти одного пода не уронила ноду.
- **CPU limit — спорный вопрос.** Многие его не ставят: throttling вредит задержкам, а при наличии requests нода и так делит CPU пропорционально. Ставят, если нужна предсказуемость или жёсткое разделение арендаторов (tenants — разные команды или клиенты на одном кластере).
- Значения подбирать по метрикам реального потребления (Prometheus, VPA — Vertical Pod Autoscaler — в режиме рекомендаций: он смотрит на фактическое потребление и советует requests, ничего не меняя сам).

### 🧪 Мини-задание

**Задача.** Запусти под с `requests.memory: 32Mi` и `limits.memory: 64Mi`, процесс в котором пытается занять 200 MB. Узнай причину и код завершения контейнера и QoS-класс пода. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl apply -f - > /dev/null <<'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: hungry
spec:
  restartPolicy: Never
  containers:
    - name: app
      image: python:3.12-slim
      command: ["python", "-c", "b = bytearray(200 * 1024 * 1024)"]
      resources:
        requests: {memory: 32Mi, cpu: 50m}
        limits: {memory: 64Mi}
EOF
sleep 30
kubectl get pod hungry -o jsonpath='{.status.containerStatuses[0].state.terminated.reason} {.status.containerStatuses[0].state.terminated.exitCode} {.status.qosClass}{"\n"}'
kubectl delete pod hungry > /dev/null
```

Ожидаемый результат:

```text
OOMKilled 137 Burstable
```

Разбор: limit памяти — жёсткая граница cgroup. При выходе за неё ядро убивает процесс (`OOMKilled`, 128 + 9 = 137), а с `restartPolicy: Always` под ушёл бы в CrashLoopBackOff. Requests — это не ограничение, а резерв: по ним планировщик решает, на какую ноду поставить под. QoS `Burstable` — requests меньше limits. При `requests == limits` по всем ресурсам будет `Guaranteed` (такие поды выселяют последними), без requests и limits — `BestEffort` (выселяют первыми). Превышение CPU limit не убивает под, а троттлит.

</details>

## Probes — проверки здоровья

**livenessProbe — «жив ли процесс?»** Если проверка проваливается (несколько раз подряд), kubelet **перезапускает контейнер**. Нужна, когда приложение может зависнуть, не упав (deadlock — взаимная блокировка: потоки ждут друг друга вечно, процесс жив, но ничего не обрабатывает).

**readinessProbe — «готов ли принимать трафик?»** Если проваливается, под **убирается из Endpoints** сервиса (списка адресов, на которые Service шлёт трафик) — трафик на него перестаёт идти, но под **не перезапускается**. Нужна при старте (прогрев кэша, загрузка данных) и при временной перегрузке.

**startupProbe — «завершился ли запуск?»** Пока не прошла, liveness и readiness не выполняются. Для приложений с долгим стартом (Java), чтобы liveness не убивала их во время загрузки.

```yaml
containers:
  - name: api
    startupProbe:
      httpGet: { path: /health/startup, port: 8080 }
      failureThreshold: 30       # 30 попыток × 10 с = до 5 минут на старт
      periodSeconds: 10
    livenessProbe:
      httpGet: { path: /health/live, port: 8080 }
      periodSeconds: 10
      timeoutSeconds: 3
      failureThreshold: 3        # 3 провала подряд → перезапуск
    readinessProbe:
      httpGet: { path: /health/ready, port: 8080 }
      periodSeconds: 5
      failureThreshold: 2
```
**Главная ошибка — проверять зависимости в liveness.** Если liveness-эндпоинт проверяет базу данных, то при недоступности БД **все** поды провалят liveness и начнут перезапускаться по кругу: сервис полностью ляжет, хотя мог бы отдавать кэш или понятную ошибку. Liveness должен проверять только сам процесс («я не завис»). Зависимости можно (осторожно) учитывать в readiness.

### 🧪 Мини-задание

**Задача.** Разверни nginx с readinessProbe на несуществующий путь `/ready` и Service к нему. Посмотри на статус пода, его готовность и на эндпоинты Service. Затем поправь путь на `/` и проверь снова. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create deployment probe --image=nginx:1.27 > /dev/null
kubectl expose deployment probe --port=80 > /dev/null
kubectl patch deployment probe --type=json -p '[{"op": "add", "path": "/spec/template/spec/containers/0/readinessProbe",
  "value": {"httpGet": {"path": "/ready", "port": 80}, "periodSeconds": 2}}]' > /dev/null
sleep 20
kubectl get pods -l app=probe --no-headers | awk '{print $2, $3}'
kubectl get endpointslices -l kubernetes.io/service-name=probe -o jsonpath='endpoints ready: {.items[0].endpoints[*].conditions.ready}{"\n"}'
kubectl patch deployment probe --type=json -p '[{"op": "replace", "path": "/spec/template/spec/containers/0/readinessProbe/httpGet/path", "value": "/"}]' > /dev/null
kubectl rollout status deployment/probe > /dev/null
kubectl get pods -l app=probe --no-headers | awk '{print $2, $3}'
kubectl delete deployment,service probe > /dev/null
```

Ожидаемый результат:

```text
0/1 Running
endpoints ready: false
1/1 Running
```

Разбор: под `Running`, но не `Ready`: процесс жив, а readinessProbe получает 404. Такой под исключён из балансировки — в EndpointSlice он помечен `ready: false`, и трафик к нему не идёт. Именно это защищает пользователей, пока приложение прогревается или временно не может обслуживать запросы. Rollout с неверной пробой тоже не завершился бы: новый под не становится Ready, и старые поды не удаляются. Liveness-проба при провале перезапускала бы контейнер — поэтому её не делают зависимой от базы и внешних сервисов, иначе сбой базы перезапустит все поды разом.

</details>

## Планирование: куда попадёт под

### nodeSelector — простейший вариант
```yaml
nodeSelector:
  node.kubernetes.io/instance-type: m6i.large   # только на ноды с этой меткой
```

### Affinity и anti-affinity
```yaml
affinity:
  nodeAffinity:                                   # требования к НОДЕ
    requiredDuringSchedulingIgnoredDuringExecution:   # обязательно
      nodeSelectorTerms:
        - matchExpressions:
            - key: karpenter.sh/capacity-type
              operator: In
              values: ["on-demand"]
  podAntiAffinity:                                # отношение к ДРУГИМ ПОДАМ
    preferredDuringSchedulingIgnoredDuringExecution:  # желательно
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels: { app: api }
          topologyKey: kubernetes.io/hostname     # «не ставь две реплики api на одну ноду»
```
- `required...` — жёсткое правило: если не выполнить, под останется Pending (ждёт, пока появится подходящая нода). В примере выше — «только on-demand ноды» (обычные инстансы по полной цене, в отличие от spot — дешёвых, которые AWS может отобрать с предупреждением за 2 минуты).
- `preferred...` — пожелание: scheduler постарается, но поставит и так.
- `IgnoredDuringExecution` — правило проверяется только при размещении; уже работающие поды не выселяются, если метки ноды изменились.
- `topologyKey` — что считать «одним местом»: нода (`kubernetes.io/hostname`) или зона доступности (`topology.kubernetes.io/zone`).

Зачем anti-affinity: если все реплики окажутся на одной ноде или в одной зоне, их падение уронит сервис целиком.

### topologySpreadConstraints — равномерное распределение
Удобнее anti-affinity для равномерного разнесения:
```yaml
topologySpreadConstraints:
  - maxSkew: 1                                 # разница в числе подов между зонами не больше 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule           # или ScheduleAnyway — мягкое правило
    labelSelector:
      matchLabels: { app: api }
```
6 реплик в 3 зонах разложатся по 2.

### Taints и tolerations
**Taint** («пятно») на ноде **отталкивает** поды. **Toleration** на поде **разрешает** ему игнорировать пятно.
```bash
kubectl taint nodes gpu-node-1 dedicated=gpu:NoSchedule   # обычные поды сюда больше не попадут
```
```yaml
tolerations:                       # этому поду можно на ноды с таким taint
  - key: dedicated
    operator: Equal
    value: gpu
    effect: NoSchedule
```
Эффекты: `NoSchedule` (новые поды без toleration не ставятся), `PreferNoSchedule` (мягкая версия), `NoExecute` (ещё и **выселяет** уже работающие поды без toleration).

**Важно: toleration разрешает, но не притягивает.** Под с toleration может уехать и на обычную ноду. Чтобы выделить ноды **только** под определённые поды, нужны оба механизма: taint на ноде (чужие не придут) плюс nodeAffinity или nodeSelector у подов (свои пойдут именно туда).

Kubernetes сам ставит taint при проблемах: `node.kubernetes.io/not-ready`, `unreachable`, `memory-pressure`, `disk-pressure`.

### 🧪 Мини-задание

**Задача.** Создай под с `nodeSelector: disktype=ssd`, когда ни у одной ноды такой метки нет. Посмотри статус и событие планировщика, затем повесь метку на `lab-worker2` и проверь, куда попал под. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl run picky --image=nginx:1.27 --overrides='{"spec": {"nodeSelector": {"disktype": "ssd"}}}' > /dev/null
sleep 5
kubectl get pod picky -o jsonpath='{.status.phase}{"\n"}'
kubectl get events --field-selector involvedObject.name=picky,reason=FailedScheduling -o jsonpath='{.items[0].message}' | cut -c1-120; echo
kubectl label node lab-worker2 disktype=ssd > /dev/null
sleep 10
kubectl get pod picky -o jsonpath='{.status.phase} on {.spec.nodeName}{"\n"}'
kubectl delete pod picky > /dev/null; kubectl label node lab-worker2 disktype- > /dev/null
```

Ожидаемый результат:

```text
Pending
0/3 nodes are available: 1 node(s) had untolerated taint {node-role.kubernetes.io/control-plane: }, 2 node(s) didn'
Running on lab-worker2
```

Разбор: под не может быть запланирован, пока нет подходящей ноды, и висит в `Pending`. Событие `FailedScheduling` объясняет причину по каждой ноде: control plane закрыт taint'ом, а у двух воркеров нет нужной метки (сообщение продолжается: «didn't match Pod's node affinity/selector»). Как только метка появилась, планировщик сам поставил под. `nodeSelector` — простейший способ, гибче `nodeAffinity` (условия «предпочтительно», операторы In и NotIn), а taints и tolerations работают наоборот: нода отталкивает поды без нужной толерантности. `Pending` всегда начинают разбирать с `kubectl describe pod` и событий.

</details>

## Масштабирование
- **HPA (Horizontal Pod Autoscaler)** — меняет количество реплик по метрике: CPU, память (относительно **requests**), или по кастомным и внешним метрикам (длина очереди, RPS — запросов в секунду). Нужен **metrics-server** (компонент, который собирает с kubelet текущее потребление CPU и памяти подов; его же использует `kubectl top`) для CPU и памяти, а для кастомных — Prometheus Adapter (отдаёт HPA метрики из Prometheus) или **KEDA** (Kubernetes Event-Driven Autoscaling: умеет масштабировать по очередям SQS, Kafka и до нуля, когда работы нет).
  ```yaml
  apiVersion: autoscaling/v2
  kind: HorizontalPodAutoscaler
  metadata: { name: api }
  spec:
    scaleTargetRef: { apiVersion: apps/v1, kind: Deployment, name: api }
    minReplicas: 3
    maxReplicas: 20
    metrics:
      - type: Resource
        resource:
          name: cpu
          target: { type: Utilization, averageUtilization: 70 }   # 70% от requests
  ```
- **VPA (Vertical Pod Autoscaler)** — подбирает requests и limits. Часто используют только в режиме рекомендаций. Не совмещают с HPA по той же метрике.
- **Cluster Autoscaler** — добавляет ноды, когда поды висят в Pending из-за нехватки ресурсов, и убирает недогруженные. Работает через группы автомасштабирования (ASG, Auto Scaling Group — группа одинаковых EC2-инстансов, число которых AWS держит на заданном уровне).
- **Karpenter** (AWS) — то же, но быстрее и гибче: смотрит на требования pending-подов и сам подбирает подходящий тип инстанса (включая spot — дешёвые инстансы из свободных мощностей AWS, которые могут отобрать), объединяет недогруженные ноды (consolidation — переселяет поды и выключает лишние ноды). Подробно — в следующем разделе.

### Скорость реакции HPA: behavior
HPA не реагирует мгновенно, и это сделано намеренно, чтобы реплики не «скакали» туда-обратно при колебаниях нагрузки. Параметр `stabilizationWindowSeconds` говорит, за какой период HPA учитывает прошлые рекомендации. По умолчанию:
- **масштабирование вверх — 0 секунд**: реплики добавляются сразу;
- **масштабирование вниз — 300 секунд**: HPA ждёт 5 минут устойчиво низкой нагрузки, прежде чем убрать реплики.

```yaml
spec:
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
        - type: Percent
          value: 100              # за 15 секунд можно удвоить число реплик
          periodSeconds: 15
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Pods
          value: 2                # убирать не больше 2 подов в минуту
          periodSeconds: 60
```
Короткая команда для быстрого эксперимента: `kubectl autoscale deployment api --cpu-percent=50 --min=3 --max=10`. В первые 1–2 минуты в колонке TARGETS будет `<unknown>/50%` — HPA ещё не получил метрики от metrics-server.

### Как проверить автомасштабирование нагрузкой
Сценарий из EKS-workshop (официальный практикум AWS по EKS — eksworkshop.com): создать CPU-нагрузку, посмотреть, как HPA добавляет поды, а Karpenter — ноды под них.
```bash
# 50 параллельных клиентов, по 10 запросов в секунду каждый, в течение часа
kubectl run load-generator --image=williamyeh/hey:latest --restart=Never -- \
  -c 50 -q 10 -z 60m http://api.prod.svc:8080/

kubectl get hpa -n prod -w                  # TARGETS растёт выше 50%, REPLICAS увеличивается
kubectl get pods -n prod                    # часть новых подов в Pending — им не хватает нод
kubectl get nodes -l karpenter.sh/nodepool  # Karpenter создаёт ноды под pending-поды

kubectl delete pod load-generator           # снять нагрузку
# через ~5 минут (stabilization window) HPA уменьшит реплики,
# затем Karpenter удалит опустевшие и недогруженные ноды
```

### VPA подробнее: подбор requests и limits
**VPA (Vertical Pod Autoscaler)** смотрит на фактическое потребление CPU и памяти подов за последние дни и рекомендует (или сам выставляет) requests. Зачем: requests «на глаз» почти всегда неточные. Завышенные — ноды простаивают, а ты платишь за зарезервированное, но неиспользуемое. Заниженные — поды попадают на перегруженные ноды и ловят OOMKilled (процесс убит ядром за превышение лимита памяти) или троттлинг CPU (ядро притормаживает контейнер, превысивший limit).

VPA ставится отдельно (в кластере его нет по умолчанию) и состоит из трёх частей:
- **recommender** — считает рекомендации по истории метрик;
- **updater** — пересоздаёт поды, у которых requests сильно отличаются от рекомендации (с учётом PodDisruptionBudget);
- **admission controller** — при создании пода подставляет рекомендованные requests.

Режимы (`updateMode`):
| Режим | Что делает | Когда |
|---|---|---|
| `Off` | только считает рекомендации, поды не трогает | старт и прод: смотришь цифры и переносишь в манифесты руками |
| `Initial` | выставляет requests только новым подам | когда перезапуски ради ресайза недопустимы |
| `Recreate` (и устаревший `Auto`, который сейчас работает так же) | пересоздаёт поды с новыми requests | batch-задачи, некритичные сервисы с несколькими репликами |
| `InPlaceOrRecreate` (в новых версиях VPA) | меняет requests работающего пода без перезапуска, если кластер умеет in-place resize (Kubernetes 1.33+), иначе пересоздаёт | когда версия кластера и VPA позволяют |

```yaml
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: api
  namespace: prod
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api
  updatePolicy:
    updateMode: "Off"                 # только рекомендации
  resourcePolicy:
    containerPolicies:
      - containerName: api
        controlledResources: ["memory"]   # CPU отдаём HPA, VPA следит только за памятью
        minAllowed: { memory: 256Mi }
        maxAllowed: { memory: 4Gi }
```
```bash
kubectl describe vpa api -n prod
# → Recommendation:
# →   Container Recommendations:
# →     Container Name: api
# →     Lower Bound:  memory: 410Mi
# →     Target:       memory: 620Mi     ← что ставить в requests
# →     Upper Bound:  memory: 1.4Gi
```

**HPA и VPA вместе.** Нельзя давать обоим одну метрику: HPA по CPU добавляет реплики, когда нагрузка на под растёт, а VPA в ответ на ту же нагрузку увеличивает CPU пода — два контроллера дёргают систему в разные стороны. Рабочие схемы: HPA по CPU или по RPS + VPA только по памяти; либо HPA по своей метрике (очередь, RPS) + VPA по CPU и памяти; либо VPA в режиме `Off` как источник цифр для ручного rightsizing (подбора размеров). Готовая панель рекомендаций по всем namespace — Goldilocks (ставит VPA в режиме `Off` на каждый Deployment и показывает цифры в веб-интерфейсе).

### KEDA: масштабирование по событиям и до нуля
**KEDA (Kubernetes Event-Driven Autoscaling)** — оператор, который масштабирует по внешним источникам: длина очереди SQS, отставание консьюмера Kafka (consumer lag — сколько сообщений ещё не прочитано), запросы Prometheus, расписание cron. Сам он не масштабирует поды: создаёт и обновляет обычный HPA, а дополнительно умеет держать **0 реплик**, когда работы нет (HPA сам опускается только до 1).

Когда KEDA, а не HPA по CPU: воркеры очередей. CPU у воркера может быть низким, а в очереди — 100 тысяч сообщений, и HPA по CPU ничего не сделает.
```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: orders-worker
  namespace: prod
spec:
  scaleTargetRef:
    name: orders-worker               # Deployment
  minReplicaCount: 0                  # нет сообщений — нет подов
  maxReplicaCount: 50
  pollingInterval: 15                 # как часто спрашивать длину очереди, секунд
  cooldownPeriod: 120                 # сколько ждать после опустошения очереди перед переходом в 0
  triggers:
    - type: aws-sqs-queue
      authenticationRef:
        name: keda-aws
      metadata:
        queueURL: https://sqs.eu-central-1.amazonaws.com/111122223333/orders
        queueLength: "20"             # цель: 20 сообщений на одну реплику
        awsRegion: eu-central-1
---
apiVersion: keda.sh/v1alpha1
kind: TriggerAuthentication
metadata:
  name: keda-aws
  namespace: prod
spec:
  podIdentity:
    provider: aws                     # права через IRSA или EKS Pod Identity самого KEDA, без ключей в секретах
```
В очереди 600 сообщений → KEDA выставит 600 / 20 = 30 реплик. Очередь пуста 2 минуты → 0 реплик.

Предсказуемые пики (рабочий день, рассылка в 10:00) удобно закрывать триггером `cron` — заранее поднять минимум реплик:
```yaml
  triggers:
    - type: cron
      metadata:
        timezone: Europe/Kyiv
        start: "0 8 * * 1-5"          # будни с 8:00
        end: "0 20 * * 1-5"           # до 20:00
        desiredReplicas: "10"
```

### Всплеск нагрузки в 10 раз: что тормозит масштабирование и как ускорить
Частый вопрос на собеседовании: «трафик вырос в 10 раз за минуту — что произойдёт и как подготовиться?». Полезно разложить, из чего складывается время реакции:

| Шаг | Обычно | Что ускоряет |
|---|---|---|
| metrics-server собрал метрики | до 15 с | метрика-предвестник: RPS или длина очереди вместо CPU |
| HPA пересчитал реплики | до 15 с (цикл контроллера) | `behavior.scaleUp` без задержки, большой `Percent` |
| планировщик нашёл место | секунды, если есть свободные ноды | резерв ёмкости (см. ниже) |
| Karpenter или Cluster Autoscaler добавил ноду | 40–90 с (запуск EC2 + регистрация в кластере) | overprovisioning, тёплые пулы, меньшие и разнообразные типы инстансов |
| скачивание образа | от секунд до минут | маленький образ, кэш образов на нодах, тот же регион у ECR |
| старт приложения и readiness | от секунд до минут (JVM, прогрев кэшей) | `startupProbe`, быстрый старт, ленивые инициализации |

Итого «холодное» добавление пода на новой ноде — 1–3 минуты. Если пик приходит быстрее, без запаса ёмкости он ударит по пользователям.

**Overprovisioning — заранее занятое место.** В кластере держат поды-«заглушки» с низким приоритетом. Они занимают ноды, но ничего не делают. Когда приходит настоящий под, планировщик **вытесняет** (preemption) заглушку и сразу ставит его на освободившееся место, а вытесненная заглушка уходит в Pending и заставляет Karpenter заказать новую ноду уже в фоне:
```yaml
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: overprovisioning
value: -10                            # ниже, чем у любых рабочих подов (по умолчанию 0)
preemptionPolicy: Never               # заглушки сами никого не вытесняют
globalDefault: false
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: capacity-reserve
  namespace: kube-system
spec:
  replicas: 3                         # размер резерва: 3 × (1 CPU, 2 ГиБ)
  selector:
    matchLabels: { app: capacity-reserve }
  template:
    metadata:
      labels: { app: capacity-reserve }
    spec:
      priorityClassName: overprovisioning
      terminationGracePeriodSeconds: 0
      containers:
        - name: pause
          image: registry.k8s.io/pause:3.10   # контейнер, который просто спит
          resources:
            requests: { cpu: "1", memory: 2Gi }
```

Что ещё проверить, кроме самих подов:
- **зависимости выдерживают рост**: 10× подов = 10× соединений к БД (нужен пул соединений: PgBouncer, RDS Proxy), лимиты внешних API, квоты AWS на число инстансов (service quotas) и свободные IP в подсетях (VPC CNI берёт IP на каждый под);
- **ограничения самого HPA**: `maxReplicas` не должен быть потолком, до которого упрётесь в первый же пик;
- **graceful degradation** (деградация вместо падения): rate limiting на ingress, отключение тяжёлых функций по флагу, отдача кэша — пусть часть запросов получит 429, чем упадёт всё;
- **нагрузочный тест** заранее, с замером времени до нужного числа реплик (см. «Как проверить автомасштабирование нагрузкой»).

### HPA, KEDA, VPA, Karpenter: кто за что отвечает и что когда брать
Главное, что нужно понимать: эти инструменты работают на **разных уровнях** и не конкурируют, а дополняют друг друга. Путаница возникает из-за того, что все они называются «автоскейлерами».

| Инструмент | Что масштабирует | По какому сигналу | Умеет до нуля | Когда брать |
|---|---|---|---|---|
| **HPA** (Horizontal Pod Autoscaler) | число реплик Deployment или StatefulSet | CPU и память относительно requests; кастомные и внешние метрики — через адаптер (Prometheus Adapter или KEDA) | нет, минимум 1 реплика | stateless веб-сервисы и API, где рост нагрузки виден по CPU |
| **KEDA** (Kubernetes Event-Driven Autoscaling) | число реплик — через HPA, который она сама создаёт | события и внешние метрики: длина очереди SQS или RabbitMQ, consumer lag Kafka (сколько сообщений ещё не прочитано), запрос в Prometheus, расписание cron — больше 70 источников (scalers) | да | воркеры очередей, пакетная обработка, предсказуемые пики по расписанию, сервисы, которые большую часть времени простаивают |
| **VPA** (Vertical Pod Autoscaler) | requests и limits контейнеров пода | фактическое потребление за последние дни | — | подобрать правильный размер подов; обычно только в режиме рекомендаций (`updateMode: "Off"`) |
| **Cluster Autoscaler** | число нод в заранее созданных группах (ASG — Auto Scaling Group в AWS, Managed Instance Group в GCP) | поды в Pending; недогруженные ноды | — | GKE, AKS, on-prem через Cluster API — там, где нет Karpenter |
| **Karpenter** | ноды: сколько и **какого типа** создать | поды в Pending (их requests, affinity, зоны, tolerations); недогруженные ноды — consolidation (уплотнение: переселить поды и удалить лишние ноды) | — | EKS (и AKS через провайдер Azure): разнородные нагрузки, Spot-инстансы, экономия |

Как они работают вместе — типичная цепочка в EKS:
```
очередь выросла → KEDA опрашивает SQS → её HPA: реплик 2 → 30 → 28 подов в Pending (нет места)
   → Karpenter видит Pending-поды, подбирает и запускает 3 Spot-инстанса → поды стартуют
очередь опустела → KEDA: 0 реплик → ноды пустеют → Karpenter consolidation удаляет ноды
```
KEDA и HPA отвечают на вопрос «**сколько подов** нужно», Karpenter — «**на каких нодах** их запустить». Один без другого не работает до конца: HPA без автоскейлера нод упрётся в Pending, как только кончится свободное место в кластере; Karpenter без HPA будет держать ноды под фиксированное число реплик. И оба считают от **requests**: HPA — процент загрузки, Karpenter — размер ноды. Без правильных requests ошибаются оба.

**KEDA или HPA:**
- Обычного HPA по CPU достаточно, если нагрузка проявляется в CPU пода — например, веб-API за балансировщиком.
- KEDA — если сигнал находится **вне пода** (очередь, lag, метрика в Prometheus, время суток), нужен **scale-to-zero** (масштабирование до нуля: нет работы — нет подов и нет расходов на них) или не хочется отдельно ставить Prometheus Adapter: KEDA сама работает как сервер внешних метрик для HPA.
- KEDA **не заменяет** HPA, а создаёт его сама (`keda-hpa-<имя ScaledObject>`). Поэтому на один Deployment нельзя одновременно повесить свой HPA и ScaledObject: два HPA будут перетягивать число реплик.
- В одном ScaledObject можно сочетать несколько триггеров (например, CPU и длину очереди): HPA берёт максимум из их рекомендаций. Триггеры CPU и памяти у KEDA тоже есть, но до нуля по ним опуститься нельзя: при 0 подов мерить нечего.
- **ScaledJob** — вариант KEDA для долгих задач: вместо масштабирования Deployment создаёт отдельный Job на порцию сообщений. Так при уменьшении реплик не убивается воркер посреди обработки длинного видео или отчёта.

**Karpenter или Cluster Autoscaler:**
- **EKS** — Karpenter, сейчас это стандарт. **EKS Auto Mode** — по сути Karpenter, которым управляет AWS.
- **GKE** — встроенный cluster autoscaler плюс node auto-provisioning (сам создаёт node pools подходящего типа — та же идея, что у Karpenter); в режиме Autopilot нодами вообще управляет Google.
- **On-prem и bare metal** — новую ноду «на лету» не купишь: Cluster Autoscaler работает только поверх Cluster API (Kubernetes-проект, который создаёт и удаляет машины, например виртуальные машины в vSphere или OpenStack), иначе ёмкость планируют заранее.

**VPA и HPA вместе:** не совмещать **по одной метрике**. VPA поднимет requests по CPU → процент загрузки относительно requests упадёт → HPA уберёт реплики → нагрузка на оставшиеся вырастет → цикл. Безопасно: HPA по CPU или RPS, а VPA только для памяти или только в режиме рекомендаций. Раньше VPA применял новые requests только пересозданием пода; в новых версиях Kubernetes появилось изменение ресурсов работающего пода без перезапуска (in-place resize), и VPA умеет его использовать (режим `InPlaceOrRecreate`).

Типичная связка в продакшене на EKS: **HPA** — для API, **KEDA** — для воркеров очередей и задач по расписанию, **VPA в режиме рекомендаций** — чтобы подобрать requests, **Karpenter** — для нод, с запасом в виде overprovisioning (резервные поды с низким приоритетом, которые держат свободное место и вытесняются при всплеске), если важна скорость реакции.

### 🧪 Мини-задание

**Задача.** Создай Deployment с requests по CPU и HPA на 50% CPU от 1 до 5 реплик. Посмотри на HPA в кластере без metrics-server и объясни, что значит `<unknown>`. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create deployment hpa-demo --image=nginx:1.27 > /dev/null
kubectl set resources deployment hpa-demo --requests=cpu=100m > /dev/null
kubectl autoscale deployment hpa-demo --cpu-percent=50 --min=1 --max=5 > /dev/null
sleep 30
kubectl get hpa hpa-demo --no-headers | awk '{print $1, $3, $4, $5, $6}'
kubectl get hpa hpa-demo -o jsonpath='{.status.conditions[?(@.type=="ScalingActive")].reason}{"\n"}'
kubectl delete hpa,deployment hpa-demo > /dev/null
```

Ожидаемый результат:

```text
hpa-demo cpu: <unknown>/50% 1 5 1
FailedGetResourceMetric
```

Разбор: HPA сам не измеряет нагрузку, он берёт метрики из Metrics API, а его предоставляет metrics-server, которого в kind по умолчанию нет. Поэтому текущее значение `<unknown>`, и HPA не масштабирует. Это частый вопрос на собеседовании. Вторая частая причина `<unknown>` — нет `requests.cpu`: процент считается от requests, и без них HPA не может его посчитать. В EKS metrics-server ставят аддоном. Для масштабирования по очередям или своим метрикам используют KEDA или Prometheus Adapter, а для добавления нод — Cluster Autoscaler или Karpenter.

</details>

## Karpenter — автомасштабирование нод в EKS

### Чем Karpenter отличается от Cluster Autoscaler
**Cluster Autoscaler** работает через **группы нод** (в AWS — Auto Scaling Groups). В одной группе все ноды обязаны быть одинакового размера, поэтому:
- чтобы использовать разные типы инстансов (обычные, с большой памятью, GPU, spot), нужно заранее создать и поддерживать много групп;
- переключение со spot на on-demand, когда spot-инстансов нет, медленное и сложное;
- автоскейлер может только добавить ещё одну ноду **заранее заданного** размера.

**Karpenter** работает **без групп** (group-less): смотрит на конкретные pending-поды (их requests, nodeSelector, affinity, tolerations, зоны) и **сам выбирает тип инстанса**, который их вместит дешевле всего, и создаёт его напрямую через EC2 API. Если нужно 5 GB памяти, он возьмёт инстанс на 8 GB, а не добавит ещё одну стандартную ноду на 32 GB.

### NodePool и EC2NodeClass
Конфигурация состоит из двух объектов:
- **NodePool** — **какие** ноды можно создавать: ограничения (requirements) на типы инстансов, spot или on-demand, зоны, архитектуру; общий лимит ресурсов; правила консолидации.
- **EC2NodeClass** — **как** создавать в AWS: AMI (образ диска, из которого запускается нода), IAM-роль ноды (права, с которыми нода ходит в AWS: скачивать образы из ECR, подключать диски), подсети и Security Groups (находятся по тегам), теги для инстансов.

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: default
spec:
  template:
    metadata:
      labels:
        intent: apps                         # метка на создаваемых нодах — поды выбирают их через nodeSelector
    spec:
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["spot", "on-demand"]      # сначала пытаться взять spot, при нехватке — on-demand
        - key: karpenter.k8s.aws/instance-category
          operator: NotIn
          values: ["t"]                      # исключить burstable t-инстансы
        - key: karpenter.k8s.aws/instance-size
          operator: NotIn
          values: ["nano", "micro", "small", "medium"]
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64", "arm64"]         # разрешить Graviton (ARM-процессоры AWS) — он дешевле на 20–40%
      nodeClassRef:
        group: karpenter.k8s.aws
        kind: EC2NodeClass
        name: default
      expireAfter: 720h                      # пересоздавать ноду раз в 30 дней — свежие AMI и патчи
  limits:
    cpu: 1000                                # суммарно не больше 1000 vCPU и 1000 GiB во всех нодах пула
    memory: 1000Gi
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 1m
    budgets:
      - nodes: "10%"                         # одновременно менять не больше 10% нод
      - schedule: "0 9 * * mon-fri"          # в рабочее время по будням — не трогать ноды вовсе
        duration: 8h
        nodes: "0"
---
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: default
spec:
  role: KarpenterNodeRole-my-cluster          # IAM-роль, которую получат ноды
  amiSelectorTerms:
    - alias: al2023@latest                    # в проде лучше фиксировать версию, а не latest
  subnetSelectorTerms:
    - tags:
        karpenter.sh/discovery: my-cluster    # в каких подсетях создавать ноды
  securityGroupSelectorTerms:
    - tags:
        karpenter.sh/discovery: my-cluster
  tags:
    team: platform                            # теги на EC2 — для учёта затрат
```
Почему лучше исключать типы, а не перечислять разрешённые: чем больше типов инстансов доступно Karpenter, тем выше шанс найти свободные spot-мощности и тем ниже цена. Для spot Karpenter использует стратегию `price-capacity-optimized`: выбирает пулы с наименьшим риском прерывания среди дешёвых.

### Consolidation — уплотнение нод
Масштабирование вверх — только половина экономии. Когда нагрузка спадает, поды остаются размазанными по многим полупустым нодам. **Consolidation** — Karpenter постоянно проверяет, можно ли:
- **удалить пустую ноду** (`reason: empty`);
- **переселить поды** с недогруженной ноды на свободное место других и удалить её (`reason: underutilized`);
- **заменить ноду более дешёвой** (например, две m5.xlarge на одну m5.2xlarge, или on-demand на spot).

Переселение идёт штатно: cordon (пометить ноду «не ставить сюда новые поды»), drain (по одному выселить с неё поды, чтобы они пересоздались на других нодах) с учётом PDB (PodDisruptionBudget — «сколько реплик приложения можно одновременно потерять», см. раздел о надёжности), удаление ноды.

Пример из workshop: 12 подов по 1.5 GiB памяти заняли 3 ноды m5.large (по ~6.9 GiB allocatable каждая — столько памяти остаётся подам после резерва на систему и kubelet). После уменьшения до 6 подов (около 9 GiB) Karpenter через `consolidateAfter` удалил одну ноду как underutilized, и поды переехали на две оставшиеся.

Настройки:
- `consolidationPolicy: WhenEmpty` — только пустые ноды; `WhenEmptyOrUnderutilized` — и недогруженные;
- `consolidateAfter` — сколько ждать после последнего изменения подов на ноде (защита от лишней «тряски»);
- `budgets` — сколько нод можно менять одновременно и когда (по расписанию); по умолчанию 10%;
- `expireAfter` — максимальное время жизни ноды: регулярная замена подтягивает свежие AMI.

### Как запретить Karpenter трогать конкретный под или ноду
- аннотация `karpenter.sh/do-not-disrupt: "true"` **на поде** — нода с таким подом не будет добровольно освобождаться (долгие batch-задачи — пакетные расчёты на часы, которые нельзя прерывать, stateful-поды без репликации);
- та же аннотация **на ноде** — эта нода не будет консолидирована;
- **PDB** — Karpenter соблюдает их при drain;
- budgets в NodePool.

### Отладка Karpenter
```bash
# какие ноды создал Karpenter и почему такого типа
kubectl -n karpenter logs deploy/karpenter -c controller --all-pods | grep 'launched nodeclaim' | tail -1 | cut -d] -f2- | jq
# какие ноды он удалил и по какой причине (empty / underutilized / drifted / expired)
kubectl -n karpenter logs deploy/karpenter -c controller --all-pods | grep 'disrupting nodeclaim' | tail -1 | cut -d] -f2- | jq

kubectl get nodeclaims                                  # NodeClaim — объект Karpenter «заявка на одну ноду»: какой инстанс заказан и в каком он состоянии
kubectl get nodes -L karpenter.sh/capacity-type,node.kubernetes.io/instance-type,topology.kubernetes.io/zone
kubectl get node <node> -o json | jq '.metadata.labels' # Karpenter ставит много полезных меток: семейство, CPU, память, гипервизор

# eks-node-viewer — наглядно: загрузка нод по requests и стоимость в час
eks-node-viewer --resources cpu,memory --extra-labels topology.kubernetes.io/zone
eks-node-viewer --node-selector karpenter.sh/nodepool --node-sort=eks-node-viewer/node-cpu-usage=asc
```
Если ноды не удаляются после снятия нагрузки, причины обычно такие: на ноде под с `do-not-disrupt`, PDB не позволяет выселение, budget равен 0 в текущем окне, на ноде есть поды без контроллера (их некуда пересоздать) или DaemonSet-поды держат ноду «непустой» (их Karpenter не учитывает, это нормально).

**Rightsizing подов** — вторая половина экономии: если requests завышены, Karpenter честно создаёт ноды под «воздух». Для подбора requests используют **Goldilocks** (open source от Fairwinds): запускает VPA в режиме рекомендаций для каждого Deployment и показывает в дашборде рекомендованные requests и limits.

### 🧪 Мини-задание

**Задача.** В EKS-кластере с Karpenter посмотри, какие NodePool настроены и какие ноды Karpenter уже создал: тип инстанса, capacity type (spot или on-demand) и зону.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl get nodepools -o custom-columns=NAME:.metadata.name,NODECLASS:.spec.template.spec.nodeClassRef.name,LIMIT_CPU:.spec.limits.cpu
kubectl get nodeclaims -o custom-columns=NAME:.metadata.name,TYPE:.metadata.labels.node\\.kubernetes\\.io/instance-type,CAPACITY:.metadata.labels.karpenter\\.sh/capacity-type,ZONE:.metadata.labels.topology\\.kubernetes\\.io/zone
```

Ожидаемый результат:

```text
NAME      NODECLASS   LIMIT_CPU
default   default     1000
NAME            TYPE         CAPACITY   ZONE
default-7xk2c   m7g.large    spot       us-east-2b
default-q9w4h   c7g.xlarge   on-demand  us-east-2a
```

Разбор: NodePool описывает, *какие* ноды Karpenter вправе создавать (архитектуры, семейства, spot или on-demand, лимит суммарных CPU), а EC2NodeClass — *как* (AMI, подсети, security groups, роль). Под каждый набор Pending-подов Karpenter подбирает подходящий по цене и размеру инстанс и создаёт NodeClaim → ноду, без заранее заданных групп нод. Consolidation потом удаляет или заменяет недогруженные ноды. Лимит CPU защищает от бесконечного роста счёта при ошибке в манифестах. Нужен EKS с установленным Karpenter, в kind этого нет.

</details>

## Остановка пода и PodDisruptionBudget

### Что происходит при удалении пода
1. Под помечается Terminating. **Параллельно** запускаются два процесса:
   - контроллер убирает под из EndpointSlice, kube-proxy и балансировщики обновляют правила (это занимает секунды);
   - kubelet начинает останавливать контейнеры.
2. kubelet выполняет **preStop hook**, если он задан (команда или HTTP-запрос, который kubelet выполняет в контейнере перед остановкой: например, пауза или «закрой соединения»).
3. Контейнерам отправляется **SIGTERM**.
4. Ожидание до `terminationGracePeriodSeconds` (по умолчанию 30 секунд).
5. Кто не завершился — получает **SIGKILL**.

Из-за параллельности шагов 1 приложение может получить SIGTERM и закрыться **раньше**, чем балансировщик перестанет слать на него трафик. Результат — ошибки 502 при каждом деплое. Решение — небольшая задержка в preStop, чтобы под успел выпасть из балансировки:
```yaml
lifecycle:
  preStop:
    exec:
      command: ["sleep", "10"]
terminationGracePeriodSeconds: 45
```

### PodDisruptionBudget
**PDB** ограничивает, сколько подов приложения может быть недоступно одновременно при **добровольных** прерываниях: `kubectl drain`, обновление нод, консолидация Karpenter. Drain будет ждать, а не выселит все реплики сразу.
```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: { name: api }
spec:
  minAvailable: 2            # или maxUnavailable: 1
  selector:
    matchLabels: { app: api }
```
Ловушка: PDB с `minAvailable`, равным числу реплик (или `maxUnavailable: 0`), **блокирует drain навсегда**, и обновление нод зависает.

### 🧪 Мини-задание

**Задача.** Создай Deployment на 2 реплики и PDB с `minAvailable: 2`, затем попробуй осушить (`drain`) ноду с одной из реплик. Что помешает обслуживанию ноды? Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create deployment pdb-demo --image=nginx:1.27 --replicas=2 > /dev/null
kubectl create poddisruptionbudget pdb-demo --selector=app=pdb-demo --min-available=2 > /dev/null
kubectl rollout status deployment/pdb-demo > /dev/null
node=$(kubectl get pods -l app=pdb-demo -o jsonpath='{.items[0].spec.nodeName}')
kubectl drain $node --ignore-daemonsets --timeout=15s 2>&1 | grep -m1 -o 'Cannot evict pod as it would violate the pod.s disruption budget.'
kubectl uncordon $node > /dev/null
kubectl delete pdb,deployment pdb-demo > /dev/null
```

Ожидаемый результат:

```text
Cannot evict pod as it would violate the pod's disruption budget.
```

Разбор: `drain` выселяет поды через Eviction API (специальный запрос «выселить под», который, в отличие от простого удаления, проверяет PDB), а оно соблюдает PDB. Бюджет требует, чтобы всегда были доступны 2 пода из 2, поэтому выселить нельзя ни один, и drain повторяет попытки до таймаута. Обычно ставят `maxUnavailable: 1` или `minAvailable` меньше числа реплик, иначе узлы невозможно обновлять. PDB защищает только от добровольных прерываний (drain, обновление нод, Karpenter consolidation), но не от падения ноды. Сам под при выселении получает `SIGTERM` и `terminationGracePeriodSeconds` на корректное завершение.

</details>

## Как сделать деплой без простоя (zero-downtime)
Частый вопрос. Нужно собрать все части:
1. `RollingUpdate` с `maxUnavailable: 0` — ёмкость не снижается;
2. правильная **readinessProbe** — трафик идёт только на готовые поды;
3. **preStop с задержкой** — под успевает выпасть из балансировки;
4. **приложение корректно обрабатывает SIGTERM**: перестаёт принимать новые запросы, дорабатывает текущие;
5. `terminationGracePeriodSeconds` больше, чем время доработки запросов;
6. **PDB** и несколько реплик в разных зонах — на случай обновления нод;
7. **обратно совместимые миграции БД** — старая и новая версии работают одновременно с одной схемой. Пример: переименовать колонку нельзя за один релиз — сначала добавляют новую колонку и пишут в обе, потом переключают чтение, и только в следующем релизе удаляют старую (паттерн expand/contract).

### 🧪 Мини-задание

**Задача.** Обнови Deployment на несуществующий тег образа со стратегией `maxUnavailable: 0`. Посмотри, сколько подов продолжает работать, пока новая версия не может стартовать, и откати выкатку. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create deployment zdt --image=nginx:1.27 --replicas=3 > /dev/null
kubectl patch deployment zdt -p '{"spec": {"strategy": {"rollingUpdate": {"maxUnavailable": 0, "maxSurge": 1}}}}' > /dev/null
kubectl rollout status deployment/zdt > /dev/null
kubectl set image deployment/zdt nginx=nginx:does-not-exist > /dev/null
kubectl rollout status deployment/zdt --timeout=20s 2>&1 | tail -1
kubectl get deployment zdt -o jsonpath='available={.status.availableReplicas} updated={.status.updatedReplicas}{"\n"}'
kubectl rollout undo deployment/zdt > /dev/null && kubectl rollout status deployment/zdt | tail -1
kubectl delete deployment zdt > /dev/null
```

Ожидаемый результат:

```text
error: timed out waiting for the condition
available=3 updated=1
deployment "zdt" successfully rolled out
```

Разбор: с `maxUnavailable: 0` Kubernetes не убирает старые поды, пока новые не стали Ready. Новый под застрял в `ImagePullBackOff`, поэтому все 3 старых продолжают обслуживать трафик, и пользователи сбоя не заметили. `maxSurge: 1` разрешает один лишний под на время выкатки. `rollout undo` вернул предыдущий ReplicaSet. Для настоящего zero-downtime нужны ещё readinessProbe (иначе «Ready» = «процесс запущен»), корректная обработка `SIGTERM` и `preStop`-пауза, чтобы под успел выпасть из балансировки до остановки.

</details>

## Сетевая модель и NetworkPolicy

Требования Kubernetes к сети:
- у каждого пода свой IP;
- **любой под может обратиться к любому другому поду** по его IP без NAT, на какой бы ноде тот ни был;
- реализует это CNI-плагин.

### Как CNI-плагин настраивает сеть пода
1. kubelet получает задачу запустить под и через CRI просит containerd создать **«песочницу» пода** (служебный контейнер pause, который держит общие для всех контейнеров пода namespaces) с новым сетевым namespace (изолированная копия сетевого стека: свои интерфейсы, IP, маршруты).
2. Рантайм вызывает **бинарник CNI-плагина** (лежит в `/opt/cni/bin/`, конфигурация — в `/etc/cni/net.d/`) с командой `ADD`.
3. Плагин:
   - создаёт **veth-пару** (виртуальный «кабель» из двух интерфейсов: что вошло в один конец, выходит из другого): один конец становится `eth0` внутри пода, другой остаётся на ноде;
   - выделяет IP из диапазона подов (IPAM, IP Address Management — учёт, какие адреса уже выданы);
   - прописывает маршруты и, если нужно, правила файрвола;
   - возвращает результат (IP пода) рантайму.
4. При удалении пода — команда `DEL`, всё убирается.

Сам CNI — это просто спецификация «как вызвать плагин и что он должен вернуть». Всё остальное (как пакеты попадут на другую ноду) решает конкретный плагин.

### Сравнение CNI-плагинов
| Плагин | Как передаёт трафик между нодами | NetworkPolicy | Особенности |
|---|---|---|---|
| **Flannel** | Overlay (оверлей — сеть подов «поверх» сети нод: пакет пода целиком заворачивается в другой пакет между IP нод): пакеты подов заворачиваются в VXLAN-туннель между нодами | **Нет** | Самый простой, мало ресурсов. Для учебных и маленьких кластеров |
| **Calico** | Без overlay — обычная маршрутизация L3, ноды обмениваются маршрутами подов по **BGP**; или туннели IP-in-IP (IP-пакет внутри другого IP-пакета) или VXLAN, если сеть не позволяет | Да, плюс свои расширенные политики (глобальные, с порядком, deny) | Самый распространённый, высокая производительность, шифрование через WireGuard (быстрый современный VPN-протокол в ядре Linux) |
| **Cilium** | Маршрутизация или туннели, логика реализована на **eBPF** — программах внутри ядра Linux | Да, включая **L7-политики** (по HTTP-пути, методу, DNS-имени) | Может заменить kube-proxy (сервисы без iptables), прозрачное шифрование, наблюдаемость сети через **Hubble**, функции service mesh без sidecar. Требует свежее ядро |
| **AWS VPC CNI** | Без overlay: каждый под получает **настоящий IP из подсети VPC** | Да, с включённым network policy agent (eBPF) | Поды напрямую видны в VPC, но быстро тратятся IP-адреса (см. тему AWS) |

**Почему eBPF быстрее iptables.** В режиме iptables каждый Service — это набор правил, которые проверяются последовательно. При тысячах сервисов правил десятки тысяч, их обновление и проверка тормозят. eBPF использует хеш-таблицы прямо в ядре: время поиска не растёт с числом сервисов.

Hubble показывает реальный сетевой трафик между подами — удобно при отладке NetworkPolicy:
```bash
hubble observe --from-pod prod/frontend --to-namespace prod --verdict DROPPED   # какие соединения отброшены политиками
```

Следствие: **по умолчанию все поды видят друг друга**, включая поды из других namespace. Для ограничения используется **NetworkPolicy** — файрвол на уровне подов. Работает, только если CNI её поддерживает (Calico, Cilium; в EKS — VPC CNI с включённым network policy agent). Flannel, например, не поддерживает: политики создадутся, но ничего не будут блокировать.

Типичный подход — сначала запретить всё входящее в namespace, потом точечно разрешить:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-ingress
  namespace: prod
spec:
  podSelector: {}            # пустой селектор — все поды namespace
  policyTypes: [Ingress]     # для них запрещён весь входящий трафик, кроме явно разрешённого
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-api-to-db
  namespace: prod
spec:
  podSelector:
    matchLabels: { app: postgres }      # политика для подов БД
  ingress:
    - from:
        - podSelector:
            matchLabels: { app: api }   # разрешить входящий только от подов api
      ports:
        - port: 5432
```
Если запрещаешь и исходящий трафик (Egress), не забудь разрешить DNS (порт 53 к CoreDNS), иначе поды перестанут резолвить имена.

### 🧪 Мини-задание

**Задача.** Подними сервис `api` и закрой к нему входящий трафик политикой default deny, затем разреши доступ только подам с меткой `role=frontend`. Проверь доступ из пода с этой меткой и без неё. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create deployment api --image=nginx:1.27 > /dev/null && kubectl expose deployment api --port=80 > /dev/null
kubectl rollout status deployment/api > /dev/null
kubectl apply -f - > /dev/null <<'EOF'
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: api-allow-frontend
spec:
  podSelector:
    matchLabels: {app: api}
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector:
            matchLabels: {role: frontend}
EOF
for role in frontend other; do
  kubectl run "client-$role" --rm -i --restart=Never --image=busybox:1.36 --labels="role=$role" -- \
    sh -c 'wget -q -T 3 -O /dev/null http://api && echo allowed || echo blocked' 2>/dev/null | head -1 | sed "s/^/$role: /"
done
kubectl delete networkpolicy api-allow-frontend > /dev/null; kubectl delete deployment,service api > /dev/null
```

Ожидаемый результат:

```text
frontend: allowed
other: blocked
```

Разбор: по умолчанию в Kubernetes любой под может обратиться к любому — сеть плоская. Как только на под (`app: api`) действует хотя бы одна политика с `Ingress`, всё, что ею явно не разрешено, запрещается, поэтому одна политика тут и default deny, и исключение для frontend. Политики работают по меткам, а не по IP, потому что IP подов постоянно меняются. Исполняет их CNI: kindnet в новых kind, Calico или Cilium. В EKS VPC CNI поддерживает NetworkPolicy только при включённой опции, иначе политики молча игнорируются — частая ловушка.

</details>

## Как внешний запрос доходит до пода

Частый вопрос «что происходит, когда пользователь открывает сайт в EKS» проверяет, понимаешь ли ты, **через что реально идёт трафик**, а что только **настраивает** его маршрут. Здесь легко запутаться, потому что в процессе участвует много объектов с похожими названиями.

### Кто есть кто
| Что | Что это | Идёт ли через него трафик |
|---|---|---|
| **Ingress** (объект) | Набор правил в YAML: «запросы на `shop.example.com/api` отправлять в сервис `api`» | Нет — это просто описание |
| **Ingress-контроллер** | Программа в кластере, которая читает объекты Ingress и **настраивает балансировщик** по этим правилам | Зависит от контроллера: AWS Load Balancer Controller — **нет**; ingress-nginx — **да**, он сам и есть прокси |
| **ALB / NLB** | Балансировщик AWS, работает **вне кластера**, в VPC | Да |
| **Service** (объект) | Описание группы подов по меткам (selector) и порта | Сам объект — нет. Через ClusterIP или NodePort — да, но только если обращаются именно к ним |
| **EndpointSlice** | Актуальный список IP **готовых** подов сервиса; ведётся автоматически | Нет — это список, по нему ищут адреса |
| **kube-proxy** | Агент на каждой ноде, который пишет правила iptables или IPVS для ClusterIP и NodePort | Да, но **только** для трафика на ClusterIP или NodePort |
| **CNI** (в EKS — VPC CNI) | Выдаёт подам IP и обеспечивает связь между ними | Обеспечивает доставку пакетов. В EKS благодаря ему у пода **настоящий IP из подсети VPC**, и до пода можно достучаться напрямую из VPC |

Главное различие: **control plane** (контроллер, объекты Ingress и Service) решает, как всё должно быть устроено, и записывает настройки. **Data plane** (балансировщик, прокси, правила kube-proxy) реально пропускает пакеты. На собеседовании ценят именно умение это различать.

Общий для всех сценариев шаг: контроллер смотрит на Ingress, по нему находит Service, по Service — EndpointSlice, а из EndpointSlice берёт IP готовых подов. Дальше сценарии различаются тем, **куда** контроллер записывает эти адреса.

### Сценарий 1. AWS Load Balancer Controller, режим IP — типичный для EKS
```
Пользователь
  → DNS: shop.example.com → ALB (запись Alias в Route 53 — особая запись AWS, которая указывает прямо на ресурс AWS и сама следит за его IP)
  → ALB (в публичных подсетях VPC): TLS, правила по домену и пути
  → target group, в которой зарегистрированы IP подов
  → под 10.0.12.34:8080 (на любой ноде, напрямую по VPC-сети)

Сбоку, вне пути трафика:
AWS Load Balancer Controller ── следит за Ingress, Service, EndpointSlice
                             └─ через AWS API создаёт ALB, правила и target groups,
                                регистрирует и снимает IP подов
```
Пошагово:
1. Ты применяешь манифесты Ingress и Service.
2. **AWS Load Balancer Controller** (обычный Deployment в кластере) видит новый Ingress с `ingressClassName: alb` и через API AWS создаёт ALB, listener'ы на 80 и 443 (listener — «слушатель» балансировщика: порт и протокол, на которых он принимает соединения), правила маршрутизации и target group.
3. Контроллер смотрит EndpointSlice сервиса и **регистрирует в target group IP-адреса подов** с портом контейнера. Под пересоздался и получил новый IP — контроллер снимает старый адрес и регистрирует новый.
4. Запрос пользователя приходит на ALB. ALB по правилам (домен, путь) выбирает target group и отправляет запрос **сразу на IP пода**. Это возможно, потому что VPC CNI выдал поду реальный адрес из подсети VPC — для ALB под выглядит как обычный сервер в сети.
5. Контроллер в этом пути **не участвует**. Даже если его под упадёт, трафик продолжит идти: ALB уже настроен. Не будут применяться только новые изменения.
6. kube-proxy, NodePort и ClusterIP тоже не участвуют. Service здесь нужен только чтобы выбрать поды по меткам.

Манифесты:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: shop-api
  namespace: shop
spec:
  type: ClusterIP                 # в режиме IP достаточно ClusterIP — NodePort не нужен
  selector:
    app: shop-api
  ports:
    - port: 80
      targetPort: 8080            # порт контейнера: именно он будет зарегистрирован в target group
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: shop
  namespace: shop
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing        # ALB в публичных подсетях (internal — внутренний)
    alb.ingress.kubernetes.io/target-type: ip                # регистрировать IP подов
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}]'
    alb.ingress.kubernetes.io/ssl-redirect: "443"            # HTTP → HTTPS
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:eu-central-1:123456789012:certificate/abcd-1234
    alb.ingress.kubernetes.io/healthcheck-path: /health      # health check ALB к каждому поду
    alb.ingress.kubernetes.io/group.name: shop               # несколько Ingress могут делить один ALB
spec:
  ingressClassName: alb
  rules:
    - host: shop.example.com
      http:
        paths:
          - path: /api
            pathType: Prefix
            backend:
              service:
                name: shop-api
                port:
                  number: 80
```
Как убедиться, что ALB действительно шлёт трафик на IP подов:
```bash
kubectl get pods -n shop -l app=shop-api -o wide          # IP подов, например 10.0.12.34 и 10.0.45.6
kubectl get endpointslices -n shop -l kubernetes.io/service-name=shop-api   # те же IP в EndpointSlice
kubectl get targetgroupbindings -n shop                   # связь Service ↔ target group, создаёт контроллер
aws elbv2 describe-target-health --target-group-arn <arn из TargetGroupBinding> \
  --query 'TargetHealthDescriptions[].[Target.Id,Target.Port,TargetHealth.State]' --output table
# в Target.Id будут те же IP подов и порт 8080, а не ID EC2-инстансов
```
Подводные камни этого режима:
- **Security Groups:** ALB подключается к подам напрямую, поэтому Security Group нод (или подов, если используются Security Groups for Pods — функция EKS, позволяющая повесить отдельную Security Group на конкретный под, а не на всю ноду) должна разрешать входящий трафик от Security Group ALB на порт контейнера. Контроллер обычно добавляет это правило сам.
- **Ошибки 502 при деплое.** Новый под может стать Ready в Kubernetes **раньше**, чем ALB зарегистрирует его и проверит health check, а старый под в это время уже удаляется. На короткое время в target group нет здоровых целей. Решение — **readiness gate**: под не считается Ready, пока не станет healthy в target group. Включается меткой на namespace:
  ```bash
  kubectl label namespace shop elbv2.k8s.aws/pod-readiness-gate-inject=enabled
  ```
- **Удаление пода.** ALB продолжает слать запросы на под, пока не завершится его снятие из target group (deregistration delay, по умолчанию 300 секунд, часто уменьшают до 30–60). Поэтому у пода нужен `preStop` с паузой, чтобы он не закрылся раньше, чем ALB перестанет на него слать трафик (см. «Остановка пода»).

### Сценарий 2. AWS Load Balancer Controller, режим instance
```
Пользователь → ALB → target group с EC2-инстансами (все ноды) на порту NodePort, например 31234
  → нода A: kube-proxy ловит пакет на NodePort и подменяет адрес на IP одного из подов (DNAT)
  → под может оказаться на ноде B — тогда пакет идёт ещё и на другую ноду
```
Пошагово:
1. Service должен быть типа **NodePort** (или LoadBalancer): тогда Kubernetes открывает один и тот же порт (30000–32767) на **каждой** ноде.
2. Контроллер регистрирует в target group **ноды**, а не поды: «инстанс i-0abc, порт 31234».
3. ALB отправляет запрос на любую ноду на этот порт.
4. На ноде **kube-proxy** заранее записал правила iptables: пакет на NodePort подменяется (DNAT) на IP одного из готовых подов сервиса — случайно выбранного, возможно, на другой ноде.
5. Если под на другой ноде, пакет идёт туда — это **лишний сетевой хоп**: больше задержка и межзонный трафик (он платный, если ноды в разных AZ).

```yaml
apiVersion: v1
kind: Service
metadata:
  name: shop-api
  namespace: shop
spec:
  type: NodePort                       # в режиме instance нужен NodePort
  externalTrafficPolicy: Cluster       # Local — только на поды той же ноды, сохраняет IP клиента
  selector:
    app: shop-api
  ports:
    - port: 80
      targetPort: 8080
      nodePort: 31234                  # можно не указывать — Kubernetes выберет сам
```
В Ingress меняется одна аннотация: `alb.ingress.kubernetes.io/target-type: instance`.

**`externalTrafficPolicy`:**
- `Cluster` (по умолчанию) — пакет может уйти на под на другой ноде. При этом нода подменяет и адрес источника (SNAT), поэтому приложение видит IP ноды, а не клиента;
- `Local` — нода отправляет трафик только на поды **на самой себе**. Нет лишнего хопа, IP клиента сохраняется, но ноды без подов этого сервиса не проходят health check и не получают трафик.

Как увидеть, что делает kube-proxy (на ноде, режим iptables):
```bash
iptables -t nat -L KUBE-NODEPORTS -n | grep 31234             # правило для NodePort
iptables -t nat -L KUBE-SERVICES -n | grep shop-api           # правило для ClusterIP сервиса
iptables -t nat -L KUBE-SVC-XXXXXXXXXXXXXXXX -n               # подставить имя цепочки из вывода выше
# в цепочке KUBE-SVC-... правила "statistic mode random probability 0.5" выбирают один из KUBE-SEP-...
iptables -t nat -L KUBE-SEP-YYYYYYYYYYYYYYYY -n               # DNAT на конкретный IP:порт пода
```
Сегодня режим instance выбирают редко: режим IP проще и быстрее. Instance нужен, если IP подов не маршрутизируются в VPC (например, CNI с overlay-сетью вместо VPC CNI). На Fargate (режим EKS без нод: каждый под AWS запускает в отдельной микро-VM, платишь за под, а не за серверы), наоборот, работает только режим IP: там нет нод, на которых можно открыть NodePort.

### Сценарий 3. ingress-nginx (или другой контроллер-прокси)
```
Пользователь
  → DNS → NLB (создан для Service типа LoadBalancer контроллера)
  → поды ingress-nginx (это обычный nginx, работающий в кластере)
  → nginx по правилам Ingress выбирает upstream и проксирует прямо на IP пода приложения
```
Здесь всё иначе: **контроллер сам и есть прокси**, весь трафик проходит через его поды.
1. ingress-nginx ставится в кластер как Deployment (поды с nginx) плюс Service типа **LoadBalancer**. Для этого Service в AWS создаётся **NLB**, который передаёт TCP-соединения подам ingress-nginx.
2. Контроллер внутри этих подов следит за объектами Ingress и **генерирует конфигурацию nginx**: server-блоки для доменов, location для путей.
3. Адреса бэкендов nginx берёт **из EndpointSlice** — это IP подов приложения. При изменении списка подов он обновляет их на лету (через встроенный балансировщик на Lua — скриптовом языке, встроенном в nginx через OpenResty), без reload nginx.
4. Запрос: NLB → под ingress-nginx → TLS, правила, лимиты → **напрямую IP пода приложения**. ClusterIP сервиса и kube-proxy и здесь не используются (если не включить аннотацию `nginx.ingress.kubernetes.io/service-upstream: "true"` — тогда nginx пойдёт на ClusterIP).

```yaml
# Service контроллера ingress-nginx (обычно задаётся через values Helm-чарта)
apiVersion: v1
kind: Service
metadata:
  name: ingress-nginx-controller
  namespace: ingress-nginx
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: external              # NLB создаст AWS Load Balancer Controller
    service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: ip         # NLB шлёт трафик на IP подов ingress-nginx
    service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing
spec:
  type: LoadBalancer
  selector:
    app.kubernetes.io/name: ingress-nginx
  ports:
    - name: http
      port: 80
      targetPort: 80
    - name: https
      port: 443
      targetPort: 443
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: shop
  namespace: shop
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "20m"          # то, что в nginx было бы client_max_body_size
    nginx.ingress.kubernetes.io/proxy-read-timeout: "120"
spec:
  ingressClassName: nginx
  tls:
    - hosts: [shop.example.com]
      secretName: shop-tls                                       # сертификат в Secret (часто от cert-manager)
  rules:
    - host: shop.example.com
      http:
        paths:
          - path: /api
            pathType: Prefix
            backend:
              service:
                name: shop-api
                port:
                  number: 80
```
Как посмотреть, куда реально проксирует ingress-nginx:
```bash
kubectl -n ingress-nginx exec deploy/ingress-nginx-controller -- cat /etc/nginx/nginx.conf | grep -A3 "server_name shop.example.com"
kubectl -n ingress-nginx exec deploy/ingress-nginx-controller -- /dbg backends list         # список upstream'ов
kubectl -n ingress-nginx exec deploy/ingress-nginx-controller -- /dbg backends get shop-shop-api-80
# в "endpoints" будут IP подов приложения — те же, что в kubectl get endpointslices
kubectl -n ingress-nginx logs deploy/ingress-nginx-controller --tail=20    # access-лог: upstream_addr = IP пода
```
Плюсы и минусы: можно использовать все возможности nginx (rate limit, rewrite, кэш, сложные правила) и не зависеть от облака. Но поды контроллера — дополнительный хоп, их нужно масштабировать, мониторить и обновлять. Плюс, как говорилось в теме про nginx, сообщество прекратило поддержку ingress-nginx, и для новых кластеров смотрят на Gateway API.

### Сценарий 4. Запрос изнутри кластера — вот где работают ClusterIP и kube-proxy
```
под frontend → DNS: shop-api.shop.svc.cluster.local → ClusterIP 172.20.15.7
  → на ноде пода frontend kube-proxy (iptables) подменяет 172.20.15.7:80 на 10.0.12.34:8080 (DNAT)
  → пакет уходит на под shop-api напрямую по сети подов
```
1. Под обращается к сервису по имени. CoreDNS возвращает **ClusterIP** — виртуальный адрес, которого нет ни на одном интерфейсе.
2. Пакет на ClusterIP перехватывают правила iptables (или IPVS), которые kube-proxy заранее записал **на каждой ноде**. Подмена адреса на IP пода происходит **прямо на ноде отправителя** — никакого центрального балансировщика нет.
3. Дальше пакет идёт на под как обычный трафик между подами, его доставляет CNI.

С Cilium в режиме замены kube-proxy то же самое делают программы eBPF в ядре, без правил iptables.

### Сравнение вариантов
| | ALB, режим IP | ALB, режим instance | ingress-nginx за NLB | Внутри кластера |
|---|---|---|---|---|
| Кто настраивает | AWS Load Balancer Controller | AWS Load Balancer Controller | Контроллер ingress-nginx | kube-proxy по Service и EndpointSlice |
| Через что идёт трафик | ALB → под | ALB → нода (NodePort, kube-proxy) → под | NLB → под ingress-nginx → под | под → (DNAT на ноде) → под |
| Сколько хопов в сети | Минимум | Возможен лишний (на другую ноду) | Лишний (через поды контроллера) | Минимум |
| Тип Service | ClusterIP | NodePort | ClusterIP у приложения, LoadBalancer у контроллера | ClusterIP |
| Где TLS | На ALB (сертификат из ACM) | На ALB | В nginx (Secret, cert-manager) или на NLB | Обычно нет, или mTLS в service mesh |
| Когда выбирать | По умолчанию в EKS | Если IP подов не маршрутизируются в VPC | Нужны возможности nginx, независимость от облака | Сервис-сервис внутри кластера |

### 🧪 Мини-задание

**Задача.** Создай Service для двух подов nginx и пройди путь запроса по шагам: DNS-имя Service → его ClusterIP → эндпоинты-поды. Затем обратись к нему с мака через `kubectl port-forward`. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create deployment shop --image=nginx:1.27 --replicas=2 > /dev/null && kubectl expose deployment shop --port=80 > /dev/null
kubectl rollout status deployment/shop > /dev/null
echo "ClusterIP: $(kubectl get svc shop -o jsonpath='{.spec.clusterIP}')"
kubectl run dns --rm -i --restart=Never --image=busybox:1.36 -- nslookup shop.default.svc.cluster.local 2>/dev/null | grep -A1 '^Name:' | tail -1
echo "pods: $(kubectl get endpointslices -l kubernetes.io/service-name=shop -o jsonpath='{.items[0].endpoints[*].addresses[0]}')"
kubectl port-forward svc/shop 8080:80 > /dev/null & pf=$!; sleep 2
curl -s http://localhost:8080 | grep -o '<title>.*</title>'
kill $pf; kubectl delete deployment,service shop > /dev/null
```

Ожидаемый результат:

```text
ClusterIP: 10.96.143.27
Address: 10.96.143.27
pods: 10.244.1.7 10.244.2.5
<title>Welcome to nginx!</title>
```

Разбор: CoreDNS отвечает на имя Service его ClusterIP — виртуальным адресом, за которым нет ни одного интерфейса. kube-proxy (iptables или IPVS) на каждой ноде перехватывает трафик на этот адрес и раскидывает его по IP подов из EndpointSlice. Снаружи кластера до Service доходят через NodePort, LoadBalancer (в EKS — NLB через AWS Load Balancer Controller) или Ingress/Gateway (ALB), который роутит по хосту и пути. `port-forward` — отладочный туннель через API-сервер, для продакшен-трафика его не используют. Суффиксы имён подов и IP у тебя будут свои.

</details>

## Безопасность и доступ: RBAC

**RBAC** (Role-Based Access Control) определяет, кто и что может делать в API.
- **Role** — набор разрешений в одном namespace: какие действия (verbs: get, list, watch, create, update, patch, delete) над какими ресурсами.
- **ClusterRole** — то же на уровне кластера, или для ресурсов вне namespace (ноды, PV).
- **RoleBinding** / **ClusterRoleBinding** — **кому** выдать роль: пользователю, группе или **ServiceAccount** (учётная запись для подов).

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-reader
  namespace: dev
rules:
  - apiGroups: [""]                 # "" — основная группа API (pods, services, configmaps...)
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: ci-reads-pods
  namespace: dev
subjects:
  - kind: ServiceAccount
    name: ci-bot
    namespace: dev
roleRef:
  kind: Role
  name: pod-reader
  apiGroup: rbac.authorization.k8s.io
```
Приём переиспользования: **ClusterRole + RoleBinding** даёт права из кластерной роли, но только в одном namespace. Одну роль «developer» описывают один раз, а привязывают в нужных namespace.

Проверка прав:
```bash
kubectl auth can-i create deployments -n prod                               # могу ли я?
kubectl auth can-i list secrets --as=system:serviceaccount:dev:ci-bot -n dev   # может ли этот ServiceAccount?
```

**В EKS** есть два уровня:
- **люди и CI → Kubernetes**: IAM-роли сопоставляются с пользователями и группами Kubernetes через **access entries** (записи в API EKS «такая IAM-роль = такая группа в кластере»; раньше то же делал ConfigMap `aws-auth`, ошибка в котором отрезала доступ к кластеру всем);
- **поды → AWS API**: поду через его ServiceAccount выдаётся IAM-роль — **IRSA** (IAM Roles for Service Accounts, через OIDC — стандарт, по которому кластер выдаёт поду подписанный токен, а AWS ему доверяет и меняет его на временные ключи роли) или **EKS Pod Identity** (более новый и простой способ того же, без настройки OIDC). Так под получает доступ, например, только к своему S3-бакету, а не все права ноды.

Ещё о безопасности подов: запуск не от root (`securityContext.runAsNonRoot: true`), `readOnlyRootFilesystem` (корневая ФС контейнера только для чтения — взломщик не сможет записать свой бинарник), без привилегий (`allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`). Принудительно — через **Pod Security Admission** (встроенный механизм: на namespace вешается уровень privileged — без ограничений, baseline — запрет явно опасного вроде privileged-контейнеров, restricted — строгие правила, включая запуск не от root) или политики Kyverno.

### Что защищает кластер, кроме RBAC
RBAC отвечает только на вопрос «кто что может сделать в API Kubernetes». Безопасность кластера строится слоями, и на собеседовании ждут, что ты перечислишь больше одного:

| Слой | Инструменты | От чего защищает (пример) |
|---|---|---|
| Сеть между подами | NetworkPolicy (по умолчанию всё запрещено, разрешаем нужное), Cilium/Calico политики по DNS-именам | взломанный фронтенд не может напрямую подключиться к базе |
| Что можно запускать | Pod Security Admission, Kyverno или OPA Gatekeeper (политики на этапе создания объекта) | запрет privileged-контейнеров, hostPath, запуска от root, образов с тегом `latest` |
| Откуда образы | подпись образов (cosign) и проверка подписи политикой, сканирование (Trivy, ECR scanning), только свой registry | подмена образа в registry, образ с известной уязвимостью |
| Секреты | шифрование Secret в etcd ключом KMS (envelope encryption), External Secrets Operator или Secrets Store CSI вместо секретов в git | утечка бэкапа etcd, секрет в репозитории |
| Доступ подов к облаку | IRSA или EKS Pod Identity (у каждого пода своя роль), IMDSv2 с hop limit 1 | под крадёт права роли ноды через сервис метаданных EC2 |
| API-сервер | приватный endpoint (или список разрешённых IP), audit-логи в CloudWatch | перебор и атаки на API из интернета, нет следов, кто что удалил |
| Рантайм | Falco или GuardDuty EKS Runtime Monitoring | запуск shell в контейнере, майнер, чтение `/etc/shadow` |
| Ноды | минимальная ОС (Bottlerocket), автообновление образов нод, без SSH (SSM) | эксплуатация уязвимостей ОС ноды |

**NetworkPolicy «всё запрещено» для namespace** — отправная точка, к которой потом добавляют разрешающие политики:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
  namespace: prod
spec:
  podSelector: {}                     # все поды namespace
  policyTypes: [Ingress, Egress]      # запретить и входящий, и исходящий трафик
```
Не забудь отдельно разрешить исходящий трафик к CoreDNS (UDP и TCP 53 в `kube-system`), иначе у подов сломается разрешение имён. NetworkPolicy работает, только если CNI её поддерживает: в EKS у VPC CNI её нужно включить, Calico и Cilium поддерживают из коробки.

**Pod Security Admission** включается метками на namespace:
```bash
kubectl label namespace prod \
  pod-security.kubernetes.io/enforce=restricted \
  pod-security.kubernetes.io/warn=restricted
# под, который запускается от root или с privileged: true, будет отклонён при создании
```

**Проверка подписи образа** (подпись ставит CI после сборки, проверяет политика Kyverno или вручную):
```bash
cosign verify registry.example.com/shop/api:1.4.2 \
  --certificate-identity "https://github.com/acme/shop/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
# keyless-подпись: образ подписан во время конкретного workflow в GitHub Actions, без хранения ключа
```

**IMDS hop limit.** Сервис метаданных EC2 (`169.254.169.254`) отдаёт временные ключи роли ноды. Если под может до него достучаться, он получит все права ноды — в обход IRSA. Ограничение `http-put-response-hop-limit 1` не даёт ответу IMDSv2 пройти лишний сетевой «прыжок» до контейнера в отдельном сетевом namespace:
```bash
aws ec2 modify-instance-metadata-options --instance-id i-0abc1234def567890 \
  --http-tokens required --http-put-response-hop-limit 1
```
В EKS это задают в launch template или EC2NodeClass Karpenter (`metadataOptions`), а не руками на каждом инстансе.

### 🧪 Мини-задание

**Задача.** Создай ServiceAccount `viewer` и дай ему через Role и RoleBinding право только читать поды в namespace `default`. Проверь его права через `kubectl auth can-i` от его имени. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl create serviceaccount viewer > /dev/null
kubectl create role pod-reader --verb=get,list,watch --resource=pods > /dev/null
kubectl create rolebinding viewer-pod-reader --role=pod-reader --serviceaccount=default:viewer > /dev/null
as=system:serviceaccount:default:viewer
echo "get pods: $(kubectl auth can-i get pods --as=$as)"
echo "delete pods: $(kubectl auth can-i delete pods --as=$as)"
echo "get secrets: $(kubectl auth can-i get secrets --as=$as)"
echo "get pods -n kube-system: $(kubectl auth can-i get pods -n kube-system --as=$as)"
kubectl delete rolebinding viewer-pod-reader > /dev/null; kubectl delete role pod-reader > /dev/null; kubectl delete serviceaccount viewer > /dev/null
```

Ожидаемый результат:

```text
get pods: yes
delete pods: no
get secrets: no
get pods -n kube-system: no
```

Разбор: в RBAC по умолчанию запрещено всё, а Role перечисляет разрешённые глаголы над ресурсами. Role действует в своём namespace, поэтому в `kube-system` прав нет; для всего кластера нужны ClusterRole и ClusterRoleBinding. Секреты не входят в «чтение подов» — права даются по ресурсам точечно. `--as` проверяет права от чужого имени, так и отлаживают «почему у пайплайна Forbidden». В EKS IAM-роль связывают с Kubernetes-группой через access entries, а поду дают AWS-права через IRSA или Pod Identity.

</details>

## CRD и операторы
**CustomResourceDefinition (CRD)** добавляет в API Kubernetes новый тип объекта. Например, cert-manager добавляет `Certificate`, Prometheus Operator — `ServiceMonitor` (описание «какие сервисы опрашивать за метриками»), ArgoCD — `Application` (какой репозиторий в какой кластер разворачивать).

**Оператор** — контроллер, который следит за этими объектами и выполняет цикл согласования для них. В него закладывают знания администратора: например, оператор PostgreSQL по объекту `Cluster` разворачивает реплики, настраивает репликацию, делает бэкапы и переключение при отказе (failover — автоматически делает реплику главной, если основной сервер упал).

### 🧪 Мини-задание

**Задача.** Создай CRD `Backup` с полями `schedule` и `retentionDays` и колонками для `kubectl get`, создай один объект `Backup` и выведи список. Проверь, что API-сервер отклоняет объект с неверным типом поля. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl apply -f - > /dev/null <<'EOF'
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: backups.lab.example.com
spec:
  group: lab.example.com
  scope: Namespaced
  names: {plural: backups, singular: backup, kind: Backup}
  versions:
    - name: v1
      served: true
      storage: true
      additionalPrinterColumns:
        - {name: Schedule, type: string, jsonPath: .spec.schedule}
        - {name: Retention, type: integer, jsonPath: .spec.retentionDays}
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                schedule: {type: string}
                retentionDays: {type: integer}
EOF
sleep 2
kubectl apply -f - <<'EOF'
apiVersion: lab.example.com/v1
kind: Backup
metadata:
  name: orders-db
spec:
  schedule: "0 3 * * *"
  retentionDays: 7
EOF
kubectl get backups
kubectl patch backup orders-db --type=merge -p '{"spec": {"retentionDays": "week"}}' 2>&1 | grep -o 'spec.retentionDays: Invalid value: "string": spec.retentionDays in body must be of type integer: "string"'
kubectl delete crd backups.lab.example.com > /dev/null
```

Ожидаемый результат:

```text
backup.lab.example.com/orders-db created
NAME        SCHEDULE    RETENTION
orders-db   0 3 * * *   7
spec.retentionDays: Invalid value: "string": spec.retentionDays in body must be of type integer: "string"
```

Разбор: CRD добавляет в API новый тип ресурса: после него `kubectl get backups` работает как для встроенных объектов, а OpenAPI-схема заставляет API-сервер проверять поля. Но сам по себе объект `Backup` ничего не делает — это просто запись в etcd. Оператор — это контроллер, который следит за такими объектами и приводит реальность к описанному: создаёт CronJob, делает снапшот, пишет результат в `status`. Так устроены cert-manager, ArgoCD, Karpenter, Prometheus Operator. При удалении CRD удаляются и все объекты этого типа — в проде это делают очень осторожно.

</details>

## Static pods
**Static pod** — под, которым управляет **kubelet конкретной ноды напрямую**, без apiserver и без scheduler. kubelet следит за папкой с манифестами (обычно `/etc/kubernetes/manifests/`, задаётся параметром `staticPodPath`) и сам запускает всё, что там лежит.

Зачем: так запускается **сам control plane** в кластерах kubeadm — kube-apiserver, etcd, scheduler и controller-manager лежат манифестами в этой папке. apiserver не может запустить сам себя, а kubelet может.

Особенности:
- kubelet создаёт в apiserver **зеркальный под** (mirror pod), чтобы его было видно в `kubectl get pods`. Его имя заканчивается **именем ноды**: `kube-apiserver-master-1`;
- `kubectl delete pod` такой под не удалит: kubelet тут же создаст его снова. Удалить — **убрать файл манифеста** из папки на ноде;
- изменить — отредактировать файл, kubelet перезапустит под.

Классический вопрос с подвохом «как удалить под без kubectl?» — именно про static pods.

### 🧪 Мини-задание

**Задача.** Посмотри, какие манифесты лежат в каталоге static pod'ов на control plane ноде kind, затем удали «зеркальный» под `kube-scheduler` через API и проверь, что с ним стало. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
docker exec lab-control-plane ls /etc/kubernetes/manifests
kubectl delete pod -n kube-system kube-scheduler-lab-control-plane > /dev/null
sleep 10
kubectl get pod -n kube-system kube-scheduler-lab-control-plane --no-headers | awk '{print $1, $3}'
```

Ожидаемый результат:

```text
etcd.yaml
kube-apiserver.yaml
kube-controller-manager.yaml
kube-scheduler.yaml
kube-scheduler-lab-control-plane Running
```

Разбор: static pod'ы kubelet запускает сам из файлов в `/etc/kubernetes/manifests`, без API-сервера и планировщика. Именно так запускается сам control plane: API-сервер не может запустить себя через API. В API видна лишь «зеркальная» копия: её удаление через `kubectl` ничего не останавливает, kubelet тут же воссоздаёт зеркало. Чтобы реально остановить или изменить такой под, правят или убирают файл на ноде. kubeadm при обновлении кластера переписывает именно эти манифесты.

</details>

## kubectl create и kubectl apply
- **`kubectl create`** — **императивно**: «создай этот объект». Если объект уже существует — ошибка. Изменения вносят другими командами (`kubectl edit`, `set`, `scale`), и история того, что было задумано, не сохраняется.
- **`kubectl apply`** — **декларативно**: «пусть объект выглядит так». Создаёт, если объекта нет, и **обновляет**, если есть, меняя только отличающиеся поля. Последнюю применённую конфигурацию запоминает в аннотации `kubectl.kubernetes.io/last-applied-configuration`, чтобы при следующем apply понять, какие поля ты **удалил** из манифеста (и удалить их из объекта), а какие изменил кто-то другой.

Для GitOps и хранения манифестов в git подходит только apply (а ещё лучше server-side apply: `kubectl apply --server-side` — учёт «владельцев» полей ведёт apiserver, и конфликты между инструментами видны явно).

### 🧪 Мини-задание

**Задача.** Создай ConfigMap из файла дважды через `kubectl create`, потом дважды через `kubectl apply`, поменяй значение и примени снова. Сравни ответы и найди аннотацию, в которой `apply` хранит последнее применённое состояние. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cd "$(mktemp -d)"
printf 'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: app-cfg\ndata:\n  LOG_LEVEL: info\n' > cm.yaml
kubectl create -f cm.yaml; kubectl create -f cm.yaml 2>&1 | cut -c1-80
kubectl delete -f cm.yaml > /dev/null
kubectl apply -f cm.yaml; kubectl apply -f cm.yaml
sed -i.bak 's/info/debug/' cm.yaml && kubectl apply -f cm.yaml
kubectl get cm app-cfg -o jsonpath='{.metadata.annotations.kubectl\.kubernetes\.io/last-applied-configuration}' | jq -c .data
kubectl delete -f cm.yaml > /dev/null
```

Ожидаемый результат:

```text
configmap/app-cfg created
Error from server (AlreadyExists): error when creating "cm.yaml": configmaps "app-cf
configmap/app-cfg created
configmap/app-cfg unchanged
configmap/app-cfg configured
{"LOG_LEVEL":"debug"}
```

Разбор: `create` — императивная команда «создай», и повторный вызов падает с `AlreadyExists`. `apply` — декларативная: «приведи объект к этому описанию». Он создаёт, если объекта нет, ничего не делает, если совпадает, и обновляет, если отличается, поэтому его можно запускать сколько угодно раз — в CI и GitOps используют именно его. Аннотация `last-applied-configuration` нужна для трёхстороннего слияния: так `apply` понимает, какое поле ты удалил из файла, а какое добавил кто-то другой. При `--server-side` эту работу делает API-сервер через `managedFields`.

</details>

## Несколько кластеров: контексты kubeconfig
kubeconfig (`~/.kube/config`) содержит **кластеры** (адрес и сертификат CA), **пользователей** (как аутентифицироваться) и **контексты** (пара «кластер + пользователь + namespace по умолчанию»).
```bash
kubectl config get-contexts                       # все контексты, текущий отмечен *
kubectl config current-context
kubectl config use-context prod-eks               # переключиться на другой кластер
kubectl config set-context --current --namespace=payments   # namespace по умолчанию для текущего контекста
aws eks update-kubeconfig --region eu-central-1 --name prod-cluster --alias prod-eks   # добавить EKS-кластер
```
Удобные инструменты: `kubectx` и `kubens` (быстрое переключение кластера и namespace), Lens (графическое приложение) или k9s (интерфейс прямо в терминале). Чтобы случайно не выполнить команду в проде, показывай текущий контекст в приглашении shell (kube-ps1, starship — дописывают в промпт имя кластера и namespace).

### 🧪 Мини-задание

**Задача.** Посмотри, какие контексты есть в kubeconfig и какой из них текущий. Создай контекст `lab-dev` для того же кластера с namespace по умолчанию `dev`, переключись на него, проверь namespace и вернись обратно. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl config get-contexts -o name
kubectl config current-context
kubectl config set-context lab-dev --cluster=kind-lab --user=kind-lab --namespace=dev > /dev/null
kubectl config use-context lab-dev
kubectl config view --minify -o jsonpath='namespace: {..namespace}{"\n"}'
kubectl config use-context kind-lab > /dev/null && kubectl config delete-context lab-dev > /dev/null
kubectl config current-context
```

Ожидаемый результат:

```text
kind-lab
kind-lab
Switched to context "lab-dev".
namespace: dev
kind-lab
```

Разбор: контекст — это тройка «кластер + пользователь (креды) + namespace по умолчанию». Переключая контекст, ты меняешь и кластер, и namespace, поэтому перед опасной командой всегда смотрят `current-context` (или выводят его в промпт оболочки). `--minify` показывает только текущий контекст. Несколько kubeconfig-файлов объединяют через переменную `KUBECONFIG=file1:file2`. Для быстрого переключения есть `kubectx` и `kubens`. Привычка «в каждой команде явно `--context`» в скриптах спасает от `delete` не в том кластере.

</details>

## Локальный кластер на Mac
Для экспериментов не нужен облачный кластер:
- **kind** — Kubernetes в Docker-контейнерах, кластер за минуту, удобно для CI и тестов манифестов;
- **k3d** — лёгкий k3s (облегчённый дистрибутив Kubernetes одним бинарником) в Docker; на Mac без Docker Desktop его можно запускать поверх **Lima** или **Colima** (лёгкие Linux-VM);
- **minikube** — классический вариант с аддонами;
- **OrbStack**, **Rancher Desktop** — Docker и Kubernetes одним приложением (замена Docker Desktop).

### 🧪 Мини-задание

**Задача.** Создай через kind учебный кластер `lab` из одной control plane ноды и двух воркеров (его используют остальные задания этой темы) и проверь ноды. Нужны запущенный Docker Desktop и `brew install kind`.

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
cat > /tmp/kind-lab.yaml <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
  - role: worker
EOF
kind create cluster --name lab --config /tmp/kind-lab.yaml > /dev/null 2>&1
kubectl get nodes --no-headers | awk '{print $1, $2, $3}'
docker ps --format '{{.Names}}' | grep '^lab-' | sort
```

Ожидаемый результат:

```text
lab-control-plane Ready control-plane
lab-worker Ready <none>
lab-worker2 Ready <none>
lab-control-plane
lab-worker
lab-worker2
```

Разбор: каждая «нода» kind — Docker-контейнер, внутри которого работают systemd, containerd и kubelet, а поды запускаются уже внутри него. Поэтому кластер поднимается за минуту и удаляется без следов: `kind delete cluster --name lab`. kind сам прописал контекст `kind-lab` в `~/.kube/config` и переключился на него. Ограничения: нет облачных LoadBalancer'ов (только NodePort или port-forward), метрик (`metrics-server` ставят отдельно) и настоящего хранилища. Для тестов манифестов, Helm-чартов и операторов в CI этого достаточно.

</details>

## kubectl — шпаргалка
```bash
# просмотр
kubectl get pods -A -o wide                      # все поды всех namespace, с нодами и IP
kubectl get pod api-7d9f -o yaml                 # полный объект со статусом
kubectl describe pod api-7d9f                    # описание + Events внизу — ПЕРВОЕ, что смотреть при проблемах
kubectl get events -n prod --sort-by=.lastTimestamp
kubectl explain deployment.spec.strategy         # документация по полю прямо в терминале
kubectl api-resources                            # какие типы объектов есть в кластере

# логи и отладка
kubectl logs api-7d9f -c api --previous          # логи ПРЕДЫДУЩЕГО (упавшего) запуска контейнера
kubectl logs -l app=api --tail=100 -f            # по метке, со всех подов
kubectl exec -it api-7d9f -- sh
kubectl debug -it api-7d9f --image=busybox --target=api   # ephemeral container — временный контейнер с утилитами рядом с основным (в образе приложения часто нет ни sh, ни curl)
kubectl debug node/ip-10-0-1-5 -it --image=ubuntu         # shell на ноде (файловая система ноды в /host)
kubectl port-forward svc/api 8080:80             # доступ к сервису с локальной машины
kubectl top pods; kubectl top nodes              # потребление (нужен metrics-server)

# изменения
kubectl apply -f manifest.yaml
kubectl diff -f manifest.yaml                    # что изменится, до apply
kubectl scale deploy/api --replicas=5
kubectl set image deploy/api api=registry/api:1.5
kubectl rollout status|history|undo|restart deploy/api

# ноды
kubectl cordon node1                             # запретить новые поды на ноде
kubectl drain node1 --ignore-daemonsets --delete-emptydir-data   # выселить поды (с учётом PDB)
kubectl uncordon node1

# генерация манифестов
kubectl create deployment web --image=nginx --dry-run=client -o yaml > deploy.yaml
kubectl get pods -o jsonpath='{.items[*].spec.containers[*].image}'   # все образы
kubectl get pods -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName,RESTARTS:.status.containerStatuses[0].restartCount
```

### 🧪 Мини-задание

**Задача.** Выведи все поды кластера таблицей с namespace, нодой, числом рестартов и образом первого контейнера, отсортированной по числу рестартов, а затем найди поды не в фазе `Running`. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl get pods -A --sort-by='.status.containerStatuses[0].restartCount' \
  -o custom-columns=NS:.metadata.namespace,POD:.metadata.name,NODE:.spec.nodeName,RESTARTS:.status.containerStatuses[0].restartCount,IMAGE:.spec.containers[0].image | tail -3
kubectl get pods -A --field-selector=status.phase!=Running -o name
```

Ожидаемый результат:

```text
kube-system   kube-proxy-r4nkt                    lab-control-plane   0          registry.k8s.io/kube-proxy:v1.33.1
kube-system   kube-scheduler-lab-control-plane    lab-control-plane   1          registry.k8s.io/kube-scheduler:v1.33.1
kube-system   kube-controller-manager-lab-control-plane   lab-control-plane   2          registry.k8s.io/kube-controller-manager:v1.33.1
```

Разбор: `custom-columns` и `jsonpath` превращают kubectl в инструмент отчётов без `jq`, а `--sort-by` сортирует по любому полю объекта: в конце окажутся самые «нестабильные» поды. `--field-selector` фильтрует на стороне API-сервера, но поддерживает немногие поля (`status.phase`, `spec.nodeName`, `metadata.name`), а по меткам фильтруют через `-l`. Пустой вывод второй команды — хороший знак: все поды в `Running`. Полезные дополнения: `-w` для наблюдения в реальном времени, `kubectl explain pod.spec.containers` для документации прямо в терминале. Суффиксы имён подов и IP у тебя будут свои.

</details>

## Диагностика: статусы подов и что с ними делать

Алгоритм всегда один: **`kubectl describe pod` (секция Events) → `kubectl logs` (и `--previous`) → события namespace → состояние ноды.**

| Статус | Что значит | Где искать причину |
|---|---|---|
| `Pending` | Под не назначен на ноду или ещё не запущен | Events: `Insufficient cpu/memory` (requests больше свободного; Karpenter или Cluster Autoscaler не добавил ноду), `didn't tolerate taint`, не подходит affinity, `unbound PersistentVolumeClaim` (PVC не создался или диск в другой зоне) |
| `ContainerCreating` долго | Тянется образ, монтируется том, CNI не выдаёт IP | Events: ошибки монтирования, `failed to assign an IP address` (в EKS — закончились IP в подсети) |
| `ImagePullBackOff` / `ErrImagePull` | Не удаётся скачать образ | Опечатка в имени или теге; нет доступа к приватному registry (`imagePullSecrets` — Secret с логином к реестру, IAM-права ноды на ECR — реестр образов AWS); лимиты Docker Hub (анонимно — 100 скачиваний за 6 часов с одного IP, большой кластер за NAT упирается в них быстро) |
| `CrashLoopBackOff` | Контейнер запускается и падает, Kubernetes перезапускает с растущей паузой | `kubectl logs --previous`: ошибка приложения, нет переменной окружения или конфига, неверная команда запуска, не может подключиться к БД; или liveness probe слишком агрессивна |
| `OOMKilled` (exit 137) | Превышен лимит памяти | `describe` → Last State: Reason OOMKilled. Увеличить лимит или искать утечку; для JVM и .NET — проверить настройки памяти рантайма (например, `-XX:MaxRAMPercentage` у Java: рантайм сам решает, сколько памяти взять под heap, и без настройки может не уложиться в лимит контейнера) |
| `CreateContainerConfigError` | Нет ConfigMap или Secret, на который ссылается под | Events: `secret "x" not found` |
| `Running`, но `0/1 Ready` | Не проходит readiness probe | Неверный путь или порт проверки, приложение ещё прогревается, зависимость недоступна |
| `Terminating` бесконечно | Под не удаляется | Finalizers на объекте (метки «прежде чем удалять, контроллер должен закончить уборку» — если контроллер сломан, объект висит вечно), нода недоступна (kubelet не подтверждает удаление). `--force --grace-period=0` — крайняя мера |
| `Evicted` | Выселен kubelet'ом | Нехватка памяти или диска на ноде (`DiskPressure`, `MemoryPressure` — состояния ноды, при которых kubelet начинает освобождать ресурсы), под без requests (BestEffort) выселяется первым |

**Сервис не отвечает:**
```bash
kubectl get endpointslices -l kubernetes.io/service-name=api   # есть ли адреса у сервиса?
```
Если адресов нет — селектор сервиса не совпадает с метками подов, или поды не Ready. Если адреса есть — проверить, что `targetPort` совпадает с портом, который реально слушает приложение, и что приложение слушает `0.0.0.0`, а не `127.0.0.1`. Затем — NetworkPolicy.

### 🧪 Мини-задание

**Задача.** Создай два «сломанных» пода: с несуществующим образом и с приложением, которое падает сразу после старта. Определи статус каждого, причину из событий и логи упавшего контейнера. Нужен учебный кластер kind `lab` — он создаётся в задании раздела «Локальный кластер на Mac».

<details class="qa"><summary>Решение: команда и ожидаемый результат</summary>

Скопируй блок в терминал целиком:

```bash
kubectl run badimage --image=nginx:no-such-tag > /dev/null
kubectl run crasher --image=busybox:1.36 -- sh -c 'echo "connecting to db..."; echo "FATAL: DATABASE_URL not set" >&2; exit 1' > /dev/null
sleep 40
kubectl get pods badimage crasher --no-headers | awk '{print $1, $3, $4}'
kubectl get events --field-selector involvedObject.name=badimage,reason=Failed -o jsonpath='{.items[0].message}' | cut -c1-70; echo
kubectl logs crasher --previous
kubectl delete pod badimage crasher > /dev/null
```

Ожидаемый результат:

```text
badimage ImagePullBackOff 0
crasher CrashLoopBackOff 3
Failed to pull image "nginx:no-such-tag": rpc error: code = NotFound
connecting to db...
FATAL: DATABASE_URL not set
```

Разбор: два разных класса проблем. `ImagePullBackOff` — контейнер даже не стартовал: опечатка в теге, нет доступа к приватному реестру (`imagePullSecrets`, права ECR) или лимит Docker Hub; причина — в событиях `kubectl describe pod`. `CrashLoopBackOff` — контейнер стартует и падает, kubelet перезапускает его с растущей задержкой; причину ищут в логах, и именно `--previous`, потому что текущий экземпляр мог ещё не успеть ничего написать. Порядок разбора любого пода: `get` (статус) → `describe` (события) → `logs` (`--previous`) → `exec` или `kubectl debug`.

</details>

## Вопросы с собеседований

**Чем Deployment отличается от StatefulSet?**
Deployment — для взаимозаменяемых реплик: случайные имена, общий или никакой диск, параллельный запуск. StatefulSet — для приложений с состоянием: стабильные имена (`app-0`) и DNS-имена каждого пода, свой постоянный диск у каждой реплики через `volumeClaimTemplates`, упорядоченный запуск и остановка. Для баз данных, Kafka, ZooKeeper.

**Как работает Service, если у него нет процесса?**
ClusterIP — виртуальный адрес, которого нет ни на одном интерфейсе. EndpointSlice-контроллер ведёт список IP готовых подов, а kube-proxy на каждой ноде создаёт правила iptables или IPVS: трафик на ClusterIP:port подменяется (DNAT) на IP одного из подов. Балансировка происходит на ноде клиента.

**Что происходит при `kubectl apply` Deployment?**
apiserver (аутентификация, RBAC, admission) сохраняет объект в etcd → Deployment-контроллер создаёт ReplicaSet → ReplicaSet-контроллер создаёт поды → scheduler назначает ноды → kubelet через containerd запускает контейнеры, CNI выдаёт IP → после прохождения readiness под попадает в EndpointSlice сервиса. Все компоненты взаимодействуют только через apiserver.

**Что будет, если упадёт control plane?**
Уже работающие поды продолжат работать и обслуживать трафик: kubelet и kube-proxy работают на нодах автономно, правила iptables остаются. Но не будет ничего нового: ни планирования, ни перезапуска упавших подов на других нодах, ни масштабирования, ни деплоев, `kubectl` не работает.

**Что будет при потере кворума etcd?**
Запись в etcd невозможна, apiserver не может изменять состояние: кластер фактически замораживается. Работающие поды продолжают работать. Восстановление — вернуть узлы etcd или восстановить из снапшота.

**Как сделать деплой без простоя?**
RollingUpdate с `maxUnavailable: 0`, корректная readiness probe, preStop с задержкой, обработка SIGTERM в приложении, достаточный `terminationGracePeriodSeconds`, PDB и несколько реплик в разных зонах, обратно совместимые миграции БД.

**Опиши порядок остановки пода.**
Под помечается Terminating; параллельно его удаляют из EndpointSlice и запускают preStop hook; затем SIGTERM; ожидание до `terminationGracePeriodSeconds` (30 с по умолчанию); затем SIGKILL. Из-за параллельности нужен preStop с паузой, иначе в балансировщик ещё идёт трафик на закрывшееся приложение.

**Чем requests отличаются от limits? Что будет без requests?**
Requests — гарантированный минимум, по ним scheduler выбирает ноду. Limits — потолок: превышение памяти ведёт к OOMKilled, превышение CPU — к throttling. Без requests под получает класс BestEffort: scheduler считает его бесплатным, ноды переполняются, под выселяется первым, HPA не может считать процент загрузки.

**Чем liveness отличается от readiness?**
Провал liveness — перезапуск контейнера. Провал readiness — под убирается из балансировки без перезапуска. В liveness нельзя проверять внешние зависимости, иначе их недоступность вызовет каскадный перезапуск всех подов.

**Как ограничить сетевой доступ между подами и namespace?**
NetworkPolicy: политика default deny на namespace и разрешающие политики по `podSelector` и `namespaceSelector`. CNI должен поддерживать NetworkPolicy (Calico, Cilium, VPC CNI с network policy agent). При запрете исходящего трафика — разрешить DNS.

**Как правильно хранить секреты?**
Не в git открытым текстом: External Secrets Operator с AWS Secrets Manager или SSM, Sealed Secrets, SOPS. Включить шифрование Secret в etcd через KMS. Ограничить RBAC-доступ к Secret. Помнить, что Secret — это base64, а не шифрование.

**Как обновить версию кластера?**
Проверить устаревшие API в манифестах заранее (`pluto`, `kubent` — утилиты, которые находят в манифестах и Helm-релизах API-версии, удалённые в новой версии Kubernetes), обновить control plane (по одной минорной версии за раз), затем аддоны (CoreDNS, kube-proxy, CNI, CSI-драйверы), затем ноды: новая группа нод или drift в Karpenter (Karpenter замечает, что нода не соответствует текущему NodePool или AMI, и сам её заменяет), cordon и drain старых с учётом PDB.

**Что такое оператор и CRD?**
CRD добавляет в API новый тип ресурса. Оператор — контроллер, который реконсилит эти ресурсы и содержит в коде знания администратора (развернуть кластер БД, сделать бэкап, переключить мастера). Примеры: cert-manager, Prometheus Operator, ArgoCD.

**HPA не масштабирует. Почему?**
Нет metrics-server; у контейнеров нет requests (процент считается от них); достигнут `maxReplicas`; метрика не поступает (`kubectl describe hpa` покажет ошибку); масштабирование упирается в отсутствие нод (поды Pending).

**Под не может обратиться к сервису по DNS-имени.**
Проверить CoreDNS (поды и логи в `kube-system`), `/etc/resolv.conf` внутри пода, `nslookup api.prod.svc.cluster.local` изнутри пода, NetworkPolicy на исходящий трафик к порту 53, существование сервиса в нужном namespace (без namespace имя ищется в своём).

**Taint и toleration — как выделить ноды только под определённое приложение?**
Taint на нодах (чужие поды не придут) плюс у нужных подов toleration (могут прийти) и nodeAffinity или nodeSelector (должны прийти именно туда). Одна toleration не притягивает под к ноде.

**Под в Pending, хотя ресурсы в кластере есть.**
Events в `describe pod`: не подходят taints или affinity; PVC в другой зоне (нужен `WaitForFirstConsumer`); ресурсы есть суммарно, но ни на одной ноде нет нужного куска (фрагментация); ResourceQuota namespace исчерпана; лимит подов на ноде (в EKS с VPC CNI зависит от количества ENI — сетевых интерфейсов — и IP на каждом у типа инстанса: например, у `t3.medium` всего 17 подов).

**Чем ClusterIP отличается от NodePort и LoadBalancer?**
ClusterIP — внутренний адрес, доступен только в кластере. NodePort — плюс порт 30000–32767 на каждой ноде. LoadBalancer — плюс внешний балансировщик облака. Каждый следующий тип включает предыдущий.

**Зачем нужен headless Service?**
Без виртуального IP: DNS возвращает IP всех подов напрямую. Нужен StatefulSet'ам (стабильные DNS-имена каждого пода) и клиентам, которые сами балансируют или должны знать все реплики (кластеры БД, Kafka).

**Как под получает доступ к AWS (например, к S3)?**
Через IRSA или EKS Pod Identity: ServiceAccount пода связывается с IAM-ролью, под получает временные креды только этой роли. Не через роль ноды — тогда доступ получили бы все поды на ноде.

**Как удалить под без kubectl?**
Вопрос с подвохом про static pods: ими управляет kubelet напрямую по файлам в `/etc/kubernetes/manifests/`. Удалить такой под — удалить файл манифеста на ноде; `kubectl delete` не поможет, kubelet создаст его снова. Отличить static pod можно по имени: оно заканчивается именем ноды.

**Чем `kubectl create` отличается от `kubectl apply`?**
`create` — императивное создание, падает, если объект существует. `apply` — декларативное: создаёт или обновляет до описанного состояния, запоминает последнюю применённую конфигурацию в аннотации и по ней понимает, какие поля удалить. Для GitOps подходит только apply.

**Чем Karpenter лучше Cluster Autoscaler?**
Cluster Autoscaler масштабирует заранее созданные группы одинаковых нод (ASG): для разных типов инстансов и spot нужно много групп, переключение медленное. Karpenter работает без групп: по requirements pending-подов сам выбирает самый дешёвый подходящий тип инстанса, быстрее запускает ноды, сам переключается между spot и on-demand и постоянно уплотняет нагрузку (consolidation), удаляя пустые и недогруженные ноды.

**HPA, KEDA и Karpenter — в чём разница и как они работают вместе?**
Они на разных уровнях. HPA меняет число реплик по CPU и памяти относительно requests, минимум одна реплика. KEDA тоже меняет число реплик, но по внешним событиям — длина очереди, consumer lag Kafka, запрос в Prometheus, cron — и умеет опускаться до нуля; под капотом она создаёт обычный HPA, поэтому свой HPA на тот же Deployment вешать нельзя. Karpenter масштабирует не поды, а ноды: видит поды в Pending, подбирает самый дешёвый подходящий тип инстанса (включая Spot) и убирает лишние ноды через consolidation. Типичная связка: HPA для API, KEDA для воркеров очередей, Karpenter под ними для нод; всем им нужны правильные requests.

**Почему Karpenter не удаляет ноды после снятия нагрузки?**
На ноде под с аннотацией `karpenter.sh/do-not-disrupt`, PDB не даёт выселить поды, disruption budget в текущем окне равен 0, `consolidationPolicy: WhenEmpty` (недогруженные не трогаются), или на ноде поды без контроллера. Смотреть логи Karpenter по `disrupting nodeclaim` и события на нодах.

**Почему HPA медленно уменьшает реплики?**
По умолчанию окно стабилизации для масштабирования вниз — 300 секунд: HPA ждёт 5 минут устойчиво низкой нагрузки, чтобы не удалять реплики при кратковременном спаде. Настраивается в `spec.behavior.scaleDown`.

**Чем отличаются Flannel, Calico и Cilium?**
Flannel — простой overlay на VXLAN без поддержки NetworkPolicy. Calico — маршрутизация L3 с обменом маршрутами по BGP (или туннели), полноценные NetworkPolicy и свои расширенные политики. Cilium — на eBPF: высокая производительность, замена kube-proxy, L7-политики, наблюдаемость через Hubble, прозрачное шифрование.

**Как внешний запрос в EKS доходит до пода? Участвует ли в этом kube-proxy?**
Зависит от того, как настроен вход. С AWS Load Balancer Controller в режиме IP контроллер только настраивает ALB, а сам ALB по правилам Ingress отправляет запрос сразу на IP пода, зарегистрированный в target group: kube-proxy, NodePort и ClusterIP не участвуют, это возможно благодаря тому, что VPC CNI выдаёт подам адреса из VPC. В режиме instance ALB шлёт на NodePort любой ноды, и уже kube-proxy подменяет адрес на IP пода, возможно на другой ноде. С ingress-nginx запрос идёт через NLB в поды nginx, которые проксируют прямо на IP подов из EndpointSlice. ClusterIP и kube-proxy работают в основном для трафика внутри кластера.

**Трафик вырос в 10 раз за минуту. Что произойдёт с кластером и как к этому подготовиться?**
HPA увидит рост метрик за 15–30 секунд и добавит реплики, но если свободных нод нет, поды уйдут в Pending, а Karpenter или Cluster Autoscaler добавит ноды только через 40–90 секунд, плюс скачивание образа и старт приложения: «холодный» под — 1–3 минуты. Готовятся заранее: резерв ёмкости через поды-заглушки с низким приоритетом (overprovisioning), масштабирование по метрике-предвестнику (RPS, очередь) вместо CPU, быстрый `behavior.scaleUp`, маленькие образы и быстрый старт, запас минимума реплик на известные пики (KEDA cron). Плюс зависимости: пул соединений к БД, квоты AWS, свободные IP в подсетях, rate limiting, чтобы при перегрузке часть запросов получила 429, а не упало всё.

**Чем VPA отличается от HPA и можно ли их совмещать?**
HPA меняет число реплик, VPA — requests и limits одного пода по истории потребления. Совмещать по одной метрике нельзя: на рост нагрузки HPA добавит реплики, а VPA увеличит под, и они будут мешать друг другу. Рабочие варианты: HPA по CPU или RPS + VPA только по памяти, или VPA в режиме `Off` как источник рекомендаций для ручной настройки. Классический VPA применяет новые значения пересозданием пода, поэтому в проде его часто держат в `Off` или `Initial`.

**Когда нужен KEDA вместо обычного HPA?**
Когда масштабировать нужно по внешнему событию: длина очереди SQS, отставание консьюмера Kafka, запрос в Prometheus, расписание. И когда нужно опускаться до нуля реплик, если работы нет. KEDA создаёт и ведёт обычный HPA, добавляя к нему источники метрик и режим scale-to-zero. Типичный пример — воркеры очереди, у которых CPU низкий, а сообщений накопились тысячи.

**Как защитить Kubernetes-кластер, кроме RBAC?**
Слоями: NetworkPolicy с запретом по умолчанию, Pod Security Admission или Kyverno против privileged-контейнеров и запуска от root, только подписанные и просканированные образы из своего registry, секреты зашифрованы KMS в etcd и приходят из Secrets Manager через External Secrets, у подов свои IAM-роли через IRSA или Pod Identity и закрытый доступ к IMDS (hop limit 1), приватный API endpoint и audit-логи, рантайм-детект (Falco, GuardDuty), минимальная ОС нод без SSH.

## Kubernetes The Hard Way: конспект прохождения

Итоги ручной сборки кластера по гайду Kelsey Hightower (пройден в октябре 2026 на Mac с процессором Apple M1, версия гайда с поддержкой arm64 — 64-битной архитектуры ARM). Зачем вообще его проходить — в разделе [Практика: Kubernetes The Hard Way](#01-kubernetes.md/s1). Здесь — что именно делается на каждом шаге, как это работает, как то же самое устроено в kubeadm (официальной утилите установки) и EKS (управляемом Kubernetes в AWS), и реальная поломка, которую пришлось чинить.

### Стенд: 4 виртуальные машины в Lima
**Lima** — утилита для Mac, которая одной командой поднимает Linux-VM (виртуальную машину) на встроенной виртуализации Apple. Гайд требует 4 машины на Debian 12, которые видят друг друга по сети. CPU в таблице — виртуальные ядра процессора, GB — гигабайты оперативной памяти:

| Машина | Роль | Ресурсы |
|---|---|---|
| `jumpbox` | Отсюда выполняются все команды: генерация сертификатов, раскладка файлов по SSH (Secure Shell — шифрованный удалённый вход) | 1 CPU, 1 GB |
| `server` | Control plane (управляющая часть кластера): etcd, kube-apiserver, scheduler, controller-manager | 1 CPU, 2 GB |
| `node-0`, `node-1` | Воркеры (ноды, где работают поды): containerd, kubelet, kube-proxy | по 1 CPU, 2 GB |

```bash
limactl start --name=node-0 --containerd=none --cpus=1 --memory=2 --disk=20 \
  --network=lima:user-v2 template://debian-12
limactl shell node-0 -- sudo -i     # зайти на машину под root
limactl delete -f node-0            # удалить машину вместе с диском
```
- Сеть `lima:user-v2` — общая виртуальная сеть Lima, в которой виртуальные машины видят друг друга по IP-адресам. Без неё каждая VM изолирована и видит только Mac.
- `--containerd=none` — не ставить предустановленный containerd Lima. Без этого флага получается конфликт версий (см. «Инцидент» ниже).

**Что пришлось адаптировать под Lima:**
- **Вход root по SSH.** Гайд включает вход root по паролю и делает `ssh-copy-id`. У root в образе Lima нет пароля, поэтому публичный ключ jumpbox раскладывается в `/root/.ssh/authorized_keys` остальных машин через `limactl shell` с Mac. Вход по ключу для root в Debian разрешён по умолчанию (`PermitRootLogin prohibit-password`).
- **`/etc/hosts` без строки `127.0.1.1`.** Команда гайда `sed` её заменяет, а раз строки нет — молча ничего не делает. Строку нужно добавить.
- **cloud-init** (система первоначальной настройки облачных образов) при каждой загрузке возвращает имя машины `lima-<имя>`. Лечится файлом `/etc/cloud/cloud.cfg.d/99-keep-hostname.cfg` со строкой `preserve_hostname: true`. Иначе после перезагрузки имя не совпадёт с сертификатами.

### Как устроен собранный кластер
```text
jumpbox ── kubectl (admin) ──────────────────────────────┐
                                                         ▼
server:   etcd ◄── kube-apiserver ◄── kube-scheduler, kube-controller-manager
                        ▲     │
          kubelet, kube-proxy (node-0, node-1) ─┘     └──► kubelet (logs, exec)
node-N:   kubelet → containerd → containerd-shim-runc-v2 → runc → контейнер
```
- **etcd** — база «ключ-значение», где хранится всё состояние кластера. С ней разговаривает только kube-apiserver.
- **kube-apiserver** — сервер API (Application Programming Interface — программного интерфейса, через который управляется кластер) и единственная точка входа: все компоненты и ты сам работают только через него, а не друг с другом.
- **scheduler** выбирает ноду для нового пода, **controller-manager** приводит фактическое состояние к желаемому (например, создаёт поды для Deployment). Оба — обычные клиенты API.
- **kubelet** на ноде видит в API поды, назначенные на его ноду, и просит **containerd** (среду запуска контейнеров) их запустить. containerd на каждый контейнер запускает **shim** (`containerd-shim-runc-v2`, процесс-посредник, который держит контейнер, даже если сам containerd перезапустят), а shim вызывает **runc** — утилиту, которая через механизмы ядра (namespaces, cgroups) создаёт сам контейнер.
- **kube-proxy** превращает Services в правила iptables (сетевой фильтр ядра Linux), чтобы запрос на ClusterIP (виртуальный IP-адрес сервиса) попадал на один из подов.
- Порядок шагов гайда повторяет зависимости: сначала PKI (сертификаты) и конфиги (04–06), потом etcd (07), потом control plane (08), потом ноды (09), сеть (11) и проверка (12).

### Шаг 04: PKI — кто кому предъявляет сертификат
**PKI** (Public Key Infrastructure — инфраструктура сертификатов) кластера: свой **CA** (Certificate Authority — центр сертификации, который подписывает остальные сертификаты) и по сертификату на каждый компонент. Все соединения идут по **mTLS** (mutual TLS — обе стороны предъявляют сертификаты, а не только сервер). kube-apiserver берёт из клиентского сертификата **CN** (Common Name) как **имя пользователя** и **O** (Organization) как **группу**, а дальше **RBAC** (Role-Based Access Control — права по ролям) решает, что этому клиенту можно.

Описание всех сертификатов лежит в `ca.conf` — конфиге для OpenSSL, по секции на компонент:

| Сертификат | CN → пользователь | O → группа | Роль в TLS (Transport Layer Security — шифрование соединения) | Зачем |
|---|---|---|---|---|
| `ca` | `CA` | — | подписывает остальные | Корень доверия. Кто владеет `ca.key`, может выпустить себе любой доступ |
| `admin` | `admin` | `system:masters` | клиент | Ты с kubectl. `system:masters` обходит RBAC совсем, отозвать такой сертификат нельзя |
| `node-0`, `node-1` | `system:node:node-0` | `system:nodes` | клиент и сервер | kubelet. Node Authorizer (особый режим авторизации для нод) пускает ноду только к её подам; как сервер kubelet принимает `logs`/`exec` от apiserver |
| `kube-proxy` | `system:kube-proxy` | `system:node-proxier` | клиент | Встроенная роль: читать Services и EndpointSlices |
| `kube-controller-manager` | `system:kube-controller-manager` | — | клиент | Встроенная привязка прав по этому имени |
| `kube-scheduler` | `system:kube-scheduler` | — | клиент | То же (в `ca.conf` в поле O опечатка `system:system:`, на работу не влияет: права привязаны к CN) |
| `kube-api-server` | `kubernetes` | — | **сервер** | Главный серверный сертификат, у него длинный SAN (список имён и IP-адресов, на которые он выписан) |
| `service-accounts` | — | — | пара ключей | controller-manager подписывает токены подов, apiserver их проверяет |

**SAN** (Subject Alternative Name — список имён и IP, для которых сертификат действителен как серверный) у apiserver: `127.0.0.1`, `10.32.0.1` (первый IP-адрес диапазона сервисов — это ClusterIP сервиса `kubernetes.default`, через который поды ходят к API-серверу), `kubernetes.default.svc` и похожие имена, `server.kubernetes.local`. Обращение по адресу, которого нет в SAN, даёт ошибку `x509: certificate is valid for …, not …` — одну из самых частых при ручной сборке и после смены адреса балансировщика.

Как выпускается каждый сертификат:
```bash
openssl genrsa -out node-0.key 4096                                     # приватный ключ
openssl req -new -key node-0.key -config ca.conf -section node-0 \
  -out node-0.csr                                                       # CSR: запрос на подпись с CN, O, SAN
openssl x509 -req -in node-0.csr -CA ca.crt -CAkey ca.key \
  -copy_extensions copyall -days 3653 -out node-0.crt                   # CA подписывает
openssl x509 -in node-0.crt -noout -subject -ext subjectAltName         # проверить, что получилось
```
**CSR** (Certificate Signing Request) — запрос на подпись: открытый ключ плюс данные владельца, которые CA превращает в сертификат.

### Шаг 05: kubeconfig для каждого компонента
**kubeconfig** — YAML-файл (текстовый формат конфигов) с тремя списками: `clusters` (адрес API-сервера и CA — центр сертификации, которому доверять), `users` (клиентский сертификат и ключ), `contexts` (связка «кластер + пользователь») и указателем `current-context`. Это «пропуск + адрес» для каждого клиента apiserver.

```bash
kubectl config set-cluster kubernetes-the-hard-way --certificate-authority=ca.crt \
  --embed-certs=true --server=https://server.kubernetes.local:6443 --kubeconfig=node-0.kubeconfig
kubectl config set-credentials system:node:node-0 --client-certificate=node-0.crt \
  --client-key=node-0.key --embed-certs=true --kubeconfig=node-0.kubeconfig
kubectl config set-context default --cluster=kubernetes-the-hard-way \
  --user=system:node:node-0 --kubeconfig=node-0.kubeconfig
kubectl config use-context default --kubeconfig=node-0.kubeconfig
```
- `--embed-certs=true` вставляет сертификаты в файл в base64, и файл становится самодостаточным: его достаточно скопировать на ноду.
- Кем компонент будет для API-сервера, решают **CN и O (имя пользователя и группа) внутри сертификата**, а не имя записи в `users`.
- Файлов пять видов: kubelet (по одному на ноду, `/var/lib/kubelet/kubeconfig`), kube-proxy (`/var/lib/kube-proxy/kubeconfig`), controller-manager и scheduler (на `server`), admin. У admin адрес `https://127.0.0.1:6443`, потому что он используется прямо на `server`; у остальных — `server.kubernetes.local`, по сети.
- В kubeconfig лежит приватный ключ, поэтому файл — это пароль: `admin.kubeconfig` даёт полную власть над кластером.

Проверить, кем представится компонент:
```bash
kubectl config view --raw --kubeconfig node-0.kubeconfig \
  -o jsonpath='{.users[0].user.client-certificate-data}' | base64 -d | openssl x509 -noout -subject
```

### Шаг 06: шифрование Secrets в etcd
По умолчанию Secret в etcd хранится открытым текстом (base64 — это кодировка, а не шифрование), и пароли видит любой, у кого есть доступ к etcd или его бэкапу. **Encryption at rest** (шифрование на хранении) значит, что apiserver шифрует Secret перед записью в etcd.

```bash
export ENCRYPTION_KEY=$(head -c 32 /dev/urandom | base64)        # 32 случайных байта = ключ AES-256
envsubst < configs/encryption-config.yaml > encryption-config.yaml # подставить ключ в шаблон
scp encryption-config.yaml root@server:~/
```
```yaml
kind: EncryptionConfiguration
apiVersion: apiserver.config.k8s.io/v1
resources:
  - resources:
      - secrets
    providers:
      - aescbc:
          keys:
            - name: key1
              secret: ${ENCRYPTION_KEY}
      - identity: {}
```
- **AES** (Advanced Encryption Standard — стандартный симметричный шифр) в режиме **CBC** (Cipher Block Chaining). Записи шифруются **первым** провайдером списка; при чтении провайдеры перебираются по очереди.
- `identity: {}` — «без шифрования», стоит последним, чтобы читать Secrets, записанные до включения шифрования. Поставить его первым — значит писать новые Secrets открытыми.
- В etcd запись получает префикс `k8s:enc:aescbc:v1:key1:` — по нему apiserver понимает, каким ключом расшифровывать.
- Ротация ключа: добавить `key2` вторым → перезапустить apiserver → поставить `key2` первым → перезапустить → перезаписать все Secrets (`kubectl get secrets -A -o json | kubectl replace -f -`) → удалить `key1`.
- Ограничение: ключ лежит в файле на том же сервере. В проде используют провайдер **KMS v2** (Key Management Service): данные шифрует ключ DEK (Data Encryption Key), а его — мастер-ключ KEK (Key Encryption Key), который не покидает внешний KMS (AWS KMS — сервис ключей Amazon Web Services, HashiCorp Vault). `aescbc` документация Kubernetes уже не рекомендует.

### Шаги 07–09: etcd, control plane и воркеры
- **07 etcd** — systemd-сервис на `server`. Однонодовый etcd без кворума (договорённости большинства узлов): для обучения достаточно, в проде etcd запускают на 3 или 5 узлах.
- **08 control plane** — kube-apiserver, controller-manager и scheduler как systemd-юниты на `server`. Ключевые флаги apiserver связывают предыдущие шаги: `--client-ca-file` (каким CA — центром сертификации — проверять клиентов, шаг 04), `--tls-cert-file` (свой серверный сертификат), `--encryption-provider-config` (шаг 06), `--service-cluster-ip-range=10.32.0.0/24` (диапазон виртуальных IP-адресов сервисов), `--authorization-mode=Node,RBAC`.
- **09 воркеры** — на каждой ноде containerd, runc, CNI-плагины (CNI, Container Network Interface — стандарт плагинов, которые выдают подам IP-адреса и подключают их к сети), kubelet и kube-proxy. У каждой ноды свой диапазон адресов подов: `node-0` — `10.200.0.0/24`, `node-1` — `10.200.1.0/24`, его раздаёт плагин `bridge` с IPAM (IP Address Management — выдача адресов) `host-local`.

### Шаг 11: маршруты между подсетями подов
Под на `node-0` (адрес из `10.200.0.0/24`) не знает, как добраться до пода на `node-1` (`10.200.1.0/24`): это разные машины. Гайд прописывает обычные статические маршруты на `server` и на нодах:
```bash
ip route add 10.200.1.0/24 via 192.168.104.5    # на node-0: подсеть подов node-1 — через IP node-1
ip route add 10.200.0.0/24 via 192.168.104.3    # на node-1: подсеть подов node-0 — через IP node-0
```
В настоящих кластерах это делает CNI (Container Network Interface — сетевой плагин кластера): Flannel и Calico строят такие маршруты или туннели сами, а в EKS (управляемом Kubernetes в AWS — Amazon Web Services) **VPC CNI** выдаёт подам адреса прямо из подсетей VPC (Virtual Private Cloud — изолированной сети в AWS), и маршруты не нужны.

### Шаг 12: smoke test — что проверяется
**Smoke test** — быстрая проверка «основные функции живы». Первым делом — что Secret действительно зашифрован в etcd (команда выполняется на `server`):
```bash
etcdctl get /registry/secrets/default/kubernetes-the-hard-way | hexdump -C | head
# в выводе виден префикс k8s:enc:aescbc:v1:key1 вместо значения секрета
```
Дальше проверяется работа кластера:
- Deployment nginx создаёт под (scheduler, controller-manager, kubelet, containerd работают);
- `kubectl port-forward`, `kubectl logs`, `kubectl exec` — apiserver достучался до kubelet по его серверному сертификату;
- Service типа NodePort (порт, открытый на каждой ноде) отвечает на `curl` — работают kube-proxy и сеть.

**Чего в кластере нет:** CoreDNS (DNS внутри кластера; DNS — система имён, которая превращает имя в IP-адрес) в текущей версии гайда не ставится. Отсюда предупреждение `MissingClusterDNS` с текстом `kubelet does not have ClusterDNS IP configured`: поды получают DNS-настройки ноды, и имена вида `nginx.default.svc` не резолвятся. Ещё нет отказоустойчивости control plane и автоматических обновлений — это учебный кластер, а не способ установки.

### Hard way, kubeadm и EKS: одно и то же в трёх вариантах
**kubeadm** — официальная утилита, которая делает шаги 04–09 автоматически. **EKS** (Elastic Kubernetes Service) — управляемый Kubernetes в AWS (Amazon Web Services), где control plane целиком на стороне AWS.

| Что | Hard way | kubeadm | EKS |
|---|---|---|---|
| Доступ администратора | `admin.kubeconfig` с сертификатом группы `system:masters` | `/etc/kubernetes/admin.conf` (с 1.29 права через RBAC-группу, отдельно аварийный `super-admin.conf`) | Файла нет: `aws eks update-kubeconfig` пишет `exec: aws eks get-token`, личность — IAM-роль (Identity and Access Management — учётки и роли AWS), права через EKS access entries (записи «IAM-роль → права в кластере») и RBAC (права по ролям) |
| Control plane | systemd-юниты на `server` | static pods (поды, которые kubelet запускает прямо из файлов в `/etc/kubernetes/manifests/`) | Скрыт, управляет AWS |
| PKI (сертификаты кластера) | Твой CA (центр сертификации) с шага 04 | `/etc/kubernetes/pki/`, сертификаты на 1 год, `kubeadm certs renew all` | У AWS, ротация автоматическая |
| kubeconfig компонентов | `/var/lib/kubelet`, `/var/lib/kube-proxy`, `/var/lib/kubernetes`, сертификаты вложены (`--embed-certs`) | `/etc/kubernetes/*.conf`, набор зависит от роли ноды | Только у kubelet на ноде, без сертификата: токен от IAM-роли инстанса |
| kubelet | Сертификат выпущен вручную | TLS bootstrapping (TLS — шифрование соединения; нода сама запрашивает сертификат по временному токену) и авторотация | Токен IAM-роли ноды → пользователь `system:node:<имя>` |
| kube-proxy | systemd на нодах, свой kubeconfig | DaemonSet (под на каждой ноде) + ConfigMap | EKS add-on, тоже DaemonSet |
| Шифрование Secrets | `aescbc`, ключ в файле | Выключено по умолчанию, тот же `EncryptionConfiguration` | AWS KMS (Key Management Service — сервис ключей), envelope encryption: данные шифрует ключ, который сам зашифрован мастер-ключом в KMS |
| Сеть подов | Статические маршруты | Ставишь CNI (сетевой плагин) сам: Calico, Cilium, Flannel | VPC CNI: поды получают IP-адреса из VPC (сети AWS) |
| Новая нода | Всё руками (шаг 09) | `kubeadm join` с bootstrap-токеном | Managed node group или Karpenter: EKS AMI (Amazon Machine Image — образ ноды) + IAM-роль |

Вывод для собеседования: в EKS шаги 04–08 забирает AWS, а у тебя остаются ноды (образ, kubelet, IAM-роль), доступ (IAM → access entries → RBAC), сеть (VPC CNI — сетевой плагин AWS) и add-ons (дополнения кластера: kube-proxy, CoreDNS, VPC CNI).

### Инцидент: под не создаётся из-за двух версий containerd
Разбор в формате **STAR** (Situation, Task, Action, Result — ситуация, задача, действия, результат), так его удобно рассказывать на собеседовании.

**Ситуация.** На smoke test под nginx висит в `ContainerCreating`, в `kubectl describe pod` событие:
```text
Warning  FailedCreatePodSandbox  kubelet  Failed to create pod sandbox: rpc error: code = Unknown
desc = failed to start sandbox "e43ef26390e515ed92fbb06fcb1e21f367b3fffa37cfba7c31f942fb254841cb":
failed to create containerd task: failed to start shim: start failed: io.containerd.runc.v2:
bootstrap parameters do not match command-line arguments: shim was started through the
deprecated API but was built against the new API; if you upgraded containerd, you may need to
restart the daemon: exit status 1
```
**Pod sandbox** — «оболочка» пода: сетевой namespace и служебный контейнер `pause`, который создаётся до контейнеров приложения. Если sandbox не создаётся, не запустится ни один контейнер пода.

**Задача.** Понять, почему containerd не может запустить shim, и починить без пересоздания кластера.

**Действия.** Проверил на ноде, какие бинарники реально используются:
```bash
which -a containerd containerd-shim-runc-v2                     # все копии в PATH по порядку
containerd --version; containerd-shim-runc-v2 -v                # версии каждой
systemctl cat containerd | grep ExecStart                       # что запускает systemd
readlink /proc/$(systemctl show containerd -p MainPID --value)/exe   # что реально работает
```
Оказалось две установки: гайд положил containerd **v2.1.0-beta** в `/bin`, а образ Lima заранее поставил свой containerd **v2.4.1** в `/usr/local/bin`. systemd запускал демон из `/bin`, а демон искал shim **по `PATH`**, где `/usr/local/bin` стоит раньше, и находил shim от другой версии с несовместимым протоколом запуска.

**Результат.** Отключил лишнюю копию и перезапустил сервисы, под поднялся:
```bash
mv /usr/local/bin/containerd /usr/local/bin/containerd.lima-disabled
mv /usr/local/bin/containerd-shim-runc-v2 /usr/local/bin/containerd-shim-runc-v2.lima-disabled
systemctl restart containerd kubelet
```
Вывод на будущее: создавать VM (виртуальные машины) с `--containerd=none`. В проде та же проблема возникает после обновления образа ноды или ручной доустановки пакетов: при `FailedCreatePodSandbox` первым делом сверяют версии и пути `containerd`, shim и `runc`.

### Чек-лист: где искать, когда кластер сломан
| Симптом | Где смотреть |
|---|---|
| `x509: certificate is valid for …, not …` | SAN (список имён и IP-адресов) серверного сертификата apiserver: `openssl x509 -in kube-api-server.crt -noout -ext subjectAltName`; адрес в kubeconfig клиента |
| `x509: certificate signed by unknown authority` | Клиент и сервер доверяют разным CA (центрам сертификации): сверить `certificate-authority-data` в kubeconfig и `--client-ca-file` apiserver |
| `x509: certificate has expired` | Сроки сертификатов: `openssl x509 -in kube-api-server.crt -noout -enddate`; в kubeadm `kubeadm certs check-expiration` |
| `forbidden: User "…" cannot …` | Аутентификация прошла, не хватает прав: CN и O сертификата (имя и группа), RBAC — права по ролям (`kubectl auth can-i get pods --as=system:node:node-0`) |
| Нода `NotReady` | `journalctl -u kubelet` на ноде: kubeconfig, доступ к API-серверу, работает ли containerd |
| Под в `ContainerCreating`, `FailedCreatePodSandbox` | `journalctl -u containerd`, версии и пути containerd, shim, runc; конфиги CNI (сетевого плагина) в `/etc/cni/net.d/` |
| Поды на разных нодах не видят друг друга | Маршруты (`ip route`) или CNI (сетевой плагин); в EKS (управляемый Kubernetes AWS) — security groups (файрвол инстансов) и VPC CNI |
| apiserver не стартует | `journalctl -u kube-apiserver`: доступ к etcd, пути к сертификатам, конфиг шифрования |
