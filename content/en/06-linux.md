# Linux OS and the file system

How Linux works from an operations point of view, in depth: boot, files and inodes, permissions, processes and signals, memory, performance troubleshooting, systemd. Each topic: what it is, why you need to know it, how it works, commands with explanations and typical problems discussed in interviews.

The second part covers kernel and host tuning for low latency: CPU, NUMA, hugepages, interrupts and the network card, DPDK, EC2 specifics and FPGAs.

## Kernel, user space and system calls

Linux is the **kernel**: a program that manages the hardware (CPU, memory, disks, network) and hands it out to processes. Everything else (bash, systemd, nginx, the `ls` and `ps` utilities) is ordinary programs in **user space**. Together with the GNU utilities and a package manager this makes up a distribution: Ubuntu, RHEL, Amazon Linux, Alpine.

A program can't read a disk or open a network connection by itself. It asks the kernel via a **system call** (syscall): `open` (open a file), `read`, `write`, `fork` (create a process), `execve` (run a program), `socket`, `connect`. Why: the kernel checks permissions, isolates processes from each other and doesn't let one program corrupt another's memory.

To see a process's system calls — `strace`. It's one of the most powerful diagnostic tools when "the program just hangs" or "crashes without a clear error":
```bash
strace -f -e trace=openat,connect curl -s https://example.com > /dev/null
# shows which files curl opens (certificates, resolv.conf) and where it connects
strace -p 1234 -f            # attach to an already running process: what is it stuck on?
```

### 🧪 Mini task

**Task.** On a Linux machine use `strace` to see which system calls `cat /etc/hostname` makes: how the file is opened, how many bytes are read and how the result gets to the screen.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
strace -e trace=openat,read,write cat /etc/hostname 2>&1 >/dev/null | grep -A3 'etc/hostname'
```

Expected output:

```text
openat(AT_FDCWD, "/etc/hostname", O_RDONLY) = 3
read(3, "web-01\n", 131072)             = 7
write(1, "web-01\n", 7)                 = 7
read(3, "", 131072)                     = 0
```

Walkthrough: a program can't read the disk by itself — it asks the kernel. `openat` returned file descriptor 3 (a file descriptor is the number of an open file inside a process; 0, 1, 2 are already taken by stdin, stdout, stderr). `read` read 7 bytes, `write(1, …)` sent them to stdout, and a `read` returning 0 means end of file. `strace` is the main tool when a process "hangs" or crashes without a clear error: you immediately see which call it is stuck on and which error the kernel returned (`ENOENT` — "no such file or directory", `EACCES` — "permission denied"). Your hostname will be your own.

</details>

## The boot process

What happens from pressing the power button to the login prompt:

1. **BIOS or UEFI** (the motherboard firmware; UEFI is the modern replacement for BIOS) checks the hardware and finds the boot disk.
2. **The GRUB2 boot loader** (GRand Unified Bootloader — the first program from the disk that the firmware starts) shows a menu and loads the kernel (`/boot/vmlinuz-*`) and the initramfs into memory. In this menu you can pick the previous kernel version if the new one doesn't boot.
3. **initramfs** (initial RAM filesystem) — a small temporary file system in memory with the drivers needed to find and mount the real root partition (disk drivers, LVM — the logical volume manager, encryption). Without it the kernel wouldn't see the disk the system itself lives on.
4. **The kernel** initialises the hardware, mounts the root FS and starts the first process — **PID 1** (the process with number 1, the "ancestor" of all others; if it exits, the kernel panics).
5. **PID 1 — systemd** (in modern distributions) starts all the services by dependencies up to the target state. A target state in systemd is called a **target** — a named group of units: `multi-user.target` for a server (network and services without graphics), `graphical.target` for a desktop.

Why know this: "the server doesn't boot after a kernel update" (pick the old kernel in the GRUB menu), "a disk from `/etc/fstab` doesn't mount" (the system drops into **emergency mode** — a rescue mode with a minimal root shell without network and services, where you fix fstab and reboot), "an EC2 instance doesn't boot" (a cloud VM has no monitor — look at the boot output via the EC2 console "Get system log" or "EC2 Serial Console").

### 🧪 Mini task

**Task.** On a Linux machine with systemd find out how long boot took per stage, which units took the longest, and whether the current boot's journal has errors.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
systemd-analyze
systemd-analyze blame | head -3
journalctl -b -p err --no-pager | tail -3
```

Expected output:

```text
Startup finished in 2.104s (kernel) + 6.871s (userspace) = 8.975s
multi-user.target reached after 6.512s in userspace.
3.402s cloud-init.service
1.218s snapd.seeded.service
 804ms systemd-networkd-wait-online.service
-- No entries --
```

Breakdown: the time splits into stages — the kernel and user space (systemd with its units). `blame` sorts units by start time: on cloud VMs `cloud-init` leads, and `*-wait-online` waits for the network. `journalctl -b -p err` shows errors and worse from the current boot only — the first place to look after a reboot. "-- No entries --" means there are none. `systemd-analyze critical-chain` shows the chain that actually delays the boot. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## The filesystem

### Directory hierarchy (FHS)
Linux has one directory tree rooted at `/`. Other disks don't get letters as in Windows; they are "attached" (mounted) to some folder of that tree. Which directories are for what is described in the **FHS** standard (Filesystem Hierarchy Standard), so the layout is almost the same on Ubuntu, RHEL and Alpine, and you look for logs in `/var/log` everywhere.

| Directory | What's there and why you should know |
|---|---|
| `/etc` | System and service configuration (`/etc/nginx/`, `/etc/ssh/sshd_config`, `/etc/fstab`) |
| `/var` | Variable data: `/var/log` (logs), `/var/lib` (service data: docker, postgres), `/var/cache`. A common cause of a full disk |
| `/usr` | Programs and libraries installed by the package manager (`/usr/bin`, `/usr/lib`) |
| `/opt` | Third-party software installed manually |
| `/home`, `/root` | Home directories of users and of root |
| `/tmp` | Temporary files, often in memory (tmpfs — a file system in RAM: fast, but it uses memory), cleared on reboot |
| `/proc` | A **virtual** FS: it isn't on disk, the kernel generates it on the fly. Information about processes (`/proc/<pid>/`) and the system (`/proc/meminfo`, `/proc/cpuinfo`) |
| `/sys` | A virtual FS with information about devices and drivers, cgroups (`/sys/fs/cgroup` — the kernel mechanism that limits CPU and memory of a group of processes; container limits are built on it) |
| `/dev` | Device files: disks (`/dev/nvme0n1`), `/dev/null`, `/dev/random` |
| `/boot` | The kernel and bootloader |
| `/run` | Runtime data: PID files, sockets. In memory |

"Everything in Linux is a file": devices, processes (`/proc`), sockets, pipes. That's why the same tools (`cat`, `echo`, redirections) work with all of them: `cat /proc/loadavg`, `echo 1 > /proc/sys/net/ipv4/ip_forward`.

### Inode — how a file is actually stored
A file on disk consists of two parts:
- the **inode** — a record with *metadata*: type, permissions, owner, size, modification times and pointers to the data blocks. **The file name is not in the inode.**
- the **data blocks** — the content itself.

The file name is stored in the **directory**. A directory is just a "name → inode number" table.

```bash
ls -i /etc/hosts          # → 131074 /etc/hosts — the inode number
stat /etc/hosts           # all inode metadata: size, permissions, link count, times
df -i                     # how many inodes are used and free on each filesystem
```
The number of inodes is set when the filesystem is created (ext4) and doesn't grow. Hence the classic problem: **"No space left on device", although `df -h` shows free space.** The inodes ran out: millions of small files on the disk (PHP sessions, cache, a mail queue, small logs). Check with `df -i`. To find where there are many: `find / -xdev -type f | cut -d/ -f2-3 | sort | uniq -c | sort -rn | head`.

### Hard and symbolic links
A **hard link** is another name for the same inode. Both directory entries are equal; there's no "original".
```bash
echo "data" > a.txt
ln a.txt b.txt            # a hard link
ls -li a.txt b.txt        # the same inode number, link count = 2
rm a.txt                  # removed one name
cat b.txt                 # → data — the data lives as long as at least one name remains
```
Limitations: only within one filesystem (an inode number only makes sense inside it) and not for directories.

A **symbolic link (symlink)** is a separate small file containing the *path* to another file. Like a shortcut in Windows.
```bash
ln -s /opt/app-1.4 /opt/app     # /opt/app points to /opt/app-1.4
ls -l /opt/app                  # → /opt/app -> /opt/app-1.4
```
It can point to a directory and to another FS. If the target is deleted, the link "dangles" (a dangling link — the link exists, but the file it points to doesn't; accessing it gives "No such file or directory"). A typical use is switching versions: deploy the new version to `/opt/app-1.5` and switch the link with `ln -sfn /opt/app-1.5 /opt/app` (`-f` — replace the existing one, `-n` — don't descend into the old link to a directory). Strictly speaking, `ln -sfn` deletes the old link and creates a new one — for a fraction of a millisecond there is no link. Fully atomic (without such a window): `ln -s /opt/app-1.5 /opt/app.tmp && mv -T /opt/app.tmp /opt/app` — a rename in Linux is atomic.

### When a file is really deleted — and why the space wasn't freed
A file's data is freed when **both** conditions hold:
1. the hard link count is 0 (no names are left);
2. **no process holds the file open.**

A classic interview case: a giant log was deleted with `rm /var/log/app.log`, but `df` shows the space wasn't freed. The application keeps writing into an open file descriptor, and the file exists without a name.
```bash
lsof +L1                      # open files with a link count of 0 (deleted)
lsof | grep deleted           # the same
# → java  4321  app  12w  REG  ...  15G  /var/log/app.log (deleted)

# free the space without restarting the process: truncate the file via the descriptor
: > /proc/4321/fd/12
# or restart the process / ask it to reopen its logs (often the HUP signal)
```
`du` then shows less than `df`: `du` counts the files it sees by name, while `df` counts the blocks actually in use.

How to clean up logs properly: `logrotate` (the standard log-rotation utility, run once a day by a timer: it renames `app.log` to `app.log.1`, compresses old ones, deletes the oldest) with the `copytruncate` option (copy and truncate, the process keeps writing to the same file) or with `postrotate`, which sends the process a signal to reopen the file — usually `SIGHUP` (the "reread the config / reopen files" signal; nginx, for example, reopens its logs on `kill -USR1`).

### Permissions
Every file has an owner (user), a group and three sets of permissions: for the owner, the group and everyone else (other).
```bash
ls -l deploy.sh
# -rwxr-x---  1 alex devops  1024 Oct 12 10:00 deploy.sh
# │└┬┘└┬┘└┬┘
# │ │  │  └─ other: ---  (nothing)
# │ │  └──── group: r-x  (read and execute)
# │ └─────── user:  rwx  (everything)
# └───────── type: - file, d directory, l symlink
```
**What permissions mean** differs for a file and a directory:

| Permission | For a file | For a directory |
|---|---|---|
| `r` (4) | read the content | list the files (`ls`) |
| `w` (2) | modify the content | create, delete and rename files **in it** |
| `x` (1) | run it as a program | "enter" it (`cd`) and access files inside by name |

Consequences that come up in interviews:
- You can delete a file if you have `w` on the **directory**, even without permissions on the file itself (deletion is a change to the directory).
- Without `x` on a directory you can't read a file inside it, even if the file has `r`.

**Numeric notation:** the sum of r=4, w=2, x=1 for each triplet:
```bash
chmod 755 script.sh     # rwxr-xr-x — the owner can do everything, the rest read and execute
chmod 644 config.yaml   # rw-r--r-- — typical for files
chmod 600 id_rsa        # rw------- — an SSH private key (ssh refuses to work with more open permissions)
chmod 700 ~/.ssh        # rwx------
chmod u+x,g-w file      # symbolic notation: add x for the owner, remove w for the group
chown alex:devops file  # change the owner and group
chmod -R g+rX /srv/app  # X (capital) — x only for directories and files that are already executable
```

**umask** — which permissions to *remove* from new files (a mask the process inherits from its parent; set in `~/.profile`, in a systemd unit `UMask=`). Files are created with 666, directories with 777, minus the umask. With `umask 022` files get 644, directories 755. Situation: a service creates files that another service in the same group can't read — check its umask (it needs `002`, not `022`).

**Special bits:**
- **SUID** (4000, `s` in place of the owner's `x`) — the program runs with the permissions of the *file's owner*, not of whoever started it. Example: `/usr/bin/passwd` belongs to root and has SUID, so an ordinary user can change their password in `/etc/shadow`. Finding all SUID files is part of a security audit: `find / -perm -4000 -type f 2>/dev/null`.
- **SGID** (2000) — on a directory: new files inherit the directory's *group*. Handy for a team's shared folders.
- **Sticky bit** (1000, `t` at the end) — on a directory: only a file's owner can delete it. Set on `/tmp` (permissions `1777`): everyone can write, but nobody can delete other people's files.

**Additionally:**
- **ACL** (Access Control List — a list of permissions for individual users and groups) — permissions for specific users on top of the three groups: `setfacl -m u:deploy:rx /srv/app`, `getfacl /srv/app`. Needed when the `deploy` user must get access to a directory without being added to the owner's group. The sign of an ACL is a `+` at the end of the permissions in `ls -l` (`drwxr-x---+`).
- **Attributes** — `chattr +i file` makes a file immutable even for root (until the attribute is removed). `lsattr` shows the attributes. A classic puzzle: root can't delete a file and gets "Operation not permitted" — check `lsattr`.
- **Capabilities** — "pieces" of root's rights: instead of "all or nothing" the kernel splits privileges into ~40 separate ones (`CAP_NET_BIND_SERVICE` — listen on ports below 1024, `CAP_NET_ADMIN` — configure the network, `CAP_SYS_ADMIN` — almost everything). For example, allow a program to listen on a port below 1024 without running as root: `setcap cap_net_bind_service=+ep /usr/local/bin/app`. The same mechanism in Docker and Kubernetes: `capabilities: drop: [ALL]` in the securityContext.

### Disks, partitions, mounting
```bash
lsblk -f                  # a tree of disks and partitions with filesystem types and mount points
blkid                     # partition UUIDs
df -hT                    # free space per filesystem (h — human-readable units, T — the filesystem type)
du -sh /var/log/*         # how much each item takes
du -xh / --max-depth=1 2>/dev/null | sort -h   # large directories at the root; -x — don't cross into other filesystems
findmnt                   # the tree of mount points
```
Attach a new disk (for example, an EBS volume in EC2 — Elastic Block Store, the AWS network disk that attaches to an instance as an ordinary block device):
```bash
lsblk                                 # find the new disk, e.g. /dev/nvme1n1
sudo mkfs.ext4 /dev/nvme1n1           # create a filesystem (destroys the data on the disk!)
sudo mkdir /data
sudo mount /dev/nvme1n1 /data
# to mount it after a reboot — a line in /etc/fstab (by UUID, since nvme names can change):
# UUID=1234-abcd  /data  ext4  defaults,nofail  0  2
sudo mount -a                         # check fstab without rebooting: an error here is better than a server that won't boot
```
`nofail` — if the disk is missing, the system still boots (without it the server drops into emergency mode).

Grow the disk after enlarging an EBS volume in AWS (the volume size is changed in the console or via `aws ec2 modify-volume` without stopping the instance, but the partition and the file system inside don't grow by themselves — you grow them with the commands below):
```bash
sudo growpart /dev/nvme0n1 1          # grow partition 1 to the whole disk
sudo resize2fs /dev/nvme0n1p1         # grow ext4 (online, without unmounting)
sudo xfs_growfs /                     # for XFS — by mount point
```

**LVM** (Logical Volume Manager) — a layer between disks and file systems: physical disks (PV, physical volume) are combined into a group (VG, volume group — a shared "pool" of space), from which logical volumes are carved (LV, logical volume — what you create a FS on). It lets you grow volumes and add disks without repartitioning: `/data` ran out of space — add a new disk to the group and stretch the volume without stopping the service: `lvextend -r -L +10G /dev/vg0/data` (`-r` grows the FS right away).

**File system types:** ext4 (general-purpose, the default in Ubuntu), XFS (the default in RHEL and Amazon Linux, good for large files, **can't be shrunk**), btrfs (snapshots — instant copies of the FS state), tmpfs (in memory), overlayfs (stacks several directories on top of each other so they look like one — this is how container image layers are assembled), NFS/EFS (network: NFS is a network file system protocol, EFS is managed NFS in AWS that can be mounted on many servers at once).

### 🧪 Mini task

**Task.** On Linux (a `docker run -it --rm ubuntu:24.04` container will do) create a file, a hard link and a symbolic link to it. Check the inode and the link count, then delete the original file and try to read both links.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo data > orig
ln orig hard
ln -s orig soft
[ "$(stat -c %i orig)" = "$(stat -c %i hard)" ] && echo "hard link: same inode"
echo "links: $(stat -c %h orig)"
echo "soft -> $(readlink soft)"
rm orig
cat hard
cat soft
```

Expected output:

```text
hard link: same inode
links: 2
soft -> orig
data
cat: soft: No such file or directory
```

Breakdown: a file name is just a directory entry pointing to an inode, which holds the metadata and references to data blocks. A hard link is a second name for the same inode, hence the link count of 2, and the data lives as long as at least one link exists. A symbolic link is a separate file containing a path: the path is gone, so it "dangles". The same logic explains why `df` still shows space used after `rm` of a large log: the space is freed when the process closes the file (`lsof +L1` finds such files).

</details>

## Processes

### What a process is
A process is a running program. Each one has:
- a **PID** — a unique number; a **PPID** — the parent's number;
- a **UID** — which user it runs as;
- an **environment** — environment variables;
- **file descriptors** — open files and sockets (0 — stdin, 1 — stdout, 2 — stderr);
- its own virtual memory, current directory, limits.

### How processes are created: fork and exec
A new process in Linux is created in two steps:
1. **`fork()`** — the process creates a copy of itself (a child process). Copying is cheap thanks to **copy-on-write**: at first the parent and the child share the same memory pages, and a page is actually copied only when one of them tries to change it.
2. **`exec()`** — the child replaces its program with another one.

That's how the shell runs a command: `fork` → in the child `exec("/bin/ls")` → the parent waits for completion via `wait()` and gets the exit code.

### Process states
In `ps` and `top`, the `STAT`/`S` column:
- **R** (running) — running or ready to run.
- **S** (sleeping) — sleeping while waiting for an event (network, timer). The normal state of most processes.
- **D** (uninterruptible sleep) — waiting for I/O from the kernel (disk, NFS — a network disk) and **doesn't react to signals, not even `kill -9`**. Many processes in D is a sign of trouble with a disk or network storage. A typical situation: an NFS server went away, and every process that touched the mounted folder is "stuck" in D forever, and `df` hangs too.
- **Z** (zombie) — the process has exited, but the parent hasn't collected its exit code yet.
- **T** (stopped) — stopped (Ctrl+Z or SIGSTOP).

### Zombies and orphans
A **zombie** is a process that has already exited and consumes no resources, but its record (with the exit code) stays in the process table because the parent didn't call `wait()`. One zombie is not a problem. Thousands are: they take up PID numbers, and their number is limited.
- You can't kill a zombie: it's already dead.
- The fix is to fix or restart the **parent**. When the parent dies, PID 1 "adopts" the zombie and reaps it.

An **orphan** is a process whose parent died while it is still running. PID 1 adopts it. That's normal (daemons work this way).

Connection to containers: in a container your application is often PID 1 and must reap the zombies of its child processes. Most applications can't do that, so people use `tini` (a tiny init process: it becomes PID 1, starts the application as a child, forwards signals to it and reaps zombies) or `docker run --init`, which inserts tini automatically.

### Signals
A signal is a notification sent to a process by the kernel or another process. A process can catch a signal with its own handler (except KILL and STOP).

| Signal | Number | What it means | Typical use |
|---|---|---|---|
| SIGTERM | 15 | "Please exit" | A normal shutdown: `kill PID`, `systemctl stop`, `docker stop`, deleting a pod. The application should close its connections properly and exit |
| SIGKILL | 9 | Immediate kill by the kernel | Can't be caught: the process has no chance to save anything. A last resort |
| SIGINT | 2 | Interrupt | Ctrl+C in a terminal |
| SIGHUP | 1 | "Terminal hangup" | For daemons, by convention — reload the config (`nginx -s reload`, `kill -HUP`) |
| SIGSTOP / SIGCONT | 19 / 18 | Pause / continue | Ctrl+Z, `fg` |
| SIGCHLD | 17 | A child process has exited | A signal to the parent: time to call `wait()` |
| SIGSEGV | 11 | Accessing someone else's memory | A program crash (segfault) |

The exit code of a process killed by a signal is `128 + signal number`: **137** = 128 + 9 (SIGKILL, often the OOM killer — the kernel mechanism that kills a process when memory runs out, see the "Memory" section), **143** = 128 + 15 (SIGTERM), **139** = 128 + 11 (segfault — the program accessed memory that doesn't belong to it, usually a bug in C/C++ code or a library).

**The PID 1 peculiarity.** The kernel doesn't apply default signal actions to PID 1: a SIGTERM for which there's no handler is simply ignored. That's why an application in a container running as PID 1 without a handler doesn't stop on `docker stop` and gets SIGKILL after 10 seconds.

### Commands for working with processes
```bash
ps aux                      # all processes: user, PID, %CPU, %MEM, state, command
ps aux --sort=-%mem | head  # top by memory
ps -ef --forest             # the process tree (who started whom)
pstree -p                   # the same, more readable
pgrep -af nginx             # find processes by name (with arguments)
kill 1234                   # send SIGTERM
kill -9 1234                # SIGKILL — only if TERM didn't help
pkill -HUP nginx            # send a signal to all processes by name
top / htop                  # interactive monitoring
lsof -p 1234                # which files and sockets the process has open
lsof -i :8080               # who holds port 8080
cat /proc/1234/status       # state, memory, threads
cat /proc/1234/limits       # the process's limits (for example, Max open files)
tr '\0' '\n' < /proc/1234/environ   # the process's environment variables
ls -l /proc/1234/fd         # open descriptors
ls -l /proc/1234/cwd        # the process's current directory
nice -n 10 ./backup.sh      # run with lower CPU priority (from -20 to 19, higher means lower priority)
ionice -c3 ./backup.sh      # lower disk priority
```

### Limits: "Too many open files"
Each process can open a limited number of files and sockets (often 1024 by default). Importantly, every network connection is also an open "file" (a socket). A busy web server or database with thousands of clients hits this limit and starts failing with `Too many open files` — usually right at peak load.
```bash
ulimit -n                       # the current limit in this shell
cat /proc/<pid>/limits          # the limit of a specific process
ls /proc/<pid>/fd | wc -l       # how many it has open right now
```
How to raise it: for a systemd service — `LimitNOFILE=65536` in the unit file; for user sessions — `/etc/security/limits.conf`; in containers — `--ulimit nofile=65536:65536`.

### 🧪 Mini task

**Task.** Start three background processes and stop them with signals: a script with a `SIGTERM` handler (graceful shutdown), `sleep` with `SIGTERM`, and `sleep` with `SIGKILL`. Compare the exit codes.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
bash <<'EOF' 2>/dev/null
bash -c 'trap "echo got SIGTERM, cleaning up; exit 0" TERM; while true; do sleep 0.1; done' &
pid=$!; sleep 0.5; kill -TERM $pid; wait $pid; echo "graceful exit=$?"
sleep 30 & pid=$!; kill -TERM $pid; wait $pid; echo "sleep + SIGTERM exit=$?"
sleep 30 & pid=$!; kill -KILL $pid; wait $pid; echo "sleep + SIGKILL exit=$?"
EOF
```

Expected output:

```text
got SIGTERM, cleaning up
graceful exit=0
sleep + SIGTERM exit=143
sleep + SIGKILL exit=137
```

Breakdown: a process killed by a signal gets code 128 + the signal number: 143 = 128 + 15 (TERM), 137 = 128 + 9 (KILL). Exit code 137 on a Kubernetes pod means it was killed with KILL — usually the OOM killer or an expired `terminationGracePeriodSeconds`. `SIGTERM` can be caught for a clean exit (finish requests, close connections), `SIGKILL` can't. That's why an app in a container must handle TERM, or it dies hard on every deploy. `2>/dev/null` hides bash messages like "Terminated".

</details>

## Memory

### How to read free
```bash
free -m
#               total   used   free  shared  buff/cache  available
# Mem:          15890   6120    410     210       9360       9240
# Swap:          2047     12   2035
```
- **used** — taken by processes.
- **free** — not used at all. **A small free value is normal.**
- **buff/cache** — memory used for the **page cache**: the kernel caches the content of recently read files so that the next read comes from memory, not from disk. This memory is given to processes on demand.
- **available** — how much memory can actually be allocated to new processes (free plus what can be reclaimed from the cache). **This is the field to look at.**

A typical beginner's mistake: "the server has only 400 MB free, memory is running out!" — with 9 GB `available` everything is fine. Linux deliberately uses free memory for cache: unused memory is wasted memory.

### Swap
Swap is an area on disk where the kernel moves rarely used memory pages when memory runs short.
- The upside: a buffer against a sudden OOM.
- Downside: a disk is thousands of times slower than memory. If the system swaps actively (in `vmstat` the `si`/`so` columns — swap in/out, how many pages per second are read from and written to swap — are constantly non-zero), everything starts to crawl.
- `vm.swappiness` (0–100, default 60) — how eagerly the kernel pushes process memory to swap instead of freeing the page cache. On database servers it's often set to 1–10 so the database memory doesn't go to disk.
- Kubernetes historically required swap to be disabled on nodes: with it, pod memory limits can't be guaranteed (a pod could "live" on the slow disk instead of being OOMKilled). Since version 1.28 there is limited support.

### OOM killer
When memory has completely run out, the kernel picks a process and kills it with SIGKILL. The choice is by `oom_score` (a "badness" score from 0 to 1000 the kernel computes for every process; usually the biggest memory consumer gets the highest). A typical picture: at night a Java process or database suddenly "disappeared" from the server and the application logs say nothing — look at the kernel log.
```bash
dmesg -T | grep -iE "killed process|out of memory"
# → Out of memory: Killed process 4321 (java) total-vm:..., anon-rss:7800000kB
journalctl -k | grep -i oom
cat /proc/<pid>/oom_score          # the higher it is, the more likely the process gets killed
echo -1000 > /proc/<pid>/oom_score_adj   # protect a process (-1000 — never kill)
```
In containers the OOM kill happens inside the **cgroup** (control group — a group of processes for which the kernel accounts and limits CPU and memory; each container has its own) when the container's memory limit is exceeded, even if the host has free memory. The sign is `OOMKilled: true` and exit code 137.

### 🧪 Mini task

**Task.** Using Docker, run a container with a 64 MB memory limit and try to allocate 200 MB in it. Check the exit code and the `OOMKilled` flag in `docker inspect`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run --name oomtest -m 64m python:3.12-slim python -c "b = bytearray(200 * 1024 * 1024)"; echo "exit=$?"
docker inspect -f 'OOMKilled={{.State.OOMKilled}}' oomtest
docker rm oomtest > /dev/null
```

Expected output:

```text
exit=137
OOMKilled=true
```

Breakdown: `-m 64m` creates a cgroup memory limit. When the process tried to exceed it, the kernel invoked the OOM killer inside the cgroup and killed the process with `SIGKILL` — hence 137. That's exactly what Kubernetes shows as `OOMKilled` when `resources.limits.memory` is exceeded. There is no Python `MemoryError`: the process had no chance to react. On the first run Docker will also pull the image and print `Pulling` lines.

</details>

## Performance: where to start troubleshooting

To the question "the server is slow, what will you do?" a systematic approach is expected, not "I'll reboot it". It's convenient to follow the **USE** method: for each resource (CPU, memory, disk, network) check **U**tilization, **S**aturation (the queue — work that's waiting) and **E**rrors.

The classic "first minute" (a checklist from Brendan Gregg's article "Linux Performance Analysis in 60,000 Milliseconds" — a Netflix engineer and author of books on Linux performance):
```bash
uptime                 # load average over 1, 5, 15 minutes — is the load growing or falling?
dmesg -T | tail -20    # kernel errors: OOM killer, disk errors, network errors
vmstat 1 5             # the overall picture every second
mpstat -P ALL 1 3      # load of each core: one core at 100% = a single-threaded bottleneck
pidstat 1 3            # which processes load the CPU
iostat -xz 1 3         # disks: %util (share of time the disk is busy), await (average response time in ms), queue
free -m                # memory (look at available)
sar -n DEV 1 3         # traffic per network interface
sar -n TCP,ETCP 1 3    # new connections, retransmits — resends of lost TCP packets (a sign of network problems)
top                    # interactively: who is eating CPU and memory
```

### Load average — what it really is
The average number of processes that are **running or waiting for the CPU** (state R) **plus waiting for disk I/O** (state D), averaged over 1, 5 and 15 minutes.
- Compare it with the number of cores (`nproc`). LA 4 on 4 cores — the system is fully loaded; LA 16 on 4 cores — processes are queuing.
- Comparing the three numbers shows the trend: `1.2, 4.5, 8.0` — the load is decreasing; `8.0, 4.5, 1.2` — it's growing.
- **A high LA with low CPU usage** means processes are stuck in state D, i.e. the bottleneck is the disk or NFS, not the processor.

### How to read vmstat
```bash
vmstat 1
# procs ---------memory---------- ---swap-- -----io---- -system-- ------cpu-----
#  r  b   swpd   free  buff  cache   si   so    bi    bo   in   cs us sy id wa st
#  9  0      0 412000  2100 9300000   0    0     5    40 2100 4000 85 10  5  0  0
```
- `r` — processes queued for the CPU. More than the number of cores — not enough CPU.
- `b` — processes in state D (waiting for I/O).
- `si`/`so` — swap in/out. Constantly non-zero — not enough memory.
- `us` — CPU time in program code, `sy` — in the kernel (many syscalls or context switches), `id` — idle.
- `wa` — the CPU is idle waiting for the disk. A high `wa` — a disk problem.
- `st` (steal) — time the hypervisor (the software virtual machines run on) gave to other virtual machines on the same physical server. On cloud instances a high `st` means a "noisy neighbour" or exhausted CPU credits on burstable instances (t3/t4g — cheap instances that earn "credits" while idle and spend them on bursts; when the credits run out, the CPU is throttled to a baseline of 10–40%).

### How to read iostat
```bash
iostat -xz 1
# Device   r/s   w/s  rkB/s  wkB/s  r_await w_await aqu-sz  %util
# nvme0n1  150  2200   6000  90000     0.4    25.0   55.0  100.0
```
- `r_await`/`w_await` — the average time to serve a read/write request in milliseconds (including the wait in the queue). For SSD/EBS gp3 (gp3 is the standard SSD EBS volume type in AWS) single-digit milliseconds are normal, tens mean requests are queuing.
- `aqu-sz` — the average length of the disk request queue.
- `%util` — the share of time the disk was busy. For an HDD 100% is the limit, for SSD and NVMe it's less telling (they serve many requests in parallel, and 100% doesn't mean "can't do more"), look at `await`.
- In AWS, hitting the IOPS (I/O operations per second) or throughput (megabytes per second) limit of an EBS volume looks exactly like this: `await` grows while IOPS sit flat at one level — the level paid for on the volume (gp3 defaults to 3000 IOPS and 125 MB/s).

### 🧪 Mini task

**Task.** On a Linux VM create CPU load with two `yes > /dev/null` processes and see how it shows up in `uptime` and `vmstat`. Then stop the load.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
yes > /dev/null & yes > /dev/null &
sleep 10
uptime
vmstat 1 3 | tail -2
kill %1 %2
```

Expected output:

```text
 10:42:07 up 3 days,  2:11,  1 user,  load average: 1.62, 0.48, 0.17
procs -----------memory---------- ---swap-- -----io---- -system-- -------cpu-------
 r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa st gu
 2  0      0 612340  41212 905188    0    0     0     0 1043  262 99  1  0  0  0  0
 2  0      0 612340  41212 905188    0    0     0     0 1021  248 100 0  0  0  0  0
```

Breakdown: load average is the average number of processes running or waiting for a CPU over 1, 5 and 15 minutes. The first number climbs towards 2, the others lag: the load is recent. In `vmstat` the `r` column = 2 means two processes queued for CPU, `us` near 100% is time in user code, `wa` = 0 says the disk isn't involved. Compare `r` with the number of cores (`nproc`): if `r` is consistently higher, CPU is the bottleneck. High `wa` and `b` would point to I/O instead. This is the first minute of the USE method: utilisation, saturation, errors. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## Low latency: kernel and host tuning

The following sections are for hosts where the product handles traffic with a strict latency SLO (SLO, Service Level Objective — an internal quality target, for example "P99 ≤ 5 ms": 99% of requests must fit into 5 ms), and the hosts are your own, on EC2, sometimes with hardware accelerators (FPGAs — programmable chips on which part of the processing is done "in hardware", faster than on a CPU). Examples of such products: exchange quote processing, network traffic filtering, real-time advertising. This part covers how to measure latency tails and how to tune the CPU, NUMA, memory, interrupts and network, including DPDK, EC2 specifics and FPGAs. All of this is usually baked into a machine image (AMI — Amazon Machine Image, a disk template instances are launched from) — its lifecycle is covered in the "Docker / AMI images" topic.

How to read: the commands can be run on any Linux machine (preferably an EC2 instance bigger than `xlarge`, so that there are several cores and network card queues). The output is shown in comments after `# →`.

### 🧪 Mini task

**Task.** On a Linux host check the current state of three tuning "knobs": the CPU frequency governor (the policy by which the kernel picks the processor frequency: `powersave` saves energy, `performance` keeps the maximum), the Transparent Huge Pages mode (automatic use of large memory pages, see the "Memory" section) and the core isolation parameters on the kernel command line.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor 2>/dev/null || echo "no cpufreq (VM hides it)"
cat /sys/kernel/mm/transparent_hugepage/enabled
tr ' ' '\n' < /proc/cmdline | grep -E 'isolcpus|nohz_full|rcu_nocbs' || echo "no isolation params"
```

Expected output:

```text
no cpufreq (VM hides it)
always [madvise] never
no isolation params
```

Breakdown: on many cloud VMs the hypervisor controls the frequency and there is no `cpufreq` in the guest — then you can't set a governor and need the right instance type. In THP the brackets mark the active mode: `madvise` gives huge pages only to those who ask; latency-sensitive services often use `never` to avoid pauses during defragmentation. No `isolcpus`/`nohz_full` means the cores aren't isolated and the scheduler can put anything on them. This is the baseline check before any tuning: record what is there first. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## Latency, tails and why "fast on average" means nothing

**Latency** is the time from a request arriving to the response. **Throughput** is how many requests per second the system processes. These are different goals, and optimisations for them often contradict each other: batching and interrupt coalescing increase throughput but add waiting to every request.

**The P99 percentile** is the value faster than which 99% of requests were processed. "P99 = 5 ms" means: only 1 request in 100 is slower than 5 ms. The average is useless here: if 98 requests take 1 ms each and 2 requests take 200 ms each, the average is about 5 ms, while for 2% of users everything is bad.

**Tail latency** — rare slow requests. They're exactly what such projects fight. Where tails almost always come from:
- the process **waited for the CPU**: something else was running on the core, the scheduler switched context;
- **interrupts and softirqs** were handled on the same core where the application runs;
- **crossing between NUMA nodes**: the memory or the network card is "far" from the core;
- **TLB misses and page faults**: the TLB (Translation Lookaside Buffer) is a small processor cache for translating virtual memory addresses into physical ones; a miss means the translation has to be looked up in the page tables, which costs hundreds of cycles. A page fault is an access to a page that isn't in memory yet, so the kernel has to allocate or load it. Memory compaction (defragmentation) for transparent huge pages belongs here too;
- **CPU frequency drops** and waking from deep sleep (C-states — power-saving levels of an idle core: C0 is running, C1 is dozing, C6 is almost off; the deeper the sleep, the longer the wake-up);
- **garbage collection**, locks in the application itself;
- **the network**: losses and retransmissions, queues in the network card, EC2's packets-per-second limits.

The main principle: **measure first, then tune**. Every setting below costs something (cores, memory, energy, simplicity), and without measuring before and after you can't tell whether it helped.

### 🧪 Mini task

**Task.** Out of 100 requests, 98 took 2 ms and 2 took 500 ms. Compute the average, p50, p95, p99 and the maximum, and think about which number belongs on the dashboard.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import math
samples = sorted([2] * 98 + [500] * 2)
def pct(p):
    return samples[math.ceil(p / 100 * len(samples)) - 1]
print(f"avg={sum(samples) / len(samples)}ms p50={pct(50)}ms p95={pct(95)}ms p99={pct(99)}ms max={samples[-1]}ms")
EOF
```

Expected output:

```text
avg=11.96ms p50=2ms p95=2ms p99=500ms max=500ms
```

Breakdown: the 12 ms average describes no real request: 98% were six times faster, and 2% forty times slower. p50 and p95 say "all good", and only p99 sees the tail. If a page makes 50 calls to such a service, almost every user hits the tail at least once. That's why latency SLOs are set on p99 or p99.9, and an average on a dashboard is misleading.

</details>

## How to measure

**From outside — a load test with the right methodology.** The important trap is **coordinated omission**: if the load generator waits for a response before sending the next request, then while the server "hangs" it stops sending requests and doesn't account for all those that should have arrived in that time. Example: the server hung for 1 second, and the generator sent 1 request in that second instead of 1000 — one slow data point got into the statistics instead of a thousand. P99 comes out pretty and false. Tools with a constant request rate (constant rate — requests go out on a schedule regardless of responses) avoid this: `wrk2`, `vegeta`, `k6` in `constant-arrival-rate` mode.

**Inside the host — tools by level:**
```bash
mpstat -P ALL 1              # load per core: usr, sys, irq, soft, idle — you can see where interrupts are burning
pidstat -t -p <PID> 1        # CPU per process thread
pidstat -w -p <PID> 1        # context switches: cswch/s (voluntary) and nvcswch/s (preemption) — rising nvcswch = the process is being kicked off the CPU
vmstat 1                     # r — the CPU run queue, cs — context switches, in — interrupts
cat /proc/interrupts         # interrupts per core, for each network card queue
cat /proc/softirqs           # NET_RX / NET_TX per core
cat /proc/net/softnet_stat   # a line per core: the 2nd column — dropped packets, the 3rd — time_squeeze (couldn't finish processing in the allotted time)
ss -tin                      # for each TCP connection: rtt, retrans, cwnd
nstat -az | grep -i -E "retrans|drop|overflow"   # network stack counters
perf top                     # what is spending CPU right now, including kernel functions
perf stat -e cycles,instructions,cache-misses,context-switches -p <PID> -- sleep 10
```
eBPF-based tools (a kernel mechanism that lets you safely run small programs right inside the kernel and collect precise statistics with almost no overhead; the `bpfcc-tools` or `bcc-tools` package) show latencies, not just load:
```bash
runqlat-bpfcc 10 1      # a histogram: how long processes waited in the CPU run queue
softirqs-bpfcc 10 1     # how much time went to softirqs
hardirqs-bpfcc 10 1     # how much time went to hardware interrupts
tcpretrans-bpfcc        # every TCP retransmission in real time
offcputime-bpfcc -p <PID> 10   # where a thread was stalled, not running on the CPU (locks, I/O)
```
`runqlat` is the key tool for tails: if processes wait 2–3 ms for the CPU, a 5 ms SLO is already eaten.

**The host's own jitter** (jitter — the spread of latency: how unpredictably the system responds) is measured by `cyclictest` (the `rt-tests` package): a thread asks to be woken after a given interval and records how many microseconds late it was. A good way to compare a host before and after tuning:
```bash
sudo cyclictest --mlockall --priority=80 --interval=200 --distance=0 --duration=60 -a 2-3 -t 2
# → T: 0 ... Min: 2 Act: 3 Avg: 3 Max: 41     — Max in microseconds: the worst lateness
```

### 🧪 Mini task

**Task.** Measure how precisely the OS wakes up a process: ask to sleep for 1 ms a thousand times and look at the distribution of the actual delay — p50, p99 and the maximum.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
import time
lat = []
for _ in range(1000):
    t = time.perf_counter_ns()
    time.sleep(0.001)
    lat.append((time.perf_counter_ns() - t) / 1e6)
lat.sort()
print(f"p50={lat[499]:.3f}ms p99={lat[989]:.3f}ms max={lat[-1]:.3f}ms")
EOF
```

Expected output:

```text
p50=1.071ms p99=1.392ms max=4.817ms
```

Breakdown: we asked for 1 ms and got a bit more, with a long tail: the scheduler wakes the process, and if the core is busy it has to wait. That jitter is what core isolation, `nohz_full` and busy polling remove. Measurement rules: capture a distribution, not a single number; warm up before measuring; measure under production-like load; use monotonic clocks (`perf_counter_ns`, not `time.time`). Your numbers will differ; on macOS the tail is usually longer than on a tuned Linux box. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## CPU: frequency, sleep, core isolation

### Frequency and sleep states
The processor saves energy: it lowers its frequency (P-states — "performance states", steps of frequency and voltage) and enters deep sleep (C-states — sleep levels of an idle core). Waking from deep sleep takes tens of microseconds, and that adds directly to the tail: a request arrives at a "sleeping" core and waits for it to wake. The fix on latency-critical hosts is the `performance` governor and forbidding deep C-states: the cores use more energy but are always ready.
```bash
cpupower frequency-info        # the current governor and frequencies
sudo cpupower frequency-set -g performance   # keep the maximum frequency
cpupower idle-info             # the available C-states
```
On EC2 you can't control this everywhere: on most virtual instances the hypervisor controls the frequency, and AWS gives control of C-states and P-states on some large sizes and on `metal`. To check what's available — `cpupower` and the "Processor state control" documentation for the specific instance type.

**tuned** — a service with ready-made profiles that tune the system for a workload type (it changes dozens of parameters at once: governor, sysctl, THP); it's in RHEL and Amazon Linux, and there's a package for Ubuntu:
```bash
sudo tuned-adm profile network-latency   # the performance governor, no deep C-states, THP disabled and more
tuned-adm active
```
This is a good starting point, but a profile is a set of someone else's decisions. You need to know what it changes and measure the result.

### Core isolation and CPU pinning
The idea: give the application **dedicated cores** on which nothing else runs — no other processes, no interrupts, no kernel timers. Then the thread with the hot request-processing path never waits for the CPU.

An example layout on 8 cores:
- cores 0–1 — the system: SSH, monitoring agents, logs, interrupts;
- cores 2–7 — the application (hot threads).

Kernel boot parameters:
```text
isolcpus=managed_irq,domain,2-7   # the scheduler doesn't put ordinary tasks on 2-7 and doesn't move work there from other cores
nohz_full=2-7                     # turn off the periodic timer "tick" (up to 1000 times a second) on these cores when only one task runs there
rcu_nocbs=2-7                     # RCU is a synchronisation mechanism in the kernel; move its background callbacks off these cores
irqaffinity=0-1                   # interrupts go to the system cores by default
```
How to set them: on Ubuntu — the `GRUB_CMDLINE_LINUX` line in `/etc/default/grub`, then `sudo update-grub` and a reboot; on Amazon Linux 2023 and RHEL — `sudo grubby --update-kernel=ALL --args="isolcpus=managed_irq,domain,2-7 nohz_full=2-7 rcu_nocbs=2-7 irqaffinity=0-1"`. To check after the reboot: `cat /proc/cmdline`, `cat /sys/devices/system/cpu/isolated`.

Important: **the scheduler itself won't put anything** on isolated cores, not even your application. It has to be pinned explicitly:
```bash
taskset -c 2-7 /opt/runtime/bin/runtime     # run on cores 2-7
taskset -cp 4 <TID>                          # pin a specific thread to core 4
```
Better — in a systemd unit file, then the setting lives together with the service:
```ini
# /etc/systemd/system/runtime.service.d/cpu.conf
[Service]
CPUAffinity=2-7
```
And everything else (system.slice, user.slice) — restrict to the system cores:
```ini
# /etc/systemd/system.conf.d/cpu.conf
[Manager]
CPUAffinity=0-1
```
An alternative without a reboot is a cgroup v2 cpuset via systemd (cpuset is the part of cgroups that limits which cores a group's processes may run on; a slice is what systemd calls a group of services in the cgroup hierarchy): `AllowedCPUs=2-7` for the service and `AllowedCPUs=0-1` for `system.slice` (all system services) and `user.slice` (user sessions, for example SSH). It's more flexible than `isolcpus`, but it won't move kernel timers and RCU off the cores.

**Hyper-Threading (SMT, Simultaneous Multithreading).** One physical core is presented to the system as two logical CPUs that share its caches and execution units — so the neighbouring thread can slow down the hot one. Options: don't use the "sibling" of the hot core (`lscpu -e` shows which logical CPUs share a core) or turn SMT off (`echo off > /sys/devices/system/cpu/smt/control`). On EC2 you can launch an instance with `ThreadsPerCore=1` via CPU options (a launch parameter that disables the second thread on every core).

**Real-time priority.** `chrt -f 50 <cmd>` or `CPUSchedulingPolicy=fifo` in systemd start a process with the SCHED_FIFO policy (a "real-time" scheduler policy: the process runs until it gives up the CPU itself or a higher-priority process arrives): ordinary tasks don't preempt it. The risk: a looping FIFO thread can "starve" the core, and even necessary system tasks won't run on it. On isolated cores the normal policy is usually enough, because there are no competitors there.

### 🧪 Mini task

**Task.** On a Linux machine look at the CPU topology, start a process pinned to core 0 with `taskset` and check its affinity.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
lscpu | grep -E '^CPU\(s\)|Thread\(s\) per core|NUMA node\(s\)'
taskset -c 0 sleep 30 &
taskset -cp $!
kill $!
```

Expected output:

```text
CPU(s):                               4
Thread(s) per core:                   2
NUMA node(s):                         1
pid 2841's current affinity list: 0
```

Breakdown: 4 logical CPUs with 2 threads per core means 2 physical cores with Hyper-Threading: sibling threads share caches and execution units, so a critical thread and a "noisy" neighbour on the same physical core interfere. `taskset -c 0` pinned the process to core 0, so the scheduler won't move it — otherwise the warm cache is lost on every migration. But pinning without `isolcpus` doesn't protect the core from other processes: they can be put on core 0 too. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## NUMA: memory and devices "next to" the core

**NUMA (Non-Uniform Memory Access)** — the architecture of multi-processor servers: each processor socket (a physical processor on the motherboard) has its own memory and its own PCIe devices (PCIe is the bus network cards, NVMe disks and GPUs are attached to). Access to "its own" memory is faster, to another node's memory noticeably slower (roughly 1.5–2 times), over the inter-processor bus. Situation: on a two-processor server the application runs on the second processor's cores while the network card hangs off the first — every packet crosses the bus, and latency grows for no visible reason.

```bash
lscpu | grep -i numa
# → NUMA node(s):  2
# → NUMA node0 CPU(s): 0-31,64-95
# → NUMA node1 CPU(s): 32-63,96-127
numactl --hardware                         # the nodes, the memory on each, the "distances" between them
cat /sys/class/net/eth0/device/numa_node   # which node the network card is attached to (-1 — no data, usually on small VMs)
numastat -p <PID>                          # how much of the process's memory is on each node
```
On EC2 large instances have several NUMA nodes (usually the biggest sizes and `metal`). A small instance is one node, and the topic is irrelevant.

The rule: **the application, its memory, the network card and its interrupts — on one node.**
```bash
numactl --cpunodebind=0 --membind=0 /opt/runtime/bin/runtime
```
In systemd (version 243+):
```ini
[Service]
CPUAffinity=2-15
NUMAPolicy=bind
NUMAMask=0
```

**NUMA auto-balancing** (`kernel.numa_balancing=1`) — the kernel periodically checks which node a process accesses memory from and moves pages closer to it on the fly. For an application that is already pinned correctly these are extra page faults (the kernel deliberately "breaks" access to a page to track who uses it) and migrations, so on latency-sensitive hosts it's often turned off: `sysctl kernel.numa_balancing=0`.

### 🧪 Mini task

**Task.** On a two-socket server or a large instance (for example `c6i.metal`) look at the NUMA topology: which CPUs are in which node, how much memory each node has and the "distances" between nodes. Find out which node the network card is attached to.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
numactl --hardware | grep -E '^available|cpus|size|^  0|^  1|distances'
cat /sys/class/net/ens5/device/numa_node
```

Expected output:

```text
available: 2 nodes (0-1)
node 0 cpus: 0 1 2 3 4 5 6 7 32 33 34 35 36 37 38 39
node 0 size: 127864 MB
node 1 cpus: 8 9 10 11 12 13 14 15 40 41 42 43 44 45 46 47
node 1 size: 129019 MB
node distances:
  0:   10   21
  1:   21   10
0
```

Breakdown: each socket has its own "near" memory. Distance 10 is access to local memory, 21 to remote memory, roughly twice as expensive. The network card sits on node 0's PCIe bus, so the app handling the traffic should run on node 0's CPUs and memory: `numactl --cpunodebind=0 --membind=0 ./app`. Small VMs have a single node and print `available: 1 nodes (0)` — NUMA doesn't matter there. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## Memory: hugepages, THP, swap

### Hugepages
Memory is allocated in **pages** — 4 KB blocks; for each page the kernel stores which physical address it maps to. The processor caches this address translation in a small buffer, the **TLB** (Translation Lookaside Buffer, usually a few thousand entries). If a process works with tens of gigabytes (a database, a JVM with a big heap, a DPDK application), the TLB doesn't have enough entries, it keeps missing, and every miss is a walk of the page tables in memory, hundreds of cycles. **Hugepages** (large 2 MB or 1 GB pages) cut the number of pages hundreds of times, and there are far fewer misses.

**Static hugepages** are reserved in advance and don't take part in ordinary memory management:
```bash
sudo sysctl -w vm.nr_hugepages=1024        # 1024 × 2 MB = 2 GB
grep -i huge /proc/meminfo
# → HugePages_Total:    1024
# → HugePages_Free:     1024
# → Hugepagesize:       2048 kB
```
It's more reliable to reserve them at boot (the kernel parameter `hugepages=1024`, or for 1 GB: `default_hugepagesz=1G hugepagesz=1G hugepages=4`), because on a running system memory is fragmented and a contiguous 2 MB or 1 GB may not be found. For NUMA — reserve them on the right node: `/sys/devices/system/node/node0/hugepages/hugepages-2048kB/nr_hugepages`.

The application uses them via `mmap` (the system call that maps memory or a file into the process address space) with the `MAP_HUGETLB` flag or via files in a mounted `hugetlbfs` (a special file system where every file lives in hugepages; this is what DPDK does). This memory needs `LimitMEMLOCK=infinity` in the unit file if the application locks memory (`mlock` — forbidding pages from being swapped out), otherwise systemd limits the amount of locked memory to a few megabytes.

### Transparent Huge Pages (THP)
The kernel itself tries to assemble ordinary pages into large ones — without the application's involvement. Convenient, but the background `khugepaged` process (a kernel thread that finds and "glues together" suitable regions) and memory compaction (moving pages around to free a contiguous 2 MB chunk) cause **unpredictable pauses** of milliseconds right in the middle of handling a request — a classic source of tails.
```bash
cat /sys/kernel/mm/transparent_hugepage/enabled
# → [always] madvise never
echo madvise | sudo tee /sys/kernel/mm/transparent_hugepage/enabled   # only for those who explicitly asked via madvise()
```
On latency-sensitive hosts — `madvise` or `never`. Many databases (Redis, MongoDB) explicitly recommend disabling THP.

### Swap and memory locking
Accessing a page swapped out to disk takes milliseconds (the page has to be read from disk). For a latency-critical host: swap is off or `vm.swappiness=1`, and hot processes lock their memory with `mlockall` (pin all of the process's pages in RAM) so it's never swapped out.

### 🧪 Mini task

**Task.** On a Linux machine check whether hugepages are reserved, their size, the THP mode and how eagerly the system swaps.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
grep -E '^HugePages_(Total|Free)|^Hugepagesize' /proc/meminfo
cat /sys/kernel/mm/transparent_hugepage/enabled
cat /proc/sys/vm/swappiness
swapon --show || echo "no swap"
```

Expected output:

```text
HugePages_Total:       0
HugePages_Free:        0
Hugepagesize:       2048 kB
always [madvise] never
60
no swap
```

Breakdown: `HugePages_Total: 0` — no explicit hugepages are reserved. They're reserved via `vm.nr_hugepages` for DPDK, databases and the JVM to cut TLB misses on large memory. The page size is 2 MB instead of the usual 4 KB. `swappiness=60` is the default; for servers with latency-sensitive services it's lowered to 1–10, and Kubernetes nodes usually have swap off. An empty `swapon --show` means no swap is configured. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## Interrupts and the network card

**What this section is about and when you need it.** An ordinary web server doesn't need these settings: the Linux kernel handles networking well by default. You have to dig into them on two kinds of hosts:
- **latency-critical** — exchange trading, real-time ad bidding (RTB), telecom, traffic processing in the "data plane" (the part of a system the user packets themselves flow through, as opposed to the managing "control plane"). There every ten microseconds matter, and what matters is not the average latency but the tail — P99 (99% of requests are faster than this value);
- **high-traffic** — load balancers, proxies, gateways at 10–100 Gbit/s, where one overloaded CPU core starts dropping packets while the others are idle.

The typical symptom everything starts with: in `top` or `mpstat` one core is at 100% in the `si` column (softirq — see below), the others are almost free, and the loss counters in `/proc/net/softnet_stat` keep growing.

### How a packet reaches the application
1. The network card (NIC, Network Interface Card) puts the packet into a **ring buffer** of one of its **queues**. A ring buffer is a fixed-size array in memory (for example, for 1024 or 4096 packets) that the card writes packets into in a circle while the kernel takes them from the other end. If the kernel can't keep up, the buffer fills and the card simply drops new packets — you see it in counters like `rx_missed`/`rx_dropped` in `ethtool -S` (the exact names depend on the card's driver). A modern card has several such queues (usually 4–64), each with its own buffer.
2. The card raises an **interrupt** (IRQ, interrupt request — a signal to the processor "drop what you're doing, there is data") on the CPU core that queue is bound to.
3. The interrupt handler does the minimum and defers the main work to a **softirq** — a "software interrupt", the deferred part of the handling that the kernel runs a little later on the same CPU. For network receive this is the softirq named **NET_RX**: in it the kernel takes packets in batches from the ring buffer (the NAPI mechanism — polling instead of an interrupt per packet), runs them through the network stack (IP, TCP, iptables/nftables rules) and puts the data into the right socket's buffer. Time spent in softirq shows in `top` as `si`, and in `ps` as the `ksoftirqd/N` threads when the load is high.
4. The application reads the data from the socket with `read`/`recv`.

Why it matters: steps 2–3 run on some specific CPU core and take its time. If it's the same core where the application's "hot" thread runs, the application gets latency. If all queues are bound to one core, it can't keep up: `time_squeeze` grows (the third column of `/proc/net/softnet_stat` — how many times the core ran out of its allotted time before processing all packets), and losses begin.

```bash
cat /proc/net/softnet_stat   # one line per CPU core, numbers in hex: column 1 — processed, 2 — dropped, 3 — time_squeeze
mpstat -P ALL 1              # load of every core; look at the %soft column
```

### Queues and RSS
**RSS (Receive Side Scaling)** is a feature of the network card itself: it computes a hash of the packet's addresses and ports (source and destination IP, TCP/UDP ports) and picks a queue by it. Packets of one connection always land in the same queue (order is preserved), while different connections spread across different queues, which means across different CPU cores. Example: a card with 8 queues on 8 cores — 10,000 connections are spread roughly evenly, and each core processes its own eighth of the traffic.
```bash
ethtool -l eth0                 # how many queues (channels) exist and how many are enabled
# → Pre-set maximums: Combined: 8
# → Current hardware settings: Combined: 8
sudo ethtool -L eth0 combined 4 # keep 4 queues
ethtool -x eth0                 # the RSS indirection table
ethtool -g eth0                 # the ring buffer sizes
sudo ethtool -G eth0 rx 2048    # enlarge the receive buffer (fewer losses during bursts, but a longer queue)
ethtool -c eth0                 # interrupt coalescing
ethtool -k eth0                 # offload features: GRO, TSO and others
```
- **Channels** in `ethtool -l` are the queues; `Combined` means one queue serves both receive and transmit (that's how AWS ENA — Elastic Network Adapter, the EC2 network card — works).
- **The RSS indirection table** (`ethtool -x`) says which hash value goes to which queue; it's changed, for example, to stop sending traffic to the queue bound to the application's core.
- **Ring buffer size** (`-g`/`-G`): a bigger buffer survives short traffic bursts without losses, but a packet may wait in the queue longer. It's enlarged when `rx_dropped`/`rx_missed` in `ethtool -S` grow during bursts.

**Interrupt coalescing** — the card doesn't poke the CPU for every packet but waits until several packets have arrived or a few microseconds have passed (`rx-usecs`), and reports them with one interrupt. The upside is fewer interrupts and higher throughput, the downside is that **every packet waits** for that interval. On hosts where latency matters the interval is reduced or turned off; on hosts where volume matters (backups, streaming) it's increased instead.

**Offload features** — work the card or the kernel takes on to save CPU: **GRO** (Generic Receive Offload) merges several incoming TCP packets of one flow into one large one before passing it to the stack, **TSO** (TCP Segmentation Offload) is the opposite — the card itself cuts a large outgoing block into packets. GRO saves CPU, but a packet waits until a batch is assembled — on latency-critical hosts it's sometimes turned off (`ethtool -K eth0 gro off`) and then measured to see whether things improved.

### Pinning interrupts
Every queue has its own interrupt number (IRQ), and for each IRQ you can set which CPU cores handle it (affinity).
```bash
grep eth0 /proc/interrupts      # the queues' interrupt numbers, for example ens5-Tx-Rx-0 ... -3
cat /proc/irq/45/smp_affinity_list
echo 1 | sudo tee /proc/irq/45/smp_affinity_list   # handle interrupt 45 on core 1
```
`/proc/interrupts` is a table: a row per interrupt, a column per core, the numbers are how many times that interrupt was handled on that core. It shows a skew at once: if all the `ens5-Tx-Rx-*` queues grow only in the CPU0 column, the whole network sits on one core.

The **irqbalance** service periodically redistributes interrupts across cores by itself and will overwrite manual pinning. For an ordinary server that's good, for a latency-critical one it's bad: it may move an interrupt onto a core given to the application. Options: turn it off (`systemctl disable --now irqbalance`) and lay out the interrupts with your own script at boot, or keep it but forbid it the isolated cores (`IRQBALANCE_BANNED_CPULIST=2-7` in `/etc/default/irqbalance` in newer versions). Isolated cores are cores stripped of all unrelated work so that only the application thread runs on them (`isolcpus`, see the CPU section).

The classic layout for minimal latency: **one queue — one core**, and the queue's interrupts and the application thread serving those connections on one **NUMA node**. NUMA (Non-Uniform Memory Access) — on servers with two or more processors each processor has its own "near" memory and its own PCIe devices; access to "foreign" memory or to a card attached to another processor goes over the inter-processor bus and is slower. To find which node the card is attached to: `cat /sys/class/net/eth0/device/numa_node`.

**RPS (Receive Packet Steering)** and **RFS (Receive Flow Steering)** are software analogues of RSS for when the card has few queues (for example, a virtual card with one queue and 16 cores). The interrupt still arrives on one core, but that core only quickly hands the packets out to other cores by hash, and the heavy stack processing happens there. RPS spreads packets simply by hash, RFS is smarter — it sends a packet to the core where the application thread reading that socket is currently running (the data ends up "hot" in its cache).
```bash
echo f > /sys/class/net/eth0/queues/rx-0/rps_cpus   # a bitmask: f = 1111 = cores 0–3 process packets from queue rx-0
```
**XPS (Transmit Packet Steering)** is the same for sending: which cores send packets into which card queue, so that cores don't fight over one queue.

### Network sysctls you meet on such hosts
**sysctl** — Linux kernel parameters that are changed on the fly (`sysctl -w name=value`) and persisted in `/etc/sysctl.d/*.conf`. "Such hosts" are the latency-critical and high-traffic ones from the start of the section; on an ordinary web server these values don't need touching.
```bash
net.core.busy_poll = 50          # poll the socket queue for up to 50 µs instead of sleeping until an interrupt
net.core.busy_read = 50          # the same for reads
net.core.netdev_max_backlog = 10000   # the per-core packet queue before softirq
net.core.somaxconn = 4096        # the queue of accepted connections
net.ipv4.tcp_low_latency = 1     # has no effect in new kernels — found in old guides
```
- **`net.core.busy_poll` / `busy_read`** (default 0 — off). Normally a thread waiting for data falls asleep, and when a packet arrives it's woken up — the wake-up and context switch cost from a few to tens of microseconds. **Busy polling** — the thread doesn't sleep but polls the queue in a loop for up to the given number of microseconds. This removes the wake-up latency but **burns a CPU core at 100%** even when there's no traffic — justified only on dedicated (isolated) cores of a latency-critical application. Example: an exchange quote gateway, where winning 10 µs matters more than a spare core.
- **`net.core.netdev_max_backlog`** (default 1000) — how many packets can wait in one core's queue before the stack processes them (used, for example, with RPS). If the queue overflows, packets are dropped — the second column of `/proc/net/softnet_stat` grows. It's raised on hosts with heavy, bursty traffic.
- **`net.core.somaxconn`** (default 4096 in kernels 5.4+, 128 before) — the maximum length of the **accept queue**: connections that have completed the TCP handshake but that the application hasn't taken yet with `accept()`. If the application can't keep up, the queue fills and new connections are dropped — `netstat -s` shows a growing `times the listen queue of a socket overflowed`. It's raised on load balancers and APIs under peak load; it works together with the `backlog` parameter in the application itself (for example, `listen 80 backlog=4096` in nginx).
- **`net.ipv4.tcp_low_latency`** does nothing in modern kernels (since 4.14). It shows up in old "tuning" guides — if you see it in a config, that's a sign the config was copied without understanding.

The rule for an interview: each such value is changed **for a measured problem** (we see drops in `softnet_stat` → raise `netdev_max_backlog` → measure again), not as a list "just in case".

### 🧪 Mini task

**Task.** On an EC2 instance look at how many queues the network card has, how its queue interrupts are spread across CPUs and whether `irqbalance` is running.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
ethtool -l ens5 | grep -A2 'Current hardware'
grep -E 'CPU0|ens5' /proc/interrupts | awk '{print $1, $2, $3, $NF}'
systemctl is-active irqbalance
```

Expected output:

```text
Current hardware settings:
RX:		n/a
TX:		n/a
CPU0 CPU1 CPU2 CPU3
27: 812934 0 ens5-Tx-Rx-0
28: 0 790311 ens5-Tx-Rx-1
active
```

Breakdown: ENA queues are combined (`Tx-Rx`), and each has its own interrupt. The counters show queue 0 handled by CPU0 and queue 1 by CPU1: the load is spread. If all interrupts went to CPU0, one core would become the network bottleneck while the rest idle. `irqbalance` spreads interrupts automatically. On latency-critical hosts it's disabled and interrupts are pinned by hand via `/proc/irq/N/smp_affinity_list`, away from the isolated application cores. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## DPDK and kernel-bypass networking

**DPDK (Data Plane Development Kit)** — a library that takes the network card away from the Linux kernel and lets the application work with it directly from user space. No system calls, no interrupts, no copying of data between the kernel and the application, no shared network stack. Latency drops to single-digit microseconds, and packets per second go up several times. Where it's used: software routers and firewalls, load balancers, telecom (mobile traffic processing), traffic filtering and analysis systems — places where the application parses packets itself and doesn't need the kernel's TCP stack.

How it works:
- the card is unbound from the kernel driver and bound to `vfio-pci` (a generic kernel driver that does nothing with the card itself but safely hands its memory and registers to a user-space program); **the interface disappears from `ip link`**, the kernel no longer sees it;
- **PMD (poll mode driver)** — the card's driver inside the application, which constantly polls the queues instead of waiting for interrupts (the CPU core is always at 100%, even without traffic — that's normal);
- packet memory lives in **hugepages** (see the "Memory" section) to avoid TLB misses;
- packets are processed by threads pinned to CPU cores (in DPDK terms — lcores, logical cores).

```bash
dpdk-devbind.py --status                          # which cards are bound to which driver
sudo modprobe vfio-pci
sudo dpdk-devbind.py --bind=vfio-pci 0000:00:06.0  # hand the card to DPDK
```
**The main trap on EC2:** if you unbind the primary network interface from the kernel, you **lose SSH** and access to the instance. For DPDK you attach a **second ENI** (Elastic Network Interface — a virtual network card in AWS; several can be attached to an instance), and management goes through the first. An address like `0000:00:06.0` in the command above is the card's address on the PCI bus; `dpdk-devbind.py --status` or `ethtool -i` shows it.

For the AWS network card (ENA) DPDK has its own driver, the ENA PMD. It has its own requirements (for example, LLQ mode — Low Latency Queue, where packets are written straight into the card's memory, and write-combining — a way of writing to device memory in large batches; both give better performance) — they're described in the AWS `amzn-drivers` documentation and change between versions, so they're checked against the docs when building the image.

The price of DPDK: your own network stack (TCP has to come from a library or not be used), the usual tools (`tcpdump`, `ss`, `iptables`) don't see the traffic, you need your own telemetry. Because of this, an SRE on such hosts has to build monitoring on the application's own metrics and the DPDK counters.

Intermediate options without fully bypassing the kernel: **XDP** (eXpress Data Path — a small eBPF program processes a packet right in the driver, before the network stack: it can drop, redirect or pass it on; DDoS protection is built this way) and **AF_XDP** (a socket type through which XDP hands selected packets to the application bypassing the stack, while the card stays with the kernel and `ip link` still sees it).

### 🧪 Mini task

**Task.** On a Linux host find out which kernel driver currently serves the network card and where it sits on the PCI bus. That is what changes when the card is handed to DPDK.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
ethtool -i ens5 | grep -E '^(driver|bus-info)'
readlink /sys/class/net/ens5/device/driver
```

Expected output:

```text
driver: ena
bus-info: 0000:00:05.0
../../../../bus/pci/drivers/ena
```

Walkthrough: right now the card is served by the kernel driver `ena`, and packets go through the kernel network stack (interrupts, copies, sk_buff — the structure in which the kernel keeps every packet). For DPDK the card is unbound from `ena` and bound to `vfio-pci` (`dpdk-devbind.py --bind=vfio-pci 0000:00:05.0`). After that the `ens5` interface disappears from `ip link`: the kernel no longer sees it, and the application polls the card's queues itself in user space. That's why DPDK needs a separate card or a second ENI — otherwise you lose access to the machine. The values in the output depend on the machine — what matters is not the exact answer but being able to read it.

</details>

## EC2 for low latency

### The instance's network
- **ENA (Elastic Network Adapter)** — the network card of Nitro instances (Nitro is the AWS hardware/software platform all modern EC2 types run on: networking and disks are served by separate dedicated cards rather than the host CPU), with "enhanced networking" (accelerated networking without emulation in the hypervisor) on by default for modern types. Driver `ena`, check: `ethtool -i eth0` → `driver: ena`.
- **Network-optimized instances** (with the letter `n`: `c6in`, `c7gn`, `m6in`) — up to 200 Gbit/s and more packets per second. Used for load balancers, proxies, traffic processing.
- **Cluster placement group** — telling AWS to place instances on nearby hardware in one AZ (Availability Zone — a separate data centre or group of data centres inside a region): lower latency and higher speed between them. The price: all instances are in one zone, and if it fails they all suffer.
- **ENA Express** — the AWS SRD protocol (Scalable Reliable Datagram — AWS's own transport that spreads the packets of one flow over several paths in the network and quickly resends lost ones) on top of ENA on supported instances in one AZ: higher single-flow speed and more stable tail latency. It's enabled on the ENI.

### EC2's invisible limits
Every instance has limits not only on bandwidth but also on **packets per second**, the number of tracked connections (conntrack — the connection table a Security Group keeps to let reply traffic through) and requests to local services at link-local addresses `169.254.x.x` (the VPC DNS resolver, IMDS — the instance metadata service temporary IAM credentials come from, and the time server). When exceeded, packets are **silently dropped** — from outside it looks like random timeouts and tails. The driver shows the counters:
```bash
ethtool -S eth0 | grep allowance
# → bw_in_allowance_exceeded: 0
# → bw_out_allowance_exceeded: 0
# → pps_allowance_exceeded: 1532        ← we hit the packets-per-second limit
# → conntrack_allowance_exceeded: 0
# → linklocal_allowance_exceeded: 0     ← too many requests to DNS/IMDS/NTP on link-local addresses
```
These counters must be exported to monitoring (node_exporter — the standard Prometheus agent that exposes host metrics — has an ethtool collector, enabled with the `--collector.ethtool` flag). A growing `pps_allowance_exceeded` is a reason to take a bigger or network-optimized instance, `conntrack_allowance_exceeded` — to reduce the number of connections or remove connection tracking in the Security Group (if the rules allow all traffic in both directions, conntrack isn't needed), `linklocal_allowance_exceeded` — to cache DNS locally.

### Time
To measure latency between hosts you need accurate time: if two servers' clocks differ by 2 ms, you can't measure a 1 ms latency between them. **Amazon Time Sync Service** is available on every instance at `169.254.169.123` and is used via `chrony` (the standard time synchronisation daemon in Linux, a replacement for `ntpd`). Some instances have a hardware **PTP** clock (Precision Time Protocol — a protocol for precise time synchronisation over the network) via ENA (`/dev/ptp0`) — microsecond accuracy.

### Hardware accelerators: FPGAs on AWS
An **FPGA** is a chip whose logic can be "reflashed" for a task: for example, processing data as a stream at network speed with microsecond latency. In AWS these are the **F** family instances (F2 is the current generation).

The main concepts:
- **AFI (Amazon FPGA Image)** — the FPGA "firmware" (a description of the logic the chip will execute). Hardware logic developers build it, register it with `aws ec2 create-fpga-image` and get a global identifier `agfi-...` (by which the firmware is loaded on any F instance).
- **Loading an AFI into the FPGA** on the instance — with utilities from the AWS FPGA SDK (`aws-fpga`):
  ```bash
  sudo fpga-load-local-image -S 0 -I agfi-0123456789abcdef0   # slot 0
  sudo fpga-describe-local-image -S 0 -H                      # what's loaded and in what state
  ```
- **A driver** for exchanging data between the application and the FPGA (over PCIe, usually DMA — Direct Memory Access, where the device reads and writes RAM itself without the CPU).

What here is the SRE's job rather than the FPGA developers':
- **an AMI with pinned versions** of the kernel, the driver and the SDK: a kernel update can break the driver module;
- **a systemd unit** that loads the needed AFI at boot and checks its state **before** the application starts (`Before=runtime.service`, `RequiredBy=runtime.service`);
- **health checking** of the accelerator in monitoring and in the host's health check;
- **compatibility:** which AFI version works with which application version — this is part of the image or release version;
- **capacity:** F instances aren't available in every region and zone, which has to be taken into account in planning and DR (Disaster Recovery — the plan for recovering after an outage: where to move if a region or zone is unavailable).

### 🧪 Mini task

**Task.** Use the AWS CLI to compare two instance types by network performance, ENA support and vCPU count to choose a host for a latency-sensitive service.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws ec2 describe-instance-types --instance-types c6i.large c7gn.large \
  --query 'InstanceTypes[].[InstanceType, VCpuInfo.DefaultVCpus, NetworkInfo.NetworkPerformance, NetworkInfo.EnaSupport]' \
  --output text | sort
```

Expected output:

```text
c6i.large	2	Up to 12.5 Gigabit	required
c7gn.large	2	Up to 30 Gigabit	required
```

Breakdown: "Up to" means burst: small instances get the full bandwidth only briefly, then drop to the baseline — bad for stable latency. Families with an `n` (`c7gn`) are built for networking. ENA is required on modern types. For minimal latency between instances, put them in a cluster placement group in one AZ, and for full control over the CPU take `.metal` or large sizes without neighbours. You need configured AWS credentials (`aws sts get-caller-identity`).

</details>

## Tying it all together: a checklist for a latency-sensitive host

| Layer | What's configured | How to check |
|---|---|---|
| Cores | system 0–1, the application 2–N; `isolcpus`, `nohz_full`, `rcu_nocbs` | `/proc/cmdline`, `/sys/devices/system/cpu/isolated` |
| Frequency | the performance governor, no deep C-states (where available) | `cpupower frequency-info`, `cpupower idle-info` |
| NUMA | the application, memory and NIC on one node; auto-balancing off | `numastat -p`, `sysctl kernel.numa_balancing` |
| Memory | static hugepages; THP madvise/never; no swap | `/proc/meminfo`, `/sys/kernel/mm/transparent_hugepage/enabled` |
| Interrupts | NIC queues on the system cores of their node; irqbalance doesn't touch the isolated ones | `/proc/interrupts`, `smp_affinity_list` |
| NIC | the number of queues, buffers, coalescing tuned for latency | `ethtool -l/-g/-c` |
| EC2 | a network-optimized type, a cluster placement group, allowance monitoring | `ethtool -S eth0 | grep allowance` |
| Measurement | runqlat, softnet_stat, the application's P99 before and after | a dashboard + a constant-rate load test |

All of this is described in an Ansible role, baked into the AMI and checked in the pipeline — then every new host is guaranteed to be the same.

### 🧪 Mini task

**Task.** Write a checklist script that prints the host's state in one go: governor, THP, swappiness, irqbalance, core isolation. Such output is handy to attach to a ticket "before" and "after" tuning.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
bash <<'EOF'
check() { printf '%-12s %s\n' "$1" "$2"; }
check governor   "$(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor 2>/dev/null || echo n/a)"
check thp        "$(grep -o '\[[a-z]*\]' /sys/kernel/mm/transparent_hugepage/enabled)"
check swappiness "$(cat /proc/sys/vm/swappiness)"
check irqbalance "$(systemctl is-active irqbalance 2>/dev/null)"
check isolcpus   "$(grep -o 'isolcpus=[^ ]*' /proc/cmdline || echo none)"
EOF
```

Expected output:

```text
governor     n/a
thp          [madvise]
swappiness   60
irqbalance   active
isolcpus     none
```

Breakdown: tuning without recording the starting state can be neither verified nor rolled back. This script captures the "before"; after the changes you run it again and compare. For a latency-sensitive host the target is usually: governor `performance` (if available), THP `never`, swappiness 1–10, irqbalance off with interrupts placed by hand, `isolcpus`/`nohz_full` for the app cores. In production the same check runs in Ansible or the AMI pipeline so every host is guaranteed to be configured identically. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## Common host tuning mistakes

- **Tuning without measuring** — "we enabled everything from the article", and the tail grew because, for example, busy polling took a core the application needed.
- **Isolating cores and not pinning the application to them** — the isolated cores sit idle while the application is crammed onto the two system ones.
- **Forgetting about irqbalance** — manual interrupt pinning resets "by itself" a minute later.
- **Giving DPDK the primary interface on EC2** — losing access to the instance.
- **Looking only at the average** and at an hourly P99 — tail spikes drown in the aggregation.
- **Not monitoring the ENA allowance counters** — packet losses on EC2 limits look like "the network sometimes lags".

### 🧪 Mini task

**Task.** On a Linux VM change `net.core.somaxconn` with `sysctl -w` and make sure the change won't survive a reboot. Then persist it in `/etc/sysctl.d/` and apply all settings files.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
sysctl net.core.somaxconn
sudo sysctl -w net.core.somaxconn=8192
grep -rh somaxconn /etc/sysctl.conf /etc/sysctl.d/ 2>/dev/null || echo "not persisted"
echo 'net.core.somaxconn = 8192' | sudo tee /etc/sysctl.d/90-tuning.conf > /dev/null
sudo sysctl --system | grep somaxconn
```

Expected output:

```text
net.core.somaxconn = 4096
net.core.somaxconn = 8192
not persisted
net.core.somaxconn = 8192
```

Breakdown: `sysctl -w` changes the value only in the running kernel — after a reboot it's back to 4096 and the "tuning" silently disappears. It's one of the most common mistakes, along with changing things without measuring before and after and copying someone else's settings without understanding them. A file in `/etc/sysctl.d/` is applied at boot, and `sysctl --system` rereads all files at once and shows the values it set. The number in the file name sets the order: a higher number overrides a lower one. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## systemd: managing services

systemd is PID 1 in modern distributions (Ubuntu, RHEL, Amazon Linux, Debian). It starts services in the right order, restarts crashed ones, collects their logs (journald — the systemd logging service: everything a service writes to stdout/stderr goes into a shared journal with timestamps and the service name), manages timers (a replacement for cron — the classic scheduler of tasks on a schedule), mounts and cgroups (CPU and memory limits for each service).

### Core commands
```bash
systemctl status nginx           # state, PID, the last log lines — the first thing to look at
systemctl start|stop|restart nginx
systemctl reload nginx           # reload the config without stopping (if the service supports it)
systemctl enable --now nginx     # enable start at boot AND start now
systemctl disable nginx
systemctl is-active nginx; systemctl is-enabled nginx   # for scripts
systemctl list-units --type=service --failed            # what has failed
systemctl cat nginx              # show the unit file (with all overrides)
systemctl edit nginx             # create a drop-in override (without touching the package's file)
systemctl daemon-reload          # reload unit files after a change
systemd-analyze blame            # what took the longest to start at boot
```

### A unit file for your own service
```ini
# /etc/systemd/system/myapp.service
[Unit]
Description=My API service
After=network-online.target           # start after the network is up
Wants=network-online.target

[Service]
Type=simple                           # the ExecStart process is the service (it doesn't fork)
User=myapp                            # not as root
WorkingDirectory=/opt/myapp
EnvironmentFile=/etc/myapp/env        # environment variables from a file
ExecStart=/opt/myapp/bin/myapp --port 8080
Restart=on-failure                    # restart if it crashed with an error
RestartSec=5
LimitNOFILE=65536                     # the open files limit
# hardening:
NoNewPrivileges=true
ProtectSystem=strict                  # the filesystem is read-only...
ReadWritePaths=/var/lib/myapp         # ...except this folder

[Install]
WantedBy=multi-user.target            # which target the autostart (enable) is attached to
```
```bash
sudo systemctl daemon-reload && sudo systemctl enable --now myapp
```
**Why a drop-in instead of editing the file.** Packages' unit files live in `/lib/systemd/system/` and are overwritten when the package is updated. `systemctl edit nginx` creates `/etc/systemd/system/nginx.service.d/override.conf`, where you can override, for example, `LimitNOFILE`, and an update won't erase it.

### Logs: journalctl
```bash
journalctl -u nginx              # the service's logs
journalctl -u nginx -f           # in real time (like tail -f)
journalctl -u nginx --since "1 hour ago" -p err   # for the last hour, errors and worse only
journalctl -b -1                 # logs of the previous boot (why did the server go down?)
journalctl -k                    # kernel messages (like dmesg)
journalctl --disk-usage
sudo journalctl --vacuum-size=500M   # limit the journal size
```

### Timers instead of cron
cron: `crontab -e`, the format is `minute hour day month weekday command`, for example `0 3 * * * /opt/backup.sh` — every day at 03:00. The downsides: output is lost (or goes to mail), there are no dependencies, and it's hard to tell whether the task ran.

A systemd timer: a pair of `backup.service` (what to do) + `backup.timer` (when). Logs go to journald, `systemctl list-timers` shows the last and next run times, and `Persistent=true` runs a missed run if the server was off.

### Sandboxing and resources of a service in systemd
The unit file example above already has a couple of security settings. On production hosts they are made fuller: systemd can lock a service into a "sandbox" without containers, using kernel features — namespaces (the service sees its own separate view of the FS, /tmp, devices), seccomp (a system call filter: a forbidden call kills the process) and capabilities (separate pieces of root's rights). The point: if a vulnerability is found in the service, the attacker ends up in a cage and can't read other files, load kernel modules or open arbitrary sockets.
```ini
# /etc/systemd/system/runtime.service.d/hardening.conf
[Service]
User=runtime                          # or DynamicUser=yes — a temporary user created for the service's lifetime
NoNewPrivileges=yes                   # the process and its children can't gain more rights (setuid programs like sudo won't elevate)
ProtectSystem=strict                  # the whole filesystem is read-only
ReadWritePaths=/var/lib/runtime /var/log/runtime
ProtectHome=yes                       # /home, /root are inaccessible
PrivateTmp=yes                        # its own /tmp, not shared with other services
ProtectKernelTunables=yes             # /proc/sys, /sys are read-only
ProtectKernelModules=yes              # kernel modules can't be loaded
ProtectControlGroups=yes
RestrictNamespaces=yes
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX   # only these socket types
SystemCallArchitectures=native
SystemCallFilter=@system-service      # allow only the system calls of an ordinary service (seccomp)
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
AmbientCapabilities=CAP_NET_BIND_SERVICE   # listen on a port <1024 without root
LockPersonality=yes
```
To check how well a service is protected: `systemd-analyze security runtime.service` — for each parameter it shows whether it's enabled, plus an overall score from 0 (good) to 10 (UNSAFE).

Where to be careful:
- `MemoryDenyWriteExecute=yes` breaks applications with a JIT (Just-In-Time compilation — the program generates machine code in memory on the fly and executes it: Java, Node.js, LuaJIT), because it forbids executing memory that was written to;
- `PrivateDevices=yes` hides devices — if a service needs `/dev/vfio/*` (DPDK) or an accelerator device, specific ones are allowed via `DeviceAllow=` instead;
- `SystemCallFilter` — after enabling it, test the service under load: a forbidden call kills the process (the journal will show `SIGSYS`).

**Resources and placement** also belong in the unit file, not in startup scripts:
```ini
[Service]
CPUAffinity=2-7                # which cores to run on
NUMAPolicy=bind                # memory only from NUMA node...
NUMAMask=0                     # ...number 0
LimitMEMLOCK=infinity          # memory may be locked (hugepages, mlock) — needed by DPDK and latency-sensitive applications
LimitNOFILE=1048576
MemoryMax=8G                   # a hard cgroup memory limit: when exceeded — OOM inside the service, not on the whole host
TasksMax=4096
OOMScoreAdjust=-500            # the OOM killer will pick this process later than others
```
Monitoring and logging agents are meanwhile restricted to the system cores (`CPUAffinity=0-1`, `CPUQuota=50%` — no more than half of one core in total) so they don't take CPU from the application. More on core placement in the [CPU: frequency, sleep, core isolation](#06-linux.md/s10) section of this topic.

### 🧪 Mini task

**Task.** On a Linux VM create a `hello.service` unit with `Restart=on-failure`, start it, kill its process with `SIGKILL` and check that systemd restarted the service. Look at the last journal lines.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
sudo tee /etc/systemd/system/hello.service > /dev/null <<'EOF'
[Unit]
Description=Hello loop

[Service]
ExecStart=/bin/bash -c 'while true; do echo hello; sleep 5; done'
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now hello
sudo systemctl kill -s KILL hello; sleep 3
systemctl show hello -p ActiveState -p NRestarts
journalctl -u hello -n 3 --no-pager -o cat
```

Expected output:

```text
ActiveState=active
NRestarts=1
hello.service: Main process exited, code=killed, status=9/KILL
hello.service: Failed with result 'signal'.
hello.service: Scheduled restart job, restart counter is at 1.
```

Breakdown: `daemon-reload` is needed after every unit file change, otherwise systemd keeps the old version. `enable --now` both enables start at boot and starts the service immediately. The process was killed with KILL, systemd saw an abnormal exit and, per `Restart=on-failure`, restarted it after 2 seconds — `NRestarts=1`. The journal explains what happened. Had the process exited on its own with code 0, there would be no restart; that's what `Restart=always` is for. To remove the test service: `sudo systemctl disable --now hello`.

</details>

## Users, sudo, SSH

```bash
id alex                          # UID, GID and groups
useradd -m -s /bin/bash deploy   # create a user with a home directory
usermod -aG docker deploy        # ADD to a group (-a is mandatory, otherwise the groups are overwritten!)
passwd deploy
sudo -l                          # what I'm allowed to do via sudo
visudo                           # edit /etc/sudoers with a syntax check (an error in sudoers = losing sudo)
```
Files: `/etc/passwd` (users; there's no password in it despite the name), `/etc/shadow` (password hashes, readable only by root), `/etc/group`, `/etc/sudoers` and `/etc/sudoers.d/`.

**SSH:**
```bash
ssh-keygen -t ed25519 -C "alex@laptop"     # create a key pair
ssh-copy-id deploy@server                  # put the public key into ~/.ssh/authorized_keys on the server
ssh -J bastion.example.com deploy@10.0.1.5 # via a jump host (bastion)
ssh -L 5432:db.internal:5432 bastion       # a tunnel: localhost:5432 → db.internal:5432 via the bastion
ssh -v user@host                           # verbose output — debugging "it won't let me in"
```
Permissions are mandatory: `~/.ssh` — 700, `authorized_keys` and the private key — 600. Otherwise sshd silently ignores the key. Hardening `sshd_config`: `PasswordAuthentication no`, `PermitRootLogin no`. In AWS, instead of SSH and a bastion it's better to use **SSM Session Manager** (an AWS Systems Manager service: an agent on the instance makes an outbound connection to AWS itself, and you open a terminal from the console or with `aws ssm start-session`): no need to open port 22 or hand out keys, access via IAM (AWS policies decide who may connect), sessions are logged.

### SSH tunnels and jump hosts
Servers in internal networks aren't reachable directly — only via a **bastion (jump host)**. And internal web interfaces (Jenkins, Grafana, Kubernetes Dashboard, databases) often listen only on the server's `localhost`. SSH can handle both.

**Connecting via a jump host** (`-J`):
```bash
ssh -J jump.example.com:22 deploy@10.0.5.20 -p 2222 -i ~/.ssh/dmz_key
# SSH first connects to jump.example.com, and from there to 10.0.5.20:2222
ssh -J user@jump1,user@jump2 user@target    # a chain of several jump hosts
```
The key isn't copied to the jump host: the connection to the target goes through it like a tunnel, and authentication to the target happens from your machine. This is safer than the outdated `ssh -A` (agent forwarding — forwarding your ssh-agent to the server so you can hop further with your keys from it), where the bastion's administrator, while you're connected, can use your agent and log in anywhere as you.

**Local port forwarding** (`-L`) is the most common case: open a port on your side that leads to a service "as the server sees it":
```bash
ssh -L 8080:localhost:8080 ec2-user@bastion -i ~/.ssh/key.pem
# format: -L <local_port>:<address as the server sees it>:<port>
# you open http://localhost:8080 locally → traffic goes over SSH to the bastion → the bastion connects to its own localhost:8080

ssh -L 5432:db.internal:5432 ec2-user@bastion     # a database in the internal network: psql -h localhost on your machine
ssh -L 8099:localhost:8099 -L 8088:localhost:8088 user@server   # several ports in one command
ssh -fN -L 5432:db.internal:5432 bastion          # -f — go to the background after connecting, -N — no shell, just the tunnel
```

**Dynamic forwarding — a SOCKS proxy** (`-D`; SOCKS is a generic proxy protocol: the client tells the proxy "connect me to such-and-such address", and the proxy makes the connection on its own behalf):
```bash
ssh -D 5555 -fN user@jump.example.com
# local port 5555 becomes a SOCKS proxy: everything going through it exits to the network as the jump host
curl --socks5-hostname localhost:5555 http://intranet.local/   # -hostname: DNS is resolved on the server side too
```
Handy when the internal network has many services: point the browser at the SOCKS proxy instead of a dozen `-L`s.

**Reverse forwarding** (`-R`) — the other way round: open a port **on the server** that leads to you: `ssh -R 9000:localhost:3000 server` — on the server `localhost:9000` reaches your local `:3000` (to show local development, to receive a webhook).

**`~/.ssh/config`** — so you don't have to type long commands:
```
Host bastion
    HostName jump.example.com
    User deploy
    IdentityFile ~/.ssh/dmz_key

Host 10.0.*
    ProxyJump bastion          # any 10.0.x.x host — automatically via the bastion
    User ec2-user

Host grafana-tunnel
    HostName bastion.example.com
    LocalForward 3000 grafana.internal:3000
```
After that `ssh 10.0.5.20` or `ssh -fN grafana-tunnel` is enough.

### 🧪 Mini task

**Task.** Write an `ssh_config` with a bastion host and a database server behind it (`ProxyJump`), then, without connecting, use `ssh -G` to find out the final settings OpenSSH will apply to `prod-db`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > cfg <<'EOF'
Host bastion
  HostName 203.0.113.10
  User ec2-user

Host prod-*
  User ubuntu
  IdentityFile ~/.ssh/prod_ed25519
  ProxyJump bastion

Host prod-db
  HostName 10.0.5.10

Host *
  ServerAliveInterval 30
EOF
ssh -G -F cfg prod-db | grep -E '^(hostname|user|port|proxyjump|identityfile|serveraliveinterval) ' | sort
```

Expected output:

```text
hostname 10.0.5.10
identityfile ~/.ssh/prod_ed25519
port 22
proxyjump bastion
serveraliveinterval 30
user ubuntu
```

Breakdown: OpenSSH merges settings from all matching `Host` blocks, and for each setting the *first* value found wins. That's why generic `Host *` settings go at the end of the file and specific ones at the top. `prod-db` matched `prod-*`, its own block and `*`. `ssh prod-db` connects to the bastion first and through it to `10.0.5.10`, without copying the key to the bastion, unlike `ForwardAgent`. `ssh -G` is the best way to debug a config without connecting. Permissions: `~/.ssh` is 700, the private key 600, otherwise ssh rejects it.

</details>

## Server hardening

**Hardening** means reducing the attack surface: everything that isn't needed is turned off or removed, and what is needed runs with minimal privileges. It's not done by hand on every server but in code (an Ansible role) and baked into the image, so that all hosts are identical.

**Where to start — what is open and running at all:**
```bash
ss -tulpn                                    # which ports are listening and by which processes
systemctl list-unit-files --state=enabled    # what starts at boot
find / -xdev -perm -4000 -type f 2>/dev/null # files with SUID — they run with the owner's privileges (often root)
find / -xdev -type d -perm -0002 ! -perm -1000 2>/dev/null   # world-writable directories without the sticky bit
```

**The main measures:**
- **A minimum of packages and services.** No compiler, extra interpreters, sample configs or unnecessary daemons. Every package is a potential vulnerability.
- **SSH:** key-only login, no root, restricted by group:
  ```text
  # /etc/ssh/sshd_config.d/10-hardening.conf
  PasswordAuthentication no
  KbdInteractiveAuthentication no
  PermitRootLogin no
  AllowGroups ssh-users
  MaxAuthTries 3
  X11Forwarding no
  ```
  Check before restarting: `sudo sshd -t`. Even better — **don't open SSH at all** and log in via AWS SSM Session Manager: no open port, access via IAM, sessions are recorded in a log.
- **sudo** — only for the groups that need it, no `NOPASSWD: ALL` for people. Service users get specific commands.
- **A host firewall** (nftables — the modern packet filtering subsystem in the Linux kernel, or its predecessor iptables) — incoming is denied by default, only what's needed is allowed. In AWS — plus Security Groups (a firewall at the level of the instance's network interface). Two layers are needed so that a mistake in one doesn't open the whole server.
- **sysctl** against typical attacks:
  ```bash
  kernel.kptr_restrict = 2                 # don't show kernel addresses to unprivileged users (makes kernel exploits harder)
  kernel.dmesg_restrict = 1                # dmesg only for root
  kernel.unprivileged_bpf_disabled = 1     # forbid eBPF for ordinary users
  kernel.yama.ptrace_scope = 1             # ptrace (attaching to another process, as a debugger does) only to your own children
  net.ipv4.conf.all.rp_filter = 1          # drop packets with a spoofed source address (the reply would leave via the wrong interface)
  net.ipv4.conf.all.accept_redirects = 0   # don't accept ICMP redirect — "go via another gateway" (a route could be spoofed)
  net.ipv4.conf.all.send_redirects = 0     # the server isn't a router — don't send redirects
  net.ipv4.conf.all.accept_source_route = 0   # don't accept packets where the sender sets the route itself
  ```
- **Mount** `/tmp`, `/var/tmp`, `/dev/shm` with `nodev,nosuid,noexec` (`noexec` — forbid running programs from this partition, `nosuid` — ignore SUID bits, `nodev` — forbid device files) — a downloaded exploit can't be run from there: a typical attack on a web application downloads a binary into `/tmp` and runs it.
- **Mandatory access control** (MAC, Mandatory Access Control — a kernel policy that restricts a process on top of ordinary permissions, even if it runs as root): SELinux (RHEL, Amazon Linux) or AppArmor (Ubuntu) in enforcing mode (violations are blocked, not just logged). Example: a compromised nginx under SELinux can't read `/etc/shadow` even if it gets root.
- **Auditing:** `auditd` (a kernel service that records chosen events: who opened a file, who called `sudo`) with rules on changes to critical files (`/etc/passwd`, `/etc/sudoers`, `sshd_config`) and privilege-escalating calls; logs go to external storage so an attacker can't erase them.
- **Security updates:** on "pets" (unique servers that live long and are fixed by hand, as opposed to cattle — identical hosts from an image that aren't fixed but replaced) — `unattended-upgrades` or `dnf-automatic` (automatic installation of security updates); on identical hosts from an image — regular AMI rebuilds and instance replacement.
- **Services — with separate users and the systemd sandbox** (section above).

**Standards and checking:** the CIS Benchmark (Center for Internet Security — an organisation that publishes detailed secure-configuration checklists for each distribution; auditors and SOC 2/PCI DSS requirements often refer to it) — ready-made Ansible roles, OpenSCAP profiles (a tool for automatically checking compliance with a standard), checking with `oscap xccdf eval` or `lynis audit system` (a host security scanner with recommendations), package vulnerability scanning (Amazon Inspector — an AWS service, Trivy — an open-source scanner). The check result is part of the image pipeline: a host that fails the check doesn't reach production.

### 🧪 Mini task

**Task.** On a Linux server check the effective `sshd` configuration (including all include files) for three key settings, find files with the SUID bit and check which ports listen externally.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
sudo sshd -T | grep -E '^(permitrootlogin|passwordauthentication|pubkeyauthentication) '
sudo find / -xdev -perm -4000 -type f 2>/dev/null | head -4
sudo ss -ltnp | awk 'NR > 1 {print $4, $6}'
```

Expected output:

```text
permitrootlogin no
pubkeyauthentication yes
passwordauthentication no
/usr/bin/sudo
/usr/bin/passwd
/usr/bin/su
/usr/bin/mount
0.0.0.0:22 users:(("sshd",pid=812,fd=3))
127.0.0.53%lo:53 users:(("systemd-resolve",pid=503,fd=14))
```

Breakdown: `sshd -T` prints the settings actually in effect — more reliable than reading `sshd_config`, because files in `sshd_config.d/` may override values. The desired state: root login denied, passwords off, keys only. SUID files run with the owner's rights (usually root), so the list must be short and expected, and a stray SUID is a classic privilege escalation path. `ss` shows that only SSH is open externally, while the DNS resolver listens only on `127.0.0.53`. The values depend on the machine — what matters is being able to read the output, not the exact numbers.

</details>

## Package managers
```bash
# Debian/Ubuntu
apt update && apt install -y nginx
apt list --installed | grep nginx; dpkg -L nginx (package files); dpkg -S /usr/sbin/nginx (which package owns this file)
# RHEL/Amazon Linux
dnf install -y nginx; rpm -qa | grep nginx; rpm -ql nginx; rpm -qf /usr/sbin/nginx
# Alpine
apk add --no-cache curl
```

### 🧪 Mini task

**Task.** In Ubuntu and Rocky Linux containers find out which package owns `/usr/bin/ls`, and on Ubuntu pin a package against upgrades with `apt-mark hold`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run --rm ubuntu:24.04 bash -c 'dpkg -S /usr/bin/ls; apt-mark hold coreutils; apt-mark showhold'
docker run --rm rockylinux:9 rpm -qf --qf '%{NAME}\n' /usr/bin/ls
```

Expected output:

```text
coreutils: /usr/bin/ls
coreutils set on hold.
coreutils
coreutils
```

Breakdown: `dpkg -S` (Debian/Ubuntu) and `rpm -qf` (RHEL/Rocky/Amazon Linux) answer "where does this file come from" — handy to understand what an upgrade will touch or what another package broke. `apt-mark hold` prevents a package from being upgraded by `apt upgrade`; in the RHEL family `dnf versionlock` does the same. That's how kernel, driver or Kubernetes component versions (`kubelet`, `kubeadm`) are pinned so they aren't upgraded along with everything else.

</details>

## The kernel: sysctl and container primitives

**sysctl** — kernel parameters at runtime (files in `/proc/sys/`):
```bash
sysctl net.ipv4.ip_forward                   # routing packets between interfaces (needed for Docker and K8s)
sysctl -w net.core.somaxconn=4096            # the length of the incoming connection queue
sysctl -w fs.file-max=2097152                # the open-file limit for the whole system (the per-process limit is separate, ulimit -n)
sysctl -w vm.swappiness=10
# permanently — in /etc/sysctl.d/99-custom.conf, apply with: sysctl --system
```

**Namespaces and cgroups** are the kernel mechanisms containers are built on:
- **namespaces** isolate *what a process sees*: its own PIDs (the process thinks it's PID 1), network, mount points, hostname, users.
- **cgroups** limit and account for *how many resources* a process may consume: CPU, memory, disk, the number of processes.

A container is an ordinary process in its own namespaces and cgroup. More in the Docker topic.

### 🧪 Mini task

**Task.** Run an Alpine container and look from inside at the kernel primitives it's made of: its own PID, its own hostname (UTS namespace), its own cgroup and a network sysctl inside the network namespace.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run --rm --hostname sandbox alpine sh -c '
  echo "pid=$$"
  hostname
  cat /proc/self/cgroup
  ls /proc | grep -cE "^[0-9]+$"
  sysctl net.ipv4.ip_local_port_range'
```

Expected output:

```text
pid=1
sandbox
0::/
2
net.ipv4.ip_local_port_range = 32768	60999
```

Breakdown: a container is an ordinary Linux process that the kernel gave its own namespaces. PID namespace: the shell sees itself as PID 1, and `/proc` has just two processes — itself and `ls` — although the host has hundreds. UTS namespace: its own hostname. Cgroup namespace: the process sees its cgroup as the root `/`, and CPU and memory limits are applied through it. Network namespace: its own interfaces and network sysctls, which is why they can be changed inside a pod via `securityContext.sysctls`. PID 1 in a container must handle signals and reap zombies — that's what `tini` and `docker run --init` are for.

</details>

## Interview questions

**How does a hard link differ from a symbolic one?**
A hard link is a second name of the same inode: equal to the original, the data lives as long as at least one name remains; works only within one filesystem and not for directories. A symbolic link is a separate file with the path to the target: it can point to a directory and to another filesystem, and it "dangles" if the target is deleted.

**A large file was deleted, but the disk space wasn't freed. Why, and what do you do?**
A process holds the file open: the data is freed only when the last descriptor is closed. To find it — `lsof +L1` or `lsof | grep deleted`. The fix — restart the process, ask it to reopen its files (SIGHUP) or truncate the file via `/proc/<pid>/fd/<n>`. Prevention — logrotate with `copytruncate`.

**"No space left on device", but `df -h` shows free space.**
The inodes ran out (many small files) — check with `df -i`. Other options: the space is reserved for root (5% on ext4), files hidden under a mount point.

**Why does `du` show less than `df`?**
Deleted but still open files (`df` sees them, `du` doesn't), or data lies in a directory over which another filesystem is mounted.

**What do permissions 755 and 644 mean? What does `x` on a directory give?**
755 — the owner reads, writes and executes, the rest read and execute (scripts, directories). 644 — the owner reads and writes, the rest only read (ordinary files). `x` on a directory is the right to enter it and access files inside by name.

**What are SUID, SGID and the sticky bit?**
SUID — the program runs with the permissions of the file's owner (`passwd`). SGID on a directory — new files inherit the group. The sticky bit on a directory — only a file's owner can delete it (`/tmp`).

**What is load average? What does LA 8 on 4 cores mean?**
The average number of processes running or waiting for the CPU, plus those waiting for the disk (state D), over 1, 5 and 15 minutes. 8 on 4 cores — twice as much work as there are cores: a queue. If the CPU isn't busy at the same time — the problem is disk I/O.

**How do you find what is slowing a server down?**
Using the USE method: `uptime` (the load trend) → `dmesg` (OOM, disk errors) → `vmstat 1` (CPU queue, swap, iowait, steal) → `mpstat` (one core at 100%?) → `iostat -xz` (disk await and queue) → `free -m` (available) → `sar -n DEV`/`TCP` (network, retransmits) → the specific process (`pidstat`, `top`) → `strace`/`perf`/application logs.

**`kill -9` doesn't kill a process. Why?**
The process is in state D (waiting for I/O from the kernel, for example a hung NFS): the signal is delivered only after it leaves the kernel. Or it's a zombie: it's already dead, and you need to deal with the parent.

**What is a zombie process and how do you get rid of it?**
A process that has exited and whose exit code the parent hasn't collected via `wait()`. It consumes no resources other than an entry in the process table and a PID number. It can't be killed — you need to restart or fix the parent. Then PID 1 adopts the zombie and reaps it.

**How does SIGTERM differ from SIGKILL?**
SIGTERM is a request to exit: the process can catch the signal, close connections, save data. SIGKILL is an immediate kill by the kernel and can't be caught. The right shutdown order is TERM first, wait, then KILL. That's what systemd, Docker and Kubernetes do.

**Why is there almost no free memory on the server? Is that a problem?**
Usually not: Linux uses free memory for the page cache. Look at `available` in `free -m`. It's a problem when `available` is low, swap grows (`si`/`so` in vmstat) or the OOM killer fires.

**How do you find out why a process was killed?**
`dmesg -T | grep -i "killed process"` or `journalctl -k` — the OOM killer. Exit code 137 means SIGKILL (often OOM), 143 — SIGTERM, 139 — segfault. For a service — `systemctl status` and `journalctl -u`.

**How does a process in Linux start another program?**
`fork()` creates a copy of the process, in the child `exec()` replaces the program, and the parent waits via `wait()` and gets the exit code.

**What happens when you type `ls -l` in a terminal?**
The shell reads the line, parses it and expands variables and patterns, looks for `ls` in the `PATH` directories, calls `fork`, and in the child process `execve("/usr/bin/ls", ...)`. Via the `openat` and `getdents` system calls `ls` reads the directory, via `stat` (`statx`) it gets each file's metadata, and via `write` it prints to stdout (the terminal). The shell gets the exit code via `wait` and shows the prompt.

**How do you see which process is listening on a port?**
`ss -tlpn | grep :8080` or `lsof -i :8080`.

**What is `/proc`?**
A virtual filesystem the kernel generates on the fly: information about each process (`/proc/<pid>/` — command line, environment, open files, limits) and about the system (`meminfo`, `cpuinfo`, `loadavg`), as well as tunable kernel parameters (`/proc/sys`).

**How do you make a service start at boot and restart when it crashes?**
A systemd unit file with `Restart=on-failure` and an `[Install] WantedBy=multi-user.target` section, then `systemctl daemon-reload` and `systemctl enable --now`.

**Swap — good or bad?**
A buffer against a sudden OOM, but the system degrades when it works with swap actively. On servers with a predictable load it's often limited (`swappiness`) or disabled. Kubernetes historically required swap off to guarantee memory limits.

**A user was added to the docker group, but has no access.**
Groups are applied at login: you need to log in again (or `newgrp docker`). And check that `usermod -aG` was used, not `-G` without `-a`, which replaces all the groups.

**How do you tune the Linux kernel? Which parameters have you changed?**
Kernel parameters are changed via `sysctl` (on the fly: `sysctl -w`, permanently: a file in `/etc/sysctl.d/` and `sysctl --system`). Change them only based on measurements, not "just in case". Common parameters for busy servers:
- `net.core.somaxconn` and `net.ipv4.tcp_max_syn_backlog` — the lengths of the incoming connection queues: connections already established but not yet accepted by the application, and "half-open" ones (waiting for the TCP handshake to finish). Otherwise connections get dropped at peaks;
- `net.ipv4.ip_local_port_range` — the range of ephemeral ports (temporary ports the kernel hands out for outgoing connections; about 28 thousand by default), when the server makes very many outgoing connections (a proxy, a load balancer) and hits the `Cannot assign requested address` error;
- `net.ipv4.tcp_tw_reuse` — reuse connections in TIME_WAIT (the state in which a closed connection holds the port for another 60 seconds to catch late packets) for outgoing connections;
- `fs.file-max` and the `nofile` limits — the maximum of open files;
- `vm.swappiness` — how eagerly the kernel uses swap (usually 1–10 for databases);
- `vm.max_map_count` — the maximum number of memory regions (mmap) per process; Elasticsearch and OpenSearch need it (at least 262144), otherwise they don't start;
- `net.ipv4.ip_forward` — packet routing (needed by Docker and Kubernetes);
- `net.core.rmem_max` and `wmem_max` — the maximum sizes of a socket's receive and send buffers: on 10+ Gbit/s links with high latency (between regions) a small buffer keeps a single connection from speeding up.
Plus the choice of I/O scheduler (the kernel algorithm that decides in which order to send requests to the disk; usually `none` for NVMe), transparent huge pages (many databases recommend turning them off), the CPU governor. In Kubernetes some sysctls are set in the pod's `securityContext.sysctls` (the safe, namespaced ones), and the rest at the node level (user data — a script run on the instance's first boot, or a DaemonSet — a pod Kubernetes runs on every node).

**How do you expose an internal service through a bastion?**
Local port forwarding: `ssh -L 5432:db.internal:5432 user@bastion` — then connect to `localhost:5432`. For many services — a SOCKS proxy `ssh -D 5555 user@bastion`. Connecting to the internal hosts themselves — `ssh -J bastion user@10.0.5.20` or `ProxyJump` in `~/.ssh/config`. In AWS — SSM Session Manager with port forwarding instead of an open SSH.

**How would you harden a new Linux host?**
I'd describe it in code (an Ansible role, baked into the image): a minimum of packages and services, key-only SSH without root or login via SSM, sudo by group, a firewall denying incoming traffic by default, sysctl against address spoofing and kernel information leaks, `noexec,nosuid,nodev` for `/tmp`, SELinux/AppArmor in enforcing mode, auditd shipping logs out, services under their own users with the systemd sandbox. Verification against CIS via OpenSCAP or Lynis and a vulnerability scan in the pipeline.

**How do you run a service with minimal privileges using systemd?**
A dedicated `User=` or `DynamicUser=yes`, `NoNewPrivileges`, `ProtectSystem=strict` with explicit `ReadWritePaths`, `ProtectHome`, `PrivateTmp`, `ProtectKernel*`, `RestrictAddressFamilies`, `SystemCallFilter=@system-service`, capabilities via `CapabilityBoundingSet`/`AmbientCapabilities` instead of root. Assess with `systemd-analyze security <unit>`. Test under load: an unnecessary restriction kills the process with SIGSYS or breaks a JIT.

**How do you restrict which cores and NUMA node a service runs on?**
In the unit file: `CPUAffinity=`, `NUMAPolicy=bind` with `NUMAMask=`, or a cgroup v2 cpuset via `AllowedCPUs=`. Other services — `CPUAffinity` in `system.conf` set to the system cores. To verify: `taskset -cp <PID>`, `numastat -p <PID>`.

**Why do latency SLOs look at P99 rather than the average?**
The average hides the tail: a small share of very slow requests barely changes the average, but they're exactly what spoils users' experience and breaks the SLO. P99 and P99.9 show what the worst 1% or 0.1% of requests get. And with many requests per user operation (fan-out), almost every operation hits the tail.

**What is coordinated omission?**
A load-test error where the generator waits for a response before the next request: while the server hangs, it stops sending requests and doesn't account for those that should have arrived. The tail comes out understated. The solution is constant-rate generators (wrk2, vegeta, k6 constant-arrival-rate) and measuring from the scheduled rather than the actual send time.

**Where do you start if P99 grew from 3 to 12 ms while the average latency didn't change?**
I'd correlate it in time with changes (a release, a new AMI, load growth). On the host: are processes waiting for CPU (`runqlat`, `vmstat r`, `pidstat -w` preemptions), have interrupts landed on the application's cores (`mpstat`, `/proc/interrupts`), are there drops and retransmits (`softnet_stat`, `nstat`, `tcpretrans`), are we hitting EC2 limits (`ethtool -S | grep allowance`), are there pauses from THP or GC (garbage collection in Java, Go, .NET, during which the application may stop). I'd compare the problem host with a healthy one.

**What do isolcpus and nohz_full give you?**
`isolcpus` removes cores from ordinary scheduling: random processes won't get there, only explicitly pinned ones. `nohz_full` disables the periodic timer interrupt on a core running a single task, so that the kernel doesn't interrupt it. Together with `rcu_nocbs` and pinning interrupts to other cores, this gives the application almost interference-free cores. The application then has to be pinned explicitly via `taskset` or `CPUAffinity`.

**What is NUMA and how does it affect latency?**
In a multi-socket server each processor has its own memory and its own devices, and access to another node's memory and devices is slower. If the application runs on node 0 while its memory or network card is on node 1, every access costs more. The solution is to pin the application, its memory and the network card's interrupts to one node (`numactl`, `NUMAPolicy` in systemd) and check with `numastat`.

**How do hugepages differ from transparent huge pages and why is THP often disabled?**
Static hugepages are reserved in advance and handed to the application explicitly (DPDK, databases): predictable, with no background kernel work. THP is created by the kernel itself on the fly; for that the background `khugepaged` and memory compaction move pages around, which causes unpredictable pauses. So on hosts with strict latency THP is set to `madvise` or `never`.

**What is interrupt coalescing and how does it affect latency?**
The network card delays the interrupt until it has accumulated several packets or a set time has passed. This lowers the CPU load and increases throughput, but every packet waits longer. For low latency coalescing is reduced or turned off (`ethtool -C`), accepting that the number of interrupts will grow.

**Why pin interrupts and what does irqbalance do?**
So that packet processing happens on specific cores: on the same NUMA node as the card, and not on the cores of the application's hot threads. irqbalance automatically spreads interrupts across cores and will overwrite manual pinning, so it's turned off or forbidden the isolated cores.

**What is DPDK and what are its downsides?**
A library for processing packets in user space, bypassing the kernel: a poll mode driver (PMD), hugepages, pinned cores. It gives microsecond latencies and millions of packets per second. The downsides: the cores are 100% busy polling, there's no standard network stack or ordinary tools (tcpdump, iptables, ss), you need your own telemetry, operations are harder. On EC2 a separate ENI is dedicated to DPDK so as not to lose access to the instance.

**How do you tell that EC2 is dropping packets because of its limits?**
The ENA driver's counters: `ethtool -S eth0 | grep allowance` — `pps_allowance_exceeded`, `bw_in/out_allowance_exceeded`, `conntrack_allowance_exceeded`, `linklocal_allowance_exceeded`. Their growth means silent losses on the instance's limits. Solutions: a bigger or network-optimized instance type, fewer connections or no connection tracking, a local DNS cache.

**How do you operate hosts with FPGAs?**
Pin the kernel, driver and SDK versions in the AMI; load the needed AFI with a systemd unit at startup and check its state before the application starts; include the accelerator's state in the health check and monitoring; keep a mapping of AFI and application versions; take into account the limited availability of F instances across regions and zones.
