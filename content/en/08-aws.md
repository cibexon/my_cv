# AWS

The main AWS services asked about in DevOps/SRE interviews, in depth: global infrastructure and account organisation, IAM, VPC networking, compute, EKS, storage and databases, observability, cost and resilience. Each topic: what it is, why, how it works, examples and typical problems.

## Global infrastructure

- A **region** (for example `us-east-2` — Ohio, `eu-central-1` — Frankfurt) is a geographic area with several isolated data centres. Regions are independent: resources are created in a specific region, and data doesn't leave it without explicit configuration. Prices and the set of services differ between regions.
- An **Availability Zone** (AZ: `us-east-2a`, `us-east-2b`) is one or more data centres within a region with independent power, cooling and networking. The zones of one region are connected by fast links (latency of about a millisecond) but are physically separated so that an incident (a fire, a power outage) in one doesn't affect the others.
- **Edge locations** are CloudFront (CDN) and Route 53 points of presence all over the world, closer to users.

**Why you need to know this:** high availability is built like this — **several AZs** in one region (multi-AZ) protect against a data centre failure; **several regions** (multi-region) — against the failure of a whole region and for disaster recovery. Resources tied to one AZ (an EBS disk, a subnet, an instance) are a single point of failure unless duplicated.

### 🧪 Mini task

**Task.** List the availability zones of `us-east-2` together with their Zone IDs and find out whether the region has Local Zones.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws ec2 describe-availability-zones --region us-east-2 --all-availability-zones \
  --query 'AvailabilityZones[].[ZoneName, ZoneId, ZoneType, OptInStatus]' --output text
```

Expected output:

```text
us-east-2a	use2-az1	availability-zone	opt-in-not-required
us-east-2b	use2-az2	availability-zone	opt-in-not-required
us-east-2c	use2-az3	availability-zone	opt-in-not-required
```

Walkthrough: us-east-2 has three ordinary AZs — physically separate data centres with their own power and network. The name `us-east-2a` may point to a different physical zone in each account (AWS shuffles them so that everyone doesn't pile into "a"), while the Zone ID `use2-az1` is the same everywhere. That's why for resources shared between accounts (VPC sharing — one VPC whose subnets are used by several accounts; PrivateLink — private access to another account's service; latency between services) you compare the Zone ID. Local Zones (small AWS sites in big cities closer to users) and Wavelength (sites inside 5G carriers' networks) would appear in the output with a different `ZoneType`. Configured AWS credentials are required. The identifiers and values will be your own.

</details>

## Account organisation

An AWS account is the isolation boundary for resources, permissions and billing. Good practice is **many accounts**, not everything in one:
- separate accounts for prod, stage, dev — a mistake in dev physically cannot affect prod;
- separate accounts for security (CloudTrail logs — the journal of all API calls in the account: who did what and when; auditing), shared services (network, CI), sandboxes.

**AWS Organizations** combines accounts: a single bill, grouping into organisational units (an OU, Organizational Unit — a "folder" of accounts, for example `Prod` or `Sandbox`), policies on top.

**SCPs (Service Control Policies)** are policies at the Organizations level that **cap the maximum** of what can be done in the accounts of an OU, even by that account's administrator. For example: forbid working outside allowed regions, forbid disabling CloudTrail, forbid deleting backups. An SCP doesn't grant anything — it only restricts.

**IAM Identity Center** (formerly AWS SSO) — a single sign-on for people into all accounts via the corporate directory (Google Workspace, Entra ID — formerly Azure AD, Okta — corporate identity services) with a choice of role. No IAM users with passwords and keys for people: a departing employee is disabled in one place. On a machine — `aws configure sso`, then `aws sso login`.

**Control Tower** is a ready-made "landing zone": Organizations, baseline accounts, SCPs and logging according to best practices.

### 🧪 Mini task

**Task.** Find out which role and account you're currently working as, which profiles are configured in the CLI and (if you have rights in the management account) which accounts belong to the organisation.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws sts get-caller-identity --query '[Account, Arn]' --output text
aws configure list-profiles
aws organizations list-accounts --query 'Accounts[].[Name, Id, Status]' --output text
```

Expected output:

```text
029940625330	arn:aws:sts::029940625330:assumed-role/AWSReservedSSO_AdministratorAccess_1a2b3c4d5e6f7a8b/alex
default
prod-admin
dev-admin
Management	111122223333	ACTIVE
Prod	029940625330	ACTIVE
Dev	444455556666	ACTIVE
```

Breakdown: `get-caller-identity` is the first command for any access problem: it shows who AWS thinks you are. `assumed-role/AWSReservedSSO_…` means you logged in through IAM Identity Center (SSO) with temporary credentials, not with user keys. The accepted layout: separate accounts per environment and function (prod, dev, security, logs), grouped in Organizations with SCP guardrails on top. The account list is available only from the management account or a delegated administrator. You need configured AWS credentials. Your IDs and values will differ.

</details>

## IAM — access management

### Core concepts
- **Principal** — who makes the request: an IAM user, an IAM role, an AWS service, a federated user.
- **IAM user** — a permanent account with a password and/or access keys. In modern practice it isn't used for people (SSO instead), and people try to avoid it for machines too.
- **An IAM role** — a set of permissions **without permanent credentials**. It's "assumed", yielding **temporary** credentials via STS (Security Token Service — the service that issues temporary keys; valid from 15 minutes to 12 hours). Who may assume the role is described by the role's **trust policy**. Roles are used by: EC2 (instance profile), Lambda (execution role), EKS pods (IRSA, Pod Identity), CI (OIDC — the CI system presents AWS a signed token instead of stored keys), people via SSO, cross-account access.
- **Policy** — a JSON document with permissions.

### How a policy is structured
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadAppBucket",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:ListBucket"],
      "Resource": [
        "arn:aws:s3:::my-app-bucket",
        "arn:aws:s3:::my-app-bucket/*"
      ],
      "Condition": {
        "StringEquals": { "aws:PrincipalTag/team": "payments" }
      }
    }
  ]
}
```
- `Effect` — `Allow` or `Deny`;
- `Action` — which API calls (`service:Operation`);
- `Resource` — on which resources, by ARN (Amazon Resource Name — a unique resource identifier);
- `Condition` — additional conditions (tags, IP, MFA — sign-in with a second factor, VPC endpoint).

Note the two ARNs for S3: `ListBucket` applies to the **bucket**, while `GetObject` applies to the **objects** (`/*`). A common mistake is specifying only one.

### Types of policies
- **Identity-based** — attached to a user, group or role: "what this principal can do".
- **Resource-based** — attached to a resource: an S3 bucket policy, a KMS key policy, an SQS queue policy, a role's trust policy. "Who can access this resource". They make it possible to grant access to another account.
- **Permission boundary** — the maximum permissions a role or user can have regardless of their policies. Used when teams are allowed to create roles themselves, but within limits.
- **SCP** — the maximum permissions at the account or OU level.
- **Session policy** — a restriction applied when assuming a role.

### How AWS makes a decision
1. By default everything is **denied** (implicit deny).
2. If any policy contains an **explicit Deny** for the request — it's **denied**. An explicit Deny beats any Allow.
3. At least one **Allow** in an identity-based or resource-based policy is needed.
4. The Allow must fit within all the limiters: SCPs, the permission boundary, the session policy.

**Cross-account access** requires permission **on both sides**: in account A the role must have the right to perform the action, and the resource in account B must trust A (a resource policy, or the trust policy of a role in B that A assumes).

### Best practices
- No access keys for people — SSO only. The root account — MFA, no keys, used only for rare tasks.
- **Least privilege** — the minimum necessary permissions. IAM Access Analyzer generates a policy from the actions actually used, based on CloudTrail.
- Roles and temporary credentials wherever possible.
- CloudTrail enabled in all accounts and regions, with logs in a separate security account.

### 🧪 Mini task

**Task.** Write a policy: reading and writing objects under `logs/` in `my-bucket` is allowed, but deletion is explicitly denied. Use the policy simulator to check the decisions for `GetObject` and `DeleteObject` on `logs/`, and for `GetObject` on another prefix.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {"Effect": "Allow", "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
     "Resource": "arn:aws:s3:::my-bucket/logs/*"},
    {"Effect": "Deny", "Action": "s3:DeleteObject", "Resource": "*"}
  ]
}
EOF
for res in logs/app.log secret/key.pem; do
  aws iam simulate-custom-policy --policy-input-list file://policy.json \
    --action-names s3:GetObject s3:DeleteObject --resource-arns "arn:aws:s3:::my-bucket/$res" \
    --query 'EvaluationResults[].[EvalResourceName, EvalActionName, EvalDecision]' --output text
done
```

Expected output:

```text
arn:aws:s3:::my-bucket/logs/app.log	s3:GetObject	allowed
arn:aws:s3:::my-bucket/logs/app.log	s3:DeleteObject	explicitDeny
arn:aws:s3:::my-bucket/secret/key.pem	s3:GetObject	implicitDeny
arn:aws:s3:::my-bucket/secret/key.pem	s3:DeleteObject	explicitDeny
```

Breakdown: IAM decision logic: everything is denied by default (`implicitDeny`), `Allow` permits, and an explicit `Deny` beats any `Allow` — `DeleteObject` is denied although it's in the allow statement. Nothing allows `secret/`, hence the implicit deny. The simulator checks a policy without touching anything — that's how policies are debugged before being applied, while `simulate-principal-policy` checks a real user or role with all its policies. AWS credentials are required (the simulator is free).

</details>

## Networking: VPC

### How a VPC is structured
A **VPC (Virtual Private Cloud)** is your isolated network in a region with a chosen address range (a CIDR from /16 to /28, for example `10.0.0.0/16`).

A **subnet** is a part of the VPC range **tied to one AZ**. That's why subnets are created in several zones for resilience.

**A public and a private subnet differ only by a route:**
- **Public subnet** — its route table has the route `0.0.0.0/0 → Internet Gateway`. Resources with a public IP are reachable from the internet and can reach the internet. Load balancers, NAT Gateways and bastions are placed there.
- **Private subnet** — the route is `0.0.0.0/0 → NAT Gateway`. Resources can go out to the internet (download packages, call APIs), but **nobody outside can connect to them**. Applications, EKS nodes and databases are placed there.
- Sometimes **isolated** subnets without any internet access are created — for databases.

**Internet Gateway (IGW)** — the VPC's gateway to the internet. One per VPC, scales automatically, free.

**NAT Gateway** — gives private subnets **outbound** internet access via SNAT. It lives in a public subnet. It's tied to one AZ: for resilience you put **one in each zone** (otherwise, if the zone with the NAT fails, private subnets in other zones lose internet access). It's paid for: per hour **and per gigabyte** of traffic — a frequent unexpected line on the bill.

**Route table** — a table of routes associated with subnets. There's always a `<VPC CIDR> → local` route — connectivity inside the VPC.

### Security Group and Network ACL
| | Security Group | Network ACL |
|---|---|---|
| Level | A network interface (instance, pod, RDS) | A subnet |
| State | **Stateful**: the reply to an allowed request is let through automatically | **Stateless**: return traffic has to be allowed by a separate rule |
| Rules | Allow rules only | Allow and deny rules |
| Order | All rules are evaluated together | By number, until the first match |
| References | Another SG can be specified as a source | CIDRs only |

**Referencing an SG as a source** is a key feature: a rule "allow port 5432 from the application's Security Group" instead of a list of IPs. The application scales and changes addresses — the rule keeps working.

**The NACL trap**: you allowed incoming port 443 but forgot the outgoing traffic to **ephemeral ports** (1024–65535) on which replies go back to clients — connections don't work. In practice NACLs are usually left at the default (everything allowed), and filtering is done with Security Groups.

### Connecting a VPC to other networks
- **VPC Peering** — a direct connection between two VPCs (including across accounts and regions). **Not transitive**: if A is connected to B and B to C, A doesn't see C. With dozens of VPCs it turns into a spider web.
- **Transit Gateway** — a central router (a "hub") to which VPCs, VPNs and Direct Connect attach. It solves peering's scaling problem.
- **VPC Endpoints** — access to AWS services **without going out to the internet** (traffic doesn't go through NAT):
  - **Gateway endpoints** — for S3 and DynamoDB, **free**, added as a route in the route table;
  - **Interface endpoints** (PrivateLink) — for other services (ECR, SSM, Secrets Manager, STS): a network interface in your subnet, paid.
- **PrivateLink** — publish your own service to other VPCs and accounts without peering.
- **Site-to-Site VPN** — an encrypted tunnel over the internet to an office or data centre. **Direct Connect** — a dedicated physical link (stable latency, high bandwidth).

**CIDRs at design time** must not overlap between VPCs or with the office network — otherwise peering, Transit Gateway and VPN can't be set up.

### Route 53 — DNS
- **Hosted zones** — public (a zone on the internet) and private (visible only from associated VPCs: internal service names).
- **Alias record** — a special AWS record pointing to an AWS resource (a load balancer, CloudFront, S3). It works at the domain apex (where a CNAME is forbidden), queries to it are free, and it automatically follows the resource's IP changes.
- **Routing policies:** simple; **weighted** (a percentage of traffic to each option — for canary releases and migrations); **latency** (to the region with the lowest latency for the user); **failover** (to a standby resource when the primary's health check fails); geolocation.
- **Health checks** — Route 53 checks endpoint availability itself and excludes unavailable ones.

### Load balancers (Elastic Load Balancing)
- **ALB (Application Load Balancer)** — L7, HTTP and HTTPS: routing by domain, path, headers; TLS termination with a certificate from ACM; WAF integration; target groups of instances, IPs (EKS pods) or Lambda; authentication via OIDC.
- **NLB (Network Load Balancer)** — L4, TCP, UDP and TLS: millions of requests per second, minimal latency, a **static IP** in each zone (can be added to partners' allowlists), preserves the client IP. For non-HTTP protocols and gRPC without L7 logic.
- **GWLB** (Gateway Load Balancer) — for inserting third-party virtual firewalls (Palo Alto, Fortinet): all VPC traffic is transparently run through a group of such appliances.

### CloudFront, WAF, Shield
**CloudFront** — a CDN: caches content at edge locations closer to users, reduces load on the origin (S3, ALB), terminates TLS. **WAF** — HTTP request filtering (SQL injections, bots, rate limiting, geo-blocking) on ALB, CloudFront or API Gateway (the managed AWS service for publishing HTTP APIs, often in front of Lambda). **Shield** — DDoS protection (a distributed attack in which thousands of machines flood a service with requests; Standard is on for everyone for free, Advanced is paid and comes with the AWS response team).

### 🧪 Mini task

**Task.** For each subnet in your VPC determine whether it's public or private: check whether its route table has a `0.0.0.0/0` route via an Internet Gateway or via a NAT Gateway.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
vpc=$(aws ec2 describe-vpcs --filters Name=tag:Name,Values=main --query 'Vpcs[0].VpcId' --output text)
for subnet in $(aws ec2 describe-subnets --filters Name=vpc-id,Values=$vpc --query 'Subnets[].SubnetId' --output text); do
  target=$(aws ec2 describe-route-tables --filters Name=association.subnet-id,Values=$subnet \
    --query 'RouteTables[0].Routes[?DestinationCidrBlock==`0.0.0.0/0`] | [0].[GatewayId, NatGatewayId]' --output text)
  case "$target" in
    igw-*) echo "$subnet public  ($target)" ;;
    *nat-*) echo "$subnet private (via ${target##*	})" ;;
    *) echo "$subnet isolated / main route table" ;;
  esac
done
```

Expected output:

```text
subnet-0a1b2c3d4e5f60001 public  (igw-0f1e2d3c4b5a69788	None)
subnet-0a1b2c3d4e5f60002 private (via nat-0123456789abcdef0)
subnet-0a1b2c3d4e5f60003 isolated / main route table
```

Breakdown: whether a subnet is public in AWS is decided only by its route: `0.0.0.0/0 → igw-…` is public (instances also need a public IP), `→ nat-…` is private with internet egress, no default route is isolated. A subnet without an explicit association uses the VPC's main route table — that's the last branch of the script, and such a subnet deserves a separate check. The typical layout: ALB and NAT in public subnets, apps in private ones, databases in isolated ones. You need configured AWS credentials. Your IDs and values will differ.

</details>

## Compute

### EC2
Virtual machines.
- **Instance families:** `t` — burstable, cheap, accumulating CPU credits (when they run out performance drops — a trap for a constant load); `m` — balanced; `c` — compute-optimised; `r` — memory-optimised; `g`, `p` — with GPUs; `i` — with fast local NVMe disks. The letter `g` in the name (`m7g`, `c7g`) means **Graviton** (ARM) processors: about 20% cheaper with the same or better performance, but images need to be built for arm64.
- **Pricing models:**
  - **On-Demand** — hourly billing without commitments;
  - **Savings Plans / Reserved Instances** — a discount of up to ~70% for a commitment to use a certain amount for 1–3 years;
  - **Spot** — spare capacity at a discount of up to 90%, but AWS can **take the instance back with a 2-minute warning**. Suitable for stateless workloads, CI runners, batch processing, Kubernetes nodes managed by Karpenter.
- **AMI** — the disk image to launch from. **User data** — a script run on first boot. **Launch template** — a template of launch parameters.
- **Auto Scaling Group** — maintains the required number of instances, replaces unhealthy ones, scales on metrics, spreads across zones.
- **Instance Metadata Service (IMDS)** — `http://169.254.169.254`: the instance learns its parameters and **its role's temporary credentials**. Use **IMDSv2** (it requires a session token) — it protects against credential theft via SSRF vulnerabilities (Server-Side Request Forgery: an attacker makes the application itself send an HTTP request to an address they choose — for example, `169.254.169.254` — and return the response; this is how Capital One's data leaked in 2019).

### Storage for EC2
- **EBS** — a network block disk, **tied to one AZ**. Types: **gp3** (general-purpose SSD, IOPS — read/write operations per second — and throughput are configured independently of size — cheaper and more flexible than gp2), io2 (guaranteed high IOPS for databases), st1/sc1 (HDD for large sequential data: logs, archives). Snapshots are stored in S3 and are incremental (each subsequent one stores only the changed blocks); a disk is moved to another zone or region via a snapshot.
- **Instance store** — the server's local disks: very fast, but **the data is lost** when the instance stops.
- **EFS** — a network filesystem (NFS) accessible from many instances and **all zones** at once. For shared files, ReadWriteMany in Kubernetes.

### Serverless and containers
- **Lambda** — functions triggered by an event (HTTP via API Gateway, an SQS message, a file in S3, a schedule). You pay for execution time, scaling is automatic. Limitations: up to 15 minutes per invocation, **cold start** (the latency of the first invocation when a new instance starts), a per-account concurrent execution limit.
- **ECS** — AWS's own container orchestrator, simpler than Kubernetes. Runs on EC2 or **Fargate** (no server management: you pay for the task's CPU and memory).
- **EKS** — managed Kubernetes.

### EKS, ECS or Lambda: how to choose
| | Lambda | ECS (on Fargate or EC2) | EKS |
|---|---|---|---|
| What you run | a function on an event | a container as a task or service | containers in Kubernetes |
| Management | almost nothing: AWS scales by itself | services, task definitions, autoscaling | the cluster, add-ons, version upgrades once a year, nodes (or Karpenter) |
| Limitations | up to 15 minutes per invocation, up to 10 GB of memory, cold start, a concurrency limit | AWS only, fewer ready-made tools than in Kubernetes | complexity: networking, IAM wiring, upgrades, a team is needed |
| Billing | per request and execution time (down to milliseconds) | Fargate — for the tasks' CPU and memory; EC2 — for the instances | $0.10 per hour per cluster + nodes; more expensive on outdated versions (extended support) |
| Portability | low (the code is tied to AWS events) | low (ECS is AWS-only) | high: the same manifests and Helm charts in GKE, AKS, on-prem |
| Fits | events, irregular load, glue between services | a team without Kubernetes experience, a dozen services | many services, multi-cloud, the ecosystem is needed (ArgoCD, operators, service mesh) |

Arguments that sound convincing in an interview:
- **Lambda** pays off under uneven load: 0 requests at night = 0 cost. Under constant high load (hundreds of requests per second around the clock) a container comes out cheaper. Traps: cold start for synchronous APIs with strict latency (fixed by provisioned concurrency), DB connections (RDS Proxy), hard local debugging.
- **ECS** is "Kubernetes without Kubernetes": no cluster that needs upgrading, no add-ons. Deep integration with ALB, task IAM roles, CloudWatch. The downside — AWS only, fewer tools around it (no equivalent of the Helm ecosystem, operators, ArgoCD).
- **EKS** — when portability and the ecosystem are needed: operators for Kafka and PostgreSQL, GitOps, service mesh, one approach across several clouds or on-prem. The price is a team that understands Kubernetes: upgrades once a year are mandatory (each minor version is supported for about 14 months in standard support), networking and IAM are more complex.
- A mixed setup is normal: the main application in EKS or ECS, S3 event processing and cron jobs in Lambda.

An example line of reasoning: "A startup, 5 services, a team of two developers without DevOps" → ECS on Fargate (or Lambda for an API with uneven load). "60 microservices, three teams, some customers require on-prem" → EKS: the same charts will go to the customer's site.

### 🧪 Mini task

**Task.** Compare three instance types: `t3.medium`, `m7i.large` and `m7g.large` — architecture, vCPU, memory and whether they use the burstable CPU credit model.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws ec2 describe-instance-types --instance-types t3.medium m7i.large m7g.large \
  --query 'InstanceTypes[].[InstanceType, ProcessorInfo.SupportedArchitectures[0], VCpuInfo.DefaultVCpus, MemoryInfo.SizeInMiB, BurstablePerformanceSupported]' \
  --output text | sort
```

Expected output:

```text
m7g.large	arm64	2	8192	False
m7i.large	x86_64	2	8192	False
t3.medium	x86_64	2	4096	True
```

Breakdown: `t3` is burstable: baseline performance is below 100%, and above it you run on accumulated CPU credits. When they run out, the CPU drops to the baseline (or, in unlimited mode, you get a bill). Steady loads use `m`/`c`. `m7g` is Graviton (ARM): usually about 20% cheaper than `m7i` with the same or better performance, but images and binaries must be built for `arm64` — multi-arch Docker images solve that. The letters: m — general purpose, c — compute, r — memory, g — Graviton, i — Intel. You need configured AWS credentials. Your IDs and values will differ.

</details>

## EKS — Kubernetes on AWS

**What AWS does:** manages the control plane (apiserver, etcd, scheduler) across several zones, upgrades it, backs up etcd.
**What you do:** nodes, add-ons, version upgrades, networking, permissions.

### Nodes
- **Managed node groups** — groups of EC2 instances that AWS helps manage (AMI updates with drain).
- **Karpenter** — node autoscaling: it looks at Pending pods, **picks suitable instance types itself** (including Spot and Graviton), launches them quickly, and consolidates and removes underused nodes (consolidation). More flexible and faster than Cluster Autoscaler.
- **Fargate** — pods without nodes: each pod in its own micro-VM. No DaemonSets and no access to the node.
- **EKS Auto Mode** — AWS manages both the nodes and the core add-ons.

### Add-ons you should know
- **VPC CNI** — every pod gets a **real IP from the VPC subnet**. The upside: pods are directly reachable in the VPC, and an ALB can send traffic straight to pods. The downside: **IP addresses in subnets run out quickly**, and the number of pods per node is limited by the instance type's number of network interfaces and IPs. Solutions: large subnets, **prefix delegation** (giving a node /28 blocks instead of individual IPs), an additional CIDR for pods (`100.64.0.0/10`).
- **CoreDNS**, **kube-proxy**.
- **EBS CSI driver** — PersistentVolumes on EBS; **EFS CSI driver** — on EFS.
- **AWS Load Balancer Controller** — creates an **ALB** for Ingress and an **NLB** for Services of type LoadBalancer.
- **ExternalDNS** — automatically creates Route 53 records for Ingresses and Services.
- **cert-manager** — TLS certificates (Let's Encrypt — a free automated certificate authority) inside the cluster.
- **External Secrets Operator** — syncs secrets from Secrets Manager and SSM into Kubernetes Secrets.

### Access
- **People and CI → the cluster:** IAM roles are mapped to Kubernetes permissions via **access entries** (previously — the `aws-auth` ConfigMap, which was easy to break and lose access to the cluster).
- **Pods → AWS:** **IRSA** (IAM Roles for Service Accounts — the cluster publishes an OIDC provider, and the ServiceAccount is annotated with the role ARN) or **EKS Pod Identity** (simpler: the association is created via the EKS API, without per-cluster OIDC setup). A pod gets the permissions **of its own role only**. Without this you'd have to give permissions to the node role — and **all** pods on the node would get them.

### Upgrading EKS
Kubernetes versions in EKS are supported for about 14 months (after that — paid extended support). The order:
1. check for deprecated APIs in manifests (`pluto`, `kubent`) and add-on compatibility;
2. upgrade the control plane (one minor version at a time);
3. upgrade the add-ons;
4. upgrade the nodes (a new AMI: a managed node group does a rolling update, Karpenter — via drift), respecting PDBs.

### 🧪 Mini task

**Task.** Find out your EKS cluster's version, whether the API is reachable from outside and the authentication mode, set up `kubeconfig` and check who Kubernetes thinks you are.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws eks describe-cluster --name main --region us-east-2 \
  --query 'cluster.[version, resourcesVpcConfig.endpointPublicAccess, accessConfig.authenticationMode]' --output text
aws eks update-kubeconfig --name main --region us-east-2 --alias main > /dev/null
kubectl auth whoami -o jsonpath='{.status.userInfo.username}{"\n"}'
kubectl get nodes -o custom-columns=NODE:.metadata.name,VERSION:.status.nodeInfo.kubeletVersion --no-headers | head -2
```

Expected output:

```text
1.33	True	API_AND_CONFIG_MAP
arn:aws:sts::029940625330:assumed-role/AWSReservedSSO_AdministratorAccess_1a2b3c4d5e6f7a8b/alex
ip-10-0-12-34.us-east-2.compute.internal   v1.33.4-eks-1a2b3c4
ip-10-0-45-67.us-east-2.compute.internal   v1.33.4-eks-1a2b3c4
```

Breakdown: `update-kubeconfig` writes the cluster and the `aws eks get-token` command: kubectl gets a token from your AWS credentials, and Kubernetes sees you as an IAM role. Rights in the cluster come from an access entry or the `aws-auth` ConfigMap (`API_AND_CONFIG_MAP` means both work; new clusters move to `API`). The public API endpoint is better closed or limited by CIDR. The kubelet version on nodes must not lag the control plane beyond the allowed skew — that's checked before a cluster upgrade. You need configured AWS credentials. Your IDs and values will differ.

</details>

## Storage and databases

### S3 — object storage
- Data is stored as **objects** (a file plus metadata) in **buckets**. A bucket name is globally unique.
- **Eleven nines of durability** (99.999999999%): an object is stored in at least three AZs.
- **Strong consistency**: after a write, the new version is read immediately.
- **Storage classes**: Standard; Intelligent-Tiering (automatically moves rarely used data to a cheaper class); Standard-IA and One Zone-IA (IA — Infrequent Access: cheaper storage, more expensive reads); Glacier Instant, Flexible and Deep Archive (archive: very cheap, retrieval from milliseconds to 12 hours).
- **Lifecycle policies** — automatically move objects to cheaper classes and delete old ones ("logs older than 30 days — to IA, older than a year — delete").
- **Versioning** — keep all versions of an object: protection against accidental deletion and overwrites (mandatory for a bucket holding Terraform state).
- **Replication** (to another region — CRR, Cross-Region Replication, within the same one — SRR) — for DR (recovery after an outage) and compliance requirements (laws and standards, e.g. "a copy of the data must be in another region").
- **Encryption**: SSE-S3 (Server-Side Encryption — S3 encrypts objects with its own keys) is on by default; SSE-KMS — with your own KMS keys and usage auditing.
- **Block Public Access** — forbid public access at the bucket and account level. **Always enable it**, except for intentionally public buckets. Public buckets with data are one of the most frequent causes of leaks.
- **Presigned URL** — a temporary link to upload or download a specific object without granting permissions.
- **Object Lock** — protection against deletion for a given period (backups, protection against ransomware).

### RDS and Aurora — managed relational databases
**RDS** — PostgreSQL, MySQL, MariaDB, SQL Server, Oracle managed by AWS: installation, patches, backups, monitoring.

**Multi-AZ and Read Replica — a frequent question:**
| | Multi-AZ | Read Replica |
|---|---|---|
| Why | **High availability** | **Scaling reads** |
| Replication | Synchronous | Asynchronous (a small lag) |
| Can be read | No (the standby in the other zone just waits) | Yes |
| When the primary fails | **Automatic failover** in 1–2 minutes, the DNS name doesn't change | The replica has to be promoted to primary manually |
| Location | Another AZ | Any AZ or another region |

- **Backups**: automatic (up to 35 days) with **restore to any second** (PITR — point-in-time recovery). A restore creates a **new** instance. Plus manual snapshots.
- **Aurora** — an engine reworked by AWS, compatible with MySQL and PostgreSQL: the storage replicates itself across 3 AZs (6 copies), up to 15 read replicas with little lag, fast failover (switching to a replica when the primary fails), Serverless v2 (the database's capacity grows and shrinks with load automatically, and you pay for what's actually used).
- **RDS Proxy** — a connection pool in front of the database: needed when there are many short-lived connections (Lambda, a large number of pods).

### DynamoDB
A NoSQL key-value and document database. Serverless, with single-digit millisecond latency at any scale. A table has a **partition key** (data is distributed by it) and optionally a sort key. Modes: on-demand (pay per request) and provisioned (a set capacity). **GSIs** — additional indexes for queries by other fields. Choosing the key correctly matters: with an uneven distribution of requests, "hot" partitions appear.

### ElastiCache
Managed Redis, Valkey (an open-source fork of Redis after its licence change) or Memcached: cache, sessions, queues, rate limiting.

### Secrets and keys
- **Secrets Manager** — stores secrets with **automatic rotation** (for example, the RDS password changes on a schedule, and the application gets the new one via the API). Paid per secret.
- **SSM Parameter Store** — configuration parameters and secrets (the SecureString type, KMS encryption). Standard parameters are free. No built-in rotation.
- **KMS** — encryption key management. Used by almost every service (EBS, S3, RDS, Secrets Manager). **Envelope encryption**: data is encrypted with a data key, and the data key with a KMS key that never leaves KMS. The key policy defines who can use the key; every use is logged in CloudTrail.

### Database backups: types and verification
What backups there are and what each gives:
| Type | How it works | RPO | What for |
|---|---|---|---|
| **Snapshot** (RDS snapshot, EBS snapshot) | a copy of the disk at a point in time, incremental | the time between snapshots | fast full restore, a copy to another region or account |
| **PITR** (point-in-time recovery) in RDS | a daily snapshot + the transaction log, uploaded every ~5 minutes | about 5 minutes | rolling back to the second before a mistake ("a table was dropped at 09:56") |
| **Logical dump** (`pg_dump`, `mysqldump`) | SQL or an archive with data and schema | the time between dumps | moving between versions and clouds, restoring a single table |
| **Physical backup + WAL** (pgBackRest, WAL-G, Barman) | a copy of the database files + a continuous archive of WAL (PostgreSQL's change log) in object storage | seconds to minutes | PITR for your own PostgreSQL (on EC2, in Kubernetes, on-prem) |

**A replica isn't a backup.** A read replica and Multi-AZ protect against server failure, but `DELETE FROM orders` without a `WHERE` is instantly replicated to all copies. Only backups with history protect against human error and ransomware.

**RDS: restores and copies:**
```bash
# restore the database to the moment before the mistake — a NEW instance is created, the old one isn't touched
aws rds restore-db-instance-to-point-in-time \
  --source-db-instance-identifier orders-db \
  --target-db-instance-identifier orders-db-restore-0955 \
  --restore-time 2026-10-28T09:55:00Z

# copy a snapshot to another region (for DR): it's encrypted with the destination region's KMS key
aws rds copy-db-snapshot --region eu-west-1 --source-region eu-central-1 \
  --source-db-snapshot-identifier arn:aws:rds:eu-central-1:111122223333:snapshot:orders-2026-10-28 \
  --target-db-snapshot-identifier orders-2026-10-28 \
  --kms-key-id alias/dr-backup
```
Then the application is switched to the new instance, or the needed rows are selectively carried over from it. For permanent protection against losing a region RDS has **cross-region automated backups** (replication of automated backups and logs to another region — PITR there too), and Aurora has Global Database.

**A logical dump** is a portable copy that can be brought up anywhere, including another cloud:
```bash
pg_dump -Fc -d "$DATABASE_URL" -f orders-$(date +%F).dump        # -Fc: a compressed format for pg_restore
pg_restore -d "$RESTORE_URL" -j 4 --no-owner orders-2026-10-28.dump   # -j 4: in 4 parallel jobs
pg_restore -d "$RESTORE_URL" -t orders --data-only orders-2026-10-28.dump   # a single table only
```
On large databases (hundreds of gigabytes) a dump takes hours and loads the database, so it's taken from a replica, while snapshots and PITR remain the main backup.

**Your own PostgreSQL: pgBackRest with two repositories** — S3 in AWS and GCS in Google Cloud, so that the backup survives losing an account or a cloud:
```ini
# /etc/pgbackrest/pgbackrest.conf
[global]
repo1-type=s3
repo1-s3-bucket=acme-pg-backup
repo1-s3-region=eu-central-1
repo1-s3-endpoint=s3.eu-central-1.amazonaws.com
repo1-s3-key-type=auto
repo1-path=/orders
repo1-retention-full=4
repo2-type=gcs
repo2-gcs-bucket=acme-pg-backup-gcp
repo2-gcs-key=/etc/pgbackrest/gcs-key.json
repo2-path=/orders
repo2-retention-full=2
start-fast=y

[orders]
pg1-path=/var/lib/postgresql/16/main
```
```bash
# in postgresql.conf: archive_mode = on, archive_command = 'pgbackrest --stanza=orders archive-push %p'
pgbackrest --stanza=orders stanza-create
pgbackrest --stanza=orders --type=full backup            # a full backup to both repositories
pgbackrest --stanza=orders --repo=2 --type=time \
  --target="2026-10-28 09:55:00+00" restore              # restore from GCS to a point in time (PostgreSQL stopped)
```
`repo1-s3-key-type=auto` — take the permissions from the instance's IAM role, with no keys in the file. In Kubernetes the **CloudNativePG** operator does the same (backups and WAL to object storage via Barman).

**A backup that has never been restored isn't a backup.** Verification must be automatic:
- a regular (for example, weekly) restore into a separate instance by a script or via **AWS Backup restore testing** (the service itself restores selected resources on a schedule and deletes them after the check);
- a check query after the restore: the row count in key tables, the date of the latest record;
- metrics: the restore time (that's the real RTO), the age of the last successful backup (an alert if it's older than a day);
- encryption and permissions: backups are encrypted, and the same user who manages the database can't delete them (see "Backups across accounts and regions").

### RDS or your own database (on EC2 or in Kubernetes)
| | RDS / Aurora | Own DB on EC2 | Own DB in Kubernetes (an operator) |
|---|---|---|---|
| Backups, PITR, patches, failover | out of the box | you set it up yourself (pgBackRest, Patroni) | the operator (CloudNativePG, Zalando, Percona) |
| Access to the OS and superuser | no | full | full |
| Extensions and versions | only those RDS supports, new versions arrive with a delay | any | any |
| Cost | more expensive for the same size (roughly 1.5–2x per instance), but without the team's time | cheaper in hardware, more expensive in people's time | cheaper in hardware, strong knowledge of Kubernetes and storage is needed |
| Scale and limits | up to 64 TiB per RDS instance (Aurora — from 128 TiB, more in newer versions), IOPS limits by disk type | limited by the instance and disks | limited by the nodes and the StorageClass |
| Who's on call at night | AWS for the infrastructure, you for the queries and schema | you entirely | you entirely |

Arguments in an interview:
- **For RDS:** a small team, no DBA (database administrator), predictable backups and failover matter, audit requirements (encryption, logs, AWS certifications). Engineers' time is usually more expensive than the difference in instance price.
- **For your own DB:** you need extensions or versions that RDS doesn't have (for example, special PostgreSQL extensions), you need OS access and fine tuning, very large volumes where the price difference is tens of thousands of dollars a month, multi-cloud or on-prem where there's no RDS.
- **Your own DB in Kubernetes** is justified when the platform is already on Kubernetes and there's experience with StatefulSets, storage and operators: the same deployment method in any cloud. Risks: losing a volume with the wrong StorageClass (`reclaimPolicy: Delete`), operator upgrades, hard disk diagnostics.
- An intermediate option: **RDS Custom** (OS access while AWS manages it, for Oracle and SQL Server).

### 🧪 Mini task

**Task.** Create a test bucket with versioning, upload the same key twice, delete it and look at which versions and delete markers remain. Then restore the file and remove the bucket.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
b=lab-versioning-$(date +%s)
aws s3 mb s3://$b --region us-east-2 > /dev/null
aws s3api put-bucket-versioning --bucket $b --versioning-configuration Status=Enabled
echo v1 | aws s3 cp - s3://$b/app.conf; echo v2 | aws s3 cp - s3://$b/app.conf
aws s3 rm s3://$b/app.conf > /dev/null
aws s3 ls s3://$b/ | wc -l
aws s3api list-object-versions --bucket $b --query '[length(Versions), length(DeleteMarkers)]' --output text
marker=$(aws s3api list-object-versions --bucket $b --query 'DeleteMarkers[0].VersionId' --output text)
aws s3api delete-object --bucket $b --key app.conf --version-id "$marker" > /dev/null
aws s3 cp s3://$b/app.conf -
aws s3 rb s3://$b --force > /dev/null 2>&1 || echo "delete versions before rb"
```

Expected output:

```text
       0
2	1
v2
```

Breakdown: with versioning `rm` doesn't delete data, it adds a delete marker — the object disappeared from the listing, but both versions are still there. Deleting the marker itself "resurrected" the latest version. That's how versioning protects against accidental deletes and overwrites; against malicious deletion add MFA Delete or Object Lock. Old versions are billed like normal objects, so versioning always comes with a `NoncurrentVersionExpiration` lifecycle rule. `rb --force` may fail to delete a versioned bucket; then the versions are deleted separately. You need configured AWS credentials. Your IDs and values will differ.

</details>

## Service integration
- **SQS** — a message queue: decouples the sender and the receiver, smooths out load peaks. Standard (order isn't guaranteed, duplicates are possible) and FIFO (strict order, exactly once). **Visibility timeout** — how long a received message is hidden from other consumers; if it isn't deleted within that time, it becomes visible again. **DLQ** (dead-letter queue) — where messages go that couldn't be processed N times.
- **SNS** — pub/sub (publish/subscribe: the sender publishes a message to a topic without knowing the receivers, and every subscriber of the topic gets a copy): one message goes to all subscribers (SQS queues, Lambda, email, HTTP). The **fan-out** pattern: SNS → several SQS queues for independent handlers (for example, order created → one queue for the warehouse, another for the customer email).
- **EventBridge** — an event bus with rule-based routing, events from AWS services (for example, "the instance was stopped"), schedules.

### 🧪 Mini task

**Task.** Create an SQS queue with a 5-second visibility timeout, send a message, receive it without deleting, try to receive again immediately and after 6 seconds. Then delete the message and the queue.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
q=$(aws sqs create-queue --queue-name lab-orders --attributes VisibilityTimeout=5 --query QueueUrl --output text)
aws sqs send-message --queue-url $q --message-body 'order 42' > /dev/null
recv() { aws sqs receive-message --queue-url $q --attribute-names ApproximateReceiveCount \
  --query 'Messages[0].[Body, Attributes.ApproximateReceiveCount]' --output text; }
recv
recv
sleep 6
recv
handle=$(aws sqs receive-message --queue-url $q --visibility-timeout 0 --query 'Messages[0].ReceiptHandle' --output text)
aws sqs delete-message --queue-url $q --receipt-handle "$handle"
aws sqs delete-queue --queue-url $q
```

Expected output:

```text
order 42	1
None
order 42	2
```

Breakdown: SQS doesn't delete a message on receive — it hides it for the visibility timeout. While a consumer processes it, others don't see it (the second call returned nothing). If the consumer doesn't call `delete-message` within 5 seconds (crashed, hung), the message becomes visible again and is delivered again — the receive count rose to 2. Hence "at least once" delivery and the need for idempotent processing. After N failed receives the message moves to a DLQ if a `RedrivePolicy` is configured. You need configured AWS credentials. Your IDs and values will differ.

</details>

## Observability and auditing
- **CloudWatch Metrics** — metrics of AWS services (CPU, network traffic, load balancer errors) and your own. **Alarms** — notifications on thresholds or anomalies. **CloudWatch Logs** — log collection, **Logs Insights** — queries over them.
- **CloudTrail** — the journal of **all API calls** in the account: who, when, from where and what. The main source in investigations ("who deleted the resource?"). Logs are kept in S3 (in a separate account) and analysed with Athena (a service that runs SQL queries straight over files in S3).
- **AWS Config** — the configuration history of resources and compliance checks against rules ("are all buckets encrypted").
- **GuardDuty** — threat detection from CloudTrail, VPC Flow Logs and DNS: suspicious API calls, crypto-mining, calls to known malicious addresses. **Security Hub** — a summary of security findings from GuardDuty, Inspector, Config in one place, scored against standards (CIS, AWS Foundational).
- **VPC Flow Logs** — a log of network connections (who connected to whom and whether it was allowed) — for debugging Security Groups and for investigations.

### 🧪 Mini task

**Task.** Use CloudTrail to find out who stopped EC2 instances over the last few days, when, and from which IP.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws cloudtrail lookup-events --region us-east-2 --max-results 3 \
  --lookup-attributes AttributeKey=EventName,AttributeValue=StopInstances \
  --query 'Events[].CloudTrailEvent' --output text |
jq -r '[.eventTime, .userIdentity.arn, .sourceIPAddress, .requestParameters.instancesSet.items[0].instanceId] | @tsv'
```

Expected output:

```text
2026-10-25T18:02:11Z	arn:aws:sts::029940625330:assumed-role/AWSReservedSSO_AdministratorAccess_1a2b3c4d5e6f7a8b/alex	198.51.100.23	i-0abc123def4567890
2026-10-24T03:00:05Z	arn:aws:sts::029940625330:assumed-role/instance-scheduler/1729738800	lambda.amazonaws.com	i-0fed987cba6543210
```

Walkthrough: CloudTrail records every API call: who (`userIdentity`), from where (`sourceIPAddress`), what and with which parameters. `lookup-events` searches only management events (resource management operations — create, stop, delete; as opposed to data events — reading and writing data, such as S3 objects) for the last 90 days and only in one region. For long retention and search, trails are written to S3 and queried via Athena or CloudTrail Lake (CloudTrail's built-in store with SQL search). The second line shows automation: the instance was stopped by a scheduler Lambda under its own role — telling a person from automation is only possible through such an audit. Configured AWS credentials are required. The identifiers and values will be your own.

</details>

## Cost (FinOps)
The main sources of cost and ways to reduce them:
- **Compute**: size instances by actual load (rightsizing — Compute Optimizer), Savings Plans for a constant load, **Spot** for stateless workloads (Karpenter), **Graviton**, turn dev environments off at night and at weekends.
- **Traffic**: the **NAT Gateway** (per gigabyte) — add **VPC endpoints** for S3 and ECR; **cross-AZ traffic** is paid — take it into account in the architecture; outbound traffic to the internet — CloudFront is cheaper.
- **Storage**: gp3 instead of gp2 (cheaper with the same performance), S3 lifecycle policies, delete unattached EBS disks, old snapshots and AMIs, unused Elastic IPs (they cost money when not attached).
- **Visibility**: **tags** on all resources (team, project, environment) — without them you can't tell who is spending. Cost Explorer (spending charts in the console broken down by service and tag), Budgets with alerts ("warn if the month exceeds $5000"), the Cost and Usage Report (a detailed export of all charges to S3 for analysis).

### 🧪 Mini task

**Task.** Find two classic sources of waste: EBS volumes not attached to any instance and unassociated Elastic IPs.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws ec2 describe-volumes --region us-east-2 --filters Name=status,Values=available \
  --query 'Volumes[].[VolumeId, Size, VolumeType, CreateTime]' --output text
echo ---
aws ec2 describe-addresses --region us-east-2 \
  --query 'Addresses[?AssociationId==null].[PublicIp, AllocationId]' --output text
```

Expected output:

```text
vol-0a1b2c3d4e5f60789	200	gp3	2026-03-14T09:21:44.000Z
vol-0f9e8d7c6b5a43210	50	gp2	2025-11-02T16:05:12.000Z
---
3.142.10.20	eipalloc-0123456789abcdef0
```

Walkthrough: a volume in the `available` state isn't attached to anything but is billed every month: 200 GB of gp3 is about $16. Usually these are leftovers of deleted instances that didn't have `DeleteOnTermination`. Before deleting, take a snapshot if the data may be needed. Public IPv4 addresses have always been paid since 2024, and an unused EIP is pure loss. `gp2` is worth converting to `gp3`: it's cheaper and has configurable IOPS. Such things are found regularly by Trusted Advisor (built-in AWS checks on cost, security and limits) or Cost Optimization Hub, but in an interview the "how would you find them" question is expected to be answered with the CLI. Configured AWS credentials are required. The identifiers and values will be your own.

</details>

## Well-Architected Framework
A set of AWS principles for assessing an architecture along six pillars:
1. **Operational Excellence** — automation, IaC, observability, incident reviews;
2. **Security** — least privilege, encryption, auditing, protection at every layer;
3. **Reliability** — multi-AZ, automatic recovery, backups, failure testing;
4. **Performance Efficiency** — suitable services and resource types, scaling;
5. **Cost Optimization** — pay only for what you need;
6. **Sustainability** — efficient use of resources.

### 🧪 Mini task

**Task.** Check two basics from the Reliability pillar: whether RDS databases run Multi-AZ and how many zones the Auto Scaling groups span.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws rds describe-db-instances --region us-east-2 \
  --query 'DBInstances[].[DBInstanceIdentifier, Engine, MultiAZ]' --output text
echo ---
aws autoscaling describe-auto-scaling-groups --region us-east-2 \
  --query 'AutoScalingGroups[].[AutoScalingGroupName, length(AvailabilityZones), DesiredCapacity]' --output text
```

Expected output:

```text
orders-db	postgres	True
reports-db	postgres	False
---
web-asg	3	6
batch-asg	1	2
```

Breakdown: `reports-db` without Multi-AZ will be unavailable during a zone failure or maintenance until recovered; in prod that breaks the "no single point of failure" principle. `batch-asg` in one AZ survives an instance failure, but not a zone failure. That's how a Well-Architected Review is done: for each pillar (reliability, security, cost, performance, operations, sustainability) you check concrete facts, not "I think it's configured". Sometimes a single AZ is a deliberate cost trade-off; then it's recorded as an accepted risk. You need configured AWS credentials. Your IDs and values will differ.

</details>

## Disaster Recovery
**RPO** (Recovery Point Objective) — how much data you can afford to lose (for example, 15 minutes = a backup or replication at least that often). **RTO** (Recovery Time Objective) — how quickly service must be restored.

Strategies from cheap (large RPO and RTO) to expensive (near zero):
1. **Backup & Restore** — only backups in another region; recovery takes hours. The infrastructure is brought up from IaC.
2. **Pilot Light** — a minimal core runs in the standby region (a database replica), the rest is brought up during an incident; tens of minutes.
3. **Warm Standby** — a scaled-down but fully working copy; scaled up during an incident; minutes.
4. **Multi-site active/active** — full copies in several regions take traffic at the same time; almost instant, but expensive and complex (data consistency).

### How to choose a DR strategy: from RPO and RTO to architecture
A strategy is chosen not "by taste" but from the business requirements: how much an hour of downtime costs and how much data can be lost. An example: an online shop loses $20,000 per hour of downtime → an RTO of 1 hour and an RPO of 5 minutes justify Pilot Light; an internal wiki → Backup & Restore with an RTO of a day.

| Strategy | RPO | RTO | What runs constantly in the standby region | Cost |
|---|---|---|---|---|
| Backup & Restore | hours (the time between backups) | hours to a day | nothing, only backup copies | minimal |
| Pilot Light | seconds to minutes (data replication) | tens of minutes | DB replicas, images, IaC ready to apply | low |
| Warm Standby | seconds | minutes | a scaled-down working copy of the whole stack | medium |
| Active/Active | about zero | about zero | a full copy that takes traffic | maximal, plus the complexity of data consistency |

What has to be in the standby region for recovery to work at all — this is what interviews ask about:
- **data**: Aurora Global Database, RDS cross-region automated backups, S3 replication (CRR), DynamoDB Global Tables;
- **images and artefacts**: ECR replication to another region, AMIs copied;
- **secrets and keys**: Secrets Manager replica secrets, KMS keys in the destination region (multi-region keys if the application itself encrypts the data);
- **infrastructure**: Terraform with a region variable, a tested `apply` in the standby region;
- **quotas**: the standby region's service quotas (number of vCPUs, EIPs, ENIs) raised in advance — they won't be increased in 10 minutes during an outage;
- **traffic switching**: a Route 53 failover policy with health checks or **Route 53 Application Recovery Controller** (managed switchover "routing controls" that work even when the primary region has problems);
- **a runbook and drills** (see below).

**AWS Elastic Disaster Recovery (DRS)** — for servers that can't be quickly rebuilt from IaC (legacy, Windows): it continuously replicates the servers' disks to a cheap staging area in another region and brings them up within minutes during an outage.

### Backups across accounts and regions
**Why another account.** If an attacker (or ransomware) got administrator rights in the prod account, they'll delete both the databases and their backups in the same account. A copy in a **separate backup account**, where prod has no rights to delete, survives this. Another region protects against losing a region, another account — against losing access and malicious intent. Both are needed.

**AWS Backup** — unified backup plans for RDS, Aurora, EBS, EFS, DynamoDB, S3 and others: a schedule, a retention period and **copying** to another region and another account in the organization:
```json
{
  "BackupPlanName": "prod-daily",
  "Rules": [
    {
      "RuleName": "daily-35d",
      "TargetBackupVaultName": "prod-vault",
      "ScheduleExpression": "cron(0 3 * * ? *)",
      "Lifecycle": { "DeleteAfterDays": 35 },
      "CopyActions": [
        {
          "DestinationBackupVaultArn": "arn:aws:backup:eu-west-1:999988887777:backup-vault:dr-vault",
          "Lifecycle": { "DeleteAfterDays": 90 }
        }
      ]
    }
  ]
}
```
The copy goes to the `dr-vault` vault of account `999988887777` in region `eu-west-1` — another region and another account at once. Requirements: cross-account backup enabled in AWS Organizations, an access policy on the destination vault allowing copies from the source account, and **KMS keys**: resources (for example, RDS and EBS) encrypted with an AWS managed key (`aws/rds`, `aws/ebs`) can't be copied to another account — you need a customer managed key with a key policy granting access to the destination account.

**Protection against deletion:**
- **Backup Vault Lock** in compliance mode — WORM (write once, read many: you can write, but nobody can delete or change anything before the retention period ends, not even the account's root);
- **logically air-gapped vault** — a vault in a separate isolated AWS storage zone, which can be shared with other accounts for restores;
- an **SCP** in Organizations denying `backup:DeleteRecoveryPoint`, `rds:DeleteDBSnapshot` and disabling plans in prod accounts.

**S3: replication and object locking:**
- **CRR** (Cross-Region Replication) and SRR (within the same region) asynchronously copy new objects to a bucket in another region and, if needed, another account (switching the objects' owner to the destination account). Versioning must be enabled on both buckets. Existing objects aren't copied — **S3 Batch Replication** is for them. **Replication Time Control** guarantees that 99.99% of objects are copied within 15 minutes;
- deletions (delete markers) aren't replicated by default — this protects the copy from an accidental deletion in the source;
- **Object Lock** (compliance mode) on the destination bucket — objects can't be deleted before the retention period ends.

### Backups to another cloud
When cross-cloud is required (a regulator, a "don't depend on one provider" strategy, protection against an account being blocked):
- **S3 objects → GCS**: Storage Transfer Service in GCP pulls the data from S3 on a schedule by itself (with federated access to an AWS role, no long-lived keys), or `rclone sync s3:acme-data gcs:acme-data-copy` from a scheduled job;
- **databases**: logical dumps or pgBackRest/WAL-G with a second repository in GCS (the example above in "Database backups"). RDS snapshots can't be moved to another cloud — it's an AWS format;
- **Kubernetes**: Velero with storage in GCS (see "DR for EKS");
- **cost**: outgoing traffic from AWS is paid (cents per gigabyte), so large volumes are copied incrementally and compressed, and full copies are made less often;
- **verification**: restoring from another cloud has to be rehearsed just as regularly — otherwise at the moment of an outage it turns out there are no permissions, encryption keys or a compatible version.

### DR for EKS: what to back up and how to bring a cluster up
A Kubernetes cluster in DR consists of three parts, and each has its own strategy:
1. **The cluster itself and the add-ons** — from Terraform (VPC, EKS, node pools) and GitOps (ArgoCD/Flux installs the add-ons and applications from git). Recreation in another region takes 20–40 minutes. Backing up etcd in EKS isn't needed and isn't possible: AWS manages the control plane.
2. **Application manifests** — already in git thanks to GitOps. The new cluster syncs from the same repository.
3. **State**: PersistentVolumes, as well as objects not created from git (secrets, CRD resources created by operators) — **Velero** is for them.

**Velero** backs up Kubernetes objects (as JSON) and volume data to object storage (S3, GCS):
```bash
velero schedule create prod-daily --schedule="0 3 * * *" \
  --include-namespaces prod --ttl 720h            # every day at 3:00, keep for 30 days
velero backup describe prod-daily-20261028030012 --details
velero restore create --from-backup prod-daily-20261028030012 \
  --namespace-mappings prod:prod-restore           # restore into another namespace for a check
```
A subtlety: EBS snapshots are tied to a region and an account. To restore volumes in another region or cloud, Velero has to be set up for **file system backup** (an agent on the node copies the volume data via Kopia to object storage), or the snapshots have to be copied separately. And databases are better moved out of the cluster into RDS or backed up with their own tools — a volume snapshot of a running database may turn out inconsistent.

### DR drills
A DR plan that hasn't been tested almost certainly won't work: outdated runbooks, no quotas, expired keys, a forgotten manual step. The practice:
- a **game day** once a quarter: a real switch to the standby region for part of the traffic or a test environment, measuring the actual RTO and RPO;
- **AWS Fault Injection Service (FIS)** — managed experiments: stop instances, take out an AZ, inject network latency, check that the automation copes;
- the result of a drill is a list of fixes to the runbook and IaC, not an "everything is fine" report.

### 🧪 Mini task

**Task.** For your RDS databases find the automated backup retention and the latest restorable time. Compare it with the current time and estimate the actual RPO.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
date -u +%Y-%m-%dT%H:%M:%SZ
aws rds describe-db-instances --region us-east-2 \
  --query 'DBInstances[].[DBInstanceIdentifier, BackupRetentionPeriod, LatestRestorableTime]' --output text
```

Expected output:

```text
2026-10-26T11:00:30Z
orders-db	7	2026-10-26T10:55:21+00:00
reports-db	0	None
```

Breakdown: `orders-db` keeps backups for 7 days and can be restored to 5 minutes ago: losing the database loses at most ~5 minutes of data — that's the actual RPO. Point-in-time recovery works to any second within the retention window. `reports-db` has backups off (`0`) and could be lost entirely. RTO — how long a restore takes — can't be learned this way: only a practice restore tells you. To survive a region loss, snapshots are copied to another region or a cross-region read replica is used. You need configured AWS credentials. Your IDs and values will differ.

</details>

## AWS CLI in everyday work
Sign in via SSO instead of permanent keys:
```bash
aws configure sso                         # once: describe the profile (SSO portal, account, role) in ~/.aws/config
aws sso login --profile eks-prod          # get temporary credentials via the browser
export AWS_PROFILE=eks-prod               # the default profile for all following commands (and for Terraform, kubectl)
aws sts get-caller-identity               # "who am I": the account and role — check before any changes
aws configure list                        # which profile, and where the credentials come from

aws eks update-kubeconfig --region eu-central-1 --name prod-cluster --alias prod   # kubectl and Lens access to EKS
aws ecr get-login-password --region eu-central-1 \
  | docker login --username AWS --password-stdin <ACCOUNT_ID>.dkr.ecr.eu-central-1.amazonaws.com
docker push <ACCOUNT_ID>.dkr.ecr.eu-central-1.amazonaws.com/fluent-bit:4.0.1
```
Useful tricks: `--query` (a JMESPath filter — the JSON query language built into the AWS CLI — right in the CLI) and `--output table|text|json`, for example:
```bash
aws ec2 describe-instances --output table \
  --query 'Reservations[].Instances[].[InstanceId, State.Name, Tags[?Key==`Name`]|[0].Value]'
```

### 🧪 Mini task

**Task.** Print all running instances as a table: the name from the `Name` tag, ID, type, private IP and AZ, sorted by name.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws ec2 describe-instances --region us-east-2 --filters Name=instance-state-name,Values=running \
  --query 'sort_by(Reservations[].Instances[], &(Tags[?Key==`Name`] | [0].Value))[].{Name: Tags[?Key==`Name`] | [0].Value, Id: InstanceId, Type: InstanceType, IP: PrivateIpAddress, AZ: Placement.AvailabilityZone}' \
  --output table
```

Expected output:

```text
-------------------------------------------------------------------------------
|                              DescribeInstances                              |
+------------+-----------------------+-----------+--------------+-------------+
|     AZ     |          Id           |    IP     |    Name      |    Type     |
+------------+-----------------------+-----------+--------------+-------------+
|  us-east-2a|  i-0abc123def4567890  |  10.0.1.15|  bastion     |  t4g.nano   |
|  us-east-2b|  i-0fed987cba6543210  |  10.0.12.4|  web-1       |  m7g.large  |
|  us-east-2c|  i-0123456789abcdef0  |  10.0.23.8|  web-2       |  m7g.large  |
+------------+-----------------------+-----------+--------------+-------------+
```

Breakdown: `--filters` filters on the AWS side (less data over the wire), while `--query` (JMESPath) filters on the client side, over the received response. A tag is extracted with `` Tags[?Key==`Name`] | [0].Value `` because tags are a list of pairs, not a map. The hash `{Name: …, Id: …}` gives columns with headers. In `table` the columns are sorted alphabetically, not in query order. Scripts use `text` or `json` with `jq` instead of `table`. The CLI paginates responses itself; `--max-items` limits them. You need configured AWS credentials. Your IDs and values will differ.

</details>

## Additionally: Solutions Architect-level services and details
Questions in the style of the AWS Solutions Architect certification (the most popular AWS architecture certification: an exam of "what would you choose for this task" scenarios) often come up in interviews: "which service would you choose for this scenario". Below is what the main sections cover briefly or not at all.

### EC2: purchasing options in more detail
- **On-Demand** — no commitment, for unpredictable load and short projects.
- **Savings Plans** — a commitment to spend a certain amount per hour for 1 or 3 years:
  - **Compute Savings Plans** — a discount of up to ~66%, applies to any instance family and region, as well as Fargate and Lambda. Flexible;
  - **EC2 Instance Savings Plans** — a discount of up to ~72%, but only for the chosen family in the chosen region.
- **Reserved Instances** — the older model of reserving specific parameters (Standard and Convertible). Savings Plans are usually chosen today.
- **Spot** — up to 90% off, AWS can take the instance back with a 2-minute warning.
- **Dedicated Instances** — instances on hardware dedicated to your account only, but without control over the specific server.
- **Dedicated Hosts** — a whole physical server at your disposal: the sockets and cores are visible, and instances can be pinned to the host. Needed for "per core and per socket" licences (Windows Server, SQL Server, Oracle) and compliance requirements.

### Placement Groups — physical placement of instances
- **Cluster** — instances on nearby hardware in **one AZ**: minimal latency and maximum throughput between them. For HPC and distributed computing. The downside: a rack failure can hit all of them. Launch all the instances in one request, otherwise there may not be enough capacity nearby.
- **Spread** — each instance on a **separate rack** with separate power and networking; at most 7 instances per AZ in a group. For a small number of critical instances that must not fail together.
- **Partition** — instances are split into **partitions** (up to 7 per AZ), and partitions don't share racks. For large distributed systems that are topology-aware: Kafka, Cassandra (a distributed NoSQL database), HDFS (the Hadoop distributed file system) — replicas are spread across different partitions, so the failure of one rack doesn't kill all copies of the data.

### EC2 networking for low latency and large flows
- **ENA and enhanced networking** — the network card of Nitro instances, enabled by default on current types (`ethtool -i eth0` → `driver: ena`). Bandwidth and the number of queues grow with instance size.
- **Network-optimised types** with the letter `n` (`c6in`, `c7gn`, `m6in`) — several times higher network and packets-per-second rates.
- **Cluster placement group** — minimal RTT between instances.
- **ENA Express** — the AWS SRD protocol on top of ENA: higher single-flow speed and more stable tail latency between supported instances in one AZ.
- **Jumbo frames** — up to a 9001-byte MTU inside a VPC: fewer packets for the same volume (via an internet gateway and VPN — 1500 or less).
- **Invisible instance limits:** besides bandwidth, packets per second, the number of tracked connections and requests to link-local services (DNS, IMDS, NTP) are limited. Exceeding them means silent drops. Visible in `ethtool -S eth0 | grep allowance` (`pps_allowance_exceeded`, `conntrack_allowance_exceeded`, `linklocal_allowance_exceeded`); these counters must be exported to monitoring.
- **Time** — the Amazon Time Sync Service (`169.254.169.123`) via chrony, and on some instances a PTP hardware clock.
- **FPGA** — F-family instances with loadable logic images (AFI).

For details on tuning a host for latency, DPDK and FPGAs, see the "Linux OS and the file system" topic, and for the AMI lifecycle, the "Docker / AMI images" topic.

### ALB: types of routing
ALB listener rules choose a target group by:
- **path** (`/api/*` → one service, `/static/*` → another);
- **host** (`api.example.com` and `www.example.com` on one ALB);
- **HTTP headers** (for example, `User-Agent` or your own version header);
- **method** (GET, POST);
- **query string parameters** (`?version=v2`);
- **source IP address**.

Rule actions: forward to one or **several target groups with weights** (canary and blue/green at the ALB level: 90% to the old version, 10% to the new), redirect (for example, HTTP → HTTPS), a fixed response, authentication via OIDC or Cognito (the AWS service for signing up and signing in an application's users). **Classic Load Balancer** is an obsolete type not used in new projects.

### S3: uploads, lifecycle, encryption
**Speeding up uploads of large files:**
- **Multipart upload** — the file is split into parts that are uploaded in parallel and, on failure, re-uploaded individually. Recommended from 100 MB, required from 5 GB. The AWS CLI does it automatically. Incomplete multipart uploads take up space and cost money — they are removed with the `AbortIncompleteMultipartUpload` lifecycle rule.
- **S3 Transfer Acceleration** — the client uploads the file to the nearest CloudFront edge location, and from there the data travels over the AWS network. Paid; helps with uploads from far away (users all over the world).

**Lifecycle:** objects can be moved to Standard-IA or One Zone-IA **no earlier than 30 days** after creation. The IA and Glacier classes have a minimum storage duration and a retrieval fee, so moving frequently read data there doesn't pay off. If the access pattern is unknown — Intelligent-Tiering.

**Object encryption options:**
| Option | Who manages the keys | When to choose it |
|---|---|---|
| **SSE-S3** | Entirely AWS (AES-256), enabled by default | Baseline encryption without extra effort |
| **SSE-KMS** | A key in KMS (AWS managed or your own customer managed) | When you need control over access to the key, auditing of every use in CloudTrail, separate keys per project. Watch the KMS request limits under heavy traffic (S3 Bucket Keys reduce them) |
| **SSE-C** | You pass the key in every request; AWS doesn't store it | A requirement to keep the keys only on your side. Lose the key — lose the data. HTTPS only |
| **Client-side** | You encrypt before sending to S3 | S3 never sees plaintext data at all |

Protection against deletion: **versioning**, **MFA Delete** (deleting versions and disabling versioning only with the root account's MFA), **Object Lock** (a ban on deletion for a period — for backups and regulatory requirements).

### KMS in more detail
- **Key types:** **AWS managed** (`aws/s3`, `aws/ebs` — created by services, AWS manages the policy) and **customer managed** (you create them yourself: your own key policy, rotation, disabling, a separate key per project). Previously called CMKs (customer master keys).
- **Envelope encryption:** a KMS key never leaves KMS and encrypts only **data keys**; the data itself is encrypted locally with a data key. That's how large volumes are encrypted without sending them to KMS.
- **Rotation** of customer managed keys is enabled with one setting: once a year by default (the period is configurable). Old key versions are kept, so previously encrypted data can still be decrypted.
- **Deleting a key is irreversible**, so AWS makes it deferred: the key first goes into the **pending deletion** state for 7–30 days (30 by default). During that time it can be brought back via `CancelKeyDeletion`. After deletion, all data encrypted with that key can never be decrypted. If you're not sure, it's better to **disable** the key than delete it. An alert on `ScheduleKeyDeletion` calls in CloudTrail is useful.

### File storage and hybrid
- **EFS** — NFS for Linux, data across several AZs, the size grows by itself. Standard and Infrequent Access classes with lifecycle rules. In Kubernetes — ReadWriteMany volumes.
- **FSx** — managed "real" filesystems:
  - **FSx for Windows File Server** — SMB (the Windows file-sharing protocol), integration with Active Directory (Microsoft's corporate user directory), for Windows applications;
  - **FSx for Lustre** — a parallel FS for HPC and machine learning with very high throughput (Lustre is a supercomputer file system), integrates with S3 (data from a bucket is visible as files);
  - **FSx for NetApp ONTAP** — NetApp features (snapshots, replication, NFS, SMB, iSCSI — a protocol for a block disk over the network), for migrating from on-prem NetApp;
  - **FSx for OpenZFS** — ZFS with snapshots and clones.
- **Storage Gateway** — a bridge between on-premises infrastructure and AWS storage:
  - **File Gateway** — a local NFS or SMB share whose files are stored as objects in S3;
  - **Volume Gateway** — block volumes over iSCSI: *stored* (all data locally, asynchronous snapshots in AWS) or *cached* (the main data in S3, frequently used data in a local cache);
  - **Tape Gateway** — a virtual tape library for existing backup software; the tapes are stored in S3 and Glacier.

### Databases: which one to choose
| Task | Service |
|---|---|
| A relational database (PostgreSQL, MySQL, SQL Server, Oracle) | **RDS** |
| Relational with high performance and fast failover | **Aurora**: shared storage across 3 AZs, up to 15 read replicas with low lag, a replica becomes primary in tens of seconds (failover priorities are set via tiers). **Aurora Global Database** — replicas in other regions with lag usually under a second, for DR and reading close to users |
| Key-value with predictable latency at any scale | **DynamoDB**; **DAX** — a built-in cache in front of DynamoDB with microsecond read latency, without changing query code |
| MongoDB-style documents | **DocumentDB** |
| Cache, sessions, rate limiting | **ElastiCache** (Redis, Valkey, Memcached) |
| Redis as the primary durable database | **MemoryDB** — Redis-compatible, with a transaction log across several AZs |
| Analytics, a data warehouse (OLAP) | **Redshift** — columnar storage; **Redshift Spectrum** runs SQL directly over files in S3 without loading them |
| Relationship graphs (social networks, recommendations, fraud) | **Neptune** |
| Time series | **Timestream** |
| Serverless SQL queries over files in S3 | **Athena** (you pay for the volume of data scanned — hence columnar formats like Parquet and partitioning) |

### Streaming data and queues
- **Kinesis Data Streams** — a real-time event stream (logs, clicks, telemetry) retained from a day to a year; several consumers read one stream independently, and order is kept within a shard (a shard is a "lane" of the stream with a fixed throughput; more shards — more speed). An analogue of Kafka; managed Kafka is **Amazon MSK** (in detail — in the [Kafka](#19-kafka.md/s13) topic).
- **Amazon Data Firehose** (formerly Kinesis Data Firehose) — delivers a stream to S3, Redshift, OpenSearch (a search and analytics database, a fork of Elasticsearch), Splunk (a commercial log analysis system) without code, with buffering, compression and transformation. "Near real time" (a buffer of seconds to minutes).
- **Managed Service for Apache Flink** (formerly Kinesis Data Analytics) — real-time stream processing and aggregation (SQL or Flink).
- **Kinesis Video Streams** — ingesting and storing video streams from cameras.

How to choose between Kinesis, SQS and SNS: **SQS** — a task queue, a message is processed by one consumer and deleted. **SNS** — delivering one event to many subscribers. **Kinesis** — a stream that several consumers read independently and can re-read from a chosen position.

**SQS: Standard and FIFO in more detail.** Standard — practically unlimited throughput, **at-least-once** delivery (duplicates are possible), order isn't guaranteed. FIFO — strict order (within a message group) and exactly-once processing (deduplication), but throughput is limited: 300 operations per second, or 3000 messages per second with batches of 10 (significantly more in high throughput mode). Handlers of a Standard queue's messages must be **idempotent**.

### API Gateway
A managed "front door" for APIs:
- **REST API** — the most features: response caching, API keys and usage plans (quotas for clients), request transformation, WAF integration;
- **HTTP API** — simpler, faster and about 70% cheaper, for proxying to Lambda and HTTP backends;
- **WebSocket API** — a persistent two-way connection (chats, real-time notifications); unlike REST, the connection is stateful.

Features: authorisation (IAM, Cognito, JWT — a signed user token, your own Lambda authorizer — a function that decides itself whether to let a request in), throttling (limiting requests per second), stages (dev, prod) with variables, logging to CloudWatch, X-Ray tracing (the AWS distributed tracing service: it shows a request's path through all services). The typical serverless combo: API Gateway → Lambda → DynamoDB.

### Lambda in more detail
- Triggered by events: API Gateway, S3 (a new file), SQS, Kinesis and DynamoDB Streams, SNS, EventBridge (including on a schedule — a cron replacement).
- **Layers** — shared libraries and dependencies attached to several functions.
- **Concurrency**: simultaneously running instances. **Reserved concurrency** — guarantee a function part of the account's overall limit or, conversely, **cap** it (for example, so that Lambda doesn't open thousands of connections to a database). **Provisioned concurrency** — pre-warmed instances without a cold start (paid).
- The scaling trap: Lambda easily scales to hundreds of instances, but a database doesn't. Solutions: RDS Proxy (a connection pool), limiting concurrency, buffering via SQS.

### Hybrid connectivity: how to connect a data centre to AWS
| Option | What it is | When |
|---|---|---|
| **Site-to-Site VPN** | An encrypted IPsec tunnel over the internet to a VPC or Transit Gateway | Quick to set up, inexpensive; bandwidth and latency depend on the internet |
| **Direct Connect** | A dedicated physical link from the data centre to AWS (1–100 Gbit/s) | Stable latency, large volumes, cheaper traffic. Provisioning takes weeks. It's **not encrypted** by itself (MACsec — link-layer encryption, or a VPN on top) |
| **Direct Connect + VPN** | A VPN as the backup link | Resilience with an inexpensive backup |
| **Transit Gateway** | A central router for many VPCs, VPNs and Direct Connect | Many VPCs and accounts; not a link to the data centre by itself but an attachment point |
| **VPN CloudHub** | Several remote offices connect to each other via the AWS VPN gateway | Linking branch offices via AWS |
| **Client VPN** | A VPN for individual employees (an OpenVPN client) | Remote access for people to a VPC |
| **PrivateLink** | Private access to a specific service (AWS or a partner's) from a VPC | Not for connecting to a data centre but for accessing services without the internet or peering |

### CloudFront in more detail
- It caches content at edge locations; between the edge and the origin there's an intermediate layer — **regional edge caches**.
- **Requests that bypass the regional caches** go straight to the origin: requests with the **PUT, POST, PATCH, OPTIONS, DELETE** methods (proxied, not cached) and **dynamic content**, when a cache behavior forwards all headers to the origin (the response depends on every request).
- **Origin Access Control (OAC)** — the S3 bucket is closed to everyone except CloudFront: users can't bypass the CDN and WAF by going to S3 directly.
- **Signed URLs and signed cookies** — access to paid or private content via a temporary signed link.
- An ACM certificate for CloudFront must be in the **us-east-1** region.

### Special placement zones
- **Local Zones** — AWS mini-zones in large cities for single-digit millisecond latency to users (games, video, finance).
- **Wavelength Zones** — AWS infrastructure inside 5G carriers' networks for mobile applications with ultra-low latency.
- **Outposts** — AWS racks in your own data centre (when the data must stay on site).

### CloudWatch in more detail
- Service **metrics** are collected automatically. For EC2 every 5 minutes by default, **detailed monitoring** — every minute (paid).
- **EC2 memory and disk space are not collected by default** — the hypervisor can't see them. The **CloudWatch Agent** inside the instance is needed. A frequent interview question.
- **Alarms** trigger actions: an SNS notification, ASG scaling, stopping or recovering an instance.
- **Logs Insights** — a query language over logs:
  ```
  fields @timestamp, @message
  | filter @message like /ERROR/
  | stats count(*) as errors by bin(5m)
  | sort errors desc
  ```
- **Metric filters** — turn log lines into metrics (the number of errors) and attach an alarm to them.
- **EventBridge** (formerly CloudWatch Events) — reacting to AWS events and schedules.
- The default retention of log groups is **never expire** — a frequent cause of a growing bill; set retention.

### GuardDuty in more detail
It analyses CloudTrail (suspicious API calls), VPC Flow Logs (communication with known malicious addresses, port scanning, crypto mining), DNS queries, and also (with optional modules) S3, EKS audit logs and runtime activity, EBS volumes for malware. Findings are sent to Security Hub and EventBridge (for example, automatic isolation of an instance by a Lambda function). When the service is disabled, its data and findings are deleted.

### Connecting AWS and GCP
A typical question: "some services are in AWS, some in GCP — how do you connect the networks". Options from simple to expensive:

| Option | How it works | Bandwidth | When |
|---|---|---|---|
| **HA VPN ↔ Site-to-Site VPN** | IPsec tunnels over the internet, routes via BGP | usually ~1.25 Gbit/s per AWS tunnel (more with newer tunnel types), more in total via several tunnels and ECMP on Transit Gateway | a start, moderate traffic, set up in an hour |
| **A partner link** (Equinix Fabric, Megaport) | virtual Direct Connect and Partner Interconnect circuits through the partner's shared exchange point | 50 Mbit/s–10 Gbit/s and more | stable latency without your own hardware in a colocation |
| **Cross-Cloud Interconnect** (Google) or the joint managed AWS and Google link | a physical connection from Google directly to AWS in a colocation | 10 or 100 Gbit/s | large constant data flows |

In 2025 AWS and Google announced a joint managed link between the clouds (AWS Interconnect – multicloud together with Cross-Cloud Interconnect on Google's side) — before designing, check whether it's available in the regions you need.

**The HA VPN setup that Google recommends:**
- in GCP: an **HA VPN gateway** (it has two external interfaces with different IPs) and a **Cloud Router** (a virtual BGP router, a private ASN, for example 65534);
- in AWS: a Virtual Private Gateway or **Transit Gateway** (an ASN, for example 64512) and two **customer gateways** — one for each GCP interface;
- two AWS VPN connections with two tunnels each = **4 tunnels**, with a BGP session over each. The failure of any tunnel or of one side doesn't break connectivity — routes switch over via BGP.

**BGP** (Border Gateway Protocol — the protocol routers use to exchange the lists of their networks) spares you manual static routes here: Cloud Router advertises the GCP VPC subnets, AWS advertises the VPC subnets, and when a tunnel fails the route disappears automatically.

What gets asked and where it breaks:
- **non-overlapping CIDRs**: `10.0.0.0/16` in both clouds — routing is impossible, and redoing the networks takes weeks. An address plan for all clouds and offices is drawn up in advance;
- **MTU**: an IPsec tunnel adds headers, and a 1500-byte packet no longer fits. The symptom — SSH and `ping` work, while large HTTP responses and TLS handshakes hang. Fixed by MSS clamping (limiting the TCP segment size on the gateway) and an MTU below 1500 on the application side;
- **DNS between clouds**: Route 53 Resolver endpoints ↔ Cloud DNS forwarding (see the "Networking" topic, the DNS section);
- **cost**: outgoing traffic is paid in both clouds; chatty services (synchronous calls between clouds on every request) are expensive and slow, so cross-cloud calls are made asynchronous and batched;
- **security**: GCP firewall rules and AWS Security Groups allow only the needed subnets and ports; a tunnel is no reason to open everything.

### Kafka and Amazon MSK
Kafka, Amazon MSK, Strimzi and choosing between them are in a separate topic: [Kafka → Kafka in the cloud and Kubernetes](#19-kafka.md/s13). Comparing Kafka with SQS, SNS, Kinesis and RabbitMQ (and Amazon MQ) — [Kafka → Kafka and other brokers](#19-kafka.md/s14).

### Security beyond IAM and Security Groups
To the question "how do you protect AWS", the answer "IAM and Security Groups" sounds weak. A strong answer goes in layers, with an example of what each layer protects against:

| Layer | Tools | Example threat |
|---|---|---|
| Organization | SCPs (deny regions, disabling CloudTrail, deleting backups), **RCPs** (Resource Control Policies — restrictions on resources, for example "the organization's buckets can't be read from outside the organization"), separate accounts for prod, logs and backups | a stolen admin key of a dev account doesn't allow touching prod and the logs |
| Identity | IAM Identity Center instead of IAM users, MFA, temporary credentials, permission boundaries (a ceiling of permissions for roles that teams create themselves), IAM Access Analyzer (finds resources accessible from outside and unused permissions) | a long-lived key leaked to git |
| Network | private subnets without public IPs, NACLs as a second layer, **VPC endpoints with an endpoint policy** (S3 can be accessed only for your own buckets), PrivateLink between accounts, **AWS Network Firewall** or a proxy for filtering outgoing traffic by domain | exfiltrating data to someone else's server, malware calling its command-and-control server |
| Perimeter | WAF (SQL injection, bots, rate limiting), Shield Advanced (DDoS protection), CloudFront with Origin Access Control | a DDoS on a public API, password brute force |
| Data | KMS with customer managed keys and key policies, S3 Block Public Access at the account level, Object Lock, Macie (finds personal data in S3), encryption in transit (TLS) | an accidentally public bucket, a snapshot read by outsiders |
| Instances and containers | IMDSv2 mandatory, SSM Session Manager instead of SSH and bastions, Inspector (vulnerabilities in EC2, ECR, Lambda), patching via SSM Patch Manager | an SSRF vulnerability in the application steals the role's credentials via the metadata service |
| Detection and audit | CloudTrail for the whole organization into a separate log account, GuardDuty, Security Hub (a summary of findings and checks against the CIS and AWS Foundational standards), AWS Config rules (continuous checking of settings) | the compromise is noticed after half a year — or after 10 minutes |
| Response | EventBridge rules on GuardDuty findings → Lambda isolates the instance (a Security Group with no rules), revokes the role's sessions | the attacker manages to move further while waiting for a human |
| Secrets | Secrets Manager with rotation, no secrets in image environment variables or in plain text in Terraform state | a DB password in a public container image |

An SCP example that's often cited:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ProtectSecurityBaseline",
      "Effect": "Deny",
      "Action": [
        "cloudtrail:StopLogging",
        "cloudtrail:DeleteTrail",
        "guardduty:DeleteDetector",
        "backup:DeleteRecoveryPoint",
        "s3:PutAccountPublicAccessBlock"
      ],
      "Resource": "*",
      "Condition": {
        "ArnNotLike": { "aws:PrincipalArn": "arn:aws:iam::*:role/SecurityAdmin" }
      }
    }
  ]
}
```
Even the account's administrator can't turn off auditing and threat detection or delete backups — only the security team's special role can.

**IMDSv2 and SSRF.** SSRF (Server-Side Request Forgery — the application makes an HTTP request to an arbitrary address at the attacker's request) is the classic way to steal keys: a request to `http://169.254.169.254/latest/meta-data/iam/security-credentials/` returns the instance role's temporary keys. IMDSv2 requires first obtaining a token with a PUT request with a special header, which an ordinary SSRF can't do. It's enabled at the account level as the default for new instances and checked by the AWS Config rule `ec2-imdsv2-check`.

### 🧪 Mini task

**Task.** Hand out temporary access to an object in a private bucket with a 60-second presigned URL. Download the file with it right away and try again after 70 seconds.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
b=lab-presign-$(date +%s)
aws s3 mb s3://$b --region us-east-2 > /dev/null
echo "report 2026-10" | aws s3 cp - s3://$b/report.txt
url=$(aws s3 presign s3://$b/report.txt --expires-in 60 --region us-east-2)
curl -s "$url"
curl -s -o /dev/null -w '%{http_code}\n' "https://$b.s3.us-east-2.amazonaws.com/report.txt"
sleep 70
curl -s -o /dev/null -w '%{http_code}\n' "$url"
aws s3 rb s3://$b --force > /dev/null
```

Expected output:

```text
report 2026-10
403
403
```

Breakdown: a presigned URL carries a signature made with your credentials and an expiry. Anyone holding the link gets the object without their own AWS rights, but only while the link is valid. The direct unsigned request got 403: the bucket is private. After 70 seconds the signed link returned 403 too (`Request has expired`). That's how files are handed to clients and uploads go straight into S3 (presigned PUT) without passing through your backend. A link can't outlive the credentials that signed it: temporary SSO credentials cap it at the session lifetime. You need configured AWS credentials. Your IDs and values will differ.

</details>

## Interview questions

**An instance in a private subnet can't download packages. What do you check?**
The subnet's route table: is there a `0.0.0.0/0` route to a NAT Gateway. The NAT Gateway is working and is located in a **public** subnet with a route to the Internet Gateway. The Security Group allows outbound traffic. The NACL allows outbound traffic and replies on ephemeral ports. DNS works in the VPC (`enableDnsSupport`). If only S3 or ECR is needed — add a VPC endpoint.

**How does a Security Group differ from a NACL?**
An SG is stateful, on a network interface, has allow rules only, and can reference other SGs. A NACL is stateless (reply traffic must be allowed explicitly), per subnet, with allow and deny rules evaluated in order of their numbers.

**How do you give a pod in EKS access to S3?**
IRSA or EKS Pod Identity: an IAM role with a policy for the specific bucket, linked to the pod's ServiceAccount. Not via the node role — otherwise all pods on that node get access. Traffic to S3 — via a Gateway endpoint.

**How do you design a resilient web application in AWS?**
Route 53 → CloudFront with WAF → an ALB across several AZs → the application in an Auto Scaling Group or EKS across three AZs → RDS Multi-AZ (or Aurora) → ElastiCache with replicas → static content in S3 via CloudFront. A NAT Gateway in each zone. Backups with PITR and a copy in another region. Everything in IaC, monitoring and alerts, regular failure testing.

**How do you reduce the AWS bill?**
Rightsizing, Savings Plans for a constant load, Spot for stateless workloads and CI, Graviton, gp3 instead of gp2, S3 lifecycle policies, VPC endpoints instead of NAT for S3 and ECR, turning dev off on a schedule, deleting unused disks, snapshots and Elastic IPs, tags and budgets for visibility.

**Someone deleted a resource in production. How do you find out who?**
CloudTrail: Event history in the console (90 days) or an Athena query over the CloudTrail logs in S3 by event name (`DeleteDBInstance`, `TerminateInstances`) and resource ID.

**How do you prevent a public S3 bucket?**
Block Public Access at the account level, an SCP forbidding turning it off, AWS Config rules for detection, IaC checks in CI (checkov, trivy — static analysers that find public buckets, open Security Groups and other dangerous settings in Terraform code before `apply`).

**How does Multi-AZ differ from a Read Replica in RDS?**
Multi-AZ is for availability: a synchronous standby in another zone that can't be read, with automatic failover on failure without changing the DNS name. A Read Replica is for scaling reads: an asynchronous copy, readable, can be in another region, failover is manual.

**What is an IAM role, and why is it better than a user with keys?**
A set of permissions without permanent credentials: it's assumed for a period and issues temporary credentials. There's nothing to steal for long and nothing to rotate. Who can assume the role is explicitly described in the trust policy.

**How does AWS decide whether to allow a request?**
Denied by default; an explicit Deny in any policy denies it for good; an Allow in an identity-based or resource-based policy is needed; and it must fit within the SCPs, the permission boundary and the session policy.

**How does cross-account access work?**
A role in account B with a trust policy that allows a principal from account A, and the `sts:AssumeRole` permission on that role for the principal in A. Or a resource-based policy on the resource (a bucket, a KMS key) with a permission for account A. Permission is needed on both sides.

**Why is the NAT Gateway expensive, and how do you cut costs?**
You pay both for the running time and for every gigabyte of processed traffic. The main traffic is often pulling images from ECR and calls to S3. A Gateway endpoint for S3 (free) and interface endpoints for ECR take this traffic off the NAT.

**What are RPO and RTO? What DR strategies are there?**
RPO is the acceptable data loss measured in time, RTO is the acceptable recovery time. Strategies: Backup & Restore, Pilot Light, Warm Standby, Multi-site active/active — from cheap and slow to expensive and fast.

**How does SQS differ from SNS?**
SQS is a queue: a message is received and processed by one consumer, and messages are stored until processed. SNS is broadcasting: a message is delivered to all subscribers at once. They're often combined: SNS fans out into several SQS queues.

**Why does EKS run out of IP addresses, and what do you do?**
The VPC CNI gives every pod a real IP from the subnet, and nodes reserve addresses in advance (a warm pool — a stock of free IPs on the node so new pods start quickly). Fixes: large subnets, prefix delegation, an extra CIDR (`100.64.0.0/10`) for pods via custom networking (a VPC CNI setting in which pods get IPs from separate subnets rather than the nodes' subnets).

**Why IMDSv2?**
IMDS hands out the instance role's temporary credentials. In version 1 they can be stolen via an SSRF vulnerability in an application (making the application do a GET to `169.254.169.254`). IMDSv2 requires first getting a token with a PUT request with a special header, which SSRF usually doesn't allow.

**Why is there no EC2 memory metric in CloudWatch?**
The hypervisor sees the instance's CPU, network and disk usage but doesn't know how the OS inside uses memory. Memory and disk usage are collected by the CloudWatch Agent installed in the instance and sent as custom metrics.

**How do you speed up uploads of large files to S3 from all over the world?**
Multipart upload (parallel upload in parts) and S3 Transfer Acceleration (uploading via the nearest edge location and then over the AWS network). For regular transfers from a data centre — DataSync (a managed service for copying data between storage systems), for tens of terabytes without a link — Snowball (a physical storage device AWS ships to you; you load the data and send it back).

**How do SSE-S3, SSE-KMS and SSE-C differ?**
SSE-S3 — S3 manages the keys entirely, nothing needs to be done. SSE-KMS — the key is in KMS: you can control access to the key, see every use in CloudTrail, separate keys by project. SSE-C — the client passes the key in every request, and AWS doesn't store it; losing the key means losing the data.

**A KMS key was deleted by accident. What do you do?**
Deletion is deferred: the key stays in the pending deletion state for 7 to 30 days. Cancel the deletion via `CancelKeyDeletion` and re-enable the key. After the waiting period ends, data encrypted with that key can't be recovered.

**What kinds of Placement Groups are there, and when do you use each?**
Cluster — minimal latency between instances in one AZ (HPC). Spread — each instance on a separate rack, up to 7 per AZ (a few critical servers). Partition — partitions without shared racks for large distributed systems like Kafka and Cassandra.

**How does a Dedicated Host differ from a Dedicated Instance?**
Both run on hardware dedicated to your account only. A Dedicated Host gives you control over a specific physical server (the sockets and cores are visible, instances can be pinned) — needed for per-core and per-socket licences. A Dedicated Instance doesn't give that control.

**How do you connect a data centre to AWS?**
Site-to-Site VPN — fast and cheap over the internet. Direct Connect — a dedicated link with stable latency and high bandwidth, slow to provision, unencrypted by itself. Typically: Direct Connect as the primary link and a VPN as the backup, attached via a Transit Gateway if there are many VPCs.

**SQS, SNS or Kinesis?**
SQS — a task queue: each message is processed by one consumer. SNS — broadcasting an event to all subscribers. Kinesis — a stream of events with retention that several consumers read independently and can re-read; ordered within a shard.

**Lambda brought the database down. Why, and how do you fix it?**
During a load spike Lambda starts hundreds of instances, and each one opens connections to the database — the database hits its connection limit. Solutions: RDS Proxy (a connection pool), reserved concurrency to limit the number of instances, buffering via SQS.

**How do you serve private files from S3 via CloudFront?**
Close the bucket and allow reads only by CloudFront via Origin Access Control, and give users signed URLs or signed cookies with a limited validity period.

**An application on EC2 sometimes loses packets and gets timeouts, although CPU and network aren't loaded. What will you check?**
The instance limits that aren't visible in the usual metrics: `ethtool -S eth0 | grep allowance` — packets per second, conntrack, link-local (frequent DNS queries without a cache). Then drops in the host itself (`/proc/net/softnet_stat`, `nstat`) and in a Security Group with connection tracking. Solutions: a larger or network-optimised type, a local DNS cache, fewer connections.

**How do you update the image on a hundred instances in an Auto Scaling Group without downtime?**
Validate the new AMI in the pipeline, update the Launch Template (or the SSM parameter it references) and start an Instance Refresh: a minimum healthy percentage, warm-up, checkpoints (a 10% canary), alarms and auto-rollback. For long-lived connections — a lifecycle hook on termination that takes the instance out of the load balancer.

**How do you choose between EKS, ECS and Lambda?**
By the nature of the load and the team. Lambda — events and uneven load: you pay for execution and administer nothing, but there are 15 minutes per invocation, cold start and difficulties with DB connections. ECS — containers without Kubernetes: no cluster to upgrade, deep AWS integration, but AWS only and fewer tools around it. EKS — when there are many services, the ecosystem is needed (operators, GitOps, mesh) and portability across clouds and on-prem, but it needs a team that understands Kubernetes and yearly version upgrades. A mixed setup (the main part in EKS or ECS, events in Lambda) is normal.

**RDS or your own database — what are the arguments?**
RDS gives backups with PITR, failover, patches and monitoring out of the box; it costs more per instance but saves the team's time; the limitations are no access to the OS and superuser, only supported versions and extensions. Your own DB on EC2 or in Kubernetes (the CloudNativePG operator) is needed for special extensions and settings, multi-cloud and on-prem, or when the price difference is very noticeable at large volumes; but backups, failover and upgrades become your responsibility.

**How do you back up databases and how do you make sure a backup works?**
A combination: snapshots for a fast full restore, PITR (automatic in RDS; for your own PostgreSQL — pgBackRest or WAL-G with a WAL archive) for rolling back to the moment before a mistake, logical dumps for migration and restoring individual tables. Copies go to another region and to a separate account with deletion protection. A replica isn't a backup: an erroneous DELETE is replicated instantly. Verification is a regular automatic restore (a script or AWS Backup restore testing), check queries after the restore and metrics: the restore time and the age of the last successful backup.

**How do you make backups that survive a compromise of the prod account?**
Copy them to a separate backup account where prod has no rights to delete (AWS Backup copy actions between the organization's accounts), enable Backup Vault Lock in compliance mode or S3 Object Lock, forbid deleting backups and disabling plans via an SCP, encrypt with customer managed KMS keys that the backup account has access to. Plus a copy in another region and regular restore checks.

**How do you organise backups to another region and to another cloud?**
To another region: AWS Backup with a copy action to a vault in another region, cross-region automated backups for RDS or Aurora Global Database, S3 Cross-Region Replication (with Batch Replication for old objects), replication of ECR and secrets. To another cloud: Storage Transfer Service or rclone for S3 objects → GCS, logical dumps or pgBackRest with a second repository in GCS for databases, Velero with storage in GCS for Kubernetes. Take into account the cost of outgoing traffic and regularly rehearse the restore on the other side.

**How do you build DR for a service in EKS?**
The cluster and add-ons come from Terraform and GitOps, so they can be brought up in another region in tens of minutes; application manifests are already in git. Data lives outside the cluster in managed services with cross-region replication (Aurora Global Database, S3 CRR, DynamoDB Global Tables), and state inside the cluster is backed up via Velero, with file system backup for another region. Images — ECR replication, secrets — Secrets Manager replicas, the standby region's quotas raised in advance, switching — Route 53 failover or Application Recovery Controller. And regular drills measuring RTO and RPO.

**How do you connect AWS and GCP networks?**
To start with — HA VPN in GCP and Site-to-Site VPN in AWS: 4 IPsec tunnels with BGP via Cloud Router and a Transit Gateway or VGW, so the failure of any tunnel doesn't break connectivity. For large flows — a partner link (Equinix, Megaport) or Cross-Cloud Interconnect. Mandatory: non-overlapping CIDRs, accounting for MTU (MSS clamping), DNS via Route 53 Resolver endpoints and Cloud DNS forwarding, minimal firewall rules and Security Groups, accounting for the cost of outgoing traffic.

**How do you protect AWS infrastructure besides IAM and Security Groups?**
In layers. Organization: separate accounts, SCPs and RCPs. Identity: Identity Center, MFA, temporary credentials, Access Analyzer. Network: private subnets, VPC endpoints with policies, filtering outgoing traffic via Network Firewall. Perimeter: WAF and Shield. Data: KMS with customer managed keys, Block Public Access, Object Lock, Macie. Instances: IMDSv2, SSM instead of SSH, Inspector. Detection: an organization-wide CloudTrail into a separate account, GuardDuty, Security Hub, Config rules. Response: automatic isolation on findings via EventBridge and Lambda.
