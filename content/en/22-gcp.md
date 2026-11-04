# GCP (Google Cloud)

Google Cloud Platform basics for a DevOps/SRE engineer who already knows AWS: how GCP is built differently, which services map to which, and what gets asked in interviews. Every topic: what it is, why, how it works, an example and typical traps. The mini tasks need a GCP project: on sign-up Google gives a free trial — $300 in credits for 90 days — and some resources (for example, one small e2-micro VM in US regions) are free permanently (Free Tier).

## GCP through an AWS engineer's eyes: the service map

Cloud concepts are shared; the names and a few fundamental details differ. The mapping table is the first thing to learn: in an interview it is fine to say "I worked with AWS ALB; the GCP equivalent is the Application Load Balancer, and the difference is…".

| Task | AWS | GCP |
|---|---|---|
| Isolating resources and billing | Account, AWS Organizations, OU (Organizational Unit, a folder of accounts) | **Project**, Organization, Folder |
| Access management | IAM users, roles, policies | IAM: principals, roles, **allow policies on the resource**, service accounts |
| Virtual machines | EC2 | **Compute Engine** (GCE) |
| Auto-scaling group of VMs | Auto Scaling Group + Launch Template | **Managed Instance Group (MIG)** + Instance Template |
| Kubernetes | EKS | **GKE** (Google Kubernetes Engine) |
| Serverless containers | Fargate, App Runner | **Cloud Run** |
| Functions | Lambda | **Cloud Run functions** (formerly Cloud Functions) |
| Network | VPC (regional) | **VPC (global)** |
| VM access rules | Security Group | **VPC firewall rules** (by network tags or service accounts) |
| NAT for private VMs | NAT Gateway | **Cloud NAT** |
| Load balancers | ALB, NLB | **Application LB, Network LB** |
| DNS | Route 53 | **Cloud DNS** |
| CDN (a network of caching servers close to users) and WAF (a filter for malicious HTTP requests) | CloudFront, AWS WAF, Shield | **Cloud CDN, Cloud Armor** |
| Object storage | S3 | **Cloud Storage** (GCS, Google Cloud Storage) |
| VM disks | EBS | **Persistent Disk, Hyperdisk** |
| Managed relational DB | RDS, Aurora | **Cloud SQL, AlloyDB** |
| NoSQL | DynamoDB | **Firestore, Bigtable** |
| Cache | ElastiCache | **Memorystore** |
| Analytics data warehouse | Redshift, Athena | **BigQuery** |
| Queues and events | SQS, SNS, EventBridge | **Pub/Sub, Eventarc** |
| Secrets and keys | Secrets Manager, KMS | **Secret Manager, Cloud KMS** |
| Logs and metrics | CloudWatch | **Cloud Logging, Cloud Monitoring** |
| Action audit | CloudTrail | **Cloud Audit Logs** |
| Image registry | ECR | **Artifact Registry** |
| CI/CD | CodeBuild, CodePipeline | **Cloud Build, Cloud Deploy** |
| Organization-level guardrails | SCP (Service Control Policies) | **Organization Policies** |

The key differences worth naming in an interview:
- **The VPC is global**, while subnets are regional and span all zones of a region. In AWS a VPC lives in one region and a subnet in one availability zone.
- **The project is the main unit of isolation**, like an AWS account, but it is created in seconds and there are usually many: one per environment or team.
- **Permissions are granted on the resource** (project, bucket, topic) rather than described by a policy on the user. In AWS you more often write a policy and attach it to a role.
- **Firewall rules instead of Security Groups**: rules are defined at the network level and applied to VMs by network tag or service account.
- **A global load balancer with a single anycast IP** (one address announced from many points worldwide, so users land on the nearest one) — in AWS this needs Global Accelerator or CloudFront.
- **Live migration**: during hardware maintenance Google moves a running VM to another host without a reboot. In AWS you get "scheduled retirement" emails and have to restart the instance.

### 🧪 Mini task

**Task.** Install the gcloud CLI (GCP's command-line tool) and list the regions along with the number of zones in each, to see how regions and zones are named.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

```bash
brew install --cask google-cloud-sdk
gcloud init
gcloud compute zones list --format="value(region.basename())" | sort | uniq -c | head -5
```

Expected result:

```text
   3 africa-south1
   3 asia-east1
   3 asia-east2
   3 asia-northeast1
   3 asia-northeast2
```

Explanation: `gcloud init` opens a browser to sign in to a Google account and asks for a default project and region. Regions are named `europe-west3` (Frankfurt), `us-central1` (Iowa), and zones are the region plus a letter: `europe-west3-a`. Most regions have three zones, as in AWS. `--format="value(...)"` is gcloud's built-in formatting: it prints only the needed fields without headers, and `basename()` cuts everything but the name from the region link. You need a project with the Compute Engine API enabled, otherwise gcloud offers to enable it. Your region list will be current as of when you run it.

</details>

## Resource hierarchy: organization, folders, projects

```
Organization (example.com)            ← tied to a Google Workspace or Cloud Identity domain
├── Folder: prod
│   ├── Project: shop-prod            ← resources: VMs, buckets, clusters
│   └── Project: shop-prod-data
├── Folder: nonprod
│   ├── Project: shop-dev
│   └── Project: shop-stage
└── Project: shared-network           ← shared network (Shared VPC) for all projects
Billing Account ── pays for ── projects (one billing account can pay for many projects)
```

- **Organization** is the root of the hierarchy. It appears when a company connects its domain through Google Workspace (mail, documents) or Cloud Identity (a free user directory without mail). Without an organization, projects exist on their own — typical for personal accounts.
- **Folder** groups projects by environment, department or product. Folders can be nested.
- **Project** is a container for resources and the boundary for APIs, quotas and billing. A project has three identifiers:
  - **Project ID** (`shop-prod-4821`) — globally unique, set at creation and **never changes**; it is what you write in commands and Terraform;
  - **Project name** — a human-readable name that can be changed;
  - **Project number** (`123456789012`) — numeric, assigned by Google; it shows up in service account names and in errors.
- **Billing Account** is a separate entity with a card or contract; projects are linked to it. Unlink it and the project's resources stop.

Why the hierarchy matters: **permissions and policies are inherited downwards**. A role granted on the `prod` folder applies in all its projects. **Organization Policies** are inherited the same way — guardrails that even a project owner cannot bypass, the equivalent of AWS SCPs. Examples:
- `constraints/gcp.resourceLocations` — allow resources only in EU regions (GDPR requirements — the European personal data protection law);
- `constraints/iam.disableServiceAccountKeyCreation` — forbid creating service account keys;
- `constraints/compute.vmExternalIpAccess` — forbid VMs with public IPs.

**APIs must be enabled in every project.** In a new project almost every service is off: before creating a VM you enable the Compute Engine API (`gcloud services enable compute.googleapis.com`). A typical beginner error in Terraform is `Error 403: Compute Engine API has not been used in project … before or it is disabled`.

### 🧪 Mini task

**Task.** Create a project for experiments, make it the default project, enable the Compute Engine API in it and see which APIs are enabled.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal (the project ID must be globally unique, so we add a random suffix):

```bash
PROJECT_ID="sandbox-$RANDOM$RANDOM"
gcloud projects create "$PROJECT_ID" --name="Sandbox"
gcloud config set project "$PROJECT_ID"
gcloud billing accounts list --format="value(name)" | head -1 | xargs -I{} gcloud billing projects link "$PROJECT_ID" --billing-account={}
gcloud services enable compute.googleapis.com
gcloud services list --enabled --format="value(config.name)" | grep -E 'compute|oslogin'
```

Expected result:

```text
Create in progress for [https://cloudresourcemanager.googleapis.com/v1/projects/sandbox-1234567890].
Waiting for [operations/cp.1234567890] to finish...done.
Updated property [core/project].
billingAccountName: billingAccounts/0X0X0X-0X0X0X-0X0X0X
billingEnabled: true
compute.googleapis.com
oslogin.googleapis.com
```

Explanation: a project is created in seconds — unlike an AWS account, it is fine to create one for an experiment and delete it (`gcloud projects delete`; it then stays in the trash for 30 days). Without a billing account link, paid APIs will not enable. Dependent APIs are enabled automatically together with Compute Engine — here OS Login (signing in to VMs via IAM permissions). Your IDs and numbers will differ.

</details>

## IAM: who can do what

### Key concepts

- **Principal** — whoever access is granted to: a Google account (`user:anna@example.com`), a group (`group:devops@example.com`), a service account, an entire domain, or an external identity via Workload Identity Federation (see below).
- **Role** — a set of permissions like `compute.instances.start`. Individual permissions cannot be granted directly — only as part of a role. Three kinds of roles:
  - **Basic**: `roles/owner`, `roles/editor`, `roles/viewer`. Too broad — `editor` can change almost anything in a project. Avoided in production;
  - **Predefined** (ready-made by Google): `roles/compute.instanceAdmin.v1`, `roles/storage.objectViewer`, `roles/container.developer`. Used most often;
  - **Custom**: assembled from the needed permissions when no predefined role fits or it is too broad.
- **Allow policy** — attached to a **resource** (organization, folder, project, bucket, topic) and made of **bindings**: "role X — to these principals". This is the main difference from AWS: there a policy usually sits on a user or role and lists resources, while in GCP the resource itself stores who has access to it.
- **Inheritance**: effective permissions = the union of bindings on the resource and all its ancestors. A role on a project cannot be "taken away" on a bucket inside it — for that there are separate **deny policies** (they take precedence over allow policies), but they are rarely used.
- **IAM Conditions** — a condition on a binding: for example, access only until a certain date or only to resources with a certain name.

```bash
# let a group read objects in one bucket
gcloud storage buckets add-iam-policy-binding gs://shop-prod-assets \
  --member="group:frontend@example.com" --role="roles/storage.objectViewer"
# who has access to the project
gcloud projects get-iam-policy shop-prod --format=json
```

### Service accounts

A **service account (SA)** is an identity for programs, not people: `deployer@shop-prod.iam.gserviceaccount.com`. The equivalent of an AWS IAM role assumed by EC2 or a pod.

How a program gets an SA's permissions, from best to worst:
1. **Attach the SA to a resource**: VMs, Cloud Run and Cloud Build run "as" the SA. Code gets short-lived tokens from the **metadata server** (the service address `169.254.169.254` inside a VM, like IMDS — Instance Metadata Service — in AWS) — there are no keys at all.
2. **Workload Identity Federation** — for external systems: GitHub Actions, GitLab CI, AWS. The external system presents its OIDC token (OpenID Connect — a standard by which a service issues a signed token saying "I am a pipeline of such-and-such repository"), and GCP exchanges it for a temporary GCP token. No keys either. The AWS equivalent is an IAM role with an OIDC provider for GitHub.
3. **Impersonation**: a person with `roles/iam.serviceAccountTokenCreator` temporarily acts as the SA: `gcloud ... --impersonate-service-account=deployer@...`. Handy for testing an SA's permissions or running Terraform without broad personal rights.
4. **An SA JSON key** — a file with a private key that works until it is deleted. It can be stolen from a repository or CI. This is the **worst option**: organizations often forbid key creation with an Organization Policy.

The trap: the **Compute Engine default service account** (`PROJECT_NUMBER-compute@developer.gserviceaccount.com`) — VMs get it automatically unless another SA is specified. In older projects it has `editor` on the whole project: compromise one VM and you get the right to change almost everything. The right way is a separate SA per application with minimal roles.

### How to sign in yourself: two kinds of credentials

- `gcloud auth login` — credentials for **the gcloud tool itself**.
- `gcloud auth application-default login` — **ADC** (Application Default Credentials): picked up by Terraform and the Python or Go SDKs. On VMs and in Cloud Run, ADC come from the metadata server automatically.

A common confusion: gcloud works, but Terraform says "could not find default credentials" — that means only the first command was run.

### 🧪 Mini task

**Task.** Create a service account `reader`, grant it a VM view-only role on the project, and use impersonation to verify it can list VMs but cannot create buckets.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

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

Expected result:

```text
WARNING: This command is using service account impersonation. All API calls will be executed as [reader@sandbox-1234567890.iam.gserviceaccount.com].
Listed 0 items.
ERROR: (gcloud.storage.buckets.create) HTTPError 403: reader@sandbox-1234567890.iam.gserviceaccount.com does not have storage.buckets.create access to the Google Cloud project.
```

Explanation: the `roles/compute.viewer` role granted the `compute.instances.list` permission, so the list was read (there are no VMs yet). It has no `storage.buckets.create` permission — we get a 403, and the error text states exactly which permission is missing, which helps when debugging. To act as the SA, your user needs `serviceAccountTokenCreator` on that SA. `sleep 60` is there because IAM changes do not apply instantly, usually within a minute. No SA key was ever created. Your names will differ.

</details>

## Networking: VPC

### VPC layout

- **A VPC (Virtual Private Cloud) is a global resource.** One network can have subnets in Europe and the US, and VMs in them talk over internal IPs through Google's network without VPN or peering. In AWS this would need two VPCs and peering or a Transit Gateway.
- **A subnet is regional** and spans all zones of the region. There is no need for "a subnet per AZ" as in AWS: one `europe-west3` subnet — and VMs in zones `-a`, `-b`, `-c`.
- **VPC modes:**
  - **auto mode** — GCP itself creates a `/20` subnet in every region from `10.128.0.0/9`. This is how the `default` network that appears in a new project is built;
  - **custom mode** — you create subnets yourself. Production uses only this: auto-mode ranges overlap between projects, so they can later not be connected by peering or VPN.
- **Secondary ranges** of a subnet — for GKE pod and service IPs (alias IP, see the GKE section).
- **Routes:** every VPC has a default route `0.0.0.0/0` to the default internet gateway. There is no separate Internet Gateway object as in AWS — a VM with an external IP reaches the internet right away.
- **There are no public and private subnets in the AWS sense either**: "public" is determined by whether the VM itself has an external IP.

### Firewall rules

The equivalent of Security Groups, but built differently:
- defined **at the VPC level**, not on a network interface;
- applied to VMs by **target**: all VMs in the network, VMs with a **network tag** (a `web` or `ssh` label on the VM), or VMs with a specific **service account** (more reliable: anyone with VM permissions can set a tag, while an SA is granted through IAM);
- **stateful**: return traffic for an allowed connection passes automatically, as with Security Groups;
- there are **allow and deny** rules, each with a **priority** from 0 to 65535 — **a lower number wins**. A rule with priority 100 fires before one with 1000;
- **implied rules** with the lowest priority 65535: **all ingress denied, all egress allowed**.

```bash
# let HTTP/HTTPS in only to VMs tagged web
gcloud compute firewall-rules create allow-web \
  --network=prod-vpc --direction=INGRESS --action=ALLOW \
  --rules=tcp:80,tcp:443 --source-ranges=0.0.0.0/0 --target-tags=web --priority=1000
```

The `default` network already has `default-allow-ssh` and `default-allow-rdp` rules **from the whole internet** — one reason not to use it in production.

Besides regular rules there are **firewall policies** — rule sets attached to an organization, folder or network. Security teams use them to set common denies for all projects at once.

### Internet and Google API access without an external IP

- **Cloud NAT** — managed NAT (Network Address Translation, replacing an internal address with an external one) for VMs without an external IP, the equivalent of a NAT Gateway. It is not a separate VM but a configuration attached to a **Cloud Router** (GCP's managed router, which also exchanges routes over BGP — the protocol networks use to exchange routes — with VPN and Interconnect). One Cloud NAT serves the whole region, not a single zone.
- **Private Google Access** — a subnet flag: VMs without an external IP reach Google APIs (Cloud Storage, Artifact Registry, BigQuery) over the internal network. The equivalent of AWS VPC endpoints, but turned on with one checkbox.
- **IAP TCP forwarding** (Identity-Aware Proxy — a Google proxy that lets you reach a resource only after an IAM check): SSH to a VM without an external IP and without a bastion: `gcloud compute ssh vm1 --tunnel-through-iap`. It needs a firewall rule allowing TCP 22 from the IAP range `35.235.240.0/20`. The AWS equivalent is SSM Session Manager.

### Connecting a VPC to other networks

- **VPC Network Peering** connects two VPCs (including across projects and organizations). It is **not transitive**: if A is peered with B and B with C, A does not see C. Ranges must not overlap.
- **Shared VPC** — one network in a **host project** (the project owning the network) used by **service projects** (application projects). The network team manages subnets and firewall centrally, while application teams create VMs in "someone else's" subnets. The equivalent is VPC sharing via AWS RAM (Resource Access Manager). A very common company setup.
- **Cloud VPN** — IPsec tunnels (encrypted tunnels over the internet) to a data center or another cloud. **HA VPN** — two interfaces with a 99.99% SLA (Service Level Agreement, guaranteed availability).
- **Cloud Interconnect** — a dedicated physical link: **Dedicated** (your own cable at a Google point of presence, 10 or 100 Gbit/s) or **Partner** (through a provider). The equivalent of AWS Direct Connect.
- **Private Service Connect** — private access to another project's service or to Google APIs via an internal IP in your network. The equivalent of AWS PrivateLink.

### 🧪 Mini task

**Task.** Create a custom-mode VPC with one subnet in Frankfurt, a firewall rule allowing SSH only through IAP, and Cloud NAT for internet egress. Print the network's rules and subnets.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

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

Expected result:

```text
NAME      REGION        RANGE         PRIVATE_IP_GOOGLE_ACCESS
lab-euw3  europe-west3  10.10.0.0/24  True
NAME               DIRECTION  PRIORITY  SRC_RANGES       ALLOW
lab-allow-iap-ssh  INGRESS    1000      35.235.240.0/20  tcp:22
```

Explanation: the network is global, while the subnet is tied to a region and spans all three Frankfurt zones. SSH is open only to IAP — port 22 cannot be reached from outside; you sign in with `gcloud compute ssh ... --tunnel-through-iap`. All other ingress is closed by the implied rule with priority 65535, which is not shown in the list. Cloud NAT gives VMs without an external IP internet access for `apt update`, and Private Google Access gives access to Google APIs without NAT. Cloud NAT is paid, so delete it after experimenting: `gcloud compute routers delete lab-router --region=europe-west3`.

</details>

## Load balancers and DNS

### Load balancer types

GCP names load balancers by layer and scope:

| Load balancer | Layer | Scope | AWS equivalent | When to use |
|---|---|---|---|---|
| **Global external Application LB** | L7 (HTTP/HTTPS) | global, single anycast IP | CloudFront + ALB / Global Accelerator | public sites and APIs for users worldwide |
| **Regional external/internal Application LB** | L7 | one region | ALB | when data must not leave the region or you need an internal HTTP load balancer |
| **Proxy Network LB** | L4 (TCP, with TLS termination) | global or regional | NLB with TLS | TCP services that need a global IP |
| **Passthrough Network LB** | L4 (TCP/UDP) | regional | NLB | when the VM must see the client's source IP and handle the connection itself (games, VPN, your own HAProxy) |

L7 and L4 are OSI model layers: L7 understands HTTP (paths, headers, cookies), L4 sees only IPs, ports and TCP/UDP. A proxy load balancer terminates the client connection and opens a new one to the backend; a passthrough one lets packets through as is.

What an HTTP(S) load balancer is made of — this gets asked because in the console and Terraform the components are created separately:

```
forwarding rule (IP:443) → target HTTPS proxy (+ SSL certificate) → URL map (rules: /api → backend A)
   → backend service (+ health check, Cloud CDN, Cloud Armor) → backends: MIG or NEG
```

- **Forwarding rule** — the external IP and port clients connect to.
- **Target proxy** — terminates the TLS connection (TLS — encryption on top of TCP that HTTPS runs on); the certificate lives here (Google-managed certificates are issued and renewed automatically).
- **URL map** — routing by host and path, like ALB listener rules.
- **Backend service** — a group of backends with a health check, balancing algorithm, CDN and WAF.
- **Backends** — a Managed Instance Group (a group of VMs) or a **NEG** (Network Endpoint Group — a set of endpoints: GKE pods directly, Cloud Run, external addresses).

**The health check trap:** health checks do not come from inside the VPC but from Google addresses `35.191.0.0/16` and `130.211.0.0/22`. If the firewall does not allow them, all backends are considered unhealthy and the load balancer returns 502. A classic first-deployment problem.

### Cloud Armor and Cloud CDN

- **Cloud Armor** — a WAF (Web Application Firewall, a filter of HTTP requests against SQL injection, XSS — Cross-Site Scripting, injecting someone else's JavaScript into a page — and so on) and DDoS protection (a distributed denial-of-service attack) on external Application LBs. Rules by IP, country, headers, rate limiting, and ready-made OWASP rule sets (Open Worldwide Application Security Project, a list of the most common web application vulnerabilities).
- **Cloud CDN** (Content Delivery Network) — caching responses at Google's points of presence; enabled with a flag on the backend service.

### Cloud DNS

Managed DNS, the equivalent of Route 53:
- **public zone** — a zone visible from the internet;
- **private zone** — visible only from the specified VPCs (internal names like `db.internal.example.com`);
- **forwarding zone** — queries for the zone are forwarded to other DNS servers (in a data center or in AWS);
- inside a VPC every VM has an automatic name `VM.ZONE.c.PROJECT.internal`.

### 🧪 Mini task

**Task.** See which ready-made WAF rule sets (preconfigured rules) Cloud Armor has, and create the firewall rule without which load balancer health checks fail.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

```bash
gcloud compute security-policies list-preconfigured-expression-sets | grep -E '^(sqli|xss|lfi|rce)-v33-stable'
gcloud compute firewall-rules create lab-allow-health-checks --network=lab-vpc \
  --direction=INGRESS --action=ALLOW --rules=tcp:80 \
  --source-ranges=35.191.0.0/16,130.211.0.0/22 --target-tags=web
gcloud compute firewall-rules describe lab-allow-health-checks --format="value(sourceRanges.list())"
```

Expected result:

```text
lfi-v33-stable
rce-v33-stable
sqli-v33-stable
xss-v33-stable
35.191.0.0/16,130.211.0.0/22
```

Explanation: the `sqli`, `xss`, `lfi` (Local File Inclusion — reading the server's local files through request parameters) and `rce` (Remote Code Execution — running code on the server) sets are rules based on the OWASP ModSecurity Core Rule Set; you add them to a Cloud Armor policy with one line, `evaluatePreconfiguredWaf('sqli-v33-stable')`. The firewall rule lets Google health checks reach VMs tagged `web` — without it the load balancer considers the backends dead and returns 502. Rule set versions (`v33`) get updated over time, so your list may differ.

</details>

## Compute: Compute Engine, MIG, Cloud Run

### Compute Engine (VMs)

- **Machine families:**
  - **E2** — cheap general-purpose VMs whose CPU core may be shared with others (like T3 in AWS); `e2-micro` is in the Free Tier;
  - **N2, N2D, N4** — general purpose with steady performance (like M6i/M7i);
  - **C3, C4** — compute-optimized;
  - **M3** — large memory for SAP (big enterprise resource systems) and in-memory databases (keeping all data in RAM);
  - **C4A, T2A** — on Arm processors (C4A is Google's own Axion CPU), the equivalent of Graviton.
- **Custom machine types** — you can set any number of vCPUs and amount of memory (for example, 6 vCPUs and 20 GB) instead of picking from a fixed list as in AWS.
- **Spot VMs** — unused capacity at a 60–91% discount; Google can reclaim the VM with 30 seconds' notice. **Preemptible VMs** are the older version of Spot that lived at most 24 hours. Good for CI, batch jobs and stateless GKE nodes.
- **Live migration** — during hardware maintenance the VM moves to another host without a reboot (except VMs with GPUs and some other types).
- **Metadata and startup script** — the `startup-script` key in VM metadata runs on every boot (the equivalent of EC2 user data, though EC2 user data runs only on first launch by default).
- **OS Login** — SSH sign-in through IAM roles (`roles/compute.osLogin`, `roles/compute.osAdminLogin`) instead of distributing SSH keys to VMs. Someone leaves, you remove them from the group — access is gone on every VM.

### Managed Instance Groups (MIG)

**Instance template** (a VM template, like a Launch Template) + **MIG** (a group of identical VMs, like an Auto Scaling Group):
- **autoscaling** — by CPU, load balancer load, a Cloud Monitoring metric or a schedule;
- **autohealing** — a health check: an unresponsive VM is recreated;
- **zonal or regional MIG** — a regional group spreads VMs across several zones and survives a zone failure;
- **rolling update** — gradual replacement of VMs with a new template using `maxSurge` (how many extra VMs may be created during the update) and `maxUnavailable` (how many VMs may be down at once) — the same concepts as a Kubernetes Deployment.

### Disks

- **Persistent Disk (PD)** — network disks, the equivalent of EBS: `pd-standard` (HDD), `pd-balanced` (the default SSD), `pd-ssd` (fast SSD). **Hyperdisk** is the new generation where performance (IOPS — input/output operations per second — and throughput) is configured separately from size, like gp3/io2.
- **Regional PD** — the disk is synchronously replicated to two zones; if a zone fails, the VM is brought up in the second zone with the same disk. EBS has nothing like it.
- **Local SSD** — a disk physically attached to the host, very fast, but **data is lost** when the VM stops. For caches and temporary files.
- **Snapshots** — incremental disk snapshots, stored by default in multi-regional storage; a disk can be created from them in any region.

### Cloud Run and functions

- **Cloud Run** runs a container without managing servers: give it an image, get an HTTPS address. It scales by the number of requests, **including down to zero** (no requests — no charge). One instance handles up to 80 concurrent requests by default (`concurrency`, max 1000) — unlike Lambda, where one instance handles one request. The AWS equivalent is App Runner or Fargate, but simpler.
  - **Services** — HTTP services;
  - **Jobs** — tasks that run and finish (migrations, batch processing).
- **Cloud Run functions** (formerly Cloud Functions) — Python, Node.js or Go functions triggered by an event (an HTTP request, a Pub/Sub message, a new file in a bucket). The equivalent of Lambda; under the hood it is now the same Cloud Run.
- **App Engine** is the older platform (PaaS — Platform as a Service, a platform you upload code to rather than servers); new projects usually pick Cloud Run.

How to choose: **Cloud Run** — for stateless HTTP services and APIs without Kubernetes; **GKE** — when there are many services, you need your own operators, stateful workloads or a service mesh; **Compute Engine** — for legacy, databases on your own VMs and anything that does not fit in a container.

### 🧪 Mini task

**Task.** Deploy a public sample image to Cloud Run with one command, get the URL and check that the service responds.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

```bash
gcloud services enable run.googleapis.com
gcloud run deploy hello --image=us-docker.pkg.dev/cloudrun/container/hello \
  --region=europe-west3 --allow-unauthenticated --max-instances=2 --quiet
URL=$(gcloud run services describe hello --region=europe-west3 --format="value(status.url)")
curl -s "$URL" | grep -o '<title>.*</title>'
gcloud run services describe hello --region=europe-west3 --format="value(spec.template.spec.containerConcurrency)"
```

Expected result:

```text
Service [hello] revision [hello-00001-abc] has been deployed and is serving 100 percent of traffic.
Service URL: https://hello-123456789012.europe-west3.run.app
<title>Congratulations | Cloud Run</title>
80
```

Explanation: Cloud Run issued an HTTPS certificate itself and gave out an address. Every deploy creates a **revision** (an immutable snapshot of the image and settings), and traffic can be split between revisions (`--tag`, `--no-traffic` for canary releases). `80` is `containerConcurrency`, how many requests one instance handles at once. `--max-instances=2` is the instance ceiling that protects against an unexpected bill. `--allow-unauthenticated` opens the service to the whole internet; internal services do not set it and can only be called with the IAM role `roles/run.invoker`. To delete: `gcloud run services delete hello --region=europe-west3`. Your URL will differ.

</details>

## GKE — Kubernetes in GCP

Kubernetes comes from Google, and GKE is considered the most mature managed Kubernetes. The essentials:

### Modes: Autopilot and Standard

| | Autopilot | Standard |
|---|---|---|
| Nodes | managed by Google, you do not see them | your **node pools** (groups of nodes of one type), you pick the machines |
| Billing | for resources requested by pods (`requests`) | for node VMs, even when idle |
| Limitations | no privileged pods, no SSH to nodes, some DaemonSets not allowed | full control |
| When to use | the default for new clusters when you do not need fine-grained node tuning | GPUs, specific kernels and node settings, your own agents on nodes |

The AWS equivalent of Autopilot is EKS Auto Mode or EKS on Fargate.

### Key concepts

- **Zonal or regional cluster** — a zonal cluster's control plane (the API server and etcd — the cluster state database) lives in one zone: while that zone is under maintenance or down, `kubectl` does not work (pods keep running). A regional one has three control plane replicas in three zones. For production — regional.
- **Release channels**: Rapid, Regular, Stable, Extended. GKE **upgrades** the control plane and nodes **itself** within the chosen channel; you control this with **maintenance windows** (when upgrades are allowed) and **maintenance exclusions** (blackout periods, for example during a sale). In EKS you start upgrades yourself.
- **VPC-native cluster** — pods get IPs from a subnet's secondary range (alias IP): pods are visible in the VPC as regular addresses, without an overlay. Pod and service ranges are planned in advance — they are hard to extend, and when addresses run out new nodes are not created.
- **Private cluster** — nodes have no external IPs, and the API server is reachable only from allowed networks.
- **Workload Identity Federation for GKE** — a pod gets IAM permissions without keys: a Kubernetes ServiceAccount is linked to IAM roles. The equivalent of IRSA (IAM Roles for Service Accounts) and EKS Pod Identity in AWS. Without it pods get the node SA's permissions — meaning all pods on a node have the same rights.
- **Cluster autoscaler** (adds and removes nodes in a node pool) and **node auto-provisioning** (creates new node pools of a suitable type itself, similar to Karpenter in AWS).
- **Cost**: $0.10 per hour per cluster (the management fee), but one zonal or Autopilot cluster per billing account is effectively free — Google credits that amount back monthly.

```bash
# get a kubeconfig for the cluster (needs the gke-gcloud-auth-plugin)
gcloud container clusters get-credentials prod-gke --region=europe-west3
# grant a pod permissions via Workload Identity: Kubernetes ServiceAccount app in namespace default gets a role on a bucket
gcloud storage buckets add-iam-policy-binding gs://shop-prod-assets \
  --role=roles/storage.objectViewer \
  --member="principal://iam.googleapis.com/projects/123456789012/locations/global/workloadIdentityPools/shop-prod.svc.id.goog/subject/ns/default/sa/app"
```

### 🧪 Mini task

**Task.** Create the cheapest kind of cluster — Autopilot — get a kubeconfig, run a pod and see which nodes appeared under it.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal (creating the cluster takes 5–10 minutes):

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

Expected result:

```text
pod/web created
pod/web condition met
NAME                                     TYPE
gk3-lab-gke-pool-2-1a2b3c4d-x7kq         ek-standard-8
True	REGULAR	1.33.5-gke.1080000
```

Explanation: Autopilot created a node for the requested `requests` itself — Google decides how many nodes and of what kind, and you pay for the pod's 250m CPU and 512Mi of memory. The kubeconfig came through gcloud, and the token for `kubectl` is issued by the `gke-gcloud-auth-plugin` from your gcloud account — like `aws eks update-kubeconfig`. The cluster is subscribed to the Regular channel and will upgrade itself. Delete the cluster after the experiment: `gcloud container clusters delete lab-gke --region=europe-west3 --quiet`. Your node names and versions will differ.

</details>

## Storage and databases

### Cloud Storage (GCS)

Object storage, the equivalent of S3, with addresses like `gs://bucket/path/file`.
- **Bucket names are globally unique**, as in S3.
- **Bucket location:** region (one region), **dual-region** (two specific regions, synchronous writes), multi-region (`EU`, `US` — several regions of a continent). In S3 a bucket is always in one region and replication is configured separately.
- **Storage classes:** Standard; Nearline (30-day minimum storage, for data read once a month); Coldline (90 days); Archive (365 days). Unlike S3 Glacier, **data in all classes is available immediately**, without waiting for a restore — only reads cost more. Delete an object before its minimum duration and you still pay for the full period.
- **Autoclass** moves objects between classes based on access frequency, like S3 Intelligent-Tiering.
- **Uniform bucket-level access** — permissions only through IAM on the bucket, no ACLs (Access Control Lists — per-object access lists). Always enable it; the equivalent of "Object Ownership: bucket owner enforced" in S3.
- **Public access prevention** — forbids making a bucket public, the equivalent of S3 Block Public Access.
- **Object versioning, lifecycle rules, retention policy with bucket lock** (immutable storage: an object cannot be deleted before its time, even by an admin — for backups and regulatory requirements).
- **Signed URL** — a temporary link to an object without IAM permissions, the equivalent of an S3 presigned URL.
- CLI: `gcloud storage` (new and fast); `gsutil` is the older tool still found in scripts.

### Databases

| Service | What it is | AWS equivalent | When to use |
|---|---|---|---|
| **Cloud SQL** | managed MySQL, PostgreSQL, SQL Server | RDS | a regular relational database for an application |
| **AlloyDB** | PostgreSQL-compatible, faster and pricier than Cloud SQL | Aurora PostgreSQL | heavy Postgres load, mixed analytics |
| **Spanner** | a globally distributed SQL database with horizontal scaling and strong consistency | Aurora DSQL, DynamoDB Global Tables (partly) | finance, global systems where one database does not fit on a server |
| **Firestore** | a serverless document NoSQL database | DynamoDB | mobile and web apps, simple key-document queries |
| **Bigtable** | wide-column NoSQL for huge volumes (compatible with the HBase API) | Keyspaces, DynamoDB | time series, IoT (data from sensors and devices), analytics with millions of writes per second |
| **Memorystore** | managed Redis, Valkey, Memcached | ElastiCache | cache, sessions |
| **BigQuery** | a serverless analytics data warehouse, SQL over petabytes | Redshift, Athena | analytics, reports, log and billing exports |

What gets asked about **Cloud SQL**:
- **High availability (HA)**: a primary instance and a standby in another zone, with disk writes replicated synchronously (regional PD); on failure it switches over in about a minute to the same IP. The equivalent of RDS Multi-AZ. The standby cannot be read.
- **Read replicas** — asynchronous read copies, including in other regions (for DR — Disaster Recovery).
- **Backups and PITR** (Point-In-Time Recovery — restoring to any second within the transaction log retention period).
- **Connecting:** via **private IP** in your VPC (through Private Service Access — peering with the Google network where the database lives) or via the **Cloud SQL Auth Proxy** — a local proxy that encrypts the connection and checks permissions through IAM, without exposing the database to the internet or managing certificates.

### Secrets and keys

- **Secret Manager** — secret storage with versions, IAM permissions per secret and access auditing. The equivalent of AWS Secrets Manager, but without built-in automatic database password rotation — rotation is done through Pub/Sub notifications and your own function.
- **Cloud KMS** (Key Management Service) — encryption keys. All data in GCP is encrypted with Google's keys anyway; **CMEK** (Customer-Managed Encryption Keys) is needed when a regulator requires you to control the key: disable the key and the data becomes inaccessible.

### 🧪 Mini task

**Task.** Create a bucket with uniform access and public access prevention, add a lifecycle rule "delete objects older than 30 days", upload a file and issue a temporary link to it.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

```bash
PROJECT_ID=$(gcloud config get-value project)
B="gs://$PROJECT_ID-lab"
gcloud storage buckets create "$B" --location=europe-west3 --uniform-bucket-level-access --public-access-prevention
echo '{"rule":[{"action":{"type":"Delete"},"condition":{"age":30}}]}' > lifecycle.json
gcloud storage buckets update "$B" --lifecycle-file=lifecycle.json
echo "hello gcs" > hello.txt && gcloud storage cp hello.txt "$B/"
gcloud storage buckets describe "$B" --format="yaml(location, storage_class, uniform_bucket_level_access, public_access_prevention, lifecycle_config)"
```

Expected result:

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

Explanation: `uniform_bucket_level_access` disables object ACLs — access is controlled only by IAM on the bucket. `public_access_prevention: enforced` will not allow granting a role to `allUsers`, even if someone tries. The lifecycle rule deletes objects 30 days after creation — this is how temporary exports and old logs are cleaned up. A temporary link is made with `gcloud storage sign-url "$B/hello.txt" --duration=10m --impersonate-service-account=SA` — a service account must sign the link, so a user needs impersonation. Your bucket name will differ.

</details>

## Observability and audit

### Cloud Logging

- Logs from VMs (via the **Ops Agent** — a log and metric collection agent installed on VMs), GKE, Cloud Run, load balancers and GCP services themselves are collected in Cloud Logging automatically. The equivalent of CloudWatch Logs.
- **Log Router and sinks** — every log entry goes through rules: which **log bucket** to store it in, where to export it (Cloud Storage for cheap long-term storage, BigQuery for SQL analysis, Pub/Sub for sending to external systems like Splunk or Datadog), and what to drop (exclusion filters — a way to save money, since you pay for ingested volume).
- Built-in buckets: **`_Required`** — admin audit logs, kept 400 days for free, cannot be deleted or changed; **`_Default`** — everything else, 30 days by default.
- Queries are written in the **Logging query language**: `resource.type="k8s_container" AND severity>=ERROR AND resource.labels.namespace_name="prod"`.

### Cloud Monitoring

- Metrics for all GCP services, dashboards, **alerting policies** with notification channels (email, Slack, PagerDuty, Pub/Sub).
- **Uptime checks** — external site availability checks from several points around the world.
- **Managed Service for Prometheus** — Prometheus as a service: application metrics from GKE are collected in Prometheus format and queried with PromQL (Prometheus's query language), without storing them yourself. Grafana connects to it like to a regular Prometheus.
- **SLO monitoring** — built-in SLOs (Service Level Objective, a target reliability level such as "99.9% of requests succeed") and alerts on error budget burn rate.
- **Cloud Trace** (distributed request tracing) and **Error Reporting** (grouping exceptions from logs).

### Cloud Audit Logs — who did what

The equivalent of CloudTrail, but split into types:
- **Admin Activity** — configuration and permission changes (created a VM, changed IAM). **Always on and free**, kept 400 days.
- **Data Access** — reads of data and metadata (who read an object in a bucket, who fetched a secret). **Off by default** (except BigQuery) because there are a lot of them and they are paid. Enabled selectively for sensitive services: Secret Manager, Cloud Storage with personal data.
- **System Event** — actions by Google itself (VM live migration, automatic scaling).
- **Policy Denied** — requests rejected by an Organization Policy or VPC Service Controls (a service that restricts data exfiltration from a perimeter of projects).

### 🧪 Mini task

**Task.** Find in the audit logs who created service accounts and firewall rules in the project over the last day.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

```bash
gcloud logging read '
  logName:"cloudaudit.googleapis.com%2Factivity"
  AND (protoPayload.methodName="google.iam.admin.v1.CreateServiceAccount"
       OR protoPayload.methodName:"compute.firewalls.insert")' \
  --freshness=1d --limit=5 \
  --format="table(timestamp.date('%Y-%m-%d %H:%M'), protoPayload.authenticationInfo.principalEmail, protoPayload.methodName, protoPayload.resourceName.basename())"
```

Expected result:

```text
TIMESTAMP         PRINCIPAL_EMAIL      METHOD_NAME                                   RESOURCE_NAME
2026-10-30 14:12  anna@example.com     v1.compute.firewalls.insert                   lab-allow-health-checks
2026-10-30 13:58  anna@example.com     v1.compute.firewalls.insert                   lab-allow-iap-ssh
2026-10-30 13:41  anna@example.com     google.iam.admin.v1.CreateServiceAccount      reader@sandbox-1234567890.iam.gserviceaccount.com
```

Explanation: `cloudaudit.googleapis.com%2Factivity` is the Admin Activity log (`%2F` is the encoded `/` in the log name), which is always on. `principalEmail` is who performed the action: a person or a service account (with impersonation, `serviceAccountDelegationInfo` also shows the original user). `methodName` is which API was called. This answers the classic question "who deleted a resource in production": filter by a `methodName` containing `delete` and by the resource name. To search over a longer period, logs are exported to BigQuery with a sink. Your entries will differ.

</details>

## Service integration and CI/CD

### Pub/Sub and events

- **Pub/Sub** — a managed publisher–subscriber message queue. A publisher writes to a **topic**, a topic can have several **subscriptions**, and each gets its own copy of the messages. One entity replaces the SNS + SQS pair. Subscriptions are **pull** (the consumer fetches messages itself) or **push** (Pub/Sub sends an HTTP request to your endpoint, for example in Cloud Run). An unacknowledged message is redelivered, so the handler must be idempotent (processing twice must not corrupt data). There is a dead-letter topic for messages that cannot be processed.
- **Eventarc** routes events ("an object was created in a bucket", "an audit log entry") to Cloud Run and functions, the equivalent of EventBridge.
- **Cloud Scheduler** (cron as a service), **Cloud Tasks** (a queue of deferred HTTP calls), **Workflows** (step orchestration, the equivalent of Step Functions).

### Artifact Registry, Cloud Build, Cloud Deploy

- **Artifact Registry** — a registry for Docker images and also npm, Maven, Python and apt packages. Image address: `europe-west3-docker.pkg.dev/PROJECT/REPO/IMAGE:TAG`. The old Container Registry (`gcr.io`) was shut down in 2025 — if manifests still reference `gcr.io`, that is a reason to migrate. It can scan images for vulnerabilities and act as a caching proxy for Docker Hub (a remote repository — protects against Docker Hub pull limits).
- **Cloud Build** — builds from a `cloudbuild.yaml` file where every step is a container. Triggered from GitHub, GitLab, Bitbucket. The equivalent of CodeBuild.
- **Cloud Deploy** — release delivery along a chain of environments (dev → stage → prod) with manual approval, canary releases and rollback, for GKE and Cloud Run.

In practice CI often stays in GitHub Actions or GitLab CI and reaches GCP through **Workload Identity Federation** without JSON keys:

```yaml
# .github/workflows/deploy.yml (excerpt)
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

`id-token: write` lets GitHub issue the pipeline an OIDC token; GCP checks that the token was issued to this specific repository (the condition is set in the provider) and issues a temporary token for the `deployer` service account.

### 🧪 Mini task

**Task.** Create a Pub/Sub topic with a pull subscription, publish a message and read it with acknowledgement.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

```bash
gcloud services enable pubsub.googleapis.com
gcloud pubsub topics create lab-orders
gcloud pubsub subscriptions create lab-orders-worker --topic=lab-orders --ack-deadline=30
gcloud pubsub topics publish lab-orders --message='{"order_id": 42}' --attribute=source=shop
gcloud pubsub subscriptions pull lab-orders-worker --auto-ack --limit=1 --format="value(message.data, message.attributes.source)"
```

Expected result:

```text
messageIds:
- '13579246801357924'
{"order_id": 42}	shop
```

Explanation: the message went to the topic and was stored for every subscription created **before** publishing (a subscription does not receive messages sent before it existed). `--auto-ack` acknowledges receipt immediately; without an acknowledgement within `ack-deadline` (30 seconds) the message is delivered again — this is how Pub/Sub guarantees at-least-once delivery. With two subscriptions, each would get its own copy — that is the SNS + SQS replacement in a single entity. Your message ID will differ.

</details>

## Terraform for GCP

- Providers: **`google`** and **`google-beta`** (for features in beta APIs).
- **State backend — `gcs`**: state in a Cloud Storage bucket, **locking is built in** (the S3 backend used to need a DynamoDB table for that). Enable bucket versioning so you can roll back to a previous state.
- **Authentication**: locally — ADC (`gcloud auth application-default login`), in CI — Workload Identity Federation. A handy trick is `impersonate_service_account` in the provider: a person runs Terraform with their own credentials but acts as an SA with the needed roles.
- **APIs must be enabled with the** `google_project_service` resource, otherwise the first `apply` in a new project fails with 403.
- **Three ways to set IAM — the main trap:**
  - `google_project_iam_policy` **replaces the entire** project policy: it removes every binding not in the code, including Google's service bindings. An easy way to cut off access for yourself and for services;
  - `google_project_iam_binding` is **authoritative for one role**: it removes all other members of that role;
  - `google_project_iam_member` **adds one** member without touching others. The safe default.
- **Cloud Foundation Toolkit / terraform-google-modules** — Google's official modules (network, project-factory, kubernetes-engine), the equivalent of terraform-aws-modules.

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

`disable_on_destroy = false` keeps `terraform destroy` from disabling the API in the project (otherwise resources created by other code would break). `auto_create_subnetworks = false` means a custom-mode VPC. The `pods` and `services` secondary ranges are reserved in advance for a VPC-native GKE cluster: a `/14` gives about 260 thousand pod addresses.

### 🧪 Mini task

**Task.** Describe in Terraform a state bucket and the VPC from the example above, validate the code without applying and look at the plan.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

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

Expected result:

```text
Success! The configuration is valid.

  # google_compute_network.main will be created
  # google_storage_bucket.tfstate will be created
Plan: 2 to add, 0 to change, 0 to destroy.
```

Explanation: `terraform init` downloaded the `google` provider, `validate` checked the syntax, and `plan` called the API with ADC credentials — which is why you first need `application-default login`, not just `gcloud auth login`. The versioned bucket is then referenced in a `backend "gcs"` block and the state is moved there with `terraform init -migrate-state`. We do not `apply` here — this only checks the code.

</details>

## Cost (FinOps)

FinOps is the practice of managing cloud spending: seeing who spends what on what, and lowering the bill without losing reliability.

- **Per-second billing** for VMs (one-minute minimum).
- **Sustained use discounts (SUD)** — **automatic**: if an N1 or N2 VM runs more than a quarter of the month, the price gradually drops, to 20–30% off for a full month. Nothing to buy. AWS has nothing like it. They do not apply to E2 and the newer families (N4, C3, C4).
- **Committed use discounts (CUD)** — commit to using resources for 1 or 3 years and get up to 55–70% off:
  - **resource-based** — for specific vCPUs and memory in a region, like EC2 Reserved Instances;
  - **spend-based (flexible)** — for an hourly spend amount across services, like Savings Plans.
- **Spot VMs** — up to 91% off for interruptible workloads.
- **Paid egress** — to the internet and between regions, as in AWS. **Network Service Tiers**: **Premium** (the default: traffic travels over Google's network to the point nearest the user; required for the global load balancer) and **Standard** (cheaper: traffic leaves for the public internet right from the region).
- **Labels** — key-value pairs on resources (`env=prod`, `team=payments`), the equivalent of AWS tags; used to split the bill.
- **Budgets and alerts** — budgets with notifications at 50%, 90%, 100%. **Important: a budget does not stop spending**, it only notifies. To actually "switch off" a project on overspend, the budget sends a message to Pub/Sub and a function unlinks the project from the billing account.
- **Billing export to BigQuery** — detailed spend by service, project and label; dashboards are built on it in Looker Studio.
- **Recommender** — recommendations: downsize VMs, delete idle disks and IPs, remove excess IAM roles.

### 🧪 Mini task

**Task.** Create a $10 budget with notifications at 50% and 90% for your project, and find unused resources you are paying for: disks without a VM and reserved external IPs that are not attached.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

```bash
gcloud services enable billingbudgets.googleapis.com
BILLING=$(gcloud billing projects describe "$(gcloud config get-value project)" --format="value(billingAccountName.basename())")
gcloud billing budgets create --billing-account="$BILLING" --display-name="lab-10usd" \
  --budget-amount=10USD --threshold-rule=percent=0.5 --threshold-rule=percent=0.9
gcloud billing budgets list --billing-account="$BILLING" --format="table(displayName, amount.specifiedAmount.units, thresholdRules[].thresholdPercent.list())"
gcloud compute disks list --filter="-users:*" --format="table(name, zone.basename(), sizeGb)"
gcloud compute addresses list --filter="status=RESERVED" --format="table(name, region.basename(), address)"
```

Expected result:

```text
DISPLAY_NAME  UNITS  THRESHOLD_PERCENT
lab-10usd     10     0.5,0.9
Listed 0 items.
Listed 0 items.
```

Explanation: the budget is created on the billing account and by default covers all its projects (a project filter is set with `--filter-projects`). Notifications go by email to the billing account admins, but **the budget will not stop spending**. `-users:*` in the filter means "the disk has no users field", i.e. it is not attached to any VM, yet its space is billed. An external IP in `RESERVED` status (reserved but not attached) also costs money. In a clean project both lists are empty. For labs the simplest cleanup is deleting the whole project: `gcloud projects delete PROJECT_ID`.

</details>

## gcloud CLI in daily work

```bash
gcloud config list                              # current account, project, region
gcloud config configurations create prod       # a named settings set, like an AWS CLI profile
gcloud config set project shop-prod
gcloud config set compute/region europe-west3
gcloud config configurations activate dev      # switch to another set
CLOUDSDK_ACTIVE_CONFIG_NAME=prod gcloud compute instances list   # one-off for a single command

gcloud compute instances list --filter="labels.env=prod AND status=RUNNING" \
  --format="table(name, zone.basename(), machineType.basename(), networkInterfaces[0].networkIP)"
gcloud compute ssh vm1 --zone=europe-west3-a --tunnel-through-iap
gcloud compute instances add-metadata vm1 --metadata=enable-oslogin=TRUE
gcloud asset search-all-resources --scope=projects/shop-prod --query="labels.env:prod"
gcloud logging read 'severity>=ERROR' --freshness=1h --limit=20
gcloud projects get-iam-policy shop-prod --flatten="bindings[].members" \
  --filter="bindings.members:deployer@" --format="value(bindings.role)"
```

- **`--filter`** filters on the gcloud side (like AWS CLI `--query` for selection), **`--format`** sets what and how to print: `table(...)`, `value(...)` (for scripts, no headers), `json`, `yaml`, `csv`. `--flatten` expands nested lists into separate rows — handy for finding which roles a particular member has.
- **`gcloud asset search-all-resources`** searches resources across an entire organization or project with one command (Cloud Asset Inventory). The equivalent of AWS Resource Explorer.
- **Cloud Shell** — a free VM with a browser terminal where gcloud, kubectl and terraform are already installed and you are signed in. Handy when you cannot install tools on a work computer.

### 🧪 Mini task

**Task.** Create two gcloud configurations — `lab` and `prod` — with different regions, and show which one is active and which region each has.

<details class="qa"><summary>Solution: command and expected result</summary>

Copy the whole block into a terminal:

```bash
gcloud config configurations create lab --no-activate
gcloud config configurations create prod --no-activate
gcloud config set compute/region europe-west3 --configuration=lab
gcloud config set compute/region us-central1 --configuration=prod
gcloud config configurations activate lab
gcloud config configurations list --format="table(name, is_active, properties.compute.region)"
CLOUDSDK_ACTIVE_CONFIG_NAME=prod gcloud config get-value compute/region
```

Expected result:

```text
NAME     IS_ACTIVE  REGION
default  False
lab      True       europe-west3
prod     False      us-central1
Activated [lab].
us-central1
```

Explanation: configurations are named settings sets (account, project, region), like profiles in `~/.aws/config`. You switch with `activate`, or with the `CLOUDSDK_ACTIVE_CONFIG_NAME` variable for a single command — so you do not mix up production and a test environment in neighbouring terminal tabs. The `Activated [lab].` line goes to stderr, so it may appear elsewhere in the output. The account and project in each configuration are set the same way: `gcloud config set account ... --configuration=prod`.

</details>

## Interview questions

**How does networking in GCP differ from AWS?**
A GCP VPC is global, subnets are regional and span all zones of a region. There are no separate Internet Gateways or public subnets: internet access is through an external IP on the VM or Cloud NAT. Instead of Security Groups there are VPC-level firewall rules applied by network tags or service accounts, with priorities and deny rules. The global HTTP load balancer gives one anycast IP for the whole world.

**What is a project in GCP and why have several?**
The main unit of isolation: resources, enabled APIs, quotas, permissions and the billing link. The equivalent of an AWS account, but created in seconds. Usually separate projects per environment (dev, stage, prod) and team, grouped into folders; permissions and Organization Policies are inherited top down.

**How do you give an application permissions in GCP without keys?**
On VMs, Cloud Run and Cloud Build — attach a service account with minimal roles to the resource; tokens come from the metadata server. In GKE — Workload Identity Federation for GKE, linking a Kubernetes ServiceAccount to IAM. For external systems (GitHub Actions, GitLab, AWS) — Workload Identity Federation via OIDC. Service account JSON keys are a last resort and are usually forbidden by an Organization Policy.

**Why is the Compute Engine default service account dangerous?**
VMs get it automatically unless another is specified, and in older projects it has the Editor role on the whole project. A compromised VM gets the right to change almost everything. The right way is a separate SA per application with minimal roles, plus an Organization Policy that disables automatic role grants.

**What is the difference between google_project_iam_policy, _binding and _member in Terraform?**
`_policy` replaces the project's entire IAM policy and removes everything not in the code, including Google's service bindings. `_binding` fully manages one role and removes its other members. `_member` adds one member and touches nothing else — the safe default. Do not mix `_binding` and `_member` for the same role — they will keep overwriting each other.

**The load balancer returns 502 but the app on the VM works. What do you check?**
The backend service's health check: checks come from Google addresses `35.191.0.0/16` and `130.211.0.0/22`, and the firewall must allow them to the check port. Then — that the health check path and port match what the app listens on, and that the group's named port matches the backend service's port. `gcloud compute backend-services get-health` shows backend status.

**GKE Autopilot or Standard?**
Autopilot — Google manages the nodes, you pay for pod resources, less operational work and secure defaults, but no privileged pods or node access. Standard — your own node pools and full control, needed for GPUs, special kernels and your own node agents. For new typical clusters, Autopilot is the default choice.

**How do GKE upgrades work?**
The cluster is subscribed to a release channel (Rapid, Regular, Stable, Extended), and GKE upgrades the control plane and nodes itself. You control this with maintenance windows (when to upgrade) and maintenance exclusions (when not to). For production — the Regular or Stable channel, a regional cluster, PodDisruptionBudgets for applications and surge upgrades of node pools.

**How does Cloud Run differ from Lambda?**
Cloud Run runs any container rather than a function in a specific runtime, and one instance handles many requests at once (default concurrency 80), whereas Lambda handles one request per instance. It scales to zero and you pay for request processing time. There are jobs for batch tasks. The closest AWS equivalent is App Runner.

**How do Cloud Storage classes differ from S3 Glacier?**
Nearline, Coldline and Archive differ in minimum storage duration (30, 90, 365 days) and read price, but data in every class is available immediately, without hours-long restores as in Glacier Flexible Retrieval and Deep Archive. Early deletion is billed for the full minimum duration.

**What is Shared VPC and why use it?**
The network lives in one host project, while service projects create resources in its subnets. The network team centrally manages addressing, firewall, VPN and Interconnect, while application teams manage their own resources and bills. The standard setup for companies with multiple teams.

**How do you save money in GCP?**
Rightsizing from Recommender, CUDs for steady load (plus automatic SUDs on older families), Spot VMs for CI and stateless work, Autopilot instead of idle nodes, Cloud Run with scale-to-zero, lifecycle and Autoclass in Cloud Storage, exclusion filters in Cloud Logging, deleting unused disks and IPs, labels and billing export to BigQuery. And remember that a budget only notifies, it does not stop spending.

**Who deleted a resource in production?**
Cloud Audit Logs, the Admin Activity log — always on and kept for 400 days: filter by `protoPayload.methodName` (for example, `delete`) and resource name; the `principalEmail` field shows who did it. For data reads (who downloaded a file) you need Data Access logs, which are off by default.

**You have AWS experience, but we use GCP. How quickly will you get up to speed?**
The concepts are the same: isolation through accounts and projects, least-privilege IAM, VPCs, load balancers, managed Kubernetes, IaC with Terraform. What differs are the names and a few details — the global VPC, firewall rules with tags, IAM on the resource, projects instead of accounts. Terraform, Kubernetes, monitoring and CI carry over almost unchanged, so you can become productive on GCP within a couple of weeks.
