# Ansible

A thorough Ansible primer: how it works, inventory, playbooks, modules and idempotency, variables and their precedence, templates, handlers, roles, Vault, zero-downtime rolling updates, speed, testing and CI. Each topic: what it is, why, how it works, an example with explanations and typical mistakes. At the end — interview questions with answers.

How to read: build the lab from the next section and run the examples as you go. Ansible's output is shown in comments after `# →`.

## How to learn without a course

Ansible is best learned hands-on: write a task → run it → run it again and make sure nothing changed the second time. For that you need "servers" you don't mind breaking.

**Installing on macOS:**
```bash
brew install ansible            # installs the ansible package: ansible-core + a set of collections (community.*, amazon.aws etc.)
ansible --version               # the version, the path to ansible.cfg and to the Python Ansible runs on
```
`ansible-core` is the engine itself plus the built-in `ansible.builtin.*` modules. The `ansible` package is the engine plus a large set of collections. This difference is sometimes asked about in interviews.

**Lab option 1 — Docker containers (fast, but without systemd):**
```bash
for n in node1 node2 node3; do
  docker run -d --name "$n" python:3.12-slim sleep infinity   # the image already has Python — Ansible needs it on the target
done
```
```ini
# inventory.ini
[lab]
node1
node2
node3

[lab:vars]
ansible_connection=community.docker.docker   # connect via docker exec rather than SSH
```
```bash
ansible -i inventory.ini lab -m ansible.builtin.ping
# → node1 | SUCCESS => {"changed": false, "ping": "pong"}
```
The limitation: there's no systemd inside a container, so the `service`/`systemd` modules won't work.

**Lab option 2 — virtual machines (like real life):** `brew install multipass`, then `multipass launch -n node1`, and so on for 2–3 machines. These are full Ubuntu machines with SSH and systemd. Look up the machines' IPs with `multipass list`, and add your key to `~/.ssh/authorized_keys` of the `ubuntu` user on each machine.

The plan: inventory → ad-hoc commands → first playbook → variables and templates → handlers → a role → Vault → rolling update. After each step — a repeat run and a check that `changed=0`.

## What Ansible is and how it works

**Ansible** is a configuration management and automation tool: it installs packages, lays out configs, creates users, restarts services, deploys applications — on tens and hundreds of servers with one command.

The main properties:
- **Agentless** — no agent is needed on the servers. Only SSH access and Python (for Linux). Compare with Puppet and Chef, where an agent runs on every server and periodically pulls configuration from a central server.
- **Push model** — you (or CI) launch the changes from a managing machine (the **control node**), rather than the servers pulling them on a schedule.
- **Declarative tasks** — you describe the desired state ("the nginx package is installed", "a file with this content is here"), and the module itself checks the current state and changes only what differs.
- **YAML** — playbooks read almost like text, the entry barrier is low.

**What happens when a task runs** (simplified):
1. Ansible reads the inventory and selects the hosts.
2. It connects to each host over SSH (in parallel, by default up to 5 hosts at once — the `forks` parameter).
3. For each task it takes the module's code (Python), fills in the arguments and copies it to the host into a temporary folder `~/.ansible/tmp/...`.
4. It runs the module on the host via Python. The module checks the state, changes it if needed and returns JSON: `changed`, `failed`, data.
5. It deletes the temporary files and moves on to the next task.

By default Ansible runs **a task on all hosts, then the next task on all hosts** (the `linear` strategy). If a task failed on a host, that host drops out of the play's remaining tasks, while the others continue.

**Ansible and Terraform aren't competitors.** Terraform creates infrastructure (VPCs, VMs, load balancers) and keeps state. Ansible configures what has already been created (OS, packages, configs, deployment) and keeps no state — each time it checks the real state anew. A typical combination: Terraform brought up EC2 instances → Ansible configured them.

### 🧪 Mini task

**Task.** With no agents and no SSH, treat your local machine as a managed host: check connectivity with the `ping` module and run a command with the `command` module.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

ansible all -i hosts -m ping
ansible all -i hosts -m command -a 'uname -s'
```

Expected output:

```text
localhost | SUCCESS => {
    "changed": false,
    "ping": "pong"
}
localhost | CHANGED | rc=0 >>
Darwin
```

Breakdown: Ansible works in a push model without agents: the control machine connects to the host (usually over SSH, here `ansible_connection=local`), copies a small Python module there, runs it and collects the JSON result. `ping` isn't ICMP but a check that "I can connect and run Python". `command` always reports `CHANGED` because Ansible can't know what an arbitrary command changes. That's why changes are made with dedicated modules that check the current state themselves.

</details>

## Inventory

The **inventory** is the list of hosts Ansible manages, split into groups. Without an inventory Ansible doesn't know where to connect.

**INI format:**
```ini
# inventory/prod.ini
[web]
web1.example.com
web2.example.com ansible_host=10.0.1.12        # one name for Ansible, but connect to this IP

[db]
db1 ansible_host=10.0.2.10 ansible_user=admin ansible_port=2222

[swarm_managers]
swarm-m[1:3].example.com                        # a range: swarm-m1, swarm-m2, swarm-m3

[prod:children]                                 # a group of groups
web
db
swarm_managers
```

**The same in YAML:**
```yaml
# inventory/prod.yml
all:
  children:
    web:
      hosts:
        web1.example.com:
        web2.example.com:
          ansible_host: 10.0.1.12
    db:
      hosts:
        db1:
          ansible_host: 10.0.2.10
          ansible_user: admin
          ansible_port: 2222
```

There are two built-in groups: `all` (all hosts) and `ungrouped` (hosts without a group). A host can belong to several groups at once.

**Connection variables** (Ansible itself understands them):
| Variable | Meaning |
|---|---|
| `ansible_host` | the address to connect to, if the host name doesn't resolve |
| `ansible_user` | the SSH user |
| `ansible_port` | the SSH port |
| `ansible_connection` | the connection method: `ssh` (default), `local`, `community.docker.docker`, `winrm` |
| `ansible_python_interpreter` | the path to Python on the host, if auto-detection got it wrong |
| `ansible_become`, `ansible_become_user` | whether to escalate privileges and to whom |

**Group and host variables are moved into folders** rather than written into the inventory:
```text
inventory/
├── prod.ini
├── group_vars/
│   ├── all.yml          # for all hosts
│   ├── web.yml          # for the web group
│   └── db/              # can be a folder: all the files inside are read
│       ├── main.yml
│       └── vault.yml    # the db group's encrypted secrets
└── host_vars/
    └── db1.yml          # only for the db1 host
```
Ansible looks for `group_vars/` and `host_vars/` next to the inventory file and next to the playbook.

**Checking how Ansible sees the inventory:**
```bash
ansible-inventory -i inventory/prod.ini --graph            # the tree of groups and hosts
ansible-inventory -i inventory/prod.ini --host db1         # all the host's variables after merging
ansible -i inventory/prod.ini web --list-hosts             # which hosts the pattern selects
```

**Dynamic inventory.** In the cloud servers appear and disappear, so a static list goes stale quickly. An inventory plugin queries the cloud API itself:
```yaml
# inventory/aws_ec2.yml — the file name must end with aws_ec2.yml
plugin: amazon.aws.aws_ec2
regions:
  - eu-central-1
filters:
  instance-state-name: running
  tag:Project: shop
keyed_groups:
  - key: tags.Role               # the tag Role=web → the group role_web
    prefix: role
hostnames:
  - private-ip-address
compose:
  ansible_host: private_ip_address
```
```bash
ansible-inventory -i inventory/aws_ec2.yml --graph
# → @all:
# →   |--@role_web:
# →   |  |--10.0.1.15
# →   |  |--10.0.1.27
```
The plugin needs AWS credentials (a profile, environment variables or an EC2 role) and the `boto3`/`botocore` libraries.

**Host patterns** in commands and in a playbook's `hosts:`: `web` — a group; `web:db` — a union; `web:&prod` — an intersection; `prod:!db` — prod without db; `web[0]` — the group's first host.

### 🧪 Mini task

**Task.** Write a YAML inventory: a `prod` group with `web` (two hosts) and `db` subgroups, with variables at group and host level. Print the members of the `web` group and the final variables of host `web1`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > inventory.yml <<'EOF'
prod:
  vars:
    env: prod
  children:
    web:
      vars:
        http_port: 8080
      hosts:
        web1: {ansible_host: 10.0.1.11}
        web2: {ansible_host: 10.0.1.12}
    db:
      hosts:
        db1: {ansible_host: 10.0.2.21}
EOF
ansible-inventory -i inventory.yml --graph web
ansible-inventory -i inventory.yml --host web1
```

Expected output:

```text
@web:
  |--web1
  |--web2
{
    "ansible_host": "10.0.1.11",
    "env": "prod",
    "http_port": 8080
}
```

Breakdown: `web1` got variables from every level: `env` from the parent group `prod`, `http_port` from `web` and `ansible_host` from itself. `ansible-inventory --host` shows the final merge result — the first thing to check when "the variable is wrong". In production variables usually move out of the inventory into `group_vars/<group>.yml` and `host_vars/<host>.yml`, and for the cloud a dynamic inventory is used (the `amazon.aws.aws_ec2` plugin builds groups from EC2 tags).

</details>

## ansible.cfg

`ansible.cfg` is the settings file. Ansible takes **the first one found** (it doesn't merge them):
1. the path from the `ANSIBLE_CONFIG` environment variable;
2. `./ansible.cfg` in the current folder;
3. `~/.ansible.cfg`;
4. `/etc/ansible/ansible.cfg`.

An important security detail: `ansible.cfg` from the current folder **is ignored if the folder is world-writable**. Otherwise any user could slip in their own config. Ansible warns about this in its output.

A typical `ansible.cfg` in a project repository:
```ini
[defaults]
inventory = inventory/prod.ini
remote_user = deploy
roles_path = roles
forks = 20                     ; hosts in parallel (5 by default)
host_key_checking = True       ; don't disable in prod: protects against a spoofed server
callback_result_format = yaml  ; readable output of results and errors (ansible-core 2.13+)
interpreter_python = auto_silent

[ssh_connection]
pipelining = True              ; fewer SSH operations per task, noticeably faster
ssh_args = -o ControlMaster=auto -o ControlPersist=60s
```
To see what was actually applied: `ansible-config dump --only-changed`.

### 🧪 Mini task

**Task.** Put an `ansible.cfg` in the project folder with 20 forks, a 15-second connection timeout and host key checking disabled. Use `ansible-config` to see which settings differ from the defaults and where they came from.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > ansible.cfg <<'EOF'
[defaults]
forks = 20
timeout = 15
host_key_checking = False
EOF
ansible-config dump --only-changed | grep -E '^(DEFAULT_FORKS|DEFAULT_TIMEOUT|HOST_KEY_CHECKING)' | sed 's/(.*)//'
ansible --version | grep 'config file' | sed 's|=.*/|= .../|'
```

Expected output:

```text
DEFAULT_FORKS = 20
DEFAULT_TIMEOUT = 15
HOST_KEY_CHECKING = False
  config file = .../ansible.cfg
```

Breakdown: Ansible looks for its config in this order: the `ANSIBLE_CONFIG` variable, `ansible.cfg` in the current folder, `~/.ansible.cfg`, `/etc/ansible/ansible.cfg` — and uses the first one found rather than merging them. That's why the config lives in the repository next to the playbooks: the whole team and CI get the same behaviour. `--only-changed` shows the differences from defaults, and `ansible --version` shows which file is actually used — the first thing to check when "it works for me but not in CI". Disabling `host_key_checking` is reasonable in dynamic cloud environments, but it's a security trade-off.

</details>

## Ad-hoc commands

An **ad-hoc command** is a single task without a playbook, straight from the terminal. Handy for one-off actions and diagnostics.

```bash
ansible all -m ansible.builtin.ping                         # are the hosts reachable and do they have Python
ansible web -m ansible.builtin.command -a "uptime"          # run a command
ansible web -m ansible.builtin.shell -a "df -h | grep /var" # a pipe is needed — so shell
ansible web -b -m ansible.builtin.apt -a "name=htop state=present update_cache=true"   # -b = sudo
ansible db1 -m ansible.builtin.setup -a "filter=ansible_distribution*"                # facts about the system
ansible all -m ansible.builtin.reboot -b --limit 'web2'    # reboot and wait for it to come back
ansible all -f 50 -m ansible.builtin.command -a "systemctl is-active nginx"           # 50 hosts in parallel
```

The flags you need all the time:
| Flag | What it does |
|---|---|
| `-i` | the inventory |
| `-m` / `-a` | the module and its arguments |
| `-b` | become (via sudo by default) |
| `-K` | ask for the sudo password |
| `-u` | the SSH user |
| `--limit` | narrow down the list of hosts |
| `-f` | the number of parallel hosts |
| `-v`…`-vvvv` | output verbosity; `-vvvv` also shows SSH details |

**command versus shell.** `command` runs a program directly, without a shell: pipes `|`, redirections `>`, `&&` and `$VAR` substitution don't work. On the other hand there are no problems with escaping and injections. `shell` runs via `/bin/sh`, and all of that works. The rule: `command` by default, `shell` only when you really need shell features.

### 🧪 Mini task

**Task.** Use an ad-hoc `copy` module command to create a file with some text, then run the same command again. Compare the statuses of the two runs, then check the content with the `command` module.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

ansible all -i hosts -m copy -a "content='maintenance\n' dest=$PWD/motd" | head -1
ansible all -i hosts -m copy -a "content='maintenance\n' dest=$PWD/motd" | head -1
ansible all -i hosts -m command -a "cat $PWD/motd" | tail -1
```

Expected output:

```text
localhost | CHANGED => {
localhost | SUCCESS => {
maintenance
```

Breakdown: the first run created the file — `CHANGED`. The second compared the content (by checksum) with the desired state and did nothing — `SUCCESS`; that is idempotency. Ad-hoc commands are handy for one-off actions across many hosts: restart a service, check a package version, gather facts (`-m setup`). Anything that needs repeating and reviewing goes into a playbook.

</details>

## Your first playbook

A **playbook** is a YAML file with a list of **plays**. A play ties a group of hosts to a set of tasks. A **task** is a call to one module with arguments.

```yaml
# site.yml
- name: Configure the web servers         # a play
  hosts: web
  become: true                            # all tasks — via sudo

  tasks:
    - name: Install nginx                 # a task
      ansible.builtin.apt:
        name: nginx
        state: present
        update_cache: true
        cache_valid_time: 3600            # don't update the apt cache if it's less than an hour old

    - name: Put the main page in place
      ansible.builtin.copy:
        content: "Hello from {{ inventory_hostname }}\n"
        dest: /var/www/html/index.html
        owner: www-data
        group: www-data
        mode: "0644"

    - name: nginx is running and enabled at boot
      ansible.builtin.service:
        name: nginx
        state: started
        enabled: true
```

```bash
ansible-playbook -i inventory.ini site.yml
# → PLAY RECAP
# → web1 : ok=4  changed=3  unreachable=0  failed=0  skipped=0
ansible-playbook -i inventory.ini site.yml        # the second run
# → web1 : ok=4  changed=0  ...                    — nothing changed: that's how it should be
```
The first "extra" `ok` task is fact gathering (`Gathering Facts`), which happens at the start of every play.

**What the statuses mean:**
- `ok` — the state was already as needed, nothing was done;
- `changed` — the module changed something;
- `failed` — an error, the host drops out of the play;
- `skipped` — skipped because of a `when` condition;
- `unreachable` — couldn't connect.

**Useful run flags:**
```bash
ansible-playbook site.yml --syntax-check          # a syntax check without connecting
ansible-playbook site.yml --check --diff          # a "dry run": what would change, and a diff of the files
ansible-playbook site.yml --limit web1            # only one host
ansible-playbook site.yml --tags nginx            # only tasks with a tag
ansible-playbook site.yml --list-tasks            # which tasks will be run
ansible-playbook site.yml --start-at-task "Put the main page in place"
ansible-playbook site.yml --step                  # ask for confirmation before every task
ansible-playbook site.yml -e "app_version=1.4.2"  # pass a variable (the highest precedence)
```

### 🧪 Mini task

**Task.** Write a two-task playbook: create an `app` directory and put a `config.ini` into it. Run it twice and compare the `PLAY RECAP` lines.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

cat > site.yml <<'EOF'
- name: Configure app
  hosts: all
  gather_facts: false
  tasks:
    - name: Create app directory
      ansible.builtin.file:
        path: "{{ playbook_dir }}/app"
        state: directory
        mode: "0755"

    - name: Write config
      ansible.builtin.copy:
        dest: "{{ playbook_dir }}/app/config.ini"
        content: "[server]\nport = 8080\n"
        mode: "0644"
EOF
ansible-playbook -i hosts site.yml | grep '^localhost' | awk '{print $3, $4, $7}'
ansible-playbook -i hosts site.yml | grep '^localhost' | awk '{print $3, $4, $7}'
cat app/config.ini
```

Expected output:

```text
ok=2 changed=2 failed=0
ok=2 changed=0 failed=0
[server]
port = 8080
```

Breakdown: a playbook is a list of plays (which hosts), each with a list of tasks run in order. The first run changed both things, the second nothing, because the directory and file were already in the desired state. That's the main quality bar for a playbook: a repeat run gives `changed=0`. If something is `changed` again on the second run, the task isn't idempotent and needs fixing. `gather_facts: false` skips collecting system facts — faster when they aren't needed. Fully qualified module names (`ansible.builtin.file`, FQCN) are the modern standard.

</details>

## Modules and idempotency

**Idempotency** is the property whereby a repeat run doesn't change the system if it's already in the desired state. This is Ansible's main idea: a playbook can be run any number of times, and it only "brings" the servers up to the described state. Interviews ask about this almost always.

Idempotency is provided by **modules**: `apt` first checks whether the package is installed; `copy` compares the file's checksum; `user` looks at whether the user exists. So always look for a ready-made module rather than writing `shell`.

**Why command/shell break idempotency.** Ansible doesn't know what an arbitrary command does, so it always considers it `changed` and runs it every time. How to fix it:
```yaml
- name: Initialise Swarm (only if not initialised yet)
  ansible.builtin.command: docker swarm init --advertise-addr {{ ansible_default_ipv4.address }}
  args:
    creates: /var/lib/docker/swarm/state.json     # if the file exists — the task is skipped

- name: Find out the application version
  ansible.builtin.command: /opt/app/bin/app --version
  register: app_version
  changed_when: false                              # reading changes nothing — don't report changed

- name: Apply DB migrations
  ansible.builtin.command: /opt/app/bin/migrate up
  register: migrate
  changed_when: "'no change' not in migrate.stdout"   # changed only if there really were migrations
```
Even better is a module that can already do this itself. For the example above: `community.docker.docker_swarm` with `state: present`.

**FQCN — the fully qualified module name.** Write `ansible.builtin.copy`, not just `copy`. Short names work, but with collections installed a module with the same name from another collection may be found. ansible-lint requires FQCN.

**Modules you need every day:**

| Task | Module | Example |
|---|---|---|
| Packages | `ansible.builtin.package` (generic), `apt`, `dnf` | `name: [curl, jq] state: present` |
| Services | `ansible.builtin.systemd` / `service` | `name: nginx state: restarted daemon_reload: true` |
| A whole file | `ansible.builtin.copy` | a static file or `content:` |
| A file from a template | `ansible.builtin.template` | a Jinja2 template with variables |
| Permissions, folders, links | `ansible.builtin.file` | `state: directory mode: "0750"` |
| A line in a file | `ansible.builtin.lineinfile` | change one line in someone else's config |
| A block in a file | `ansible.builtin.blockinfile` | insert a block between markers |
| Users | `ansible.builtin.user`, `ansible.posix.authorized_key` | a user and their SSH key |
| Download / unpack | `get_url`, `unarchive` | a binary with a checksum, an archive |
| An HTTP request | `ansible.builtin.uri` | a healthcheck, an API call |
| Waiting | `wait_for`, `wait_for_connection` | a port opened, a host came back after a reboot |
| Docker | `community.docker.docker_container`, `docker_swarm`, `docker_node` | containers and Swarm |
| Kubernetes | `kubernetes.core.k8s` | apply a manifest |

Documentation for any module is right in the terminal: `ansible-doc ansible.builtin.lineinfile` (the EXAMPLES section at the end is especially useful), the list of modules — `ansible-doc -l`.

**lineinfile versus template.** `lineinfile` edits one line and suits someone else's configs that you don't want to control entirely (for example, one option in `/etc/sysctl.conf`). If the file is yours, generate it entirely with `template`. A dozen `lineinfile` tasks on one file is a sign it's time to move to a template: their order and interaction quickly become unpredictable.

### 🧪 Mini task

**Task.** Add the line `timeout=30` to a config two ways — via `shell` with `>>` and via the `lineinfile` module — running each three times. Compare the resulting files.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

echo "port=80" > shell.conf; echo "port=80" > module.conf
for i in 1 2 3; do
  ansible all -i hosts -m shell -a "echo timeout=30 >> $PWD/shell.conf" > /dev/null
  ansible all -i hosts -m lineinfile -a "path=$PWD/module.conf line=timeout=30" > /dev/null
done
echo "shell:";  cat shell.conf
echo "lineinfile:"; cat module.conf
```

Expected output:

```text
shell:
port=80
timeout=30
timeout=30
timeout=30
lineinfile:
port=80
timeout=30
```

Breakdown: `shell` just runs the command — every run appends another line and the config gets corrupted. `lineinfile` describes the desired state ("the file contains this line") and checks it first. Hence the rule: `shell` and `command` are the last resort. If you can't avoid them, add `creates`/`removes` or `changed_when` so the task reports changes honestly and doesn't repeat the action. For whole config files `template` is better than a pile of `lineinfile`s.

</details>

## Variables

Variables make one playbook suitable for different environments and hosts: the application version, ports, the list of users.

**Where they're defined** (the main places):
- in the inventory and in `group_vars/`, `host_vars/`;
- in the playbook: `vars:`, `vars_files:`;
- in a role: `defaults/main.yml` (default values) and `vars/main.yml`;
- in a task: `vars:` at the task or block level;
- dynamically: `register` (a task's result), `set_fact`;
- on the command line: `-e` / `--extra-vars`.

**Precedence** — what wins if one variable is set in several places. The full list has 22 levels, you don't need to memorise all of it. It's enough to understand the order from weakest to strongest:
1. a role's `defaults/` — the weakest: "a default value, override me";
2. the inventory, then `group_vars/all`, then `group_vars/<group>`, then `host_vars/<host>` — **a host beats a group, a specific group beats all**;
3. host facts;
4. play variables (`vars`, `vars_files`);
5. a role's `vars/`;
6. block and task variables;
7. `set_fact` and `register`;
8. `-e` on the command line — **the strongest**, nothing can override it.

Practical conclusions:
- what a role's user should be able to change goes in `defaults/`, not in the role's `vars/` — otherwise their `group_vars` won't take effect;
- `-e` is handy in CI (`-e app_version=$CI_COMMIT_TAG`), but you shouldn't overuse it: the value silently overrides everything else.

If two groups of the same level set the same variable (the host belongs to both), the group that comes later alphabetically wins. This can be controlled with `ansible_group_priority`. It's better not to allow such overlaps at all.

**Facts** are variables that Ansible collects about a host at the start of a play (the `setup` module): OS, IP, memory, disks.
```yaml
- name: Show a couple of facts
  ansible.builtin.debug:
    msg: "{{ ansible_facts['distribution'] }} {{ ansible_facts['distribution_version'] }}, IP {{ ansible_facts['default_ipv4']['address'] }}"
```
If facts aren't needed, `gather_facts: false` in the play saves seconds on every host.

**register** saves a task's result:
```yaml
- name: Check whether there is swap
  ansible.builtin.command: swapon --show --noheadings
  register: swap
  changed_when: false

- name: Disable swap (needed for Kubernetes)
  ansible.builtin.command: swapoff -a
  when: swap.stdout | length > 0
```
A registered variable has `stdout`, `stderr`, `rc`, `changed`, `failed`, and modules add their own fields. To see the structure: `- ansible.builtin.debug: var=swap`.

**Magic variables** — Ansible sets them itself:
- `inventory_hostname` — the host's name from the inventory;
- `groups` — a "group → list of hosts" dictionary: `groups['swarm_managers'][0]`;
- `hostvars` — the variables and facts of other hosts: `hostvars['db1']['ansible_host']`;
- `play_hosts` / `ansible_play_hosts` — the current play's hosts.

An example: the Swarm workers need a token that only the first manager knows:
```yaml
- name: Join the workers to Swarm
  community.docker.docker_swarm:
    state: join
    join_token: "{{ hostvars[groups['swarm_managers'][0]]['swarm_info']['swarm_facts']['JoinTokens']['Worker'] }}"
    remote_addrs: ["{{ hostvars[groups['swarm_managers'][0]]['ansible_host'] }}:2377"]
```
Here `swarm_info` is the `register` result of a task run earlier on the first manager. Important: a variable from `hostvars` appears only if that task has already run on that host in the same run.

### 🧪 Mini task

**Task.** Set the `release` variable in three places: `group_vars/all.yml`, the playbook's `vars` and `-e` on the command line. Run the playbook without and with `-e` and see which value wins.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

mkdir group_vars && echo 'release: from group_vars' > group_vars/all.yml
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars:
    release: from play vars
  tasks:
    - ansible.builtin.debug:
        msg: "{{ release }}"
EOF
ansible-playbook -i hosts site.yml | grep '"msg"'
ansible-playbook -i hosts site.yml -e release=from_extra_vars | grep '"msg"'
```

Expected output:

```text
    "msg": "from play vars"
    "msg": "from_extra_vars"
```

Breakdown: Ansible has over 20 variable precedence levels. The essentials: a role's `defaults` is the weakest level (defaults meant to be overridden), then the inventory and `group_vars`/`host_vars`, then the playbook's and role's `vars`, and `-e` (extra vars) beats everything. So `-e` is for one-off run parameters (a version, a flag), not permanent configuration. When the same variable is set in five places debugging becomes a quest — keep each variable in one obvious place.

</details>

## Jinja2 and templates

Ansible substitutes values via **Jinja2**: `{{ variable }}`, filters `{{ x | filter }}`, conditions and loops `{% ... %}` in templates.

**Common filters:**
```yaml
port: "{{ app_port | default(8080) }}"                   # a value if the variable isn't set
name: "{{ service_name | mandatory }}"                   # fail with a clear error if it isn't set
hosts: "{{ groups['web'] | join(',') }}"                 # a list → a string
upper: "{{ env | upper }}"
is_prod: "{{ (env == 'prod') | ternary('yes', 'no') }}"
cfg: "{{ app_config | to_nice_yaml }}"                   # a dictionary → YAML text
ips: "{{ groups['web'] | map('extract', hostvars, 'ansible_host') | list }}"   # the list of IPs of all web hosts
```

**The template module** renders a file on the control node and puts the result on the host. Templates live in `templates/` and have the `.j2` extension:
```jinja
{# templates/haproxy.cfg.j2 — a Jinja comment, it won't get into the resulting file #}
# Managed by Ansible. Manual edits will be overwritten.
global
    maxconn {{ haproxy_maxconn | default(4000) }}

frontend http
    bind *:80
    default_backend app

backend app
    balance roundrobin
{% for host in groups['app'] %}
    server {{ host }} {{ hostvars[host]['ansible_host'] }}:{{ app_port }} check
{% endfor %}
```
```yaml
- name: HAProxy config
  ansible.builtin.template:
    src: haproxy.cfg.j2
    dest: /etc/haproxy/haproxy.cfg
    mode: "0644"
    validate: haproxy -c -f %s          # check the file BEFORE it replaces the working one
  notify: Reload haproxy
```
`validate` is a very useful option: Ansible puts the new file in a temporary location, runs the check (`%s` is the path to the temporary file), and only on success puts it in place. That way a broken config won't reach the server. The classics: `visudo -cf %s` for sudoers, `sshd -t -f %s` for sshd.

Always write the "Managed by Ansible" line at the top of the file: colleagues will understand that editing by hand is pointless.

**Lookups** read data on the control node: `{{ lookup('file', 'files/id_rsa.pub') }}`, `{{ lookup('env', 'HOME') }}`, `{{ lookup('pipe', 'git rev-parse --short HEAD') }}`.

### 🧪 Mini task

**Task.** Write a Jinja2 template for an nginx `upstream` block that builds `server` lines from a list of backends (one has no port — default it to 80) and writes a comment with the server count. Render it with the `template` module.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

cat > upstream.conf.j2 <<'EOF'
# {{ backends | length }} servers, managed by Ansible
upstream {{ app_name | lower }} {
{% for b in backends %}
    server {{ b.host }}:{{ b.port | default(80) }};
{% endfor %}
}
EOF
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars:
    app_name: Shop
    backends:
      - {host: 10.0.1.11, port: 8080}
      - {host: 10.0.1.12, port: 8080}
      - {host: 10.0.1.13}
  tasks:
    - ansible.builtin.template:
        src: upstream.conf.j2
        dest: "{{ playbook_dir }}/upstream.conf"
EOF
ansible-playbook -i hosts site.yml > /dev/null
cat upstream.conf
```

Expected output:

```text
# 3 servers, managed by Ansible
upstream shop {
    server 10.0.1.11:8080;
    server 10.0.1.12:8080;
    server 10.0.1.13:80;
}
```

Breakdown: the template is rendered on the control machine and the host receives a finished file. `{{ }}` substitutes values, `{% %}` holds control structures, `|` applies filters (`length`, `lower`, `default`). Ansible enables `trim_blocks` by default, so the `{% for %}` lines leave no blank lines. The `default` filter saves you from an error when an item lacks a key. The "managed by Ansible" comment warns that manual edits will be overwritten on the next run. To check the generated config, `template` has a `validate: nginx -t -c %s` parameter.

</details>

## Conditions, loops and error handling

**when** — run a task only under a condition. It's written without `{{ }}`:
```yaml
- name: Packages for the Debian family
  ansible.builtin.apt:
    name: chrony
  when: ansible_facts['os_family'] == "Debian"

- name: Packages for the RHEL family
  ansible.builtin.dnf:
    name: chrony
  when:
    - ansible_facts['os_family'] == "RedHat"
    - ansible_facts['distribution_major_version'] | int >= 8   # a list of conditions = logical AND
```

**loop** — repeat a task for each element:
```yaml
- name: The team's users
  ansible.builtin.user:
    name: "{{ item.name }}"
    groups: "{{ item.groups }}"
    shell: /bin/bash
  loop:
    - { name: alex, groups: "sudo,docker" }
    - { name: ci,   groups: "docker" }
  loop_control:
    label: "{{ item.name }}"      # show only the name in the output, not the whole dictionary
```
Packages don't need a loop: `apt` and `dnf` accept a list in `name:` and install everything in one operation — that's much faster.

**until** — repeat until a condition holds (waiting):
```yaml
- name: Wait until the application responds with 200
  ansible.builtin.uri:
    url: "http://{{ ansible_host }}:{{ app_port }}/health"
    status_code: 200
  register: health
  until: health.status == 200
  retries: 30
  delay: 5                          # up to 30 attempts every 5 seconds
```

**Error control:**
- `failed_when` — your own rule for what counts as an error: `failed_when: "'ERROR' in result.stdout"`;
- `ignore_errors: true` — continue even if the task failed (use it rarely: errors get hidden);
- `any_errors_fatal: true` in a play — a task failed on one host → stop the play on all of them (needed when a partial rollout is dangerous for the cluster);
- `max_fail_percentage` — the acceptable share of failed hosts in a batch.

**block / rescue / always** — the equivalent of try / except / finally:
```yaml
- name: Update with a rollback
  block:
    - name: Roll out the new version
      ansible.builtin.copy:
        src: "app-{{ app_version }}.jar"
        dest: /opt/app/app.jar
      notify: Restart app

    - name: Apply the restart now to check health
      ansible.builtin.meta: flush_handlers

    - name: Check health
      ansible.builtin.uri:
        url: "http://localhost:{{ app_port }}/health"
      register: health
      until: health.status == 200
      retries: 12
      delay: 5

  rescue:
    - name: Restore the previous version
      ansible.builtin.copy:
        src: "app-{{ app_previous_version }}.jar"
        dest: /opt/app/app.jar
      notify: Restart app

    - name: Mark the host as failed after the rollback
      ansible.builtin.fail:
        msg: "Version {{ app_version }} didn't come up, rolled back to {{ app_previous_version }}"

  always:
    - name: Record the result
      ansible.builtin.lineinfile:
        path: /var/log/deploy.log
        line: "{{ now(utc=true, fmt='%Y-%m-%dT%H:%M:%SZ') }} {{ app_version }} {{ 'ok' if (health is defined and health is succeeded) else 'rollback' }}"
        create: true
```
The tasks in `rescue` run only if something in `block` failed. `always` — in any case.

### 🧪 Mini task

**Task.** Loop over a list of services and print only the enabled ones (`when`). Then, in a `block`, run a command that is bound to fail, handle the error in `rescue`, and in `always` print a message that must appear in any case.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars:
    services:
      - {name: api, enabled: true}
      - {name: cron, enabled: false}
      - {name: web, enabled: true}
  tasks:
    - ansible.builtin.debug:
        msg: "deploy {{ item.name }}"
      loop: "{{ services }}"
      when: item.enabled

    - block:
        - ansible.builtin.command: /bin/false
      rescue:
        - ansible.builtin.debug:
            msg: "deploy failed, rolling back"
      always:
        - ansible.builtin.debug:
            msg: "notify chat"
EOF
ansible-playbook -i hosts site.yml | grep -E '"msg"|skipping'
ansible-playbook -i hosts site.yml | grep '^localhost' | awk '{print $3, $4, $7}'
```

Expected output:

```text
skipping: [localhost] => (item={'name': 'cron', 'enabled': False})
    "msg": "deploy api"
    "msg": "deploy web"
    "msg": "deploy failed, rolling back"
    "msg": "notify chat"
ok=4 changed=0 failed=0
```

Breakdown: `when` is evaluated for each `loop` item separately, and `cron` is skipped. `block/rescue/always` is the try/except/finally equivalent: the command failed, but `rescue` caught the error and ran the rollback, and `always` ran regardless. Hence `failed=0` in the end: the error was handled and the play continues. Without `rescue` the host would drop out of the remaining tasks. Other tools: `failed_when` (your own failure condition, e.g. on output text), `ignore_errors` (use sparingly) and `any_errors_fatal` (stop everything on the first error).

</details>

## Handlers

A **handler** is a task that runs only on notification (`notify`) and only if the notifying task returned `changed`. The classic example: restart nginx only if its config changed.

```yaml
- name: Web servers
  hosts: web
  become: true

  tasks:
    - name: nginx config
      ansible.builtin.template:
        src: nginx.conf.j2
        dest: /etc/nginx/nginx.conf
        validate: nginx -t -c %s
      notify: Reload nginx

    - name: Site config
      ansible.builtin.template:
        src: site.conf.j2
        dest: /etc/nginx/conf.d/site.conf
      notify: Reload nginx

  handlers:
    - name: Reload nginx
      ansible.builtin.systemd:
        name: nginx
        state: reloaded
```

How handlers work:
- there can be many notifications, but a handler runs **once**;
- it runs not immediately but **at the end of a play section** (after `pre_tasks`, after `roles` + `tasks`, after `post_tasks`);
- handlers run **in the order they're defined in `handlers:`**, not in the order of notifications;
- if the play failed before that point, the handler **won't run**: the config is already new, but the service hasn't been restarted. The protection is `--force-handlers` or `force_handlers: true` in the play;
- run the accumulated handlers right now — `- ansible.builtin.meta: flush_handlers`;
- `listen:` lets you attach several handlers to one event name: `notify: restart web stack` triggers all handlers with `listen: restart web stack`.

**reload versus restart.** `reloaded` rereads the config without dropping connections (nginx and haproxy can do this), `restarted` restarts the process. Web servers almost always need reload.

### 🧪 Mini task

**Task.** Make a playbook where the config-writing task notifies a "restart app" handler. Run it three times: with a new config, with no changes, and with a changed value. Count how many times the handler fired.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars:
    port: 8080
  tasks:
    - ansible.builtin.copy:
        dest: "{{ playbook_dir }}/app.conf"
        content: "port={{ port }}\n"
      notify: restart app
  handlers:
    - name: restart app
      ansible.builtin.debug:
        msg: "app restarted"
EOF
for extra in "" "" "-e port=9090"; do
  ansible-playbook -i hosts site.yml $(echo $extra) | grep -c 'RUNNING HANDLER'
done
```

Expected output:

```text
1
0
1
```

Breakdown: a handler runs only if the notifying task returned `changed`, and only once at the end of the play, even if ten tasks notified it. So the service restarts only when the config really changed, not on every run. If the play fails before the end, pending handlers won't run; for that there's `--force-handlers` or `meta: flush_handlers` mid-playbook. `$(echo $extra)` splits the string into arguments the same way in bash and zsh.

</details>

## Roles

When a playbook grows, it's split into **roles** — reusable blocks with a standard structure. An `nginx` role can be plugged into different projects, a `docker` role — onto all servers.

```bash
ansible-galaxy role init --init-path roles docker      # create a role skeleton in roles/docker
```
```text
roles/docker/
├── defaults/main.yml     # default variables (weak precedence) — the "role's settings"
├── vars/main.yml         # the role's internal variables (strong precedence), not for overriding
├── tasks/main.yml        # tasks — the entry point
├── handlers/main.yml
├── templates/            # *.j2 for the template module
├── files/                # static files for copy
├── meta/main.yml         # dependencies on other roles, metadata
└── README.md             # which variables the role accepts
```
The `template` and `copy` modules inside a role look for files in that role's `templates/` and `files/` — you don't need to write the path.

**An example `docker` role:**
```yaml
# roles/docker/defaults/main.yml
docker_version: "5:27.3.1-1~ubuntu.24.04~noble"
docker_users: []
docker_daemon_options:
  log-driver: json-file
  log-opts:
    max-size: "50m"
    max-file: "3"
```
```yaml
# roles/docker/tasks/main.yml
- name: Docker repository (deb822 format, the module downloads the key itself)
  ansible.builtin.deb822_repository:
    name: docker
    types: [deb]
    uris: https://download.docker.com/linux/ubuntu
    suites: ["{{ ansible_facts['distribution_release'] }}"]
    components: [stable]
    signed_by: https://download.docker.com/linux/ubuntu/gpg

- name: Docker packages of a pinned version
  ansible.builtin.apt:
    name:
      - "docker-ce={{ docker_version }}"
      - "docker-ce-cli={{ docker_version }}"
      - containerd.io
    update_cache: true

- name: daemon.json
  ansible.builtin.copy:
    content: "{{ docker_daemon_options | to_nice_json }}\n"
    dest: /etc/docker/daemon.json
    mode: "0644"
  notify: Restart docker

- name: Users in the docker group
  ansible.builtin.user:
    name: "{{ item }}"
    groups: docker
    append: true             # without append the user loses their other groups
  loop: "{{ docker_users }}"
```
```yaml
# roles/docker/handlers/main.yml
- name: Restart docker
  ansible.builtin.systemd:
    name: docker
    state: restarted
```
```yaml
# site.yml
- name: Docker hosts
  hosts: swarm
  become: true
  roles:
    - role: docker
      vars:
        docker_users: [deploy]
```

**The package version is pinned** for a reason: without it one server ends up with last month's Docker and a new one with today's, and the cluster becomes inconsistent.

**Role dependencies** — in `meta/main.yml`:
```yaml
dependencies:
  - role: common          # runs before this role
```
Be careful with dependencies: the same role runs once per play (if the parameters match), and the order becomes non-obvious. It's often simpler to list the roles explicitly in the playbook.

### 🧪 Mini task

**Task.** Create a `web` role skeleton with `ansible-galaxy role init`, set port 80 in its `defaults` and print it in a task. Use the role in a playbook twice: without parameters and with an overridden port.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

ansible-galaxy role init roles/web > /dev/null
ls roles/web
echo 'web_port: 80' > roles/web/defaults/main.yml
printf -- '- ansible.builtin.debug:\n    msg: "web listens on {{ web_port }}"\n' > roles/web/tasks/main.yml
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  roles:
    - role: web
    - role: web
      vars:
        web_port: 8443
EOF
ansible-playbook -i hosts site.yml | grep '"msg"'
```

Expected output:

```text
README.md
defaults
files
handlers
meta
tasks
templates
tests
vars
    "msg": "web listens on 80"
    "msg": "web listens on 8443"
```

Breakdown: a role is a reusable package with a standard layout: `tasks` (what to do), `handlers`, `templates` and `files`, `defaults` (low-precedence default parameters — the ones role users override), `vars` (internal high-precedence values), `meta` (dependencies). The same role was used twice with different parameters — that's how one codebase configures different services. A good role does one thing, puts every tunable value in `defaults` and documents them in the README.

</details>

## import versus include

Tasks, roles and playbooks can be pulled in two ways, and interviewers like to ask about the difference.

| | `import_tasks`, `import_role`, `import_playbook` | `include_tasks`, `include_role` |
|---|---|---|
| When it's processed | when the playbook is parsed, before running (**statically**) | at run time (**dynamically**) |
| `when` and `tags` on this line | apply **to every** included task | apply only to the include itself |
| Visible in `--list-tasks` | yes | no |
| Can be used in a `loop` | no | yes |
| File name from a variable | no (it must be known at parse time) | yes: `include_tasks: "{{ ansible_facts['os_family'] }}.yml"` |

The rule: `import_*` by default (predictable, tags and `--list-tasks` work). `include_*` — when you need a loop or to choose a file by a variable, for example tasks for different OSes:
```yaml
- name: Tasks for the specific OS
  ansible.builtin.include_tasks: "{{ ansible_facts['os_family'] | lower }}.yml"   # debian.yml or redhat.yml
```
If you need tags to reach the tasks inside `include_*`, use `apply: { tags: [...] }`.

### 🧪 Mini task

**Task.** Bring in one task file with `import_tasks` and another with `include_tasks`, and look at the `ansible-playbook --list-tasks` output: which tasks does Ansible know about before the playbook runs?

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

printf -- '- name: static task A\n  ansible.builtin.debug: {msg: a}\n- name: static task B\n  ansible.builtin.debug: {msg: b}\n' > static.yml
printf -- '- name: dynamic task C\n  ansible.builtin.debug: {msg: c}\n' > dynamic.yml
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  tasks:
    - ansible.builtin.import_tasks: static.yml
    - name: load dynamic tasks
      ansible.builtin.include_tasks: dynamic.yml
EOF
ansible-playbook -i hosts site.yml --list-tasks | grep -E 'task|load'
```

Expected output:

```text
      static task A	TAGS: []
      static task B	TAGS: []
      load dynamic tasks	TAGS: []
```

Breakdown: `import_*` is static: the file is inserted into the playbook at parse time, so its tasks show up in `--list-tasks`, work with `--tags` and `--start-at-task`, and a `when` on the import applies to every task inside. `include_*` is dynamic: the file is read only when execution reaches it, so the list shows just the include task itself. In return it can be used in a loop or with a file name from a variable (`include_tasks: "{{ ansible_os_family }}.yml"`). Rule of thumb: import by default, include when you need dynamics.

</details>

## Collections and Galaxy

A **collection** is a package of modules, plugins and roles from one author: `community.docker`, `amazon.aws`, `kubernetes.core`. Their modules are called by FQCN: `amazon.aws.ec2_instance`.

A project's dependencies are pinned in `requirements.yml`, like `requirements.txt` in Python:
```yaml
# collections/requirements.yml
collections:
  - name: community.docker
    version: ">=3.10.0,<4.0.0"
  - name: amazon.aws
    version: "8.2.1"
roles:
  - name: geerlingguy.node_exporter
    version: "2.1.0"
```
```bash
ansible-galaxy install -r collections/requirements.yml
```
Always pin the versions: a collection update can change a module's behaviour, and a playbook that worked yesterday fails in CI today.

### 🧪 Mini task

**Task.** Describe the project's dependencies in `requirements.yml` with pinned versions of the `community.general` and `amazon.aws` collections, install them into the project folder and check which versions were installed and where Ansible takes them from.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > requirements.yml <<'EOF'
collections:
  - name: community.general
    version: "11.4.0"
  - name: amazon.aws
    version: "10.1.2"
EOF
ansible-galaxy collection install -r requirements.yml -p ./collections > /dev/null
ANSIBLE_COLLECTIONS_PATH=./collections ansible-galaxy collection list -p ./collections 2>/dev/null | grep -E '^(community.general|amazon.aws) '
```

Expected output:

```text
amazon.aws        10.1.2
community.general 11.4.0
```

Breakdown: modules that aren't in `ansible-core` (AWS, Kubernetes, Docker, most utilities) live in collections and are called by full name: `amazon.aws.ec2_instance`. Without pinned versions you install different collections today and a month from now, and the playbook can break. So `requirements.yml` with versions is committed to the repository, and CI installs from it before running. Installing into the project folder (`-p ./collections` plus `collections_path` in `ansible.cfg`) isolates projects from each other, like a venv in Python. The versions here are examples — use current ones.

</details>

## Secrets: Ansible Vault

**Ansible Vault** encrypts files or individual values (AES-256 — the standard symmetric encryption algorithm; the key is derived from the vault password) so that passwords and keys can be stored in git next to the code.

```bash
ansible-vault create inventory/group_vars/db/vault.yml      # create an encrypted file
ansible-vault edit inventory/group_vars/db/vault.yml        # edit it
ansible-vault view inventory/group_vars/db/vault.yml
ansible-vault encrypt secrets.yml                           # encrypt an existing one
ansible-vault rekey inventory/group_vars/db/vault.yml       # change the password
ansible-vault encrypt_string 'S3cr3t!' --name 'db_password' # encrypt one value for pasting into YAML
```
Running:
```bash
ansible-playbook site.yml --ask-vault-pass
ansible-playbook site.yml --vault-password-file ~/.vault_pass        # a file with the password, not in git!
ansible-playbook site.yml --vault-id dev@prompt --vault-id prod@~/.vault_prod   # different passwords for environments
```

**The two-file technique**, so that secrets can be found with grep:
```yaml
# group_vars/db/main.yml — plain, you can see which variables exist
db_user: app
db_password: "{{ vault_db_password }}"
```
```yaml
# group_vars/db/vault.yml — encrypted
vault_db_password: "S3cr3t!"
```

**no_log** — don't print values to the output and logs (otherwise the password ends up in the CI log):
```yaml
- name: Create the DB user
  community.postgresql.postgresql_user:
    name: "{{ db_user }}"
    password: "{{ db_password }}"
  no_log: true
```

**External stores.** When secrets already live in HashiCorp Vault or AWS Secrets Manager, they're read with a lookup right at run time, and nothing is stored in the repository at all:
```yaml
db_password: "{{ lookup('community.hashi_vault.vault_kv2_get', 'apps/shop/db', engine_mount_point='secret').secret.password }}"
db_password_aws: "{{ lookup('amazon.aws.secretsmanager_secret', 'prod/shop/db') }}"
```

**Binary secrets** (Kerberos keytab files — files with a service account's keys that let it sign in to the corporate domain by itself, without a password; TLS keys) can also be encrypted entirely with vault and laid out with `copy` — it decrypts the file while copying:
```yaml
- name: The service keytab
  ansible.builtin.copy:
    src: files/app.keytab.vault       # encrypted: ansible-vault encrypt files/app.keytab.vault
    dest: /etc/app/app.keytab
    owner: app
    mode: "0400"
  no_log: true
```

### 🧪 Mini task

**Task.** Encrypt a file with the database password using Ansible Vault, look at the encrypted file and use the variable from it in a playbook, passing the vault password via a file.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

echo 'correct-horse-battery' > .vault_pass
echo 'db_password: hunter2' > secrets.yml
ansible-vault encrypt secrets.yml --vault-password-file .vault_pass
head -1 secrets.yml
grep -c hunter2 secrets.yml
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  vars_files: [secrets.yml]
  tasks:
    - ansible.builtin.debug:
        msg: "password length is {{ db_password | length }}"
EOF
ansible-playbook -i hosts site.yml --vault-password-file .vault_pass | grep '"msg"'
```

Expected output:

```text
Encryption successful
$ANSIBLE_VAULT;1.1;AES256
0
    "msg": "password length is 7"
```

Breakdown: the file is encrypted as a whole (AES256), the header shows the format, and there's no plain-text password — such a file can be committed. Decryption happens in memory at runtime. In CI the vault password comes from an environment variable or a script passed to `--vault-password-file`, and `.vault_pass` itself goes into `.gitignore`. `no_log: true` on tasks keeps secrets out of the output. The downside of vault: diffs don't show what changed. That's why secrets increasingly live in Secrets Manager or HashiCorp Vault and are read with a lookup plugin at runtime.

</details>

## Zero-downtime rolling update

If you update all the servers at once, the service goes down. A **rolling update** means updating hosts in small batches, taking each out of load balancing. Ansible has everything needed for this.

**serial** — how many hosts to process at a time. The play runs fully on one batch, then on the next:
```yaml
serial: 1                 # one at a time
serial: "25%"             # a quarter at a time
serial: [1, 5, "30%"]     # first one (the canary), then 5, then 30% at a time
```

A full example: updating an application behind HAProxy:
```yaml
- name: Rolling update of the application
  hosts: app
  become: true
  serial: [1, "30%"]
  max_fail_percentage: 0          # any failure — stop, don't touch the next batches

  pre_tasks:
    - name: Take the host out of load balancing
      community.general.haproxy:
        state: disabled
        host: "{{ inventory_hostname }}"
        backend: app
        socket: /run/haproxy/admin.sock
        wait: true                # wait until active connections finish
      delegate_to: "{{ item }}"
      loop: "{{ groups['lb'] }}"

  roles:
    - role: app                   # rolling out the new version, restart via a handler

  post_tasks:
    - name: Apply handlers before the check
      ansible.builtin.meta: flush_handlers

    - name: Wait for health
      ansible.builtin.uri:
        url: "http://{{ ansible_host }}:{{ app_port }}/health"
      register: health
      until: health.status == 200
      retries: 30
      delay: 5
      delegate_to: localhost
      become: false

    - name: Return the host to load balancing
      community.general.haproxy:
        state: enabled
        host: "{{ inventory_hostname }}"
        backend: app
        socket: /run/haproxy/admin.sock
      delegate_to: "{{ item }}"
      loop: "{{ groups['lb'] }}"
```

The key tools from the example:
- **delegate_to** — run a task on another host but in the context of the current one: the command goes to the load balancer, while `inventory_hostname` is the server being updated;
- **run_once: true** — run a task once for the whole batch (for example, a DB migration before the rollout);
- **pre_tasks / post_tasks** — tasks before and after the roles; handlers are flushed between sections;
- **max_fail_percentage: 0** — if the canary failed, we don't touch the other batches.

**The same technique for Docker Swarm** — before maintaining a node (updating Docker, the kernel), take it out from under load:
```yaml
- name: Maintaining Swarm nodes one at a time
  hosts: swarm_workers
  become: true
  serial: 1

  tasks:
    - name: Drain — services move to other nodes
      community.docker.docker_node:
        hostname: "{{ ansible_hostname }}"
        availability: drain
      delegate_to: "{{ groups['swarm_managers'][0] }}"

    - name: Update packages
      ansible.builtin.apt:
        upgrade: dist
        update_cache: true

    - name: Is a reboot needed (Ubuntu creates this file after kernel and library updates)
      ansible.builtin.stat:
        path: /var/run/reboot-required
      register: reboot_flag

    - name: Reboot if required
      ansible.builtin.reboot:
        reboot_timeout: 600
      when: reboot_flag.stat.exists

    - name: Return the node to service
      community.docker.docker_node:
        hostname: "{{ ansible_hostname }}"
        availability: active
      delegate_to: "{{ groups['swarm_managers'][0] }}"
```
For Kubernetes the equivalent is `kubectl drain` / `uncordon` (the `kubernetes.core.k8s_drain` module), usually with `serial: 1` and a check that all pods are Ready again.

### 🧪 Mini task

**Task.** Simulate a rollout to five servers in batches of two with `serial: 2`. See in which order and in which groups the hosts go through the task.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
printf '[web]\n' > hosts
for i in 1 2 3 4 5; do echo "web$i ansible_connection=local ansible_python_interpreter=auto_silent" >> hosts; done
cat > site.yml <<'EOF'
- hosts: web
  gather_facts: false
  serial: 2
  max_fail_percentage: 0
  tasks:
    - name: deploy
      ansible.builtin.debug:
        msg: "deploying {{ inventory_hostname }}"
EOF
ansible-playbook -i hosts site.yml -f 1 | grep -E '^TASK \[deploy|^ok: \[' | sed -e 's/ \*.*//' -e 's/ =>.*//'
```

Expected output:

```text
TASK [deploy]
ok: [web1]
ok: [web2]
TASK [deploy]
ok: [web3]
ok: [web4]
TASK [deploy]
ok: [web5]
```

Breakdown: `serial: 2` runs the whole play on two hosts before moving on — three batches. In a real rollout a batch's tasks look like: take the hosts out of the load balancer (HAProxy via its socket, or a drain in Swarm), update, check the healthcheck and put them back (`delegate_to` the balancer host). The rest of the servers keep serving traffic all along. `max_fail_percentage: 0` stops the rollout on the very first error, so a broken version doesn't spread to every server. `serial` can also be a percentage or a list of growing batches: `[1, 5, "25%"]`.

</details>

## Speed: how to make Ansible faster on hundreds of hosts

Ansible is unhurried by default: 5 hosts in parallel, fact gathering in every play, several SSH operations per task. On 50+ clusters that's minutes and tens of minutes. What helps:

1. **forks** — raise the parallelism: `forks = 30` in `ansible.cfg` or `-f 30`. The limit is the control node's CPU and memory.
2. **pipelining = True** — the module is sent over an already open SSH connection without a separate file copy. It usually speeds things up several times over. It requires that sudoers has no `requiretty` (a sudo setting "work only from a real terminal" that breaks non-interactive runs).
3. **ControlPersist** — the SSH connection is reused between tasks, there's no need to establish a session anew each time.
4. **Facts:** `gather_facts: false` if they aren't needed; `gather_subset: [network]` if only part is needed; a fact cache between runs:
   ```ini
   [defaults]
   gathering = smart
   fact_caching = jsonfile
   fact_caching_connection = /tmp/ansible_facts
   fact_caching_timeout = 3600
   ```
5. **Packages as a list**, not in a `loop` — one apt transaction instead of ten.
6. **strategy: free** — each host goes through the tasks at its own pace, fast ones don't wait for slow ones. Not suitable when tasks on different hosts depend on each other.
7. **async** — a long task is started in the background, Ansible doesn't hold the connection:
   ```yaml
   - name: A long index rebuild
     ansible.builtin.command: /opt/app/bin/reindex
     async: 3600        # an hour at most
     poll: 0            # don't wait, move on immediately
     register: reindex_job

   - name: Wait for completion
     ansible.builtin.async_status:
       jid: "{{ reindex_job.ansible_job_id }}"
     register: job
     until: job.finished
     retries: 120
     delay: 30
   ```
8. **throttle** on a task — limit the parallelism for that task only (for example, no more than 2 hosts at once downloading a large file from an internal repository).

To find the slow tasks, enable the timing callback: `callbacks_enabled = ansible.posix.profile_tasks` in `[defaults]` — at the end it shows the top tasks by time.

### 🧪 Mini task

**Task.** Five "hosts" run a `sleep 1` task. Time the run with `-f 1` and with `-f 5` and explain the difference.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
for i in 1 2 3 4 5; do echo "h$i ansible_connection=local ansible_python_interpreter=auto_silent" >> hosts; done
printf -- '- hosts: all\n  gather_facts: false\n  tasks:\n    - ansible.builtin.command: sleep 1\n' > site.yml
for f in 1 5; do
  start=$(date +%s); ansible-playbook -i hosts site.yml -f $f > /dev/null; echo "forks=$f: $(( $(date +%s) - start ))s"
done
```

Expected output:

```text
forks=1: 7s
forks=5: 3s
```

Breakdown: `forks` is how many hosts Ansible handles in parallel (only 5 by default). With one fork the hosts go one by one, with five at the same time; the remaining seconds are Ansible and Python start-up. On hundreds of hosts the main speed-ups are: more forks (as much as the control machine can handle), `pipelining = True` and SSH ControlPersist (fewer SSH connections per task), `gather_facts: false` or a fact cache, `strategy: free` (hosts don't wait for each other between tasks) and `async` for long operations. Your numbers will differ.

</details>

## Testing and quality

A playbook is code, and it needs to be checked the same way.

**Levels of checking, from cheap to expensive:**
1. **yamllint** — YAML syntax and style.
2. **ansible-playbook --syntax-check** — Ansible understands the playbook.
3. **ansible-lint** — best practices: FQCN, task names, `command` instead of a module, missing `changed_when`, file modes without quotes:
   ```bash
   pip install ansible-lint
   ansible-lint site.yml roles/
   ```
4. **--check --diff** on real hosts — what will change. The limitation: the `command`/`shell` modules are skipped in check mode (Ansible can't predict their result), so tasks that depend on their `register` may behave differently. A task that only reads data can be allowed to run in check mode too: `check_mode: false`.
5. **Molecule** — role tests: it brings up a container or VM, applies the role, checks idempotency (the second run must give `changed=0`), runs checks and deletes the lab:
   ```bash
   pip install molecule molecule-plugins[docker]
   cd roles/docker
   molecule init scenario -d docker
   molecule test         # create → converge → idempotence → verify → destroy
   ```
   The `idempotence` step in Molecule is an automatic check of that very "a second run changes nothing".

### 🧪 Mini task

**Task.** Check a playbook without changing the system: first the syntax, then a `--check --diff` run that shows what would change in the config. Make sure the file itself stayed the same.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

echo "port=80" > app.conf
cat > site.yml <<'EOF'
- hosts: all
  gather_facts: false
  tasks:
    - ansible.builtin.lineinfile:
        path: "{{ playbook_dir }}/app.conf"
        line: timeout=30
EOF
ansible-playbook -i hosts site.yml --syntax-check
ansible-playbook -i hosts site.yml --check --diff | grep -E '^[-+@ ]' | grep -v '^ *$' | sed 's|: /.*/app.conf|: app.conf|'
cat app.conf
```

Expected output:

```text
playbook: site.yml
--- before: app.conf (content)
+++ after: app.conf (content)
@@ -1 +1,2 @@
 port=80
+timeout=30
port=80
```

Breakdown: `--syntax-check` catches YAML and playbook structure errors in a second — it's the first CI step. `--check` does a dry run: modules report what they *would* change but change nothing, and `--diff` shows the changes themselves. That's the standard review step before applying to prod: you see the future config diff on every host. Limitation: `command`/`shell` are skipped in check mode, and tasks that depend on their results may behave differently. The next levels are `ansible-lint` (style and errors) and Molecule (running a role in a container with checks).

</details>

## Ansible in CI/CD

Running playbooks from laptops in a team is bad: everyone has different versions of Ansible and collections, and there's no history of who rolled out what. So they're run from CI.

An example for GitLab CI:
```yaml
# .gitlab-ci.yml
stages: [lint, deploy]

variables:
  ANSIBLE_FORCE_COLOR: "true"
  ANSIBLE_HOST_KEY_CHECKING: "true"

.ansible:
  image: registry.example.com/tools/ansible:2.17      # your own image: pinned ansible-core, collections, lint
  before_script:
    - eval "$(ssh-agent -s)"
    - echo "$DEPLOY_SSH_KEY" | tr -d '\r' | ssh-add -          # the SSH key from a protected CI variable
    - mkdir -p ~/.ssh && echo "$SSH_KNOWN_HOSTS" > ~/.ssh/known_hosts
    - echo "$ANSIBLE_VAULT_PASSWORD" > .vault_pass && chmod 600 .vault_pass
    - ansible-galaxy install -r collections/requirements.yml

lint:
  extends: .ansible
  stage: lint
  script:
    - yamllint .
    - ansible-lint

deploy_staging:
  extends: .ansible
  stage: deploy
  script:
    - ansible-playbook -i inventory/staging.ini site.yml --vault-password-file .vault_pass --diff
  rules:
    - if: $CI_COMMIT_BRANCH == "main"

deploy_prod:
  extends: .ansible
  stage: deploy
  script:
    - ansible-playbook -i inventory/prod.ini site.yml --vault-password-file .vault_pass --diff -e "app_version=$CI_COMMIT_TAG"
  rules:
    - if: $CI_COMMIT_TAG
      when: manual                     # prod — by a button
  environment: production
```
Note: `known_hosts` is passed explicitly rather than disabling host key checking — otherwise CI will connect to any server that answers at that address.

**AWX / Ansible Automation Platform** — a web interface and API on top of Ansible: running playbooks by a button and on a schedule, access rights, credential storage, run history. AWX is the free upstream version (the open project the commercial product is built from), AAP is the commercial one from Red Hat.

**ansible-pull** flips the model: the host itself clones the repository and applies the playbook to itself (for example, via cron). Useful when the control node can't reach the hosts (they're behind NAT), or for auto-configuration on first boot.

**Execution Environments** — a container image with ansible-core, collections and Python dependencies. They're run via `ansible-navigator`. It solves the "it works for me but not in CI" problem.

### 🧪 Mini task

**Task.** Run a playbook with JSON output instead of text and use `jq` to extract the final per-host statistics — that's how CI can decide what to do next. Then look at the exit code when a task fails.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

printf -- '- hosts: all\n  gather_facts: false\n  tasks:\n    - ansible.builtin.copy: {dest: "{{ playbook_dir }}/x", content: "x"}\n' > ok.yml
printf -- '- hosts: all\n  gather_facts: false\n  tasks:\n    - ansible.builtin.command: /bin/false\n' > fail.yml
ANSIBLE_STDOUT_CALLBACK=json ansible-playbook -i hosts ok.yml | jq -c '.stats.localhost | {ok, changed, failures}'
ansible-playbook -i hosts fail.yml > /dev/null; echo "exit=$?"
```

Expected output:

```text
{"ok":1,"changed":1,"failures":0}
exit=2
```

Breakdown: CI decides by exit code: 0 — success, 2 — a task failed on some host, 4 — hosts were unreachable. So a job with a failed playbook turns red by itself. JSON output helps when you need more than "failed or not": how many changes, on which hosts, publishing a report as an artifact or to chat. A typical pipeline: lint and syntax-check → `--check --diff` with the output for review → manual approval → apply. Credentials (an SSH key, the vault password, an AWS role via OIDC) come from protected CI variables, not from the repository.

</details>

## Ansible together with Terraform

A typical combination in the cloud:
1. Terraform creates the VPC, EC2, security groups and sets tags (`Role=web`, `Project=shop`).
2. The `amazon.aws.aws_ec2` dynamic inventory finds the instances by tags.
3. Ansible configures them.

What to avoid: calling Ansible from Terraform via `local-exec` — Terraform doesn't see what Ansible did, errors and retries become unpredictable. Two separate steps in the pipeline are better.

Where the boundary is: if a service can be configured via the provider's API (RDS parameters, S3 settings), Terraform does it. Everything inside the OS is Ansible. And for Kubernetes clusters the in-cluster configuration is increasingly managed via GitOps (ArgoCD, Flux), while Ansible remains for preparing nodes and on-prem infrastructure.

**Kubespray** is a set of Ansible playbooks and roles for installing a production Kubernetes cluster on your own servers (etcd, control plane, CNI, version upgrades). Useful to know if on-prem Kubernetes comes up.

### 🧪 Mini task

**Task.** Simulate the pairing: Terraform returns the IPs of created servers via an output, and a script turns it into an Ansible inventory. Without a cloud, use `terraform_data` with a list of addresses and check the resulting inventory.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > main.tf <<'EOF'
resource "terraform_data" "web" {
  count = 2
  input = "10.0.1.${count.index + 11}"
}

output "web_ips" {
  value = terraform_data.web[*].output
}
EOF
terraform init -no-color > /dev/null && terraform apply -auto-approve -no-color > /dev/null
{ echo "[web]"; terraform output -json web_ips | jq -r 'to_entries[] | "web\(.key + 1) ansible_host=\(.value)"'; } > hosts
cat hosts
ansible-inventory -i hosts --host web2
```

Expected output:

```text
[web]
web1 ansible_host=10.0.1.11
web2 ansible_host=10.0.1.12
{
    "ansible_host": "10.0.1.12"
}
```

Breakdown: a split of responsibilities: Terraform creates infrastructure and keeps its state, Ansible configures what's inside the machines. The bridge between them is data about the created resources. Generating an inventory from `terraform output -json` is the simplest option. On AWS a dynamic `amazon.aws.aws_ec2` inventory is more common: Terraform sets tags and Ansible finds the hosts by tags at run time, with nothing to generate. Running Ansible from Terraform via `local-exec` is possible, but it loses the idempotency and transparency of both tools. For machine images the pairing is different: Packer with an Ansible provisioner.

</details>

## Cases from experience: how to tell them

In an Ansible interview you're almost always asked to talk about real experience. Below are frameworks for the topics from the resume. Put in your own numbers and details, no need to invent anything: the interviewer usually digs deep.

**50+ Docker Swarm clusters via Ansible.** What to talk about:
- how the inventory was organised: groups by cluster and role (managers, workers, lb), where the variables lived;
- which roles there were: base OS setup, Docker, HAProxy, Pacemaker/Corosync (a cluster manager: it moves the floating IP and services to a live node on failure), GlusterFS (a distributed file system: a shared volume across several servers), monitoring;
- how Docker or the OS were updated without downtime: `serial`, draining nodes, checking the managers' quorum;
- how the run across dozens of clusters was sped up (forks, pipelining, restricting `--limit` by cluster);
- what broke and how it was fixed (configuration drift, manual edits, different package versions).

**On-prem Kubernetes via Ansible.** What was used (Kubespray, kubeadm in your own roles), how versions were upgraded, how nodes were prepared (swap, kernel modules, sysctl, container runtime).

**Secrets: Vault and Kerberos keytab files.** How secrets got onto the hosts: Ansible Vault, a lookup from HashiCorp Vault, `no_log`, `0400` permissions, rotation.

The answer format (STAR): **situation** (what was there and what the problem was) → **task** → **what you personally did** → **the result in numbers** (rollout time before/after, the number of manual operations, incidents).

## Typical mistakes

- **shell/command where a module exists** — idempotency is lost, the task is "changed" on every run.
- **No `changed_when` on read-only commands** — the report is full of false `changed`, real changes aren't visible in it.
- **`mode: 644` without quotes** — YAML reads it as the decimal number 644 rather than octal, and the permissions come out strange. Write `mode: "0644"`.
- **A forgotten `append: true` in the user module** — the user loses all their other groups.
- **Role variables in `vars/` instead of `defaults/`** — they can't be overridden from `group_vars`.
- **Jinja in `when` in double curly braces** — `when: "{{ x }}"` isn't needed, write `when: x`.
- **A variable starting with `{{` without quotes** — YAML treats `name: {{ app }}` as a dictionary. It must be `name: "{{ app }}"`.
- **A handler didn't run because the play failed earlier** — a new config, an old service. `force_handlers` or `flush_handlers` in the right places.
- **`host_key_checking = False` in prod** — convenient, but opens the door to a spoofed server.
- **Unpinned versions of packages and collections** — servers and CI runs drift apart over time.
- **Secrets in the output** — passwords end up in the CI log without `no_log: true`.
- **Manual edits on servers "while nobody's looking"** — the next Ansible run overwrites them, or worse, they aren't described in code and are lost when the server is recreated.

### 🧪 Mini task

**Task.** Try counting lines through a pipe with the `command` module, then with the same text through the `shell` module. Compare the results and explain why.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo "localhost ansible_connection=local ansible_python_interpreter=auto_silent" > hosts

ansible all -i hosts -m command -a 'echo a | wc -l' | tail -1
ansible all -i hosts -m shell -a 'echo a | wc -l' | tail -1 | tr -d ' '
```

Expected output:

```text
a | wc -l
1
```

Breakdown: the `command` module runs the program directly, without a shell, so `|`, `>`, `&&`, `*` and environment variables don't work: `echo` simply received the arguments `a`, `|`, `wc`, `-l` and printed them. Only `shell` ran the pipe. That's safer (no injection through substitution), which is why `command` is the default choice. Other common mistakes: `shell` where a module exists (not idempotent), secrets without `no_log`, one variable set in five places with different precedence, no `--check` before prod, and handlers that never fired because the play failed.

</details>

## Interview questions

**What is Ansible and how does it differ from Puppet and Chef?**
An agentless configuration management tool: it connects to hosts over SSH, copies and runs Python modules, works on a push model. Puppet and Chef require an agent on every server and a central server, and the agents periodically pull the configuration themselves (pull). Ansible has a lower entry barrier and an easier start, Puppet has better built-in drift control thanks to the agent running constantly.

**What is idempotency and how do you ensure it?**
A repeat run doesn't change the system if it's already in the desired state. Modules provide it: they check the current state and change only the difference. For `command`/`shell` — `creates`/`removes`, `changed_when`, a check via `register` + `when`, or better, replacing it with a suitable module. The check — a second run must give `changed=0`, in Molecule this is a separate step.

**How does command differ from shell?**
`command` runs a program directly without a shell: no pipes, redirections or environment variables in the string, but it's safer and has no escaping problems. `shell` executes the string via `/bin/sh` with all the shell's features. `command` is used by default.

**What does a managed host need for Ansible to work?**
SSH access (for Linux) and Python. Without Python only the `raw` and `script` modules work — `raw` is usually how Python gets installed on "bare" hosts. For Windows — WinRM or SSH and PowerShell.

**How does variable precedence work? What wins: group_vars or host_vars? Role defaults or group_vars?**
From weak to strong: role defaults → inventory and group_vars (all is weaker than a specific group) → host_vars → facts → play variables → role vars → block and task variables → set_fact and register → `-e`. host_vars beats group_vars. group_vars beats role defaults, which is why role settings go in defaults. `-e` overrides everything.

**What are handlers and when do they run?**
Tasks that run on `notify` and only if the notifying task gave `changed`. They run once, at the end of a play section, in the order defined in `handlers`. If the play failed earlier, they won't run; the protection is `force_handlers`. To run them immediately — `meta: flush_handlers`.

**How does import_tasks differ from include_tasks?**
`import_*` is processed statically at parse time: `when` and `tags` apply to every task inside, the tasks are visible in `--list-tasks`, but you can't use `loop` or a file name from a run-time variable. `include_*` is dynamic: it works in a loop and with a computed name, but tags and conditions apply only to the include itself.

**How do you store secrets in Ansible?**
Ansible Vault for files and individual values in the repository, with the vault password in a protected CI variable or a file outside git. The `db_password: "{{ vault_db_password }}"` technique for readability. `no_log: true` on tasks with secrets. For mature teams — a lookup from HashiCorp Vault or AWS Secrets Manager, then there are no secrets in the repository at all.

**How do you do a zero-downtime rolling update?**
`serial` to process hosts in batches (often `[1, "30%"]`: a canary first), `max_fail_percentage: 0` to stop on a failure. In `pre_tasks` take the host out of the load balancer via `delegate_to` to the LB, update, `flush_handlers`, wait for the health check via `until`, and in `post_tasks` return it to load balancing.

**What do delegate_to and run_once do?**
`delegate_to` runs a task on another host but in the context of the current one (the current host's variables are available): for example, take a server out of HAProxy by running the command on the load balancer. `run_once` runs a task once for the whole batch of hosts — for a DB migration, creating a token, an API request.

**A playbook on 200 hosts takes 40 minutes. How do you speed it up?**
Find the slow tasks (`profile_tasks`). Then: raise `forks`, enable `pipelining` and `ControlPersist`, disable or cache fact gathering, install packages as a list, `strategy: free` where tasks are independent, `async` for long operations, don't do unnecessary work (`creates`, conditions). If some hosts change rarely — limit the run with `--limit` and tags.

**How does check mode work and what are its limitations?**
`--check` runs modules in "what would change" mode without changes, `--diff` shows the file differences. The `command` and `shell` modules are skipped in check mode, so tasks that depend on their results may behave differently than in a real run. Modules without check mode support are skipped too. A read-only task can be run in check mode as well via `check_mode: false`.

**How do you test roles?**
yamllint and ansible-lint in CI on every commit, `--syntax-check`, Molecule: it brings up a container or VM, applies the role, applies it again and checks there are no changes (idempotency), runs the checks (verify) and deletes the lab.

**What is a dynamic inventory?**
A plugin that gets the list of hosts from an external source on every run: from AWS (`amazon.aws.aws_ec2`), GCP, Azure, VMware, NetBox (an infrastructure inventory system: servers, racks, IP addresses). Groups are built from tags and attributes (`keyed_groups`). It's needed in the cloud, where servers are constantly created and deleted.

**How do Ansible and Terraform work together?**
Terraform creates the infrastructure and keeps state, Ansible configures the OS and applications on the created machines. They're linked via a dynamic inventory by the tags Terraform sets, or via an inventory file generated by Terraform. They run as separate pipeline steps, not via `local-exec`.

**What are facts and how do you work with them?**
Data about a host that the `setup` module collects at the start of a play: OS, version, IP, memory, disks. Available via `ansible_facts`. Disabled with `gather_facts: false`, partially collected via `gather_subset`, cached via `fact_caching`. You can put your own facts in `/etc/ansible/facts.d/*.fact` on the host.

**What's the difference between defaults and vars in a role?**
`defaults/main.yml` has the weakest precedence: default values that the role's user overrides via group_vars or role parameters. `vars/main.yml` has strong precedence, for the role's internal values that aren't meant to be changed (for example, package names for different OSes).

**A task failed on one of 10 hosts. What happens to the rest?**
By default the failed host drops out of the play's remaining tasks, and the other 9 continue. At the end the host is in `failed`. To change the behaviour: `any_errors_fatal: true` — stop everyone, `max_fail_percentage` — the acceptable share of failures, `ignore_errors` on a specific task, `block`/`rescue` for handling and rollback.

**How do you make sure a broken config doesn't reach the server?**
The `validate` parameter of `template` and `copy`: the file is checked with a command in a temporary location and only on success replaces the working one (`nginx -t -c %s`, `haproxy -c -f %s`, `visudo -cf %s`). Plus reload instead of restart in the handler and `--check --diff` before a rollout.

**What is ansible-pull and when is it needed?**
The reverse model: the host itself clones the repository with playbooks and applies them to itself, usually via cron or on first boot. It's needed when the control node can't connect to the hosts (NAT, thousands of ephemeral machines) or for self-configuring images.

**How do the ansible and ansible-core packages differ?**
`ansible-core` is the engine, the CLI and the built-in `ansible.builtin` modules. `ansible` is ansible-core plus a large set of community collections. In prod people often install `ansible-core` and the explicitly needed collections from `requirements.yml` with pinned versions, to control exactly what's used.
