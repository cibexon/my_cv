# Kubernetes

Kubernetes in depth: why it is needed, how it works inside, the main objects and when to use each, pod scheduling, resources, probes, networking, storage, security and troubleshooting. Each topic: what it is, why, how it works, a manifest with explanations and typical mistakes.

## Practice: Kubernetes The Hard Way

**Repository:** https://github.com/kelseyhightower/kubernetes-the-hard-way (by Kelsey Hightower).

It's a step-by-step guide to building a cluster by hand **without kubeadm** (the official tool that automatically brings up a cluster with one `kubeadm init` command) **and without cloud services**: you generate your own certificate authority (CA) and certificates for every component, write kubeconfig files (a file with the API server address and credentials a client uses to connect to the cluster), bring up etcd (the database where the whole cluster state is kept), run the control plane components (the managing part of the cluster: API server, scheduler, controllers) as systemd services, configure kubelet (the agent on every node) and containerd (the container runtime) on the workers, add routes for the pod network, install CoreDNS (DNS inside the cluster) and verify it works.

Why go through it if you use EKS at work anyway: a managed cluster hides the internals, and when something breaks it's unclear where to look. After "The Hard Way" it becomes clear that:
- **every component is just a binary with flags**, no magic;
- **all components talk only through kube-apiserver** (the only exception is the apiserver itself talking to etcd);
- **why there are so many certificates**: all connections are mutually authenticated (mTLS), and kubelet identifies itself as `system:node:<node name>` in the `system:nodes` group, while a special Node authorizer gives it access only to the pods of its own node;
- **pod networking without CNI "magic"** (CNI, Container Network Interface — a plugin that gives pods IPs and connects them across nodes): every node has its own address range for pods, and between nodes there are ordinary static routes;
- **how Secret encryption in etcd is enabled** (EncryptionConfiguration — a file for the apiserver with the encryption key; without it Secrets sit in etcd in plain form, only base64-encoded).

Additions and alternatives:
- a **kubeadm** cluster on 2–3 virtual machines (Multipass, Vagrant, Lima on a Mac — tools that bring up a Linux VM on a laptop with one command) — closer to real operations;
- **killercoda.com** and **killer.sh** — free interactive scenarios in the style of the CKA and CKAD exams (Certified Kubernetes Administrator / Application Developer — hands-on CNCF exams in a terminal);
- **kind** (Kubernetes in Docker — the cluster nodes are Docker containers), **k3d** (the same for the lightweight k3s), **minikube** — a local cluster in a minute for experimenting with manifests;
- your own repositories: `~/work/GitHubRepos/mastering-k8s`, `k8s-controller-tutorial`.

## Why Kubernetes is needed

Docker runs containers on **one** machine. Production needs much more:
- running hundreds of containers on dozens of servers and **deciding where to place each one** based on free resources;
- **restarting crashed ones** and moving containers from a dead server to live ones;
- **scaling**: adding copies under load and removing them when it drops;
- **updating without downtime**: gradually replacing the old version with the new one and rolling back on problems;
- giving services a **stable address and load balancing**, even though individual containers are constantly recreated with new IPs;
- distributing **configuration and secrets**, attaching **persistent disks**.

Kubernetes is an **orchestrator**: it does all of this automatically.

### The core idea: declarative configuration and the reconciliation loop
You don't say "start 3 containers" (imperatively). You describe the **desired state**: "3 replicas of version 1.4 of the application must be running". Kubernetes stores this description, and a set of **controllers** in an endless loop compares the desired state with the actual state and removes the difference. This loop is called the **reconciliation loop**.

Consequence: a pod crashes — the controller sees that 2 of 3 replicas are running and creates a new one. You did nothing. You changed the version in the manifest — the controller sees the difference and rolls out the new version. This is what **self-healing** means.

### 🧪 Mini task

**Task.** Create a `web` Deployment with 3 nginx replicas, delete one pod and see what Kubernetes does. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
kubectl create deployment web --image=nginx:1.27 --replicas=3 > /dev/null
kubectl rollout status deployment/web > /dev/null
victim=$(kubectl get pods -l app=web -o name | head -1)
kubectl delete "$victim" --wait=false
sleep 5
kubectl get pods -l app=web --no-headers | awk '{print $1, $3}'
kubectl delete deployment web > /dev/null
```

Expected output:

```text
pod "web-6d4cf56db6-4xk2p" deleted
web-6d4cf56db6-9tq7z Running
web-6d4cf56db6-hv5mc Running
web-6d4cf56db6-r2nwd Running
```

Breakdown: a new pod with a different name appeared right away in place of the deleted one — the ReplicaSet saw 2 pods instead of 3 and created the missing one. That's the core idea of Kubernetes: you declare the desired state ("3 replicas"), and controllers keep driving reality towards it. A crashed process or a lost node is survived the same way. Pods are disposable: they're replaced, not fixed by hand, so never rely on a specific pod's name or IP. Your pod name suffixes and IPs will differ.

</details>

## Cluster architecture

```
           ┌──────────────────── Control Plane ─────────────────────┐
kubectl ─► │  kube-apiserver ◄────► etcd (the whole cluster state)   │
           │     ▲    ▲    ▲                                         │
           │     │    │    └── kube-scheduler (where to put a pod)   │
           │     │    └─────── kube-controller-manager (controllers) │
           │     └──────────── cloud-controller-manager (the cloud)  │
           └─────▲───────────────────────────────────────────────────┘
                 │  watch: "is there any work for me?"
   ┌─────────────┴────────── Worker node ───────────────────────┐
   │  kubelet ──CRI──► containerd ──► runc ──► pod containers   │
   │  kube-proxy (rules for Services)     CNI plugin (network)  │
   └─────────────────────────────────────────────────────────────┘
```

### Control plane
**kube-apiserver** is the only entry point into the cluster. Both `kubectl` and all other components work through it. Every request goes through these stages:
1. **Authentication** — who you are (a certificate, a token, IAM in EKS — in AWS an IAM user or role is mapped to a Kubernetes user);
2. **Authorization** — what you're allowed to do (RBAC, Role-Based Access Control — rules like "user X may read pods in namespace Y", see the "Security" section);
3. **Admission controllers** (plugins that check and modify an object before it's saved) — first mutating (they can change the object: add a sidecar — a helper container, default values), then validating (they can reject it: security policies, Kyverno, OPA Gatekeeper — policy engines with rules like "running images not from our registry is forbidden");
4. **Write to etcd.**

The apiserver doesn't keep state itself, so it is easy to scale horizontally.

**etcd** is a distributed key-value store holding **all** of the cluster state: every object, every pod, every Secret. For consistency between replicas it uses the **Raft** algorithm, which needs a **quorum** — a majority of nodes:
- 3 nodes survive the loss of 1;
- 5 nodes survive the loss of 2;
- hence the odd number of nodes: 4 nodes survive the same single loss as 3, but are slower.

Losing quorum — the cluster cannot write anything. Losing etcd data without a backup — losing the cluster. Backup: `etcdctl snapshot save`. In EKS, AWS manages etcd.

**kube-scheduler** picks a node for every new pod that doesn't have a node yet. Two stages:
1. **Filtering** — discard nodes where the pod can't run: not enough resources (by requests — the CPU and memory the pod declares), a taint without a toleration (a taint is a "keep-out mark" on a node, a toleration is the pod's permission to tolerate it; see the scheduling section), nodeAffinity doesn't match (the pod's requirement on node labels), the disk is in another zone;
2. **Scoring** — rank the remaining ones: even load, spreading replicas across nodes and zones, the image already being on the node (no need to pull it).

Then it writes the chosen node into the pod's `spec.nodeName` field (binding). The scheduler itself doesn't start anything.

**kube-controller-manager** is a single process running dozens of controllers, each with its own reconciliation loop: the Deployment controller, the ReplicaSet controller, the Node controller (watches node availability), the Job controller, the EndpointSlice controller (links a Service to pods) and others.

**cloud-controller-manager** — integration with the cloud: creates load balancers for Services of type LoadBalancer, fetches node metadata (zone, instance type).

### Worker node components
**kubelet** — the agent on every node. Through the apiserver it watches the pods assigned to its node; via the **CRI** (Container Runtime Interface — the standard interface kubelet uses to drive any container runtime) it asks containerd to pull images and start containers; runs probes (container liveness and readiness checks, see the "Probes" section); mounts volumes; reports pod and node status.

**kube-proxy** — implements Services: on every node it creates iptables or IPVS rules (IPVS is a load balancer in the Linux kernel) that redirect traffic from the service's virtual IP to the real pod IPs. With Cilium it can be replaced by eBPF (programs running right in the kernel — faster than long iptables chains).

**CNI plugin** (Container Network Interface) gives a pod an IP address and provides connectivity between pods across nodes. Examples: Calico, Cilium, Flannel, **AWS VPC CNI** (a pod gets a real IP from the VPC subnet).

**CoreDNS** is the DNS server inside the cluster: the `api` service in the `prod` namespace is reachable as `api.prod.svc.cluster.local`.

### What happens on `kubectl apply -f deployment.yaml`
This is a classic question. The answer shows that you understand the architecture:
1. **kubectl** reads the kubeconfig and sends the manifest to the apiserver.
2. **apiserver**: authentication, RBAC authorization, admission controllers, storing the Deployment in **etcd**.
3. The **Deployment controller** (via a watch on the apiserver) sees the new Deployment and creates a **ReplicaSet**.
4. The **ReplicaSet controller** sees a ReplicaSet that needs 3 pods and creates 3 **Pod** objects without an assigned node.
5. The **scheduler** sees the unassigned pods, picks nodes and writes `nodeName`.
6. **kubelet** on the chosen node sees that a pod has been assigned to it: containerd pulls the image, CNI hands out an IP, volumes are mounted, init containers (preparatory ones, run one by one before the main ones) start, then the main ones.
7. kubelet runs the probes and updates the pod status. When the **readiness probe** (the "ready to take traffic" check) passes, the EndpointSlice controller adds the pod IP to the service's EndpointSlice (the list of ready pod addresses), and **kube-proxy** on all nodes updates its rules. Traffic starts flowing to the new pod.

Note: components don't call each other directly. Each one only **watches** changes through the apiserver and writes its result back there.

### 🧪 Mini task

**Task.** Find all the control plane and node components in the cluster: which node they run on and which of them run as static pods (pods kubelet starts itself from files in `/etc/kubernetes/manifests`, without the apiserver), and which via a DaemonSet (a controller of "one pod on every node"). You need the kind training cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
kubectl get pods -n kube-system -o custom-columns=POD:.metadata.name,NODE:.spec.nodeName,OWNER:.metadata.ownerReferences[0].kind --no-headers | sort
```

Expected output:

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

Breakdown: an owner of `Node` means a static pod: kubelet starts etcd, the apiserver, the controller-manager and the scheduler itself from manifests on the control plane node's disk (hence the node name in their names). `kube-proxy` and the CNI (`kindnet`) are DaemonSets, one per node. CoreDNS is a regular Deployment. In EKS the control plane is hidden and managed by AWS, and `kube-system` shows only the add-ons: CoreDNS, kube-proxy and the VPC CNI (`aws-node`). Your pod name suffixes and IPs will differ.

</details>

## Main objects

All objects are described by YAML manifests with a common structure:
```yaml
apiVersion: apps/v1         # API group and version of the object
kind: Deployment            # object type
metadata:
  name: api                 # name, unique within the namespace
  namespace: prod
  labels:                   # labels — key-value pairs for grouping and selection
    app: api
spec:                       # desired state — what you want
  ...
status:                     # actual state — filled in by Kubernetes, never written by hand
  ...
```
**Labels and selectors** are the main mechanism for linking objects. A Deployment finds its pods, a Service its backends, a NetworkPolicy its targets — not by name but by labels: "all pods with `app: api`".

### Pod
The smallest unit of execution: **one or more containers** that are always placed on the same node and share:
- the **network** — one IP per pod, containers talk to each other over `localhost`;
- **volumes** — they can mount the same volumes.

Why several containers in a pod: a helper container (**sidecar**) next to the main one — a service mesh proxy (Envoy), a log collection agent, a process that refreshes the config. **Init containers** run one by one **before** the main ones and must complete successfully: prepare the config, wait for the database, run a migration. Since version 1.29 there are "native sidecars": an init container with `restartPolicy: Always` starts before the main one and runs for the pod's whole lifetime.

Pods are **ephemeral**: when recreated, a pod gets a new name and a new IP. That's why pods are almost never created directly; controllers (Deployment and others) are used instead, and they're reached through a Service (an object with a stable name and IP in front of a group of pods, see below).

### Deployment and ReplicaSet — for stateless applications
A **ReplicaSet** keeps a given number of identical pods. A **Deployment** manages ReplicaSets and adds version updates and rollback. ReplicaSets are almost never created directly. A stateless application is one that keeps no important data on its own disk (a web API, a frontend, queue workers): any copy can be killed and started again anywhere.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 3
  selector:
    matchLabels:
      app: api                 # "my pods are the ones labelled app=api"
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1              # how many pods may be created ABOVE replicas during an update
      maxUnavailable: 0        # how many may be unavailable — 0 means no loss of capacity
  template:                    # pod template
    metadata:
      labels:
        app: api               # must match the selector
    spec:
      containers:
        - name: api
          image: registry.example.com/api:1.4.2
          ports:
            - containerPort: 8080
```
**How an update works.** When the template changes (for example, the image tag), the Deployment creates a **new ReplicaSet** and gradually scales it up while scaling the old one down, respecting `maxSurge` and `maxUnavailable`. A new pod counts as available when it passes the readiness probe. The old ReplicaSet stays with 0 replicas — you can roll back to it:
```bash
kubectl rollout status deploy/api      # watch the rollout
kubectl rollout history deploy/api     # revision history
kubectl rollout undo deploy/api        # roll back to the previous revision
kubectl rollout restart deploy/api     # recreate all pods (for example, to pick up a new Secret)
```
The `Recreate` strategy first deletes all old pods and then creates new ones. There will be downtime, but it is needed when two versions cannot run at the same time.

### StatefulSet — for stateful applications
For databases, Kafka (a distributed message queue), ZooKeeper (a cluster coordination service), Elasticsearch (a search database): the identity of each replica matters to them — each has its own data, and the replicas know each other by name.

How it differs from a Deployment:
- **stable names**: `kafka-0`, `kafka-1`, `kafka-2` — after being recreated a pod gets **the same** name;
- **stable DNS** for each pod via a headless Service (a Service without a virtual IP, see below): `kafka-0.kafka.prod.svc.cluster.local` (replicas find each other by it);
- **a persistent disk of its own for each replica** via `volumeClaimTemplates` (a template for a disk request — a PVC, see the "Storage" section): after being recreated, `kafka-0` attaches to **its own** disk `data-kafka-0`;
- **ordered** startup (0, then 1, then 2 — each after the previous one is ready) and shutdown in reverse order.

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
spec:
  serviceName: postgres          # headless Service for the pods' DNS names
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
  volumeClaimTemplates:          # each replica gets its own PVC: data-postgres-0, data-postgres-1...
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        storageClassName: gp3
        resources:
          requests:
            storage: 50Gi
```
When a StatefulSet is deleted, its PVCs are **not deleted** — protection against data loss.

### DaemonSet — one pod on every node
For agents that must run on every node: log collection (Fluent Bit — a lightweight agent that reads container logs from the node's disk and ships them to storage), node metrics (node-exporter — exposes the node's CPU, memory and disk metrics to Prometheus), the CNI plugin, kube-proxy. When a node is added, the pod appears on it automatically. With nodeSelector (choosing nodes by labels) and tolerations you can restrict the set of nodes or, conversely, allow running on tainted nodes.

### Job and CronJob — one-off and periodic tasks
A **Job** runs a pod (or several) until **successful completion**: a DB migration, one-off data processing. `backoffLimit` — how many times to retry on failure (the pauses between attempts grow exponentially), `activeDeadlineSeconds` — the overall timeout after which the Job is considered failed.

A **CronJob** creates Jobs on a schedule (cron format):
```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: backup
spec:
  schedule: "0 3 * * *"            # every day at 03:00
  concurrencyPolicy: Forbid         # don't start a new one if the previous is still running
  successfulJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 2
      template:
        spec:
          restartPolicy: OnFailure  # for a Job: OnFailure or Never (Always is not allowed)
          containers:
            - name: backup
              image: registry.example.com/backup:1.0
```

### Service — a stable address for a set of pods
**The problem.** Pods are recreated and get new IPs, and there are several of them. How do other services reach them?

A **Service** provides a **permanent virtual IP (ClusterIP) and DNS name** for all pods selected by labels and load-balances traffic between them.
```yaml
apiVersion: v1
kind: Service
metadata:
  name: api
spec:
  selector:
    app: api              # traffic goes to pods with this label that are ready (Ready)
  ports:
    - port: 80            # the service port — what clients connect to
      targetPort: 8080    # the container port where traffic actually goes
```
Now other pods call `http://api` (in the same namespace) or `http://api.prod.svc.cluster.local`.

**How it works.** A Service has no process. Its IP is **virtual**: no interface has it. The EndpointSlice controller keeps the list of ready pod IPs, and kube-proxy on **every** node writes iptables or IPVS rules: "rewrite (DNAT) a packet to ClusterIP:80 to the IP of one of the pods:8080". Load balancing happens right on the client's node.

**Service types:**
- **ClusterIP** (default) — reachable only inside the cluster.
- **NodePort** — additionally opens a port (30000–32767) on **every** node; traffic to `any_node:port` reaches the service.
- **LoadBalancer** — in addition, the cloud creates an external load balancer (in AWS with the AWS Load Balancer Controller — a controller that creates and configures AWS load balancers from Kubernetes objects — it's an NLB) pointing at the service.
- **ExternalName** — a DNS alias (CNAME) to an external name, for example to RDS (the AWS managed database): the application calls `db`, and the database address can be changed in one place.
- **Headless** (`clusterIP: None`) — no virtual IP; a DNS query returns the IPs of all pods directly. Needed for StatefulSets and client-side load balancing.

### Ingress and Gateway API — HTTP routing from outside
Creating a separate load balancer (`type: LoadBalancer`) for every service is expensive. An **Ingress** describes L7 rules: one entry point for many services, routing by domain and path, TLS.
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: main
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing   # parameters of a specific controller
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
An Ingress object on its own does nothing: you need an **Ingress Controller** that reads it and configures a real load balancer (ingress-nginx, the AWS Load Balancer Controller — creates an ALB, Traefik). The **Gateway API** is the new standard replacing Ingress: more expressive, with separation of roles (the infrastructure team manages the Gateway, the application team manages HTTPRoute routes).

### ConfigMap and Secret — configuration
They move settings out of the image so that one image works in all environments.
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
stringData:                     # stringData — plain text, Kubernetes base64-encodes it itself
  DB_PASSWORD: s3cr3t
```
Attaching to a pod — as environment variables or as files:
```yaml
containers:
  - name: api
    envFrom:
      - configMapRef: { name: api-config }     # all keys as environment variables
    env:
      - name: DB_PASSWORD
        valueFrom:
          secretKeyRef: { name: api-secrets, key: DB_PASSWORD }
    volumeMounts:
      - name: config
        mountPath: /app/config                 # ConfigMap keys as files in this folder
volumes:
  - name: config
    configMap: { name: api-config }
```
Details:
- **A Secret is base64, not encryption.** Anyone allowed to read the Secret sees the value. Protection: RBAC on Secrets, encryption in etcd (in EKS — via KMS, Key Management Service — the AWS encryption key service), and values aren't stored in git — people use the External Secrets Operator (a controller that pulls secrets from AWS Secrets Manager or Vault itself and creates Secrets from them), Sealed Secrets (the secret is encrypted with the cluster's public key, and the encrypted version can be committed safely) or SOPS (encrypting values in YAML files with a key from KMS).
- **Environment variables don't update** when a ConfigMap changes — the pod has to be restarted. Files from a volume do update (with up to a minute's delay), but the application must be able to reread its config. A common trick is a hash of the config in an annotation of the pod template (Helm `checksum/config` — the sha256 of the ConfigMap contents goes into the annotation) or `configMapGenerator` in Kustomize (it appends a hash to the ConfigMap name): a config change changes the template, and the Deployment rolls the pods automatically.

### Storage: PersistentVolume, PersistentVolumeClaim, StorageClass
**The problem.** A container's filesystem disappears with it. A database needs a disk that survives pod recreation and moving to another node.

Three objects split the responsibility:
- **PersistentVolumeClaim (PVC)** — the application's **request**: "I need a 50 GB ReadWriteOnce disk of class gp3". Written by the developer.
- **StorageClass** — the **type** of storage and how it's created: which driver (EBS CSI — CSI, Container Storage Interface, is the standard plugin through which Kubernetes creates and attaches the disks of a particular storage system), which parameters (gp3, encryption), what to do with the disk after the claim is deleted.
- **PersistentVolume (PV)** — a **specific** disk. With dynamic provisioning it is created automatically from the PVC.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3
provisioner: ebs.csi.aws.com
parameters:
  type: gp3
  encrypted: "true"
reclaimPolicy: Delete                     # delete the EBS volume when the PVC is deleted (Retain — keep it)
volumeBindingMode: WaitForFirstConsumer   # create the disk once the pod is already assigned to a node
allowVolumeExpansion: true
```
**Why `WaitForFirstConsumer` matters.** An EBS volume exists in **one** availability zone. If the disk is created immediately (`Immediate`), it may end up in zone `us-east-2a`, while the scheduler places the pod on a node in `us-east-2b` — the pod can't attach the disk and hangs in Pending. With `WaitForFirstConsumer` the disk is created in the zone of the node the pod has already been assigned to.

**Access modes:**
- `ReadWriteOnce` (RWO) — read and write from one node (EBS);
- `ReadOnlyMany` (ROX) — read-only from many nodes;
- `ReadWriteMany` (RWX) — read and write from many nodes (EFS — managed NFS in AWS, NFS): for example, a shared uploads folder for several replicas of a web application;
- `ReadWriteOncePod` (RWOP) — a single pod only.

### Namespace, ResourceQuota, LimitRange
A **Namespace** is a logical division of the cluster: by team, environment, application. Object names are unique within a namespace. Permissions (RBAC), quotas and network policies are assigned per namespace. Namespaces are not a security boundary by themselves: pods from different namespaces can see each other over the network by default.

A **ResourceQuota** limits total consumption in a namespace (no more than 20 CPU in requests, no more than 50 pods). A **LimitRange** sets default requests and limits for containers that don't specify them.

### 🧪 Mini task

**Task.** Create a Deployment and walk the owner chain upwards: from a pod to its ReplicaSet and from the ReplicaSet to the Deployment. Then delete the Deployment and check what happened to the other objects. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
pod api-5d8c6b7f9c-qw4zr -> ReplicaSet api-5d8c6b7f9c
rs api-5d8c6b7f9c -> Deployment/api
No resources found in default namespace.
```

Breakdown: a Deployment doesn't manage pods directly — it creates a ReplicaSet (one per pod template version), and the ReplicaSet creates the pods. The link lives in `ownerReferences`. The `5d8c6b7f9c` suffix is a hash of the pod template: changing the image creates a new ReplicaSet with a different hash, and the old one stays for rollback. When the Deployment was deleted, the garbage collector followed `ownerReferences` and removed both the ReplicaSet and the pods (cascading deletion; `--cascade=orphan` would have kept them). Your pod name suffixes and IPs will differ.

</details>

## Resources: requests and limits

```yaml
resources:
  requests:
    cpu: 250m          # 250 millicores = a quarter of a core
    memory: 256Mi
  limits:
    cpu: "1"
    memory: 512Mi
```
**requests** are the guaranteed minimum. **They are used by the scheduler**: a pod is placed only on a node where the sum of the requests of all pods plus this one doesn't exceed the available resources. The scheduler doesn't look at actual consumption.

**limits** are the ceiling. What happens when it is exceeded differs:
- **memory**: the process is killed by the OOM killer (the Linux kernel mechanism that kills a process when memory runs out; **OOMKilled**, exit code 137), the container restarts. Memory can't be "taken back" from a process.
- **CPU**: the process isn't killed, it's **slowed down (throttling)**: in every 100 ms period it gets no more than its quota (a limit of `1` = 100 ms of CPU time every 100 ms). The application becomes slow, latency grows. A typical picture: the pod's average CPU is 30%, yet responses are slow — in short bursts it uses up its quota and waits for the next period (visible in the `container_cpu_cfs_throttled_periods_total` metric).

### QoS classes
By the combination of requests and limits a pod gets a QoS class (Quality of Service) that decides whom to evict first (eviction — kubelet itself removes pods from a node to save it) when the node runs short of memory:
- **Guaranteed** — requests equal limits for CPU and memory in all containers. Evicted last.
- **Burstable** — requests are set but lower than limits (or limits are not set).
- **BestEffort** — nothing is set. Evicted **first**.

### Practical recommendations
- **Always set requests.** Without them the scheduler treats the pod as "free", nodes get overpacked, and HPA (Horizontal Pod Autoscaler — automatically changes the number of replicas by load) can't compute utilisation in percent, since the percentage is computed from requests.
- **Set a memory limit** (often equal to the request) so that one pod's memory leak doesn't bring down the node.
- **A CPU limit is debatable.** Many don't set it: throttling hurts latency, and with requests in place the node already shares CPU proportionally. It's set when you need predictability or hard separation of tenants (different teams or customers on one cluster).
- Pick the values from real usage metrics (Prometheus, VPA — the Vertical Pod Autoscaler — in recommendation mode: it watches actual usage and suggests requests without changing anything itself).

### 🧪 Mini task

**Task.** Run a pod with `requests.memory: 32Mi` and `limits.memory: 64Mi` whose process tries to take 200 MB. Find the container's termination reason and exit code and the pod's QoS class. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
OOMKilled 137 Burstable
```

Breakdown: a memory limit is a hard cgroup boundary. Exceeding it makes the kernel kill the process (`OOMKilled`, 128 + 9 = 137); with `restartPolicy: Always` the pod would go into CrashLoopBackOff. Requests aren't a limit but a reservation: the scheduler uses them to decide which node gets the pod. QoS `Burstable` means requests are below limits. With `requests == limits` for every resource you get `Guaranteed` (evicted last), with no requests or limits `BestEffort` (evicted first). Exceeding a CPU limit doesn't kill the pod, it throttles it.

</details>

## Probes — health checks

**livenessProbe — "is the process alive?"** If the check fails (several times in a row), kubelet **restarts the container**. Needed when an application can hang without crashing (a deadlock — threads wait for each other forever, the process is alive but processes nothing).

**readinessProbe — "is it ready to take traffic?"** If it fails, the pod is **removed from the service's Endpoints** (the list of addresses the Service sends traffic to) — traffic stops flowing to it, but the pod is **not restarted**. Needed at startup (cache warm-up, data loading) and during temporary overload.

**startupProbe — "has startup finished?"** Until it passes, liveness and readiness are not run. For applications with a long startup (Java), so that liveness doesn't kill them while they are loading.

```yaml
containers:
  - name: api
    startupProbe:
      httpGet: { path: /health/startup, port: 8080 }
      failureThreshold: 30       # 30 attempts × 10 s = up to 5 minutes to start
      periodSeconds: 10
    livenessProbe:
      httpGet: { path: /health/live, port: 8080 }
      periodSeconds: 10
      timeoutSeconds: 3
      failureThreshold: 3        # 3 failures in a row → restart
    readinessProbe:
      httpGet: { path: /health/ready, port: 8080 }
      periodSeconds: 5
      failureThreshold: 2
```
**The main mistake is checking dependencies in liveness.** If the liveness endpoint checks the database, then when the database is unavailable **all** pods fail liveness and start restarting in a loop: the service goes down completely, even though it could have served from cache or returned a clear error. Liveness should check only the process itself ("I'm not hung"). Dependencies can (carefully) be taken into account in readiness.

### 🧪 Mini task

**Task.** Deploy nginx with a readinessProbe on the non-existent path `/ready` and a Service in front of it. Look at the pod status, its readiness and the Service's endpoints. Then fix the path to `/` and check again. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
0/1 Running
endpoints ready: false
1/1 Running
```

Breakdown: the pod is `Running` but not `Ready`: the process is alive, but the readinessProbe gets a 404. Such a pod is excluded from load balancing — the EndpointSlice marks it `ready: false`, and no traffic goes to it. That's what protects users while an app warms up or temporarily can't serve. A rollout with a broken probe wouldn't complete either: the new pod never becomes Ready, and the old pods aren't removed. A failing liveness probe would restart the container — which is why it must not depend on the database or external services, or a database outage restarts every pod at once.

</details>

## Scheduling: where a pod ends up

### nodeSelector — the simplest option
```yaml
nodeSelector:
  node.kubernetes.io/instance-type: m6i.large   # only on nodes with this label
```

### Affinity and anti-affinity
```yaml
affinity:
  nodeAffinity:                                   # requirements for the NODE
    requiredDuringSchedulingIgnoredDuringExecution:   # mandatory
      nodeSelectorTerms:
        - matchExpressions:
            - key: karpenter.sh/capacity-type
              operator: In
              values: ["on-demand"]
  podAntiAffinity:                                # relation to OTHER PODS
    preferredDuringSchedulingIgnoredDuringExecution:  # preferred
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels: { app: api }
          topologyKey: kubernetes.io/hostname     # "don't put two api replicas on one node"
```
- `required...` — a hard rule: if it can't be satisfied, the pod stays Pending (waiting for a suitable node to appear). In the example above — "only on-demand nodes" (ordinary instances at full price, as opposed to spot — cheap ones AWS can take back with a 2-minute warning).
- `preferred...` — a wish: the scheduler will try, but will place the pod anyway.
- `IgnoredDuringExecution` — the rule is checked only at placement; already running pods are not evicted if node labels change.
- `topologyKey` — what counts as "one place": a node (`kubernetes.io/hostname`) or an availability zone (`topology.kubernetes.io/zone`).

Why anti-affinity: if all replicas end up on one node or in one zone, its failure takes down the whole service.

### topologySpreadConstraints — even distribution
More convenient than anti-affinity for even spreading:
```yaml
topologySpreadConstraints:
  - maxSkew: 1                                 # the difference in pod count between zones is at most 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule           # or ScheduleAnyway — a soft rule
    labelSelector:
      matchLabels: { app: api }
```
6 replicas in 3 zones will be spread 2 per zone.

### Taints and tolerations
A **taint** on a node **repels** pods. A **toleration** on a pod **allows** it to ignore the taint.
```bash
kubectl taint nodes gpu-node-1 dedicated=gpu:NoSchedule   # ordinary pods won't land here anymore
```
```yaml
tolerations:                       # this pod may go to nodes with such a taint
  - key: dedicated
    operator: Equal
    value: gpu
    effect: NoSchedule
```
Effects: `NoSchedule` (new pods without a toleration are not placed), `PreferNoSchedule` (the soft version), `NoExecute` (also **evicts** already running pods without a toleration).

**Important: a toleration allows but does not attract.** A pod with a toleration may still end up on an ordinary node. To dedicate nodes **only** to certain pods you need both mechanisms: a taint on the node (others won't come) plus nodeAffinity or nodeSelector on the pods (yours go exactly there).

Kubernetes itself sets taints when there are problems: `node.kubernetes.io/not-ready`, `unreachable`, `memory-pressure`, `disk-pressure`.

### 🧪 Mini task

**Task.** Create a pod with `nodeSelector: disktype=ssd` while no node has that label. Look at the status and the scheduler's event, then label `lab-worker2` and check where the pod lands. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
Pending
0/3 nodes are available: 1 node(s) had untolerated taint {node-role.kubernetes.io/control-plane: }, 2 node(s) didn'
Running on lab-worker2
```

Breakdown: a pod can't be scheduled until a suitable node exists, so it sits in `Pending`. The `FailedScheduling` event explains the reason per node: the control plane is closed by a taint, and the two workers lack the label (the message continues: "didn't match Pod's node affinity/selector"). As soon as the label appeared, the scheduler placed the pod by itself. `nodeSelector` is the simplest mechanism; `nodeAffinity` is more flexible ("preferred" rules, In and NotIn operators), while taints and tolerations work the other way round: the node repels pods without the right toleration. `Pending` is always investigated starting with `kubectl describe pod` and the events.

</details>

## Scaling
- **HPA (Horizontal Pod Autoscaler)** — changes the number of replicas by a metric: CPU, memory (relative to **requests**), or by custom and external metrics (queue length, RPS — requests per second). It needs **metrics-server** (a component that collects the current CPU and memory usage of pods from kubelet; `kubectl top` uses it too) for CPU and memory, and for custom metrics — Prometheus Adapter (exposes metrics from Prometheus to the HPA) or **KEDA** (Kubernetes Event-Driven Autoscaling: it can scale by SQS and Kafka queues and down to zero when there's no work).
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
          target: { type: Utilization, averageUtilization: 70 }   # 70% of requests
  ```
- **VPA (Vertical Pod Autoscaler)** — picks requests and limits. Often used only in recommendation mode. It is not combined with HPA on the same metric.
- **Cluster Autoscaler** — adds nodes when pods hang in Pending for lack of resources, and removes underused ones. It works through auto-scaling groups (ASG, Auto Scaling Group — a group of identical EC2 instances whose count AWS keeps at a set level).
- **Karpenter** (AWS) — the same, but faster and more flexible: it looks at the requirements of pending pods and picks a suitable instance type itself (including spot — cheap instances from spare AWS capacity that can be taken back), and merges underused nodes (consolidation — moving pods and shutting down extra nodes). Details in the next section.

### How fast HPA reacts: behavior
HPA doesn't react instantly, and that is deliberate, so that replicas don't "jump" back and forth as load fluctuates. The `stabilizationWindowSeconds` parameter says over what period HPA takes past recommendations into account. By default:
- **scaling up — 0 seconds**: replicas are added immediately;
- **scaling down — 300 seconds**: HPA waits for 5 minutes of consistently low load before removing replicas.

```yaml
spec:
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
        - type: Percent
          value: 100              # the number of replicas may double within 15 seconds
          periodSeconds: 15
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Pods
          value: 2                # remove no more than 2 pods per minute
          periodSeconds: 60
```
A short command for a quick experiment: `kubectl autoscale deployment api --cpu-percent=50 --min=3 --max=10`. During the first 1–2 minutes the TARGETS column shows `<unknown>/50%` — HPA hasn't received metrics from metrics-server yet.

### How to test autoscaling with load
A scenario from the EKS workshop (the official AWS hands-on course on EKS — eksworkshop.com): create CPU load and watch HPA add pods and Karpenter add nodes for them.
```bash
# 50 parallel clients, 10 requests per second each, for an hour
kubectl run load-generator --image=williamyeh/hey:latest --restart=Never -- \
  -c 50 -q 10 -z 60m http://api.prod.svc:8080/

kubectl get hpa -n prod -w                  # TARGETS goes above 50%, REPLICAS increases
kubectl get pods -n prod                    # some of the new pods are Pending — there aren't enough nodes
kubectl get nodes -l karpenter.sh/nodepool  # Karpenter creates nodes for the pending pods

kubectl delete pod load-generator           # remove the load
# after ~5 minutes (stabilization window) HPA reduces the replicas,
# then Karpenter removes the emptied and underused nodes
```

### VPA in detail: sizing requests and limits
**VPA (Vertical Pod Autoscaler)** looks at the pods' actual CPU and memory consumption over the last days and recommends (or sets itself) the requests. Why: requests set "by eye" are almost always inaccurate. Too high — nodes sit idle while you pay for reserved but unused capacity. Too low — pods land on overloaded nodes and get OOMKilled (the process is killed by the kernel for exceeding its memory limit) or CPU throttling (the kernel slows down a container that exceeded its limit).

VPA is installed separately (it isn't in the cluster by default) and consists of three parts:
- **recommender** — computes recommendations from the metrics history;
- **updater** — recreates pods whose requests differ a lot from the recommendation (respecting the PodDisruptionBudget);
- **admission controller** — sets the recommended requests when a pod is created.

Modes (`updateMode`):
| Mode | What it does | When |
|---|---|---|
| `Off` | only computes recommendations, doesn't touch pods | at the start and in prod: you look at the numbers and carry them into the manifests by hand |
| `Initial` | sets requests only for new pods | when restarts for the sake of resizing are unacceptable |
| `Recreate` (and the deprecated `Auto`, which currently works the same way) | recreates pods with new requests | batch jobs, non-critical services with several replicas |
| `InPlaceOrRecreate` (in newer VPA versions) | changes a running pod's requests without a restart if the cluster supports in-place resize (Kubernetes 1.33+), otherwise recreates it | when the cluster and VPA versions allow it |

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
    updateMode: "Off"                 # recommendations only
  resourcePolicy:
    containerPolicies:
      - containerName: api
        controlledResources: ["memory"]   # CPU is left to HPA, VPA watches only memory
        minAllowed: { memory: 256Mi }
        maxAllowed: { memory: 4Gi }
```
```bash
kubectl describe vpa api -n prod
# → Recommendation:
# →   Container Recommendations:
# →     Container Name: api
# →     Lower Bound:  memory: 410Mi
# →     Target:       memory: 620Mi     ← what to put into requests
# →     Upper Bound:  memory: 1.4Gi
```

**HPA and VPA together.** You can't give both the same metric: HPA on CPU adds replicas when the load per pod grows, while VPA responds to the same load by increasing the pod's CPU — two controllers pull the system in different directions. Working setups: HPA on CPU or RPS + VPA on memory only; or HPA on your own metric (a queue, RPS) + VPA on CPU and memory; or VPA in `Off` mode as a source of numbers for manual rightsizing. A ready-made dashboard of recommendations across all namespaces is Goldilocks (it puts a VPA in `Off` mode on every Deployment and shows the numbers in a web UI).

### KEDA: scaling on events and down to zero
**KEDA (Kubernetes Event-Driven Autoscaling)** is an operator that scales on external sources: SQS queue length, Kafka consumer lag (how many messages haven't been read yet), Prometheus queries, a cron schedule. It doesn't scale pods itself: it creates and updates an ordinary HPA, and additionally can keep **0 replicas** when there's no work (HPA itself goes down only to 1).

When KEDA rather than HPA on CPU: queue workers. A worker's CPU may be low while there are 100 thousand messages in the queue, and HPA on CPU will do nothing.
```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: orders-worker
  namespace: prod
spec:
  scaleTargetRef:
    name: orders-worker               # Deployment
  minReplicaCount: 0                  # no messages — no pods
  maxReplicaCount: 50
  pollingInterval: 15                 # how often to ask for the queue length, seconds
  cooldownPeriod: 120                 # how long to wait after the queue empties before going to 0
  triggers:
    - type: aws-sqs-queue
      authenticationRef:
        name: keda-aws
      metadata:
        queueURL: https://sqs.eu-central-1.amazonaws.com/111122223333/orders
        queueLength: "20"             # target: 20 messages per replica
        awsRegion: eu-central-1
---
apiVersion: keda.sh/v1alpha1
kind: TriggerAuthentication
metadata:
  name: keda-aws
  namespace: prod
spec:
  podIdentity:
    provider: aws                     # permissions via KEDA's own IRSA or EKS Pod Identity, no keys in secrets
```
600 messages in the queue → KEDA sets 600 / 20 = 30 replicas. The queue is empty for 2 minutes → 0 replicas.

Predictable peaks (the working day, a mailing at 10:00) are conveniently covered by a `cron` trigger — raise the minimum number of replicas in advance:
```yaml
  triggers:
    - type: cron
      metadata:
        timezone: Europe/Kyiv
        start: "0 8 * * 1-5"          # weekdays from 8:00
        end: "0 20 * * 1-5"           # until 20:00
        desiredReplicas: "10"
```

### A 10x load spike: what slows scaling down and how to speed it up
A frequent interview question: "traffic grew 10x in a minute — what happens and how do you prepare?". It helps to break down what the reaction time consists of:

| Step | Usually | What speeds it up |
|---|---|---|
| metrics-server collected the metrics | up to 15 s | a leading metric: RPS or queue length instead of CPU |
| HPA recalculated the replicas | up to 15 s (the controller loop) | `behavior.scaleUp` without delay, a large `Percent` |
| the scheduler found room | seconds, if there are free nodes | a capacity reserve (see below) |
| Karpenter or Cluster Autoscaler added a node | 40–90 s (launching EC2 + registering in the cluster) | overprovisioning, warm pools, smaller and diverse instance types |
| pulling the image | from seconds to minutes | a small image, an image cache on the nodes, ECR in the same region |
| application start and readiness | from seconds to minutes (JVM, cache warm-up) | `startupProbe`, fast start, lazy initialisation |

In total a "cold" pod addition on a new node takes 1–3 minutes. If the peak arrives faster, without spare capacity it will hit users.

**Overprovisioning — space occupied in advance.** The cluster keeps low-priority "placeholder" pods. They occupy nodes but do nothing. When a real pod arrives, the scheduler **preempts** a placeholder and immediately puts the pod in the freed space, while the preempted placeholder goes Pending and makes Karpenter order a new node in the background:
```yaml
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: overprovisioning
value: -10                            # lower than any workload pods (0 by default)
preemptionPolicy: Never               # placeholders don't preempt anyone themselves
globalDefault: false
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: capacity-reserve
  namespace: kube-system
spec:
  replicas: 3                         # reserve size: 3 × (1 CPU, 2 GiB)
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
          image: registry.k8s.io/pause:3.10   # a container that just sleeps
          resources:
            requests: { cpu: "1", memory: 2Gi }
```

What else to check besides the pods themselves:
- **dependencies withstand the growth**: 10× pods = 10× connections to the DB (a connection pool is needed: PgBouncer, RDS Proxy), external API limits, AWS quotas on the number of instances (service quotas) and free IPs in the subnets (VPC CNI takes an IP for each pod);
- **limits of HPA itself**: `maxReplicas` shouldn't be a ceiling you hit on the very first peak;
- **graceful degradation**: rate limiting on the ingress, switching off heavy features by a flag, serving from cache — better that some requests get a 429 than everything goes down;
- **a load test** in advance, measuring the time to the required number of replicas (see "How to test autoscaling with load").

### HPA, KEDA, VPA, Karpenter: who does what and when to use which
The key thing to understand: these tools work at **different levels** and do not compete — they complement each other. The confusion comes from all of them being called "autoscalers".

| Tool | What it scales | On what signal | Can scale to zero | When to use |
|---|---|---|---|---|
| **HPA** (Horizontal Pod Autoscaler) | the replica count of a Deployment or StatefulSet | CPU and memory relative to requests; custom and external metrics through an adapter (Prometheus Adapter or KEDA) | no, minimum 1 replica | stateless web services and APIs where load growth shows up in CPU |
| **KEDA** (Kubernetes Event-Driven Autoscaling) | the replica count — via an HPA it creates itself | events and external metrics: SQS or RabbitMQ queue length, Kafka consumer lag (how many messages are still unread), a Prometheus query, a cron schedule — more than 70 sources (scalers) | yes | queue workers, batch processing, predictable scheduled peaks, services that are idle most of the time |
| **VPA** (Vertical Pod Autoscaler) | the requests and limits of a pod's containers | actual usage over recent days | — | right-sizing pods; usually only in recommendation mode (`updateMode: "Off"`) |
| **Cluster Autoscaler** | the number of nodes in pre-created groups (ASG — Auto Scaling Group in AWS, Managed Instance Group in GCP) | Pending pods; underutilized nodes | — | GKE, AKS, on-prem via Cluster API — wherever Karpenter is not available |
| **Karpenter** | nodes: how many and **of what type** to create | Pending pods (their requests, affinity, zones, tolerations); underutilized nodes — consolidation (repacking: moving pods and deleting extra nodes) | — | EKS (and AKS through the Azure provider): mixed workloads, Spot instances, cost savings |

How they work together — a typical chain in EKS:
```
queue grows → KEDA polls SQS → its HPA: replicas 2 → 30 → 28 pods Pending (no room)
   → Karpenter sees the Pending pods, picks and launches 3 Spot instances → pods start
queue drains → KEDA: 0 replicas → nodes empty out → Karpenter consolidation deletes the nodes
```
KEDA and HPA answer "**how many pods** are needed", Karpenter answers "**on which nodes** to run them". Neither is complete without the other: an HPA without a node autoscaler hits Pending as soon as the cluster runs out of free room; Karpenter without an HPA keeps nodes for a fixed replica count. And both calculate from **requests**: HPA the utilization percentage, Karpenter the node size. Without correct requests, both get it wrong.

**KEDA or HPA:**
- A plain CPU-based HPA is enough when load shows up in the pod's CPU — for example, a web API behind a load balancer.
- KEDA — when the signal is **outside the pod** (a queue, lag, a Prometheus metric, time of day), when you need **scale-to-zero** (no work — no pods and no cost for them), or when you do not want to install a separate Prometheus Adapter: KEDA itself acts as the external metrics server for the HPA.
- KEDA **does not replace** HPA — it creates one itself (`keda-hpa-<ScaledObject name>`). So you cannot attach both your own HPA and a ScaledObject to the same Deployment: two HPAs will fight over the replica count.
- One ScaledObject can combine several triggers (for example, CPU and queue length): the HPA takes the maximum of their recommendations. KEDA has CPU and memory triggers too, but you cannot scale to zero on them: with 0 pods there is nothing to measure.
- **ScaledJob** is KEDA's option for long-running tasks: instead of scaling a Deployment it creates a separate Job per batch of messages. That way scaling down does not kill a worker in the middle of processing a long video or report.

**Karpenter or Cluster Autoscaler:**
- **EKS** — Karpenter; it is the standard now. **EKS Auto Mode** is essentially Karpenter managed by AWS.
- **GKE** — the built-in cluster autoscaler plus node auto-provisioning (creates node pools of a suitable type itself — the same idea as Karpenter); in Autopilot mode Google manages the nodes entirely.
- **On-prem and bare metal** — you cannot buy a new node on the fly: Cluster Autoscaler works only on top of Cluster API (a Kubernetes project that creates and deletes machines, such as VMs in vSphere or OpenStack); otherwise capacity is planned in advance.

**VPA and HPA together:** do not combine them **on the same metric**. VPA raises the CPU requests → utilization relative to requests drops → HPA removes replicas → load on the remaining ones grows → a loop. Safe: HPA on CPU or RPS, and VPA only for memory or only in recommendation mode. VPA used to apply new requests only by recreating the pod; newer Kubernetes versions can change a running pod's resources without a restart (in-place resize), and VPA can use it (the `InPlaceOrRecreate` mode).

A typical production setup on EKS: **HPA** for APIs, **KEDA** for queue workers and scheduled jobs, **VPA in recommendation mode** to pick requests, **Karpenter** for nodes — with headroom via overprovisioning (low-priority placeholder pods that hold free room and get evicted during a spike) if reaction speed matters.

### 🧪 Mini task

**Task.** Create a Deployment with CPU requests and an HPA at 50% CPU from 1 to 5 replicas. Look at the HPA in a cluster without metrics-server and explain what `<unknown>` means. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
kubectl create deployment hpa-demo --image=nginx:1.27 > /dev/null
kubectl set resources deployment hpa-demo --requests=cpu=100m > /dev/null
kubectl autoscale deployment hpa-demo --cpu-percent=50 --min=1 --max=5 > /dev/null
sleep 30
kubectl get hpa hpa-demo --no-headers | awk '{print $1, $3, $4, $5, $6}'
kubectl get hpa hpa-demo -o jsonpath='{.status.conditions[?(@.type=="ScalingActive")].reason}{"\n"}'
kubectl delete hpa,deployment hpa-demo > /dev/null
```

Expected output:

```text
hpa-demo cpu: <unknown>/50% 1 5 1
FailedGetResourceMetric
```

Breakdown: an HPA doesn't measure load itself; it reads the Metrics API, which metrics-server provides, and kind doesn't ship it by default. So the current value is `<unknown>` and the HPA doesn't scale. A common interview question. The other common cause of `<unknown>` is missing `requests.cpu`: the percentage is relative to requests, so without them the HPA can't compute it. On EKS metrics-server is installed as an add-on. Scaling on queues or custom metrics uses KEDA or the Prometheus Adapter, and adding nodes is the job of Cluster Autoscaler or Karpenter.

</details>

## Karpenter — node autoscaling in EKS

### How Karpenter differs from Cluster Autoscaler
**Cluster Autoscaler** works through **node groups** (in AWS — Auto Scaling Groups). Within one group all nodes must be the same size, so:
- to use different instance types (regular, high-memory, GPU, spot) you have to create and maintain many groups in advance;
- switching from spot to on-demand when spot instances are unavailable is slow and complicated;
- the autoscaler can only add another node of a **predefined** size.

**Karpenter** works **without groups** (group-less): it looks at the specific pending pods (their requests, nodeSelector, affinity, tolerations, zones), **picks the instance type itself** that fits them most cheaply, and creates it directly via the EC2 API. If 5 GB of memory is needed, it takes an 8 GB instance instead of adding another standard 32 GB node.

### NodePool and EC2NodeClass
The configuration consists of two objects:
- **NodePool** — **which** nodes may be created: constraints (requirements) on instance types, spot or on-demand, zones, architecture; an overall resource limit; consolidation rules.
- **EC2NodeClass** — **how** to create them in AWS: the AMI (the disk image the node boots from), the node's IAM role (the permissions the node uses with AWS: pulling images from ECR, attaching disks), subnets and Security Groups (found by tags), tags for the instances.

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  name: default
spec:
  template:
    metadata:
      labels:
        intent: apps                         # a label on the created nodes — pods select them via nodeSelector
    spec:
      requirements:
        - key: karpenter.sh/capacity-type
          operator: In
          values: ["spot", "on-demand"]      # try spot first, fall back to on-demand if it's scarce
        - key: karpenter.k8s.aws/instance-category
          operator: NotIn
          values: ["t"]                      # exclude burstable t instances
        - key: karpenter.k8s.aws/instance-size
          operator: NotIn
          values: ["nano", "micro", "small", "medium"]
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64", "arm64"]         # allow Graviton (AWS ARM processors) — 20–40% cheaper
      nodeClassRef:
        group: karpenter.k8s.aws
        kind: EC2NodeClass
        name: default
      expireAfter: 720h                      # recreate a node every 30 days — fresh AMIs and patches
  limits:
    cpu: 1000                                # no more than 1000 vCPU and 1000 GiB in total across the pool's nodes
    memory: 1000Gi
  disruption:
    consolidationPolicy: WhenEmptyOrUnderutilized
    consolidateAfter: 1m
    budgets:
      - nodes: "10%"                         # change no more than 10% of nodes at a time
      - schedule: "0 9 * * mon-fri"          # during working hours on weekdays — don't touch nodes at all
        duration: 8h
        nodes: "0"
---
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
metadata:
  name: default
spec:
  role: KarpenterNodeRole-my-cluster          # the IAM role the nodes will get
  amiSelectorTerms:
    - alias: al2023@latest                    # in production better pin a version instead of latest
  subnetSelectorTerms:
    - tags:
        karpenter.sh/discovery: my-cluster    # which subnets to create nodes in
  securityGroupSelectorTerms:
    - tags:
        karpenter.sh/discovery: my-cluster
  tags:
    team: platform                            # tags on EC2 — for cost allocation
```
Why it's better to exclude types rather than list the allowed ones: the more instance types Karpenter can use, the higher the chance of finding free spot capacity and the lower the price. For spot Karpenter uses the `price-capacity-optimized` strategy: it picks the pools with the lowest interruption risk among the cheap ones.

### Consolidation — packing nodes
Scaling up is only half the savings. When the load drops, pods stay spread across many half-empty nodes. **Consolidation** — Karpenter constantly checks whether it can:
- **remove an empty node** (`reason: empty`);
- **move pods** from an underused node to free space on other nodes and remove it (`reason: underutilized`);
- **replace a node with a cheaper one** (for example, two m5.xlarge with one m5.2xlarge, or on-demand with spot).

The move happens the standard way: cordon (mark the node "don't put new pods here"), drain (evict its pods one by one so they're recreated on other nodes) respecting PDBs (PodDisruptionBudget — "how many replicas of an application may be lost at once", see the reliability section), deleting the node.

An example from the workshop: 12 pods of 1.5 GiB memory each occupied 3 m5.large nodes (~6.9 GiB allocatable each — the memory left for pods after reserving some for the system and kubelet). After scaling down to 6 pods (about 9 GiB), Karpenter, after `consolidateAfter`, removed one node as underutilized, and the pods moved to the remaining two.

Settings:
- `consolidationPolicy: WhenEmpty` — only empty nodes; `WhenEmptyOrUnderutilized` — underused ones too;
- `consolidateAfter` — how long to wait after the last pod change on a node (protection against unnecessary "churn");
- `budgets` — how many nodes may be changed at once and when (on a schedule); 10% by default;
- `expireAfter` — the maximum node lifetime: regular replacement pulls in fresh AMIs.

### How to stop Karpenter from touching a specific pod or node
- the `karpenter.sh/do-not-disrupt: "true"` annotation **on a pod** — a node with such a pod won't be voluntarily freed (long batch jobs — hours-long computations that mustn't be interrupted, stateful pods without replication);
- the same annotation **on a node** — this node won't be consolidated;
- **PDBs** — Karpenter respects them during drain;
- budgets in the NodePool.

### Debugging Karpenter
```bash
# which nodes Karpenter created and why of this type
kubectl -n karpenter logs deploy/karpenter -c controller --all-pods | grep 'launched nodeclaim' | tail -1 | cut -d] -f2- | jq
# which nodes it removed and for what reason (empty / underutilized / drifted / expired)
kubectl -n karpenter logs deploy/karpenter -c controller --all-pods | grep 'disrupting nodeclaim' | tail -1 | cut -d] -f2- | jq

kubectl get nodeclaims                                  # a NodeClaim is a Karpenter object, a "request for one node": which instance was ordered and its state
kubectl get nodes -L karpenter.sh/capacity-type,node.kubernetes.io/instance-type,topology.kubernetes.io/zone
kubectl get node <node> -o json | jq '.metadata.labels' # Karpenter sets many useful labels: family, CPU, memory, hypervisor

# eks-node-viewer — a visual view: node utilisation by requests and cost per hour
eks-node-viewer --resources cpu,memory --extra-labels topology.kubernetes.io/zone
eks-node-viewer --node-selector karpenter.sh/nodepool --node-sort=eks-node-viewer/node-cpu-usage=asc
```
If nodes aren't removed after the load is gone, the reasons are usually: a pod with `do-not-disrupt` on the node, a PDB doesn't allow eviction, the budget is 0 in the current window, the node has pods without a controller (there is nowhere to recreate them), or DaemonSet pods keep the node "non-empty" (Karpenter doesn't count them, that's normal).

**Rightsizing pods** is the second half of the savings: if requests are inflated, Karpenter faithfully creates nodes for "air". To pick requests, use **Goldilocks** (open source from Fairwinds): it runs VPA in recommendation mode for every Deployment and shows recommended requests and limits in a dashboard.

### 🧪 Mini task

**Task.** In an EKS cluster with Karpenter look at the configured NodePools and the nodes Karpenter has already created: instance type, capacity type (spot or on-demand) and zone.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
kubectl get nodepools -o custom-columns=NAME:.metadata.name,NODECLASS:.spec.template.spec.nodeClassRef.name,LIMIT_CPU:.spec.limits.cpu
kubectl get nodeclaims -o custom-columns=NAME:.metadata.name,TYPE:.metadata.labels.node\\.kubernetes\\.io/instance-type,CAPACITY:.metadata.labels.karpenter\\.sh/capacity-type,ZONE:.metadata.labels.topology\\.kubernetes\\.io/zone
```

Expected output:

```text
NAME      NODECLASS   LIMIT_CPU
default   default     1000
NAME            TYPE         CAPACITY   ZONE
default-7xk2c   m7g.large    spot       us-east-2b
default-q9w4h   c7g.xlarge   on-demand  us-east-2a
```

Breakdown: a NodePool describes *which* nodes Karpenter may create (architectures, families, spot or on-demand, a total CPU limit), and an EC2NodeClass describes *how* (AMI, subnets, security groups, role). For each set of Pending pods Karpenter picks an instance that fits on price and size and creates a NodeClaim → node, with no predefined node groups. Consolidation later removes or replaces under-used nodes. The CPU limit protects against an endless bill if manifests go wrong. You need EKS with Karpenter installed; kind doesn't have it.

</details>

## Stopping a pod and PodDisruptionBudget

### What happens when a pod is deleted
1. The pod is marked Terminating. Two processes start **in parallel**:
   - the controller removes the pod from the EndpointSlice, and kube-proxy and load balancers update their rules (this takes seconds);
   - kubelet starts stopping the containers.
2. kubelet runs the **preStop hook** if one is set (a command or HTTP request kubelet executes in the container before stopping it: for example, a pause or "close connections").
3. The containers are sent **SIGTERM**.
4. Waiting up to `terminationGracePeriodSeconds` (30 seconds by default).
5. Whoever hasn't exited gets **SIGKILL**.

Because the steps in 1 run in parallel, the application may receive SIGTERM and shut down **before** the load balancer stops sending traffic to it. The result is 502 errors on every deployment. The fix is a small delay in preStop so the pod has time to drop out of load balancing:
```yaml
lifecycle:
  preStop:
    exec:
      command: ["sleep", "10"]
terminationGracePeriodSeconds: 45
```

### PodDisruptionBudget
A **PDB** limits how many of an application's pods may be unavailable at the same time during **voluntary** disruptions: `kubectl drain`, node upgrades, Karpenter consolidation. A drain will wait instead of evicting all replicas at once.
```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata: { name: api }
spec:
  minAvailable: 2            # or maxUnavailable: 1
  selector:
    matchLabels: { app: api }
```
The trap: a PDB with `minAvailable` equal to the number of replicas (or `maxUnavailable: 0`) **blocks drain forever**, and node upgrades hang.

### 🧪 Mini task

**Task.** Create a 2-replica Deployment and a PDB with `minAvailable: 2`, then try to `drain` a node running one of the replicas. What blocks the node maintenance? You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
kubectl create deployment pdb-demo --image=nginx:1.27 --replicas=2 > /dev/null
kubectl create poddisruptionbudget pdb-demo --selector=app=pdb-demo --min-available=2 > /dev/null
kubectl rollout status deployment/pdb-demo > /dev/null
node=$(kubectl get pods -l app=pdb-demo -o jsonpath='{.items[0].spec.nodeName}')
kubectl drain $node --ignore-daemonsets --timeout=15s 2>&1 | grep -m1 -o 'Cannot evict pod as it would violate the pod.s disruption budget.'
kubectl uncordon $node > /dev/null
kubectl delete pdb,deployment pdb-demo > /dev/null
```

Expected output:

```text
Cannot evict pod as it would violate the pod's disruption budget.
```

Walkthrough: `drain` evicts pods through the Eviction API (a special "evict this pod" request which, unlike a plain delete, checks PDBs), and it respects the PDB. The budget requires 2 of 2 pods to be available at all times, so not a single one can be evicted, and drain keeps retrying until the timeout. Usually `maxUnavailable: 1` or a `minAvailable` lower than the replica count is used, otherwise nodes can't be updated. A PDB protects only against voluntary disruptions (drain, node updates, Karpenter consolidation), not against a node crash. The pod itself gets `SIGTERM` on eviction and `terminationGracePeriodSeconds` to shut down cleanly.

</details>

## How to deploy without downtime (zero-downtime)
A frequent question. You need to put all the pieces together:
1. `RollingUpdate` with `maxUnavailable: 0` — capacity doesn't drop;
2. a correct **readinessProbe** — traffic goes only to ready pods;
3. **preStop with a delay** — the pod has time to drop out of load balancing;
4. **the application handles SIGTERM correctly**: stops accepting new requests and finishes the current ones;
5. `terminationGracePeriodSeconds` longer than the time needed to finish requests;
6. a **PDB** and several replicas in different zones — for node upgrades;
7. **backward-compatible DB migrations** — the old and new versions run at the same time against one schema. Example: you can't rename a column in one release — first you add the new column and write to both, then switch reads, and only in the next release drop the old one (the expand/contract pattern).

### 🧪 Mini task

**Task.** Update a Deployment to a non-existent image tag with the `maxUnavailable: 0` strategy. See how many pods keep working while the new version can't start, and roll the rollout back. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
error: timed out waiting for the condition
available=3 updated=1
deployment "zdt" successfully rolled out
```

Breakdown: with `maxUnavailable: 0` Kubernetes doesn't remove old pods until new ones are Ready. The new pod got stuck in `ImagePullBackOff`, so all 3 old ones keep serving traffic, and users noticed nothing. `maxSurge: 1` allows one extra pod during the rollout. `rollout undo` restored the previous ReplicaSet. True zero-downtime also needs a readinessProbe (otherwise "Ready" means "process started"), proper `SIGTERM` handling and a `preStop` pause so the pod drops out of load balancing before it stops.

</details>

## The network model and NetworkPolicy

Kubernetes' network requirements:
- every pod has its own IP;
- **any pod can reach any other pod** by its IP without NAT, no matter which node it's on;
- the CNI plugin implements this.

### How a CNI plugin sets up a pod's network
1. kubelet gets the job of starting a pod and, via the CRI, asks containerd to create the **pod "sandbox"** (a service container, pause, that holds the namespaces shared by all the pod's containers) with a new network namespace (an isolated copy of the network stack: its own interfaces, IPs, routes).
2. The runtime calls the **CNI plugin binary** (it lives in `/opt/cni/bin/`, its configuration in `/etc/cni/net.d/`) with the `ADD` command.
3. The plugin:
   - creates a **veth pair** (a virtual "cable" of two interfaces: what goes into one end comes out of the other): one end becomes `eth0` inside the pod, the other stays on the node;
   - allocates an IP from the pod range (IPAM, IP Address Management — keeping track of which addresses are already handed out);
   - adds routes and, if needed, firewall rules;
   - returns the result (the pod IP) to the runtime.
4. When the pod is deleted — the `DEL` command, everything is cleaned up.

CNI itself is just a specification of "how to call the plugin and what it must return". Everything else (how packets get to another node) is up to the specific plugin.

### Comparing CNI plugins
| Plugin | How it carries traffic between nodes | NetworkPolicy | Features |
|---|---|---|---|
| **Flannel** | Overlay (the pod network runs "on top of" the node network: a pod's packet is wrapped whole into another packet between node IPs): pod packets are wrapped into a VXLAN tunnel between nodes | **No** | The simplest, light on resources. For training and small clusters |
| **Calico** | No overlay — plain L3 routing, nodes exchange pod routes over **BGP**; or IP-in-IP tunnels (an IP packet inside another IP packet) or VXLAN if the network doesn't allow it | Yes, plus its own extended policies (global, ordered, deny) | The most widespread, high performance, encryption via WireGuard (a fast modern VPN protocol in the Linux kernel) |
| **Cilium** | Routing or tunnels, the logic is implemented in **eBPF** — programs inside the Linux kernel | Yes, including **L7 policies** (by HTTP path, method, DNS name) | Can replace kube-proxy (services without iptables), transparent encryption, network observability via **Hubble**, service mesh features without sidecars. Needs a recent kernel |
| **AWS VPC CNI** | No overlay: every pod gets a **real IP from the VPC subnet** | Yes, with the network policy agent (eBPF) enabled | Pods are directly visible in the VPC, but IP addresses run out quickly (see the AWS topic) |

**Why eBPF is faster than iptables.** In iptables mode every Service is a set of rules that are checked sequentially. With thousands of services there are tens of thousands of rules, and updating and checking them becomes slow. eBPF uses hash tables right in the kernel: lookup time doesn't grow with the number of services.

Hubble shows the real network traffic between pods — handy when debugging NetworkPolicy:
```bash
hubble observe --from-pod prod/frontend --to-namespace prod --verdict DROPPED   # which connections were dropped by policies
```

Consequence: **by default all pods can see each other**, including pods from other namespaces. To restrict this, **NetworkPolicy** is used — a firewall at the pod level. It works only if the CNI supports it (Calico, Cilium; in EKS — VPC CNI with the network policy agent enabled). Flannel, for example, doesn't support it: the policies will be created but won't block anything.

The typical approach is to first deny all ingress into a namespace and then allow specific traffic:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-ingress
  namespace: prod
spec:
  podSelector: {}            # an empty selector — all pods of the namespace
  policyTypes: [Ingress]     # all ingress traffic to them is denied except what is explicitly allowed
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-api-to-db
  namespace: prod
spec:
  podSelector:
    matchLabels: { app: postgres }      # the policy for the database pods
  ingress:
    - from:
        - podSelector:
            matchLabels: { app: api }   # allow ingress only from api pods
      ports:
        - port: 5432
```
If you also deny egress traffic, don't forget to allow DNS (port 53 to CoreDNS), otherwise pods will stop resolving names.

### 🧪 Mini task

**Task.** Start an `api` service and close all ingress to it with a default-deny policy, then allow access only from pods labelled `role=frontend`. Check access from a pod with and without that label. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
frontend: allowed
other: blocked
```

Breakdown: by default any pod in Kubernetes can reach any other — the network is flat. Once at least one policy with `Ingress` applies to a pod (`app: api`), everything it doesn't explicitly allow is denied, so here one policy is both the default deny and the frontend exception. Policies work on labels, not IPs, because pod IPs keep changing. The CNI enforces them: kindnet in recent kind, Calico or Cilium. On EKS the VPC CNI enforces NetworkPolicy only when the option is enabled; otherwise policies are silently ignored — a common trap.

</details>

## How an external request reaches a pod

The common question "what happens when a user opens a site running on EKS" tests whether you understand **what the traffic actually flows through** and what only **configures** its route. It is easy to get confused here because many objects with similar names are involved.

### Who is who
| What | What it is | Does traffic flow through it |
|---|---|---|
| **Ingress** (object) | A set of rules in YAML: "send requests for `shop.example.com/api` to the `api` service" | No — it is just a description |
| **Ingress controller** | A program in the cluster that reads Ingress objects and **configures a load balancer** according to those rules | Depends on the controller: AWS Load Balancer Controller — **no**; ingress-nginx — **yes**, it is the proxy itself |
| **ALB / NLB** | An AWS load balancer, runs **outside the cluster**, in the VPC | Yes |
| **Service** (object) | A description of a group of pods by labels (selector) and a port | The object itself — no. Via ClusterIP or NodePort — yes, but only when those are the addresses being called |
| **EndpointSlice** | The current list of IPs of the service's **ready** pods; maintained automatically | No — it is a list used to look up addresses |
| **kube-proxy** | An agent on every node that writes iptables or IPVS rules for ClusterIP and NodePort | Yes, but **only** for traffic to a ClusterIP or NodePort |
| **CNI** (VPC CNI on EKS) | Gives pods IPs and provides connectivity between them | Delivers packets. On EKS, thanks to it a pod has a **real IP from the VPC subnet**, so a pod can be reached directly from the VPC |

The key distinction: the **control plane** (the controller, the Ingress and Service objects) decides how things should be set up and writes the configuration. The **data plane** (the load balancer, the proxy, kube-proxy rules) actually passes the packets. Interviewers value exactly this ability to tell them apart.

A step common to all scenarios: the controller looks at the Ingress, finds the Service from it, the EndpointSlice from the Service, and takes the IPs of ready pods from the EndpointSlice. The scenarios then differ in **where** the controller writes those addresses.

### Scenario 1. AWS Load Balancer Controller, IP mode — typical for EKS
```
User
  → DNS: shop.example.com → ALB (an Alias record in Route 53 — a special AWS record that points straight at an AWS resource and tracks its IPs itself)
  → ALB (in the VPC's public subnets): TLS, rules by domain and path
  → a target group in which pod IPs are registered
  → pod 10.0.12.34:8080 (on any node, directly over the VPC network)

Off to the side, outside the traffic path:
AWS Load Balancer Controller ── watches Ingress, Service, EndpointSlice
                             └─ via the AWS API creates the ALB, rules and target groups,
                                registers and deregisters pod IPs
```
Step by step:
1. You apply the Ingress and Service manifests.
2. **AWS Load Balancer Controller** (an ordinary Deployment in the cluster) sees a new Ingress with `ingressClassName: alb` and, through the AWS API, creates an ALB, listeners on 80 and 443 (a listener is the load balancer's port and protocol on which it accepts connections), routing rules and a target group.
3. The controller watches the service's EndpointSlice and **registers the pod IP addresses** with the container port **in the target group**. When a pod is recreated and gets a new IP, the controller deregisters the old address and registers the new one.
4. The user's request arrives at the ALB. The ALB uses its rules (domain, path) to pick a target group and sends the request **straight to the pod IP**. This is possible because VPC CNI gave the pod a real address from the VPC subnet — to the ALB the pod looks like an ordinary server on the network.
5. The controller is **not in** this path. Even if its pod dies, traffic keeps flowing: the ALB is already configured. Only new changes stop being applied.
6. kube-proxy, NodePort and ClusterIP are not involved either. The Service is needed here only to select pods by labels.

Manifests:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: shop-api
  namespace: shop
spec:
  type: ClusterIP                 # in IP mode ClusterIP is enough — no NodePort needed
  selector:
    app: shop-api
  ports:
    - port: 80
      targetPort: 8080            # the container port: this is what gets registered in the target group
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: shop
  namespace: shop
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing        # ALB in public subnets (internal — an internal one)
    alb.ingress.kubernetes.io/target-type: ip                # register pod IPs
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}]'
    alb.ingress.kubernetes.io/ssl-redirect: "443"            # HTTP → HTTPS
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:eu-central-1:123456789012:certificate/abcd-1234
    alb.ingress.kubernetes.io/healthcheck-path: /health      # ALB health check to every pod
    alb.ingress.kubernetes.io/group.name: shop               # several Ingresses can share one ALB
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
How to make sure the ALB really sends traffic to pod IPs:
```bash
kubectl get pods -n shop -l app=shop-api -o wide          # pod IPs, e.g. 10.0.12.34 and 10.0.45.6
kubectl get endpointslices -n shop -l kubernetes.io/service-name=shop-api   # the same IPs in the EndpointSlice
kubectl get targetgroupbindings -n shop                   # Service ↔ target group link, created by the controller
aws elbv2 describe-target-health --target-group-arn <arn from the TargetGroupBinding> \
  --query 'TargetHealthDescriptions[].[Target.Id,Target.Port,TargetHealth.State]' --output table
# Target.Id will show the same pod IPs and port 8080, not EC2 instance IDs
```
Pitfalls of this mode:
- **Security Groups:** the ALB connects to pods directly, so the nodes' Security Group (or the pods', if Security Groups for Pods is used — an EKS feature that lets you attach a separate Security Group to a specific pod rather than the whole node) must allow inbound traffic from the ALB's Security Group on the container port. The controller usually adds this rule itself.
- **502 errors during a deploy.** A new pod can become Ready in Kubernetes **before** the ALB registers it and passes the health check, while the old pod is already being removed. For a short time the target group has no healthy targets. The fix is a **readiness gate**: the pod is not considered Ready until it is healthy in the target group. It is enabled with a label on the namespace:
  ```bash
  kubectl label namespace shop elbv2.k8s.aws/pod-readiness-gate-inject=enabled
  ```
- **Pod deletion.** The ALB keeps sending requests to the pod until its deregistration from the target group completes (the deregistration delay, 300 seconds by default, often reduced to 30–60). So the pod needs a `preStop` with a pause, so that it does not shut down before the ALB stops sending traffic to it (see "Stopping a pod").

### Scenario 2. AWS Load Balancer Controller, instance mode
```
User → ALB → target group with EC2 instances (all nodes) on the NodePort, e.g. 31234
  → node A: kube-proxy catches the packet on the NodePort and rewrites the address to a pod IP (DNAT)
  → the pod may be on node B — then the packet also travels to another node
```
Step by step:
1. The Service must be of type **NodePort** (or LoadBalancer): then Kubernetes opens the same port (30000–32767) on **every** node.
2. The controller registers **nodes**, not pods, in the target group: "instance i-0abc, port 31234".
3. The ALB sends the request to any node on that port.
4. On the node, **kube-proxy** has already written iptables rules: a packet to the NodePort is rewritten (DNAT) to the IP of one of the service's ready pods — chosen at random, possibly on another node.
5. If the pod is on another node, the packet goes there — an **extra network hop**: more latency and cross-zone traffic (which is charged if the nodes are in different AZs).

```yaml
apiVersion: v1
kind: Service
metadata:
  name: shop-api
  namespace: shop
spec:
  type: NodePort                       # instance mode needs a NodePort
  externalTrafficPolicy: Cluster       # Local — only to pods on the same node, keeps the client IP
  selector:
    app: shop-api
  ports:
    - port: 80
      targetPort: 8080
      nodePort: 31234                  # optional — Kubernetes picks one itself
```
In the Ingress a single annotation changes: `alb.ingress.kubernetes.io/target-type: instance`.

**`externalTrafficPolicy`:**
- `Cluster` (default) — the packet may go to a pod on another node. The node then also rewrites the source address (SNAT), so the application sees the node's IP, not the client's;
- `Local` — the node sends traffic only to pods **on itself**. No extra hop and the client IP is preserved, but nodes without pods of this service fail the health check and get no traffic.

How to see what kube-proxy does (on a node, iptables mode):
```bash
iptables -t nat -L KUBE-NODEPORTS -n | grep 31234             # the NodePort rule
iptables -t nat -L KUBE-SERVICES -n | grep shop-api           # the rule for the service ClusterIP
iptables -t nat -L KUBE-SVC-XXXXXXXXXXXXXXXX -n               # substitute the chain name from the output above
# in the KUBE-SVC-... chain, "statistic mode random probability 0.5" rules pick one of the KUBE-SEP-...
iptables -t nat -L KUBE-SEP-YYYYYYYYYYYYYYYY -n               # DNAT to a specific pod IP:port
```
Today instance mode is rarely chosen: IP mode is simpler and faster. Instance is needed if pod IPs aren't routable in the VPC (for example, a CNI with an overlay network instead of the VPC CNI). On Fargate (an EKS mode without nodes: AWS runs each pod in a separate micro-VM, and you pay per pod rather than for servers), conversely, only IP mode works: there are no nodes to open a NodePort on.

### Scenario 3. ingress-nginx (or another proxy-style controller)
```
User
  → DNS → NLB (created for the controller's Service of type LoadBalancer)
  → ingress-nginx pods (plain nginx running in the cluster)
  → nginx picks an upstream by the Ingress rules and proxies straight to the application pod IP
```
Here everything is different: **the controller is the proxy itself**, and all traffic goes through its pods.
1. ingress-nginx is installed in the cluster as a Deployment (pods with nginx) plus a Service of type **LoadBalancer**. For that Service AWS creates an **NLB** that passes TCP connections to the ingress-nginx pods.
2. The controller inside those pods watches Ingress objects and **generates the nginx configuration**: server blocks for domains, locations for paths.
3. nginx takes the backend addresses **from the EndpointSlice** — these are the application pod IPs. When the list of pods changes, it updates them on the fly (via a built-in load balancer written in Lua — a scripting language embedded into nginx via OpenResty), without reloading nginx.
4. The request: NLB → ingress-nginx pod → TLS, rules, limits → **straight to the application pod IP**. The service's ClusterIP and kube-proxy are not used here either (unless you enable the annotation `nginx.ingress.kubernetes.io/service-upstream: "true"` — then nginx goes to the ClusterIP).

```yaml
# The ingress-nginx controller Service (usually set via the Helm chart values)
apiVersion: v1
kind: Service
metadata:
  name: ingress-nginx-controller
  namespace: ingress-nginx
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: external              # the NLB is created by the AWS Load Balancer Controller
    service.beta.kubernetes.io/aws-load-balancer-nlb-target-type: ip         # the NLB sends traffic to ingress-nginx pod IPs
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
    nginx.ingress.kubernetes.io/proxy-body-size: "20m"          # what would be client_max_body_size in nginx
    nginx.ingress.kubernetes.io/proxy-read-timeout: "120"
spec:
  ingressClassName: nginx
  tls:
    - hosts: [shop.example.com]
      secretName: shop-tls                                       # certificate in a Secret (often from cert-manager)
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
How to see where ingress-nginx actually proxies to:
```bash
kubectl -n ingress-nginx exec deploy/ingress-nginx-controller -- cat /etc/nginx/nginx.conf | grep -A3 "server_name shop.example.com"
kubectl -n ingress-nginx exec deploy/ingress-nginx-controller -- /dbg backends list         # list of upstreams
kubectl -n ingress-nginx exec deploy/ingress-nginx-controller -- /dbg backends get shop-shop-api-80
# "endpoints" will show the application pod IPs — the same as in kubectl get endpointslices
kubectl -n ingress-nginx logs deploy/ingress-nginx-controller --tail=20    # access log: upstream_addr = pod IP
```
Pros and cons: you can use all of nginx's features (rate limiting, rewrites, cache, complex rules) and stay cloud-independent. But the controller pods are an extra hop and need to be scaled, monitored and upgraded. Also, as mentioned in the nginx topic, the community has stopped supporting ingress-nginx, and new clusters look at the Gateway API.

### Scenario 4. A request from inside the cluster — this is where ClusterIP and kube-proxy work
```
frontend pod → DNS: shop-api.shop.svc.cluster.local → ClusterIP 172.20.15.7
  → on the frontend pod's node kube-proxy (iptables) rewrites 172.20.15.7:80 to 10.0.12.34:8080 (DNAT)
  → the packet goes to the shop-api pod directly over the pod network
```
1. A pod calls the service by name. CoreDNS returns the **ClusterIP** — a virtual address that exists on no interface.
2. A packet to the ClusterIP is caught by iptables (or IPVS) rules that kube-proxy wrote in advance **on every node**. The address is rewritten to a pod IP **right on the sender's node** — there is no central load balancer.
3. From there the packet goes to the pod as ordinary pod-to-pod traffic, delivered by the CNI.

With Cilium replacing kube-proxy, eBPF programs in the kernel do the same without iptables rules.

### Comparing the options
| | ALB, IP mode | ALB, instance mode | ingress-nginx behind an NLB | Inside the cluster |
|---|---|---|---|---|
| Who configures it | AWS Load Balancer Controller | AWS Load Balancer Controller | The ingress-nginx controller | kube-proxy from the Service and EndpointSlice |
| What traffic flows through | ALB → pod | ALB → node (NodePort, kube-proxy) → pod | NLB → ingress-nginx pod → pod | pod → (DNAT on the node) → pod |
| Network hops | Minimal | Possibly an extra one (to another node) | An extra one (through the controller pods) | Minimal |
| Service type | ClusterIP | NodePort | ClusterIP for the app, LoadBalancer for the controller | ClusterIP |
| Where TLS ends | On the ALB (certificate from ACM) | On the ALB | In nginx (Secret, cert-manager) or on the NLB | Usually none, or mTLS in a service mesh |
| When to choose | The default on EKS | When pod IPs are not routable in the VPC | You need nginx features, cloud independence | Service-to-service inside the cluster |

### 🧪 Mini task

**Task.** Create a Service for two nginx pods and trace the request path step by step: the Service DNS name → its ClusterIP → the endpoint pods. Then reach it from your Mac via `kubectl port-forward`. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
ClusterIP: 10.96.143.27
Address: 10.96.143.27
pods: 10.244.1.7 10.244.2.5
<title>Welcome to nginx!</title>
```

Breakdown: CoreDNS answers the Service name with its ClusterIP — a virtual address with no interface behind it. kube-proxy (iptables or IPVS) on every node intercepts traffic to that address and spreads it over the pod IPs from the EndpointSlice. From outside the cluster a Service is reached via NodePort, LoadBalancer (on EKS an NLB through the AWS Load Balancer Controller) or Ingress/Gateway (an ALB), which routes by host and path. `port-forward` is a debugging tunnel through the API server, never used for production traffic. Your pod name suffixes and IPs will differ.

</details>

## Security and access: RBAC

**RBAC** (Role-Based Access Control) defines who can do what in the API.
- **Role** — a set of permissions in one namespace: which actions (verbs: get, list, watch, create, update, patch, delete) on which resources.
- **ClusterRole** — the same at cluster level, or for resources outside namespaces (nodes, PVs).
- **RoleBinding** / **ClusterRoleBinding** — **who** gets the role: a user, a group or a **ServiceAccount** (an account for pods).

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-reader
  namespace: dev
rules:
  - apiGroups: [""]                 # "" — the core API group (pods, services, configmaps...)
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
A reuse trick: **ClusterRole + RoleBinding** grants the permissions of a cluster role, but only in one namespace. A "developer" role is described once and bound in the namespaces where it's needed.

Checking permissions:
```bash
kubectl auth can-i create deployments -n prod                               # can I?
kubectl auth can-i list secrets --as=system:serviceaccount:dev:ci-bot -n dev   # can this ServiceAccount?
```

**In EKS** there are two levels:
- **people and CI → Kubernetes**: IAM roles are mapped to Kubernetes users and groups via **access entries** (records in the EKS API saying "this IAM role = this group in the cluster"; previously the `aws-auth` ConfigMap did this, and a mistake in it cut everyone off from the cluster);
- **pods → AWS API**: a pod gets an IAM role through its ServiceAccount — **IRSA** (IAM Roles for Service Accounts, via OIDC — a standard by which the cluster issues the pod a signed token that AWS trusts and exchanges for temporary role keys) or **EKS Pod Identity** (a newer, simpler way to do the same without setting up OIDC). That way a pod gets access, for example, only to its own S3 bucket rather than all of the node's permissions.

More on pod security: run as non-root (`securityContext.runAsNonRoot: true`), `readOnlyRootFilesystem` (the container's root FS is read-only — an attacker can't write their binary), no privileges (`allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`). Enforced via **Pod Security Admission** (a built-in mechanism: a namespace gets a level — privileged — no restrictions, baseline — forbids the obviously dangerous like privileged containers, restricted — strict rules including running as non-root) or Kyverno policies.

### What protects a cluster besides RBAC
RBAC answers only the question "who can do what in the Kubernetes API". Cluster security is built in layers, and in an interview they expect you to name more than one:

| Layer | Tools | What it protects against (example) |
|---|---|---|
| Network between pods | NetworkPolicy (everything denied by default, allow what's needed), Cilium/Calico policies by DNS name | a compromised frontend can't connect directly to the database |
| What may be run | Pod Security Admission, Kyverno or OPA Gatekeeper (policies at object creation time) | banning privileged containers, hostPath, running as root, images tagged `latest` |
| Where images come from | image signing (cosign) and signature verification by a policy, scanning (Trivy, ECR scanning), only your own registry | an image swapped in the registry, an image with a known vulnerability |
| Secrets | encrypting Secrets in etcd with a KMS key (envelope encryption), External Secrets Operator or Secrets Store CSI instead of secrets in git | a leaked etcd backup, a secret in the repository |
| Pod access to the cloud | IRSA or EKS Pod Identity (each pod has its own role), IMDSv2 with hop limit 1 | a pod steals the node role's permissions via the EC2 metadata service |
| API server | a private endpoint (or a list of allowed IPs), audit logs in CloudWatch | brute force and attacks on the API from the internet, no trace of who deleted what |
| Runtime | Falco or GuardDuty EKS Runtime Monitoring | a shell started in a container, a miner, reading `/etc/shadow` |
| Nodes | a minimal OS (Bottlerocket), automatic node image updates, no SSH (SSM) | exploiting vulnerabilities in the node OS |

**A "deny everything" NetworkPolicy for a namespace** is the starting point to which allowing policies are then added:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny
  namespace: prod
spec:
  podSelector: {}                     # all pods in the namespace
  policyTypes: [Ingress, Egress]      # deny both incoming and outgoing traffic
```
Don't forget to separately allow outgoing traffic to CoreDNS (UDP and TCP 53 in `kube-system`), otherwise name resolution in the pods breaks. NetworkPolicy works only if the CNI supports it: in EKS it has to be enabled in VPC CNI, Calico and Cilium support it out of the box.

**Pod Security Admission** is enabled with labels on the namespace:
```bash
kubectl label namespace prod \
  pod-security.kubernetes.io/enforce=restricted \
  pod-security.kubernetes.io/warn=restricted
# a pod that runs as root or with privileged: true will be rejected at creation
```

**Verifying an image signature** (CI signs after the build, a Kyverno policy or a person verifies):
```bash
cosign verify registry.example.com/shop/api:1.4.2 \
  --certificate-identity "https://github.com/acme/shop/.github/workflows/release.yml@refs/heads/main" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
# a keyless signature: the image was signed during a specific GitHub Actions workflow, with no stored key
```

**IMDS hop limit.** The EC2 metadata service (`169.254.169.254`) hands out temporary keys of the node's role. If a pod can reach it, it gets all the node's permissions — bypassing IRSA. The `http-put-response-hop-limit 1` restriction doesn't let the IMDSv2 response make the extra network "hop" to a container in a separate network namespace:
```bash
aws ec2 modify-instance-metadata-options --instance-id i-0abc1234def567890 \
  --http-tokens required --http-put-response-hop-limit 1
```
In EKS this is set in the launch template or the Karpenter EC2NodeClass (`metadataOptions`), not by hand on each instance.

### 🧪 Mini task

**Task.** Create a `viewer` ServiceAccount and, via a Role and a RoleBinding, grant it only read access to pods in the `default` namespace. Check its rights with `kubectl auth can-i` acting as it. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
get pods: yes
delete pods: no
get secrets: no
get pods -n kube-system: no
```

Breakdown: RBAC denies everything by default, and a Role lists the allowed verbs on resources. A Role works in its own namespace, so there are no rights in `kube-system`; cluster-wide access needs a ClusterRole and ClusterRoleBinding. Secrets aren't part of "reading pods" — rights are granted per resource. `--as` checks rights as someone else; that's how "why does the pipeline get Forbidden" is debugged. On EKS an IAM role is mapped to a Kubernetes group via access entries, and pods get AWS rights via IRSA or Pod Identity.

</details>

## CRDs and operators
A **CustomResourceDefinition (CRD)** adds a new object type to the Kubernetes API. For example, cert-manager adds `Certificate`, Prometheus Operator — `ServiceMonitor` (a description of "which services to scrape for metrics"), ArgoCD — `Application` (which repository to deploy into which cluster).

An **operator** is a controller that watches these objects and runs the reconciliation loop for them. An administrator's knowledge is built into it: for example, a PostgreSQL operator, from a `Cluster` object, deploys replicas, sets up replication, takes backups and fails over (failover — automatically promotes a replica to primary if the main server goes down).

### 🧪 Mini task

**Task.** Create a `Backup` CRD with `schedule` and `retentionDays` fields and columns for `kubectl get`, create one `Backup` object and list it. Check that the API server rejects an object with a wrong field type. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
backup.lab.example.com/orders-db created
NAME        SCHEDULE    RETENTION
orders-db   0 3 * * *   7
spec.retentionDays: Invalid value: "string": spec.retentionDays in body must be of type integer: "string"
```

Breakdown: a CRD adds a new resource type to the API: after it, `kubectl get backups` works just like for built-in objects, and the OpenAPI schema makes the API server validate fields. But a `Backup` object does nothing by itself — it's just a record in etcd. An operator is a controller that watches such objects and drives reality towards them: creates a CronJob, takes a snapshot, writes the result to `status`. cert-manager, ArgoCD, Karpenter and Prometheus Operator all work this way. Deleting a CRD deletes every object of that type — done very carefully in production.

</details>

## Static pods
A **static pod** is a pod managed **directly by the kubelet of a specific node**, without the apiserver and without the scheduler. kubelet watches a folder of manifests (usually `/etc/kubernetes/manifests/`, set by the `staticPodPath` parameter) and runs everything it finds there itself.

Why: this is how **the control plane itself** runs in kubeadm clusters — kube-apiserver, etcd, the scheduler and controller-manager are manifests in this folder. The apiserver can't start itself, but kubelet can.

Details:
- kubelet creates a **mirror pod** in the apiserver so that it is visible in `kubectl get pods`. Its name ends with the **node name**: `kube-apiserver-master-1`;
- `kubectl delete pod` won't delete such a pod: kubelet immediately creates it again. To delete it, **remove the manifest file** from the folder on the node;
- to change it, edit the file and kubelet restarts the pod.

The classic trick question "how do you delete a pod without kubectl?" is exactly about static pods.

### 🧪 Mini task

**Task.** Look at the manifests in the static pod directory on the kind control plane node, then delete the `kube-scheduler` mirror pod through the API and check what happened to it. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker exec lab-control-plane ls /etc/kubernetes/manifests
kubectl delete pod -n kube-system kube-scheduler-lab-control-plane > /dev/null
sleep 10
kubectl get pod -n kube-system kube-scheduler-lab-control-plane --no-headers | awk '{print $1, $3}'
```

Expected output:

```text
etcd.yaml
kube-apiserver.yaml
kube-controller-manager.yaml
kube-scheduler.yaml
kube-scheduler-lab-control-plane Running
```

Breakdown: kubelet runs static pods itself from files in `/etc/kubernetes/manifests`, with no API server or scheduler involved. That's how the control plane itself starts: the API server can't launch itself through the API. The API only shows a "mirror" copy: deleting it with `kubectl` stops nothing, and kubelet recreates the mirror right away. To really stop or change such a pod you edit or remove the file on the node. kubeadm rewrites exactly these manifests during a cluster upgrade.

</details>

## kubectl create and kubectl apply
- **`kubectl create`** — **imperative**: "create this object". If the object already exists — an error. Changes are made with other commands (`kubectl edit`, `set`, `scale`), and the history of what was intended isn't kept.
- **`kubectl apply`** — **declarative**: "make the object look like this". Creates the object if it doesn't exist and **updates** it if it does, changing only the differing fields. It remembers the last applied configuration in the `kubectl.kubernetes.io/last-applied-configuration` annotation so that on the next apply it can tell which fields you **removed** from the manifest (and remove them from the object) and which were changed by someone else.

For GitOps and keeping manifests in git, only apply works (and even better, server-side apply: `kubectl apply --server-side` — the apiserver tracks field "owners", and conflicts between tools become explicit).

### 🧪 Mini task

**Task.** Create a ConfigMap from a file twice with `kubectl create`, then twice with `kubectl apply`, change a value and apply again. Compare the responses and find the annotation where `apply` stores the last applied state. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
configmap/app-cfg created
Error from server (AlreadyExists): error when creating "cm.yaml": configmaps "app-cf
configmap/app-cfg created
configmap/app-cfg unchanged
configmap/app-cfg configured
{"LOG_LEVEL":"debug"}
```

Breakdown: `create` is an imperative command, "create this", and a second call fails with `AlreadyExists`. `apply` is declarative: "make the object match this description". It creates if missing, does nothing if identical and updates if different, so it can run any number of times — CI and GitOps use exactly that. The `last-applied-configuration` annotation enables a three-way merge: that's how `apply` knows which field you removed from the file and which one someone else added. With `--server-side` the API server does this work via `managedFields`.

</details>

## Several clusters: kubeconfig contexts
A kubeconfig (`~/.kube/config`) contains **clusters** (address and CA certificate), **users** (how to authenticate) and **contexts** (a "cluster + user + default namespace" triple).
```bash
kubectl config get-contexts                       # all contexts, the current one is marked with *
kubectl config current-context
kubectl config use-context prod-eks               # switch to another cluster
kubectl config set-context --current --namespace=payments   # default namespace for the current context
aws eks update-kubeconfig --region eu-central-1 --name prod-cluster --alias prod-eks   # add an EKS cluster
```
Handy tools: `kubectx` and `kubens` (quick switching of cluster and namespace), Lens (a graphical app) or k9s (a UI right in the terminal). To avoid running a command against prod by accident, show the current context in the shell prompt (kube-ps1, starship — they add the cluster and namespace name to the prompt).

### 🧪 Mini task

**Task.** See which contexts your kubeconfig has and which one is current. Create a `lab-dev` context for the same cluster with `dev` as the default namespace, switch to it, check the namespace and switch back. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
kubectl config get-contexts -o name
kubectl config current-context
kubectl config set-context lab-dev --cluster=kind-lab --user=kind-lab --namespace=dev > /dev/null
kubectl config use-context lab-dev
kubectl config view --minify -o jsonpath='namespace: {..namespace}{"\n"}'
kubectl config use-context kind-lab > /dev/null && kubectl config delete-context lab-dev > /dev/null
kubectl config current-context
```

Expected output:

```text
kind-lab
kind-lab
Switched to context "lab-dev".
namespace: dev
kind-lab
```

Breakdown: a context is a triple of "cluster + user (credentials) + default namespace". Switching context changes both the cluster and the namespace, so before a dangerous command you always check `current-context` (or show it in your shell prompt). `--minify` shows only the current context. Several kubeconfig files are merged with `KUBECONFIG=file1:file2`. `kubectx` and `kubens` make switching quick. The habit of an explicit `--context` on every command in scripts saves you from a `delete` in the wrong cluster.

</details>

## A local cluster on a Mac
You don't need a cloud cluster for experiments:
- **kind** — Kubernetes in Docker containers, a cluster in a minute, handy for CI and testing manifests;
- **k3d** — a lightweight k3s (a stripped-down Kubernetes distribution in a single binary) in Docker; on a Mac without Docker Desktop it can run on top of **Lima** or **Colima** (lightweight Linux VMs);
- **minikube** — the classic option with add-ons;
- **OrbStack**, **Rancher Desktop** — Docker and Kubernetes in one app (a Docker Desktop replacement).

### 🧪 Mini task

**Task.** Use kind to create the `lab` cluster with one control plane node and two workers (the other tasks in this topic use it) and check the nodes. You need Docker Desktop running and `brew install kind`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
lab-control-plane Ready control-plane
lab-worker Ready <none>
lab-worker2 Ready <none>
lab-control-plane
lab-worker
lab-worker2
```

Breakdown: every kind "node" is a Docker container running systemd, containerd and kubelet, and pods start inside it. That's why the cluster comes up in a minute and leaves no trace when removed: `kind delete cluster --name lab`. kind added the `kind-lab` context to `~/.kube/config` and switched to it. Limitations: no cloud LoadBalancers (only NodePort or port-forward), no metrics (`metrics-server` is installed separately) and no real storage. For testing manifests, Helm charts and operators in CI that's enough.

</details>

## kubectl — cheat sheet
```bash
# viewing
kubectl get pods -A -o wide                      # all pods in all namespaces, with nodes and IPs
kubectl get pod api-7d9f -o yaml                 # the full object with its status
kubectl describe pod api-7d9f                    # description + Events at the bottom — the FIRST thing to check on problems
kubectl get events -n prod --sort-by=.lastTimestamp
kubectl explain deployment.spec.strategy         # documentation for a field right in the terminal
kubectl api-resources                            # which object types exist in the cluster

# logs and debugging
kubectl logs api-7d9f -c api --previous          # logs of the PREVIOUS (crashed) run of the container
kubectl logs -l app=api --tail=100 -f            # by label, from all pods
kubectl exec -it api-7d9f -- sh
kubectl debug -it api-7d9f --image=busybox --target=api   # an ephemeral container — a temporary container with tools next to the main one (the app image often has neither sh nor curl)
kubectl debug node/ip-10-0-1-5 -it --image=ubuntu         # a shell on the node (the node's file system in /host)
kubectl port-forward svc/api 8080:80             # access the service from your local machine
kubectl top pods; kubectl top nodes              # consumption (needs metrics-server)

# changes
kubectl apply -f manifest.yaml
kubectl diff -f manifest.yaml                    # what will change, before apply
kubectl scale deploy/api --replicas=5
kubectl set image deploy/api api=registry/api:1.5
kubectl rollout status|history|undo|restart deploy/api

# nodes
kubectl cordon node1                             # forbid new pods on the node
kubectl drain node1 --ignore-daemonsets --delete-emptydir-data   # evict pods (respecting PDBs)
kubectl uncordon node1

# generating manifests
kubectl create deployment web --image=nginx --dry-run=client -o yaml > deploy.yaml
kubectl get pods -o jsonpath='{.items[*].spec.containers[*].image}'   # all images
kubectl get pods -o custom-columns=NAME:.metadata.name,NODE:.spec.nodeName,RESTARTS:.status.containerStatuses[0].restartCount
```

### 🧪 Mini task

**Task.** Print all pods in the cluster as a table with the namespace, node, restart count and the first container's image, sorted by restarts, and then find pods not in the `Running` phase. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
kubectl get pods -A --sort-by='.status.containerStatuses[0].restartCount' \
  -o custom-columns=NS:.metadata.namespace,POD:.metadata.name,NODE:.spec.nodeName,RESTARTS:.status.containerStatuses[0].restartCount,IMAGE:.spec.containers[0].image | tail -3
kubectl get pods -A --field-selector=status.phase!=Running -o name
```

Expected output:

```text
kube-system   kube-proxy-r4nkt                    lab-control-plane   0          registry.k8s.io/kube-proxy:v1.33.1
kube-system   kube-scheduler-lab-control-plane    lab-control-plane   1          registry.k8s.io/kube-scheduler:v1.33.1
kube-system   kube-controller-manager-lab-control-plane   lab-control-plane   2          registry.k8s.io/kube-controller-manager:v1.33.1
```

Breakdown: `custom-columns` and `jsonpath` turn kubectl into a reporting tool without `jq`, and `--sort-by` sorts by any object field: the most "unstable" pods end up at the bottom. `--field-selector` filters on the API server side but supports only a few fields (`status.phase`, `spec.nodeName`, `metadata.name`); labels are filtered with `-l`. Empty output from the second command is a good sign: every pod is `Running`. Useful extras: `-w` to watch in real time, `kubectl explain pod.spec.containers` for documentation right in the terminal. Your pod name suffixes and IPs will differ.

</details>

## Troubleshooting: pod statuses and what to do about them

The algorithm is always the same: **`kubectl describe pod` (the Events section) → `kubectl logs` (and `--previous`) → namespace events → node state.**

| Status | What it means | Where to look for the cause |
|---|---|---|
| `Pending` | The pod isn't assigned to a node or hasn't started yet | Events: `Insufficient cpu/memory` (requests exceed free capacity; Karpenter or Cluster Autoscaler didn't add a node), `didn't tolerate taint`, affinity doesn't match, `unbound PersistentVolumeClaim` (the PVC wasn't created or the disk is in another zone) |
| `ContainerCreating` for a long time | The image is being pulled, a volume is being mounted, CNI doesn't assign an IP | Events: mount errors, `failed to assign an IP address` (in EKS — the subnet ran out of IPs) |
| `ImagePullBackOff` / `ErrImagePull` | The image can't be pulled | A typo in the name or tag; no access to a private registry (`imagePullSecrets` — a Secret with the registry login, the node's IAM permissions on ECR — the AWS image registry); Docker Hub limits (anonymously — 100 pulls per 6 hours per IP, a large cluster behind NAT hits them quickly) |
| `CrashLoopBackOff` | The container starts and crashes, Kubernetes restarts it with a growing delay | `kubectl logs --previous`: an application error, a missing environment variable or config, a wrong start command, can't connect to the database; or the liveness probe is too aggressive |
| `OOMKilled` (exit 137) | The memory limit was exceeded | `describe` → Last State: Reason OOMKilled. Raise the limit or hunt for a leak; for the JVM and .NET — check the runtime's memory settings (for example, `-XX:MaxRAMPercentage` for Java: the runtime decides itself how much memory to take for the heap, and without tuning may not fit into the container limit) |
| `CreateContainerConfigError` | The ConfigMap or Secret the pod references doesn't exist | Events: `secret "x" not found` |
| `Running`, but `0/1 Ready` | The readiness probe isn't passing | A wrong probe path or port, the application is still warming up, a dependency is unavailable |
| `Terminating` forever | The pod isn't deleted | Finalizers on the object (marks saying "before deleting, a controller must finish cleanup" — if the controller is broken, the object hangs forever), the node is unreachable (kubelet doesn't confirm deletion). `--force --grace-period=0` is a last resort |
| `Evicted` | Evicted by kubelet | The node is short of memory or disk (`DiskPressure`, `MemoryPressure` — node conditions under which kubelet starts freeing resources), a pod without requests (BestEffort) is evicted first |

**The service doesn't respond:**
```bash
kubectl get endpointslices -l kubernetes.io/service-name=api   # does the service have any addresses?
```
If there are no addresses — the service selector doesn't match the pod labels, or the pods aren't Ready. If there are addresses — check that `targetPort` matches the port the application actually listens on, and that the application listens on `0.0.0.0`, not `127.0.0.1`. Then — NetworkPolicy.

### 🧪 Mini task

**Task.** Create two "broken" pods: one with a non-existent image and one with an app that crashes right after starting. Determine each one's status, the reason from the events and the logs of the crashed container. You need the kind lab cluster `lab` — it's created in the task of the "A local cluster on a Mac" section.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
kubectl run badimage --image=nginx:no-such-tag > /dev/null
kubectl run crasher --image=busybox:1.36 -- sh -c 'echo "connecting to db..."; echo "FATAL: DATABASE_URL not set" >&2; exit 1' > /dev/null
sleep 40
kubectl get pods badimage crasher --no-headers | awk '{print $1, $3, $4}'
kubectl get events --field-selector involvedObject.name=badimage,reason=Failed -o jsonpath='{.items[0].message}' | cut -c1-70; echo
kubectl logs crasher --previous
kubectl delete pod badimage crasher > /dev/null
```

Expected output:

```text
badimage ImagePullBackOff 0
crasher CrashLoopBackOff 3
Failed to pull image "nginx:no-such-tag": rpc error: code = NotFound
connecting to db...
FATAL: DATABASE_URL not set
```

Breakdown: two different classes of problem. `ImagePullBackOff` — the container never even started: a typo in the tag, no access to a private registry (`imagePullSecrets`, ECR permissions) or a Docker Hub rate limit; the reason is in the `kubectl describe pod` events. `CrashLoopBackOff` — the container starts and dies, and kubelet restarts it with a growing delay; the cause is in the logs, specifically `--previous`, because the current instance may not have written anything yet. The order for any pod: `get` (status) → `describe` (events) → `logs` (`--previous`) → `exec` or `kubectl debug`.

</details>

## Interview questions

**How does a Deployment differ from a StatefulSet?**
A Deployment is for interchangeable replicas: random names, a shared disk or none, parallel startup. A StatefulSet is for stateful applications: stable names (`app-0`) and DNS names for each pod, its own persistent disk for every replica via `volumeClaimTemplates`, ordered startup and shutdown. For databases, Kafka, ZooKeeper.

**How does a Service work if it has no process?**
ClusterIP is a virtual address that isn't on any interface. The EndpointSlice controller keeps the list of ready pod IPs, and kube-proxy on every node creates iptables or IPVS rules: traffic to ClusterIP:port is rewritten (DNAT) to the IP of one of the pods. Load balancing happens on the client's node.

**What happens on `kubectl apply` of a Deployment?**
The apiserver (authentication, RBAC, admission) stores the object in etcd → the Deployment controller creates a ReplicaSet → the ReplicaSet controller creates pods → the scheduler assigns nodes → kubelet starts the containers via containerd and CNI assigns IPs → once readiness passes, the pod is added to the service's EndpointSlice. All components interact only through the apiserver.

**What happens if the control plane goes down?**
Pods that are already running keep running and serving traffic: kubelet and kube-proxy work autonomously on the nodes, and the iptables rules stay in place. But nothing new happens: no scheduling, no restarting crashed pods on other nodes, no scaling, no deployments, and `kubectl` doesn't work.

**What happens when etcd loses quorum?**
Writes to etcd are impossible, the apiserver can't change state: the cluster is effectively frozen. Running pods keep running. Recovery — bring the etcd nodes back or restore from a snapshot.

**How do you deploy without downtime?**
RollingUpdate with `maxUnavailable: 0`, a correct readiness probe, preStop with a delay, SIGTERM handling in the application, a sufficient `terminationGracePeriodSeconds`, a PDB and several replicas in different zones, backward-compatible database migrations.

**Describe the order in which a pod is stopped.**
The pod is marked Terminating; in parallel it is removed from the EndpointSlice and the preStop hook runs; then SIGTERM; waiting up to `terminationGracePeriodSeconds` (30 s by default); then SIGKILL. Because of the parallelism a preStop with a pause is needed, otherwise the load balancer still sends traffic to an application that has already shut down.

**How do requests differ from limits? What happens without requests?**
Requests are the guaranteed minimum; the scheduler picks a node based on them. Limits are the ceiling: exceeding memory leads to OOMKilled, exceeding CPU to throttling. Without requests a pod gets the BestEffort class: the scheduler treats it as free, nodes get overloaded, the pod is evicted first, and HPA can't compute utilisation percent.

**How does liveness differ from readiness?**
A failed liveness probe restarts the container. A failed readiness probe removes the pod from load balancing without a restart. You must not check external dependencies in liveness, otherwise their unavailability causes a cascading restart of all pods.

**How do you restrict network access between pods and namespaces?**
NetworkPolicy: a default deny policy per namespace and allow policies by `podSelector` and `namespaceSelector`. The CNI must support NetworkPolicy (Calico, Cilium, VPC CNI with the network policy agent). When egress is denied — allow DNS.

**How should secrets be stored properly?**
Not in git in plain text: External Secrets Operator with AWS Secrets Manager or SSM, Sealed Secrets, SOPS. Enable Secret encryption in etcd via KMS. Restrict RBAC access to Secrets. Remember that a Secret is base64, not encryption.

**How do you upgrade the cluster version?**
Check for deprecated APIs in manifests beforehand (`pluto`, `kubent` — tools that find API versions removed in the new Kubernetes version in manifests and Helm releases), upgrade the control plane (one minor version at a time), then the add-ons (CoreDNS, kube-proxy, CNI, CSI drivers), then the nodes: a new node group or drift in Karpenter (Karpenter notices a node no longer matches the current NodePool or AMI and replaces it itself), cordon and drain the old ones respecting PDBs.

**What are an operator and a CRD?**
A CRD adds a new resource type to the API. An operator is a controller that reconciles these resources and encodes an administrator's knowledge (deploy a database cluster, take a backup, fail over the primary). Examples: cert-manager, Prometheus Operator, ArgoCD.

**HPA doesn't scale. Why?**
No metrics-server; the containers have no requests (the percentage is computed from them); `maxReplicas` has been reached; the metric doesn't arrive (`kubectl describe hpa` shows the error); scaling is blocked by the lack of nodes (pods are Pending).

**A pod can't reach a service by its DNS name.**
Check CoreDNS (pods and logs in `kube-system`), `/etc/resolv.conf` inside the pod, `nslookup api.prod.svc.cluster.local` from inside the pod, NetworkPolicy on egress to port 53, and whether the service exists in the right namespace (without a namespace the name is looked up in your own).

**Taints and tolerations — how do you dedicate nodes to a specific application only?**
A taint on the nodes (other pods won't come) plus, on the right pods, a toleration (they may come) and nodeAffinity or nodeSelector (they must go exactly there). A toleration alone doesn't attract a pod to a node.

**A pod is Pending although the cluster has resources.**
Events in `describe pod`: taints or affinity don't match; the PVC is in another zone (`WaitForFirstConsumer` is needed); resources exist in total but no single node has a big enough chunk (fragmentation); the namespace ResourceQuota is exhausted; the per-node pod limit (in EKS with the VPC CNI it depends on the number of ENIs — network interfaces — and IPs per ENI for the instance type: for example, a `t3.medium` allows only 17 pods).

**How does ClusterIP differ from NodePort and LoadBalancer?**
ClusterIP is an internal address reachable only inside the cluster. NodePort adds a port 30000–32767 on every node. LoadBalancer adds an external cloud load balancer. Each next type includes the previous one.

**Why do you need a headless Service?**
No virtual IP: DNS returns the IPs of all pods directly. Needed by StatefulSets (stable DNS names for each pod) and by clients that balance themselves or need to know all replicas (database clusters, Kafka).

**How does a pod get access to AWS (for example, S3)?**
Via IRSA or EKS Pod Identity: the pod's ServiceAccount is linked to an IAM role, and the pod gets temporary credentials for that role only. Not via the node role — otherwise all pods on the node would get the access.

**How do you delete a pod without kubectl?**
A trick question about static pods: they are managed by kubelet directly from files in `/etc/kubernetes/manifests/`. To delete such a pod, delete the manifest file on the node; `kubectl delete` won't help, kubelet will recreate it. You can tell a static pod by its name: it ends with the node name.

**How does `kubectl create` differ from `kubectl apply`?**
`create` is imperative creation and fails if the object exists. `apply` is declarative: it creates or updates to the described state, remembers the last applied configuration in an annotation and uses it to work out which fields to remove. Only apply is suitable for GitOps.

**Why is Karpenter better than Cluster Autoscaler?**
Cluster Autoscaler scales pre-created groups of identical nodes (ASGs): different instance types and spot need many groups, and switching is slow. Karpenter works without groups: from the requirements of pending pods it picks the cheapest suitable instance type itself, launches nodes faster, switches between spot and on-demand on its own and constantly packs the load (consolidation), removing empty and underused nodes.

**HPA, KEDA and Karpenter — what is the difference and how do they work together?**
They work at different levels. HPA changes the replica count based on CPU and memory relative to requests, with a minimum of one replica. KEDA also changes the replica count, but based on external events — queue length, Kafka consumer lag, a Prometheus query, cron — and can scale to zero; under the hood it creates a regular HPA, so you must not attach your own HPA to the same Deployment. Karpenter scales nodes, not pods: it sees Pending pods, picks the cheapest suitable instance type (including Spot) and removes extra nodes through consolidation. A typical setup: HPA for APIs, KEDA for queue workers, and Karpenter underneath for nodes; all of them need correct requests.

**Why doesn't Karpenter remove nodes after the load is gone?**
There's a pod with the `karpenter.sh/do-not-disrupt` annotation on the node, a PDB doesn't allow evicting pods, the disruption budget in the current window is 0, `consolidationPolicy: WhenEmpty` (underused nodes are left alone), or the node has pods without a controller. Check the Karpenter logs for `disrupting nodeclaim` and the events on the nodes.

**Why does HPA scale down slowly?**
By default the stabilization window for scaling down is 300 seconds: HPA waits for 5 minutes of consistently low load so as not to remove replicas on a brief dip. It is configured in `spec.behavior.scaleDown`.

**How do Flannel, Calico and Cilium differ?**
Flannel is a simple VXLAN overlay without NetworkPolicy support. Calico is L3 routing with route exchange over BGP (or tunnels), full NetworkPolicy and its own extended policies. Cilium is eBPF-based: high performance, kube-proxy replacement, L7 policies, observability via Hubble, transparent encryption.

**How does an external request in EKS reach a pod? Is kube-proxy involved?**
It depends on how ingress is set up. With the AWS Load Balancer Controller in IP mode the controller only configures the ALB, and the ALB itself, following the Ingress rules, sends the request straight to the pod IP registered in the target group: kube-proxy, NodePort and ClusterIP are not involved, which is possible because VPC CNI gives pods addresses from the VPC. In instance mode the ALB sends to the NodePort of any node, and kube-proxy then rewrites the address to a pod IP, possibly on another node. With ingress-nginx the request goes through an NLB to the nginx pods, which proxy straight to pod IPs from the EndpointSlice. ClusterIP and kube-proxy work mostly for traffic inside the cluster.

**Traffic grew 10x in a minute. What happens to the cluster and how do you prepare for it?**
HPA sees the metrics grow within 15–30 seconds and adds replicas, but if there are no free nodes the pods go Pending, and Karpenter or Cluster Autoscaler adds nodes only after 40–90 seconds, plus pulling the image and starting the application: a "cold" pod takes 1–3 minutes. You prepare in advance: a capacity reserve via low-priority placeholder pods (overprovisioning), scaling on a leading metric (RPS, a queue) instead of CPU, a fast `behavior.scaleUp`, small images and a fast start, a higher replica minimum for known peaks (KEDA cron). Plus the dependencies: a DB connection pool, AWS quotas, free IPs in the subnets, rate limiting so that under overload some requests get a 429 rather than everything going down.

**How does VPA differ from HPA and can they be combined?**
HPA changes the number of replicas, VPA — one pod's requests and limits based on the consumption history. They can't be combined on the same metric: in response to growing load HPA adds replicas while VPA enlarges the pod, and they interfere with each other. Working options: HPA on CPU or RPS + VPA on memory only, or VPA in `Off` mode as a source of recommendations for manual tuning. Classic VPA applies new values by recreating the pod, so in prod it's often kept in `Off` or `Initial`.

**When do you need KEDA instead of an ordinary HPA?**
When you need to scale on an external event: SQS queue length, Kafka consumer lag, a Prometheus query, a schedule. And when you need to go down to zero replicas when there's no work. KEDA creates and manages an ordinary HPA, adding metric sources and a scale-to-zero mode to it. A typical example is queue workers whose CPU is low while thousands of messages have piled up.

**How do you protect a Kubernetes cluster besides RBAC?**
In layers: NetworkPolicy with default deny, Pod Security Admission or Kyverno against privileged containers and running as root, only signed and scanned images from your own registry, secrets encrypted with KMS in etcd and delivered from Secrets Manager via External Secrets, pods with their own IAM roles via IRSA or Pod Identity and closed access to IMDS (hop limit 1), a private API endpoint and audit logs, runtime detection (Falco, GuardDuty), a minimal node OS without SSH.

## Kubernetes The Hard Way: walkthrough notes

Results of building a cluster by hand with Kelsey Hightower's guide (completed in October 2026 on a Mac with an Apple M1 processor, the guide version that supports arm64 — the 64-bit ARM architecture). Why it's worth doing at all is in [Practice: Kubernetes The Hard Way](#01-kubernetes.md/s1). Here: what exactly each step does, how it works, how the same thing is done in kubeadm (the official installation tool) and EKS (managed Kubernetes on AWS), and a real breakage that had to be fixed.

### Lab: 4 virtual machines in Lima
**Lima** is a Mac tool that brings up a Linux VM (virtual machine) with one command on Apple's built-in virtualization. The guide needs 4 Debian 12 machines that can reach each other over the network. CPU in the table means virtual processor cores, GB means gigabytes of RAM:

| Machine | Role | Resources |
|---|---|---|
| `jumpbox` | All commands run from here: generating certificates, copying files over SSH (Secure Shell — encrypted remote login) | 1 CPU, 1 GB |
| `server` | Control plane (the managing part of the cluster): etcd, kube-apiserver, scheduler, controller-manager | 1 CPU, 2 GB |
| `node-0`, `node-1` | Workers (nodes where pods run): containerd, kubelet, kube-proxy | 1 CPU, 2 GB each |

```bash
limactl start --name=node-0 --containerd=none --cpus=1 --memory=2 --disk=20 \
  --network=lima:user-v2 template://debian-12
limactl shell node-0 -- sudo -i     # log in to the machine as root
limactl delete -f node-0            # delete the machine together with its disk
```
- The `lima:user-v2` network is a shared Lima virtual network in which the virtual machines can reach each other by IP address. Without it every VM is isolated and sees only the Mac.
- `--containerd=none` — don't install Lima's preinstalled containerd. Without this flag you get a version conflict (see "Incident" below).

**What had to be adapted for Lima:**
- **Root login over SSH.** The guide enables root password login and runs `ssh-copy-id`. Root has no password in the Lima image, so the jumpbox public key is put into `/root/.ssh/authorized_keys` on the other machines via `limactl shell` from the Mac. Key-based root login is allowed in Debian by default (`PermitRootLogin prohibit-password`).
- **`/etc/hosts` without a `127.0.1.1` line.** The guide's `sed` command replaces that line, and since it's missing, it silently does nothing. The line has to be added.
- **cloud-init** (the first-boot configuration system of cloud images) resets the machine name to `lima-<name>` on every boot. Fixed with a file `/etc/cloud/cloud.cfg.d/99-keep-hostname.cfg` containing `preserve_hostname: true`. Otherwise after a reboot the name no longer matches the certificates.

### How the resulting cluster is built
```text
jumpbox ── kubectl (admin) ──────────────────────────────┐
                                                         ▼
server:   etcd ◄── kube-apiserver ◄── kube-scheduler, kube-controller-manager
                        ▲     │
          kubelet, kube-proxy (node-0, node-1) ─┘     └──► kubelet (logs, exec)
node-N:   kubelet → containerd → containerd-shim-runc-v2 → runc → container
```
- **etcd** is a key-value store holding the whole cluster state. Only kube-apiserver talks to it.
- **kube-apiserver** is the API server (API — Application Programming Interface, the programmatic interface the cluster is managed through) and the only entry point: all components and you yourself work only through it, not with each other.
- **scheduler** picks a node for a new pod, **controller-manager** brings the actual state to the desired one (for example, creates pods for a Deployment). Both are ordinary API clients.
- **kubelet** on a node sees in the API the pods assigned to its node and asks **containerd** (the container runtime) to start them. For each container containerd starts a **shim** (`containerd-shim-runc-v2`, a middleman process that keeps the container alive even if containerd itself is restarted), and the shim calls **runc** — the tool that creates the actual container using kernel mechanisms (namespaces, cgroups).
- **kube-proxy** turns Services into iptables rules (the Linux kernel packet filter) so that a request to a ClusterIP (the virtual IP address of a service) reaches one of the pods.
- The order of the guide's steps follows the dependencies: first PKI (certificates) and configs (04–06), then etcd (07), then the control plane (08), then the nodes (09), networking (11) and the check (12).

### Step 04: PKI — who presents which certificate to whom
The cluster **PKI** (Public Key Infrastructure — the certificate infrastructure): its own **CA** (Certificate Authority — the authority that signs all other certificates) and a certificate per component. All connections use **mTLS** (mutual TLS — both sides present certificates, not just the server). kube-apiserver takes the **CN** (Common Name) of the client certificate as the **user name** and the **O** (Organization) as the **group**, and then **RBAC** (Role-Based Access Control — permissions by role) decides what that client may do.

All certificates are described in `ca.conf` — an OpenSSL config with a section per component:

| Certificate | CN → user | O → group | TLS role (Transport Layer Security — connection encryption) | Why |
|---|---|---|---|---|
| `ca` | `CA` | — | signs the others | Root of trust. Whoever owns `ca.key` can issue themselves any access |
| `admin` | `admin` | `system:masters` | client | You with kubectl. `system:masters` bypasses RBAC completely, and such a certificate can't be revoked |
| `node-0`, `node-1` | `system:node:node-0` | `system:nodes` | client and server | kubelet. The Node Authorizer (a special authorization mode for nodes) lets a node access only its own pods; as a server, kubelet accepts `logs`/`exec` from the apiserver |
| `kube-proxy` | `system:kube-proxy` | `system:node-proxier` | client | Built-in role: read Services and EndpointSlices |
| `kube-controller-manager` | `system:kube-controller-manager` | — | client | Built-in permission binding for this name |
| `kube-scheduler` | `system:kube-scheduler` | — | client | Same (`ca.conf` has a typo `system:system:` in the O field; it doesn't matter, the permissions are bound to the CN) |
| `kube-api-server` | `kubernetes` | — | **server** | The main server certificate, with a long SAN (the list of names and IP addresses it's issued for) |
| `service-accounts` | — | — | key pair | controller-manager signs pod tokens, the apiserver verifies them |

The apiserver's **SAN** (Subject Alternative Name — the list of names and IPs the certificate is valid for as a server certificate): `127.0.0.1`, `10.32.0.1` (the first IP address of the service range — the ClusterIP of the `kubernetes.default` service through which pods reach the API server), `kubernetes.default.svc` and similar names, `server.kubernetes.local`. Connecting via an address that isn't in the SAN gives the error `x509: certificate is valid for …, not …` — one of the most common ones when building by hand and after changing a load balancer address.

How each certificate is issued:
```bash
openssl genrsa -out node-0.key 4096                                     # private key
openssl req -new -key node-0.key -config ca.conf -section node-0 \
  -out node-0.csr                                                       # CSR: signing request with CN, O, SAN
openssl x509 -req -in node-0.csr -CA ca.crt -CAkey ca.key \
  -copy_extensions copyall -days 3653 -out node-0.crt                   # the CA signs
openssl x509 -in node-0.crt -noout -subject -ext subjectAltName         # check the result
```
A **CSR** (Certificate Signing Request) is a signing request: the public key plus owner data, which the CA turns into a certificate.

### Step 05: a kubeconfig for each component
A **kubeconfig** is a YAML file (a text config format) with three lists: `clusters` (the API server address and the CA — the certificate authority to trust), `users` (client certificate and key), `contexts` (a "cluster + user" pair) and a `current-context` pointer. It's the "pass + address" for every apiserver client.

```bash
kubectl config set-cluster kubernetes-the-hard-way --certificate-authority=ca.crt \
  --embed-certs=true --server=https://server.kubernetes.local:6443 --kubeconfig=node-0.kubeconfig
kubectl config set-credentials system:node:node-0 --client-certificate=node-0.crt \
  --client-key=node-0.key --embed-certs=true --kubeconfig=node-0.kubeconfig
kubectl config set-context default --cluster=kubernetes-the-hard-way \
  --user=system:node:node-0 --kubeconfig=node-0.kubeconfig
kubectl config use-context default --kubeconfig=node-0.kubeconfig
```
- `--embed-certs=true` puts the certificates into the file in base64, so the file is self-contained: copying it to the node is enough.
- Who the component is for the API server is decided by **the CN and O (user name and group) inside the certificate**, not by the entry name in `users`.
- There are five kinds of files: kubelet (one per node, `/var/lib/kubelet/kubeconfig`), kube-proxy (`/var/lib/kube-proxy/kubeconfig`), controller-manager and scheduler (on `server`), admin. Admin uses `https://127.0.0.1:6443` because it's used right on `server`; the others use `server.kubernetes.local`, over the network.
- A kubeconfig contains a private key, so the file is a password: `admin.kubeconfig` gives full control of the cluster.

Check who a component will present itself as:
```bash
kubectl config view --raw --kubeconfig node-0.kubeconfig \
  -o jsonpath='{.users[0].user.client-certificate-data}' | base64 -d | openssl x509 -noout -subject
```

### Step 06: encrypting Secrets in etcd
By default a Secret is stored in etcd in plain text (base64 is an encoding, not encryption), and passwords are visible to anyone with access to etcd or its backup. **Encryption at rest** means the apiserver encrypts a Secret before writing it to etcd.

```bash
export ENCRYPTION_KEY=$(head -c 32 /dev/urandom | base64)        # 32 random bytes = an AES-256 key
envsubst < configs/encryption-config.yaml > encryption-config.yaml # put the key into the template
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
- **AES** (Advanced Encryption Standard — the standard symmetric cipher) in **CBC** mode (Cipher Block Chaining). Writes are encrypted with the **first** provider in the list; on read the providers are tried in order.
- `identity: {}` means "no encryption"; it goes last so that Secrets written before encryption was enabled can still be read. Putting it first means writing new Secrets unencrypted.
- In etcd the record gets the prefix `k8s:enc:aescbc:v1:key1:` — that's how the apiserver knows which key to decrypt with.
- Key rotation: add `key2` second → restart the apiserver → move `key2` first → restart → rewrite all Secrets (`kubectl get secrets -A -o json | kubectl replace -f -`) → remove `key1`.
- Limitation: the key sits in a file on the same server. In production the **KMS v2** provider (Key Management Service) is used: data is encrypted with a DEK (Data Encryption Key), and the DEK with a KEK master key (Key Encryption Key) that never leaves an external KMS (AWS KMS — the Amazon Web Services key service, HashiCorp Vault). The Kubernetes documentation no longer recommends `aescbc`.

### Steps 07–09: etcd, control plane and workers
- **07 etcd** — a systemd service on `server`. A single-node etcd without quorum (agreement of a majority of members): enough for learning; in production etcd runs on 3 or 5 members.
- **08 control plane** — kube-apiserver, controller-manager and scheduler as systemd units on `server`. Key apiserver flags tie the previous steps together: `--client-ca-file` (which CA — certificate authority — to verify clients with, step 04), `--tls-cert-file` (its own server certificate), `--encryption-provider-config` (step 06), `--service-cluster-ip-range=10.32.0.0/24` (the range of virtual IP addresses for services), `--authorization-mode=Node,RBAC`.
- **09 workers** — on each node containerd, runc, CNI plugins (CNI, Container Network Interface — the standard for plugins that give pods IP addresses and connect them to the network), kubelet and kube-proxy. Each node has its own pod address range: `node-0` — `10.200.0.0/24`, `node-1` — `10.200.1.0/24`, handed out by the `bridge` plugin with `host-local` IPAM (IP Address Management — address assignment).

### Step 11: routes between pod subnets
A pod on `node-0` (address from `10.200.0.0/24`) doesn't know how to reach a pod on `node-1` (`10.200.1.0/24`): they're different machines. The guide adds ordinary static routes on `server` and on the nodes:
```bash
ip route add 10.200.1.0/24 via 192.168.104.5    # on node-0: node-1's pod subnet — via node-1's IP
ip route add 10.200.0.0/24 via 192.168.104.3    # on node-1: node-0's pod subnet — via node-0's IP
```
In real clusters this is done by the CNI (Container Network Interface — the cluster network plugin): Flannel and Calico build such routes or tunnels themselves, and in EKS (managed Kubernetes on AWS — Amazon Web Services) the **VPC CNI** gives pods addresses straight from VPC subnets (Virtual Private Cloud — an isolated network in AWS), so no routes are needed.

### Step 12: smoke test — what is checked
A **smoke test** is a quick check that "the main functions are alive". First — that the Secret is really encrypted in etcd (the command runs on `server`):
```bash
etcdctl get /registry/secrets/default/kubernetes-the-hard-way | hexdump -C | head
# the output shows the prefix k8s:enc:aescbc:v1:key1 instead of the secret value
```
Then the cluster itself is checked:
- Deployment nginx creates a pod (scheduler, controller-manager, kubelet, containerd work);
- `kubectl port-forward`, `kubectl logs`, `kubectl exec` — the apiserver reached the kubelet using its server certificate;
- a Service of type NodePort (a port opened on every node) answers `curl` — kube-proxy and networking work.

**What the cluster doesn't have:** CoreDNS (in-cluster DNS; DNS is the naming system that turns a name into an IP address) isn't installed in the current version of the guide. Hence the `MissingClusterDNS` warning with the text `kubelet does not have ClusterDNS IP configured`: pods get the node's DNS settings, and names like `nginx.default.svc` don't resolve. There's also no control plane high availability and no automated upgrades — it's a learning cluster, not an installation method.

### Hard way, kubeadm and EKS: the same thing three ways
**kubeadm** is the official tool that does steps 04–09 automatically. **EKS** (Elastic Kubernetes Service) is managed Kubernetes on AWS (Amazon Web Services), where the control plane is entirely on the AWS side.

| What | Hard way | kubeadm | EKS |
|---|---|---|---|
| Administrator access | `admin.kubeconfig` with a certificate in the `system:masters` group | `/etc/kubernetes/admin.conf` (since 1.29 permissions via an RBAC group, plus a separate emergency `super-admin.conf`) | No file: `aws eks update-kubeconfig` writes `exec: aws eks get-token`, identity is an IAM role (Identity and Access Management — AWS accounts and roles), permissions via EKS access entries ("IAM role → permissions in the cluster" records) and RBAC (permissions by role) |
| Control plane | systemd units on `server` | static pods (pods the kubelet starts straight from files in `/etc/kubernetes/manifests/`) | Hidden, managed by AWS |
| PKI (cluster certificates) | Your CA (certificate authority) from step 04 | `/etc/kubernetes/pki/`, certificates valid for 1 year, `kubeadm certs renew all` | With AWS, rotated automatically |
| Component kubeconfigs | `/var/lib/kubelet`, `/var/lib/kube-proxy`, `/var/lib/kubernetes`, certificates embedded (`--embed-certs`) | `/etc/kubernetes/*.conf`, the set depends on the node role | Only for the kubelet on the node, without a certificate: a token from the instance IAM role |
| kubelet | Certificate issued by hand | TLS bootstrapping (TLS — connection encryption; the node requests its certificate itself with a temporary token) and auto-rotation | Node IAM role token → user `system:node:<name>` |
| kube-proxy | systemd on the nodes, its own kubeconfig | DaemonSet (a pod on every node) + ConfigMap | EKS add-on, also a DaemonSet |
| Secrets encryption | `aescbc`, key in a file | Off by default, the same `EncryptionConfiguration` | AWS KMS (Key Management Service — the key service), envelope encryption: data is encrypted with a key that is itself encrypted with a master key in KMS |
| Pod network | Static routes | You install a CNI (network plugin) yourself: Calico, Cilium, Flannel | VPC CNI: pods get IP addresses from the VPC (the AWS network) |
| New node | Everything by hand (step 09) | `kubeadm join` with a bootstrap token | Managed node group or Karpenter: EKS AMI (Amazon Machine Image — the node image) + IAM role |

Interview takeaway: in EKS, steps 04–08 are taken over by AWS, and you keep the nodes (image, kubelet, IAM role), access (IAM → access entries → RBAC), networking (VPC CNI — the AWS network plugin) and add-ons (cluster extensions: kube-proxy, CoreDNS, VPC CNI).

### Incident: a pod isn't created because of two containerd versions
Told in the **STAR** format (Situation, Task, Action, Result), which is convenient for interviews.

**Situation.** During the smoke test the nginx pod hangs in `ContainerCreating`, and `kubectl describe pod` shows the event:
```text
Warning  FailedCreatePodSandbox  kubelet  Failed to create pod sandbox: rpc error: code = Unknown
desc = failed to start sandbox "e43ef26390e515ed92fbb06fcb1e21f367b3fffa37cfba7c31f942fb254841cb":
failed to create containerd task: failed to start shim: start failed: io.containerd.runc.v2:
bootstrap parameters do not match command-line arguments: shim was started through the
deprecated API but was built against the new API; if you upgraded containerd, you may need to
restart the daemon: exit status 1
```
A **pod sandbox** is the pod's "shell": the network namespace and the auxiliary `pause` container created before the application containers. If the sandbox isn't created, none of the pod's containers start.

**Task.** Find out why containerd can't start the shim and fix it without rebuilding the cluster.

**Action.** Checked on the node which binaries are actually used:
```bash
which -a containerd containerd-shim-runc-v2                     # all copies in PATH, in order
containerd --version; containerd-shim-runc-v2 -v                # the version of each
systemctl cat containerd | grep ExecStart                       # what systemd starts
readlink /proc/$(systemctl show containerd -p MainPID --value)/exe   # what is actually running
```
There were two installations: the guide put containerd **v2.1.0-beta** into `/bin`, and the Lima image had preinstalled its own containerd **v2.4.1** into `/usr/local/bin`. systemd started the daemon from `/bin`, and the daemon looked up the shim **via `PATH`**, where `/usr/local/bin` comes first, and found a shim of another version with an incompatible startup protocol.

**Result.** Disabled the extra copy and restarted the services; the pod came up:
```bash
mv /usr/local/bin/containerd /usr/local/bin/containerd.lima-disabled
mv /usr/local/bin/containerd-shim-runc-v2 /usr/local/bin/containerd-shim-runc-v2.lima-disabled
systemctl restart containerd kubelet
```
Lesson for the future: create VMs (virtual machines) with `--containerd=none`. In production the same problem shows up after a node image update or manually installing extra packages: on `FailedCreatePodSandbox`, first compare the versions and paths of `containerd`, the shim and `runc`.

### Checklist: where to look when the cluster is broken
| Symptom | Where to look |
|---|---|
| `x509: certificate is valid for …, not …` | SAN (list of names and IP addresses) of the apiserver server certificate: `openssl x509 -in kube-api-server.crt -noout -ext subjectAltName`; the address in the client kubeconfig |
| `x509: certificate signed by unknown authority` | Client and server trust different CAs (certificate authorities): compare `certificate-authority-data` in the kubeconfig and the apiserver `--client-ca-file` |
| `x509: certificate has expired` | Certificate dates: `openssl x509 -in kube-api-server.crt -noout -enddate`; in kubeadm `kubeadm certs check-expiration` |
| `forbidden: User "…" cannot …` | Authentication passed, permissions are missing: CN and O of the certificate (name and group), RBAC — permissions by role (`kubectl auth can-i get pods --as=system:node:node-0`) |
| Node `NotReady` | `journalctl -u kubelet` on the node: kubeconfig, access to the API server, whether containerd is running |
| Pod in `ContainerCreating`, `FailedCreatePodSandbox` | `journalctl -u containerd`, versions and paths of containerd, shim, runc; CNI (network plugin) configs in `/etc/cni/net.d/` |
| Pods on different nodes can't reach each other | Routes (`ip route`) or the CNI (network plugin); in EKS (managed Kubernetes on AWS) — security groups (the instance firewall) and the VPC CNI |
| apiserver doesn't start | `journalctl -u kube-apiserver`: access to etcd, certificate paths, encryption config |
