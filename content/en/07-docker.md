# Docker / AMI images

Containers in depth: what they are at the Linux kernel level, how images and layers are structured, how the build cache works, multi-stage and BuildKit, networking, storage and security, plus how to go from a Docker image to a machine image (AMI) and what to build it with: Packer and EC2 Image Builder. Each topic: what it is, why, how it works, an example with explanations and typical mistakes.

The second part is about machine images: from a Docker image to an AMI, Packer, EC2 Image Builder and the full AMI lifecycle — from building and checks to rollout, rollback and deletion.

## What problem containers solve

Before containers, an application was installed on a server: the right version of .NET or Python, system libraries, configs. Hence the classic problems:
- "**it works on my machine**" — the server has different library versions;
- **dependency conflicts** — two applications on one server need different versions of the same library;
- **configuration drift** — servers configured by hand at different times differ from each other;
- **long and fragile installation** — instructions with dozens of steps.

A container packages the application **together with its whole environment** (libraries, runtime, files) into an **image** — an immutable artifact that runs the same way on a laptop, in CI and in production. Build it once — run it anywhere there's a container runtime.

### Container versus virtual machine
| | Virtual machine | Container |
|---|---|---|
| What it isolates | A whole computer: its own OS kernel on top of a hypervisor | A process: the kernel is shared with the host |
| Size | Gigabytes (a full OS) | Megabytes (only the application and its dependencies) |
| Startup | Tens of seconds to minutes (OS boot) | Milliseconds to seconds (starting a process) |
| Density | Dozens of VMs per server | Hundreds of containers per server |
| Isolation | Strong (a guest kernel vulnerability doesn't affect others) | Weaker (a shared kernel: a kernel vulnerability affects all containers) |

A consequence of the shared kernel: you can't run a Windows container on a Linux host, and Docker Desktop on Mac and Windows actually runs a small Linux VM in which the containers work.

### 🧪 Mini task

**Task.** Without installing anything on the host, run the same Python line under two different interpreter versions — 3.9 and 3.13 — and make sure they don't interfere with each other.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
for v in 3.9 3.13; do
  docker run --rm python:$v-slim python -c 'import sys; print(sys.version.split()[0])'
done
```

Expected output:

```text
3.9.23
3.13.7
```

Breakdown: each image carries its own Python version and all dependencies, the host doesn't change, and two environments live side by side without conflicts. That's the problem containers solve: "works on my machine" becomes "works anywhere the image runs". The patch versions depend on when the image was pulled, so in production images are pinned by an exact version tag or by digest.

</details>

## What a container is at the kernel level

A container **is an ordinary Linux process**. There's no "box". The kernel simply starts the process with special settings.

**1. Namespaces — what the process sees.** The kernel gives the process its own "picture of the world":

| Namespace | What it isolates | The effect in a container |
|---|---|---|
| `pid` | The process tree | The application sees itself as PID 1 and doesn't see the host's processes |
| `net` | Network interfaces, IPs, routes, ports, iptables | The container has its own IP and its own port 80 |
| `mnt` | Mount points | Its own root filesystem from the image |
| `uts` | The hostname | Its own hostname |
| `ipc` | Inter-process communication (shared memory) | |
| `user` | Users and groups | root inside can be an ordinary user on the host (rootless — a mode in which both Docker itself and the containers run without root on the host: escaping the container doesn't give an attacker root) |
| `cgroup` | Visibility of the cgroups hierarchy | |

**2. cgroups — how many resources the process may consume.** Limits and accounting: memory (`memory.max` — when exceeded, the OOM killer kills the process), CPU (`cpu.max` — a quota; when exceeded, the process is slowed down, which is called throttling), the number of processes (protection against a fork bomb — a process that multiplies endlessly and eats all process slots on the host), disk I/O. This is exactly what the `docker run --memory 512m --cpus 1` flags and `resources.limits` in Kubernetes set.

**3. A root file system from the image** — the image layers are glued together via overlayfs, and the process gets them as its `/` (the `pivot_root` system call changes the process's file system root — after it the process can't see the host's files).

**4. Reduced privileges** — even root in a container can't do everything by default: part of its **capabilities** are removed (separate "pieces" of root's rights, e.g. it can't load kernel modules or change the system time), and **seccomp** (a system call filter in the kernel) forbids around 40 dangerous system calls.

To make sure a container is an ordinary process:
```bash
docker run -d --name web nginx
ps -ef | grep "nginx: master"            # the process is visible on the host with an ordinary PID
PID=$(docker inspect -f '{{.State.Pid}}' web)
ls -l /proc/$PID/ns                      # its namespaces — links like net:[4026532301]
sudo nsenter -t $PID -n ip addr          # run a command in the container's network namespace
```
`nsenter` is a useful debugging trick: you can run the host's `tcpdump` or `ss` inside the container's network namespace, even if the image doesn't have these tools.

### 🧪 Mini task

**Task.** Run a container with `--cpus 0.5` and `-m 128m` limits and read, from inside, the cgroup v2 files where Docker wrote those limits.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run --rm --cpus 0.5 -m 128m alpine cat /sys/fs/cgroup/cpu.max /sys/fs/cgroup/memory.max
```

Expected output:

```text
50000 100000
134217728
```

Breakdown: there is no "virtual machine" — Docker just configured the process's cgroup. `cpu.max` means "50 ms of every 100 ms", i.e. half a core: above that the process is throttled, not killed. `memory.max` is 128 × 1024 × 1024 bytes; above it the OOM killer fires. kubelet creates exactly these files from a pod's `resources.limits`. Isolation (what the process sees) comes from namespaces, limits (how much it uses) from cgroups.

</details>

## Docker architecture
```
docker CLI ──HTTP API over /var/run/docker.sock──► dockerd ──► containerd ──► containerd-shim ──► runc ──► process
                                                      │
                                                      └─► BuildKit (image builds)
```
- **docker CLI** — the client. It doesn't run anything itself but sends requests to the daemon.
- **dockerd** — the Docker daemon: the API, builds, networks, volumes.
- **containerd** — the daemon that manages the container lifecycle: pulling images, storing layers, starting and stopping. Kubernetes uses it directly too.
- **containerd-shim** — a small parent process for each container. Thanks to it containers keep running when dockerd or containerd is restarted.
- **runc** — the low-level runtime: creates the namespaces and cgroups, starts the process and exits. Alternatives: crun, gVisor (isolation via a user-space kernel), Kata Containers (each container in a micro-VM).

**OCI (Open Container Initiative)** — open standards for the image format, container execution and the registry protocol. Thanks to them, an image built by Docker runs in containerd, CRI-O (a runtime made specifically for Kubernetes, used in OpenShift), Podman (a daemonless Docker replacement, the default in RHEL) and Kubernetes without changes.

**Kubernetes and Docker.** Kubelet talks to the runtime through the CRI interface. There used to be a dockershim layer between kubelet and Docker; it was removed in version 1.24, and now kubelet works with containerd or CRI-O directly. **Images built with Docker work as before** — they're standard OCI images. Only what depended on `docker.sock` on the nodes broke (Docker-in-Docker builds — when a CI pipeline in a pod calls `docker build` itself through the node's socket, some monitoring agents).

**`docker.sock` = root on the host.** Whoever can access the socket can start a container that mounts the host's root (`-v /:/host`) with privileges. So adding a user to the `docker` group and mounting the socket into a container means granting root access.

### 🧪 Mini task

**Task.** Find out the Docker client and server versions, the storage driver and cgroup version, and which daemon the client is currently connected to.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker version --format 'client={{.Client.Version}} server={{.Server.Version}}'
docker info --format 'driver={{.Driver}} cgroup=v{{.CgroupVersion}} os={{.OperatingSystem}}'
docker context ls --format '{{.Name}} {{.Current}} {{.DockerEndpoint}}'
```

Expected output:

```text
client=29.8.1 server=29.8.1
driver=overlayfs cgroup=v2 os=Docker Desktop
default false unix:///var/run/docker.sock
desktop-linux true unix:///Users/alex/.docker/run/docker.sock
```

Breakdown: `docker` is just a client that calls the `dockerd` daemon's API; the daemon creates containers through containerd and runc. On macOS the daemon runs inside Docker Desktop's Linux VM, so containers are Linux processes, not macOS processes. The context decides which daemon commands go to: that's how one client manages a remote host (`docker context create … --docker host=ssh://…`). Your sizes, versions and IDs will differ.

</details>

## The image: what it consists of

An image isn't a single file but a set of objects in a registry, each addressed by the hash of its content (a **digest**, `sha256:...`):
```
manifest list (index)      ← for multi-arch: "for linux/amd64 here's manifest A, for linux/arm64 — manifest B"
  └─ manifest              ← a list: which config and which layers (by digest)
       ├─ config (JSON)    ← metadata: ENV, CMD, ENTRYPOINT, USER, WORKDIR, build history
       ├─ layer 1 (tar.gz) ← filesystem changes
       ├─ layer 2 (tar.gz)
       └─ layer N
```

### Layers
**Every Dockerfile instruction that changes the filesystem (`RUN`, `COPY`, `ADD`) creates a new layer** — a tar archive with the difference from the previous state: which files were added, changed or deleted. The `ENV`, `CMD`, `EXPOSE`, `USER`, `WORKDIR`, `LABEL` instructions change only the config and don't create layers with data.

Why layers:
- **Deduplication.** Layers are addressed by hash, so identical layers are stored and downloaded once. Ten images on the same base image store the base layers only once. Hence the value of a shared base image in a company.
- **Build caching** — unchanged layers aren't rebuilt (more below).
- **Fast delivery** — when an image is updated, only the changed layers are downloaded.

### Tag and digest
- A **tag** (`myapp:1.4.2`, `myapp:latest`) is a **mutable** pointer to a manifest, like a branch in Git. The same tag may point to a different image tomorrow.
- A **digest** (`myapp@sha256:3f8a...`) is an **immutable** identifier of specific content. For reproducibility in production, pin the digest or at least immutable tags (a version number or commit SHA, not `latest`).

### How to look inside an image
```bash
docker image history --no-trunc myapp:1.0     # layers: which instruction created each, how much it weighs
docker image inspect myapp:1.0 | jq '.[0].Config'   # ENV, CMD, ENTRYPOINT, USER
docker buildx imagetools inspect nginx:1.27   # the manifest list: which platforms the image exists for
dive myapp:1.0                                # interactively: what each layer added and where the "junk" is
skopeo inspect docker://nginx:1.27            # metadata straight from the registry without pulling the image
```

### 🧪 Mini task

**Task.** Build an image with three instructions on top of Alpine, look at its history with layer sizes and count the filesystem layers.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > Dockerfile <<'EOF'
FROM alpine:3.20
RUN apk add --no-cache curl
COPY hello.txt /hello.txt
CMD ["cat", "/hello.txt"]
EOF
echo hi > hello.txt
docker build -q -t layers-demo . > /dev/null
docker history --format '{{.Size}}\t{{.CreatedBy}}' layers-demo | head -4 | cut -c1-60
docker image inspect -f '{{len .RootFS.Layers}} layers' layers-demo
```

Expected output:

```text
0B	CMD ["cat" "/hello.txt"]
12.3kB	COPY hello.txt /hello.txt # buildkit
8.15MB	RUN /bin/sh -c apk add --no-cache curl # buildki
0B	CMD ["/bin/sh"]
3 layers
```

Breakdown: an image is a stack of layers plus a config (CMD, ENV, ports). Only instructions that change files (`RUN`, `COPY`, `ADD`) create filesystem layers, hence 3 layers: base Alpine, `apk add` and `COPY`. `CMD` is 0B — it's metadata. Layers are immutable and shared between images: ten images on the same Alpine store its layer once. Hence the rule: a file deleted in a later `RUN` still stays in the earlier layer and in the image size. Your sizes, versions and IDs will differ.

</details>

## OverlayFS: how layers become a filesystem

Image layers are read-only and shared by all containers from that image. So how does a container write files? The storage driver (the Docker component responsible for how layers are stored on the host disk and assembled into the container's file system) **overlay2** glues several directories into one:
```
merged/    ← what the process in the container sees as "/" — the union of all layers
upperdir   ← the writable layer, separate for each container; deleted together with the container
lowerdir   ← the image layers, read-only, shared by all containers
```
The rules:
- **Reading** — the file is taken from the topmost layer where it exists.
- **Writing a new file** — the file is created in upperdir.
- **Changing a file from the image** — **copy-up**: the whole file is copied into upperdir and changed there. That's why the first write to a large file is slow, and databases inside a container without a volume work poorly.
- **Deleting a file from the image** — the file itself stays in the lower layer, and a special **whiteout file** is created in upperdir that hides it.

### Why deleting in a later layer doesn't shrink the image
The same applies to images: every Dockerfile instruction is a new layer on top of the previous ones.
```dockerfile
RUN curl -o /tmp/tool.tar.gz https://example.com/tool.tar.gz   # layer 1: +500 MB (the archive)
RUN tar -xzf /tmp/tool.tar.gz -C /opt                          # layer 2: +500 MB (the unpacked content)
RUN rm /tmp/tool.tar.gz                                        # layer 3: a whiteout — the archive is hidden, but it's still in layer 1
# result: the image grew by 1 GB, although the archive was "deleted"
```
The right way is to download, unpack and delete **in one instruction**, so that the archive doesn't end up in any layer at all:
```dockerfile
RUN curl -fsSL https://example.com/tool.tar.gz | tar -xz -C /opt   # +500 MB, no archive anywhere
```
The same principle for package managers: the cache must be cleaned in the same instruction as the installation:
```dockerfile
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl ca-certificates \
 && rm -rf /var/lib/apt/lists/*
```
Even better — multi-stage (below): everything temporary stays in the build stage.

### 🧪 Mini task

**Task.** In an Alpine container append a line to an existing file, delete another file from the image and create a new folder with a file. Then use `docker diff` to see what was written to the upper (writable) layer.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run --name ovl alpine sh -c 'echo hi >> /etc/motd; rm /etc/issue; mkdir /data && touch /data/new'
docker diff ovl
docker rm ovl > /dev/null
```

Expected output:

```text
C /etc
C /etc/motd
D /etc/issue
A /data
A /data/new
```

Breakdown: the image's lower layers are read-only; all changes go to the container's upper layer. `C` (changed): on the first write OverlayFS copies the whole `motd` file up (copy-up) — that's why writing to large files from the image is slow. `D` (deleted): the file isn't removed from the image but hidden by a whiteout marker. `A` (added): new files. The upper layer disappears with the container, which is why data you need to keep goes into a volume.

</details>

## The build cache: when a layer is rebuilt

During a build Docker goes through the instructions top to bottom and for each one checks: is there a layer in the cache produced **by the same instruction on top of the same parent layer**. If there is — it takes it from the cache (`CACHED` in the output). If not — it runs the instruction again.

How Docker decides that an instruction is "the same":
- **`RUN`** — the **command text** is compared. Docker doesn't know that `apt-get update` will return different packages today: if the line hasn't changed, the cache is used (possibly a week old). That's why `apt-get update` and `install` are kept in one `RUN`: otherwise `install` may use an outdated package list from a cached `update`.
- **`COPY` and `ADD`** — the **content of the copied files** (a checksum) is compared. The modification time doesn't matter. If even one byte of the copied files changed, the layer is rebuilt.
- **`ARG` and `ENV`** — changing a value invalidates the instructions that use it.

**The main rule: invalidation cascades.** If layer N is rebuilt, **all layers after it** are rebuilt too, even if they themselves didn't change (their parent changed).

Hence the rule for ordering instructions: **from rarely changing to frequently changing.** The project's dependencies change rarely, while the code changes all the time.

Bad:
```dockerfile
FROM mcr.microsoft.com/dotnet/sdk:8.0
WORKDIR /src
COPY . .                          # any code change invalidates this layer...
RUN dotnet restore                # ...and the restore of all NuGet packages (NuGet is the .NET package manager) runs again (minutes)
RUN dotnet publish -c Release -o /app
```
Good:
```dockerfile
FROM mcr.microsoft.com/dotnet/sdk:8.0
WORKDIR /src
COPY *.sln ./
COPY src/Api/Api.csproj src/Api/  # first only the files with dependencies
RUN dotnet restore                # cached until the .csproj files change
COPY . .                          # now the code: changes often, but restore is already cached
RUN dotnet publish -c Release -o /app --no-restore
```
The same for other languages: Node — `package.json` and `package-lock.json` first, `npm ci`, then the code; Python — `requirements.txt`, `pip install`, then the code; Go — `go.mod` and `go.sum`, `go mod download`, then the code.

### .dockerignore
During a build the whole context folder (usually `.`) is sent to the builder. `.dockerignore` excludes what isn't needed:
```
.git
**/bin
**/obj
node_modules
.env
*.tfstate
Dockerfile
```
Why: a smaller context — a faster build; junk (`.git`, local builds) doesn't bust the `COPY . .` cache; **secrets (`.env`) don't end up in the image.**

### Cache in CI
CI runners are often ephemeral: there's no local cache, and every build starts from scratch. The solution is to keep the cache in a registry:
```bash
docker buildx build \
  --cache-from type=registry,ref=registry.example.com/myapp:buildcache \
  --cache-to   type=registry,ref=registry.example.com/myapp:buildcache,mode=max \
  -t registry.example.com/myapp:$CI_COMMIT_SHORT_SHA --push .
```
`mode=max` also saves the layers of intermediate stages, not just the final one. In GitHub Actions `type=gha` (the Actions cache) is convenient. Local control: `--no-cache` (build without the cache), `--pull` (update the base image), `docker builder prune` (clean the cache).

### 🧪 Mini task

**Task.** Build an image where dependencies are copied and "installed" before the code. Change only `app.py` and rebuild. Which steps come from the cache?

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > Dockerfile <<'EOF'
FROM alpine:3.20
COPY requirements.txt /app/
RUN echo "installing deps" && sleep 3
COPY app.py /app/
EOF
echo 'requests==2.32.3' > requirements.txt; echo 'print(1)' > app.py
docker build -q -t cache-demo . > /dev/null
echo 'print(2)' > app.py
docker build --progress=plain -t cache-demo . 2>&1 | grep -E '\[[2-4]/4\]|CACHED' | sed 's/^#[0-9]* //'
```

Expected output:

```text
[2/4] COPY requirements.txt /app/
CACHED
[3/4] RUN echo "installing deps" && sleep 3
CACHED
[4/4] COPY app.py /app/
```

Breakdown: BuildKit walks the instructions top to bottom and checks a cache key for each: the instruction itself and, for `COPY`, the hash of the copied files. `requirements.txt` didn't change, so steps 2 and 3 came from the cache and the slow dependency install wasn't repeated. `app.py` changed, so step 4 and everything after it are rebuilt. With `COPY . /app/` at the top, any code change would invalidate the dependency install cache — the most common cause of slow builds.

</details>

## Multi-stage builds

**The problem.** To **build** an application you need an SDK, a compiler, the sources, dev dependencies. To **run** it — only the runtime and the finished artifact. If you build in a single image, everything not needed at runtime ends up in production: the image weighs a gigabyte and contains a compiler and the sources (extra vulnerabilities and extra tools for an attacker).

**The solution.** Several `FROM` instructions in one Dockerfile. Each one starts a new **stage** with a clean filesystem. **Only the last stage** (or the one given via `--target`) goes into the final image, plus whatever was explicitly copied from other stages via `COPY --from=<stage>`.

What it gives you:
- **Size.** The .NET SDK weighs about 800 MB, the aspnet runtime about 220 MB, the chiseled variant (a stripped-down Ubuntu from Canonical without a shell or package manager — only what the runtime needs) about 110 MB. A Go application in the `golang` image is about 1 GB, while a static binary in `scratch` (an empty image without a single file) is 10–20 MB.
- **Security.** In production there's no compiler, sources, package manager, sometimes not even a shell. Fewer vulnerabilities for the scanner and fewer tools for an attacker.
- **One Dockerfile** instead of scripts that build in one container and copy the artifact into another.
- **Parallel builds.** BuildKit builds a dependency graph of stages: independent ones are built in parallel, and those not needed for the target stage are skipped.

### Example: .NET with a separate test stage
```dockerfile
# syntax=docker/dockerfile:1.7
ARG DOTNET_VERSION=8.0

# --- stage 1: restoring dependencies (cached until the .csproj files change) ---
FROM mcr.microsoft.com/dotnet/sdk:${DOTNET_VERSION} AS restore
WORKDIR /src
COPY *.sln Directory.Packages.props ./
COPY src/Api/Api.csproj src/Api/
COPY tests/Api.Tests/Api.Tests.csproj tests/Api.Tests/
RUN --mount=type=cache,id=nuget,target=/root/.nuget/packages \
    dotnet restore

# --- stage 2: build ---
FROM restore AS build
COPY . .
RUN --mount=type=cache,id=nuget,target=/root/.nuget/packages \
    dotnet publish src/Api -c Release -o /app --no-restore

# --- stage 3: tests (built only when explicitly requested: --target test) ---
FROM build AS test
RUN --mount=type=cache,id=nuget,target=/root/.nuget/packages \
    dotnet test --no-restore

# --- stage 4: the final image, only the runtime and the build result ---
FROM mcr.microsoft.com/dotnet/aspnet:${DOTNET_VERSION}-noble-chiseled AS runtime
WORKDIR /app
COPY --from=build /app .             # from the build stage take only the published application
USER $APP_UID                        # an unprivileged user defined in the official .NET 8+ images
EXPOSE 8080
ENTRYPOINT ["dotnet", "Api.dll"]
```
```bash
docker build --target test .           # build up to the test stage and run the tests (CI)
docker build -t api:1.0 .              # the final image: the test stage isn't built at all
```

### Example: Go in a minimal image
```dockerfile
FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 — a static binary with no dependency on system libraries (glibc); -s -w — strip debug info, a smaller binary
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app ./cmd/app

FROM gcr.io/distroless/static:nonroot   # Google's distroless: only CA certificates, tzdata and a non-root user, no shell or packages
COPY --from=build /app /app
ENTRYPOINT ["/app"]
```
If you use `FROM scratch` (a completely empty image), you have to copy the CA certificates (`/etc/ssl/certs`) yourself, otherwise HTTPS requests from the application fail with a certificate verification error.

### Useful tricks
- `COPY --from=<any image>` — copy files from any image, not only from your own stage: `COPY --from=busybox:1.36 /bin/wget /usr/bin/wget`.
- A separate `debug` stage with a shell and tools on top of runtime — `--target debug` for debugging.
- An `ARG` before the first `FROM` sets base image versions for all stages.

### 🧪 Mini task

**Task.** Build a Go program in the `golang` image and copy only the binary into a final image based on `scratch`. Compare the sizes of the build and final images and run the result.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > main.go <<'EOF'
package main

import "fmt"

func main() { fmt.Println("hello from scratch") }
EOF
cat > Dockerfile <<'EOF'
FROM golang:1.25 AS build
WORKDIR /src
COPY main.go .
RUN CGO_ENABLED=0 go build -o /app main.go

FROM scratch
COPY --from=build /app /app
ENTRYPOINT ["/app"]
EOF
docker build -q -t ms-demo . > /dev/null
docker run --rm ms-demo
docker image ls --format '{{.Repository}} {{.Size}}' | grep -E '^(golang|ms-demo) '
```

Expected output:

```text
hello from scratch
ms-demo 2.21MB
golang 849MB
```

Breakdown: the final image contains only what was copied with `COPY --from=build`: the compiler, sources and caches stayed in the first stage. The image is hundreds of times smaller, pulls faster and has almost no attack surface — `scratch` doesn't even have a shell. `CGO_ENABLED=0` makes the binary static: `scratch` has no libc, and a dynamically linked program wouldn't start. Python and Node use the same trick: dependencies are built in the first stage and copied into a `-slim` or distroless image. Your sizes, versions and IDs will differ.

</details>

## BuildKit — the modern builder

BuildKit is enabled by default since Docker 23. Besides building stages in parallel, it provides special mounts for `RUN` — the data is available while the command runs but **doesn't end up in the layer**:

**Cache mount** — a package manager's cache lives between builds:
```dockerfile
RUN --mount=type=cache,target=/root/.cache/pip pip install -r requirements.txt
```
When `requirements.txt` changes, the layer is rebuilt, but already downloaded packages come from the cache rather than the internet.

**Secret mount** — a secret for the duration of one command:
```dockerfile
RUN --mount=type=secret,id=npmrc,target=/root/.npmrc npm ci
```
```bash
docker build --secret id=npmrc,src=$HOME/.npmrc .
```
The private registry token is available only during `npm ci` and stays neither in the layer nor in the image history. Compare with the bad options: `ARG NPM_TOKEN` is visible in `docker history`, and `COPY .npmrc` + `RUN rm .npmrc` leaves the file in the layer.

**SSH mount** — clone private repositories with a key from the host without copying the key into the image:
```dockerfile
RUN --mount=type=ssh git clone git@gitlab.com:org/private-lib.git
```
```bash
docker build --ssh default .
```

**Other features:**
- `COPY --link` — the layer doesn't depend on the previous ones and is reused when the base image changes, without a rebuild;
- heredocs in the Dockerfile (`RUN <<EOF ... EOF`) for multi-line scripts;
- **multi-platform**: `docker buildx build --platform linux/amd64,linux/arm64` — one tag for x86 and ARM (Graviton, Mac M series) via QEMU (an emulator of a foreign architecture: it works, but the build is 5–20 times slower) or native build nodes for each architecture;
- attestations: `--sbom=true` (SBOM, Software Bill of Materials — a list of all packages and libraries inside the image with versions: it lets you quickly answer "where do we have the vulnerable log4j version"), `--provenance=true` (a signed record of how and from what the image was built: the commit, the CI pipeline);
- `docker buildx bake` — declarative builds of several images from one file.

Building without the Docker daemon (for CI in Kubernetes, where privileges can't be granted): BuildKit in rootless mode, Buildah (Red Hat's daemonless builder), `ko` for Go (builds an image straight from Go code without a Dockerfile), Jib for Java (the same for Maven/Gradle). Kaniko was archived by Google; forks exist.

### 🧪 Mini task

**Task.** Pass a token to the build with `--secret` and use it in `RUN --mount=type=secret`. Check that the token was available during the build but stayed neither in the layers nor in the image history.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
echo -n 's3cr3t-token' > token.txt
cat > Dockerfile <<'EOF'
FROM alpine:3.20
RUN --mount=type=secret,id=token echo "token length: $(wc -c < /run/secrets/token)" > /build-info.txt
EOF
docker build -q --secret id=token,src=token.txt -t secret-demo . > /dev/null
docker run --rm secret-demo sh -c 'cat /build-info.txt; ls /run/secrets 2>&1'
docker history --no-trunc secret-demo | grep -c s3cr3t
```

Expected output:

```text
token length: 12
ls: /run/secrets: No such file or directory
0
```

Breakdown: the secret is mounted only for the duration of one `RUN`: the command read it, but it didn't get into the image and isn't in the history. Pass the token via `ARG` or `ENV` and it stays in the image metadata forever — anyone who can pull the image sees it with `docker history`. The same mechanism mounts an SSH key for private repositories (`--mount=type=ssh`) and a package manager cache (`--mount=type=cache`) that speeds up rebuilds.

</details>

## Dockerfile: instructions that get confused

### CMD and ENTRYPOINT
- **ENTRYPOINT** — *what* to run (the executable).
- **CMD** — the default arguments (or the command, if there's no ENTRYPOINT).
- `docker run image arg1 arg2` **replaces CMD**, while ENTRYPOINT stays. ENTRYPOINT can only be replaced with the `--entrypoint` flag.

```dockerfile
ENTRYPOINT ["kubectl"]
CMD ["--help"]
```
```bash
docker run kubectl-image                 # → kubectl --help
docker run kubectl-image get pods        # → kubectl get pods
```

### Exec form and shell form — a question of signals
```dockerfile
ENTRYPOINT ["dotnet", "Api.dll"]   # exec form (a JSON array): the dotnet process starts directly and becomes PID 1
ENTRYPOINT dotnet Api.dll          # shell form: what actually starts is /bin/sh -c "dotnet Api.dll"
```
In the shell form PID 1 is `sh`, and the application is its child process. `docker stop` (and Kubernetes when deleting a pod) sends SIGTERM to PID 1, i.e. `sh`, and `sh` **doesn't pass the signal on** to the child process. The application never learns it should stop, and after 10 seconds (30 in Kubernetes) SIGKILL arrives. The result: cut-off requests, unclosed connections, lost data.

If you need a wrapper script (prepare a config, wait for dependencies), it must end with `exec`:
```bash
#!/bin/sh
set -e
envsubst < /config.tpl > /app/config.json   # envsubst substitutes environment variable values into the template ($DB_HOST → db.prod)
exec dotnet Api.dll    # exec replaces the shell process with dotnet: dotnet becomes PID 1
```

### Why PID 1 is special
1. The kernel doesn't apply **default signal actions** to PID 1. For an ordinary process without a handler SIGTERM means exit, but for PID 1 without a handler the signal is simply ignored. If the application doesn't install a SIGTERM handler, it won't stop.
2. PID 1 must **reap zombies** — exited child processes. Ordinary applications can't do that.

The fix is a tiny init process as PID 1 that forwards signals and reaps zombies: `docker run --init` (the built-in tini), or `tini` / `dumb-init` in the ENTRYPOINT. In Kubernetes you can enable a shared process namespace for the pod (`shareProcessNamespace: true`: the pod's containers see each other's processes, and PID 1 becomes the pause service process, which reaps zombies).

### The rest
- **COPY and ADD.** `ADD` can additionally download from a URL and **automatically unpacks** local tar archives. Implicit behaviour, so use `COPY`. `ADD` is justified for unpacking an archive or downloading with verification: `ADD --checksum=sha256:... https://...`.
- **ARG and ENV.** `ARG` exists only during the build (but **is visible in `docker history`**, so secrets must not be passed this way). `ENV` goes into the image and is available to the application. An `ARG` declared before `FROM` is visible only in the `FROM` lines; to use it inside a stage, repeat `ARG NAME` after `FROM`.
- **EXPOSE** — documentation only (which port the application listens on). It doesn't open anything. The port is published at run time: `-p 8080:80`.
- **WORKDIR** — instead of `RUN cd /app`: every `RUN` runs in a new shell, and `cd` isn't preserved between instructions.
- **USER** — everything afterwards, including the application itself, runs as this user. It's better to specify a numeric UID: the `runAsNonRoot` check in Kubernetes requires a numeric UID.
- **HEALTHCHECK** — a health check run by Docker (and Compose uses it for `depends_on: condition: service_healthy`). **Kubernetes ignores it** — it has its own liveness and readiness probes.
- **STOPSIGNAL** — if the application expects something other than SIGTERM for a graceful stop (for example, nginx — SIGQUIT).

### 🧪 Mini task

**Task.** Build an image with `ENTRYPOINT ["echo", "entry"]` and `CMD ["default"]`. Run it with no arguments, with an argument and with an overridden `--entrypoint`, and explain what got substituted where.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > Dockerfile <<'EOF'
FROM alpine:3.20
ENTRYPOINT ["echo", "entry"]
CMD ["default"]
EOF
docker build -q -t ep-demo . > /dev/null
docker run --rm ep-demo
docker run --rm ep-demo custom args
docker run --rm --entrypoint date ep-demo +%Y
```

Expected output:

```text
entry default
entry custom args
2026
```

Breakdown: the final command = `ENTRYPOINT` + `CMD`. `CMD` holds default arguments, replaced by anything passed after the image name. `ENTRYPOINT` stays, and only the `--entrypoint` flag replaces it. In Kubernetes `command` overrides `ENTRYPOINT` and `args` overrides `CMD`. The exec form (a JSON array) starts the process directly so it receives signals. The shell form (`CMD app --port 80`) runs it via `/bin/sh -c`, and `SIGTERM` goes to the shell, not the app.

</details>

## Base images: what to choose
| Base image | Size | Features |
|---|---|---|
| `ubuntu`, `debian` | 30–80 MB | Familiar, glibc, a package manager and debugging tools. More vulnerabilities in scanner reports |
| `*-slim` | ~30 MB | Debian without the extras. A sensible default compromise |
| `alpine` | ~5 MB | Very small, but uses **musl libc instead of glibc** (libc is the standard system library through which almost any program talks to the OS; musl is a different, compact implementation): binaries built for glibc don't run; the DNS resolver behaves differently; Python has no prebuilt binary packages (they're built from source — slower, and the image ends up bigger); .NET needs `icu-libs` (a library for languages and cultures) installed |
| distroless (Google) | 2–20 MB | The runtime only: **no shell and no package manager**. Debugging — via `kubectl debug` or the `:debug` image variant |
| chiseled Ubuntu (.NET) | 10–110 MB | Canonical's take on distroless, non-root by default |
| `scratch` | 0 | An empty image for static binaries (Go, Rust) |
| Chainguard / Wolfi | small | Minimal images from the Chainguard company (Wolfi is their distribution built specifically for containers): minimal vulnerabilities, daily updates |

### 🧪 Mini task

**Task.** Pull `alpine`, `debian:bookworm-slim` and `gcr.io/distroless/static-debian12`, compare their sizes, find out which libc Alpine uses and whether distroless has a shell.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
for i in alpine:3.20 debian:bookworm-slim gcr.io/distroless/static-debian12; do docker pull -q $i > /dev/null; done
docker image ls --format '{{.Repository}}:{{.Tag}} {{.Size}}' | grep -E '^(alpine:3.20|debian:bookworm-slim|gcr.io/distroless)'
docker run --rm alpine:3.20 ldd 2>&1 | head -1
docker run --rm gcr.io/distroless/static-debian12 sh 2>&1 | grep -o 'exec: "sh": executable file not found'
```

Expected output:

```text
alpine:3.20 8.83MB
debian:bookworm-slim 97.2MB
gcr.io/distroless/static-debian12 2.53MB
musl libc (aarch64)
exec: "sh": executable file not found
```

Breakdown: Alpine is small but uses musl instead of glibc. Binaries built against glibc won't run there, Python wheels need different builds, and the DNS resolver behaves differently. Debian slim is bigger but compatible with everything. Distroless has only the runtime, no shell or package manager: nothing for an attacker to use, but you can't `docker exec … sh` either — debug with `kubectl debug` and an ephemeral container. On an Intel Mac the libc line will say `x86_64`. Your sizes, versions and IDs will differ.

</details>

## From a Docker image to a machine image (AMI)

A Docker image packages an **application** and its environment, and it runs on someone else's kernel: on a host that already has an OS and a container runtime. But someone has to prepare that host too: a Kubernetes or ECS node, a server running an application outside containers, a CI runner. For that you build a **machine image**. In AWS it's called an **AMI (Amazon Machine Image)**.

An **AMI** is a snapshot of an EBS volume (or several) with a full OS, including the kernel and boot loader, plus metadata: architecture (`x86_64` or `arm64`), boot mode, which volumes to attach. An EC2 instance is launched from an AMI. Every image has an **ID** like `ami-0abc…`, and it is valid in one region only.

The idea is the same as with Docker: build the environment once, from a description in git, and launch identical copies from it. The details differ:

| Docker | AMI |
|---|---|
| `Dockerfile` | a Packer template or an EC2 Image Builder recipe |
| `FROM ubuntu:24.04` | the base AMI: `source_ami_filter` in Packer, parent image in Image Builder |
| `RUN`, `COPY` | provisioners (shell, file, Ansible) in Packer, components in Image Builder |
| `docker build` — seconds or minutes | `packer build` or a pipeline run — 5–20 minutes: launch an instance, configure it, take a snapshot |
| Layers are cached, only what changed is rebuilt | No layer cache: every build runs all steps again, the result is a snapshot of the whole disk |
| Megabytes | Gigabytes: a full OS |
| A tag and digest in a registry (ECR, Docker Hub) | An AMI ID, name and tags in your account; the "latest version" is usually published in SSM Parameter Store (the AWS parameter store: "key → value", for example `/ami/web/latest → ami-0abc…`) |
| `docker pull` from anywhere | An AMI lives in a region and an account; it's copied to another region (`copy-image`), other accounts get launch permissions (permission to launch instances from your image) |
| `docker run` | launching EC2 from a Launch Template (a saved template of instance launch parameters: AMI, type, disks, user data) |
| `ENTRYPOINT`, `CMD` | systemd units and user data (a script the instance runs on first boot) at startup |

**How they live together.** In a typical company with Kubernetes, an AMI is built for the **nodes** (containerd, kubelet, monitoring and security agents, hardening) and updated rarely: once a week or month, for patches. The application Docker images are built on every commit. Many teams don't build their own node AMI at all and take the ready-made EKS-optimized AMI or Bottlerocket (a minimal AWS OS made specifically for containers: no package manager, a read-only root). But in companies with security requirements (CIS — secure configuration checklists, STIG — the equivalent US Department of Defense requirements) or with applications on "bare" EC2, an AMI pipeline of their own almost always exists.

What AMIs are built with: **Packer** (the industry standard, works with any cloud) and **EC2 Image Builder** (a managed AWS service). Both are below. The whole image lifecycle — bake or fry (bake everything into the image in advance or configure the instance at launch), checks, version promotion, rollout via instance refresh (gradually replacing the instances in an Auto Scaling Group with new ones from the fresh image) and deleting old images — is covered below in the [AMI lifecycle](#07-docker.md/s15) section.

### 🧪 Mini task

**Task.** Find the ID of the latest Amazon Linux 2023 AMI two ways: through the public SSM parameter (the equivalent of a `latest` tag) and through `describe-images` sorted by date. Make sure the answers match.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws ssm get-parameter --region us-east-2 \
  --name /aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64 \
  --query Parameter.Value --output text
aws ec2 describe-images --region us-east-2 --owners amazon \
  --filters 'Name=name,Values=al2023-ami-2023*-kernel-*-x86_64' 'Name=state,Values=available' \
  --query 'sort_by(Images, &CreationDate)[-1].[ImageId, Name]' --output text
```

Expected output:

```text
ami-0c5ddb3560e768732
ami-0c5ddb3560e768732	al2023-ami-2023.9.20261020.0-kernel-6.12-x86_64
```

Breakdown: AMIs have no tags like `alpine:3.20` — every image has its own ID, unique per region. The "floating tag" role is played by the SSM parameter, which AWS updates when a new image is released; you can also reference it in a launch template as `resolve:ssm:…`. Unlike a Docker image, an AMI is bound to a region (it's copied for another region) and contains EBS snapshots and a kernel. AWS credentials are required. Your sizes, versions and IDs will differ.

</details>

## Packer: building a machine image as code

**Packer** is a CLI tool from HashiCorp, the authors of Terraform. The template is written in HCL (HashiCorp Configuration Language — a declarative configuration language), the same language as Terraform. One template can build an image for several platforms at once: AWS, Azure, GCP, VMware, Proxmox, even Docker.

### What happens during `packer build` for AWS
The most common builder is `amazon-ebs`. Step by step:
1. Finds the base AMI: a specific one (`source_ami`) or the newest one matching a filter (`source_ami_filter`).
2. Creates a temporary key pair and a security group allowing SSH only from the address Packer runs from.
3. Launches a temporary EC2 instance and waits until it can connect over SSH (for Windows — WinRM, the Windows remote management protocol).
4. Runs the **provisioners** in order: shell scripts, file copies, an Ansible playbook.
5. Stops the instance (doesn't terminate it) and calls `CreateImage`: the volumes become EBS snapshots and an AMI is registered.
6. Waits for the AMI to become `available`, adds tags, copies it to other regions and grants access to other accounts if configured.
7. Terminates the temporary instance, deletes the key and the security group, then runs the **post-processors** (for example, writes `manifest.json` with the new AMI ID).

If the build fails, Packer cleans up the temporary resources by default. No AMI is created.

### What a template consists of
- **`packer {}`** — required plugins and their versions. The AWS builder and the Ansible provisioner are separate plugins; `packer init` downloads them.
- **`variable`** — build parameters (version, region); passed with `-var` or a `*.pkrvars.hcl` file.
- **`source`** — *what to build on* and *from what*: builder type, region, instance type, base AMI, name and tags of the resulting AMI.
- **`build`** — *what to do*: which `source` to use, the list of `provisioner` and `post-processor` blocks.

### Example: Amazon Linux 2023 + nginx, configured with Ansible
```hcl
packer {
  required_plugins {
    amazon = {
      source  = "github.com/hashicorp/amazon"
      version = ">= 1.3.0"
    }
    ansible = {
      source  = "github.com/hashicorp/ansible"
      version = ">= 1.1.0"
    }
  }
}

variable "version" {
  type = string # image version, e.g. 1.2.0
}

locals {
  build_time = formatdate("YYYYMMDD-hhmm", timestamp())
}

source "amazon-ebs" "web" {
  region        = "eu-central-1"
  instance_type = "t3.micro"
  ssh_username  = "ec2-user"

  # The newest official Amazon Linux 2023
  source_ami_filter {
    filters = {
      name                = "al2023-ami-2023.*-x86_64"
      virtualization-type = "hvm"
      root-device-type    = "ebs"
    }
    owners      = ["amazon"]
    most_recent = true
  }

  # The AMI name must be unique: version + build time
  ami_name = "web-${var.version}-${local.build_time}"
  tags = {
    Version = var.version
    BaseAMI = "{{ .SourceAMI }}" # which base it was built from: it changes between builds
  }

  encrypt_boot = true   # encrypted root volume
  imds_support = "v2.0" # instances from this AMI require IMDSv2 — the protected version of the metadata service (a token is required: an SSRF attack via the app can't steal the role keys)

  # The temporary security group opens SSH only for the address Packer runs from
  temporary_security_group_source_public_ip = true
}

build {
  sources = ["source.amazon-ebs.web"]

  provisioner "shell" {
    inline = ["sudo dnf -y update"]
  }

  provisioner "ansible" {
    playbook_file = "./playbook.yml"
    user          = "ec2-user"
    use_proxy     = false # Ansible connects to the instance directly, without Packer's SSH proxy
  }

  # manifest.json with the new AMI ID: the next CI step reads it
  post-processor "manifest" {
    output = "manifest.json"
  }
}
```
```bash
packer init .                         # download plugins
packer fmt .                          # format the template
packer validate -var version=1.2.0 .  # check without running
packer build -var version=1.2.0 .     # build
```
`{{ .SourceAMI }}` is a built-in Packer variable: the ID of the base AMI the build actually used. This way the tags show which OS version the image is based on.

### Useful `amazon-ebs` options
- `ami_regions = ["eu-west-1"]` — copy the AMI to other regions right away.
- `ami_users` and `ami_org_arns` — grant other accounts or the whole AWS Organization (all of a company's accounts combined under one management) permission to launch the image.
- `deprecate_at` — the date after which the AMI is marked deprecated and disappears from regular search.
- `ssh_interface = "session_manager"` — connect to the temporary instance through SSM Session Manager: no public IP and no open port 22 needed (handy in private subnets).
- `force_deregister` — overwrite an AMI with the same name. Handy while debugging, dangerous in production.

Besides `amazon-ebs` there are `amazon-chroot` (builds on an already running EC2 by mounting a new volume: faster, but more complex) and `amazon-ebssurrogate` (an image from scratch on a separate volume). For other platforms: `azure-arm`, `googlecompute`, `vsphere-iso`, `proxmox-iso`, `qemu`, `docker`.

### Debugging
- `packer build -on-error=ask` — on error Packer asks what to do, so you can keep the temporary instance and log in to look around.
- `packer build -debug` — step-by-step mode, saves the temporary SSH key to the current directory.
- `PACKER_LOG=1 packer build …` — verbose log.

### Common mistakes
- **`Timeout waiting for SSH`.** The instance has no public IP or no route to it (private subnet, security group). Fixes: `ssh_interface = "session_manager"` or running Packer inside the VPC.
- **The AMI name is already taken.** Names must be unique within a region, so a version and timestamp are added.
- **Secrets and junk in the image.** Keys, `.bash_history`, the cloud-init state (the service that applies user data and installs SSH keys on an instance's first boot; if its state stays in the image, on new instances it will think it has already run), SSH host keys (the keys by which clients recognise the server: all servers from the image would end up with the same ones) end up on every server built from the image. They're removed in the last provisioner, and secrets are fetched at launch (SSM, Secrets Manager).
- **AMI ID hard-coded in Terraform.** The ID differs per region and changes with every build. The latest ID is published to SSM Parameter Store, and Terraform reads it from there.
- **Deregistered the AMI but not the snapshots.** `deregister-image` doesn't delete EBS snapshots, and they keep being billed.

**License.** Since August 2023 Packer is distributed under BSL 1.1 (Business Source License — the code is open but with a restriction on commercial use): free to use within your company; the restriction only concerns competing commercial products. HCP Packer (HashiCorp Cloud Platform) is a separate paid registry of image metadata, not needed for building.

### 🧪 Mini task

**Task.** Without spending money on EC2, get to know the structure of a Packer template: describe a `null` source with a version variable and a `shell-local` provisioner, check it with `validate` and run `build`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > lab.pkr.hcl <<'EOF'
variable "version" {
  type    = string
  default = "1.0.0"
}

source "null" "lab" {
  communicator = "none"
}

build {
  sources = ["source.null.lab"]

  provisioner "shell-local" {
    inline = ["echo hello from packer build lab-${var.version}"]
  }
}
EOF
packer validate .
packer build -color=false -var version=1.2.0 . | grep -E 'hello|finished' | sed 's/ after.*//'
```

Expected output:

```text
The configuration is valid.
    null.lab: hello from packer build lab-1.2.0
Build 'null.lab' finished
```

Breakdown: the structure matches a real template: `variable` holds build parameters, `source` says where to build (in production `amazon-ebs` with a base AMI, instance type and SSH), `build` lists the provisioners to run (shell, Ansible, file copies). `-var` overrides variables, just as in Terraform. With `amazon-ebs` Packer would start a temporary EC2 instance, run the provisioners, stop the instance, create an AMI and delete everything temporary.

</details>

## EC2 Image Builder: AWS's managed image pipeline

**EC2 Image Builder** is an AWS service that does what Packer does in CI, but on its own: on a schedule it builds an AMI (or a Docker image), tests it, scans it for vulnerabilities, distributes it to regions and accounts, and removes old versions. The service itself is free; you pay only for EC2 and EBS during the build, snapshot storage, and Amazon Inspector if scanning is enabled.

### What it consists of
| Resource | What it is | Packer equivalent |
|---|---|---|
| **Component** | A YAML document with steps by phases `build`, `validate`, `test`. AWS provides ready-made ones: OS update, CloudWatch agent (an agent that sends host metrics and logs to CloudWatch), STIG hardening | `provisioner` |
| **Image recipe** | Base image + list of components + volumes. Versioned like `1.0.0` | `source` + `build` |
| **Container recipe** | The same for a Docker image: base image, components, a Dockerfile template, the target ECR repository | `docker` builder |
| **Infrastructure configuration** | What to build on: the instance type, the IAM instance profile (a wrapper for an IAM role through which the role is "attached" to an EC2 instance), subnet, security group, where to write logs | `source` parameters |
| **Distribution configuration** | Where the result goes: regions, accounts, AMI name and tags, which Launch Template to update | `ami_regions`, `ami_users`, a CI script |
| **Image pipeline** | Everything together + a schedule and enabling tests and scanning | a cron job in CI |
| **Lifecycle policy** | Retention rules: deprecate, disable and delete old AMIs together with their snapshots | your own cleanup script |

### How a build runs
1. A **build instance** is launched from the base image. Components run through the **SSM Agent**, so no SSH, keys or open ports are needed (the instance needs a role with the `EC2InstanceProfileForImageBuilder` and `AmazonSSMManagedInstanceCore` policies).
2. AWSTOE (AWS Task Orchestrator and Executor) runs the components: first the `build` phase, then `validate`.
3. An AMI is created from the instance.
4. A separate **test instance** is launched from the **new** AMI and runs the `test` phase of all components. This checks that a working server actually starts from the image, not just that the build succeeded.
5. If enabled, Amazon Inspector scans the image for vulnerabilities.
6. Distribution copies the AMI to regions and accounts and can update a Launch Template.

Versions: recipes and components have a semantic version (`1.0.0` — MAJOR.MINOR.PATCH: incompatible changes, new features, fixes), and each build gets a build number: `1.0.0/1`, `1.0.0/2`. The base image and AWS components can be given the version `x.x.x` — then the latest is always taken.

### Component example (AWSTOE)
```yaml
name: web-setup
description: nginx with a demo page
schemaVersion: 1.0
phases:
  - name: build
    steps:
      - name: InstallNginx
        action: ExecuteBash
        inputs:
          commands:
            - dnf -y install nginx
            - systemctl enable nginx
  - name: validate
    steps:
      - name: NginxConfigIsValid
        action: ExecuteBash
        inputs:
          commands:
            - nginx -t
  - name: test
    steps:
      - name: NginxAnswers
        action: ExecuteBash
        inputs:
          commands:
            - systemctl start nginx
            - curl -sf http://localhost/ > /dev/null
```
`validate` runs on the build instance right after the build, and `test` runs on a separate instance launched from the finished AMI. Besides `ExecuteBash` there are actions for PowerShell, downloading and uploading files to S3, rebooting and updating the OS. Ansible can be called as an ordinary command in a step.

### Pipeline example in Terraform
```hcl
resource "aws_imagebuilder_component" "web" {
  name     = "web-setup"
  platform = "Linux"
  version  = "1.0.0"
  data     = file("${path.module}/web-setup.yaml") # the component from the example above
}

data "aws_ssm_parameter" "al2023" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64"
}

resource "aws_imagebuilder_image_recipe" "web" {
  name         = "web"
  version      = "1.0.0"
  parent_image = data.aws_ssm_parameter.al2023.insecure_value

  # A ready-made AWS component: update all OS packages
  component {
    component_arn = "arn:aws:imagebuilder:eu-central-1:aws:component/update-linux/x.x.x"
  }
  component {
    component_arn = aws_imagebuilder_component.web.arn
  }
}

resource "aws_imagebuilder_infrastructure_configuration" "web" {
  name                          = "web-build"
  instance_types                = ["t3.micro"]
  instance_profile_name         = "image-builder-ec2" # a role with EC2InstanceProfileForImageBuilder + AmazonSSMManagedInstanceCore
  terminate_instance_on_failure = true
}

resource "aws_imagebuilder_distribution_configuration" "web" {
  name = "web"
  distribution {
    region = "eu-central-1"
    ami_distribution_configuration {
      name = "web-{{ imagebuilder:buildDate }}"
      ami_tags = {
        Role = "web"
      }
    }
  }
}

resource "aws_imagebuilder_image_pipeline" "web" {
  name                             = "web"
  image_recipe_arn                 = aws_imagebuilder_image_recipe.web.arn
  infrastructure_configuration_arn = aws_imagebuilder_infrastructure_configuration.web.arn
  distribution_configuration_arn   = aws_imagebuilder_distribution_configuration.web.arn

  # Every Monday at 03:00 UTC, but only if a new base or a new component is out
  schedule {
    schedule_expression                = "cron(0 3 ? * mon *)"
    pipeline_execution_start_condition = "EXPRESSION_MATCH_AND_DEPENDENCY_UPDATES_AVAILABLE"
  }

  image_tests_configuration {
    image_tests_enabled = true
  }

  image_scanning_configuration {
    image_scanning_enabled = true # Amazon Inspector scanning
  }
}
```
The `EXPRESSION_MATCH_AND_DEPENDENCY_UPDATES_AVAILABLE` condition saves money and noise: the pipeline runs on schedule only if AWS released a new base AMI or a component was updated.

### Common mistakes
- **The instance has no access to SSM.** Without a role with `AmazonSSMManagedInstanceCore` or without internet access (or VPC endpoints for SSM in a private subnet — private "entrances" to AWS services right from the VPC, without internet or NAT) the build hangs and fails on a timeout.
- **Changed a component but didn't bump its version.** Components are immutable: new content means a new version, and the recipe has to be updated too.
- **No lifecycle policy.** A scheduled pipeline piles up AMIs and snapshots, and they're billed.
- **Debugging through the full cycle.** Every attempt is a 15–30 minute pipeline run. Logs are in CloudWatch Logs or S3 (set in the infrastructure configuration).

### 🧪 Mini task

**Task.** Use the AWS CLI to find Amazon's ready-made EC2 Image Builder components for updating Linux, and see which steps one of them consists of.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws imagebuilder list-components --owner Amazon --region us-east-2 \
  --query 'componentVersionList[?contains(name, `update-linux`)].[name, platform]' --output text
aws imagebuilder get-component --region us-east-2 \
  --component-build-version-arn arn:aws:imagebuilder:us-east-2:aws:component/update-linux/x.x.x \
  --query component.data --output text | grep -E '^ +- name:' | head -4
```

Expected output:

```text
update-linux	Linux
update-linux-kernel-mainline	Linux
    - name: build
      - name: DetermineDistro
      - name: UpdateYum
      - name: UpdateApt
```

Breakdown: an Image Builder component is a YAML document for AWSTOE with phases (`build`, `validate`, `test`) and steps inside. `x.x.x` in the ARN means "latest version". Amazon's ready-made components cover typical jobs (updates, the CloudWatch agent, CIS hardening), and your own use the same format. A recipe = a base image + a list of components; a pipeline = a recipe + build infrastructure + a schedule and AMI distribution across regions and accounts. AWS credentials are required. Your sizes, versions and IDs will differ.

</details>

## Packer or Image Builder: comparison and choice

| | Packer | EC2 Image Builder |
|---|---|---|
| What it is | A CLI tool you run yourself: locally or in CI | A managed AWS service |
| Platforms | AWS, Azure, GCP, VMware, Proxmox, Docker: one template — images for several | AWS only: AMIs and Docker images in ECR |
| Definition | HCL, like Terraform | YAML components + resources in Terraform, CloudFormation or the console |
| Where it runs | On a CI runner, which needs AWS permissions | Inside AWS, no CI needed |
| Access to the build instance | SSH or WinRM (can go through Session Manager) | Through the SSM Agent, no SSH needed |
| Configuration | Any provisioners: shell, Ansible, PowerShell, Chef, Puppet | Components: Bash and PowerShell, ready-made AWS components; Ansible — as a command in a step |
| Tests | Yourself: the last provisioners or a separate job (Goss, InSpec — tools that check a server against a declarative list of expectations: "package installed", "port listening") | Built in: the `test` phase on a separate instance from the new AMI |
| Vulnerability scanning | Separately (Inspector, Trivy) | Amazon Inspector is enabled with a flag |
| Schedule | cron in CI | Built in, plus "build only when dependencies are updated" |
| Distribution to regions and accounts | `ami_regions`, `ami_users`, `ami_org_arns` | Distribution configuration, Launch Template update |
| Removing old images | Your own script, `deprecate_at` | Lifecycle policies |
| Debugging speed | Fast: run from a laptop, `-on-error=ask`, `-debug` | Slow: every attempt is a full pipeline run |
| Cost | Free (BSL), you pay for EC2 during the build | The service is free, you pay for EC2, EBS, Inspector |

**When Packer:**
- several clouds or on-prem (VMware, Proxmox) — one approach and the same Ansible roles for all of them;
- the team already lives in CI and Terraform and needs fast iteration and debugging;
- the configuration is already written in Ansible.

**When Image Builder:**
- AWS only, and you want regular scheduled patching out of the box without your own CI plumbing;
- security requirements: built-in tests, Inspector, ready-made STIG hardening components;
- many accounts in an AWS Organization, and the image has to be distributed to them with a Launch Template update;
- you want one tool for both AMIs and Docker images in ECR.

They can be combined: the platform team builds the base "golden" AMI with hardening in Image Builder, and application teams build their own images on top of it with Packer.

**How to answer in an interview if you've only worked with Docker.** The principle is the same as building a Docker image in CI: a definition in git → automated build → version and tags → publishing → rolling out a new version instead of changing live servers. What differs is the tool (Packer or Image Builder instead of `docker build`), the artifact (a disk snapshot instead of layers) and the rollout (Launch Template + instance refresh instead of a new tag in a Deployment).

### 🧪 Mini task

**Task.** The account builds AMIs with both Packer and Image Builder. List your AMIs with the tag that shows which tool and which commit built each image.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws ec2 describe-images --owners self --region us-east-2 \
  --query 'Images[].[Name, Tags[?Key==`BuiltBy`] | [0].Value, Tags[?Key==`GitCommit`] | [0].Value]' \
  --output text | sort
```

Expected output:

```text
web-1.4.0	packer	3f9c2e1
web-1.5.0	packer	a71d0b4
worker-2.0.1	image-builder	None
```

Breakdown: choosing the tool is mainly an operations question. Packer runs anywhere (CI, locally), supports many clouds and keeps templates in git next to the code, but you build the pipeline, schedule and cross-region distribution yourself. Image Builder is a managed AWS service with schedules, tests and distribution, but AWS only. Either way, AMIs must be tagged: which tool, which commit and which recipe built them. The `None` on the Image Builder image shows a gap in that discipline.

</details>

## The AMI lifecycle

An **AMI (Amazon Machine Image)** is the image EC2 is launched from: disk snapshots plus metadata (architecture, boot mode, ENA support). When the hosts are identical and there are many of them, the AMI becomes the platform's main artifact, like a Docker image for containers.

### Bake or fry
- **Bake** — everything needed is installed and configured in the image. The instance starts in seconds, all hosts are identical, there's no dependency on external repositories at startup. The downside: any change means a new image.
- **Fry** — the image is basic, everything is installed at launch via user data or Ansible. Flexible, but startup is slow, hosts can differ (what got installed at a launch on Monday versus on Friday), and a repository outage breaks scaling.

A practical compromise: **in the image — the OS, kernel settings, drivers, agents, the runtime of the right version; at launch — only environment configuration and secrets** (from SSM Parameter Store or Secrets Manager via the instance role). For a latency-sensitive platform — an almost complete bake: kernel parameters (`isolcpus`, hugepages) require a reboot anyway, and doing one on every start is unreasonable.

### The image pipeline
```text
a change in git (Packer + Ansible)
  → build the AMI (Packer)
  → hardening and a vulnerability scan
  → checks: launch a test instance from the image, tests
  → a version tag, registration as a "candidate"
  → rollout to staging → promotion to "stable"
  → rollout to prod via instance refresh with rollback
  → deprecation and deletion of old images
```

### Building in the pipeline: Packer + Ansible
**Packer** launches a temporary instance from a base image, runs provisioners (Ansible, shell), stops the instance and creates an AMI. It's better to describe host configuration with **Ansible roles** — then the same role is applied both when building the image and for on-prem. How Packer and EC2 Image Builder work is covered in the sections above; here is a template with what matters specifically for the pipeline: the version and commit in tags, the `candidate` channel, cleanup before the snapshot and a manifest with the image ID for the next steps.

```hcl
# runtime-host.pkr.hcl
packer {
  required_plugins {
    amazon  = { source = "github.com/hashicorp/amazon",  version = "~> 1.3" }
    ansible = { source = "github.com/hashicorp/ansible", version = "~> 1.1" }
  }
}

variable "version" { type = string }   # the image version, for example 2026.10.16-1
variable "git_sha" { type = string }

source "amazon-ebs" "runtime" {
  region        = "eu-central-1"
  instance_type = "c6in.xlarge"
  ssh_username  = "ec2-user"
  ami_name      = "runtime-host-${var.version}"

  source_ami_filter {                      # the latest official Amazon Linux 2023
    filters = {
      name                = "al2023-ami-2023.*-x86_64"
      virtualization-type = "hvm"
    }
    owners      = ["amazon"]
    most_recent = true
  }

  ena_support   = true
  imds_support  = "v2.0"                   # instances from the image require IMDSv2
  encrypt_boot  = true                     # the image's snapshots are encrypted with KMS

  launch_block_device_mappings {
    device_name           = "/dev/xvda"
    volume_size           = 20
    volume_type           = "gp3"
    delete_on_termination = true
  }

  tags = {
    Name          = "runtime-host"
    Version       = var.version
    GitCommit     = var.git_sha
    BaseAMI       = "{{ .SourceAMI }}"     # which base image it was built from
    BaseAMIName   = "{{ .SourceAMIName }}"
    Channel       = "candidate"            # not verified yet
  }
}

build {
  sources = ["source.amazon-ebs.runtime"]

  provisioner "ansible" {
    playbook_file   = "ansible/runtime-host.yml"   # OS, sysctl, kernel parameters, hugepages, drivers, runtime, agents, hardening
    extra_arguments = ["-e", "runtime_version=${var.version}"]
  }

  provisioner "shell" {                    # before the snapshot: remove the traces of the build
    inline = [
      "sudo cloud-init clean --logs",      # on first boot cloud-init will run again (host keys, hostname)
      "sudo rm -rf /tmp/* /var/tmp/*",
      "sudo truncate -s 0 /etc/machine-id" # machine-id — the machine's unique ID (DHCP and journald tell hosts apart by it); every instance will get its own
    ]
  }

  post-processor "manifest" {
    output = "packer-manifest.json"        # the ID of the created AMI — for the next pipeline steps
  }
}
```
```bash
packer init .
packer validate -var version=2026.10.16-1 -var git_sha=$(git rev-parse --short HEAD) .
packer build    -var version=2026.10.16-1 -var git_sha=$(git rev-parse --short HEAD) .
```
Why `cloud-init clean` and `machine-id`: without this all instances from the image inherit identical identifiers, and cloud-init decides the first boot has already happened and doesn't run the user data.

### Hardening in the image
The image is the best place for hardening: it's checked once and applied to all hosts.
- **CIS Benchmark** for the distribution: ready-made Ansible roles (for example, the ansible-lockdown project) or **OpenSCAP** profiles;
- a minimum of packages and services, SSH only by key without root (or no SSH at all — access via **SSM Session Manager**);
- services with their own users and systemd sandbox parameters (see the Linux topic);
- **IMDSv2** is mandatory;
- `auditd` and shipping logs out.

Checking the result — an OpenSCAP or Lynis report inside the build and a **vulnerability scan** (Amazon Inspector scans EC2 and images, Trivy can scan a root filesystem).

### Image checks
An image that "built" isn't working yet. The pipeline launches a test instance from the new AMI and checks:
- the host booted, cloud-init finished without errors (`cloud-init status --long`);
- the kernel parameters were applied: `cat /proc/cmdline`, `cat /sys/devices/system/cpu/isolated`, `grep HugePages_Total /proc/meminfo`;
- sysctl, THP and irqbalance are in the expected state;
- the needed services are active, the runtime responds to the health check;
- for FPGA hosts: the AFI loaded;
- a short load test: P99 is no worse than with the current stable image.

Such checks are conveniently described declaratively: **goss** (YAML with expectations like "the service is running, the port is listening, the file contains") or **InSpec**. If the checks passed, the `Channel=candidate` tag is changed to a verified one and the test instance is deleted.

### Versions and promotion
An image is found **not by name and not as "the newest"**, but via an explicit pointer. An SSM parameter is convenient:
```bash
aws ssm put-parameter --name /runtime/ami/stable --type String --overwrite --value ami-0abc1234def567890
```
The Launch Template refers to the parameter rather than a specific ID:
```hcl
resource "aws_launch_template" "runtime" {
  name_prefix   = "runtime-"
  image_id      = "resolve:ssm:/runtime/ami/stable"
  instance_type = "c6in.2xlarge"
}
```
Promotion `staging → stable` is a change of the parameter's value. Rollback is restoring the previous value. The change history is kept in the parameter's versions.

**Multi-account and multi-region:** the image is built in a separate account (tooling — a service account for CI and builds, separated from prod) and **shared** with the environment accounts (`ami_users` or `ami_org_arns` in Packer, `aws ec2 modify-image-attribute`), and copied to other regions (`ami_regions`, `aws ec2 copy-image`). For encrypted images the receiving accounts need access to the **KMS key** — a common cause of the "the instance doesn't start from the shared image" error.

### Zero-downtime rollout with rollback
**ASG Instance Refresh** replaces the group's instances in batches with new ones from the updated Launch Template:
```bash
aws autoscaling start-instance-refresh \
  --auto-scaling-group-name runtime-prod \
  --preferences '{
    "MinHealthyPercentage": 90,
    "InstanceWarmup": 120,
    "CheckpointPercentages": [10, 50, 100],
    "CheckpointDelay": 600,
    "AutoRollback": true,
    "AlarmSpecification": {"Alarms": ["runtime-p99-latency-high"]}
  }'
```
- **CheckpointPercentages** — a canary (rolling out to a small share of hosts first, like a canary in a mine: if it's unwell, the rest aren't touched): replace 10%, wait 10 minutes, look at the metrics, then continue;
- **AlarmSpecification + AutoRollback** — if an alarm on P99 or errors fired, the refresh stops and the group returns to the previous Launch Template;
- **InstanceWarmup** — how long to wait before considering a new instance fully operational (warming caches, loading the FPGA).

For hosts holding long-lived connections a **graceful drain** is needed before replacement (a soft removal from service: new connections aren't accepted, current ones finish): the `autoscaling:EC2_INSTANCE_TERMINATING` lifecycle hook (an ASG hook that pauses instance termination until your script says "done") gives time to take the host out of the load balancer and wait for connections to finish.

### Deprecation and deletion
Old images pile up and cost money — mostly for the snapshots.
- A retention policy: for example, the last N images plus everything used by Launch Templates.
- `aws ec2 enable-image-deprecation --image-id ami-... --deprecate-at 2026-12-31T00:00:00Z` — the image is marked deprecated and disappears from ordinary searches, but existing references keep working.
- **`deregister-image` doesn't delete the snapshots.** They have to be deleted separately, otherwise you pay for "deleted" images for years:
  ```bash
  SNAPS=$(aws ec2 describe-images --image-ids ami-0old --query 'Images[0].BlockDeviceMappings[].Ebs.SnapshotId' --output text)
  aws ec2 deregister-image --image-id ami-0old
  for s in $SNAPS; do aws ec2 delete-snapshot --snapshot-id "$s"; done
  ```
- It can be automated with **Amazon Data Lifecycle Manager** (policies for AMIs) or a scheduled Lambda (an AWS function that runs on an event or schedule without a server of your own). **Recycle Bin** protects against accidental deletion: deleted images and snapshots can be restored within a set period.

### The same image for customers and on-prem
When the product has to be installed in someone else's environment, it pays off if **the same set of Ansible roles** builds all variants: Packer supports several builders in one template — `amazon-ebs` for AWS, `qemu` for a qcow2 image (the disk format of KVM virtual machines — the hypervisor built into Linux — and of OpenStack-based clouds), `vsphere-iso` for VMware. Plus an update procedure the customer can run themselves and a diagnostic bundle (a script that collects the host's logs and state), because you won't have access to their servers.

### 🧪 Mini task

**Task.** Find your AMIs older than 90 days and check, for each, whether any instance uses it. This is the first cleanup step: only unused ones may be deleted.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cutoff=$(date -u -v-90d +%Y-%m-%dT%H:%M:%S 2>/dev/null || date -u -d '90 days ago' +%Y-%m-%dT%H:%M:%S)
used=$(aws ec2 describe-instances --region us-east-2 --query 'Reservations[].Instances[].ImageId' --output text)
aws ec2 describe-images --owners self --region us-east-2 \
  --query "Images[?CreationDate<'$cutoff'].[ImageId, Name]" --output text |
while read -r id name; do
  if grep -qw "$id" <<< "$used"; then echo "$id $name IN USE"; else echo "$id $name unused"; fi
done
```

Expected output:

```text
ami-0a1b2c3d4e5f60718 web-1.2.0 unused
ami-0f9e8d7c6b5a40312 web-1.3.0 IN USE
```

Breakdown: string comparison of dates in JMESPath works because ISO 8601 sorts as text. An old AMI may still be in use: an instance runs on it or a launch template references it, and then you must not delete it, or Auto Scaling can't start new machines. Launch templates are checked too. After `deregister-image` the image's snapshots must be deleted as well — the AMI itself costs almost nothing, you pay for the snapshots. The `date` command is written to work on both macOS and Linux. Your sizes, versions and IDs will differ.

</details>

## Common AMI mistakes

- **Choosing the AMI as "the newest by name"** in Terraform — a new image gets into prod unchecked on any `apply`.
- **Deleting AMIs without their snapshots** — the costs remain.
- **Not pinning the kernel and driver versions** in the image — an automatic kernel update breaks the accelerator's driver module.
- **Secrets in the image** — the image is copied and shared, and the secret leaks with it. Secrets — only at launch via the instance role.

### 🧪 Mini task

**Task.** Check your AMIs for two common mistakes: an image accidentally made public and an unencrypted disk snapshot.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
aws ec2 describe-images --owners self --region us-east-2 \
  --query 'Images[].[ImageId, Name, Public, BlockDeviceMappings[0].Ebs.Encrypted]' --output text |
awk '$3 == "True" || $4 == "False" {print "PROBLEM:", $0; next} {print "ok:     ", $0}'
```

Expected output:

```text
ok:      ami-0a1b2c3d4e5f60718	web-1.2.0	False	True
PROBLEM: ami-0c4d5e6f7a8b90123	debug-image	True	False
```

Walkthrough: a public AMI can be launched by any AWS account. If keys, configs or code were "baked" into the image, that's a leak. That's why accounts enable blocking of public access to AMIs (`enable-image-block-public-access`). An unencrypted snapshot violates SOC 2 and PCI requirements (PCI DSS — the security standard for everyone who processes bank card data), and a ready image can only be encrypted by copying it with `--encrypted`. Both checks are worth running in CI after the image build or in AWS Config (a service that constantly checks the account's resources against rules), rather than remembering them at audit time.

</details>

## When the artifact is a machine image, not a container

The usual path for applications: Dockerfile → build in CI → registry (Nexus, ECR) → roll out the needed version to ECS or Kubernetes. It's convenient when there are many services, releases are frequent and hosts are just interchangeable compute. But there is a class of products where **the unit of deployment is the whole server**, and then the main artifact is an AMI, not a Docker image.

**Signs that you need a machine image:**
- **Host-level and boot-time settings.** Isolated CPU cores, hugepages, kernel command-line parameters, device drivers, accelerator firmware. A container runs on someone else's kernel and sets none of these.
- **Working with hardware directly:** FPGA, SmartNIC (a network card with its own processor that does part of the traffic processing itself), a network card in DPDK mode (unbound from the Linux kernel and handed to the application).
- **Strict requirements on latency and its spread.** An overlay network, kube-proxy and iptables, CPU limits via cgroup quotas, neighbouring pods on the node add micro- and milliseconds and, above all, unpredictable tails.
- **One critical service on a dedicated host.** Dense packing of containers isn't needed, and the orchestrator becomes an extra layer that also has to be operated and secured.
- **Delivery to the customer as an appliance** (a ready "device": an image with the OS and the product that is simply launched, with nothing to install). The product is handed over as a ready image the customer launches in their own account or data centre.
- **Audit and hardening.** A minimal immutable host built from code and validated is easier to show an auditor than an "orchestrator + runtime + images" stack.

**Where it's done this way:**
- **Network and security vendors:** Palo Alto VM-Series, F5 BIG-IP, Fortinet FortiGate, Check Point are distributed as AMIs in AWS Marketplace. The customer runs them in their own VPC, and traffic never leaves their perimeter.
- **Finance and trading:** exchange gateways and market data handlers on dedicated hosts with CPU and network tuning, often with FPGAs.
- **Telecom:** the 5G core and packet processing (UPF — User Plane Function, the 5G network node all user traffic flows through; routers on VPP — Vector Packet Processing, an open-source software router) on DPDK.
- **Databases and caches on dedicated hosts:** hugepages, disk and network tuning.
- **Netflix** for a long time rolled out services as baked AMIs (its own tools Aminator and Spinnaker): the immutable infrastructure approach — servers aren't patched, they're replaced.

**These are different layers, not either/or.** Inside an AMI the application can be built in CI as a binary or even a Docker image, then baked into the image and started via systemd. The pipeline principle is the same as with Docker: a definition in git → build in CI → version → artifact storage → rollout by version, without changing live servers.

| | Docker + registry + ECS/EKS | AMI + Auto Scaling Group |
|---|---|---|
| What's packaged | The application and its dependencies | The whole host: kernel, drivers, OS tuning and the application |
| Good for | Many microservices, frequent releases, dense packing | One critical service on a dedicated machine, hardware access |
| Build | Seconds or minutes, layer cache | 10+ minutes, no cache |
| Rollout | Seconds: a new pod or task | Minutes: a new instance boots |
| Control over the host | None | Full |
| Shipping to a customer | Needs their cluster | Image into their account or Marketplace, an image for on-prem |

## A latency-critical data plane: NOL8 as an example

**NOL8** (a Platform & SRE vacancy via MWDN) builds an "AI Data Plane" — a layer that sits right in the traffic path of AI systems: prompts, model responses, tool calls by agents. It inspects every payload (the useful content of a request — the prompt or response text itself) on the fly, routes it to the right model, and blocks or scrubs what's forbidden (secrets, personal data, breaches of boundaries between customers). The website has three key figures, and each explains why their infrastructure is built around AMIs.

### "8 ms P99 end-to-end"
- **P99** is the 99th percentile of latency: 99% of requests complete within 8 ms, and only 1% are slower. That's the "tail" of the distribution. The average hides it: with a 2 ms average, one request in a hundred can take 200 ms. For a product that **every** request passes through, the tail is the user experience: an agent makes dozens of calls in a row, and a slow tail shows up in almost every chain.
- **End-to-end** is the time from sending the request to receiving the whole response: network, queues, parsing, matching, rewriting, forwarding. Not just the "engine time", which always looks better.
- **Why it matters for infrastructure:** keeping the tail in single-digit milliseconds means fighting sources of jitter — context switches, a process migrating between cores, TLB misses, NIC interrupts on the wrong core, access to memory of a "remote" NUMA node. All of this is configured on the host and baked into the AMI. Percentiles and measuring in detail: [Linux OS → Latency and tails](#06-linux.md/s8).

### "8,000 patterns", and throughput doesn't drop
- The product looks for matches against thousands of **patterns** in every payload: strings and regular expressions (API keys, card numbers, forbidden phrases, data-rights lists).
- **In a naive implementation** time grows with the number of patterns: checking 8,000 regexes one by one takes 8,000 times longer than one.
- **Algorithms** partly solve this: Aho-Corasick finds many strings in one pass, Intel Hyperscan compiles thousands of regexes into one automaton on the CPU's SIMD instructions (SIMD — one processor instruction processes several bytes of data at once) (used in Suricata, Snort — network intrusion detection systems, and Rspamd — a spam filter).
- **In hardware** (FPGA) thousands of comparisons run in parallel within one clock cycle, so the time hardly depends on the number of patterns. Hence "throughput stays flat as patterns grow".

### "0 GPU"
- Many protective solutions for LLMs (guardrails — filters that keep dangerous or forbidden content from going into or out of the model) themselves run on classifier models. Every check is inference (running a model on input data) on a GPU: expensive, GPUs are scarce, latency is tens to hundreds of milliseconds, and the result is probabilistic.
- NOL8 does **deterministic matching** ("patterns, not probabilities"): the same input always gives the same result, and no GPU is used. It isn't a replacement for semantic guardrails but a fast, exact filter for known patterns.
- For infrastructure this means cheap and predictable hosts, on CPU and apparently FPGA, with no GPU nodes or CUDA drivers.

### "Matched in hardware"
- The site doesn't name the hardware, but the vacancy lists FPGAs and hardware accelerators as nice-to-have, and supporting them on AWS among the responsibilities. Most likely it's an **FPGA**: a programmable chip in which logic (for example, parallel pattern comparators) is "burned in" as a circuit.
- On AWS, FPGAs are available in **F2** instances (the previous generation is F1). The firmware is packaged as an **AFI (Amazon FPGA Image)** and loaded onto the FPGA when the instance starts. Drivers, the SDK and AFI loading are the job of the image and systemd units — one more reason for an AMI.
- Where else FPGAs are used: Microsoft Azure (project Catapult) accelerates networking and Bing search on them, trading firms process market data and send orders in microseconds, telecom does packet processing.

### Why all of this leads to AMIs
1. The artifact is the whole host: kernel parameters, CPU isolation, hugepages, FPGA drivers and AFI loading, network tuning. A container doesn't have any of that.
2. An orchestrator would add jitter for the sake of one service on a dedicated machine.
3. A security product with compliance: a minimal, validated, versioned host.
4. Customer-hosted and on-prem deployment is planned: an AMI can be shared into the customer's account or Marketplace, and the same Ansible roles build an image for on-prem.
5. A small team: ASG + Launch Template + AMI is simpler than running Kubernetes for a few hosts.

**What to ask them in the interview:**
- does the service run directly on the host via systemd or in a container inside the AMI;
- what hardware: F2 with FPGAs, network instances with DPDK, something else;
- how image rollout and rollback work: instance refresh, blue/green, canary;
- how they plan to ship to customers: Marketplace, AMI sharing, an on-prem appliance;
- what the latency SLOs are and how P99 end-to-end is measured.

## Host requirements for such a product: what and why

Items from the Platform & SRE vacancy, one by one: what it is, why it's needed and where you meet it. Commands and tuning details are in the topic [Linux OS and the file system](#06-linux.md/s10).

### "Own the lifecycle of Linux runtime hosts"
**What it is.** Being responsible for the host along its whole path: image creation → configuration → updates (kernel, packages, drivers) → hardening → troubleshooting live servers → decommissioning.
**Why.** When the host itself is part of the product, its behaviour (latency, stability, security) depends on the kernel version, the driver and every setting. Someone has to own that as code and as a product.
**What it looks like in practice.** Hosts are immutable: a kernel or driver update means a new image, validation and replacing servers, not `yum update` on a live machine. Plus troubleshooting: why P99 grew, why packets are dropped on the NIC, why the service doesn't start after an update.

### "Build, validate and version AWS AMIs for production"
**What it is.** An image pipeline: build (Packer + Ansible or Image Builder), validation (image tests, vulnerability scan, a test launch, a latency check), version and tags, promotion dev → staging → prod, rollout with rollback, removing old ones.
**Why.** The image is the product release. An unvalidated image in production is an unvalidated release.
**Example.** The sections above in this topic and the pipeline in [AMI lifecycle](#07-docker.md/s15).

### CPU pinning
**What it is.** Binding a process or thread to specific CPU cores (`taskset`, `CPUAffinity=` in systemd), often together with **isolating cores** from the scheduler (`isolcpus`, `nohz_full` in kernel boot parameters): the OS doesn't put other tasks on isolated cores and interrupts them with the timer less often.
**Why.** The thread doesn't migrate between cores, its L1 and L2 caches stay "warm", neighbouring processes don't evict it. This removes jitter, i.e. makes the latency tail predictable.
**Where it's used.** Trading, DPDK applications (each core spins its own packet queue), telecom, real-time audio, the Kubernetes CPU Manager with the `static` policy for pods with guaranteed CPUs (kubelet gives a Guaranteed-class pod with a whole number of CPUs exclusive cores).
```bash
# Example: isolate cores 2-7 at boot and pin the service to them
grubby --update-kernel=ALL --args="isolcpus=2-7 nohz_full=2-7"
systemctl set-property dataplane.service AllowedCPUs=2-7
```
In detail: [CPU: frequency, sleep, core isolation](#06-linux.md/s10).

### NUMA
**What it is.** Non-Uniform Memory Access: in multi-socket servers each processor has its own "near" memory and its own PCIe devices. Accessing the other socket's memory is noticeably slower.
**Why.** If a process runs on cores of one NUMA node while its memory and NIC belong to another, every access goes over the inter-processor link: latency grows and throughput drops. The process, its memory, the NIC and the FPGA are kept on one node.
**Where it's used.** Databases, DPDK, virtualisation (a VM's vCPUs on one node), HPC (High-Performance Computing — scientific and engineering computing on clusters). On EC2 it's relevant for large instances with two sockets.
```bash
lscpu | grep -i numa                          # how many NUMA nodes and which cores belong where
numactl --cpunodebind=0 --membind=0 ./server  # CPU and memory from node 0 only
```
In detail: [NUMA](#06-linux.md/s11).

### Hugepages
**What it is.** Memory pages of 2 MB or 1 GB instead of the usual 4 KB. Allocated in advance, usually at OS boot.
**Why.** The processor caches address translations in a small TLB cache. With large pages it covers hundreds of times more memory, there are fewer TLB misses — memory access is faster and more stable. Plus memory is reserved in advance and doesn't fragment.
**Where it's used.** DPDK requires hugepages for packet buffers; Oracle and PostgreSQL (`huge_pages`), JVMs with large heaps, virtualization.
```bash
grubby --update-kernel=ALL --args="default_hugepagesz=1G hugepagesz=1G hugepages=8"  # 8 pages of 1 GB at boot
grep Huge /proc/meminfo                                                         # check after a reboot
```
In detail: [Memory: hugepages, THP, swap](#06-linux.md/s12).

### IRQ affinity
**What it is.** Binding device interrupts (first of all the NIC queues) to specific cores: `/proc/irq/<N>/smp_affinity_list`.
**Why.** Every incoming packet raises an interrupt. If it lands on the core running a latency-critical thread, that thread is interrupted. Interrupts are moved to separate cores of the same NUMA node as the NIC, and the working cores are left to the application. The `irqbalance` daemon is turned off or configured so it doesn't move interrupts back.
**Where it's used.** High-load network services, load balancers, databases, trading.
```bash
grep -E 'ens5|ena' /proc/interrupts            # NIC queue interrupts
echo 1 > /proc/irq/45/smp_affinity_list        # handle interrupt 45 only on core 1
```
In detail: [Interrupts and the NIC](#06-linux.md/s13).

### NIC tuning
**What it is.** Tuning the network card and the network stack: the number of queues and how flows are spread across them (RSS), ring buffer sizes, interrupt coalescing (how long to wait before an interrupt), offloads, network sysctls. On EC2 also the ENA driver and its limits (packets per second, connections).
**Why.** A trade-off between latency and throughput: coalescing saves CPU but adds microseconds to every packet; small buffers mean packet loss on bursts. For a data plane it's tuned towards low latency, and dropped-packet counters are watched.
**Where it's used.** Any service where the network is the bottleneck: proxies, load balancers, message brokers, trading.
```bash
ethtool -l ens5                 # number of queues
ethtool -C ens5 rx-usecs 0      # don't delay interrupts (lower latency, more CPU load)
ethtool -S ens5 | grep exceeded # ENA: packets dropped because of instance limits
```
In detail: [Interrupts and the NIC](#06-linux.md/s13) and [EC2 for low latency](#06-linux.md/s15).

### DPDK
**What it is.** Data Plane Development Kit: a library through which an application works with the NIC **directly**, bypassing the kernel network stack. The NIC is detached from the kernel driver (`vfio-pci`), and the application on dedicated cores constantly polls the queues (poll mode) instead of waiting for interrupts.
**Why.** Millions of packets per second with microsecond latency: no system calls, no copying between kernel and application, no interrupts.
**The price.** Dedicated cores are 100% busy all the time; regular tools (`tcpdump`, `ss`, iptables) don't see that NIC; you need hugepages, NUMA awareness and your own protocol implementation or a library on top.
**Where it's used.** Telecom (5G UPF, VPP virtual routers), firewalls and load balancers, trading. On AWS the ENA driver supports DPDK.
In detail: [DPDK and kernel-bypass networking](#06-linux.md/s14).

### FPGAs and hardware accelerators
**What it is.** An FPGA is a programmable logic chip: you load a circuit for a specific task into it (for example, thousands of parallel pattern comparisons). Unlike a CPU, the task is executed "in wires", in parallel and in a predictable number of clock cycles.
**Why.** Deterministic and very low latency for operations that don't map well onto a CPU, without GPUs.
**Where it's used.** Trading (market data processing in microseconds), Microsoft Azure (Catapult: networking and Bing search), telecom, compression and encryption. On AWS — **F2** instances and **AFI** firmware images.
**What it means for SRE.** The driver and SDK go into the image; at boot a systemd unit loads the right AFI and checks it; FPGA health is monitored; the AFI version becomes part of the image version. In detail: [EC2 for low latency → FPGAs on AWS](#06-linux.md/s15).

### Hardening together with the CISO
**What it is.** The **CISO** (Chief Information Security Officer) is the head of information security. **Hardening** is reducing the host's attack surface: only the needed packages and services, closed ports, no password or root login, minimal role permissions (least privilege), disk encryption, IMDSv2, auditing (auditd), regular patches. **CIS Benchmarks** — published configuration checklists for each OS — are often used as the baseline.
**Why.** A security product that other people's data flows through must itself be exemplary. The CISO sets the standard and requirements; SRE turns them into image code and pipeline checks.
**Example.** A CIS Ansible role in the image build + a Lynis or OpenSCAP check in the pipeline: the build fails if the image doesn't meet the standard.

### SOC 2
**What it is.** An AICPA (US) audit standard: an independent auditor verifies that the company's controls work against the **Trust Services Criteria** — security (mandatory), availability, processing integrity, confidentiality, privacy. **Type I** — the controls are properly designed as of a specific date; **Type II** — they actually worked over a period (usually 3–12 months), and the auditor checks the evidence.
**Why.** Large customers, especially in the US, don't buy a B2B product that their data flows through without a SOC 2 report.
**How it changes SRE work.** Every change goes through code, review and CI (git history is the evidence); regular access reviews; patches and vulnerability fixes within agreed deadlines (for example, critical ones within 7 days); centralized and immutable logs; incidents are documented with root-cause analysis; backups and restores are tested. An image pipeline with versions, scanning and rollout history is ready-made evidence for the auditor.

## Networking

### Network drivers
- **bridge** (the default) — containers in a separate virtual network on the host, going out via NAT.
- **host** — the container uses the host's network directly: no isolation and no NAT. Maximum performance, but ports conflict with the host.
- **none** — no network.
- **overlay** — a network stretched over several hosts (Docker Swarm — the simple cluster orchestrator built into Docker, now almost displaced by Kubernetes).
- **macvlan/ipvlan** — the container looks like a separate device in the physical network with its own IP.

### How bridge works
```
container: eth0 (172.17.0.2) ═══ veth pair ═══ vethA1B2 ── docker0 (bridge, 172.17.0.1) ── iptables NAT ── host eth0 ── internet
```
1. Each container has its own network namespace. Docker creates a **veth pair** — a virtual cable with two ends: one becomes `eth0` inside the container, the other is attached to the `docker0` bridge on the host.
2. `docker0` works like a switch: containers in the same network see each other.
3. Outgoing traffic to the internet goes through **SNAT** (MASQUERADE in iptables): the container's address is replaced with the host's.
4. **Publishing a port** `-p 8080:80` adds a **DNAT** rule: packets to the host's port 8080 are redirected to `172.17.0.2:80`.

**The firewall trap.** Docker adds its own iptables rules bypassing UFW and firewalld (simplified wrappers over iptables in Ubuntu and RHEL). A port published with `-p 5432:5432` is **open to the whole world** even if UFW "denies" it. To make the port reachable only locally: `-p 127.0.0.1:5432:5432`.

### DNS between containers
- In the default `bridge` network containers see each other **only by IP**.
- In a **user-defined network** (`docker network create mynet`) Docker's built-in DNS server (`127.0.0.11`) works: containers find each other **by name**. Compose creates such a network automatically, which is why services there reach each other by service name.

```bash
docker network create app-net
docker run -d --name db --network app-net postgres:16
docker run --rm --network app-net postgres:16 pg_isready -h db   # the name "db" resolves
```

**`localhost` inside a container is the container itself**, not the host. To reach the host: `host.docker.internal` in Docker Desktop, and on Linux — `--add-host=host.docker.internal:host-gateway`.

**`--network container:<name>`** — the container joins another container's network namespace: a shared IP and `localhost`. This is exactly how a Pod works in Kubernetes: a service pause container holds the namespace, and the pod's containers join it.

### 🧪 Mini task

**Task.** Create a user-defined network `lab` and run nginx in it named `web`. Reach it by name from a container in the same network and from a container on the default network.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker network create lab > /dev/null
docker run -d --name web --network lab nginx:alpine > /dev/null; sleep 2
docker run --rm --network lab alpine wget -qO- http://web | grep -o '<title>.*</title>'
docker run --rm alpine wget -qO- -T 3 http://web 2>&1 | head -1
docker rm -f web > /dev/null; docker network rm lab > /dev/null
```

Expected output:

```text
<title>Welcome to nginx!</title>
wget: bad address 'web'
```

Breakdown: on a user-defined network Docker runs an embedded DNS (127.0.0.11), and containers find each other by name. The default `bridge` network has no such DNS, so the name `web` doesn't resolve. Hence the rule: create your own network to link services — Docker Compose does it automatically. Publishing ports (`-p 8080:80`) is only needed for access from the host — containers on the same network see all of each other's ports.

</details>

## Storage

Everything a container writes to its filesystem goes into its upperdir and **disappears when the container is removed**. For data that must outlive the container there are three options:

- **Volume** (`-v pgdata:/var/lib/postgresql/data`) — storage managed by Docker (`/var/lib/docker/volumes/`). Survives the container's removal. When an empty volume is first attached, the content of that folder from the image is copied into it. The right choice for database data.
- **Bind mount** (`-v /srv/config:/etc/app:ro`) — a specific folder on the host. Handy for development (code from the host is immediately visible in the container) and for configs. The downside: it depends on the host's structure and permissions (the files belong to the host's UID, and a non-root user in the container may not be able to write). Slower on a Mac.
- **tmpfs** (`--tmpfs /tmp`) — in memory: for temporary and sensitive data.

Good practice is to run the container with `--read-only` (a read-only root filesystem) and explicitly mount a tmpfs or volume where the application needs to write. This prevents an attacker from modifying the application's files.

### 🧪 Mini task

**Task.** Write a file to a named volume from one container and read it from another, after the first is gone. Then mount a host folder (bind mount) read-only and try to write to it.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker volume create data > /dev/null
docker run --rm -v data:/data alpine sh -c 'echo "saved at build 42" > /data/state.txt'
docker run --rm -v data:/data alpine cat /data/state.txt
mkdir -p /tmp/cfg && echo "port=8080" > /tmp/cfg/app.conf
docker run --rm -v /tmp/cfg:/etc/app:ro alpine sh -c 'cat /etc/app/app.conf; echo x > /etc/app/new 2>&1'
docker volume rm data > /dev/null
```

Expected output:

```text
saved at build 42
port=8080
sh: can't create /etc/app/new: Read-only file system
```

Breakdown: data in a volume lives separately from the container: the first container is gone (`--rm`), but the file remains. That's how databases and anything that must survive recreating the container are stored. A volume is managed by Docker and lives inside its directory (inside the VM on macOS). A bind mount shows a specific host folder to the container — handy for configs and development. The `:ro` flag protects it from writes by the container.

</details>

## Resources, restarts, logs
```bash
docker run -d --name api \
  --memory 512m --memory-swap 512m \   # memory limit; swap = memory means "no swap". Exceeding it → OOM kill (code 137)
  --cpus 1.5 \                          # CPU limit: exceeding it → throttling, not a kill
  --pids-limit 200 \                    # maximum number of processes
  --restart unless-stopped \            # restart policy: no | on-failure[:N] | always | unless-stopped
  --log-opt max-size=10m --log-opt max-file=3 \   # log rotation
  --read-only --tmpfs /tmp \
  --cap-drop ALL \                      # drop all capabilities...
  --security-opt no-new-privileges \    # forbid privilege escalation (via SUID binaries)
  -p 127.0.0.1:8080:8080 \
  myapp:1.0
```
**Logs.** An application in a container should write its logs to **stdout and stderr**, not to files. Docker collects them with a logging driver (a Docker module that decides where the container's output goes: a file, journald, syslog, CloudWatch; read via `docker logs`), Kubernetes — via the runtime into files on the node, from where an agent picks them up (Fluent Bit, Promtail — the agent for Loki). The default `json-file` driver has **no rotation**, and a chatty container's logs fill up the disk (`/var/lib/docker/containers/<id>/<id>-json.log`). Rotation is best set globally in `/etc/docker/daemon.json`:
```json
{ "log-driver": "json-file", "log-opts": { "max-size": "10m", "max-file": "3" } }
```

### 🧪 Mini task

**Task.** Run a container that prints a line and exits with code 1, with the `--restart on-failure:3` policy. After 10 seconds see how many times Docker restarted it, what state it's in and what is in the logs.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run -d --name crash --restart on-failure:3 alpine sh -c 'echo "started"; exit 1' > /dev/null
sleep 10
docker inspect -f 'restarts={{.RestartCount}} status={{.State.Status}} exit={{.State.ExitCode}}' crash
docker logs crash
docker rm crash > /dev/null
```

Expected output:

```text
restarts=3 status=exited exit=1
started
started
started
started
```

Breakdown: the first run plus three restarts give four log lines, after which Docker gave up. The delay between attempts grows (100 ms, 200 ms, 400 ms…) so a crashing container doesn't load the host. `docker logs` reads the container's stdout and stderr, which is why apps in containers log to stdout, not to a file. In Kubernetes the same behaviour is `CrashLoopBackOff`, except restarts never stop and the delay grows up to 5 minutes.

</details>

## Security
- **Not root.** Set `USER` with a numeric UID. Root in a container, given a runtime vulnerability or a wrong mount, is root on the host.
- **A minimal base image and multi-stage** — fewer components, fewer vulnerabilities.
- **No secrets in the image.** Not via `ENV`, `ARG` or `COPY`. Remember: a file deleted in a later layer remains in the image. At build time — secret mounts; at run time — secrets from the orchestrator (Kubernetes Secrets, Secrets Manager).
- **Vulnerability scanning:** Trivy, Grype (open-source scanners: they match the image's packages against databases of known CVE vulnerabilities), Docker Scout, scanning in ECR. In CI — a threshold: for example, the build fails on CRITICAL vulnerabilities that have a fix available.
- **SBOM** (a list of components: `syft`, `--sbom`) and **image signing** (cosign, Notation — tools that cryptographically sign an image in the registry so that at launch you can verify it was built by our CI and not tampered with). In Kubernetes an admission controller (Kyverno, Sigstore policy-controller) can let only signed images through.
- **Pinning and updating.** Pin the base image version (tag plus digest) and rebuild regularly on a fresh one (Renovate, Dependabot — bots that open merge requests themselves to bump a dependency or base image version) — otherwise the base image's security fixes won't reach yours.
- **A Dockerfile linter** — `hadolint` (finds typical mistakes: `apt-get` without cache cleanup, `latest`, the shell form of ENTRYPOINT).
- **Runtime:** `--cap-drop ALL` and add back only what's needed, `no-new-privileges`, `--read-only`, the default seccomp profile, **never** `--privileged` or mounting `docker.sock` unless absolutely necessary. Rootless Docker or Podman.

### 🧪 Mini task

**Task.** Compare running Alpine by default and in a "hardened" mode: an unprivileged user, a read-only filesystem, all capabilities dropped. In hardened mode try to create a file and change an owner.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run --rm alpine id
docker run --rm --user 1000:1000 --read-only --cap-drop ALL --tmpfs /tmp alpine sh -c '
  id
  touch /x 2>&1
  touch /tmp/ok && echo "/tmp is writable"
  chown 0 /tmp/ok 2>&1'
```

Expected output:

```text
uid=0(root) gid=0(root) groups=0(root),1(bin),2(daemon),3(sys),4(adm),6(disk),10(wheel),11(floppy),20(dialout),26(tape),27(video)
uid=1000 gid=1000 groups=1000
touch: /x: Read-only file system
/tmp is writable
chown: /tmp/ok: Operation not permitted
```

Breakdown: by default the process in a container is root. It's the same uid 0 as on the host, just limited by namespaces and capabilities, and it stays root if it escapes the container. An unprivileged user, a read-only root and dropped capabilities close most exploitation paths. Where the app needs to write, give it a `tmpfs` or a volume. In Kubernetes the same is set in `securityContext`: `runAsNonRoot`, `readOnlyRootFilesystem`, `capabilities.drop: [ALL]`. In a Dockerfile the user is set with `USER`.

</details>

## Docker Compose — a local environment
Compose describes several related containers in one file: the application, the database, a cache, a queue.
```yaml
# compose.yaml
services:
  api:
    build:
      context: .
      target: runtime                  # a multi-stage stage
    ports:
      - "8080:8080"
    environment:
      ConnectionStrings__Db: "Host=db;Database=app;Username=app;Password=app"   # db — the service name as the host
    depends_on:
      db:
        condition: service_healthy      # wait until the database is really ready, not just until the container starts
  db:
    image: postgres:16
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: app
      POSTGRES_DB: app
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app"]
      interval: 5s
      retries: 10
volumes:
  pgdata:
```
```bash
docker compose up -d --build     # build and start in the background
docker compose logs -f api
docker compose exec db psql -U app
docker compose down              # stop and remove the containers (the data in the volume stays)
docker compose down -v           # and remove the volumes — the database data will be lost
```
Without `condition: service_healthy` Compose starts `api` right after the `db` container **starts**, when Postgres isn't accepting connections yet, and the application crashes.

### 🧪 Mini task

**Task.** Describe nginx and Redis with a healthcheck in `compose.yaml`, so that nginx starts only after Redis is healthy. Bring the environment up, look at the service states and tear it down together with the volumes.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > compose.yaml <<'EOF'
services:
  cache:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 2s
      retries: 10
    volumes: [cache-data:/data]
  web:
    image: nginx:alpine
    ports: ["8080:80"]
    depends_on:
      cache:
        condition: service_healthy
volumes:
  cache-data:
EOF
docker compose up -d --wait > /dev/null 2>&1
docker compose ps --format '{{.Service}} {{.State}} {{.Health}}'
curl -s -o /dev/null -w 'web: %{http_code}\n' http://localhost:8080
docker compose down -v > /dev/null 2>&1
```

Expected output:

```text
cache running healthy
web running
web: 200
```

Breakdown: `depends_on` with `condition: service_healthy` waits not just for the Redis container to start but for a passing healthcheck — otherwise the app could start before the database and crash. `--wait` returns once all services are running and healthy, which makes it handy for bringing up an environment for integration tests in CI. `web` has no healthcheck, so its Health column is empty. `down -v` also removes named volumes — without `-v` the Redis data would remain until the next run.

</details>

## Commands — cheat sheet
```bash
# images
docker build -t app:1.0 --target runtime --build-arg DOTNET_VERSION=8.0 .
docker buildx build --platform linux/amd64,linux/arm64 -t registry/app:1.0 --push .
docker images; docker image ls --digests; docker rmi app:1.0
docker tag app:1.0 registry/app:1.0 && docker push registry/app:1.0
aws ecr get-login-password --region us-east-2 | docker login --username AWS --password-stdin 123456789012.dkr.ecr.us-east-2.amazonaws.com

# containers
docker run --rm -it --entrypoint sh app:1.0      # get into the image without starting the application
docker ps -a                                     # all containers, including stopped ones
docker logs -f --tail 100 api
docker exec -it api sh                           # a shell in a running container
docker inspect -f '{{.State.ExitCode}} {{.State.OOMKilled}}' api   # why it crashed
docker stats                                     # resource consumption in real time
docker diff api                                  # what the container changed in its filesystem
docker cp api:/app/logs/error.log .              # copy a file out of the container

# cleanup
docker system df -v                              # what takes how much space
docker image prune -a                            # remove unused images
docker builder prune                             # remove the build cache
docker system prune -a --volumes                 # remove EVERYTHING unused, including volumes — careful
```

### 🧪 Mini task

**Task.** See how much space Docker uses (images, containers, volumes, build cache), remove dangling images and the build cache, and look at the result again.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker system df --format '{{.Type}}: {{.Size}} (reclaimable {{.Reclaimable}})'
docker image prune -f > /dev/null
docker builder prune -f > /dev/null
echo ---
docker system df --format '{{.Type}}: {{.Size}} (reclaimable {{.Reclaimable}})'
```

Expected output:

```text
Images: 1.42GB (reclaimable 1.18GB (83%))
Containers: 0B (reclaimable 0B)
Local Volumes: 0B (reclaimable 0B)
Build Cache: 312MB (reclaimable 312MB)
---
Images: 1.38GB (reclaimable 1.14GB (82%))
Containers: 0B (reclaimable 0B)
Local Volumes: 0B (reclaimable 0B)
Build Cache: 0B (reclaimable 0B)
```

Breakdown: "reclaimable" is how much can be freed without touching anything in use. `image prune` without `-a` removes only dangling images (`<none>`) left over from rebuilds under the same tag. With `-a` every image without a container goes. The build cache grows fast on CI runners and workstations. On servers and runners this cleanup goes into cron, while `docker system prune -a --volumes` is used carefully: it also deletes data in unused volumes. Your sizes, versions and IDs will differ.

</details>

## Interview questions

**How does a container differ from a virtual machine?**
A VM has its own kernel on top of a hypervisor: strong isolation, but gigabytes and an OS boot. A container is an isolated process on the host's shared kernel: megabytes, start in seconds, high density, but weaker isolation (a kernel vulnerability affects all containers). For strong isolation there are gVisor (its own "kernel" in user space intercepts the container's system calls), Kata Containers and Firecracker (micro-VMs that start almost like a container; AWS Lambda and Fargate run on Firecracker).

**Which kernel mechanisms is a container "assembled" from?**
Namespaces (what the process sees: PIDs, network, filesystem, hostname, users), cgroups (how many resources it may consume), overlayfs (a root filesystem from the image layers), capabilities and seccomp (reduced privileges).

**What is an image layer? Why does the order of instructions in a Dockerfile matter?**
A layer is an archive of filesystem changes created by a `RUN`, `COPY` or `ADD` instruction. Layers are cached, and when one changes, all the following ones are rebuilt. That's why dependencies are copied and installed before copying the code: otherwise any code change reinstalls all the packages.

**A file was deleted in the next `RUN` instruction, but the image didn't shrink. Why?**
Layers are immutable: deletion creates a whiteout file in the new layer that hides the file, but the file itself stays in the previous layer. You need to create and delete in one instruction or use multi-stage.

**What is a multi-stage build and why is it needed?**
Several `FROM` stages in one Dockerfile. The build happens in a stage with the SDK, and only the result is copied into the final image. The outcome: a small image without a compiler and sources (security), one Dockerfile, parallel stage builds, and you can have a separate test stage.

**How do you pass a secret at build time?**
`RUN --mount=type=secret,id=...` and `docker build --secret`: the secret is available only during the command and doesn't end up in the layer. `ARG` and `ENV` are visible in `docker history`, and `COPY` + `rm` leave the file in the layer.

**How do you speed up image builds in CI if the runners are ephemeral?**
The right layer order, `.dockerignore`, cache mounts for package managers, exporting the BuildKit cache to a registry (`--cache-to/--cache-from type=registry,mode=max`) or to the CI cache (`type=gha`), a shared base image.

**How does CMD differ from ENTRYPOINT? The exec form from the shell form?**
ENTRYPOINT is the executable, CMD is the default arguments (replaced by the `docker run` arguments). In the exec form (`["app"]`) the application becomes PID 1 and receives signals. In the shell form PID 1 is `/bin/sh`, which doesn't pass SIGTERM on to the application.

**`docker stop` waits 10 seconds, and the application doesn't shut down cleanly. Why?**
The application doesn't receive SIGTERM (a shell-form ENTRYPOINT or a script without `exec`), or as PID 1 it ignores it without a handler. After the timeout SIGKILL arrives. The fix: the exec form, `exec` in the wrapper script, a SIGTERM handler in the application, `--init` or tini.

**What do exit codes 0, 1, 125, 126, 127, 137, 139, 143 mean?**
0 — success; 1 — an application error; 125 — an error of `docker run` itself; 126 — the command isn't executable; 127 — the command wasn't found; 137 = 128+9 — SIGKILL (most often OOM, check `OOMKilled`); 139 = 128+11 — segfault; 143 = 128+15 — SIGTERM, a normal stop.

**Why isn't Alpine always a good choice?**
Musl libc instead of glibc: binaries built for glibc don't work, the DNS resolver behaves differently, performance is sometimes lower, and there are no prebuilt binary packages for Python (builds take longer and the image ends up bigger). The alternatives are slim or distroless.

**How do you debug a container from a distroless image without a shell?**
`kubectl debug -it <pod> --image=busybox --target=<container>` — a temporary container with tools in the same process namespace. In Docker — `docker run -it --pid=container:<id> --network=container:<id> nicolaka/netshoot` (an image with a set of network tools: tcpdump, dig, curl, ss). Or the image's `:debug` variant, or `nsenter` from the host.

**Why don't containers find each other by name?**
They're in the default `bridge` network, which has no name-based DNS. You need a user-defined network (`docker network create`): the built-in DNS at 127.0.0.11. In Compose — use the service name. And `localhost` inside a container is the container itself.

**How does publishing a port with `-p 8080:80` work?**
Docker adds a DNAT rule to iptables: packets to the host's port 8080 are redirected to the container's IP, port 80. These rules bypass UFW, so the port is open to the outside; for local access — `-p 127.0.0.1:8080:80`.

**How does a volume differ from a bind mount?**
A volume is managed by Docker, doesn't depend on the host's structure, and is filled with content from the image on first attachment — for data. A bind mount is a specific host folder — for development and configs.

**What happens to a container's data after `docker rm`?**
Everything written to the container's filesystem (the writable layer) is deleted. Only what lives in volumes or bind mounts is kept.

**How do you build images in CI running in Kubernetes? What are the risks of Docker-in-Docker?**
DinD requires `--privileged` (effectively root on the node). Passing through `docker.sock` is also root on the host, plus a daemon shared between jobs. Builders without a daemon and without privileges are better: BuildKit rootless, Buildah.

**How do you reduce an image's attack surface?**
A minimal base image (distroless, chiseled), multi-stage, a non-root user, no shell or package manager, regular rebuilds on a fresh base image, scanning, signing. At run time — `--read-only`, `--cap-drop ALL`, `no-new-privileges`.

**What is a multi-arch image? What does the `exec format error` mean?**
A manifest list with a separate image for each architecture (amd64, arm64); the client pulls the right one itself. Built with `buildx --platform`. `exec format error` — an image of the wrong architecture was started (for example, an image built only for arm64 on an M1 Mac, running on an x86 server).

**How do Docker, containerd, CRI-O and Podman differ?**
Docker is the full kit (CLI, daemon, builds, networks) on top of containerd. Containerd is a runtime daemon used by Kubernetes directly. CRI-O is a runtime made specifically for Kubernetes (OpenShift). Podman is a Docker CLI equivalent without a daemon and rootless by default. They all run the same OCI images via runc or crun.

**Why was dockershim removed from Kubernetes, and what broke?**
Kubelet talked to Docker through an extra layer, although Docker itself uses containerd — an unnecessary link. Images didn't break, because they're standard OCI. What broke was anything that used `docker.sock` on the nodes: DinD builds, some monitoring and logging agents.

**`docker system df` shows 50 GB. What can be cleaned up?**
Unused images (`docker image prune -a`), the build cache (`docker builder prune`), stopped containers (`docker container prune`), unused volumes (`docker volume prune` — careful, that's data). Separately check container logs without rotation in `/var/lib/docker/containers/`.

**How is a Docker image different from an AMI?**
A Docker image contains an application and its dependencies and runs on the host's kernel; it's stored as layers in a registry and builds in seconds thanks to the cache. An AMI is a disk snapshot with a full OS, including the kernel and boot loader, that an EC2 instance is launched from; it builds in 5–20 minutes without a layer cache, and its ID is valid in one region. In practice the AMI prepares the host (for example, a Kubernetes node), and the Docker image prepares the application on it.

**What happens during `packer build` with the `amazon-ebs` builder?**
Packer finds the base AMI, creates a temporary key and security group, launches an instance, connects over SSH and runs the provisioners (shell, Ansible). Then it stops the instance, creates snapshots from its volumes and registers the AMI, adds tags, copies it to other regions if needed and deletes all temporary resources.

**Packer or EC2 Image Builder — which would you choose?**
Packer — if there are several clouds or on-prem, the build already lives in CI, the configuration is written in Ansible and fast debugging matters. Image Builder — if it's AWS only and you need a managed pipeline: scheduled builds when a new base is out, built-in tests on a separate instance, Inspector scanning, distribution to accounts and lifecycle policies without your own plumbing. They can be combined: the base image in Image Builder, application images on top of it in Packer.

**How does Terraform learn the new AMI ID after a build?**
The ID changes with every build and differs per region, so it isn't hard-coded. The build publishes the ID to SSM Parameter Store (for example, `/images/web/latest`), Terraform reads it with `data "aws_ssm_parameter"` and puts it into the Launch Template. A rollback means putting the previous ID back into the parameter and applying again.

**How does EC2 Image Builder build an image without SSH?**
AWSTOE runs the component steps through the SSM Agent on the build instance, so no keys and no open port 22 are needed. The instance needs a role with `EC2InstanceProfileForImageBuilder` and `AmazonSSMManagedInstanceCore` and access to SSM (internet or VPC endpoints).

**Old AMIs were deleted, but the storage bill didn't go down. Why?**
`deregister-image` removes only the image registration; the EBS snapshots stay and are billed. They have to be deleted separately (`delete-snapshot`), or an Image Builder lifecycle policy that deletes images together with their snapshots has to be set up.

**Why is a latency-critical product often shipped as an AMI rather than a container in Kubernetes?**
Its artifact is the whole host: CPU isolation, hugepages and kernel parameters are set at boot, it needs accelerator drivers (FPGA) and a NIC in DPDK mode — a container on someone else's kernel can't set any of that. An orchestrator adds jitter (overlay network, cgroup quotas, neighbouring pods) for the sake of one service on a dedicated machine. Plus an image is easy to hand to a customer as an appliance: into their account, into Marketplace, or built from the same code for on-prem.

**What does the "8 ms P99 end-to-end" metric mean?**
99% of requests, from sending to receiving the full response, complete within 8 ms; only 1% are slower. End-to-end includes the network, queues and all processing, not just the engine time. For a service in the path of every request the tail is what matters: agents make chains of dozens of calls, and a rare slow request shows up in almost every chain.

**Why is pattern matching moved into hardware, and what does "0 GPU" mean?**
In software the time grows with the number of patterns; algorithms like Aho-Corasick and Hyperscan soften that, and an FPGA runs thousands of comparisons in parallel, so throughput hardly depends on their number. "0 GPU" means the check is deterministic, without classifier-model inference: no expensive and scarce GPUs, predictable latency, the same input gives the same result.

**What is SOC 2, and how does it change SRE work?**
An audit of controls against the Trust Services Criteria (security, availability, processing integrity, confidentiality, privacy); Type II confirms the controls worked over a period. For SRE it means changes only through code and review, regular access reviews, patches within agreed deadlines, centralized immutable logs, incident root-cause analysis and tested backups. A versioned image pipeline with rollout history serves as evidence for the auditor.

**What does "owning the lifecycle of runtime hosts" mean?**
Being responsible for the host from image creation to decommissioning: configuration, kernel and driver updates, hardening, troubleshooting on live servers. Hosts are immutable: any update is a new image, validation and replacing servers with the ability to roll back, not changing a live machine.

**How would you build the AMI lifecycle?**
Packer plus Ansible roles in git; building in CI from an official base image; CIS hardening and a vulnerability scan; launching a test instance and automatic checks (kernel parameters, services, health check, a short load test); tags with the version and commit; promotion via an SSM parameter that the Launch Template refers to; rollout via ASG Instance Refresh with a canary, alarms and auto-rollback; deprecation and deletion of old images together with their snapshots.

**Bake or fry — which would you choose?**
For identical production hosts — bake: a fast and predictable start, no dependency on external repositories, all hosts identical, and the image checked in advance. Especially since kernel parameters require a reboot. At launch — only environment configuration and secrets from SSM or Secrets Manager. Fry fits fast-changing dev environments.

**How do you roll back a bad image?**
Put the previous ID back into the SSM parameter (the Launch Template refers to it) and start an Instance Refresh, or rely on AutoRollback if an alarm stopped the refresh. That's why old images are deleted not immediately but according to the retention policy, and the last verified versions are always available.

**Why won't an instance launch in another account from a shared encrypted AMI?**
The image's snapshots are encrypted with the owner account's KMS key, and the recipient account has no rights to that key. You need to grant it access in the key policy (and for Auto Scaling, also to the Auto Scaling service-linked role) or copy the image, re-encrypting it with your own key.

**What should you do to an image before the snapshot?**
Clean cloud-init (`cloud-init clean`), empty the `machine-id`, delete temporary files, build keys, shell history and logs. Otherwise all instances inherit identical identifiers, cloud-init won't run the user data, and traces of the build remain in the image. Secrets aren't put into the image at all.

**How do you move such a platform to a customer (on-prem)?**
One source of truth — the Ansible roles; Packer builds both the AMI and images for KVM or VMware from them. An installation and update procedure the customer can perform themselves, environment checks before installation (CPU, NUMA, network cards, kernel version) and a diagnostic bundle for collecting logs and state, because there won't be direct access.
