# Troubleshooting cases and behavioral

Practical "something broke, what will you do" tasks and behavioral questions. What's tested here isn't knowledge of individual commands but **the way you think**: how you structure the search, what you do first, how you communicate and what you change afterwards. Each case is broken down step by step: first actions, hypotheses, how to check them, common causes and prevention.

## How to answer a case

The interviewer almost never expects one "right answer". What matters to them is hearing your train of thought. A good answer structure:

**1. Clarify the context.** Ask 2–3 questions before rushing to fix things:
- What exactly doesn't work, and how does it show (errors, slowness, complete unavailability)?
- The scale: all users or some, one region, one service?
- Since when? What changed shortly before (a deployment, a config, the infrastructure, the load)?
- Which tools are available: metrics, logs, traces?

**2. Restore first, investigate later.** If users are affected, the first goal is to get things working again: roll back the last deployment, shift traffic, scale, turn off a feature. Saying this out loud is a strong signal for the interviewer.

**3. Move systematically.** By layers (network → host → container → application → dependencies) or along the request path (DNS → load balancer → service → pod → database). At every step, **name a specific command or metric** and what you expect to see.

**4. Form hypotheses and test them.** "If the cause is X, I'll see Y. I'll check with command Z."

**5. Finish with prevention.** What to change so it doesn't happen again and so that next time it's detected faster: an alert, a test, automation, a process change, a postmortem.

## Cases

### After a deployment the site returns 502
**First:** roll back — `helm rollback`, `kubectl rollout undo deploy/api` or revert the commit in the GitOps repository. A 502 after a deployment is almost always related to the deployment.

**Then figure out** what broke in the new version:
1. **Are the pods ready?** `kubectl get pods -l app=api` — Running and `1/1 Ready`? If CrashLoopBackOff — `kubectl logs --previous` (the new version crashes at startup: a missing environment variable, can't connect to the database).
2. **Does the service have addresses?** `kubectl get endpointslices -l kubernetes.io/service-name=api`. Empty — the pods aren't Ready, or the labels changed in the new version and the service selector doesn't find them.
3. **Does the port match?** The service's `targetPort` and the port the application actually listens on. A typical mistake — the application started listening on 8080 instead of 80, or on `127.0.0.1` instead of `0.0.0.0`. Check from inside the pod: `kubectl exec -it <pod> -- ss -tlnp`.
4. **The load balancer:** does the ALB target group health check pass? Has the health check path changed?
5. **The ingress controller's logs** or the ALB access logs: which backend exactly returns the error.
6. **Keep-alive timeouts:** if the 502s are rare and random — the backend closes keep-alive connections earlier than the load balancer. The application's keep-alive timeout must be longer than the ALB's idle timeout (60 seconds by default).

**Prevention:** a readiness probe that checks real readiness; canary or gradual rollouts with automatic rollback on error metrics; smoke tests after deployment; `helm --atomic`.

### The application in a pod restarts periodically
1. `kubectl describe pod <pod>` → the `Last State` section: **Reason** and **Exit Code**.
2. **OOMKilled, code 137** — the memory limit was exceeded. Look at the pod's memory graph (`container_memory_working_set_bytes`): if it grows steadily up to the limit — a memory leak; if there are sharp spikes — processing large volumes. For .NET and the JVM — check that the runtime respects the container limit (heap settings). The fix — raise the limit or fix the leak.
3. **Error, code 1 and others** — the application crashes by itself: `kubectl logs <pod> --previous` (the logs of the previous run).
4. **Restarts without errors in the logs** — the **liveness probe** is killing it: Events will show `Liveness probe failed`. Causes: the check timeout is too short, the check endpoint is heavy and doesn't respond in time under load, or liveness checks dependencies (the database), and when they have problems all the pods restart. Fixes: increase `timeoutSeconds` and `failureThreshold`, a lightweight liveness endpoint, a startupProbe for slow startup.
5. **Code 143** — a normal stop (SIGTERM): the pod was evicted or is being moved; check the node's events and Karpenter.

### API latency rose sharply
1. **What changed?** A deployment, traffic growth, a configuration change, the work of a neighbouring system? Match the time the latency rose against events.
2. **The service's golden signals:** has traffic grown? Are errors growing? One endpoint or all of them?
3. **Traces:** which span got longer — the service itself, the database, an external call? This narrows the search immediately.
4. **If it's the database:** slow queries (`pg_stat_statements` — a PostgreSQL extension with statistics on each query: how many times it ran and how long it took; the slow query log — a log of queries longer than a set threshold), locks, an exhausted **connection pool** (queries wait for a free connection), a new query without an index after a deployment, disk load on the database.
5. **If it's the service itself:** **CPU throttling** (`container_cpu_cfs_throttled_periods_total` — the pod hits its CPU limit), garbage collection (GC pauses), an exhausted thread pool.
6. **Infrastructure:** a "noisy neighbour" on the node, exhausted CPU credits on a burstable instance (t3), steal time, DNS latency (CoreDNS problems, ndots).

### An EKS node went NotReady
1. `kubectl describe node <node>` → **Conditions**: `MemoryPressure`, `DiskPressure`, `PIDPressure`, `Ready=False` with a reason. **Events.**
2. Get onto the node (SSM Session Manager or `kubectl debug node/<node>`):
   - `journalctl -u kubelet` — why kubelet doesn't report: it can't reach the API, certificate errors, the runtime doesn't respond;
   - `df -h` — the disk is full of images and container logs;
   - `free -m`, `dmesg -T | grep -i oom` — a lack of memory, the OOM killer killed kubelet or the runtime.
3. **Networking:** the node lacks IP addresses in the subnet (VPC CNI), Security Groups between the nodes and the control plane don't work.
4. **EC2:** the instance's status checks in the console, a spot instance being reclaimed.
5. **What happens to the pods:** after a while (about 5 minutes) Kubernetes marks them for deletion and recreates them on other nodes (if they're managed by controllers).
6. **The fix:** cordon and drain the problematic node, then replace it (with Karpenter or a managed node group this is quick). Nodes should be disposable: "fixing" a specific node usually isn't worth it.

**Prevention:** log rotation, kubelet image garbage collection (`imageGCHighThresholdPercent`), a sufficient disk size, alerts on node disk usage.

### Pods are stuck in Pending, "the cluster is full"
1. `kubectl describe pod <pod>` → Events will tell you the reason:
   - `Insufficient cpu` / `Insufficient memory` — the nodes don't have enough room for the **requests**. Check whether the requests are inflated (the pod asks for 4 CPU but uses 0.2), and why the autoscaler didn't add a node.
   - `didn't match Pod's node affinity/selector` or `had untolerated taint` — placement constraints.
   - `pod has unbound immediate PersistentVolumeClaims` or `volume node affinity conflict` — the disk is in one AZ while the free nodes are in another (`volumeBindingMode: WaitForFirstConsumer` is needed).
   - `Too many pods` — the per-node pod limit has been reached (in EKS it depends on the instance type and IP addresses).
2. **The autoscaler:** the logs of Karpenter or Cluster Autoscaler — why it didn't create a node: the NodePool or Auto Scaling Group limits have been reached, the required instance type isn't available in the AZ (especially spot and GPU), the account's EC2 quotas are exhausted.
3. **The namespace's ResourceQuota** is exhausted — the pod won't even be created, and the error will be in the ReplicaSet's events.

### `terraform apply` failed halfway
1. **Don't panic:** the resources that managed to be created are already recorded in the state.
2. Read the error: an AWS limit or quota, the role lacks permissions, a wrong parameter, a resource with that name already exists, a dependency isn't ready yet.
3. If the state stayed locked (the process was killed) — make sure nobody is running apply, and `terraform force-unlock <ID>`.
4. Fix the cause and run `terraform plan` again — Terraform shows only what's left to do.
5. If a resource was created in AWS but didn't make it into the state (a drop at the moment of creation) — the next apply fails with "already exists": import it (an `import` block).

### You need to rename a resource or change a CIDR without downtime
- **Renaming or moving into a module** — a `moved` block: Terraform will understand it's the same object.
- **A VPC's CIDR can't be changed** — you can add a secondary CIDR and new subnets and gradually move the workload.
- **An attribute that causes recreation** (`forces replacement`): for stateless resources — `create_before_destroy` (create the new one first, then delete the old one); for stateful ones (a database) — migrating the data and switching over, for example via a snapshot or a replica.

### A server's disk is 100% full
1. **Where exactly:** `df -h` — which filesystem; `df -i` — maybe the inodes ran out (many small files), not the space.
2. **What takes up the space:** `du -xh / --max-depth=1 2>/dev/null | sort -h`, then descend into the largest folder.
3. **Deleted but open files:** `lsof +L1` — if `du` shows less than `df`. The application holds a deleted log: restart the process or truncate the file via `/proc/<pid>/fd/<n>`.
4. **Common culprits:** logs without rotation (`/var/log`, Docker logs in `/var/lib/docker/containers`), the systemd journal (`journalctl --vacuum-size=500M`), old Docker images (`docker system prune`), core dumps, temporary files, backups put there "temporarily".
5. **Free up space carefully** (without deleting what a running process is writing to), then deal with the cause.
6. **Prevention:** logrotate, Docker log size limits, an alert with a fill-up forecast (`predict_linear`), a sufficient disk size.

### A secret leaked into a public repository
1. **Rotate (revoke) the secret immediately.** This is the main and first thing. Deleting it from git doesn't help: the repository may have been cloned, and bots scan public repositories within minutes.
2. **Check whether it was used:** CloudTrail for AWS keys (calls with that key from unfamiliar IPs), the service's access logs.
3. If it was used — it's a security incident: assess the damage (created resources — crypto mining; data access).
4. **Remove it from git history:** `git filter-repo` or BFG, a force push, ask everyone to re-clone the repository.
5. **Prevention:** pre-commit with gitleaks, secret scanning on the platform (GitHub, GitLab), secrets only in Vault, Secrets Manager or protected CI variables, short-lived credentials instead of permanent keys (OIDC, roles).
6. A blameless postmortem.

### The pipeline now takes 40 minutes instead of 10
1. **Which jobs grew?** Compare job durations before and after — the problem is usually in one or two.
2. **Time in the queue or execution time?** If jobs wait a long time for a runner — there aren't enough runners or they don't scale.
3. **The cache stopped working:** the cache key changed, new runners without a local cache, the Docker layer cache isn't used (the order in the Dockerfile changed, the base image changed).
4. **The tests grew** — split them into parallel parts.
5. **External dependencies:** a slow package registry, Docker Hub limits, network problems on the runners.
6. **Solutions:** a cache with the right key, `needs` for parallelism, running only what changed, ready-made images with tools, more runners.

### Service A can't reach service B in another namespace
1. **The DNS name:** from another namespace you need the full name `b.ns-b.svc.cluster.local` (or at least `b.ns-b`). Plain `b` is looked up in your own namespace.
2. **Check from inside pod A:** `nslookup b.ns-b.svc.cluster.local`, `curl -v http://b.ns-b:80`.
3. **Does service B have addresses:** `kubectl get endpointslices -n ns-b` — the selector matches the pods, and the pods are Ready.
4. **NetworkPolicy:** namespace B may have a default deny — an allow policy for traffic from namespace A is needed (`namespaceSelector`). And egress from A, if it's restricted.
5. **The port:** the service port (`port`) is used, not the container's.
6. **Service mesh:** mTLS authorisation policies may block the call.

### Draw an HLD of the full path of a Node.js application: from the repository to running in the cloud
A High-Level Design is a diagram in large blocks, without configuration details. Draw from left to right and explain **why** each block is there. An example for AWS:

**1. Code and review.** A repository in GitLab or GitHub, a protected `main` branch, merge requests with mandatory review and CODEOWNERS (a file that assigns mandatory reviewers to paths in the repository). Pre-commit: eslint (a JavaScript linter), prettier (auto-formatting), gitleaks (secret detection).

**2. CI (on every MR):**
- `npm ci` with a cache keyed by `package-lock.json`;
- a linter, unit tests, a coverage report;
- SAST (code analysis for vulnerabilities) and a dependency check (`npm audit`, Snyk — a service that finds known vulnerabilities in libraries, or the platform's built-in scanners);
- a multi-stage Docker image build: the build stage on `node:22`, the final image on `node:22-slim` or distroless, running as non-root, `npm ci --omit=dev`;
- a Trivy image scan, publishing to **ECR** tagged with the commit SHA, signing with cosign.

**3. CD:**
- after the merge, CI updates the image tag in the **configuration repository** (Helm values or a Kustomize overlay);
- **ArgoCD** in the cluster sees the change and rolls it out: dev — automatically, prod — via a merge request to the configuration or a manual sync;
- for critical services — a canary via Argo Rollouts with analysis of error metrics.

**4. Infrastructure (Terraform, a separate pipeline):** a VPC across three AZs with public and private subnets, **EKS** with Karpenter, **RDS** or Aurora Multi-AZ, **ElastiCache** for sessions and cache, S3 for files, Route 53 and ACM certificates, WAF.

**5. The user's request path:** Route 53 → **CloudFront** (static content from S3, cache) → an **ALB** with WAF and TLS → Ingress (AWS Load Balancer Controller) → Service → the Node.js pods (HPA on CPU or RPS, PDB, readiness and liveness probes, graceful shutdown on SIGTERM) → ElastiCache and RDS.

**6. Secrets and access:** database passwords in Secrets Manager → External Secrets Operator → a Kubernetes Secret; pod access to S3 via IRSA or Pod Identity; CI access to AWS via OIDC.

**7. Observability:** application metrics via `prom-client` or OpenTelemetry → Prometheus and Grafana; JSON logs to stdout → Fluent Bit → Loki or CloudWatch; OpenTelemetry traces → Tempo or X-Ray; SLOs and burn rate alerts in Alertmanager → Opsgenie or Slack.

**8. Reliability and cost:** several replicas in different AZs, RDS backups with PITR, Karpenter with spot for stateless workloads, tags and budgets, runbooks and on-call.

Expect follow-up questions on any block: "why ArgoCD rather than deploying from CI?", "how will you roll back?", "where are the secrets stored?", "what happens if one AZ goes down?". The answers are in the corresponding topics.

### How would you design a platform for 30 microservices from scratch?
An open question that tests your breadth of view. Go through the layers and for each name a solution and the reason:
- **Accounts and environments:** AWS Organizations, separate accounts for prod, stage, dev, security and shared services; SCPs; people sign in via SSO.
- **Networking:** a VPC per environment with non-overlapping CIDRs, private subnets across three AZs, a NAT in each zone, VPC endpoints, a Transit Gateway for connectivity.
- **IaC:** Terraform with versioned modules, separate states per environment and layer, CI with plan in the MR and apply after merge, access via OIDC.
- **Compute:** EKS, Karpenter (spot and on-demand, Graviton), the base add-ons (Load Balancer Controller, ExternalDNS, cert-manager, External Secrets).
- **Delivery:** a single CI template for the services (build, tests, scanning, an image in ECR tagged with the SHA), GitOps via ArgoCD and ApplicationSet, a shared Helm chart for the services, progressive delivery for the critical ones.
- **Secrets:** Secrets Manager and External Secrets Operator, IRSA or Pod Identity for pods' access to AWS.
- **Data:** RDS or Aurora Multi-AZ, ElastiCache, backups with PITR and a copy in another region.
- **Observability:** Prometheus (or Amazon Managed Prometheus) and Grafana, Loki for logs, Tempo or X-Ray for traces, OpenTelemetry for instrumentation, SLOs and burn rate alerts, on-call.
- **Security:** Pod Security, NetworkPolicy, image and IaC scanning, image signing, GuardDuty, CloudTrail auditing.
- **Cost:** tags, budgets, per-team reports.
- **Developer experience:** a "golden path" (the recommended and fully automated way to do a typical thing, for example to launch a new service) — a template for a new service that immediately gets CI, deployment, monitoring and alerts; documentation and self-service (teams create environments and access themselves via a portal or templates, without a ticket to the platform team).

## Behavioral questions

### The STAR method
Answer behavioral questions using this scheme:
- **S — Situation:** the context in 1–2 sentences — where, what system, what scale.
- **T — Task:** what had to be done and what was hard about it. What **your** role was.
- **A — Action:** what **you** did (say "I", not "we"): the decisions, why exactly that way, which alternatives you rejected. This is the most important and detailed part.
- **R — Result:** the measurable outcome — numbers (time, money, availability, number of incidents) and what you learned.

An answer fits into 2–3 minutes. The interviewer will ask follow-up questions — be ready for details.

### Prepare 5–6 stories
Stories prepared in advance cover almost all behavioral questions. Think about your experience:
1. **The hardest incident** — how it was detected, how you looked for the cause, how service was restored, what was changed afterwards. (Fits questions about pressure, debugging, ownership.)
2. **Automation that saved time or money** — with numbers. (FinOps audits, automating image patching, pipelines.)
3. **A migration** — for example, moving microservices from a data centre to AWS or from Swarm to Kubernetes: how you planned it, what the risks were, how you minimised downtime, the outcome.
4. **A disagreement or conflict** with a colleague or manager — how you reached a decision. (Show that you rely on data, listen and negotiate.)
5. **Your mistake** — what happened, how you fixed it, what conclusions you drew. (Honesty and conclusions matter more than the mistake itself.)
6. **Introducing something new in the team** — standards, pre-commit, CI templates, an AI agent with MCP: how you convinced people and what came of it.

### Common questions
- Why are you leaving your current job? (Talk about what you're striving for, not about what you dislike.)
- Why DevOps and SRE? What do you like about this work?
- Tell me about a project you're proud of.
- How do you prioritise when an incident is on fire, someone asks for an urgent task and there's planned work?
- How do you explain a technical decision to the business or management?
- What do you do when you don't know the answer or the technology?
- What are you learning now? (These notes, AI and MCP, Kubernetes The Hard Way come in handy here.)
- How do you feel about on-call?

### Questions for the employer
At the end of an interview you're always asked "do you have any questions". Good questions show interest and help you understand whether the company is right for you:
- How is on-call organised: the rotation, how many incidents on average, is there compensation?
- How many environments and clusters are there, what share of the infrastructure is in IaC?
- How often and how do you deploy: GitOps, manual steps?
- Is there a culture of blameless postmortems?
- The team size, the ratio of developers to infrastructure engineers, who owns a service in production?
- What hurts the most right now, and what will I be working on in the first three months?
- How is success measured in this position?
