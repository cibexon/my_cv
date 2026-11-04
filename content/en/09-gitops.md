# Helm, Kustomize, ArgoCD (GitOps)

How to manage Kubernetes manifests for many services and environments and how to deliver them to a cluster, in depth: Helm (templates and packages), Kustomize (patches without templates), GitOps and ArgoCD. Each topic: what it is, why, how it works, examples with explanations and typical problems.

## What problem these tools solve

One service in Kubernetes is a Deployment, Service, Ingress, ConfigMap, HPA, PDB and ServiceAccount: 200–300 lines of YAML. In three environments (dev, stage, prod) they differ only a little: the number of replicas, resources, the domain, the image tag. And there are 30 services.

You can't copy the YAML for every service and environment: a change to the standard (for example, adding a securityContext) would have to be made in 90 places, and the copies would inevitably drift apart. You need a way to describe what's common once and the differences separately. Two main approaches:
- **Helm** — templates with parameters: one template and different parameter values;
- **Kustomize** — base manifests and patches on top of them for each environment.

### 🧪 Mini task

**Task.** Simulate life without templating: the stage manifest was copied to prod and edited by hand. Compare the two files with `diff` and find which differences are intended and which are accidental drift.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > stage.yaml <<'EOF'
replicas: 1
image: shop/api:1.4.0
env: stage
readinessProbe: /healthz
memoryLimit: 256Mi
EOF
cat > prod.yaml <<'EOF'
replicas: 3
image: shop/api:1.3.2
env: prod
readinessProbe: /health
memoryLimit: 256Mi
EOF
diff stage.yaml prod.yaml
```

Expected output:

```text
1,4c1,4
< replicas: 1
< image: shop/api:1.4.0
< env: stage
< readinessProbe: /healthz
---
> replicas: 3
> image: shop/api:1.3.2
> env: prod
> readinessProbe: /health
```

Breakdown: `replicas` and `env` should differ — they're environment parameters. But the different readinessProbe path and the lagging image version are drift: someone edited prod by hand, and stage no longer tests what will go to production. Across dozens of services such copies diverge unnoticed. Helm and Kustomize solve this: the common part is described once, and environments set only explicit differences (values files or an overlay) that are visible in review.

</details>

## Helm

### Core concepts
- **Chart** — a package: manifest templates, default values (`values.yaml`) and metadata.
- **Values** — parameters substituted into templates. Set via files (`-f values-prod.yaml`) and flags (`--set image.tag=1.4.2`).
- **Release** — an instance of a chart installed in a cluster under a specific name. One chart can be installed many times under different names.
- **Revision** — every install or upgrade of a release creates a new revision. The revision history is stored in the cluster (in Secrets in the release's namespace), which makes **rolling back** to any previous one possible.
- **Repository** — where charts are published: an HTTP repository or an OCI registry (a registry that stores artifacts in the same OCI format as container images — for example ECR or the GitLab registry where the images live).

### Chart structure
```
mychart/
  Chart.yaml            # the name, chart version, appVersion, dependencies on other charts
  values.yaml           # default values
  values.schema.json    # a JSON schema for validating values (optional but useful)
  templates/
    _helpers.tpl        # named templates: names, labels — reused in other files
    deployment.yaml
    service.yaml
    ingress.yaml
    hpa.yaml
    NOTES.txt           # text printed after installation
  charts/               # downloaded dependencies
```
```yaml
# Chart.yaml
apiVersion: v2
name: api
version: 1.3.0          # the CHART version (changes when the templates change)
appVersion: "2.7.1"     # the APPLICATION version (the default image tag)
dependencies:
  - name: redis
    version: "~19.0"
    repository: oci://registry-1.docker.io/bitnamicharts
    condition: redis.enabled   # install the dependency only if redis.enabled = true in values
```

### Templates
Templates are written in **Go templates** (the template engine built into Go) with the Sprig function library (about 100 ready-made functions: strings, lists, hashes, dates). Expressions go inside `{{ }}`.
```yaml
# values.yaml
replicaCount: 2
image:
  repository: registry.example.com/api
  tag: ""                 # empty — take appVersion from Chart.yaml
resources:
  requests: { cpu: 100m, memory: 128Mi }
env:
  LOG_LEVEL: info
ingress:
  enabled: false
  host: ""
```
```yaml
# templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "api.fullname" . }}          # a named template from _helpers.tpl
  labels:
    {{- include "api.labels" . | nindent 4 }}   # insert a block of labels indented by 4 spaces
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "api.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "api.selectorLabels" . | nindent 8 }}
      annotations:
        # a hash of the ConfigMap: when the config changes, the pod template changes → the Deployment restarts the pods
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
    spec:
      containers:
        - name: api
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          {{- with .Values.resources }}          # with: if the value isn't empty — enter the block, the dot = this value
          resources:
            {{- toYaml . | nindent 12 }}         # turn the structure from values into YAML indented by 12
          {{- end }}
          {{- if .Values.env }}
          env:
            {{- range $key, $value := .Values.env }}   # a loop over a map
            - name: {{ $key }}
              value: {{ $value | quote }}               # quote — in quotes so that "true" and "123" stay strings
            {{- end }}
          {{- end }}
```
What's important to understand:
- **`.Values`** — the values, **`.Chart`** — data from Chart.yaml, **`.Release.Name`**, **`.Release.Namespace`** — the release's name and namespace.
- **`{{-` and `-}}`** — strip whitespace and newlines to the left or right of the expression. The main source of errors in charts is broken YAML indentation. That's why `nindent` (a newline plus indentation) and `toYaml` are used.
- **`include`** calls a named template from `_helpers.tpl` (names and labels are described once).
- **`required`** — force a value to be set: `{{ required "ingress.host is required" .Values.ingress.host }}`.
- Whole objects can be included conditionally: the entire `ingress.yaml` file is wrapped in `{{- if .Values.ingress.enabled }} ... {{- end }}`.

**The `checksum/config` annotation** solves a common problem: pods aren't restarted when a ConfigMap changes (environment variables are read only at startup). The hash of the ConfigMap's content in an annotation of the pod template changes together with the config, the Deployment sees the template change and rolls the pods.

### Environments via values files
```
values.yaml          # shared default values
values-dev.yaml      # replicaCount: 1, small resources
values-prod.yaml     # replicaCount: 4, large resources, ingress.host: api.example.com
```
```bash
helm upgrade --install api ./mychart -n prod -f values-prod.yaml --set image.tag=2.7.2
# the order of precedence: values.yaml < -f files (in the order given) < --set
```

### Commands
```bash
helm repo add bitnami https://charts.bitnami.com/bitnami && helm repo update   # Bitnami — a popular catalogue of ready charts (PostgreSQL, Redis, Kafka)
helm search repo postgresql
helm show values bitnami/postgresql > pg-values.yaml   # see all of a chart's parameters

helm upgrade --install api ./mychart \      # install if it's not there, or upgrade if it is
  -n prod --create-namespace \
  -f values-prod.yaml \
  --set image.tag=2.7.2 \
  --atomic --wait --timeout 5m              # wait until ready; on failure — automatic rollback

helm template api ./mychart -f values-prod.yaml   # render the manifests locally without applying anything
helm lint ./mychart                                # check the chart
helm diff upgrade api ./mychart -f values-prod.yaml   # the helm-diff plugin: what will change in the cluster

helm list -A                                # all releases
helm history api -n prod                    # a release's revisions
helm rollback api 3 -n prod                 # roll back to revision 3
helm get values api -n prod                 # which values the release was installed with
helm get manifest api -n prod               # which manifests were actually applied
helm uninstall api -n prod

helm package ./mychart && helm push api-1.3.0.tgz oci://registry.example.com/charts
```
**`--atomic`** matters in CI: without it a failed rollout leaves the release in the `failed` state, while with it Helm itself rolls back to the previous working revision.

### Hooks
A **hook** is a chart resource (usually a Job) that Helm creates not together with the rest, but at a specific moment in the release lifecycle. Typical tasks: a DB schema migration before a new application version, a backup before an upgrade, cache warm-up or a notification after installation, a verification test.

All the points:
| Hook | When it runs |
|---|---|
| `pre-install` | after the templates are rendered, before any resources are created on `helm install` |
| `post-install` | after all resources are created during installation |
| `pre-upgrade` | before resources are updated on `helm upgrade` (the classic — **DB migrations**) |
| `post-upgrade` | after resources are updated |
| `pre-rollback`, `post-rollback` | before and after `helm rollback` |
| `pre-delete`, `post-delete` | before and after resources are deleted on `helm uninstall` |
| `test` | only on the `helm test <release>` command (checking that the release works) |

```yaml
# templates/db-migrate-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ .Release.Name }}-db-migrate
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "-5"                     # order among hooks of one phase: lower runs earlier
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  backoffLimit: 1                                   # don't retry the migration many times
  activeDeadlineSeconds: 600                        # kill it if it runs longer than 10 minutes
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: migrate
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"   # the same version as the application
          command: ["/app/migrate", "up"]
          envFrom:
            - secretRef:
                name: {{ .Release.Name }}-db
```
How it works:
- Helm creates the hook and **waits** until the Job finishes (for a Job or Pod — until it completes successfully or fails, for other kinds — only until it's created). If the Job failed, the release is marked `failed`, and for `pre-upgrade` the main resources aren't updated. The waiting time is `--timeout` (5 minutes by default);
- `hook-weight` is a string with a number; hooks of one phase run in ascending order of weight, and with equal weights — by kind and name;
- `hook-delete-policy`: `before-hook-creation` (delete the old Job before creating a new one — otherwise the second `upgrade` fails with "already exists"; this is the default behaviour if no policy is set), `hook-succeeded` (delete after success), `hook-failed` (delete after a failure — then the logs of the failed migration are lost, so it's usually not used);
- hooks **aren't part of the release**: `helm uninstall` doesn't delete them, and `helm rollback` doesn't undo their effect — a migration performed by `pre-upgrade` stays applied.

Pitfalls that get asked about:
- **Migrations and rollback.** Helm rolls back the Deployment, but not the DB schema. So migrations are written to be **backward compatible** (expand/contract: first add a column, roll out code that writes to both, and only in the next release remove the old one), so that the old application version works on the new schema.
- **Chicken and egg with `pre-install`.** The hook runs before the chart's resources are created, so a Secret or ConfigMap from the same chart doesn't exist yet — the Job won't start. Solutions: make the needed Secret a hook too with a lower weight, or take the secret from an external source (External Secrets, an already created secret).
- **Several replicas and parallel releases.** A hook runs once per release, but two parallel rollouts (two pipelines) start two migrations. The protection is a lock in the migration tool itself (Flyway, Liquibase, golang-migrate hold a lock in the DB) and forbidding parallel deploys in CI.
- **`--wait` and hooks.** `post-install` runs after the resources are created, but without `--wait` — without waiting for the pods to become Ready.
- **Long migrations** block the deploy and run into `--timeout`. Heavy data changes are moved out of hooks into separate jobs.

**Hooks and ArgoCD.** ArgoCD doesn't run `helm install`; it renders the templates and applies them itself, so it translates Helm hooks into its own: `pre-install` and `pre-upgrade` → `PreSync`, `post-install` and `post-upgrade` → `PostSync`, `post-delete` → `PostDelete` (in newer ArgoCD versions), `hook-delete-policy` → the corresponding ArgoCD deletion policies. ArgoCD makes no difference between install and upgrade: a `pre-install` hook runs **on every sync**, while `pre-rollback`, `post-rollback`, `test` and `pre-delete` aren't supported as Helm hooks. So for charts deployed by ArgoCD, hooks are written with ArgoCD annotations right away (`argocd.argoproj.io/hook: PreSync`) and made idempotent. The order within a phase is `argocd.argoproj.io/sync-wave`.

**`helm test`** is the `test` hook: a pod or Job that checks the installed release (for example, a `curl` to the service). Handy to run in CI right after `helm upgrade`:
```bash
helm upgrade --install shop ./chart -n prod --wait --timeout 10m
helm test shop -n prod --logs          # run the test hooks and show their logs
```

### Helmfile
A declarative description of a set of releases: which charts, with which values, in which environments. `helmfile apply` brings the cluster to the described state. Handy for infrastructure add-ons (ingress, cert-manager, monitoring) without GitOps.

### 🧪 Mini task

**Task.** Create a sample chart with `helm create`, lint it and render it without a cluster: with the default values and with `prod` values (3 replicas, its own image tag). Compare the replica count and image in the Deployment.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
helm create shop > /dev/null
helm lint shop | tail -1
printf 'replicaCount: 3\nimage:\n  tag: "1.27"\n' > values-prod.yaml
helm template web ./shop | yq 'select(.kind == "Deployment") | [.spec.replicas, .spec.template.spec.containers[0].image] | join(" ")'
helm template web ./shop -f values-prod.yaml | yq 'select(.kind == "Deployment") | [.spec.replicas, .spec.template.spec.containers[0].image] | join(" ")'
```

Expected output:

```text
1 chart(s) linted, 0 chart(s) failed
1 nginx:1.16.0
3 nginx:1.27
```

Breakdown: `helm template` renders templates locally, with no cluster — that's how charts are checked in CI and how you see exactly what will go to Kubernetes. Defaults live in the chart's `values.yaml`, and `-f values-prod.yaml` overrides only the keys it sets; everything else comes from the chart. With no tag set, the template uses `appVersion` from `Chart.yaml` (`1.16.0`).

</details>

## Kustomize

### The idea
No templates and no language of its own: ordinary valid Kubernetes manifests (the **base**) plus **overlays** for environments that **patch** the base. Built into kubectl: `kubectl apply -k <folder>`.

```
k8s/
  base/
    kustomization.yaml
    deployment.yaml
    service.yaml
  overlays/
    dev/
      kustomization.yaml
    prod/
      kustomization.yaml
      replicas-patch.yaml
```
```yaml
# base/kustomization.yaml
resources:
  - deployment.yaml
  - service.yaml
labels:
  - pairs:
      app.kubernetes.io/part-of: shop     # add a label to all resources
```
```yaml
# overlays/prod/kustomization.yaml
resources:
  - ../../base                            # take the base
namespace: prod                           # set the namespace on all resources
namePrefix: prod-                         # add a prefix to the names
images:
  - name: registry.example.com/api        # replace the image tag without editing the manifests
    newTag: "2.7.2"
patches:
  - path: replicas-patch.yaml             # a patch: change part of an object
configMapGenerator:
  - name: api-config
    literals:
      - LOG_LEVEL=warn
```
```yaml
# overlays/prod/replicas-patch.yaml — a strategic merge patch: a partial object merged with the base
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 4
  template:
    spec:
      containers:
        - name: api                       # the container is matched by name
          resources:
            limits: { memory: 1Gi }
```
```bash
kubectl kustomize overlays/prod          # see the resulting manifests
kubectl apply -k overlays/prod           # apply
```
**`configMapGenerator`** creates a ConfigMap with a **hash of its content in the name** (`api-config-7g2k9h`) and substitutes that name into the Deployment itself. The config changed — the name changed — the pod template changed — the pods rolled. The same effect as `checksum/config` in Helm.

### Helm or Kustomize
| | Helm | Kustomize |
|---|---|---|
| Approach | Templates with parameters | Patches on top of ordinary manifests |
| Strengths | Distributing ready-made packages (third-party charts), complex parameterisation, releases with history and rollback, hooks | Simplicity, no template language, manifests stay readable YAML |
| Weaknesses | Templates are hard to read and debug (indentation, conditions) | Complex logic and conditions are awkward to express, no releases or rollback |
| Typical use | Third-party components (ingress-nginx, cert-manager, monitoring), a company-wide chart for services | Your own services with small differences between environments |

They're often **combined**: Kustomize can render a Helm chart and patch the result (`helmCharts` in kustomization), and ArgoCD supports both.

### 🧪 Mini task

**Task.** Make a `base` with a Deployment and a `prod` overlay that adds a name prefix, an environment label, 3 replicas via a patch and a new image tag. Render the overlay and check the result.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && mkdir -p base overlays/prod
cat > base/deployment.yaml <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  replicas: 1
  selector:
    matchLabels: {app: web}
  template:
    metadata:
      labels: {app: web}
    spec:
      containers:
        - name: web
          image: nginx:1.25
EOF
echo 'resources: [deployment.yaml]' > base/kustomization.yaml
cat > overlays/prod/kustomization.yaml <<'EOF'
resources: [../../base]
namePrefix: prod-
labels:
  - pairs: {env: prod}
images:
  - name: nginx
    newTag: "1.27"
patches:
  - target: {kind: Deployment, name: web}
    patch: |
      - op: replace
        path: /spec/replicas
        value: 3
EOF
kustomize build overlays/prod | yq '[.metadata.name, .metadata.labels.env, .spec.replicas, .spec.template.spec.containers[0].image] | join(" ")'
```

Expected output:

```text
prod-web prod 3 nginx:1.27
```

Breakdown: Kustomize uses no templates: the base is plain valid YAML, and the overlay describes *what to change*. `namePrefix` changed the name, `labels` added a label, `images` swapped the tag without touching the Deployment YAML, and the JSON patch changed `replicas`. The base can be applied without the overlay. `kubectl kustomize overlays/prod` gives the same result — Kustomize is built into kubectl. By default `labels` doesn't add the label to the selector, which is right: a Deployment's selector can't be changed.

</details>

## GitOps

### Principles
GitOps is a delivery approach in which **the git repository is the single source of truth** about the desired state of the cluster.
1. **Declarative** — the whole state is described by manifests.
2. **Versioned** — the description lives in git: history, review, rollback via `git revert`.
3. **Pull** — **an agent inside the cluster** pulls the changes from git and applies them itself. CI doesn't go to the cluster.
4. **Continuous reconciliation** — the agent constantly compares the cluster with git and fixes discrepancies (drift).

### Push versus pull
**The push model** (classic CI/CD): the pipeline runs `helm upgrade` or `kubectl apply`.
- CI needs write access to the cluster — compromising it means compromising the cluster.
- Nobody notices manual changes in the cluster.
- To find out what's deployed, you have to look into the cluster.

**Pull model** (GitOps): ArgoCD or Flux (the two most popular GitOps agents for Kubernetes, both CNCF projects) in the cluster watches git.
- CI doesn't need cluster credentials: it only builds the image and updates the tag in git.
- **Drift is visible**: if someone changed a resource by hand, the agent shows OutOfSync and can restore the state.
- **Audit and rollback** — via the git history.
- **Recovery**: a new cluster is brought to the right state simply by connecting it to the same repository.

The downsides of GitOps: imperative steps (migrations, one-off operations) are harder to express; a separate mechanism is needed for secrets (they can't be put in git in plain text); an extra component in the cluster.

### A typical repository layout
- **The application repository** — the code and Dockerfile. CI: tests → building an image tagged with the SHA → publishing to the registry → **committing the new tag to the configuration repository** (for prod — a merge request approved by a human).
- **The configuration (deploy/gitops) repository** — Helm values or Kustomize overlays for all services and environments. ArgoCD watches it.

The separation is needed so that configuration changes don't trigger a code build and permissions on the production configuration can be restricted separately.

### 🧪 Mini task

**Task.** Set up a "GitOps repository" with a single parameter — the replica count. Increase it with a commit, then roll back with `git revert`, and after each step show the state described in git and what the history says about it.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && git init -q -b main && git config user.name dev && git config user.email dev@example.com
echo 'replicas: 2' > values-prod.yaml && git add . && git commit -qm "prod: initial 2 replicas"
state() { echo "git says: $(yq .replicas values-prod.yaml) replicas"; }
state
yq -i '.replicas = 5' values-prod.yaml && git commit -qam "prod: scale to 5 for sale"
state
git revert -q --no-edit HEAD
state
git log --format='%s'
```

Expected output:

```text
git says: 2 replicas
git says: 5 replicas
git says: 2 replicas
Revert "prod: scale to 5 for sale"
prod: scale to 5 for sale
prod: initial 2 replicas
```

Breakdown: in GitOps the cluster's desired state is the content of git, and an agent in the cluster (ArgoCD, Flux) continuously reconciles the cluster towards it. So every change is a commit (reviewed through a merge request, author and time visible), and a rollback is `git revert`, also a commit: history isn't rewritten, and you see who rolled back what and when. If someone changes replicas with `kubectl scale` bypassing git, the agent shows OutOfSync, and with self-heal restores the value from git.

</details>

## ArgoCD

### The main object — Application
An Application links a **source** (a repository, path, branch or Helm chart) with a **destination** (a cluster and namespace) and sets the sync policy.
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: api-prod
  namespace: argocd
spec:
  project: prod                                     # AppProject — permission restrictions (see below)
  source:
    repoURL: https://gitlab.com/mycompany/deploy.git
    targetRevision: main                            # a branch, tag or commit
    path: apps/api/overlays/prod                    # a Kustomize overlay (ArgoCD detects the type itself)
  destination:
    server: https://kubernetes.default.svc          # this same cluster
    namespace: api
  syncPolicy:
    automated:
      prune: true          # delete resources from the cluster that are no longer in git
      selfHeal: true       # revert manual changes in the cluster to the state in git
    syncOptions:
      - CreateNamespace=true
```

### An application's two statuses
- **Sync status** — whether the cluster matches git: **Synced** or **OutOfSync** (git has changes that haven't been applied yet, or someone changed a resource by hand).
- **Health status** — whether the resources are healthy: **Healthy**, **Progressing** (a Deployment is rolling out), **Degraded** (pods are crashing, the rollout failed), **Missing**.

An application can be Synced but Degraded: everything is applied as in git, but the new version doesn't start.

### Syncing
- **Manual** — ArgoCD shows OutOfSync, and a human applies it with the Sync button. Often done this way for prod.
- **Automatic** (`automated`) — changes are applied by themselves within a few minutes of the commit (or immediately with a webhook from git).
- **`prune`** — without it a resource removed from git stays in the cluster.
- **`selfHeal`** — without it manual changes in the cluster are only highlighted, not reverted.

### Order of application: sync waves and hooks
- **Sync waves** — the `argocd.argoproj.io/sync-wave: "-1"` annotation: resources are applied in waves in ascending order of number. For example, first CRDs and namespaces, then the application.
- **Hooks** — `argocd.argoproj.io/hook: PreSync` (run before syncing — database migrations), `PostSync` (after — smoke tests), `SyncFail`.

### ignoreDifferences
Some fields are changed not by git but by something in the cluster. Example: the number of replicas is changed by HPA. ArgoCD sees that `replicas` in the cluster doesn't match git and keeps showing OutOfSync (and with selfHeal it reverts the replicas, fighting HPA). Solutions: remove `replicas` from the Deployment manifest altogether, or configure it to be ignored:
```yaml
spec:
  ignoreDifferences:
    - group: apps
      kind: Deployment
      jsonPointers:
        - /spec/replicas
```

### Scaling to many applications and clusters
- **App of Apps** — one "root" Application points to a folder containing the manifests of other Applications. Add a file to the folder — a new application appears.
- **ApplicationSet** — generates Applications from a template using **generators**:
  - `list` — from an explicit list;
  - `git` (directories or files) — by folders in the repository: one application per `apps/*` folder;
  - `cluster` — for every cluster connected to ArgoCD (rolling monitoring out to all 20 clusters);
  - `matrix` — a combination of two generators (each application × each cluster).
```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: services-prod
  namespace: argocd
spec:
  generators:
    - git:
        repoURL: https://gitlab.com/mycompany/deploy.git
        revision: main
        directories:
          - path: apps/*/overlays/prod            # each service folder → a separate application
  template:
    metadata:
      name: '{{path[1]}}-prod'                    # path[1] — the service name from the path apps/<service>/...
    spec:
      project: prod
      source:
        repoURL: https://gitlab.com/mycompany/deploy.git
        targetRevision: main
        path: '{{path}}'
      destination:
        server: https://kubernetes.default.svc
        namespace: '{{path[1]}}'
      syncPolicy:
        automated: { prune: true, selfHeal: true }
```

### Permissions: AppProject
An **AppProject** restricts which repositories, which clusters and namespaces, and which resource types a group of applications may deploy. For example, the `payments` team's project can deploy only from its own repository and only into `payments-*` namespaces, without the right to create ClusterRoles. Plus ArgoCD RBAC for users (who may press Sync in prod).

### How ArgoCD works with Helm
ArgoCD **renders** the chart **itself** (`helm template`) and applies the resulting manifests. Consequences:
- `helm list` **won't show** such a release: there's no Helm release in the cluster, ArgoCD manages it;
- a rollback is done via ArgoCD or `git revert`, not `helm rollback`;
- ArgoCD converts Helm hooks into its own hooks.

### Updating images
Options for how a new image tag gets into the configuration repository:
1. **CI commits** the new tag (`yq` or `kustomize edit set image`) to the deploy repository — the most transparent option. For prod — via a merge request.
2. **ArgoCD Image Updater** — watches the registry and updates the tag itself (in git or in the application's parameters).

### Progressive delivery
**Argo Rollouts** and **Flagger** replace an ordinary Deployment with a resource with a canary or blue/green strategy: traffic is shifted to the new version gradually (5% → 25% → 100%), and between steps metrics from Prometheus (error rate, latency) are analysed automatically. On degradation — an automatic rollback.

### Flux — an alternative to ArgoCD
A set of separate controllers: source-controller (watches git, Helm repositories, OCI), kustomize-controller, helm-controller (creates real Helm releases), image-automation (tag updates). No web UI by default; everything is configured via CRDs. It natively supports SOPS for secrets. ArgoCD is more popular thanks to its UI and ApplicationSet.

### 🧪 Mini task

**Task.** Without access to an ArgoCD server, generate an `Application` manifest for `overlays/prod` from a git repository with automated sync, pruning of extra resources and self-heal. Look at the source, destination and sync policy.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
argocd admin app generate-spec shop-prod \
  --repo https://gitlab.com/mygroup/shop-deploy.git --path overlays/prod --revision main \
  --dest-server https://kubernetes.default.svc --dest-namespace shop \
  --sync-policy automated --auto-prune --self-heal --sync-option CreateNamespace=true |
yq '{"source": .spec.source, "destination": .spec.destination, "syncPolicy": .spec.syncPolicy}'
```

Expected output:

```text
source:
  path: overlays/prod
  repoURL: https://gitlab.com/mygroup/shop-deploy.git
  targetRevision: main
destination:
  namespace: shop
  server: https://kubernetes.default.svc
syncPolicy:
  automated:
    prune: true
    selfHeal: true
  syncOptions:
    - CreateNamespace=true
```

Breakdown: an `Application` links a source (repository, branch, path) with a destination (cluster and namespace). `automated` means sync on its own when git changes. `prune: true` removes from the cluster resources deleted from git (without it they remain as orphans). `selfHeal: true` reverts manual changes in the cluster to the git state. `kubernetes.default.svc` is the cluster ArgoCD itself runs in. This YAML goes into git and is applied (`kubectl apply -n argocd`); for many apps it's generated with an ApplicationSet.

</details>

## Secrets in GitOps
Secrets can't be stored in git in plain text, while GitOps requires everything to be in git. Solutions:
- **External Secrets Operator (ESO)** — git holds an `ExternalSecret` object with a **reference** to a secret in AWS Secrets Manager, SSM or Vault. The operator creates and updates the Kubernetes Secret itself. The most common option in AWS.
  ```yaml
  apiVersion: external-secrets.io/v1beta1
  kind: ExternalSecret
  metadata:
    name: api-db
  spec:
    refreshInterval: 1h
    secretStoreRef:
      name: aws-secrets-manager
      kind: ClusterSecretStore
    target:
      name: api-db                 # which Kubernetes Secret to create
    data:
      - secretKey: DB_PASSWORD
        remoteRef:
          key: prod/api/db         # the secret's name in Secrets Manager
          property: password
  ```
- **Sealed Secrets** — the secret is encrypted with the public key of a controller in the cluster (`kubeseal`), git holds the encrypted `SealedSecret`, and only the controller can decrypt it.
- **SOPS** — encrypting values in YAML files with a KMS key or age (a simple modern file encryption tool with short keys — a replacement for PGP); Flux decrypts natively, ArgoCD via a plugin.

### 🧪 Mini task

**Task.** Encrypt a Kubernetes Secret with SOPS and an age key so that only the data is encrypted and the metadata stays readable. Make sure the password isn't in the file in plain text, and decrypt it back. Requires: `brew install sops age`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
age-keygen -o key.txt 2> /dev/null
cat > secret.yaml <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: db-credentials
stringData:
  password: hunter2
EOF
sops --encrypt --age "$(grep -o 'age1.*' key.txt)" --encrypted-regex '^(data|stringData)$' secret.yaml > secret.enc.yaml
yq '.metadata.name' secret.enc.yaml
yq '.stringData.password' secret.enc.yaml | cut -c1-20
grep -c hunter2 secret.enc.yaml
SOPS_AGE_KEY_FILE=key.txt sops --decrypt secret.enc.yaml | yq '.stringData.password'
```

Expected output:

```text
db-credentials
ENC[AES256_GCM,data:
0
hunter2
```

Breakdown: `secret.enc.yaml` can be committed to git: the value is encrypted, while the name and kind stay readable, so the diff in review makes sense. Only the holder of the age private key can decrypt it — in the cluster that's an ArgoCD plugin or Flux with the key in a Secret, and in the cloud KMS often serves as the key. Alternatives: Sealed Secrets (encrypted with the in-cluster controller's key) and External Secrets Operator (git holds only a reference; the value lives in Secrets Manager or Vault). A plain base64 Secret in git is plain text.

</details>

## Interview questions

**How does a push deployment differ from GitOps (pull)?**
With push, CI applies the changes to the cluster itself and must have write access to it. With pull, an agent in the cluster (ArgoCD, Flux) pulls the desired state from git itself and constantly reconciles it with the cluster. The upsides of pull: CI doesn't need cluster credentials, manual drift is visible and fixed, audit and rollback via git. The downsides: imperative steps are harder, a mechanism for secrets is needed.

**How do you store secrets with GitOps?**
External Secrets Operator (git holds a reference to Secrets Manager or Vault), Sealed Secrets (git holds an encrypted object that the controller decrypts), SOPS with KMS.

**Someone changed the number of replicas by hand. What will ArgoCD do?**
With `selfHeal: true` — restore the value from git. Without it — show OutOfSync. If HPA manages the replicas, you need to remove `replicas` from the manifest or add `ignoreDifferences`, otherwise ArgoCD and HPA will overwrite the value endlessly.

**How do you roll an application out to 20 clusters?**
An ApplicationSet with the `cluster` generator: an Application is created for each registered cluster, and the differences are set via cluster labels and different values.

**How do you run a database migration before a deployment?**
A Job with the Helm `pre-upgrade` hook or the ArgoCD `PreSync` hook. Migrations must be backward compatible (first expand the schema, then roll out the code, then remove the old parts) so that the old version of the application keeps working during the rollout and on rollback.

**How does ArgoCD work with Helm charts?**
It renders them itself (`helm template`) and applies the manifests. There's no Helm release in the cluster, so `helm list` won't show it, and rollback is done via ArgoCD or git, not `helm rollback`. Helm hooks become ArgoCD hooks.

**A Helm release is stuck in the `pending-upgrade` state. What do you do?**
The previous operation was interrupted (CI crashed, a timeout). `helm history` shows the state of the revisions; `helm rollback` to the last successful revision removes the lock. Prevention — `--atomic` and `--timeout` in CI.

**How does Helm differ from Kustomize? When do you use each?**
Helm — templates with parameters, packages for distribution, releases with history and rollback, hooks. Kustomize — ordinary manifests and patches for environments, without a template language. Helm — for third-party components and the company-wide chart, Kustomize — for your own services with small differences. They're often combined.

**Pods weren't restarted after a ConfigMap change. Why, and how do you fix it?**
Environment variables are read only when the container starts, and a ConfigMap change doesn't change the pod template. Fixes: an annotation with the config hash in the pod template (`checksum/config` in Helm), `configMapGenerator` in Kustomize (a hash in the ConfigMap name), Reloader (a controller that restarts a Deployment itself when a linked ConfigMap or Secret changes), or `kubectl rollout restart`.

**How does Sync status differ from Health status in ArgoCD?**
Sync — whether the cluster matches git (Synced or OutOfSync). Health — whether the resources work (Healthy, Progressing, Degraded). You can be Synced and Degraded at the same time: everything is applied, but the new version crashes.

**What are App of Apps and ApplicationSet?**
App of Apps — a root Application that deploys other Applications from a folder in git. ApplicationSet — generates Applications from a template using a list, repository folders or a list of clusters. ApplicationSet is more flexible and is preferred today.

**How do you organise repositories with GitOps?**
The application code and the deployment configuration in different repositories. The application's CI builds the image and updates the tag in the configuration repository (for prod — via a merge request). ArgoCD watches only the configuration repository.

**What is a canary deployment in GitOps?**
Argo Rollouts or Flagger shift traffic to the new version gradually and analyse metrics (errors, latency) from Prometheus between steps. On degradation — an automatic rollback to the stable version.

**What are Helm hooks and what are their pitfalls?**
Chart resources (usually Jobs) that run in a specific phase of a release: `pre-install`, `pre-upgrade`, `post-upgrade`, `pre-delete`, `test` and others, ordered via `hook-weight` and cleaned up via `hook-delete-policy`. The typical use is DB migrations in `pre-upgrade`. Pitfalls: hooks aren't part of the release and aren't rolled back by `helm rollback` (so migrations must be backward compatible), at `pre-install` the Secrets and ConfigMaps from the same chart don't exist yet, without `before-hook-creation` a repeated upgrade fails on the already existing Job, long migrations run into `--timeout`, and parallel deploys start the migration twice.

**How does ArgoCD handle Helm hooks?**
ArgoCD renders the chart itself and translates Helm hooks into its own: pre-install and pre-upgrade into PreSync, post-install and post-upgrade into PostSync. There's no difference between install and upgrade, so an install hook runs on every sync and must be idempotent. Some hooks (rollback, test, pre-delete) aren't supported. For charts deployed by ArgoCD it's better to use the `argocd.argoproj.io/hook` and `sync-wave` annotations from the start.
