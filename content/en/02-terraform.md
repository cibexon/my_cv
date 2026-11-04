# Terraform

Terraform in depth: how it works, the HCL language, state and teamwork, modules and how to build them properly, refactoring without recreating resources, CI/CD for infrastructure. Each topic: what it is, why, how it works, an example with explanations and typical mistakes. At the end — interview questions and cases.

## How to learn without a course

The HashiCorp course and certification are useful, but you can quickly build the foundation on your own:
- **Official tutorials**: https://developer.hashicorp.com/terraform/tutorials — the "Get Started — AWS" and "Terraform Associate prep" tracks (Terraform Associate is HashiCorp's entry-level Terraform certification). Free, each lesson 10–20 minutes.
- **The Terraform Associate exam topics list**: https://developer.hashicorp.com/terraform/tutorials/certification-003/associate-review-003 — a ready-made checklist of "what an engineer should know", even if you don't take the exam.
- **The book** "Terraform: Up & Running" (Yevgeniy Brikman) — the best one on patterns, modules and code organisation.
- **Practice** on your own repository `~/work/GitHubRepos/tf-aws-eks-cluster`: bring up a VPC and EKS from modules, deliberately break the state, import an existing resource (bring a manually created resource under Terraform's management), rename a resource with `moved` (both operations are covered in the refactoring section).

## What Terraform is and how it works

**Infrastructure as Code (IaC)** — describing infrastructure (networks, servers, databases, DNS records, permissions) as code that lives in git, goes through review and is applied automatically. Instead of clicking around the AWS console:
- the infrastructure is **reproducible** (bringing up another environment means running the same code);
- changes are **visible and reviewed** (merge request, change plan);
- there is **history** (who changed what, when and why);
- there is no **drift** between environments.

**Terraform is declarative.** You describe *what* should exist ("a VPC with this CIDR, three subnets"), not *how* to create it step by step. Terraform works out itself what needs to be created, changed or deleted so that reality matches the description.

### What it consists of
- **Terraform CLI** — reads `.tf` files, builds a dependency graph, compares it with the current state and calls the providers.
- **Providers** — plugins that know the API of a specific system: `aws`, `kubernetes`, `helm`, `cloudflare`, `gitlab`, `datadog`. Terraform itself knows nothing about AWS — everything goes through the provider.
- **State** — a file where Terraform remembers which real objects correspond to the resources in the code.

### The workflow
```bash
terraform init        # download providers and modules, configure the state backend
terraform fmt         # format the code
terraform validate    # check syntax and types (without calling the cloud)
terraform plan        # show what will be changed
terraform apply       # apply the changes (shows the plan and asks for confirmation)
terraform destroy     # delete everything described in this configuration
```

### What plan does
1. Reads the code and builds the resource **dependency graph**.
2. **Refresh**: for every resource in the state it asks the API for its current real state.
3. Compares three pictures: the **code** (desired), the **state** (what Terraform created earlier) and **reality**.
4. Shows the action plan with symbols:
   - `+` create;
   - `~` change in place (update in-place);
   - `-` delete;
   - `-/+` **delete and create again** (replace) — because an attribute changed that cannot be modified on an existing object. **The most important thing to look for in a plan**: replacing a database means losing data.

```
  # aws_db_instance.main must be replaced
-/+ resource "aws_db_instance" "main" {
      ~ identifier = "app-db" -> "app-db-prod" # forces replacement
```
The phrase `forces replacement` tells you exactly which attribute causes the recreation.

Good practice is to save the plan and apply **exactly that plan**:
```bash
terraform plan -out=tfplan     # the plan is written to a file
terraform apply tfplan         # exactly what was in the plan is applied, without recalculating
```
That way nothing changes between reviewing the plan and applying it.

### 🧪 Mini task

**Task.** Go through the full workflow with the `local` provider, no cloud needed: `init` → `plan` → `apply` → `plan` again → `destroy`. The resource is a `motd.txt` file with some text.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
Terraform has been successfully initialized!
  # local_file.motd will be created
Plan: 1 to add, 0 to change, 0 to destroy.
Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
hello from terraform
No changes. Your infrastructure matches the configuration.
Destroy complete! Resources: 1 destroyed.
```

Breakdown: `init` downloaded the provider into `.terraform/` and created `.terraform.lock.hcl` with the exact version. `plan` compared the code with the state and reality and showed "will be created". `apply` created the file and recorded it in `terraform.tfstate`. The second `plan` showed "No changes" — that is idempotency: code, state and reality match. `grep` and `tail` only trim the output here — run the commands without them to see the full plan.

</details>

## The HCL language

### Provider and versions
```hcl
terraform {
  required_version = ">= 1.6"          # minimum Terraform CLI version
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"               # any 5.x but not 6.0 — protection from breaking changes
    }
  }
}

provider "aws" {
  region = var.region
  default_tags {                        # tags that ALL resources of this provider get
    tags = {
      ManagedBy   = "terraform"
      Environment = var.env
    }
  }
}

provider "aws" {                        # a second provider instance — for another region
  alias  = "us_east_1"
  region = "us-east-1"                  # e.g. ACM certificates for CloudFront must be in us-east-1
}
```
**Version operators:** `= 5.31.0` — exactly this one; `>= 5.0` — not lower; `~> 5.31` — 5.31 and higher, but below 6.0; `~> 5.31.0` — only 5.31.x patches.

**The `.terraform.lock.hcl` file** is created by `init` and pins the exact provider versions and their hashes. **It is committed to git**: that way everyone on the team and in CI uses the same provider version. To upgrade providers: `terraform init -upgrade`.

### Resources and data sources
```hcl
resource "aws_vpc" "main" {             # resource "<type>" "<local name>"
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  tags = { Name = "main" }
}

resource "aws_subnet" "private_a" {
  vpc_id            = aws_vpc.main.id   # a reference to another resource's attribute: <type>.<name>.<attribute>
  cidr_block        = "10.0.1.0/24"
  availability_zone = "us-east-2a"
}

data "aws_availability_zones" "available" {   # data — READS what exists, creates nothing
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
**The resource address** is `aws_vpc.main`. By it the resource is found in the state, and other resources and commands refer to it (`terraform state show aws_vpc.main`).

**Dependencies.** The reference `aws_vpc.main.id` creates an **implicit dependency**: Terraform knows the subnet has to be created after the VPC and deleted before it. Independent resources are created in parallel. An explicit `depends_on` is rarely needed — when a dependency exists but isn't visible through references (for example, an IAM policy must be attached before the application starts using the role).

### Variables, locals, outputs
**Input variables** — parameters of a configuration or module:
```hcl
variable "env" {
  type        = string
  description = "Environment: dev, stage or prod"
  validation {
    condition     = contains(["dev", "stage", "prod"], var.env)
    error_message = "env must be dev, stage or prod."
  }
}

variable "instance_count" {
  type    = number
  default = 2                            # has a default — the variable is optional
}

variable "db_password" {
  type      = string
  sensitive = true                       # don't show the value in plan and apply output
}

variable "subnets" {
  type = map(object({                    # a complex type: a map of objects
    cidr = string
    az   = string
  }))
}
```
Where values come from (from lowest to highest priority):
1. `default` in the declaration;
2. environment variables `TF_VAR_env=prod`;
3. the `terraform.tfvars` file;
4. `*.auto.tfvars` files;
5. the `-var-file=prod.tfvars` and `-var="env=prod"` flags.

**Locals** — computed values inside a configuration, to avoid repeating expressions:
```hcl
locals {
  name_prefix = "${var.project}-${var.env}"               # "shop-prod"
  azs         = slice(data.aws_availability_zones.available.names, 0, 3)   # the first three zones
  common_tags = { Project = var.project, Owner = "platform" }
}
```

**Outputs** — values a configuration exposes: shown after apply, passed from a module to the calling code, read from another configuration.
```hcl
output "vpc_id" {
  value       = aws_vpc.main.id
  description = "ID of the created VPC"
}
```

### count and for_each — several instances of a resource
**count** — create N identical instances addressed by **index**:
```hcl
resource "aws_instance" "web" {
  count         = 3
  ami           = data.aws_ami.al2023.id
  instance_type = "t3.micro"
  tags          = { Name = "web-${count.index}" }   # web-0, web-1, web-2
}
# addresses: aws_instance.web[0], aws_instance.web[1], aws_instance.web[2]
```
**The problem with count and lists.** If you create from a list and remove an element from the **middle**, the indexes shift:
```hcl
variable "users" { default = ["alice", "bob", "carol"] }
resource "aws_iam_user" "u" {
  count = length(var.users)
  name  = var.users[count.index]
}
# removed "bob" → the list is ["alice", "carol"]
# Terraform sees: [1] was bob, now carol → RENAME; [2] was carol → DELETE
# instead of simply "delete bob" — carol gets recreated
```

**for_each** — instances are addressed by **key** rather than by position:
```hcl
resource "aws_iam_user" "u" {
  for_each = toset(["alice", "bob", "carol"])
  name     = each.key
}
# addresses: aws_iam_user.u["alice"], aws_iam_user.u["bob"], aws_iam_user.u["carol"]
# removing "bob" affects only aws_iam_user.u["bob"]
```
With a map you can pass parameters for each instance:
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
  cidr_block        = each.value.cidr       # each.key — the key, each.value — the value
  availability_zone = each.value.az
  tags              = { Name = each.key }
}
```
**The rule:** `for_each` — for collections of objects; `count` — for "create or not" (`count = var.enable_bastion ? 1 : 0`) and truly identical instances.

A limitation: `for_each` keys must be known at plan time. You can't use the ID of a resource that hasn't been created yet in the keys.

### Expressions and functions
```hcl
# a conditional
instance_type = var.env == "prod" ? "m6i.large" : "t3.small"

# for expressions — like list/dict comprehensions in Python
private_subnet_ids = [for s in aws_subnet.this : s.id]
subnet_by_az       = { for k, s in aws_subnet.this : s.availability_zone => s.id }
prod_only          = [for u in var.users : u if u.env == "prod"]

# splat — shorthand for a list of attributes (for count)
instance_ids = aws_instance.web[*].id
```
Functions you need all the time:
- `cidrsubnet("10.0.0.0/16", 8, 1)` → `10.0.1.0/24` — carving subnets (add 8 bits to the prefix, take subnet number 1);
- `merge(local.common_tags, { Name = "x" })` — merging maps;
- `lookup(var.sizes, var.env, "t3.small")` — a value from a map with a default;
- `try(var.config.timeout, 30)` — the value, or a fallback if the expression fails;
- `coalesce(var.name, local.default_name)` — the first non-empty one;
- `flatten([[1, 2], [3]])` → `[1, 2, 3]`;
- `templatefile("user-data.sh.tpl", { cluster = var.name })` — a template file with substitution;
- `jsonencode({...})` — an HCL object to JSON (IAM policies);
- `yamldecode(file("config.yaml"))` — read a YAML file as a structure.

To experiment with expressions: `terraform console`.

### dynamic — repeated nested blocks
Some resources contain repeated nested blocks (Security Group rules). `dynamic` generates them from a collection:
```hcl
resource "aws_security_group" "web" {
  name   = "web"
  vpc_id = aws_vpc.main.id

  dynamic "ingress" {
    for_each = var.allowed_ports          # e.g. [80, 443]
    content {
      from_port   = ingress.value
      to_port     = ingress.value
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }
  }
}
```
Don't overuse it: code with nested `dynamic` blocks is hard to read.

### lifecycle — controlling a resource's behaviour
```hcl
resource "aws_launch_template" "web" {
  # ...
  lifecycle {
    create_before_destroy = true          # on replacement create the new one first, then delete the old (no downtime)
    prevent_destroy       = true          # a plan that deletes this resource fails with an error — protects databases, buckets
    ignore_changes        = [tags["LastScanned"], desired_capacity]   # don't touch attributes someone else changes
    replace_triggered_by  = [aws_ssm_parameter.ami_id]   # recreate if another resource changed
    precondition {
      condition     = var.instance_type != "t2.micro"
      error_message = "t2.micro is too small for this service."
    }
  }
}
```
A typical example of `ignore_changes`: an Auto Scaling Group's `desired_capacity` is changed by the autoscaler. Without `ignore_changes` every `apply` would reset it to the value from the code.

### Secrets in Terraform
- `sensitive = true` **only hides the value in plan and apply output**. **In the state the value is stored in plain text.** That is why state is encrypted and access to it is restricted.
- Don't keep secrets in `.tfvars` in git. Options: read them from AWS Secrets Manager or SSM Parameter Store via a data source; pass them via `TF_VAR_*` from protected CI variables.
- Better still, the secret never gets into state at all: let AWS generate and keep the password itself (`manage_master_user_password = true` on RDS — the password is created and rotated in Secrets Manager, Terraform never sees it), or use ephemeral resources (values that exist only during plan/apply and aren't saved) and write-only attributes (attributes passed to the API but not written to state) — they appeared in Terraform 1.10–1.11.

### 🧪 Mini task

**Task.** Without a single resource, experiment with HCL expressions in `terraform console`: build a name prefix from a variable, filter services that have a port with `for … if`, sum the replicas, generate a list of names, and fetch an optional attribute with `try`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

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

Breakdown: `terraform console` evaluates expressions in the context of the configuration — the best way to debug a tricky `for` or function without applying anything. `for … if` returned a map with only the services whose port is above 0. `values()` + `for` + `sum()` added up the replicas. `keys()` on a map returns its keys, and `sort()` makes the order predictable. `try()` returned the default because there is no `cpu` attribute — without `try` it would be an error.

</details>

## State

### Why state is needed
State is the `terraform.tfstate` file (JSON) where, for every resource in the code, Terraform records which real object it corresponds to (the ID in AWS) and its last known attributes.

Why you can't do without it:
- **Mapping.** In the code it's `aws_instance.web`, in AWS it's `i-0abc123`. Only the state links them. Without state Terraform doesn't know it already created this instance and will try to create another one.
- **Deletion.** If a resource is removed from the code, Terraform learns that it needs to delete it only from the state.
- **Dependencies on deletion** and **performance** (an attribute cache).

### Remote backend
By default the state is stored locally. That doesn't work for a team: everyone has their own copy, there is no protection from concurrent runs, and a laptop can get lost. So the state is stored remotely:
```hcl
terraform {
  backend "s3" {
    bucket       = "mycompany-terraform-state"
    key          = "prod/network/terraform.tfstate"   # the path — separate for each stack
    region       = "us-east-2"
    encrypt      = true                                # server-side encryption in S3
    use_lockfile = true                                # locking via a file in S3 (Terraform 1.10+)
    # in older versions locking used DynamoDB: dynamodb_table = "terraform-locks"
  }
}
```
Settings for the state bucket: **versioning** (you can roll a corrupted state back to the previous version), encryption, Block Public Access, access only for CI and administrator roles.

### Locking
If two people run `apply` at the same time, both read the same state, make different changes and overwrite each other's state. Resources get "lost". A **lock** guarantees that only one process changes the state at any moment: the second one gets a "state locked" error.

If a process crashed and didn't release the lock (the CI job was killed):
```bash
terraform force-unlock <LOCK_ID>   # only after making sure nobody is actually running apply
```

### Commands for working with state
```bash
terraform state list                         # all resources in the state
terraform state show aws_vpc.main            # a resource's attributes
terraform state mv aws_instance.a aws_instance.b   # rename an address (the moved block is better, see below)
terraform state rm aws_s3_bucket.logs        # "forget" a resource: remove it from the state WITHOUT deleting it in the cloud
terraform state pull > backup.tfstate        # download the state (before risky operations)
terraform plan -refresh-only                 # show what changed in the cloud outside Terraform (drift)
terraform apply -replace=aws_instance.web    # force-recreate a resource (replaces the deprecated taint)
AWS_PROFILE=test terraform state rm 'module.opensearch.opensearch_roles_mapping.all_access'
# addresses with indexes and modules go in single quotes, otherwise the shell eats [ ] and "
```

### Dangerous flags: -lock=false and -target
- **`-lock=false`** disables state locking. It's acceptable for reading (`plan` while debugging, when you're sure nobody is applying), but **`apply -lock=false` in production** is a risk: two parallel applies overwrite each other's state and resources get "lost". If the state is locked — find out who holds it instead of disabling the lock.
- **`-target=address`** applies changes only to the given resource (for example, `-target='helm_release.nginx[0]'`). It's needed in emergencies: urgently fix one resource without touching the rest. But Terraform then **ignores everything that depends on that resource**, and the state may diverge from the code. After `-target`, a full `plan` and `apply` are mandatory. Using `-target` regularly is a sign that the state is too big and it's time to split it.

### Drift — changes that bypass Terraform
Someone changed a Security Group by hand in the console. The next `plan` shows the difference and proposes to put everything back as in the code. Options:
- the change was a mistake — apply, restoring the state from the code;
- the change is needed — **move it into the code** so the plan becomes empty.

Detect drift in advance: a regular scheduled `plan` in CI with a notification when the result is not empty.

### 🧪 Mini task

**Task.** Create two files with `local_file` and look at what is in the state. Then make Terraform "forget" one resource with `terraform state rm`, and delete the other file by hand, bypassing Terraform (drift). What will `plan` show?

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

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

Breakdown: `state rm` removed only the state record — `b.txt` is still on disk, but Terraform no longer knows about it and wants to create it again (in a cloud that would be a duplicate or a name conflict; the right way is `import`). `a.txt` was deleted behind Terraform's back: during `plan` the provider refreshed the state from reality, saw the file was gone and planned to create it. That is drift: reality diverged from the state, and `plan` shows it.

</details>

## Project structure and environments

### A typical structure
```
modules/                  ← reusable modules
  vpc/
    main.tf
    variables.tf
    outputs.tf
    versions.tf
    README.md
  eks/
envs/                     ← root configurations, one per environment and stack
  dev/
    network/
      main.tf             ← calls modules/vpc with dev parameters
      backend.tf          ← key = "dev/network/terraform.tfstate"
      terraform.tfvars
    eks/
  prod/
    network/
    eks/
```
The **root module** is the folder where you run `terraform`. **Child modules** are called from it through a `module` block.

### How to split state
One huge state for everything is bad:
- `plan` takes long (refreshing thousands of resources);
- **a large blast radius**: a mistake in a DNS record change can hit the database;
- locking gets in the way of teams working in parallel.

Split by **environment** (dev, stage, prod — always separate) and by **layers** that change at different rates and have different criticality: network → data (RDS, S3) → compute (EKS) → applications. The less often a layer changes and the more critical it is, the more separate it should be.

### How to link stacks together
The EKS stack needs the subnet IDs from the network stack. Options:
- `data "terraform_remote_state"` — read the outputs of another state. Simple, but couples the stacks tightly and requires read access to the whole foreign state (including secrets).
- **Data sources by tags**: `data "aws_subnets"` with a tag filter. The link goes through the real infrastructure, without access to the state.
- **SSM Parameter Store**: the network stack writes the IDs into parameters, the EKS stack reads them. An explicit contract between teams.

### Environments: directories, workspaces or Terragrunt
- **Separate directories** per environment (as above) — the most explicit and widespread way. Environments can differ, and each has its own backend and permissions.
- **Workspaces** (`terraform workspace new prod`) — one codebase, several states. Suitable only for **identical** environments. It's easy to make a mistake and apply in the wrong workspace, and differences between environments turn into `var.env == "prod" ? ... : ...` all over the code.
- **Terragrunt** — a wrapper around Terraform: removes duplication of backend and provider settings, defines dependencies between stacks (`dependency`) and runs them in the right order.
- **One codebase and parameter files per environment** (partial backend configuration) — see below.

### One codebase, many environments: -backend-config and -var-file
A common option in practice when environments are built the same way but must have **separate states** and values. There is one codebase, and for each environment and region a folder with two files:
```
main.tf  variables.tf  versions.tf  backend.tf
env/
  test/eu-central-1/
    backend.tfvars      ← where this environment's state lives
    terraform.tfvars    ← this environment's variable values
  prod/eu-central-1/
    backend.tfvars
    terraform.tfvars
```
In the code the backend is described **partially** — without concrete values:
```hcl
# backend.tf
terraform {
  backend "s3" {}       # the parameters come from -backend-config at init
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
**`-reconfigure`** is mandatory when switching between environments in the same folder: without it Terraform remembers the previous backend (in `.terraform/`) and offers to **migrate the state** from test to prod — that's an easy way to overwrite the production state with the test one. Even more reliable is a separate working copy or a clean `.terraform` folder for each environment (in CI this happens naturally).

The upside of this approach is no code duplication and guaranteed identical environments. The downside is that all environments are always on the same code version: to roll out a change to test first and then to prod you need branches, tags or flags in variables.

### File layout in a configuration
Terraform reads all `.tf` files in a folder as one; file names don't matter to it. But a conventional layout helps when reading other people's code:
- `versions.tf` — `required_version` and `required_providers`;
- `providers.tf` — provider settings (region, profile, `default_tags`, alias);
- `backend.tf` — where the state is stored;
- `variables.tf` — input variables with types, descriptions and `validation`;
- `locals.tf` — computed values (common tags, name prefixes);
- `data.tf` — data sources (AMI, an existing VPC, zones);
- `main.tf` or files by purpose (`network.tf`, `eks.tf`, `iam.tf`) — resources and modules;
- `outputs.tf` — output values.

### Environment secrets: not in notes and not in tfvars
Secret variables (the client secret for SSO in ArgoCD and Grafana, tokens, API keys) are often passed via `export TF_VAR_name=...` before running. It's a working mechanism, but **the values should come from a secrets manager**, not be kept in notes, tfvars or shell history:
```bash
export TF_VAR_grafana_client_secret="$(aws secretsmanager get-secret-value \
  --secret-id prod/grafana/sso --query SecretString --output text)"
```
Even better — read the secret right in the code via `data "aws_secretsmanager_secret_version"` or an `ephemeral` resource, and in CI pass it through protected variables. Remember: the value still ends up in the state if it's written into resource attributes.

### 🧪 Mini task

**Task.** One configuration — two environments. Create `dev.tfvars` (1 replica) and `prod.tfvars` (3 replicas), apply dev in the `default` workspace, then create a `prod` workspace and apply prod. See which files you get and where each environment's state lives.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

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

Breakdown: one codebase, but each workspace has its own state: `default` in `terraform.tfstate`, the others in `terraform.tfstate.d/<name>/` (with an S3 backend — under the `env:/` prefix). That's why the second `apply` didn't touch dev but created a new resource. The asterisk marks the current workspace — it's easy to mix up, and `apply` goes to the wrong place. That's why real dev/prod usually use separate directories or `-backend-config` with different state keys, and workspaces are used for temporary copies.

</details>

## Modules

### What a module is and why you need it
A module is any folder with `.tf` files. Calling a module is a way to reuse code: "create a VPC to our standard" instead of copying 200 lines into every environment. Modules give you:
- **reuse** — the same VPC in dev, stage and prod;
- **standards** — tags, encryption and logging are built into the module and won't be forgotten;
- **abstraction** — the application team sets 5 parameters instead of figuring out 30 resources.

### Calling a module
```hcl
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"    # a module from the public Terraform Registry
  version = "~> 5.13"                           # ALWAYS pin the version

  name = local.name_prefix
  cidr = "10.0.0.0/16"
  azs  = local.azs
  private_subnets = [for i, az in local.azs : cidrsubnet("10.0.0.0/16", 4, i)]
  public_subnets  = [for i, az in local.azs : cidrsubnet("10.0.0.0/16", 8, i + 48)]

  enable_nat_gateway = true
  single_nat_gateway = var.env != "prod"       # one NAT in dev (cheaper), one per zone in prod
}

module "eks" {
  source = "git::https://gitlab.com/mycompany/tf-modules.git//eks?ref=v2.3.0"
  # git source: // separates the path inside the repository, ?ref= — a tag or commit

  cluster_name = local.name_prefix
  vpc_id       = module.vpc.vpc_id              # module outputs: module.<name>.<output>
  subnet_ids   = module.vpc.private_subnets

  providers = {
    aws = aws.us_east_1                         # pass a specific provider instance to the module
  }
}
```
Module sources: a local folder (`./modules/vpc`), Git (with `?ref=`), the Terraform Registry (public or private — GitLab has a built-in module registry), S3.

**Why the version must always be pinned.** Without `version` or `?ref=`, the next `init` pulls a new version of the module, and the plan suddenly wants to recreate half of the infrastructure.

### How to write your own modules properly
**1. A module is a logical unit, not a wrapper around one resource.** An "s3-bucket" module that simply passes through all the arguments of `aws_s3_bucket` gives you nothing but an extra layer. A good module: "a log bucket to the company standard" (encryption, versioning, lifecycle rules, public access block, policy) or "an environment's network".

**2. A well-thought-out interface.**
```hcl
# modules/app-bucket/variables.tf
variable "name" {
  type        = string
  description = "Bucket name (the environment prefix will be added)"
}
variable "retention_days" {
  type        = number
  default     = 90
  description = "After how many days to delete objects"
  validation {
    condition     = var.retention_days >= 1
    error_message = "retention_days must be positive."
  }
}
```
Every variable has a type, a description, validation where needed and a sensible default. Outputs — everything that may be needed outside (IDs, ARNs, names).

**3. Don't declare a `provider` inside a module.** Only `required_providers` with versions in `versions.tf`. Providers are passed from the root module. Otherwise the module can't be used in another region or account, and removing the module breaks (the provider is removed together with the resources, and Terraform can't delete them).

**4. Don't make a module overly "smart".** Dozens of `enable_this`, `create_that` flags and conditions turn a module into an unreadable monster. Several small modules that are put together (composition) are better.

**5. Versioning.** Semantic versions in git tags: patch — fixes, minor — new features without breaking changes, major — breaking ones (renamed variables, changes that lead to recreation). A CHANGELOG. The README is generated by `terraform-docs`.

**6. Examples and tests.** An `examples/` folder with working calls of the module. Tests: the built-in `terraform test` (`*.tftest.hcl` files, since 1.6: you describe inputs and assertions, Terraform runs plan or apply and checks the conditions) or Terratest (a Go library that actually creates resources in the cloud, checks them and deletes them).
```hcl
# tests/bucket.tftest.hcl
run "creates_bucket_with_versioning" {
  command = plan
  variables {
    name = "test"
  }
  assert {
    condition     = aws_s3_bucket_versioning.this.versioning_configuration[0].status == "Enabled"
    error_message = "versioning must be enabled"
  }
}
```

### 🧪 Mini task

**Task.** Write a local module `modules/svc` that creates a service config from `name` and `port` inputs and returns the file path as an output. Call it for two services with `for_each` and look at the resource addresses in the state and the final output.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
module.svc["api"].local_file.this
module.svc["web"].local_file.this
{
  "api" = "./api.conf"
  "web" = "./web.conf"
}
```

Breakdown: resources inside a module have the address `module.<name>[key].<resource>` — that's what you pass to `state mv`, `import` and `-target`. `for_each` over a map gave stable `api` and `web` keys: if you remove `api`, `web` isn't recreated (with `count` the indexes would shift). A module exposes only what is declared in `output`, and the caller collects the outputs of all instances into one map. `path.root` points to the configuration root, not the module folder.

</details>

## Refactoring and importing without recreation

### moved — rename or move a resource
You renamed a resource in the code or moved it into a module — Terraform sees "the old address disappeared, a new one appeared" and plans to **delete it and create it again**. For a database this is a disaster. The `moved` block says: "it's the same object, just a different address":
```hcl
moved {
  from = aws_instance.web
  to   = aws_instance.frontend
}

moved {
  from = aws_db_instance.main
  to   = module.database.aws_db_instance.this    # moving into a module
}

moved {
  from = aws_iam_user.u[1]                        # switching from count to for_each
  to   = aws_iam_user.u["bob"]
}
```
After `apply` the plan shows a move without changes. The block is kept in the code for a while (so that all environments and state copies apply it), then removed. Previously the same was done with the `terraform state mv` command, but the `moved` block is better: the change goes through review and is applied the same way in all environments.

### import — bring an existing resource under management
A resource was created by hand or by another tool, and it needs to be moved into Terraform:
```hcl
import {
  to = aws_s3_bucket.legacy
  id = "my-old-bucket"                 # the identifier in the cloud (each resource type has its own format)
}
```
```bash
terraform plan -generate-config-out=generated.tf   # Terraform generates the resource code from the real object itself
```
Then clean up the generated code, reach an **empty plan** (the code fully matches reality) and apply. The old `terraform import <address> <id>` command does the same without review.

### removed — stop managing a resource without deleting it
```hcl
removed {
  from = aws_s3_bucket.archive
  lifecycle {
    destroy = false                    # remove from the state but keep the object in AWS
  }
}
```

### 🧪 Mini task

**Task.** Rename the `local_file.old` resource to `local_file.app`. First just change the name and look at the plan — Terraform wants to destroy and create. Then add a `moved` block and make sure the plan is empty and the address in the state has changed.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
Plan: 1 to add, 0 to change, 1 to destroy.
  # local_file.old has moved to local_file.app
Plan: 0 to add, 0 to change, 0 to destroy.
Apply complete! Resources: 0 added, 0 changed, 0 destroyed.
local_file.app
```

Breakdown: to Terraform a resource is its address. Without `moved` the new name is a new resource and the old one must go: in a cloud that means recreating a database or a cluster. The `moved` block says "this is the same object under a new address", and the plan is empty — only the state record changes. Unlike `terraform state mv`, `moved` lives in code, goes through review and is applied by CI to every environment. Once applied everywhere, the block can be removed.

</details>

## CI/CD for Terraform

### A typical pipeline
1. **Merge request:**
   - `terraform fmt -check` — the code is formatted;
   - `terraform validate`;
   - **tflint** — a linter: wrong instance types, deprecated syntax, naming conventions;
   - **trivy config** / **checkov** — security checks: open Security Groups, unencrypted disks, public buckets;
   - `terraform plan -out=tfplan` — **the plan is posted as a comment in the MR**, so the reviewer sees what will change in the infrastructure, not just the code.
2. **After merge to main:** `terraform apply tfplan`. For prod — with manual approval (a protected environment — an environment in GitLab or GitHub that only certain people may deploy to and that requires pressing a button).

Ready-made solutions: Atlantis (a bot that runs plan and apply on commands in MR comments), Spacelift, env0, HCP Terraform (cloud platforms for running Terraform with policies, history and state storage).

### pre-commit
Checks before the commit on the developer's machine (pre-commit is a git hook framework: before every `git commit` it automatically runs the configured checks and won't let you commit if they fail): `terraform_fmt`, `terraform_validate`, `terraform_tflint`, `terraform_docs` (update the module README), `terraform_trivy`. They catch errors before they reach an MR.

### CI access to AWS without keys
Don't store `AWS_ACCESS_KEY_ID` in CI variables. Use **OIDC**: GitLab or GitHub gives the job a signed token, AWS verifies it and issues temporary credentials for a role. The role's trust policy restricts which project, branch or environment can assume it. Separation of permissions: the role for `plan` (read-only) is available in all MRs, the role for `apply` (write) — only for main and the protected environment.

### 🧪 Mini task

**Task.** Repeat the steps of a typical pipeline and look at the exit codes CI makes decisions on: `fmt -check` on a badly formatted file and after `fmt`, `validate`, then `plan -detailed-exitcode` before and after applying a saved plan.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

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

Expected output:

```text
main.tf
fmt -check exit=3
main.tf
after fmt exit=0
Success! The configuration is valid.
plan exit=2
plan exit=0
```

Breakdown: `fmt -check` changes nothing; it prints the badly formatted files and returns a non-zero code — the lint job fails. `fmt` fixed the file (and printed its name too). `plan -detailed-exitcode` has three codes: 0 — no changes, 1 — error, 2 — there are changes. Code 2 lets the pipeline decide whether the apply step is needed, and a nightly job uses it to catch drift. `-out=tfplan` saves the plan, and `apply tfplan` applies exactly what was reviewed rather than a new plan.

</details>

## Terraform and OpenTofu
In 2023 HashiCorp changed Terraform's licence from the open MPL (Mozilla Public License — a free licence) to BSL (a restriction on competing commercial products). The community made a fork, **OpenTofu**, under the Linux Foundation with an open licence. The syntax and providers are compatible, and OpenTofu has its own features (for example, built-in state encryption). For most companies the difference is small so far, but the question is asked to check that you follow the ecosystem.

## Interview questions

**What is state, and why must it not be lost or kept in git?**
State links the resources in the code with real objects in the cloud. Without it Terraform doesn't know what has already been created and will try to create everything again, and won't delete resources removed from the code. It isn't kept in git: it contains secrets in plain text, there's no locking against concurrent changes, and merge conflicts are constant. The right way is a remote backend with encryption, versioning and locking.

**Someone deleted a resource by hand in the console. What will plan show?**
During refresh Terraform discovers the object is gone, removes it from the state and proposes to create it again.

**Someone changed a resource by hand. What do you do?**
`plan` (or `plan -refresh-only`) shows the drift. If the change was a mistake — `apply` restores the state from the code. If it's needed — move the change into the code so the plan becomes empty. Prevention — restrict permissions for manual changes and run plan on a schedule.

**How do you bring an existing resource under Terraform management?**
An `import` block with the resource address and ID, `terraform plan -generate-config-out` to generate the code, refine the code until the plan is empty, `apply`.

**How do you rename a resource or move it into a module without recreating it?**
A `moved { from = ... to = ... }` block (or `terraform state mv`). Without it Terraform deletes the old one and creates a new one.

**The state is locked, and the pipeline crashed. What do you do?**
Make sure nobody is actually running apply (check the running CI jobs), and release the lock: `terraform force-unlock <LOCK_ID>`.

**How do you work with secrets in Terraform?**
Don't keep them in tfvars in git. Read them from Secrets Manager or SSM via a data source, pass them through protected CI variables (`TF_VAR_*`). Remember that `sensitive` only hides the output, while the value is in plain text in the state: encrypt the state and restrict access to it. Better still, the secret shouldn't get into the state at all (AWS generates and stores the password, ephemeral resources).

**How does count differ from for_each?**
count addresses instances by index: removing an element from the middle of a list shifts the indexes and causes the following resources to be recreated. for_each addresses by key: changes affect only the relevant instance. count is for "create or not", for_each is for collections.

**How do you organise several environments?**
Separate directories with separate states and backends for each environment, with shared code in versioned modules. Workspaces — only for truly identical environments. Terragrunt — to remove duplication and link stacks.

**The plan wants to recreate a resource. Why, and what do you do?**
An attribute changed that cannot be modified in place (the plan marks it `forces replacement`: name, zone, database engine); a for_each key or count index changed; behaviour changed after a provider upgrade. Solutions: revert the value, `moved` when renaming, `ignore_changes` if the attribute is changed externally, `create_before_destroy` so the replacement happens without downtime, `prevent_destroy` as a safeguard.

**What are provisioners, and why do people try to avoid them?**
`local-exec` and `remote-exec` run commands when a resource is created. Their result isn't tracked in the state, they aren't idempotent, on failure the resource is marked tainted, and Terraform doesn't know about re-running them. Better: `user_data` or cloud-init, pre-built images (Packer), Ansible after creation.

**How does Terraform differ from CloudFormation, CDK, Pulumi and Ansible?**
Terraform — many providers for any system, an explicit state, the HCL language. CloudFormation — AWS only, AWS keeps the state, automatic rollback on failure. CDK and Pulumi — infrastructure in general-purpose programming languages (CDK generates CloudFormation). Ansible — primarily configuring operating systems and applications (sequential tasks), not managing the lifecycle of cloud resources.

**What is OpenTofu?**
A fork of Terraform under an open license after Terraform moved to the BSL in 2023. Compatible in syntax and providers.

**How do you speed up plan on a large project?**
Split the state into smaller stacks, reduce the number of data sources, use the provider cache (`TF_PLUGIN_CACHE_DIR`), increase `-parallelism`. In exceptional cases — `-refresh=false` or `-target` (only for emergencies: they break the integrity of the plan).

**What is `.terraform.lock.hcl` for, and should it be committed?**
It pins the exact versions and hashes of providers so that everyone and CI use the same version. Yes, commit it. Upgrade deliberately via `terraform init -upgrade`.

**How do you set up CI/CD for Terraform?**
In the MR: fmt, validate, tflint, a security scanner, plan with the result posted to the MR. After merge — apply the saved plan, prod with manual approval. Access to AWS via OIDC with different roles for plan and apply. State locking, a regular plan to detect drift.

**What do you do if apply failed halfway?**
The resources that managed to be created are already recorded in the state. Fix the cause of the error (limits, permissions, a wrong parameter) and run plan and apply again: Terraform continues from where it stopped. If a resource was created in the cloud but didn't make it into the state (a rare case on a connection drop) — import it.

**Why `-reconfigure` with `terraform init`, and what happens without it?**
With a partial backend configuration (`backend "s3" {}` and a different `-backend-config` per environment) Terraform remembers the last backend in the `.terraform` folder. If you switch to another environment without `-reconfigure`, it decides the backend has "moved" and offers to copy the state — that's how you can overwrite the prod state with the test one. `-reconfigure` says: forget the previous backend and connect to the new one without migrating anything.

**When are `-target` and `-lock=false` acceptable?**
Only in exceptional situations. `-target` — to fix one resource in an emergency, followed by a mandatory full plan and apply. `-lock=false` — only for reading (plan) when you're sure nobody is applying changes; an apply without locking risks a corrupted state.
