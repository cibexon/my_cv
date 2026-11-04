# Puppet

A thorough Puppet primer: how it works (agent, server, catalog), how it differs from Ansible, Chef, Salt and Terraform, the manifest language, resources and the dependencies between them, modules, Hiera, the roles and profiles pattern, secrets, exported resources, testing, CI and operating it on hundreds of servers. Each topic: what it is, why, how it works, an example with explanations and typical mistakes. At the end — interview questions with answers.

How to read: start with the lab from the first section and run the examples via `puppet apply`. Puppet's output is shown in comments after `# →`. If you already know Ansible, keep the comparison from the "Puppet among other tools" section in mind — interviews almost always ask about exactly that difference.

## How to learn without a course

Puppet can be learned without a server: the `puppet apply` command takes a file with a description and applies it to the machine it's run on. You'll need a server later, when you get to catalogs, certificates and PuppetDB.

**Lab option 1 — a Docker container (fast, but without systemd):**
```bash
docker run -it --name puppet-lab ubuntu:24.04 bash
apt update && apt install -y puppet-agent      # a package from the Ubuntu repository: the Puppet 8 agent and Facter
puppet --version
# → 8.4.0
```
The limitation: there's no systemd in a container, so the `service` resource won't work — for packages, files and users it's enough.

**Lab option 2 — virtual machines (like real life):** `brew install multipass`, then `multipass launch -n puppet-server -m 4G` and `multipass launch -n node1`. The server gets **puppetserver** (a server on the JVM — the Java virtual machine, so it needs at least 2–4 GB of memory), the nodes get the agent. Packages come from the official Puppet repository or from **OpenVox** (a community fork of Puppet, see the "Versions and OpenVox" section). The installation commands change between versions — check the instructions on the chosen project's site.

The difference in paths: in packages from the Ubuntu/Debian repository the configuration lives in `/etc/puppet`, in the official Puppet and OpenVox packages — in `/etc/puppetlabs/puppet`, and the code — in `/etc/puppetlabs/code/environments/<environment>`. The paths of the official packages are used below.

The plan: `puppet resource` → first manifest → a class → dependencies → a template → a module → Hiera → roles and profiles → server and agent → PuppetDB and exported resources. After each step — a repeat run and a check that there are no changes.

## What Puppet is and how it works

**Puppet** is a configuration management tool: you describe what state the servers should be in (which packages are installed, which files with what content are where, which services are running), and Puppet brings them to that state and **keeps them** in it.

The main properties:
- **A declarative language** — you write *what* should be, not *how* to do it: "the nginx package is installed", not "run apt-get install".
- **A pull model with an agent** — an agent runs on every server and goes to the server for configuration by itself every 30 minutes (the `runinterval` parameter). Ansible is the opposite: you launch the changes yourself from a control machine (push).
- **Continuous drift correction** (drift — divergence of the real state from the described one, for example someone edited a config by hand). The agent returns the file to the described form every 30 minutes. This is the main difference from Ansible, which changes something only when it's run.
- **The resource model** — everything is described by resources (`package`, `file`, `service`, `user`), and Puppet builds the graph of their dependencies itself.

**Components:**
- **Puppet Server** (previously called puppet master) — compiles configurations for nodes, serves files, acts as the certificate authority. It listens on port **8140** (HTTPS).
- **Puppet Agent** — a service on every managed machine (node).
- **Facter** — a utility that collects **facts** about the machine: OS, IP, memory, CPU count. The command `facter os.family` → `Debian`.
- **PuppetDB** — a database (on PostgreSQL) where the server stores the facts, latest catalogs and reports of all nodes. Needed for exported resources and queries like "on which servers is openssl 3.0.2 installed".
- **CA (Certificate Authority)** — built into Puppet Server. The agent and the server communicate over **mTLS** (mutual TLS: not only does the client verify the server, the server also verifies the client's certificate), so every node has to get a signed certificate.

**What happens during one agent run** (a Puppet run):
1. The agent collects facts via Facter.
2. It sends the facts to the server and asks for a **catalog** for its name (certname — the node's name from its certificate).
3. The server determines which classes this node needs (classification), takes data from Hiera and **compiles the catalog** — the final list of resources with concrete values and a dependency graph. Puppet code runs on the server, not on the node.
4. The agent receives the catalog and **applies** it: for each resource it compares the current state with the desired one and changes only the difference.
5. The agent sends a **report** (what changed, what failed, how long it took) — to the server and to PuppetDB.

A real-life situation: a colleague changed `worker_connections` in `/etc/nginx/nginx.conf` by hand in prod. Within 30 minutes the agent will notice the difference from the catalog, restore the file, restart nginx and record `changed` in the report. If the edit was needed, it has to be made in the Puppet code, not on the server.

## Puppet among other tools

"IaC" (Infrastructure as Code — infrastructure is described by code in git rather than configured by hand) is the umbrella term. Inside it there are two different tasks: **creating infrastructure** (networks, VMs, load balancers) and **configuring what's inside the OS**. Puppet, Ansible, Chef and Salt solve the second, Terraform the first.

| | Puppet | Ansible | Chef | Salt | Terraform |
|---|---|---|---|---|---|
| Task | OS configuration | OS configuration, deployment, orchestration | OS configuration | OS configuration, remote execution | creating cloud infrastructure |
| Model | pull: the agent goes to the server itself | push: you run it from a control node | pull: the chef-client agent | push and pull: master + minion | push: you run plan/apply |
| Agent on the server | yes | no (SSH + Python) | yes | yes (minion), there's an agentless mode | no |
| Language | its own declarative DSL | YAML, tasks in order | Ruby DSL | YAML + Jinja | HCL |
| Execution order | a dependency graph | top to bottom | top to bottom in a recipe | a graph with `require` | a dependency graph |
| State | doesn't keep it, compares with reality on every run | doesn't keep it | doesn't keep it | doesn't keep it | a state file |
| Drift correction | automatic, every 30 min | only when run | automatic, on a schedule | on a schedule or on events | only on `apply` |

When each is chosen:
- **Puppet** — thousands of long-lived servers (on-prem, banks, telecom, the public sector), where constant compliance with a standard, auditing and reports on every change matter.
- **Ansible** — a quick start, one-off operations, application deployment, orchestrating sequential steps (take out of load balancing → update → return), ephemeral cloud machines.
- **Terraform** — creating cloud resources. A frequent combination: Terraform creates a VM → the Puppet agent from user data registers with the server and configures it.
- In the Kubernetes world Puppet is hardly needed inside the cluster: application configuration is managed via GitOps (ArgoCD, Flux). Puppet remains for nodes, databases, on-prem hardware.

An interview answer to "why is Puppet better than Ansible" is best built as a trade-off: Puppet is stronger at constant state control and scale (agents come for the catalog themselves, the server doesn't hold thousands of SSH sessions), Ansible — at simplicity, having no agents and orchestrating steps in order.

## Resources: what any configuration consists of

A **resource** is one unit of system state: a package, a file, a service, a user, a cron job. The syntax:
```puppet
package { 'nginx':          # the resource type and its title (a unique name within the catalog)
  ensure => installed,      # attributes: the desired state
}
```

Puppet can **show the current state** of any resource as code — the best way to understand the syntax:
```bash
puppet resource user root
# → user { 'root':
# →   ensure => 'present',
# →   home   => '/root',
# →   shell  => '/bin/bash',
# →   uid    => 0,
# →   ...
# → }
puppet resource package openssl
puppet resource service ssh
puppet describe file            # documentation for the resource type and all its attributes
```

**RAL (Resource Abstraction Layer)** — why the same `package { 'nginx': }` works on Ubuntu and RHEL. Each type has **providers** — implementations for a specific system: for `package` these are `apt`, `dnf`, `yum`, `pip`, `gem`; for `service` — `systemd`, `init`. Puppet picks the provider itself based on facts; if needed it's set explicitly: `provider => pip`.

The main types:

| Type | Example | What matters |
|---|---|---|
| `package` | `ensure => installed` / `'1.24.0-1'` / `latest` / `absent` | `latest` updates the package on every run once a new version comes out — a pinned version is better in prod |
| `file` | `ensure => file`, `content`, `source`, `mode => '0644'` | also folders (`ensure => directory`) and links (`ensure => link`) |
| `service` | `ensure => running`, `enable => true` | `enable` — start at boot |
| `user`, `group` | `ensure => present`, `uid`, `groups`, `managehome => true` | |
| `exec` | running a command | not idempotent by itself, see below |
| `cron` | `command`, `hour`, `minute` | |
| `host` | an entry in `/etc/hosts` | |
| `mount` | a mount point and an fstab line | |

**The first manifest.** A **manifest** is a file with Puppet code, with the `.pp` extension:
```puppet
# hello.pp
file { '/etc/motd':
  ensure  => file,
  content => "This server is managed by Puppet. Manual edits will be overwritten.\n",
  owner   => 'root',
  mode    => '0644',
}
```
```bash
puppet apply hello.pp
# → Notice: Compiled catalog for node1 in environment production in 0.01 seconds
# → Notice: /Stage[main]/Main/File[/etc/motd]/ensure: defined content as '{sha256}f38fc8d5...'
# → Notice: Applied catalog in 0.03 seconds
puppet apply hello.pp       # the second run
# → Notice: Applied catalog in 0.01 seconds    — no lines about resources: no changes, that's how it should be
```

## Idempotency and exec

**Idempotency** — a repeat run doesn't change the system if it's already in the desired state. For Puppet this is critical: the agent applies the catalog every 30 minutes, and non-idempotent code will change something (or restart services) 48 times a day.

The built-in types are idempotent: `file` compares the checksum, `package` checks the installed version. The exception is **`exec`**: Puppet doesn't know what an arbitrary command does, and without conditions it runs it on every catalog application.

How to make `exec` idempotent:
```puppet
exec { 'init-swarm':
  command => '/usr/bin/docker swarm init',
  creates => '/var/lib/docker/swarm/state.json',     # skip if the file already exists
}

exec { 'set-hostname':
  command => "/usr/bin/hostnamectl set-hostname ${facts['networking']['fqdn']}",
  unless  => "/usr/bin/test \"$(hostname -f)\" = '${facts['networking']['fqdn']}'",   # skip if the command returned 0
}

exec { 'reload-sysctl':
  command     => '/usr/sbin/sysctl --system',
  refreshonly => true,          # run ONLY on a notification from another resource (notify/subscribe)
}
```
- `creates` — a path to a file: if it exists, the command isn't run;
- `onlyif` — run only if the check command returned 0;
- `unless` — run only if the check command returned NOT 0;
- `refreshonly => true` — run only when a notification arrived (the equivalent of a handler in Ansible).

Commands in `exec` are written **with the full path** (`/usr/bin/docker`) or with `path => ['/usr/bin', '/usr/sbin']`: the agent has its own environment, and `PATH` there may differ from your console.

The rule is the same as in Ansible: first look for a ready-made type or module on the Forge, `exec` is the last resort.

## The Puppet language: variables, facts, conditions, loops

**Variables** start with `$` and are **assigned once** in their scope — they can't be reassigned (it's a declarative language, not a script):
```puppet
$app_port = 8080
$app_port = 9090          # error: Cannot reassign variable '$app_port'
```

**Strings:** in single quotes — as is, in double quotes — with interpolation:
```puppet
$srv = 'web'
notice('Name: $srv')             # → Name: $srv
notice("Name: ${srv}-01")        # → Name: web-01
```
`notice()` prints a message during compilation — handy for debugging. The name `$name` doesn't work for your own variables: it's a built-in variable (in classes and defined types — the resource's name, like `$title`), and it can't be assigned.

**Facts** are available via the `$facts` hash:
```puppet
notice($facts['os']['family'])                  # → Debian
notice($facts['os']['release']['major'])        # → 24.04
notice($facts['networking']['ip'])              # → 10.0.1.15
notice($facts['processors']['count'])           # → 4
notice($facts['memory']['system']['total'])     # → 7.75 GiB
```
The old style `$::osfamily` or `$::ipaddress` (legacy facts — the "flat" facts from Puppet 3) is disabled by default in Puppet 8. If you see it in someone else's code, it's a sign of an old module.

**Trusted facts** (`$trusted`) are data from the node's certificate: `$trusted['certname']`, `$trusted['extensions']['pp_role']`. The node sends ordinary facts itself, and a compromised node can forge, for example, a `role` fact to get someone else's secrets. Trusted facts are signed by the CA and can't be forged, so decisions about what to give a node (a role, secrets) are made based on them.

**Data types** — class parameters are checked at compile time:
```puppet
String, Integer, Boolean, Array[String], Hash[String, Integer]
Enum['present', 'absent']                # one of the values
Optional[String]                         # a string or undef
Stdlib::Port, Stdlib::Absolutepath       # ready-made types from the puppetlabs-stdlib module
Sensitive[String]                        # a secret: not printed in logs and reports
```

**Conditions:**
```puppet
if $facts['os']['family'] == 'Debian' {
  $pkg = 'apache2'
} elsif $facts['os']['family'] == 'RedHat' {
  $pkg = 'httpd'
} else {
  fail("OS ${facts['os']['family']} is not supported")    # abort compilation with a clear error
}

case $facts['os']['family'] {
  'Debian': { $pkg = 'apache2' }
  'RedHat': { $pkg = 'httpd' }
  default:  { fail('not supported') }
}

# selector — choosing a value, shorter than case
$pkg = $facts['os']['family'] ? {
  'Debian' => 'apache2',
  'RedHat' => 'httpd',
}
```

**Loops** (iterating over an array or a hash):
```puppet
$users = {
  'alex' => { 'groups' => ['sudo', 'docker'] },
  'ci'   => { 'groups' => ['docker'] },
}
$users.each |String $name, Hash $opts| {
  user { $name:
    ensure     => present,
    groups     => $opts['groups'],
    shell      => '/bin/bash',
    managehome => true,
  }
}

['curl', 'jq', 'htop'].each |$p| {
  package { $p: ensure => installed }
}
```
Packages don't require a loop: `package { ['curl', 'jq', 'htop']: ensure => installed }` — an array in the title creates three resources.

## Order and dependencies

Puppet builds a **dependency graph** from the resources and applies it. Since Puppet 4 resources without explicit relationships run in the order they're written in the manifest, but you can't rely on that: between classes and modules the order isn't obvious. Dependencies are written explicitly.

**Relationship metaparameters** (they work on any resource):
- `require => Package['nginx']` — apply this resource **after** the given one;
- `before => File['/etc/nginx/nginx.conf']` — apply this resource **before** the given one;
- `notify => Service['nginx']` — after me, and if I changed — send a **refresh** to the given resource. For a `service` a refresh means a restart;
- `subscribe => File['/etc/nginx/nginx.conf']` — the same, but from the receiver's side.

Note: when referring to a resource the type is written **with a capital letter** (`Package['nginx']`), when declaring it — in lower case (`package { 'nginx': }`).

**The classic "package → file → service" pattern:**
```puppet
package { 'nginx':
  ensure => installed,
}

file { '/etc/nginx/nginx.conf':
  ensure  => file,
  content => epp('profile/nginx/nginx.conf.epp', { 'workers' => $facts['processors']['count'] }),
  require => Package['nginx'],          # put the config after the package is installed
  notify  => Service['nginx'],          # the config changed → restart nginx
}

service { 'nginx':
  ensure => running,
  enable => true,
}
```

**The same with arrows** (chaining arrows):
```puppet
Package['nginx'] -> File['/etc/nginx/nginx.conf'] ~> Service['nginx']
# ->  "before" (like before/require)
# ~>  "before and notify" (like notify/subscribe)
```

How a refresh in Puppet differs from a handler in Ansible: the notification fires only if the source resource really changed, and the service restarts **once**, even if three files changed. The restart happens right away within the same run, in graph order — there's no need to wait for the end of a play.

**Circular dependencies** are a common mistake: A requires B, B requires A. Compilation passes, but the agent fails with `Found 1 dependency cycle`. `puppet agent -t --graph` helps find the cycle: Puppet writes the graph into `.dot` files (`/opt/puppetlabs/puppet/cache/state/graphs/`), which can be opened with Graphviz.

**Relationships between classes** — `include` doesn't set the order. For a class's resources to end up "inside" another class and obey its dependencies, `contain` is used:
```puppet
class profile::app {
  contain profile::app::install      # contain: the class's resources count as part of profile::app
  contain profile::app::config
  contain profile::app::service

  Class['profile::app::install']
  -> Class['profile::app::config']
  ~> Class['profile::app::service']
}
```
Without `contain` the expression `Class['profile::app'] -> Class['other']` doesn't guarantee that the inner classes run before `other`.

## Classes and defined types

A **class** is a named block of resources that's applied to a node **once**: "nginx setup", "base OS setup".
```puppet
class profile::nginx (
  String                $version            = 'installed',
  Integer[1]            $worker_processes   = $facts['processors']['count'],
  Array[Stdlib::Port]   $listen_ports       = [80],
) {
  package { 'nginx':
    ensure => $version,
  }
  file { '/etc/nginx/nginx.conf':
    ensure  => file,
    content => epp('profile/nginx/nginx.conf.epp', {
      'worker_processes' => $worker_processes,
    }),
    require => Package['nginx'],
    notify  => Service['nginx'],
  }
  service { 'nginx':
    ensure => running,
    enable => true,
  }
}
```

**Two ways to pull in a class:**
```puppet
include profile::nginx              # include-like: can be called any number of times and from different places

class { 'profile::nginx':           # resource-like: with parameters right in the code
  version => '1.24.0-2ubuntu7',
}
```
A resource-like declaration can be made **only once** per catalog, and it conflicts with an `include` of the same class if that was declared earlier (`Duplicate declaration: Class[Profile::Nginx] is already declared`). So class parameters are almost always set via Hiera (see below), and the code contains only `include`.

A **defined type** is a resource template that can be declared **many times** with different parameters. It's needed for repeating entities: virtual hosts, application users, site configs.
```puppet
define profile::nginx::vhost (
  String               $server_name,
  Stdlib::Port         $port = 80,
  Stdlib::Absolutepath $root = "/var/www/${title}",    # $title — the name the resource was declared with
) {
  file { $root:
    ensure => directory,
  }
  file { "/etc/nginx/conf.d/${title}.conf":
    ensure  => file,
    content => epp('profile/nginx/vhost.conf.epp', {
      'server_name' => $server_name,
      'port'        => $port,
      'root'        => $root,
    }),
    notify  => Service['nginx'],
  }
}

profile::nginx::vhost { 'shop':
  server_name => 'shop.example.com',
}
profile::nginx::vhost { 'api':
  server_name => 'api.example.com',
  port        => 8080,
}
```
The difference in one phrase: a class is a singleton ("nginx on this machine"), a defined type can have many instances ("every site").

## Templates: EPP and ERB

A template generates a file's content from variables. Two formats:
- **EPP** (Embedded Puppet) — Puppet syntax inside the template, parameters are declared explicitly. Recommended.
- **ERB** (Embedded Ruby) — Ruby inside the template, found in old modules.

```epp
<%- | Integer $worker_processes | -%>
# Managed by Puppet (the profile module). Manual edits will be overwritten.
user www-data;
worker_processes <%= $worker_processes %>;

events {
    worker_connections 4096;
}

http {
    include /etc/nginx/conf.d/*.conf;
}
```
- `<%- | ... | -%>` — the template's parameter list with types: if you forget to pass a parameter, compilation fails with a clear error;
- `<%= expression %>` — insert a value;
- `<% code %>` — execute without output (conditions, loops);
- `-` on the tags removes extra blank lines.

Templates live in the module's `templates/` folder, and the `epp('module/path.epp', {...})` function looks for them there: `epp('profile/nginx/nginx.conf.epp')` → `profile/templates/nginx/nginx.conf.epp`.

Static files go into the module's `files/` and are pulled in via `source`:
```puppet
file { '/etc/logrotate.d/app':
  ensure => file,
  source => 'puppet:///modules/profile/logrotate-app',   # → profile/files/logrotate-app on the server
}
```

## Modules and the Forge

A **module** is a folder with a standard structure, the unit of reuse (like a role in Ansible):
```text
nginx/
├── manifests/
│   ├── init.pp          # class nginx — the module's main class
│   ├── install.pp       # class nginx::install
│   └── vhost.pp         # define nginx::vhost
├── templates/           # *.epp, *.erb
├── files/               # static files for source => 'puppet:///modules/nginx/...'
├── lib/facter/          # your own facts in Ruby
├── data/                # the module's data for Hiera (default values)
├── hiera.yaml
├── spec/                # rspec-puppet tests
└── metadata.json        # name, version, dependencies, supported OSes
```
A class's name reflects the path: `nginx::vhost` ↔ `manifests/vhost.pp`. Puppet finds classes automatically (autoloading), so the name and the path must match.

**PDK (Puppet Development Kit)** — generates a module skeleton, classes and tests, runs checks:
```bash
pdk new module profile
pdk new class profile::nginx
pdk validate            # syntax, puppet-lint, metadata
pdk test unit           # rspec-puppet tests
```

**Puppet Forge** (forge.puppet.com) is the public module catalog. The most needed: `puppetlabs-stdlib` (functions and data types), `puppetlabs-concat` (assemble a file from pieces from different resources), `puppetlabs-apt`, `puppetlabs-firewall`, `puppet-nginx`, `puppetlabs-haproxy`, `puppetlabs-postgresql`. Modules with the `puppet-` prefix are maintained by the **Vox Pupuli** community; they have good quality and tests.

## Environments, the control repo and r10k

An **environment** is a separate set of code on the server: `production`, `staging`, `feature_x`. A node works in one environment, and you can test changes on some of the nodes without touching prod.

The **control repo** is a git repository where **each branch = an environment**. It contains:
```text
control-repo/
├── Puppetfile            # which modules and which versions to download
├── environment.conf
├── hiera.yaml            # the environment's Hiera hierarchy
├── data/                 # Hiera data
├── manifests/site.pp     # node classification
└── site-modules/         # your own modules: role/, profile/
    ├── role/
    └── profile/
```

The **Puppetfile** is the list of external modules with pinned versions (like `requirements.txt` in Python):
```ruby
forge 'https://forge.puppet.com'

mod 'puppetlabs-stdlib',     '9.6.0'
mod 'puppetlabs-concat',     '9.0.2'
mod 'puppetlabs-postgresql', '10.3.0'

mod 'internal_app',
  git: 'git@gitlab.example.com:puppet/internal_app.git',
  tag: 'v2.4.1'
```

**r10k** is a utility that deploys the control repo on the server: for each branch it creates an environment folder and downloads the modules from the Puppetfile:
```bash
r10k deploy environment production --modules   # update the production environment together with its modules
r10k deploy environment --modules              # all branches
# → /etc/puppetlabs/code/environments/production
# → /etc/puppetlabs/code/environments/staging
# → /etc/puppetlabs/code/environments/feature_nginx_tuning
```
r10k is usually run by CI after a merge or by a webhook from GitLab. In Puppet Enterprise the built-in **Code Manager** does the same. **g10k** is a faster analogue of r10k written in Go.

To test a change on one node without touching the others:
```bash
git checkout -b feature_nginx_tuning && git push          # → r10k created the feature_nginx_tuning environment
sudo puppet agent -t --environment feature_nginx_tuning --noop   # what would change on this node
```

## Hiera: data separate from code

**Hiera** is a hierarchical "key → value" store in YAML built into Puppet. The idea: the module code is the same for everyone, while the differences between environments, roles and specific nodes (versions, ports, pool sizes, addresses) live in the data.

The **hierarchy** is described in `hiera.yaml` — from the most specific level to the most general. Hiera looks for a key from top to bottom and takes **the first value found**:
```yaml
# control-repo/hiera.yaml
---
version: 5
defaults:
  datadir: data
  data_hash: yaml_data
hierarchy:
  - name: "A specific node"
    path: "nodes/%{trusted.certname}.yaml"
  - name: "A role in an environment"
    path: "roles/%{trusted.extensions.pp_role}/%{server_facts.environment}.yaml"
  - name: "A role"
    path: "roles/%{trusted.extensions.pp_role}.yaml"
  - name: "The OS family"
    path: "os/%{facts.os.family}.yaml"
  - name: "Common"
    path: "common.yaml"
```
`%{...}` substitutes facts: for the node `web01.example.com` with the role `web` in the `production` environment Hiera will look in turn at `nodes/web01.example.com.yaml` → `roles/web/production.yaml` → `roles/web.yaml` → `os/Debian.yaml` → `common.yaml`.

**Automatic parameter lookup** is the main magic: Puppet looks up the class parameter `profile::nginx::worker_processes` in Hiera by itself, by the key `profile::nginx::worker_processes`. Nothing needs to be written in the code:
```yaml
# data/common.yaml
profile::nginx::worker_processes: 2
profile::nginx::listen_ports: [80]

# data/roles/web/production.yaml
profile::nginx::worker_processes: 8
profile::nginx::listen_ports: [80, 443]
```
The order a parameter's value comes from: explicitly in `class { ... }` → Hiera → the default value in the class's code.

**The lookup function** — when a value is needed explicitly:
```puppet
$ntp_servers = lookup('profile::base::ntp_servers', Array[String], 'first', ['169.254.169.123'])
#                      key                            type          strategy  default value
```
**Merge strategies** — what to do if a key is found on several levels:
- `first` (the default) — the first value found;
- `unique` — combine arrays from all levels without duplicates (for example, a package list: common + for the role);
- `hash` — combine hashes, the upper level wins when keys collide;
- `deep` — a recursive merge of nested hashes.

The strategy for a specific key can be set in the data:
```yaml
lookup_options:
  profile::base::packages:
    merge: unique
```

To debug where a value came from:
```bash
puppet lookup profile::nginx::worker_processes --node web01.example.com --explain
# → Searching for "profile::nginx::worker_processes"
# →   Path "data/nodes/web01.example.com.yaml"  — not found
# →   Path "data/roles/web/production.yaml"     — Found key: value: 8
```

## Roles and profiles

**Roles and profiles** is the standard pattern for organising Puppet code. It's asked about in almost every interview where Puppet comes up.

Three layers:
1. **Component modules** — generic modules from the Forge or your own (`nginx`, `postgresql`, `docker`). They know nothing about your company.
2. **Profile** — a wrapper for "how we use the technology here": it takes a component module, passes it our parameters from Hiera, adds our specifics (monitoring, logs, firewall). Examples: `profile::nginx`, `profile::base`, `profile::postgresql`.
3. **Role** — the server's business role, **only** a list of profiles and nothing else. Examples: `role::web`, `role::db`.

```puppet
# site-modules/role/manifests/web.pp
class role::web {
  include profile::base          # users, SSH, NTP, monitoring — on every server
  include profile::nginx
  include profile::app
}

# site-modules/profile/manifests/nginx.pp
class profile::nginx (
  Integer $worker_processes,     # the value comes from Hiera
) {
  class { 'nginx':               # a component module from the Forge
    worker_processes => $worker_processes,
    server_tokens    => 'off',
  }
  include profile::monitoring::nginx_exporter
}
```
Rules:
- **Each node has exactly one role.** If you need "web + a bit of cache", a new role `role::web_with_cache` is created, not two roles on the node.
- A role has no resources or parameters — only `include profile::...`.
- Resource-like `class { 'nginx': ... }` is written only in profiles, and the profile takes its data from Hiera.

Why: a role reads as documentation ("what this server is"), a profile is the only place where a technology and our settings are tied together, and component modules can be updated from the Forge without rewriting your own code.

## Node classification: what to apply to which node

**Classification** is how the server decides which role a node gets. Options:

**1. site.pp** — by node name right in the code:
```puppet
# manifests/site.pp
node 'db01.example.com' {
  include role::db
}
node /^web\d+\.example\.com$/ {           # a regular expression on the certname
  include role::web
}
node default {
  include role::base
}
```
The downside: with a thousand servers the file turns into a long list, and names don't always reflect the role.

**2. The role from trusted facts** — the role is baked into the certificate when the node first registers, and `site.pp` becomes a single line:
```yaml
# /etc/puppetlabs/puppet/csr_attributes.yaml on the node — BEFORE the agent's first run
extension_requests:
  pp_role: web
  pp_environment: production
```
```puppet
# manifests/site.pp
node default {
  include "role::${trusted['extensions']['pp_role']}"
}
```
This file is conveniently created from user data when a VM launches (Terraform → cloud-init → csr_attributes.yaml → the agent). Since the role is signed by the CA, a node can't "promote" its own role.

**3. ENC (External Node Classifier)** — an external script or service that returns YAML with classes and an environment for a node name. This is how the Puppet Enterprise console, Foreman and in-house CMDBs (server inventory databases) work.

## Secrets

Passwords, keys and tokens can't be put into Hiera in plain text: everyone sees the repository, and the values end up in catalogs and reports.

**hiera-eyaml** encrypts individual values in Hiera YAML files (PKCS7 by default: anyone can encrypt with the public key, the private key lives only on the Puppet server):
```bash
eyaml createkeys                                    # a key pair, the private one — only on the Puppet server
eyaml encrypt -l 'profile::db::password' -s 'S3cr3t!'
# → profile::db::password: ENC[PKCS7,MIIBiQYJKoZIhvcNAQcDoIIBejCCAXYCAQAx...]
```
```yaml
# hiera.yaml — the level with secrets
  - name: "Secrets"
    lookup_key: eyaml_lookup_key
    paths:
      - "secrets/nodes/%{trusted.certname}.eyaml"
      - "secrets/common.eyaml"
    options:
      pkcs7_private_key: /etc/puppetlabs/puppet/eyaml/private_key.pkcs7.pem
      pkcs7_public_key: /etc/puppetlabs/puppet/eyaml/public_key.pkcs7.pem
```

**The Sensitive type** — a value is marked as a secret and isn't printed in logs, `--noop` diffs and reports:
```puppet
class profile::db (
  Sensitive[String] $password,          # comes from Hiera already as Sensitive (if lookup_options sets convert_to: Sensitive)
) {
  postgresql::server::role { 'app':
    password_hash => postgresql::postgresql_password('app', $password),
  }
  notice($password)                     # → Sensitive [value redacted]
  notice($password.unwrap)              # → S3cr3t!  — unwrap gets the value; never write it to logs like this
}
```
```yaml
lookup_options:
  profile::db::password:
    convert_to: Sensitive
```

**External stores** — Deferred functions: the value is fetched **on the node at apply time** rather than on the server at compile time, so the secret never gets into the catalog at all:
```puppet
$token = Deferred('vault_lookup::lookup', ['secret/data/app/api', 'https://vault.example.com:8200'])
```
(a function from the `puppet-vault_lookup` module; the node authenticates to HashiCorp Vault with its Puppet certificate).

## Facter and your own facts

Facts are what decisions in the code and in the Hiera hierarchy are based on.
```bash
facter                         # all facts
facter os                      # a nested hash
facter networking.ip
facter -p                      # plus the facts that Puppet modules added to the node (custom facts)
puppet facts show os.family    # the same via puppet
```

**External facts** — a simple file on the node, no programming:
```yaml
# /etc/puppetlabs/facter/facts.d/datacenter.yaml
datacenter: fra1
rack: r12
```
It can also be an executable script in the same folder that prints `key=value`.

**Your own facts in Ruby** — in a module, `lib/facter/*.rb`; the server distributes them to the nodes itself (pluginsync):
```ruby
# site-modules/profile/lib/facter/app_version.rb
Facter.add(:app_version) do
  confine kernel: 'Linux'                          # Linux only
  setcode do
    path = '/opt/app/VERSION'
    File.read(path).strip if File.exist?(path)     # nil → the fact won't appear
  end
end
```
```puppet
if $facts['app_version'] and versioncmp($facts['app_version'], '2.0.0') < 0 {
  notice('an old application version, a migration is needed')
}
```
Remember: the node sends ordinary facts itself, so for security (which role and which secrets to give) trusted facts are used.

## Virtual and exported resources

A **virtual resource** is declared but not applied until it's explicitly "realized". It solves the duplicates problem: both the admins profile and the developers profile want to create the user `alex`, but a resource can't be declared twice.
```puppet
# declare all users once, virtually (the @ symbol)
@user { 'alex': ensure => present, groups => ['sudo'], tag => 'admins' }
@user { 'olga': ensure => present, groups => ['dev'],  tag => 'developers' }

# realize them where needed
realize(User['alex'])
User <| tag == 'developers' |>        # a collector: realize all virtual Users tagged developers
```

An **exported resource** — a node declares a resource "for others", it's stored in PuppetDB, and other nodes collect it. This way services find each other without manual lists.

The classic example: web servers register themselves in the HAProxy load balancer:
```puppet
# profile::app — on every web node
@@haproxy::balancermember { $facts['networking']['fqdn']:    # @@ — export
  listening_service => 'web',
  server_names      => $facts['networking']['hostname'],
  ipaddresses       => $facts['networking']['ip'],
  ports             => '8080',
  options           => 'check',
}

# profile::loadbalancer — on the load balancer
haproxy::listen { 'web':
  ipaddress => '0.0.0.0',
  ports     => '80',
}
Haproxy::Balancermember <<| listening_service == 'web' |>>    # collect all the exported ones from PuppetDB
```
How this works over time: a new web node sends the resource to PuppetDB during its agent run → on the next agent run on the load balancer (up to 30 minutes) it gets into the HAProxy config. This is **not instant**, and a removed node has to be cleaned out of PuppetDB (`puppet node deactivate web07.example.com`), otherwise it stays in load balancing. For fast-changing cloud groups, service discovery (Consul, cloud target groups) is better.

## Running, noop, reports and debugging

```bash
sudo puppet agent -t                 # --test: run now, verbose output, once, without the daemon
sudo puppet agent -t --noop          # a "dry run": show what would change without changing anything
sudo puppet agent -t --debug         # as verbose as possible
sudo puppet agent -t --tags nginx    # apply only resources with a tag (a class's tag = its name)
sudo puppet agent --disable "INC-1234: manual diagnostics, alex"   # temporarily disable the agent with a reason
sudo puppet agent --enable
sudo puppet apply --noop site.pp     # the same for a local manifest
```

**Exit codes** with `--detailed-exitcodes` (enabled automatically in `-t`) — checked in CI and scripts:
- `0` — success, no changes;
- `2` — success, there were changes;
- `4` — there were errors;
- `6` — there were both changes and errors.

A typical trap: a script treats any non-zero code as an error and fails on `2`, although that's a normal successful run with changes.

**Where to look:**
- `/opt/puppetlabs/puppet/cache/state/last_run_summary.yaml` — the summary of the last run on the node (time, number of changed and failed resources). Handy for monitoring: an alert if the last successful run was more than 2 hours ago;
- `journalctl -u puppet` — the agent's log;
- reports in PuppetDB and dashboards (Puppetboard, the Puppet Enterprise console) — changes across all nodes;
- `/var/log/puppetlabs/puppetserver/puppetserver.log` — the server's log.

**Checking code before a commit:**
```bash
puppet parser validate manifests/      # syntax
puppet-lint site-modules/              # style: indentation, quotes, line length, documentation
puppet epp validate templates/         # EPP templates
```

## Testing and CI

Levels of checking, from cheap to expensive:
1. **puppet parser validate, puppet-lint, yamllint** — syntax and style, seconds.
2. **rspec-puppet** — unit tests: it compiles a class's catalog with given facts and checks that it contains the needed resources. No machine is needed.
3. **Onceover** — compiles the catalogs of **all roles** in the control repo for a set of typical nodes: it catches Hiera errors and conflicts between profiles.
4. **Litmus** (or the outdated Beaker) — acceptance tests: it brings up a container or VM, applies the class for real, checks idempotency (a second run without changes) and the server's state.
5. **noop on real nodes** — CI runs `puppet agent -t --noop --environment <branch>` on a few nodes and shows the diff in the merge request.

An rspec-puppet example:
```ruby
# site-modules/profile/spec/classes/nginx_spec.rb
require 'spec_helper'

describe 'profile::nginx' do
  on_supported_os.each do |os, os_facts|          # for each OS from metadata.json
    context "on #{os}" do
      let(:facts) { os_facts }
      let(:params) { { 'worker_processes' => 4 } }

      it { is_expected.to compile.with_all_deps }                      # the catalog compiles, all dependencies exist
      it { is_expected.to contain_package('nginx') }
      it { is_expected.to contain_service('nginx').with_ensure('running') }
      it { is_expected.to contain_file('/etc/nginx/nginx.conf').with_content(%r{worker_processes 4;}) }
    end
  end
end
```

A control repo pipeline in GitLab CI:
```yaml
stages: [lint, test, deploy]

lint:
  stage: lint
  image: registry.example.com/tools/pdk:3
  script:
    - puppet parser validate site-modules/ manifests/
    - puppet-lint --fail-on-warnings site-modules/
    - yamllint data/

unit:
  stage: test
  image: registry.example.com/tools/pdk:3
  script:
    - bundle exec onceover run spec        # the catalogs of all roles

deploy:
  stage: deploy
  script:
    - ssh puppet@puppet.example.com "r10k deploy environment ${CI_COMMIT_REF_NAME} --modules"
  rules:
    - if: $CI_COMMIT_BRANCH
```
Each branch becomes an environment; after a merge to `production` all nodes pick up the changes within `runinterval`. To avoid rolling out a change to a thousand servers at once, a canary environment is used: the branch is first applied to a group of nodes, then merged.

## Scale and operations

**Server load.** Compiling a catalog is the most expensive operation. Every node comes once every 30 minutes: 3000 nodes ≈ 100 compilations a minute. What helps:
- **compilers** (compiling servers) behind a load balancer: the primary server keeps the CA, while several additional ones compile the catalogs;
- tuning the JVM and the number of Puppet Server JRuby interpreters (`max-active-instances` — how many catalogs are compiled in parallel);
- **splay** (`splay = true` in `puppet.conf`) — a random delay before a run, so that all agents don't arrive at once after a restart or a network outage;
- fewer `exec`s, heavy functions and PuppetDB queries in the code.

**Certificates.**
```bash
puppetserver ca list                                  # requests waiting to be signed
puppetserver ca sign --certname web01.example.com
puppetserver ca clean --certname web01.example.com    # revoke when a node is removed
sudo puppet ssl clean                                 # on the node: delete its own certificates (when recreating it)
```
Autosigning (`autosign`) is handy in the cloud, but signing everything is dangerous: anyone who can reach port 8140 gets a catalog, and the secrets with it. The safe option is **policy-based autosign**: a script checks a shared secret from `csr_attributes.yaml` (`challengePassword`) or that the instance really exists in the cloud.

A frequent problem: a VM was recreated with the same name, while the old certificate stayed on the server → the agent fails with a certificate verification error. The fix is `puppetserver ca clean` on the server and `puppet ssl clean` on the node.

**Bolt** is a tool from Puppet's creators for one-off operations and orchestration **without an agent**, over SSH or WinRM (essentially what Ansible does):
```bash
bolt command run 'systemctl restart nginx' --targets web01,web02
bolt plan run profile::rolling_restart --targets web    # a plan: steps in order, for example one host at a time
```
The classic combination: Puppet maintains the constant state, Bolt (or Ansible) is for one-off actions where the order of steps matters.

## Versions and OpenVox

- **Puppet 8** (2023) is the current major version: Ruby 3.2, `strict` mode is on by default (referring to an unknown variable is an error rather than a warning), legacy facts are disabled by default.
- The Puppet company was bought by **Perforce** in 2022. Since late 2024 Perforce publishes new Puppet binary builds only for commercial customers (free with a limit on the number of nodes), while the source code remains open.
- In response the community (Vox Pupuli) made the **OpenVox** fork — an agent, server and PuppetDB compatible at the code level, with open packages (`openvox-agent`, `openvox-server`). Module and manifest code doesn't change.
- **Puppet Enterprise** is the commercial version: a web console, point-and-click node classification, an orchestrator, Code Manager, compliance reports.

In an interview this knowledge shows that you follow the ecosystem: if the company is currently on open source Puppet, the question "will you move to OpenVox or buy a licence" is appropriate.

## If your experience is in Ansible but they ask about Puppet

What to say honestly and usefully:
- the concepts are the same: idempotency, desired state, separating code and data (group_vars ↔ Hiera), reusable blocks (Ansible roles ↔ modules and profiles), encrypted secrets (Vault ↔ eyaml);
- the main differences that you understand: the agent and the pull model with continuous drift correction, catalog compilation on the server, a dependency graph instead of task order, certificates and mTLS;
- ready to show an example: "nginx: a package → a config from a template → a service restarted on change" — the same scenario in Ansible and in Puppet.

Mapping the concepts:

| Ansible | Puppet |
|---|---|
| playbook | a manifest (`.pp`), a catalog |
| task + module | a resource + a type and provider |
| role | a module; the roles and profiles pattern |
| inventory, groups | node classification (site.pp, trusted facts, ENC) |
| group_vars / host_vars | Hiera with a hierarchy |
| handler + notify | `notify` / `subscribe`, `~>` |
| `when` | `if`, `case`, selector |
| Ansible Vault | hiera-eyaml, Sensitive, Deferred + HashiCorp Vault |
| `--check --diff` | `--noop` |
| Molecule | rspec-puppet, Litmus, Onceover |
| ansible-galaxy, requirements.yml | Puppet Forge, Puppetfile, r10k |

## Typical mistakes

- **`exec` without `creates`/`onlyif`/`unless`/`refreshonly`** — the command runs on every agent run, i.e. 48 times a day.
- **`ensure => latest` in prod** — the package updates on all servers within 30 minutes of a new version appearing in the repository, untested. A pinned version in Hiera is needed.
- **A duplicate resource** — the same `package { 'git': }` is declared in two modules → `Duplicate declaration`. Solutions: one profile owns the resource, `ensure_packages` from stdlib, or virtual resources.
- **A resource-like class declaration in several places** — the same `Duplicate declaration`. In code — only `include`, parameters — in Hiera.
- **Logic in Hiera, data in code** — versions, ports and addresses hardcoded in manifests, while conditions are written in Hiera. It should be the other way round.
- **Secret decisions based on ordinary facts** — a node can forge the `role` fact. For security — `$trusted`.
- **Manual edits on a server** — Puppet reverts everything within 30 minutes. If it's urgent — `puppet agent --disable "reason"`, the edit, then a commit to the code and `--enable`.
- **A forgotten `puppet agent --disable`** — the agent is off for months, and the server silently drifts from the code. Monitor `last_run_summary.yaml`.
- **Circular dependencies** from `require` in both directions — `Found 1 dependency cycle`.
- **`include` instead of `contain` where order is needed** — the dependency `Class['a'] -> Class['b']` doesn't extend to the classes inside `a`.
- **Dead nodes in PuppetDB** — exported resources of removed servers stay in load balancer and monitoring configs. `puppet node deactivate` when a node is removed.
- **Autosigning all certificates** — anyone on the network gets a catalog with secrets.

## Interview questions

**What is Puppet and how does one agent run work?**
A configuration management tool with a declarative language and an agent on every node. The agent collects facts via Facter, sends them to Puppet Server, the server classifies the node, takes data from Hiera and compiles a catalog — a list of resources with concrete values and a dependency graph. The agent applies the catalog, changing only what differs, and sends a report. By default this repeats every 30 minutes.

**How does Puppet differ from Ansible?**
Puppet is a pull model with an agent: nodes come for the catalog themselves every 30 minutes and constantly correct drift. Ansible is agentless push over SSH, changes happen only when it's run. Puppet has its own declarative language and a dependency graph, Ansible has YAML with tasks run in order. Puppet is stronger at constant state control of thousands of long-lived servers and reporting, Ansible — at simplicity, one-off operations and orchestrating sequential steps.

**How does Puppet differ from Terraform?**
Terraform creates infrastructure (VPCs, VMs, managed databases) via provider APIs and keeps a state file. Puppet configures what's inside the OS and keeps no state — each time it compares with reality. They usually work together: Terraform creates a VM and registers it in Puppet with the right role via user data.

**What is a catalog and where is it compiled?**
The final document for a specific node: all resources with computed values and the relationships between them. It's compiled on Puppet Server from the code, the node's facts and Hiera data. The node receives a ready catalog; Puppet code doesn't run on it (except for facts and Deferred functions).

**What is idempotency and how do you make exec idempotent?**
Re-applying doesn't change the system if it's already in the desired state. The built-in types are idempotent by themselves, while `exec` runs every time. For it, `creates` (skip if the file exists), `onlyif` and `unless` (a check command) and `refreshonly => true` (only on a notification) are used. It's better to replace `exec` with a ready-made type or module.

**How do you set the order of resources? How does require differ from notify?**
With the metaparameters `before`, `require`, `notify`, `subscribe` or the arrows `->` and `~>`. `require` and `before` only set the order. `notify` and `subscribe` set the order plus a refresh: if the source resource changed, the receiver is refreshed (the service is restarted). The restart happens once, even if several files changed.

**How does a class differ from a defined type?**
A class is applied to a node once (a singleton): "nginx setup". A defined type is a resource template that can be declared many times with different parameters and a unique title: "a virtual host", "an application user".

**How does include differ from a resource-like class declaration? And contain?**
`include` can be called any number of times, the parameters come from Hiera. Resource-like `class { 'x': param => ... }` sets the parameters in code but is allowed only once per catalog, otherwise `Duplicate declaration`. `contain` works like `include` but makes the class's resources part of the current class, so that dependencies like `Class['a'] -> Class['b']` extend to them.

**What is Hiera and how does the hierarchy work?**
A built-in "key → value" data store in YAML that separates data from code. `hiera.yaml` defines a hierarchy of levels from specific to general (node → role in an environment → role → OS → common), with facts substituted into the paths. Hiera looks for a key from top to bottom and by default takes the first value. The `unique`, `hash` and `deep` strategies merge values from several levels. Class parameters are looked up automatically by the key `class::parameter`.

**What is the roles and profiles pattern?**
Three layers: component modules (generic, from the Forge), profiles (how we use a technology: a module plus our data from Hiera and our specifics) and roles (the server's business role — only a list of profiles). Each node has exactly one role. A role reads as documentation, a profile is the only place where a technology is tied to our settings.

**How do you classify nodes?**
Via `site.pp` with `node` blocks by name or regular expression, via a role in trusted facts (pp_role from csr_attributes.yaml, baked into the certificate at registration) or via an ENC — an external service that returns classes and an environment for a node name (the Puppet Enterprise console, Foreman, a CMDB).

**How do facts differ from trusted facts?**
The node collects and sends ordinary facts itself, and a compromised node can forge them. Trusted facts come from the certificate signed by the CA (the certname and extensions like pp_role) and can't be forged. Security-relevant decisions (the role, handing out secrets) are made based on trusted facts.

**How do you store secrets in Puppet?**
hiera-eyaml: values in Hiera are encrypted with the public key, the private key lives only on the server. The `Sensitive` type keeps a value out of logs, diffs and reports. For external stores — Deferred functions (for example, `vault_lookup`): the secret is requested on the node at apply time and doesn't get into the catalog. Plus trusted facts in the hierarchy, so that a node gets only its own secrets.

**What are environments and how does r10k work?**
An environment is a separate set of code on the server; a node works in one of them. The control repo keeps the code in git, each branch = an environment. r10k deploys the branches into environment folders and downloads modules at the versions from the Puppetfile. This way a branch can be tested on some of the nodes via `--environment` and `--noop` without touching prod.

**What are exported resources and what are their limitations?**
A node declares a resource with `@@`, it's stored in PuppetDB, and other nodes collect it with the `<<| |>>` collector: for example, web servers register in HAProxy and hosts in monitoring. Limitations: PuppetDB is required, changes reach the receiver only on its next run (up to 30 minutes), and removed nodes have to be deactivated in PuppetDB, otherwise their resources remain.

**What are virtual resources and why are they needed?**
A resource declared with `@` that isn't applied until it's realized via `realize()` or a `<| |>` collector. It solves the duplicates problem when the same resource (a user, a package) is needed by several profiles but can be declared only once.

**How do you test a change without breaking prod?**
`puppet parser validate` and `puppet-lint` in CI, rspec-puppet unit tests, Onceover for the catalogs of all roles. Then the branch becomes an environment, and `puppet agent -t --noop --environment <branch>` is run on a few nodes. After the merge — first to a canary group of nodes. In operation — monitoring errors in PuppetDB reports.

**What do the puppet agent exit codes mean?**
With `--detailed-exitcodes` (enabled in `-t`): 0 — success without changes, 2 — success with changes, 4 — errors, 6 — changes and errors. Code 2 isn't an error; scripts and CI have to account for that.

**The agent on a node fails with a certificate error. What do you do?**
Most often the node was recreated with the same name and the old certificate remained on the server, or an old one remained on the node. On the server `puppetserver ca clean --certname <name>`, on the node `puppet ssl clean`, then `puppet agent -t` and sign the new request. Also check the time on the node (a certificate "from the future" won't pass verification) and that the certname matches the expected one.

**Puppet Server can't keep up with the load. What do you do?**
Add compilers behind a load balancer, keeping the CA on the primary server. Tune the JVM and the number of JRuby instances. Enable `splay` so that agents don't arrive at the same time. Find expensive classes from compile-time metrics and simplify them: fewer heavy functions and PuppetDB queries. If needed, increase `runinterval`.

**Someone edited a config on a server by hand. What happens, and how should urgent changes be made?**
On the next agent run (within 30 minutes) Puppet restores the file to the described form and restarts the service if that's set up via notify. The right way: for urgent diagnostics — `puppet agent --disable "reason"`, then the change in the Puppet code, a noop check and `--enable`. A manual edit not carried over into the code will be lost.

**What has changed around Puppet in recent years?**
Puppet 8: Ruby 3.2, strict mode and legacy facts disabled by default. The company was bought by Perforce, and since late 2024 new binary builds are available only under a commercial licence (free with a limit on the number of nodes). The Vox Pupuli community released the compatible open fork OpenVox (agent, server, PuppetDB); module and manifest code doesn't change when migrating.
