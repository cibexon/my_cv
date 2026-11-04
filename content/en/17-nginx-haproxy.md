# Nginx and HAProxy

A detailed look at the two most widely used proxies and load balancers: how they work inside, how they handle connections, how they decide where to send a request, and which settings come up in interviews. Each topic covers what it is, why it exists, how it works, a config with explanations, and common mistakes. All configs are validated with the built-in checks (`nginx -t`, `haproxy -c`).

## Why reverse proxies and load balancers exist

A **forward proxy** sits on the client side and goes to the internet on the client's behalf (a corporate proxy). A **reverse proxy** sits in front of servers and accepts requests **instead of them**: the client thinks it is talking to the site, but it is actually talking to the proxy, which passes the request on.

What a reverse proxy gives you:
- **load balancing** — requests are spread across several application instances, unhealthy ones are excluded;
- **TLS termination** — certificates and encryption in one place, applications receive plain HTTP;
- **a single entry point** — one address for many services, routing by domain and path;
- **protection** — request limits, body size limits, filtering, hiding the internal layout;
- **caching and static files** — takes load off the application;
- **buffering slow clients** — the proxy itself "finishes reading" the request and "finishes writing" the response to a slow mobile client, while the application is freed immediately.

**Nginx** is a general-purpose web server and reverse proxy: serves static files, proxies HTTP, caches, and does a lot at the HTTP level. **HAProxy** is a specialised L4 and L7 load balancer: queues, detailed health checks, rich statistics, managing servers on the fly. A detailed comparison is at the end of the topic.

### 🧪 Mini task

**Task.** Put nginx in front of the `app1` backend as the single entry point. Make a request through the proxy and try to reach the backend directly from the host. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done

cat > default.conf <<'EOF'
server {
    listen 80;
    location / { proxy_pass http://app1:5678; }
}
EOF
docker run -d --name ngx --network lb -p 8080:80 -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2

curl -s http://localhost:8080/
curl -s -m 2 http://localhost:5678/ || echo "backend is not reachable from outside"
docker rm -f ngx app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
app1
backend is not reachable from outside
```

Breakdown: only the proxy is reachable from outside, while the backend lives on the internal network with its port unpublished. That's the main benefit of a reverse proxy: one entry point concentrating TLS, load balancing, caching, limits and logs, while backends can be changed, scaled and hidden. The client doesn't even know how many there are or where. The same principle in the cloud: an ALB in a public subnet, apps in private ones.

</details>

## Nginx: how it works inside

### Master and worker processes
```
master (root) — reads the config, opens ports 80/443, manages workers
 ├─ worker (nginx) — handles connections
 ├─ worker (nginx)
 ├─ worker (nginx)
 └─ worker (nginx)
```
- The **master process** runs as root (to open ports below 1024 and read keys), reads and validates the configuration, creates the listening sockets and starts the workers. It does not handle requests itself.
- **Worker processes** run as an unprivileged user and serve **all** connections. Usually one worker per core: `worker_processes auto;`.

### The event model: why nginx holds tens of thousands of connections
Older servers (classic Apache with prefork — the "pre-started process per connection" model) allocated **a process or thread per connection**. 10,000 slow clients — 10,000 threads, each with its own memory stack, and the kernel spends time switching between them. This is the well-known C10K problem (how to serve 10 thousand simultaneous connections on one server — a challenge posed in 1999).

Nginx works differently: each worker is **one thread with an event loop**:
1. All the worker's sockets are put into **non-blocking** mode (a `read` call doesn't wait for data but immediately returns "nothing yet").
2. The worker asks the kernel via **epoll** (Linux) or kqueue (BSD, macOS) — system mechanisms that report in a single call which of thousands of sockets are ready: "which of my sockets have an event — data arrived, writable, new connection?"
3. It handles the ready events one by one: read part of a request, sent part of a response, forwarded to the backend — and moves straight to the next socket. Nobody waits for anybody.
4. Back to step 2.

A connection that is doing nothing (a client slowly reading a response, keep-alive between requests) costs just a little memory, not a whole thread. That is why one worker serves thousands of connections.

**The downside:** if something inside a worker **blocks** the thread (reading from a slow disk, a heavy Lua script), all of that worker's connections stall. For disk operations there's a thread pool (`aio threads`: file reads go to separate threads, and the event loop doesn't wait).

### How many connections nginx can handle
```nginx
worker_processes auto;            # one per CPU core
worker_rlimit_nofile 65535;       # open-file limit for a worker — sockets are files too

events {
    worker_connections 8192;      # max simultaneous connections per worker
}
```
Maximum clients ≈ `worker_processes × worker_connections`. **When proxying, divide by two**: each client takes two connections — client to nginx and nginx to backend. `worker_connections` is capped by the process open-file limit, so `worker_rlimit_nofile` must be at least as large. If it runs out, the error log shows `worker_connections are not enough`.

### Reloading without downtime
```bash
nginx -t            # always test the config first
nginx -s reload     # or: kill -HUP <master pid>, or systemctl reload nginx
```
What happens:
1. The master receives **HUP** and validates the new configuration. **If it has an error, the master keeps running with the old one** and logs the error.
2. The master starts **new workers** with the new configuration. The listening sockets stay open — new connections are accepted by the new workers right away.
3. The old workers get a **graceful shutdown** command: they stop accepting new connections, finish the current ones and exit.

Not a single connection is dropped. A consequence: with long-lived connections (WebSocket — a persistent two-way connection for chats and notifications, large file downloads) the old workers can live a long time — hours, holding memory; this can be limited with `worker_shutdown_timeout`.

Signals to the master process:
| Signal | Action |
|---|---|
| `HUP` | Re-read the configuration (reload) |
| `QUIT` | Graceful stop: finish connections and exit |
| `TERM`, `INT` | Fast stop |
| `USR1` | Reopen log files (used by logrotate after renaming logs) |
| `USR2` + `WINCH` | Upgrade the nginx binary without downtime: start a new master next to the old one, then shut down the old workers |

### 🧪 Mini task

**Task.** Run nginx in a container and look at its processes: the master and the workers. Run `nginx -s reload` and check whether the master and worker PIDs changed.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run -d --name ngx nginx:alpine > /dev/null; sleep 2
procs() { docker exec ngx ps -o pid,args | grep 'nginx:' | grep -v grep; }
before=$(procs)
echo "$before" | awk '{print $2, $3, $4}' | sort | uniq -c
docker exec ngx nginx -s reload 2> /dev/null; sleep 2
after=$(procs)
[ "$(echo "$before" | grep master)" = "$(echo "$after" | grep master)" ] && echo "master PID unchanged"
[ "$(echo "$before" | grep worker)" != "$(echo "$after" | grep worker)" ] && echo "workers replaced"
docker rm -f ngx > /dev/null
```

Expected output:

```text
      1 nginx: master process
      4 nginx: worker process
master PID unchanged
workers replaced
```

Breakdown: the master reads the config, opens the ports and manages the workers, while the workers handle requests — one per core (`worker_processes auto`), each with an epoll event loop holding thousands of connections in a single thread. On `reload` the master validates the new config and starts new workers, while the old ones finish their current requests and exit — that's why reloading the configuration doesn't drop connections. If the config is broken, the master keeps the old workers. Your worker count equals the number of CPUs given to Docker.

</details>

## Nginx: configuration structure

### Contexts
The configuration consists of nested blocks (contexts). Directives set higher up are **inherited** by nested blocks unless overridden there.
```nginx
user nginx;                         # main — global process settings
worker_processes auto;

events {                            # connection handling settings
    worker_connections 4096;
}

http {                              # everything related to HTTP
    include       mime.types;
    sendfile      on;

    upstream app_backend {          # a group of backends
        server 10.0.1.10:8080;
        server 10.0.1.11:8080;
    }

    server {                        # a virtual server: address, port and domain names
        listen 80;
        server_name example.com;

        location / {                # rules for request paths
            proxy_pass http://app_backend;
        }
    }
}

stream {                            # TCP and UDP proxying (L4): databases, MQTT, DNS
    server {
        listen 5432;
        proxy_pass 10.0.2.20:5432;
    }
}
```
An inheritance trap: array-like directives such as `proxy_set_header` and `add_header` are inherited **only if no such directive is set at the current level**. Add a single `add_header` in a `location`, and all `add_header` directives from `server` disappear for that location.

### How nginx picks a server
When a request arrives, nginx first picks a `server` block:
1. It selects servers whose `listen` matches the address and port the connection came to.
2. Among them it looks up the `Host` header (or SNI for TLS) in `server_name`, in priority order:
   1. **exact name** — `example.com`;
   2. the longest name with a **leading wildcard** — `*.example.com`;
   3. the longest name with a **trailing wildcard** — `mail.*`;
   4. the first matching **regular expression** in config order — `~^(?<sub>.+)\.example\.com$`.
3. If nothing matches — the server marked `default_server` for that port, and without one — the **first** server defined on that port.

The last point is a common cause of "someone else's site": a request by IP or with an unknown domain lands on whichever server comes first. Good practice is an explicit catch-all server:
```nginx
server {
    listen 80 default_server;
    server_name _;
    return 444;                     # special nginx code: close the connection without a response
}
```

### How nginx picks a location — a classic question
Inside a server nginx picks **one** location by the request path (without the query string):
1. If there is an **exact match** `location = /path` — it is used and the search stops.
2. The **longest matching prefix** among regular locations (`location /api/`) is found. If it has the **`^~`** modifier — it is used and regular expressions are not checked.
3. **Regular expressions** (`~` case-sensitive, `~*` case-insensitive) are checked **in the order they appear in the config**. The first match wins.
4. If no regular expression matches — the longest prefix from step 2 is used.

```nginx
server {
    listen 80;
    server_name example.com;

    location = /health        { return 200 "ok\n"; }                  # 1: exactly /health only
    location ^~ /static/      { root /var/www; }                      # 2: prefix, skip regexes
    location ~* \.(png|jpg)$  { root /var/www/img; expires 30d; }     # 3: regex, case-insensitive
    location /api/            { proxy_pass http://127.0.0.1:8080; }   # prefix
    location /                { proxy_pass http://127.0.0.1:3000; }   # everything else
}
```
Where requests go:
- `/health` → exact match;
- `/static/logo.png` → `^~ /static/`: the `\.png` regex is not even checked;
- `/api/avatar.PNG` → the `~*` regex beats the `/api/` prefix even though the prefix is longer (regexes are checked after prefixes and take priority);
- `/api/users` → `/api/`;
- `/about` → `/`.

### root and alias
```nginx
location /static/ {
    root /var/www;            # /static/a.css → /var/www/static/a.css (the path is APPENDED to root)
}
location /assets/ {
    alias /srv/build/;        # /assets/a.css → /srv/build/a.css (the location prefix is REPLACED)
}
```
With `alias`, watch the slashes: `location /assets/` and `alias /srv/build/` — both with a trailing slash, otherwise you get paths like `/srv/builda.css`. A careless `alias` with no slash in the location is a known vulnerability (path traversal: `/assets../etc/passwd`).

### try_files — serving an SPA and falling back to the application
`try_files` is needed, for example, for an SPA (Single Page Application — a React, Vue or Angular frontend where all the "pages" are drawn by JavaScript from one `index.html`): for any path like `/orders/42` the server must return `index.html`, and the browser then shows the right page itself.
```nginx
location / {
    root /var/www/spa;
    try_files $uri $uri/ /index.html;    # file exists — serve it; otherwise serve index.html (SPA routing)
}
```
`try_files` checks the options in order and uses the first that exists; the last argument is a fallback path or a code (`=404`).

### 🧪 Mini task

**Task.** Define four `location`s of different kinds in one `server`: exact match `=`, prefix with `^~`, regex `~` and a plain prefix. Predict which one handles each of five paths, then check with `curl`.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > default.conf <<'EOF'
server {
    listen 80;
    location = /health       { return 200 "exact\n"; }
    location ^~ /static/     { return 200 "prefix-priority\n"; }
    location ~ \.(png|jpg)$  { return 200 "regex\n"; }
    location /               { return 200 "prefix\n"; }
}
EOF
docker run --rm -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine nginx -t 2>&1 | tail -1
docker run -d --name ngx -p 8080:80 -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2
for p in /health /health/x /static/logo.png /img/logo.png /api; do printf '%-17s -> %s' $p "$(curl -s localhost:8080$p)"; echo; done
docker rm -f ngx > /dev/null
```

Expected output:

```text
nginx: configuration file /etc/nginx/nginx.conf test is successful
/health           -> exact
/health/x         -> prefix
/static/logo.png  -> prefix-priority
/img/logo.png     -> regex
/api              -> prefix
```

Breakdown: the order of `location` selection doesn't depend on the order in the file. Exact `=` is checked first, then the longest matching prefix is found; if it has `^~`, the search stops there. Otherwise regexes are checked in the order written and the first match wins, and if none match, the prefix found earlier is used. That's why `/static/logo.png` didn't hit the image regex but `/img/logo.png` did. `/health/x` isn't equal to `/health`. `nginx -t` validates the syntax before every reload.

</details>

## Nginx as a reverse proxy

### A basic proxy location and headers
```nginx
location / {
    proxy_pass http://app_backend;

    proxy_set_header Host              $host;                       # without this the backend gets Host: app_backend
    proxy_set_header X-Real-IP         $remote_addr;                # client IP
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;  # IP chain through all proxies
    proxy_set_header X-Forwarded-Proto $scheme;                     # http or https — so the app builds correct links

    proxy_connect_timeout 5s;        # establish the TCP connection to the backend
    proxy_send_timeout    60s;       # between two write operations to the backend
    proxy_read_timeout    60s;       # between two read operations — NOT for the whole response
}
```
Why the headers matter: the backend sees a connection from nginx, not from the client. Without `Host` a backend serving several sites does not know which one is wanted; without `X-Forwarded-For` the application logs show the nginx IP; without `X-Forwarded-Proto` an application behind TLS termination generates links and redirects to `http://`.

### The slash in proxy_pass — the main trap
Whether `proxy_pass` has a **URI part** (even a single `/` after the address) determines which path the backend receives:
```nginx
location /api/ {
    proxy_pass http://127.0.0.1:8080;     # no URI: the path is passed AS IS
}                                         # /api/users → http://127.0.0.1:8080/api/users

location /v2/ {
    proxy_pass http://127.0.0.1:8080/;    # with URI "/": the matched prefix /v2/ is REPLACED with "/"
}                                         # /v2/users → http://127.0.0.1:8080/users
```
This explains half of the "why does the backend return 404" questions: an extra or missing slash.

### Keepalive to backends: why reuse connections

**How a request goes through nginx.** When a user opens a site, **two separate connections** are involved:
1. **client → nginx** — the browser connects to nginx;
2. **nginx → backend** — to get the response, nginx itself connects to the application (for example, a service on port 8080) and forwards the request to it.

These are two independent TCP connections, each with its own settings. nginx keeps connections with clients open by default (`keepalive_timeout` in the `http` block), but with the second connection — **to the backend** — it is wasteful by default.

**Why opening a connection is expensive.** Before a single byte of the request can be sent, a TCP connection has to be established:
- the **three-way handshake** (SYN → SYN-ACK → ACK) — at least one network round trip;
- if the backend is reached over HTTPS — also a **TLS handshake**: key exchange and certificate verification, i.e. one or two more round trips and noticeable CPU load;
- after a connection is closed, the side that closed it keeps a record of it in the **TIME_WAIT** state for about a minute. While it lingers, that local port cannot be used for a new connection to the same address. There are about 28 thousand ports for outgoing connections, and at thousands of new connections per second they can run out — then nginx cannot connect to the backend at all (the `cannot assign requested address` error).

**What nginx does by default.** For connections to the backend nginx uses **HTTP/1.0** and adds the header **`Connection: close`** to every request — "close the connection after the response". So for **every** user request nginx opens a new connection to the backend, sends the request, receives the response and closes the connection.

An analogy: you call a colleague, ask one question, hang up, and a second later dial again with the next question. Every time — ringing and "hello", when you could simply have stayed on the line.

```text
WITHOUT a pool — every request opens and closes its own connection:
  request 1:  nginx ──[handshake]──[request]──[response]──[close]──► backend
  request 2:  nginx ──[handshake]──[request]──[response]──[close]──► backend
  request 3:  nginx ──[handshake]──[request]──[response]──[close]──► backend

WITH a pool — the connection is opened once and reused:
  nginx ──[handshake]──[request 1][response 1]──[request 2][response 2]──[request 3][response 3]──► backend
```

**A pool of persistent connections (keepalive)** — nginx does not close the connection to the backend after the response but puts it aside in a "pool" of idle connections. The next request to that backend takes an already open connection from the pool. The result: lower latency (no time spent on handshakes), less CPU load on nginx and the backend, no thousands of connections in TIME_WAIT.

**How to enable it: three settings, and all three are required.**
```nginx
upstream app_backend {
    server 10.0.1.10:8080;
    server 10.0.1.11:8080;

    keepalive 32;               # (1) enable the pool: keep up to 32 idle connections per worker
    keepalive_timeout 60s;      # how long an idle connection may sit in the pool (default 60 s)
    keepalive_requests 1000;    # after how many requests to close the connection anyway and open a new one
}

server {
    listen 80;

    location / {
        proxy_pass http://app_backend;   # refer to the upstream by name — the pool only works this way
        proxy_http_version 1.1;          # (2) talk to the backend over HTTP/1.1 instead of HTTP/1.0
        proxy_set_header Connection "";  # (3) do not send the "Connection: close" header to the backend
    }
}
```
What each of the three required lines does:
1. **`keepalive 32`** in the `upstream` block — creates the pool itself. The number is how many **idle** connections each nginx worker keeps ready for this group of servers. It is not a limit on the total number of connections: under a load spike nginx opens as many as needed and closes the extra ones after the response. The directive exists only inside `upstream`, so `proxy_pass` must point to the group name (`http://app_backend`), not directly to an address.
2. **`proxy_http_version 1.1`** — in HTTP/1.0 the connection is closed after every response by default; persistent connections there are an optional extension. In HTTP/1.1 the connection stays open by default. Without this line the backend will close the connection itself.
3. **`proxy_set_header Connection ""`** — by default nginx adds the `Connection: close` header to the request to the backend, explicitly asking it to close the connection after the response. An empty value removes this header.

If you forget any of the three, the pool formally exists but connections are still closed after every request. That is why interviews ask "which three directives are needed for keep-alive to the backend".

**How to check that the pool works.**
```bash
# on the nginx host: ESTABLISHED connections to the backend — their count should be stable, not "jumping"
ss -tan state established '( dport = :8080 )' | wc -l

# TIME_WAIT connections to the backend — thousands without a pool, a handful with one
ss -tan state time-wait '( dport = :8080 )' | wc -l
```
Another sign is the `$upstream_connect_time` variable in the nginx log (see the logs section): when a connection is reused it equals `0.000`, because there was no need to connect again.

**Where occasional 502 errors come from.** The pool has a catch: a connection sits in the pool **on both sides** — both nginx and the backend consider it open, and each decides on its own timer when to close it.
1. The connection is idle in nginx's pool.
2. The backend has its own idle timeout — for example, Node.js closes an idle connection after 5 seconds by default. The backend closes the connection.
3. nginx does not know about it yet and at exactly this moment takes the connection from the pool for a new request.
4. The request goes into an already closed connection, the backend replies with a reset (RST), and the user gets a **502**. The nginx error.log shows `upstream prematurely closed connection` or `Connection reset by peer`.

The error is random and rare, so it is hard to catch. The fix is that **nginx, not the backend, should close idle connections**: the backend's idle timeout is made **longer** than `keepalive_timeout` in the nginx upstream. For example, nginx — 60 seconds, the backend — 65:
```javascript
// Node.js: keep an idle connection longer than nginx does (60 s)
server.keepAliveTimeout = 65000;
server.headersTimeout = 66000;   // must be greater than keepAliveTimeout
```
The same story happens with an AWS ALB in front of an application: the ALB idle timeout is 60 seconds by default, and the application's keep-alive timeout must also be longer.

### Connecting to the backend: TCP or a Unix socket

**Two ways nginx can "reach" the application:**
- **over TCP** — the application listens on a network address and port (`127.0.0.1:8080` or `10.0.1.10:8080`), and nginx connects to it like to any network service;
- **through a Unix socket** — the application listens on a **special file** in the file system (for example, `/run/php/php8.3-fpm.sock`), and nginx connects to that file.

**What a Unix socket is.** It is a mechanism for exchanging data between processes **on the same machine** (IPC). From the outside it looks like a file (`ls -l` shows its type as `s`), but data is not written to disk: the kernel passes it directly from one process's memory to another's. Programs use the same calls as with network sockets (`connect`, `send`, `recv`), so for the application the difference comes down to one line of configuration.

**How it differs from TCP over `127.0.0.1`.** Even a localhost connection goes through the kernel's network stack: the TCP handshake, sequence numbers, acknowledgements, checksums, a port number, the TIME_WAIT state after closing. A Unix socket has none of this — there is no network, so none of what is needed to deliver packets over a network. Hence:
- **less overhead** — less kernel work per request, lower latency. The gain is noticeable with many short requests; for heavy requests, where most of the time goes to the application and the database, the difference is barely visible;
- **no ports and no TIME_WAIT** — the port range cannot run out, and there is no need to watch for the port being taken by another process;
- **access is controlled by file permissions** — only someone with write permission on the socket file can connect. Whereas any process and any user on the machine can connect to `127.0.0.1:8080`;
- **it only works within one machine** — you cannot connect to a Unix socket on another server.

**When to choose which:**
| | TCP | Unix socket |
|---|---|---|
| nginx and the application on the same machine | Possible | **Better**: faster and more secure |
| The application on another server, several servers behind the balancer | **TCP only** | Impossible |
| Containers in the same Kubernetes pod | Possible (shared `localhost`) | Possible via a shared `emptyDir` volume |
| Different pods, different nodes | **TCP only** (via a Service) | Impossible |
| Debugging "from outside" (`curl`, `nc`, `tcpdump`) | The usual tools | `curl --unix-socket`, `ss -x`; `tcpdump` does not see the socket |

The classic example is **nginx + PHP-FPM on one server** (PHP-FPM — FastCGI Process Manager, a manager of a pool of processes that execute PHP code; nginx doesn't run PHP itself): almost always via a Unix socket. Python applications are often connected the same way (Gunicorn, uWSGI — servers that run a Python web application in several processes), less often Node.js and .NET.

### Configs: nginx, PHP-FPM, Gunicorn
**PHP-FPM** speaks not HTTP but the **FastCGI** protocol (a binary "web server → external executor process" protocol, older and simpler than HTTP), so nginx uses `fastcgi_pass` rather than `proxy_pass`:
```nginx
upstream php_backend {
    server unix:/run/php/php8.3-fpm.sock;   # Unix socket
    # server 127.0.0.1:9000;                # the same over TCP
    keepalive 16;                           # the persistent connection pool works for a socket too
}

server {
    listen 80;
    root /var/www/app/public;

    location / {
        try_files $uri /index.php$is_args$args;
    }

    location ~ \.php$ {
        include fastcgi_params;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;   # which PHP file to run
        fastcgi_pass php_backend;
        fastcgi_keep_conn on;               # for FastCGI instead of proxy_http_version: do not close the connection
    }
}
```
The PHP-FPM pool that listens on this socket:
```ini
; /etc/php/8.3/fpm/pool.d/www.conf
[www]
user = www-data
group = www-data
listen = /run/php/php8.3-fpm.sock    ; or listen = 127.0.0.1:9000 for TCP
listen.owner = www-data              ; owner of the socket file
listen.group = www-data              ; group — nginx must be in it or run as this user
listen.mode = 0660                   ; only the owner and group can read and write
listen.backlog = 511                 ; how many connections may wait in the queue while all processes are busy
pm = dynamic                         ; the number of processes changes with load (static — always max_children)
pm.max_children = 50
```
**An HTTP application** (Gunicorn, Node.js, .NET) — an ordinary `proxy_pass`, only the address is a socket:
```nginx
upstream py_app {
    server unix:/run/app/app.sock;
    keepalive 16;
}

server {
    listen 80;
    location / {
        proxy_pass http://py_app;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
    }
}
```
```bash
gunicorn --bind unix:/run/app/app.sock --workers 4 app:app          # Python
# .NET (Kestrel): an environment variable or appsettings
ASPNETCORE_URLS="http://unix:/run/app/app.sock" dotnet App.dll
```

### Permissions and common traps
**1. Permissions on the socket file.** nginx worker processes run not as root but as the user from the `user` directive (usually `nginx` or `www-data`). They need **write** permission on the socket file and permission to enter its directory. If the socket's owner and group are `www-data` and the mode is `0660`, nginx must run as `www-data` or be a member of that group.

**2. A socket in `/tmp` and `PrivateTmp` in systemd.** Many systemd services (and nginx itself on some distributions) run with `PrivateTmp=true`: the service gets its own `/tmp`, invisible to others. The application creates the socket in its own `/tmp`, nginx looks for it in its own — and does not find it, even though `ls` as root shows the file. That is why sockets go into **`/run/<service-name>/`** (in systemd — with the `RuntimeDirectory=app` directive, which creates `/run/app` with the right owner at start and removes it at stop).

**3. A stale socket file.** If the application crashed, the socket file may remain. nginx connects to the file, but nobody is listening — a `Connection refused` error. Many applications remove the old file themselves at start; `RuntimeDirectory` in systemd solves this automatically.

**4. The connection queue (backlog).** When all application processes are busy, new connections wait in the socket's queue. Its size is limited by the value the application set (`listen.backlog` in PHP-FPM) and by the system maximum `net.core.somaxconn`. When the queue overflows, nginx immediately gets an error and returns **502**. Over TCP in this situation you more often see timeouts, while a Unix socket refuses instantly, which is convenient: overload is visible right away.

**5. SELinux** (RHEL, Amazon Linux with SELinux enabled) may forbid nginx from connecting to the socket even with correct permissions — `audit.log` will contain a `denied` entry.

### How to diagnose
```bash
ls -l /run/php/php8.3-fpm.sock                       # does the file exist? type "s", owner, group, mode
ss -xlp | grep php                                    # who listens on this socket (x — Unix sockets, l — listening, p — process)
ps -o user,pid,cmd -C nginx                          # the master runs as root, workers as the user from the user directive
sudo -u www-data test -w /run/php/php8.3-fpm.sock && echo "write allowed"
curl --unix-socket /run/app/app.sock http://localhost/health    # a request to the HTTP application bypassing nginx
```
The error text in the nginx `error.log` tells you straight away what is wrong:
| Message | Meaning | What to do |
|---|---|---|
| `connect() to unix:/run/php/php8.3-fpm.sock failed (2: No such file or directory)` | The socket file does not exist | The application is not running, the path is wrong, or the socket is in someone else's `PrivateTmp` |
| `... failed (13: Permission denied)` | No permission on the file or directory | The socket's permissions and group (`listen.owner`, `listen.mode`), the nginx user, SELinux |
| `... failed (111: Connection refused)` | The file exists, but nobody is listening | The application crashed and an old socket file remained |
| `... failed (11: Resource temporarily unavailable)` | The connection queue is full | All processes are busy: increase `pm.max_children`, `listen.backlog`, `net.core.somaxconn`, look for slow requests |

### Unix sockets in containers
- **Docker:** nginx and the application in different containers on the same host can talk through a socket in a **shared volume**: both containers mount one volume, the application creates the socket there, and nginx connects to it. The users' UIDs in the containers must match, or the socket's permissions must allow it.
- **Kubernetes:** containers **of the same pod** share an `emptyDir` volume — the typical "nginx + PHP-FPM in one pod" setup. A Unix socket does not work between different pods, only TCP via a Service.
```yaml
spec:
  volumes:
    - name: php-sock
      emptyDir: {}                       # a shared volume for the lifetime of the pod
  containers:
    - name: php-fpm
      image: php:8.3-fpm
      volumeMounts:
        - name: php-sock
          mountPath: /run/php            # PHP-FPM creates the socket here (in the pool config: listen = /run/php/php-fpm.sock)
    - name: nginx
      image: nginx:1.27
      volumeMounts:
        - name: php-sock
          mountPath: /run/php            # nginx sees the same socket file
```

### WebSocket: a persistent two-way channel through nginx

**Why WebSocket is needed.** Ordinary HTTP works as "question — answer": the client sends a request, the server responds, and that's it. The server itself cannot write to the client first — it can only wait for the next request. For many tasks that is not enough:
- **chats and messengers** — a new message must appear immediately, not when the browser asks;
- **notifications and "live" dashboards** — deployment status, Grafana graphs, quotes, scoreboards;
- **collaborative editing** of documents, online games;
- **a terminal in the browser** — for example, a tab with pod logs or a shell in the Kubernetes dashboard, Lens, ArgoCD.

Without WebSocket the browser has to keep asking the server "anything new?" (polling) — extra requests and latency. **WebSocket** is a persistent connection over which **both sides can send data at any moment** while it is open (minutes, hours).

**How WebSocket is established.** It starts as an ordinary HTTP request in which the client asks to "switch protocols":
```text
1. Client → server (ordinary HTTP/1.1):
     GET /ws/chat HTTP/1.1
     Host: example.com
     Upgrade: websocket            ← "I want to switch to the WebSocket protocol"
     Connection: Upgrade           ← "this connection needs to be switched"
     Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==

2. Server → client:
     HTTP/1.1 101 Switching Protocols    ← "agreed, switching"
     Upgrade: websocket
     Connection: Upgrade

3. From then on, over THE SAME TCP connection, there are no more HTTP requests
   but WebSocket messages in both directions — until the connection is closed.
```

**Why this does not work through nginx by default.** Remember that nginx has two separate connections — to the client and to the backend (see the keep-alive section above). By the HTTP standard the `Upgrade` and `Connection` headers are **hop-by-hop**: they describe only **one link** of the path (client ↔ nginx), and a proxy **must not** forward them further. So by default nginx:
- does not pass `Upgrade: websocket` to the backend;
- sends its own `Connection: close` instead of `Connection: Upgrade`;
- talks to the backend over HTTP/1.0, where the Upgrade mechanism does not exist at all.

As a result the backend receives an ordinary HTTP request with no request to switch, answers it as an ordinary request (often `400` or `200`), and the browser console shows an error like `WebSocket connection failed: Error during WebSocket handshake: Unexpected response code: 400`.

**The fix is to pass the switch request to the backend explicitly.** The config and what each line does:
```nginx
# (1) map computes the $connection_upgrade variable from the client's Upgrade header
map $http_upgrade $connection_upgrade {
    default upgrade;     # the client sent Upgrade (wants WebSocket) → pass "Connection: upgrade" to the backend
    ''      close;       # no Upgrade header (an ordinary request) → "Connection: close", as usual
}

server {
    listen 80;

    location /ws/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;                            # (2) Upgrade exists only in HTTP/1.1
        proxy_set_header Upgrade    $http_upgrade;         # (3) forward the client's Upgrade header to the backend
        proxy_set_header Connection $connection_upgrade;   # (4) and "Connection: upgrade" (the value from map)
        proxy_read_timeout 1h;                             # (5) do not cut the connection if it is silent for a long time
        proxy_send_timeout 1h;
    }
}
```
1. **`map`** — a lookup table in the `http` block: it computes one variable from the value of another. Here: if the client sent an `Upgrade` header (the `$http_upgrade` variable is not empty), then `$connection_upgrade` = `upgrade`, otherwise `close`. Why so complicated instead of simply writing `Connection "upgrade"`: ordinary HTTP requests may come to the same location too, and for them the backend should not be asked to switch protocols.
2. **`proxy_http_version 1.1`** — the Upgrade mechanism exists only in HTTP/1.1, while nginx talks to the backend over HTTP/1.0 by default.
3. **`proxy_set_header Upgrade $http_upgrade`** — `$http_upgrade` holds the value of the `Upgrade` header from the client's request (`websocket`); this line passes it to the backend.
4. **`proxy_set_header Connection $connection_upgrade`** — pass `Connection: upgrade` to the backend for WebSocket requests and `close` for ordinary ones.
5. **`proxy_read_timeout`** — how long nginx waits for data from the backend, **60 seconds by default**. In ordinary HTTP this protects against a hung backend, but a WebSocket can be silent for minutes (nobody is writing in the chat) — and after 60 seconds of silence nginx cuts the connection. So the timeout is increased, and even more reliably, the application sends a ping message every 20–30 seconds, so the connection is never idle longer than the timeout.

**How to check.**
- **The browser:** DevTools → Network → the **WS** filter. The connection should have the status **101 Switching Protocols**, and the Messages tab shows messages in both directions.
- **curl** — check that the handshake goes through nginx:
```bash
curl -i -N http://example.com/ws/chat \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ=="
# we expect the first line of the response: HTTP/1.1 101 Switching Protocols
```
- **websocat** — a console WebSocket client that lets you send and receive messages: `websocat ws://example.com/ws/chat`.

**What else to keep in mind.**
- **Connections drop exactly once a minute** — almost always an idle timeout: `proxy_read_timeout` in nginx, the AWS ALB idle timeout (60 seconds by default), `timeout tunnel` in HAProxy (`timeout tunnel 1h` — for long connections after an Upgrade). The timeout has to be raised **at every link**, or ping enabled in the application.
- **Load balancing.** A WebSocket connection lives on **one** backend for its whole lifetime. If the session is stored in the server's memory, after a drop and reconnect the client may land on another one. For such connections the `least_conn` algorithm is better (the connections are long, and round robin distributes them unevenly).
- **Deploys and reloads.** On `nginx -s reload` the old workers wait until their connections close, and a WebSocket can live for hours — the old processes hang around for a long time. `worker_shutdown_timeout` limits this time, after which the connections are cut. The client must be able to **reconnect on its own** — a mandatory part of any WebSocket application.
- **Capacity.** Every connected client is an open connection in nginx and on the backend for as long as they are on the page. 50,000 users online means 50,000 connections: check `worker_connections` and open-file limits.
- **Kubernetes and ingress-nginx.** WebSocket works without extra settings (the controller passes the needed headers itself), but the default timeouts are the same 60 seconds — they are changed with the annotations `nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"` and `proxy-send-timeout`.
- **Server-Sent Events (SSE)** — a simpler alternative when data only needs to flow **from the server to the client** (notifications, a streaming language model response): an ordinary long HTTP response, no Upgrade needed. But in nginx buffering must be turned off for it (`proxy_buffering off`), otherwise events arrive in batches (see "Buffering" below).

### Buffering and request body size
- **`proxy_buffering on`** (default): nginx quickly pulls the response from the backend into its buffers (and onto disk if it does not fit) and serves it to the client at the client's pace. The backend is freed immediately, even if the client is on slow mobile internet. It is switched `off` for streaming responses: Server-Sent Events, streaming LLM responses, long polling.
- **`client_max_body_size`** — the maximum request body size, **1 MB by default**. Larger — **413 Request Entity Too Large**. The classic: "files larger than a megabyte won't upload".
- **`proxy_buffer_size`** — the buffer for the backend response headers. If the backend sends large headers (many cookies, long JWTs — signed authorisation tokens that easily weigh several kilobytes), the log shows `upstream sent too big header while reading response header from upstream`, and the client gets a 502. The fix is to increase `proxy_buffer_size` and `proxy_buffers`.

### The real client IP behind a load balancer
If there is another proxy in front of nginx (AWS ALB, Cloudflare), `$remote_addr` is that proxy's address. The realip module substitutes the real address from a header, but **only if the header came from a trusted source**:
```nginx
set_real_ip_from 10.0.0.0/16;          # load balancer addresses we trust
real_ip_header   X-Forwarded-For;
real_ip_recursive on;                  # skip all trusted addresses in the chain, take the first untrusted one
```
Without `set_real_ip_from` any client could spoof its IP by sending an `X-Forwarded-For` header and bypass IP-based restrictions.

### 🧪 Mini task

**Task.** Make a backend nginx that answers with the request path and the `Host` and `X-Forwarded-For` headers, and two locations in the proxy: `proxy_pass` without a trailing slash and with one. See which path reaches the backend in each case.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && docker network create lb > /dev/null
echo 'server { listen 80; location / { return 200 "path=$request_uri host=$host xff=$http_x_forwarded_for\n"; } }' > backend.conf
docker run -d --name be --network lb -v "$PWD/backend.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null
cat > default.conf <<'EOF'
server {
    listen 80;
    location /a/ {
        proxy_pass http://be;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
    location /b/ {
        proxy_pass http://be/;
    }
}
EOF
docker run -d --name ngx --network lb -p 8080:80 -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2

curl -s http://localhost:8080/a/users -H 'Host: shop.local' | sed 's/xff=.*/xff=<client ip>/'
curl -s http://localhost:8080/b/users
docker rm -f ngx be > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
path=/a/users host=shop.local xff=<client ip>
path=/users host=be xff=
```

Breakdown: `proxy_pass` without a URI (`http://be`) passes the path as is, while with a URI (`http://be/`) it replaces the matched location part with it, so `/b/users` becomes `/users`. This is the most common proxy mistake. The second location sets no headers, so the backend got `Host: be` (the name from `proxy_pass`) and never learned the client IP. Without `proxy_set_header Host`, virtual hosts and app-generated links break, and without `X-Forwarded-For` the app logs show only the proxy's IP.

</details>

## Nginx: load balancing

```nginx
upstream app_backend {
    least_conn;                                            # algorithm (default — round robin)

    server 10.0.1.10:8080 weight=3;                        # gets three times more requests
    server 10.0.1.11:8080 max_fails=3 fail_timeout=30s;    # 3 errors in 30 s — exclude for 30 s
    server 10.0.1.12:8080 backup;                          # only if all primary servers are down
    server 10.0.1.13:8080 down;                            # taken out manually for now

    keepalive 32;
}
```
**Algorithms:**
- **round robin** (default) — in turn, respecting weights;
- **`least_conn`** — to the server with the fewest active connections. Better when requests vary a lot in duration;
- **`ip_hash`** — a client IP always goes to the same server (simple session affinity);
- **`hash $request_uri consistent`** — by an arbitrary key; `consistent` (consistent hashing) remaps only a small share of keys when a server is added or removed. Needed for caching backends;
- **`random two least_conn`** — pick two random servers and choose the one with fewer connections ("power of two choices"). Works well when there are several load balancers.

**Health checks in open-source nginx are passive only.** A server is considered down if real requests to it failed `max_fails` times within `fail_timeout` (1 time in 10 seconds by default). Active checks (periodically requesting `/health`) exist only in commercial NGINX Plus. So nginx learns about a problem only after spoiling several real requests. HAProxy does active checks out of the box.

**Retrying on another server** (`proxy_next_upstream`): by default nginx tries the next server on a connection error or timeout. But **non-idempotent requests (POST, PATCH)** are not retried by default if the request was already sent to the backend — otherwise a retry might, for example, create two orders. Enable retries for them (`non_idempotent`) only deliberately.
```nginx
proxy_next_upstream error timeout http_502 http_503;
proxy_next_upstream_tries 2;          # at most two attempts — protection against a retry storm
proxy_next_upstream_timeout 10s;
```

### 🧪 Mini task

**Task.** Define an `upstream` of `app1` with weight 2 and `app2` with weight 1, make 6 requests and count the distribution. Then mark `app2` as `backup` and repeat. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done

cat > default.conf <<'EOF'
upstream app { server app1:5678 weight=2; server app2:5678; }
server { listen 80; location / { proxy_pass http://app; } }
EOF
docker run -d --name ngx --network lb -p 8080:80 -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2

for i in 1 2 3 4 5 6; do curl -s localhost:8080; done | sort | uniq -c
sed -i.bak 's/server app2:5678;/server app2:5678 backup;/' default.conf
docker exec ngx nginx -s reload 2> /dev/null; sleep 1
echo ---; for i in 1 2 3 4 5 6; do curl -s localhost:8080; done | sort | uniq -c
docker rm -f ngx app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
   4 app1
   2 app2
---
   6 app1
```

Breakdown: by default nginx does weighted round robin: with 2:1 weights four of six requests went to `app1`. A `backup` server gets traffic only when all primary servers are down, so the second run sent everything to `app1`. Besides round robin there's `least_conn` (fewest active connections — for requests of varying length), `ip_hash` and `hash $cookie_x consistent` (pinning a client to a server). Free nginx checks backends only passively: after `max_fails` errors it removes a server for `fail_timeout`. Active health checks come with HAProxy and nginx Plus.

</details>

## Nginx: TLS
```nginx
server {
    listen 80;
    server_name example.com;
    return 301 https://$host$request_uri;          # all HTTP — to HTTPS
}

server {
    listen 443 ssl;
    http2 on;                                      # in nginx 1.25.1+ instead of "listen 443 ssl http2"
    server_name example.com;

    ssl_certificate     /etc/nginx/certs/example.com.fullchain.pem;   # certificate + intermediates
    ssl_certificate_key /etc/nginx/certs/example.com.key;

    ssl_protocols TLSv1.2 TLSv1.3;                 # old versions disabled
    ssl_session_cache   shared:SSL:10m;            # session cache shared by workers — repeat connections skip the full handshake
    ssl_session_timeout 1h;

    add_header Strict-Transport-Security "max-age=31536000" always;   # HSTS: from now on the browser uses only HTTPS

    location / {
        proxy_pass http://127.0.0.1:8080;
    }
}
```
Important:
- `ssl_certificate` needs the **fullchain** — the certificate together with the intermediates. Without intermediates browsers sometimes cope on their own, but curl, Java and mobile clients do not: "unable to get local issuer certificate";
- picking a certificate by name works through **SNI**: many domains on one IP;
- nginx reads certificates at startup and on reload: if you updated the files (certbot — the Let's Encrypt client that issues and renews certificates itself; cert-manager), you need `nginx -s reload`.

### 🧪 Mini task

**Task.** Issue a self-signed certificate, enable HTTPS in nginx with TLS 1.2 and 1.3, an HTTP redirect and the HSTS header. Check the redirect, the protocol version and the header.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
openssl req -x509 -newkey rsa:2048 -nodes -keyout tls.key -out tls.crt -days 30 -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost" 2> /dev/null
cat > default.conf <<'EOF'
server {
    listen 80;
    return 301 https://$host:8443$request_uri;
}
server {
    listen 443 ssl;
    http2 on;
    ssl_certificate     /tls/tls.crt;
    ssl_certificate_key /tls/tls.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    add_header Strict-Transport-Security "max-age=31536000" always;
    location / { return 200 "secure\n"; }
}
EOF
docker run -d --name ngx -p 8080:80 -p 8443:443 -v "$PWD:/tls:ro" -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2
curl -s -o /dev/null -w '%{http_code} -> %{redirect_url}\n' http://localhost:8080/login
curl -s --cacert tls.crt -o /dev/null -w '%{http_version} ' https://localhost:8443/ ; curl -sv --cacert tls.crt https://localhost:8443/ 2>&1 | grep -oE 'SSL connection using TLSv1\.[23]'
curl -sI --cacert tls.crt https://localhost:8443/ | grep -i strict-transport
docker rm -f ngx > /dev/null
```

Expected output:

```text
301 -> https://localhost:8443/login
2 SSL connection using TLSv1.3
strict-transport-security: max-age=31536000
```

Breakdown: the HTTP server only answers with a redirect to HTTPS. The client negotiated TLS 1.3 and HTTP/2 on its own. `--cacert tls.crt` tells curl to trust our self-signed certificate — without it you'd get a verification error, just like in a browser. HSTS tells the browser "use only HTTPS here for a year", protecting against downgrade attacks on the first plain-HTTP visit. `always` adds the header to error responses too. In production certificates come from Let's Encrypt (certbot) or ACM on the load balancer, and TLS 1.0/1.1 are disabled.

</details>

## Nginx: caching
```nginx
proxy_cache_path /var/cache/nginx levels=1:2 keys_zone=app_cache:10m max_size=1g inactive=60m use_temp_path=off;
# keys_zone — name and memory for keys (1 MB ≈ 8000 keys); max_size — disk limit;
# inactive — drop an item not accessed for 60 minutes

server {
    listen 80;
    location /catalog/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_cache app_cache;
        proxy_cache_valid 200 301 10m;          # keep successful responses for 10 minutes
        proxy_cache_valid 404 1m;
        proxy_cache_use_stale error timeout updating http_500 http_502 http_503 http_504;
        proxy_cache_background_update on;       # refresh a stale item in the background
        proxy_cache_lock on;                    # on a miss ONE request goes to the backend, the rest wait
        add_header X-Cache-Status $upstream_cache_status;   # HIT, MISS, EXPIRED, STALE — handy for debugging
    }
}
```
- **`proxy_cache_use_stale`** — if the backend is down, serve the stale copy: the site keeps working in "read-only" mode.
- **`proxy_cache_lock`** protects against a **cache stampede**: when a popular item expires, hundreds of simultaneous requests do not all hit the backend at once.
- The default cache key is scheme, host and URI with query string. Responses with `Set-Cookie` and requests with authorization are not cached by default — and rightly so: otherwise one user would see another's data.
- **Microcaching** — cache even dynamic pages for 1–5 seconds. At 1000 requests per second, one request per second reaches the backend.

### 🧪 Mini task

**Task.** Enable `proxy_cache` in front of the backend for 1 minute and add a response header with the cache status. Make three requests in a row and look at each one's status. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done

cat > default.conf <<'EOF'
proxy_cache_path /tmp/cache keys_zone=app_cache:10m max_size=100m inactive=10m;
server {
    listen 80;
    location / {
        proxy_pass http://app1:5678;
        proxy_cache app_cache;
        proxy_cache_valid 200 1m;
        add_header X-Cache-Status $upstream_cache_status;
    }
}
EOF
docker run -d --name ngx --network lb -p 8080:80 -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2

for i in 1 2 3; do curl -sI localhost:8080/ | grep -i x-cache-status; done
docker rm -f ngx app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
X-Cache-Status: MISS
X-Cache-Status: HIT
X-Cache-Status: HIT
```

Breakdown: the first request went to the backend (MISS) and nginx stored the response, then served the next ones from the cache (HIT) — the backend never saw them. On heavy pages and APIs this removes most of the load. The default cache key is scheme, host and URI; responses with `Set-Cookie` and requests with authorisation must be excluded deliberately, or one user will see another's page. `proxy_cache_use_stale error timeout updating` serves a stale copy while the backend is down — so the cache also improves availability. The `EXPIRED`, `STALE` and `BYPASS` statuses explain the rest of the behaviour.

</details>

## Nginx: rate limiting
```nginx
limit_req_zone  $binary_remote_addr zone=per_ip:10m rate=10r/s;   # 10 requests per second per IP
limit_conn_zone $binary_remote_addr zone=conn_per_ip:10m;

server {
    listen 80;
    location /api/ {
        limit_req  zone=per_ip burst=20 nodelay;   # a burst of up to 20 above the rate — without delay
        limit_req_status 429;                       # default is 503, the correct code is 429 Too Many Requests
        limit_conn conn_per_ip 20;                  # at most 20 simultaneous connections per IP
        proxy_pass http://127.0.0.1:8080;
    }
}
```
How it works: the **leaky bucket** algorithm. Requests leak out at a constant `rate`. `burst` is the bucket size: that many requests above the rate may wait in a queue. Without `nodelay` they are **delayed** and executed one by one at the configured rate (the client sees slow responses); with `nodelay` they are executed **immediately**, but bucket slots free up at the normal rate. Anything that does not fit is rejected.

`$binary_remote_addr` is the IP in binary form (4 bytes for IPv4 instead of up to 15 characters): a 10 MB zone holds about 160 thousand addresses. Behind a load balancer, configure realip first, otherwise the limit applies to all of the balancer's traffic as if it were one client.

### 🧪 Mini task

**Task.** Limit requests per IP to 1 per second with no burst and a 429 response code. Send 10 requests in a row and count the response codes.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > default.conf <<'EOF'
limit_req_zone $binary_remote_addr zone=per_ip:10m rate=1r/s;
server {
    listen 80;
    location / {
        limit_req zone=per_ip;
        limit_req_status 429;
        return 200 "ok\n";
    }
}
EOF
docker run -d --name ngx -p 8080:80 -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2
for i in $(seq 1 10); do curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/; done | sort | uniq -c
docker logs ngx 2>&1 | grep -c 'limiting requests'
docker rm -f ngx > /dev/null
```

Expected output:

```text
   1 200
   9 429
   9
```

Breakdown: `limit_req` uses a leaky bucket. At 1 request per second with no burst only the first one passed, and the other nine arriving in the same second were rejected. Each rejection is written to the error log. By default nginx answers 503; `limit_req_status 429` gives the proper "Too Many Requests" code. In production you add `burst=10 nodelay` to let short spikes from normal clients through, key on the IP, API token or user, and give the login endpoint its own stricter zone against password guessing.

</details>

## Nginx: static files, compression and performance
```nginx
http {
    sendfile    on;          # serve files via the sendfile syscall: the kernel copies from disk to socket with no copy in nginx
    tcp_nopush  on;          # with sendfile — send headers and the start of the file in one packet
    tcp_nodelay on;          # do not delay small packets (Nagle's algorithm) on keep-alive connections
    keepalive_timeout 65s;   # how long to keep an idle client connection

    gzip on;
    gzip_types text/css application/javascript application/json image/svg+xml;   # text/html is always compressed
    gzip_min_length 1024;    # do not compress small responses — no benefit
    gzip_comp_level 5;       # balance between CPU and compression ratio

    open_file_cache max=10000 inactive=60s;   # cache of descriptors and metadata of static files

    server {
        listen 80;
        location /static/ {
            root /var/www;
            expires 30d;                              # Cache-Control for the browser
            add_header Cache-Control "public, immutable";
        }
    }
}
```
Do not compress what is already compressed (JPEG and PNG images, video, archives): it wastes CPU.

### 🧪 Mini task

**Task.** Serve a 100 KB text file through nginx with gzip enabled. Download it without compression and with `Accept-Encoding: gzip`, and compare the sizes and headers.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && mkdir html
python3 -c "print('kubernetes deployment log line\n' * 3300)" > html/app.log
cat > default.conf <<'EOF'
server {
    listen 80;
    root /usr/share/nginx/html;
    gzip on;
    gzip_types text/plain application/json;
    types { text/plain log; }
}
EOF
docker run -d --name ngx -p 8080:80 -v "$PWD/html:/usr/share/nginx/html:ro" -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2
plain=$(curl -s -o /dev/null -w '%{size_download}' localhost:8080/app.log)
gz=$(curl -s -H 'Accept-Encoding: gzip' -o /dev/null -w '%{size_download}' localhost:8080/app.log)
echo "plain=${plain}B gzip_smaller_than_1KB=$([ $gz -lt 1024 ] && echo yes)"
curl -sI -H 'Accept-Encoding: gzip' localhost:8080/app.log | grep -i -E '^content-encoding'
docker rm -f ngx > /dev/null
```

Expected output:

```text
plain=102301B gzip_smaller_than_1KB=yes
Content-Encoding: gzip
```

Breakdown: repetitive text shrinks hundreds of times, real HTML, JSON and logs usually 3–10 times. That's a direct saving in traffic and load time. Compression happens only if the client sent `Accept-Encoding: gzip`, and only for types in `gzip_types` (by default just `text/html`). Images and archives aren't compressed again — that would waste CPU. For static files nginx uses `sendfile` (data goes from the page cache to the socket without copying into user space), and `expires` and `Cache-Control` let browsers and CDNs avoid coming back at all.

</details>

## Nginx: logs and variables
JSON logs are convenient to ship to Loki (the log storage system from Grafana Labs) or ELK (Elasticsearch + Logstash + Kibana — the classic log search stack) and to build metrics from:
```nginx
log_format json_combined escape=json '{'
    '"time":"$time_iso8601",'
    '"remote_addr":"$remote_addr",'
    '"request":"$request",'
    '"status":$status,'
    '"body_bytes_sent":$body_bytes_sent,'
    '"request_time":$request_time,'
    '"upstream_addr":"$upstream_addr",'
    '"upstream_status":"$upstream_status",'
    '"upstream_connect_time":"$upstream_connect_time",'
    '"upstream_response_time":"$upstream_response_time",'
    '"request_id":"$request_id",'
    '"http_user_agent":"$http_user_agent"'
'}';

access_log /var/log/nginx/access.json json_combined;
error_log  /var/log/nginx/error.log warn;
```
The most useful variables for investigations:
- **`$request_time`** — the full time nginx spent on the request (from the first byte of the request to sending the last byte of the response), including a slow client;
- **`$upstream_response_time`** — how long the backend took to prepare the response. Comparing the two shows who is slow: a large `upstream_response_time` means the backend; a large `request_time` with a small upstream time means a slow client or network;
- **`$upstream_connect_time`** — slow connection establishment points to an overloaded backend or the network;
- **`$upstream_addr`** and **`$upstream_status`** — which server the request hit and what it answered (with retries, all attempts are listed comma-separated);
- **`$request_id`** — a unique request ID: pass it to the backend as an `X-Request-ID` header and correlate nginx and application logs.

### 🧪 Mini task

**Task.** Configure the access log in JSON format with the status, URI, upstream address and response time fields. Make a couple of requests through the proxy and parse the log with `jq`. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done

cat > default.conf <<'EOF'
log_format json escape=json '{"status":"$status","uri":"$request_uri","upstream":"$upstream_addr","rt":"$request_time","urt":"$upstream_response_time"}';
upstream app { server app1:5678; server app2:5678; }
server {
    listen 80;
    access_log /dev/stdout json;
    location / { proxy_pass http://app; }
}
EOF
docker run -d --name ngx --network lb -p 8080:80 -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2

curl -s localhost:8080/orders > /dev/null; curl -s localhost:8080/missing > /dev/null
docker logs ngx 2> /dev/null | grep '^{' | jq -c '{status, uri, has_upstream: (.upstream | length > 0), rt}'
docker rm -f ngx app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
{"status":"200","uri":"/orders","has_upstream":true,"rt":"0.002"}
{"status":"200","uri":"/missing","has_upstream":true,"rt":"0.001"}
```

Breakdown: a JSON log is parsed straight into Loki, ELK or ClickHouse without regexes. `$request_time` is the full time from the first request byte to the last response byte, `$upstream_response_time` is how long the backend took. Their difference shows where the delay is: in the app or on the network to the client. `$upstream_addr` tells which backend served the request — without it you can't find the "sick" server in the pool. `escape=json` escapes quotes in values, otherwise the log line becomes invalid JSON. Your timings will differ. `/missing` returned 200 because http-echo answers any path.

</details>

## Nginx: diagnostics and error codes
```bash
nginx -t                    # test the configuration
nginx -T                    # test and PRINT the whole effective config with all includes — what is really applied
nginx -V                    # version and the modules nginx was built with
systemctl reload nginx
tail -f /var/log/nginx/error.log
ss -tanp | grep nginx | awk '{print $1}' | sort | uniq -c   # nginx connection states
```
The `stub_status` module provides simple metrics (active connections, accepted, requests) — nginx-prometheus-exporter collects them:
```nginx
server {
    listen 127.0.0.1:8081;
    location = /nginx_status {
        stub_status;
        allow 127.0.0.1;
        deny all;
    }
}
```
**Codes and messages that come up in interviews:**
| Symptom | Meaning | Where to look |
|---|---|---|
| **502 Bad Gateway** | The backend did not respond correctly | In error.log: `connect() failed (111: Connection refused)` — the backend is not listening; `upstream prematurely closed connection` — it crashed mid-request or closed keep-alive; `upstream sent too big header` — increase `proxy_buffer_size`; `no live upstreams` — all servers marked as down |
| **504 Gateway Timeout** | The backend did not respond within `proxy_read_timeout` (60 s) | Slow requests in the application or the database. Raising the timeout is the last resort |
| **499** | A non-standard nginx code: **the client closed the connection** before getting the response | The client or a load balancer in front of nginx gave up on its own timeout — often a consequence of a slow backend |
| **413** | The request body is larger than `client_max_body_size` (1 MB by default) | Raise the limit in the relevant location |
| **414, 400 Request Header Too Large** | URL or headers too long (large cookies) | `large_client_header_buffers` |
| `worker_connections are not enough` | The worker connection limit is exhausted | `worker_connections`, `worker_rlimit_nofile` |
| `Too many open files` | The process descriptor limit | `worker_rlimit_nofile`, `LimitNOFILE` in systemd |

### 🧪 Mini task

**Task.** Get two different proxy error codes from nginx: 502 — the backend is down, 504 — the backend takes longer than `proxy_read_timeout`. For each find the error log line that explains the cause.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && docker network create lb > /dev/null
docker run -d --name slow --network lb python:3.12-alpine python -c "
import http.server, time
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        time.sleep(3); self.send_response(200); self.end_headers(); self.wfile.write(b'late')
http.server.HTTPServer(('', 8000), H).serve_forever()" > /dev/null
cat > default.conf <<'EOF'
server {
    listen 80;
    location /down { proxy_pass http://127.0.0.1:9999; }
    location /slow { proxy_pass http://slow:8000; proxy_read_timeout 1s; }
}
EOF
docker run -d --name ngx --network lb -p 8080:80 -v "$PWD/default.conf:/etc/nginx/conf.d/default.conf:ro" nginx:alpine > /dev/null; sleep 2

curl -s -o /dev/null -w '/down -> %{http_code}\n' localhost:8080/down
curl -s -o /dev/null -w '/slow -> %{http_code}\n' localhost:8080/slow
docker logs ngx 2>&1 | grep -oE 'connect\(\) failed \(111: Connection refused\)|upstream timed out \(110: Operation timed out\)'
docker rm -f ngx slow > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
/down -> 502
/slow -> 504
connect() failed (111: Connection refused)
upstream timed out (110: Operation timed out)
```

Breakdown: 502 Bad Gateway means nginx couldn't get a proper response from the backend: the port is closed, the process crashed, the backend dropped the connection. 504 Gateway Timeout means the backend is alive but missed the timeout. Both codes come from the proxy, not the app, so the cause is in nginx's error log: it shows which upstream and which system error. The app logs may be empty on a 502. 499 means the client closed the connection before the answer, 413 that the request body exceeds `client_max_body_size`. The 502 checklist: is the pod or process alive → does it listen on the right port → does the address in `proxy_pass` match.

</details>

## HAProxy: how it works inside

### One process, many threads, an event loop
HAProxy, like nginx, is built on an **event-driven model** with non-blocking sockets and epoll. The difference is in organisation: modern HAProxy is **one process with several threads** (`nbthread`, by default one per core). The threads share common state: connection counters, server state, stick tables (in-memory "key → counters" tables, for example "IP → number of requests in 10 seconds"; rate limiting and session affinity are built on them). That's why limits like "no more than 100 connections per server" are enforced exactly for the whole process, not per worker as in nginx.

### Master-worker and seamless reload
In master-worker mode (`-W`, which is how the systemd unit runs it) the master supervises the worker process. On reload:
1. the master starts a **new worker** with the new configuration;
2. **the listening sockets are passed** to the new process — there is no moment when the port is closed, and new connections are not lost;
3. the old worker finishes its connections and exits; `hard-stop-after` limits how long it may wait.
```bash
haproxy -c -f /etc/haproxy/haproxy.cfg     # test the configuration — before every reload
systemctl reload haproxy
```

### Configuration sections
```
global     — process settings: threads, limits, logging, admin socket
defaults   — default values for all following frontends and backends
frontend   — where to accept connections: bind, ACLs, rules, backend selection
backend    — where to send: servers, balancing algorithm, health checks
listen     — frontend and backend in one block (handy for simple services and the stats page)
```
**Modes:** `mode http` — HAProxy parses HTTP: routing by paths and headers, header rewriting, cookies, HTTP logs. `mode tcp` — proxies a byte stream (L4): databases, Redis, MQTT, TLS without decryption.

### 🧪 Mini task

**Task.** Write a minimal HAProxy config (a frontend and a backend), check it with the built-in `-c` check, and look at the version and the number of threads HAProxy will start with.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > haproxy.cfg <<'EOF'
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  default_backend be_app
backend be_app
  server app1 127.0.0.1:5678
EOF
docker run --rm -v "$PWD/haproxy.cfg:/cfg:ro" haproxy:3.2 haproxy -c -f /cfg 2>&1 | tail -1
docker run --rm haproxy:3.2 haproxy -vv | grep -E '^HAProxy version|Built with multi-threading' | cut -c1-40
```

Expected output:

```text
Configuration file is valid
HAProxy version 3.2.6-6a1aac9 2025/10/0
Built with multi-threading support (MAX_
```

Breakdown: HAProxy is a single process with several threads (one per core), each with its own event loop. The config has `global` (the process), `defaults` (shared settings), `frontend` (where to listen and how to route) and `backend` (where to send). `haproxy -c` validates the config without starting it — a mandatory step before a reload, like `nginx -t`. On reload the master starts a new worker while the old one finishes existing connections. HAProxy resolves server names already during the check, which is why an address is used here instead of a container name. Your version will differ.

</details>

## HAProxy: a full example explained
```haproxy
global
    log stdout format raw local0
    maxconn 50000                              # maximum connections for the whole process
    stats socket /var/run/haproxy.sock mode 660 level admin expose-fd listeners   # Runtime API
    stats timeout 30s

defaults
    mode http
    log global
    option httplog                             # detailed HTTP log with timers
    option dontlognull                         # do not log empty connections (port scanners)
    option forwardfor                          # add X-Forwarded-For
    timeout connect         5s                 # establish the TCP connection to a server
    timeout client          30s                # client inactivity
    timeout server          30s                # server inactivity (waiting for the response)
    timeout http-request    10s                # receive the request headers in full — protection against slowloris (an attack: thousands of connections send headers a byte at a time and keep the server busy)
    timeout http-keep-alive 10s
    timeout queue           30s                # how long a request may wait in the queue for a server
    retries 2

frontend fe_https
    bind :80
    bind :443 ssl crt /etc/haproxy/certs/example.com.pem alpn h2,http/1.1   # alpn — negotiate HTTP/2 with the client inside TLS
    http-request redirect scheme https code 301 unless { ssl_fc }   # ssl_fc — "the connection came over TLS"; everything on HTTP — to HTTPS
    http-request set-header X-Forwarded-Proto https if { ssl_fc }

    acl is_api    path_beg /api/
    acl is_admin  hdr(host) -i admin.example.com
    acl office    src 203.0.113.0/24

    http-request deny if is_admin !office      # admin site — only from the office
    use_backend be_api if is_api
    default_backend be_web

backend be_api
    balance leastconn
    option httpchk
    http-check send meth GET uri /health
    http-check expect status 200
    default-server inter 2s fall 3 rise 2 maxconn 100
    server api1 10.0.1.10:8080 check
    server api2 10.0.1.11:8080 check
    server api3 10.0.1.12:8080 check backup

backend be_web
    balance roundrobin
    cookie SERVERID insert indirect nocache      # pin the client to a server via a cookie
    server web1 10.0.2.10:3000 check cookie web1
    server web2 10.0.2.11:3000 check cookie web2
```
The key parts are explained in the following sections.

### 🧪 Mini task

**Task.** Run HAProxy with a frontend on port 8080 and a two-server backend with health checks, make four requests and look at the distribution and the log lines. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done
cat > haproxy.cfg <<'EOF'
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
  option httplog
frontend fe_main
  bind *:8080
  default_backend be_app
backend be_app
  balance roundrobin
  option httpchk GET /
  server app1 app1:5678 check
  server app2 app2:5678 check
EOF
docker run -d --name hap --network lb -p 8080:8080 -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3

for i in 1 2 3 4; do curl -s localhost:8080; done
docker logs hap 2>&1 | grep -c 'fe_main be_app'
docker rm -f hap app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
app1
app2
app1
app2
4
```

Breakdown: the frontend accepts connections and decides which backend gets them. The backend holds the server list, the balancing algorithm and the health checks. `check` with `option httpchk` sends an HTTP request to each server every 2 seconds and removes those that don't answer. `option httplog` enables the detailed HTTP log: one line per request with the frontend, backend, server, timings and status. Four log lines match four requests.

</details>

## HAProxy: maxconn and queues — the main idea

This is one of HAProxy's strongest features and a common question.

`maxconn` is set at three levels:
- **`global maxconn`** — how many connections the process accepts at all;
- **`frontend maxconn`** (or in defaults) — how many connections a particular frontend accepts;
- **`maxconn` on a server** — how many requests to send **to that server** concurrently.

What happens when a server reaches its `maxconn`: a new request is **neither sent to it nor rejected — it is queued** inside HAProxy. As soon as the server frees up, it gets the next request from the queue. If a request waited longer than `timeout queue`, the client gets **503**.

Why this matters. Say an application handles 50 concurrent requests efficiently; at 500 it starts drowning: thread switching, an exhausted DB connection pool, garbage collection, and response time grows for **everyone**. Without a limit the load balancer will "drown" the server at peak. With `maxconn 50` the server runs in its optimal regime, while the excess waits neatly in the HAProxy queue, where waiting costs almost nothing. The result: higher throughput, more predictable latency, and the server does not fall over.

The queue is visible in the statistics (`qcur` — current queue, `qmax` — maximum) and in logs (the `Tw` timer — time in the queue). A persistent queue is a signal to add servers.

### 🧪 Mini task

**Task.** Put HAProxy with `maxconn 1` on the server in front of a slow backend (answers in 1 second) and send 5 requests at once. Measure the total time and look at the maximum queue length in the statistics.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)" && docker network create lb > /dev/null
docker run -d --name slow --network lb python:3.12-alpine python -c "
import http.server, time
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        time.sleep(1); self.send_response(200); self.end_headers(); self.wfile.write(b'ok')
http.server.ThreadingHTTPServer(('', 8000), H).serve_forever()" > /dev/null
cat > haproxy.cfg <<'EOF'
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  default_backend be_slow
backend be_slow
  timeout queue 30s
  server slow slow:8000 maxconn 1
listen stats
  bind *:8404
  stats enable
  stats uri /stats
EOF
docker run -d --name hap --network lb -p 8080:8080 -p 8404:8404 -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3
start=$(date +%s)
for i in 1 2 3 4 5; do curl -s -o /dev/null localhost:8080 & done; wait
echo "5 parallel requests took $(( $(date +%s) - start ))s"
curl -s 'localhost:8404/stats;csv' | awk -F, '$1 == "be_slow" && $2 == "slow" {print "qmax=" $4}'
docker rm -f hap slow > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
5 parallel requests took 5s
qmax=4
```

Breakdown: `maxconn 1` lets the server hold only one request at a time. HAProxy doesn't reject the rest or push them onto the overloaded backend; it queues them itself: one active plus up to four waiting — `qmax=4`, and the requests completed one by one in 5 seconds. That's HAProxy's main idea: the backend runs at its optimal load and doesn't choke, while the extra requests wait in the balancer's cheap queue. `timeout queue` caps the wait, after which the client gets a 503. The right `maxconn` comes from a load test: as many as the server handles without latency growing.

</details>

## HAProxy: balancing algorithms
| Algorithm | How it picks | When |
|---|---|---|
| `roundrobin` | In turn with weights; weights can be changed on the fly | Default for short, uniform requests |
| `static-rr` | In turn, fixed weights, slightly faster | A very large number of servers |
| `leastconn` | The server with the fewest active connections | Long or uneven requests, WebSocket, databases |
| `first` | The first server that has not reached maxconn | Packing load onto the fewest servers (autoscaling) |
| `source` | Hash of the client IP | Affinity without cookies, tcp mode |
| `uri`, `url_param`, `hdr(name)` | Hash of part of the request | Caching backends: the same URL always on the same server |
| `random` | Random; `random(2)` — the better of two random picks | Many load balancers sharing one server pool |

`hash-type consistent` for hashing algorithms — when the server set changes, only a small share of keys is remapped.

### 🧪 Mini task

**Task.** Compare two algorithms on the same two-server backend: `roundrobin` and `source`. For each make 6 requests and count how many distinct servers served them. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done

for algo in roundrobin source; do
  cat > haproxy.cfg <<EOF
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  default_backend be_app
backend be_app
  balance $algo
  server app1 app1:5678
  server app2 app2:5678
EOF
  docker run -d --name hap --network lb -p 8080:8080 -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3
  echo "$algo: $(for i in 1 2 3 4 5 6; do curl -s localhost:8080; done | sort -u | wc -l | tr -d ' ') distinct server(s)"
  docker rm -f hap > /dev/null
done
docker rm -f app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
roundrobin: 2 distinct server(s)
source: 1 distinct server(s)
```

Breakdown: `roundrobin` hands out requests in a circle. `source` hashes the client IP, so all requests from one address land on one server — simple stickiness without cookies, but NAT breaks it: a whole office behind one IP ends up on one server. Other algorithms: `leastconn` (fewest active connections — for long sessions, WebSockets, databases), `uri` and `hdr(...)` (the same resource on the same server, good for caches), `random(2)` ("power of two choices"). Hash-based ones get `hash-type consistent` so a lost server reassigns only its own share of clients.

</details>

## HAProxy: health checks
Unlike open-source nginx, HAProxy checks servers **on its own and continuously**, without waiting for real requests to fail.
```haproxy
backend be_api
    option httpchk
    http-check send meth GET uri /health ver HTTP/1.1 hdr Host api.example.com
    http-check expect status 200
    server api1 10.0.1.10:8080 check inter 2s fall 3 rise 2
```
- `check` — enable checks (without it the server is always considered alive);
- `inter 2s` — the check interval;
- `fall 3` — after **3 consecutive failures** the server is marked DOWN and removed from balancing;
- `rise 2` — after **2 consecutive successes** it returns to service. Different values protect against the server "flapping" back and forth;
- without `option httpchk` the check is just a TCP connection to the port, which does not show whether the application works;
- **`agent-check`** — the server can report its own weight or "I'm in maintenance" to HAProxy via a separate agent port.

**What to check in `/health`.** The endpoint should answer whether **this instance** can serve requests. If it includes a check of a shared database, then when the database is unavailable **all** servers go DOWN at once and HAProxy returns 503 even for requests that would have worked from cache. The same logic as with liveness probes in Kubernetes.

### 🧪 Mini task

**Task.** Enable active health checks for two servers, then stop `app2` and wait a few seconds. See where requests go now and what HAProxy wrote to its log. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done
cat > haproxy.cfg <<'EOF'
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  default_backend be_app
backend be_app
  option httpchk GET /
  default-server inter 1s fall 2 rise 2
  server app1 app1:5678 check
  server app2 app2:5678 check
EOF
docker run -d --name hap --network lb -p 8080:8080 -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3

docker stop app2 > /dev/null; sleep 4
for i in 1 2 3 4; do curl -s localhost:8080; done | sort | uniq -c
docker logs hap 2>&1 | grep -o 'Server be_app/app2 is DOWN'
docker rm -f hap app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
   4 app1
Server be_app/app2 is DOWN
```

Breakdown: HAProxy checked the servers every second (`inter 1s`), and after two consecutive failures (`fall 2`) marked `app2` DOWN and stopped sending it requests. Users didn't notice: all 4 requests were served by `app1`. When the server comes back, HAProxy enables it after two successful checks (`rise 2`) — so a "flapping" server doesn't get traffic too early. An `httpchk` is better than a plain TCP check: the port can be open while the app is broken. A good health endpoint checks the app itself without dragging in all its dependencies.

</details>

## HAProxy: ACLs and routing
An **ACL** is a named condition used to make decisions. Conditions are combined: a space means AND, `||` or `or` means OR, `!` means NOT.
```haproxy
frontend fe_main
    bind :80
    acl is_api      path_beg /api/
    acl is_static   path_end .css .js .png
    acl is_beta     req.cook(beta) -m str 1
    acl bad_agent   hdr_sub(user-agent) -i sqlmap nikto   # sqlmap, nikto — popular vulnerability scanners

    http-request deny deny_status 403 if bad_agent
    use_backend be_api_beta if is_api is_beta    # API with cookie beta=1 — to the new version
    use_backend be_api      if is_api
    use_backend be_static   if is_static
    default_backend be_web
```
`http-request` rules are executed in order and can do much more: modify headers and the path, redirect, return a response without a backend, set log markers.

### 🧪 Mini task

**Task.** Set up routing in one frontend: requests with a `/api` path go to the `api` backend, requests with a `Host: admin.local` header to `admin`, everything else to `web`. Check three requests. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done
docker run -d --name app3 --network lb hashicorp/http-echo -text=admin > /dev/null
cat > haproxy.cfg <<'EOF'
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  acl is_api   path_beg /api
  acl is_admin hdr(host) -i admin.local
  use_backend be_api   if is_api
  use_backend be_admin if is_admin
  default_backend be_web
backend be_api
  server app1 app1:5678
backend be_admin
  server app3 app3:5678
backend be_web
  server app2 app2:5678
EOF
docker run -d --name hap --network lb -p 8080:8080 -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3

echo "/api/orders     -> $(curl -s localhost:8080/api/orders)"
echo "admin.local /   -> $(curl -s -H 'Host: admin.local' localhost:8080/)"
echo "/               -> $(curl -s localhost:8080/)"
docker rm -f app3 > /dev/null
docker rm -f hap app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
/api/orders     -> app1
admin.local /   -> admin
/               -> app2
```

Breakdown: an ACL is a named condition (path, header, source IP, method, SNI), and `use_backend … if …` picks a backend based on them. Rules are checked in order, the first match wins, and `default_backend` catches the rest. So a request to `admin.local/api` would go to `api`: the `/api` rule comes first. `-i` makes the comparison case-insensitive. The same ACLs drive `http-request deny` (closing `/admin` to the outside), redirects and limits. It's the counterpart of nginx's `location` and `server_name`, except conditions can be combined (`if is_api !is_internal`).

</details>

## HAProxy: session persistence and stick tables
**Persistence** is needed when a session is stored in the memory of a particular server (older applications). Storing sessions in Redis and avoiding persistence is better, but if you need it:
- **cookie** (`cookie SERVERID insert`) — HAProxy itself inserts a cookie with the server name into the first response, and the client comes back to the same server. If the server dies, the client moves to another one (with `option redispatch`);
- **`balance source`** — by IP hash: works in tcp mode too, but clients behind one NAT end up on one server.

**Stick tables** are in-memory "key → counters" tables built into HAProxy (the key is an IP, cookie, header). They are used for persistence and, especially, for **rate limiting and abuse protection**:
```haproxy
frontend fe_main
    bind :80
    stick-table type ip size 100k expire 30s store http_req_rate(10s),conn_cur
    http-request track-sc0 src                                             # track every IP
    http-request deny deny_status 429 if { sc_http_req_rate(0) gt 100 }    # more than 100 requests in 10 s — 429
    http-request deny deny_status 429 if { sc_conn_cur(0) gt 50 }          # more than 50 concurrent connections
    default_backend be_web
```
Tables can be synchronised between several HAProxy instances (the `peers` section) so that limits apply across the whole cluster of load balancers.

### 🧪 Mini task

**Task.** Use a stick table for rate limiting: no more than 5 requests per 10 seconds from one IP, answering 429 when exceeded. Send 10 requests and count the codes. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done
cat > haproxy.cfg <<'EOF'
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  stick-table type ip size 100k expire 30s store http_req_rate(10s)
  http-request track-sc0 src
  http-request deny deny_status 429 if { sc_http_req_rate(0) gt 5 }
  default_backend be_app
backend be_app
  server app1 app1:5678
EOF
docker run -d --name hap --network lb -p 8080:8080 -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3

for i in $(seq 1 10); do curl -s -o /dev/null -w '%{http_code}\n' localhost:8080; done | sort | uniq -c
docker rm -f hap app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
   5 200
   5 429
```

Breakdown: a stick table is an in-memory HAProxy table with a key (here the client IP) and counters. `track-sc0 src` updates the client's entry on every request, and the rule rejects requests once the 10-second rate exceeds 5. The same mechanism gives session persistence: `stick on src` or on a cookie remembers which server served the client, and the table can be synced between two balancers via `peers`. For app-level stickiness `cookie SERVERID insert` is simpler: HAProxy sets a cookie with the server name itself.

</details>

## HAProxy: TLS
```haproxy
frontend fe_tls
    bind :443 ssl crt /etc/haproxy/certs/ alpn h2,http/1.1 ssl-min-ver TLSv1.2
    http-response set-header Strict-Transport-Security "max-age=31536000"
    default_backend be_web
```
- `crt` points to a **PEM file containing the certificate, intermediates and key together**, or to a **directory**: HAProxy loads all certificates and picks the right one by SNI.
- `alpn h2,http/1.1` — enables HTTP/2 for clients.
- **SSL passthrough** — in `mode tcp` HAProxy does not decrypt traffic but only routes by SNI (`req.ssl_sni`). Needed when TLS must terminate on the application itself.
- Encryption to the backend: `server s1 10.0.1.10:8443 ssl verify required ca-file /etc/haproxy/ca.pem`.

### 🧪 Mini task

**Task.** Enable TLS termination in HAProxy: build a PEM from the certificate and key, listen for HTTPS on 8443 and redirect plain HTTP. Check the redirect and the HTTPS response. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done

openssl req -x509 -newkey rsa:2048 -nodes -keyout tls.key -out tls.crt -days 30 -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost" 2> /dev/null
cat tls.crt tls.key > site.pem
cat > haproxy.cfg <<'EOF'
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  bind *:8443 ssl crt /tls/site.pem alpn h2,http/1.1
  http-request redirect scheme https code 301 unless { ssl_fc }
  http-response set-header Strict-Transport-Security "max-age=31536000"
  default_backend be_app
backend be_app
  server app1 app1:5678
EOF
docker run -d --name hap --network lb -p 8080:8080 -p 8443:8443 -v "$PWD:/tls:ro" -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3
curl -s -o /dev/null -w '%{http_code} -> %{redirect_url}\n' http://localhost:8080/pay
curl -s --cacert tls.crt https://localhost:8443/
curl -s --cacert tls.crt -o /dev/null -w 'http_version=%{http_version}\n' https://localhost:8443/
docker rm -f hap app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
301 -> https://localhost:8080/pay
app1
http_version=2
```

Breakdown: HAProxy expects the certificate and key in one PEM file (or a directory of them — then the certificate is chosen by SNI). `ssl_fc` means "the connection came over TLS", and the redirect fires for plain HTTP. The port in the redirect stayed 8080 because HAProxy changes only the scheme: production listens on the standard 80 and 443, so the problem doesn't arise. Traffic reaches the backend over plain HTTP inside the network — that's TLS termination. To encrypt up to the backend, add `ssl verify required ca-file …` to the `server` line. `alpn h2` enables HTTP/2 for clients.

</details>

## HAProxy: the Runtime API and draining a server
The Runtime API controls a running HAProxy through a Unix socket **without a reload**: view statistics, change weights, take servers out for maintenance. Commands are sent with the `socat` utility (a universal "adapter": it connects stdin/stdout to the other end — a socket, a TCP port, a file).
```bash
echo "show stat"  | socat stdio /var/run/haproxy.sock        # statistics (CSV)
echo "show servers state be_api" | socat stdio /var/run/haproxy.sock
echo "show info"  | socat stdio /var/run/haproxy.sock        # general info: connections, version, uptime

echo "set server be_api/api1 state drain" | socat stdio /var/run/haproxy.sock
# drain: no new connections to api1 (except pinned clients), current ones finish
echo "set server be_api/api1 state maint" | socat stdio /var/run/haproxy.sock
# maint: the server is fully out of service
echo "set server be_api/api1 state ready" | socat stdio /var/run/haproxy.sock    # back into service
echo "set server be_api/api1 weight 50%"  | socat stdio /var/run/haproxy.sock    # reduce its share of traffic
```
**How to update a server without downtime** (a rolling update, for example with Ansible):
1. `state drain` — new requests go to the other servers;
2. wait until active connections reach 0 (`show stat`, the `scur` field);
3. `state maint`, stop the application, update it, start it;
4. wait until the server answers on `/health`;
5. `state ready` — the server is back in service; move on to the next one.

Changes made through the Runtime API **are not preserved** across a reload. To keep server state across reloads, use `server-state-file`.

### 🧪 Mini task

**Task.** Open the Runtime API on a TCP port and use `nc` to put `app2` into the `drain` state. Check that new requests go only to `app1`, look at the server states and bring `app2` back. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done
cat > haproxy.cfg <<'EOF'
global
  stats socket ipv4@0.0.0.0:9999 level admin
  log stdout format raw local0
defaults
  mode http
  log global
  stats socket ipv4@0.0.0.0:9999 level admin
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  default_backend be_app
backend be_app
  server app1 app1:5678
  server app2 app2:5678
EOF
docker run -d --name hap --network lb -p 8080:8080 -p 9999:9999 -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3
api() { echo "$1" | nc -w 1 localhost 9999; }
api "set server be_app/app2 state drain"
for i in 1 2 3 4; do curl -s localhost:8080; done | sort | uniq -c
api "show stat" | awk -F, '$1 == "be_app" && $2 ~ /^app/ {print $2, $18}'
api "set server be_app/app2 state ready"
api "show stat" | awk -F, '$1 == "be_app" && $2 == "app2" {print $2, $18}'
docker rm -f hap app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
   4 app1
app1 UP
app2 DRAIN
app2 UP
```

Breakdown: the Runtime API changes HAProxy's state on the fly, without a reload or a config edit. `drain` means the server gets no new requests but finishes its current connections. That's how a server goes into maintenance without a single user-facing error: drain → wait for active sessions to reach 0 → update → `ready`. Exactly what an Ansible rolling-update playbook does via `delegate_to` the balancer. Unlike `drain`, `maint` cuts traffic immediately. In production the socket is a unix file with permissions, not a TCP port: `level admin` means full control over the balancer.

</details>

## HAProxy: statistics and metrics
```haproxy
frontend stats
    bind :8404
    http-request use-service prometheus-exporter if { path /metrics }   # built-in Prometheus exporter
    stats enable
    stats uri /stats
    stats refresh 10s
    stats admin if LOCALHOST                   # server control buttons only from localhost
```
The `/stats` page is a table per frontend, backend and server: current and maximum sessions (`scur`, `smax`), the queue (`qcur`), responses by class (2xx–5xx), health check state, time of the last status change. For Prometheus there is a built-in exporter; no third-party one is needed.

### 🧪 Mini task

**Task.** Enable HAProxy's built-in Prometheus exporter on a separate port, make a few requests and find the server status and the number of 2xx responses in the metrics. The lab: a Docker network `lb` and backends `app1`, `app2` that answer with their own name; the command itself creates and removes everything.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
docker network create lb > /dev/null
for a in app1 app2; do docker run -d --name $a --network lb hashicorp/http-echo -text=$a > /dev/null; done
cat > haproxy.cfg <<'EOF'
global
  log stdout format raw local0
defaults
  mode http
  log global
  timeout connect 2s
  timeout client 10s
  timeout server 10s
frontend fe_main
  bind *:8080
  default_backend be_app
backend be_app
  server app1 app1:5678 check
  server app2 app2:5678 check
frontend fe_metrics
  bind *:8405
  http-request use-service prometheus-exporter if { path /metrics }
EOF
docker run -d --name hap --network lb -p 8080:8080 -p 8405:8405 -v "$PWD/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro" haproxy:3.2 > /dev/null; sleep 3
for i in 1 2 3 4; do curl -s localhost:8080 > /dev/null; done
curl -s localhost:8405/metrics | grep -E '^haproxy_server_status\{proxy="be_app".*state="UP"\}|^haproxy_backend_http_responses_total\{proxy="be_app",code="2xx"\}'
docker rm -f hap app1 app2 > /dev/null; docker network rm lb > /dev/null
```

Expected output:

```text
haproxy_backend_http_responses_total{proxy="be_app",code="2xx"} 4
haproxy_server_status{proxy="be_app",server="app1",state="UP"} 1
haproxy_server_status{proxy="be_app",server="app2",state="UP"} 1
```

Breakdown: the exporter is built into HAProxy, no extra container needed — Prometheus scrapes `/metrics` directly. What matters for alerts and dashboards: server status (`haproxy_server_status`), responses by code class (the 5xx share), queues (`haproxy_backend_current_queue`), active sessions versus limits, and backend response times. The metrics port shouldn't be exposed externally. The same information in human-readable form is on the HTML `stats uri` page — handy to open during an incident.

</details>

## HAProxy: reading the logs
An HTTP log line (`option httplog`) contains timers and **termination flags** — they immediately show who is to blame:
```
203.0.113.7:51234 [19/Oct/2026:10:00:01.123] fe_https~ be_api/api2 0/0/1/-1/30002 504 194 - - sH-- 120/118/30/12/0 0/0 "GET /api/report HTTP/1.1"
```
**Timers** `TR/Tw/Tc/Tr/Ta` (in milliseconds):
- **TR** — how long the client took to send the request headers;
- **Tw** — how long the request **waited in the queue** (grows when servers hit `maxconn`);
- **Tc** — establishing the TCP connection to the server (a large value means an overloaded server or network problems);
- **Tr** — how long the server took to prepare the response (until the first byte of headers);
- **Ta** — the total time.

In the example: 0 ms in the queue, 1 ms to connect to the server, but no response at all (`Tr = -1`), and after 30 seconds (`timeout server`) HAProxy returned 504 to the client. The network and the load balancer are fine — the problem is in the application.

**Termination flags** (4 characters, the first two matter):
- **first — who or what ended the session:** `C` — the client aborted, `S` — the server aborted or refused, `P` — HAProxy itself (for example a deny rule), `R` — out of resources, `c` — client timeout, `s` — server timeout, `-` — normal completion;
- **second — at which stage:** `R` — waiting for the request, `Q` — the queue, `C` — connecting to the server, `H` — waiting for response headers, `D` — data transfer, `-` — normal.

Common combinations:
| Flags | What happened | What the client got |
|---|---|---|
| `sH` | The server did not send response headers within `timeout server` | 504 |
| `SC` | The server refused the connection (port not listening, crashed) | 503 |
| `sQ` | The request sat in the queue longer than `timeout queue` | 503 |
| `SH` | The server dropped the connection before responding (crashed while processing) | 502 |
| `CD` | The client closed the connection itself during data transfer | — |
| `cR` | The client did not send a request within `timeout http-request` (slow client or a slowloris attack) | 408 |
| `PR` | HAProxy itself rejected the request at the request stage (a deny rule) | 403 |

After the flags come connection counters (process, frontend, backend, server, retries) and queues — also useful when investigating overload.

### 🧪 Mini task

**Task.** Take apart an HAProxy HTTP log line: extract the `Tq/Tw/Tc/Tr/Ta` timings, the session termination flags and the queue counters, and explain where the request spent its time and how it ended.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
line='10.0.0.7:51234 [26/Oct/2026:10:00:01.123] fe_main be_app/app2 0/1250/1/30001/31252 504 194 - - sH-- 120/118/40/20/0 0/15 "GET /api/report HTTP/1.1"'
echo "$line" | awk '{
  split($5, t, "/"); split($11, c, "/"); split($12, q, "/")
  print "server:", $4
  print "Tq(request)=" t[1] "ms Tw(queue)=" t[2] "ms Tc(connect)=" t[3] "ms Tr(response)=" t[4] "ms Ta(total)=" t[5] "ms"
  print "status:", $6, "termination flags:", $10
  print "backend queue when served:", q[2]
}'
```

Expected output:

```text
server: be_app/app2
Tq(request)=0ms Tw(queue)=1250ms Tc(connect)=1ms Tr(response)=30001ms Ta(total)=31252ms
status: 504 termination flags: sH--
backend queue when served: 15
```

Breakdown: the timings show where the time went: the request waited 1.25 seconds in the backend queue (`Tw`) because the servers were busy (15 requests were queued). The connection took 1 ms, and then the backend sent no response for 30 seconds. The `sH` flags: `s` — the server timeout fired, `H` — while waiting for the response headers. Hence the 504. Conclusion: the problem is a slow application (and overload — the queue), not the network or HAProxy. Other common flags: `CD` — the client aborted, `SC` — the server refused the connection, `--` — a normal termination.

</details>

## High availability of the load balancer itself
A load balancer removes the single point of failure for applications but becomes one itself. Solutions:
- **keepalived (VRRP** — Virtual Router Redundancy Protocol, the protocol by which several servers agree on who currently owns a shared IP**)** — two or more servers running HAProxy or nginx share a **floating IP address** (VIP, virtual IP). The primary holds the VIP and sends VRRP announcements; if they stop (the server died) or a check script shows HAProxy isn't working, the backup takes over the VIP within seconds. Clients keep going to the same address.
- **Pacemaker and Corosync** — a full cluster manager: manages resources (VIP, the HAProxy service) with quorum (the cluster may run only while it sees a majority of its nodes) and **fencing** (forcibly cutting off a faulty node so that two nodes never both believe they are primary — split-brain). More complex than keepalived but more reliable for complex scenarios.
- **DNS with several addresses** — simple but slow: clients remember a dead server's address for a long time (DNS caching).
- **Anycast and BGP (ECMP)** — several load balancers announce the same address over BGP (anycast — one IP in several places in the network), and routers split traffic between them evenly (ECMP — Equal-Cost Multi-Path, balancing over several equal-cost routes). For very large loads — this is how Cloudflare and big CDNs work.
- **In the cloud** a managed load balancer (AWS ALB or NLB) is usually used, with HAProxy and nginx behind it, inside the cluster.

```
               VIP 10.0.0.100 (moves on failure)
                   │
        ┌──────────┴──────────┐
   lb1: HAProxy           lb2: HAProxy
   keepalived MASTER      keepalived BACKUP
        └──────────┬──────────┘
         app1   app2   app3
```

### VIP and VRRP: how it works
A **VIP (virtual IP, also floating IP)** is an address not tied to a specific server. DNS and clients use it, while physically it sits on the network interface of whichever server is currently primary. Technically it is just a second address on the interface: `ip addr add 10.0.0.100/24 dev eth0`.

For traffic to reach the new server after the VIP moves, that server sends a **gratuitous ARP**. ARP (Address Resolution Protocol) is how hosts on a local network find the MAC address (the hardware address of a network card) for an IP address: inside the network, packets are delivered by MAC. A gratuitous ("unsolicited") ARP is a reply nobody asked for: "IP 10.0.0.100 now belongs to this MAC address". The switch and neighbouring hosts update their ARP tables, and packets go to the new machine. Without it, clients would keep sending packets to the dead server's MAC address for another minute or two.

**VRRP (Virtual Router Redundancy Protocol)** is the protocol servers use to decide who holds the VIP:
- every server in the group has a `priority` (1–254); the highest is **MASTER**, the rest are **BACKUP**;
- every `advert_int` seconds (usually 1) the MASTER sends an advertisement via multicast (group delivery: a packet to 224.0.0.18 reaches all VRRP nodes in that network segment at once) to 224.0.0.18. It is a separate IP protocol number 112, neither TCP nor UDP — the firewall must allow it;
- if a BACKUP hears no advertisements for about three intervals (~3 seconds), it considers the MASTER dead, brings the VIP up locally and sends a gratuitous ARP.

A typical situation: lb1 hangs, and 3–4 seconds later the VIP is already on lb2. Users see a couple of broken requests, but nobody has to change DNS.

### keepalived: an example for HAProxy and nginx
**keepalived** is a daemon that implements VRRP on Linux and can check not only "is the server alive" but also "is the service on it working". It does not care what the service is: for nginx the setup is the same, only the check script changes.

```conf
# /etc/keepalived/keepalived.conf on lb1
vrrp_script chk_lb {
    script "/usr/bin/pidof haproxy"
    interval 2
    weight -20
    fall 2
    rise 2
}

vrrp_instance VI_1 {
    state MASTER
    interface eth0
    virtual_router_id 51
    priority 110
    advert_int 1
    authentication {
        auth_type PASS
        auth_pass s3cr3t
    }
    virtual_ipaddress {
        10.0.0.100/24
    }
    track_script {
        chk_lb
    }
}
```

lb2 is identical except for `state BACKUP` and `priority 100`. What matters here:
- `virtual_router_id` (0–255) is the VRRP group number: the same on both servers and unique within the network. If another keepalived pair in the same network uses the same number, they start interfering with each other.
- `vrrp_script` is the service check. For nginx: `script "/usr/bin/pidof nginx"`, or more reliably `curl -fs http://127.0.0.1/health`, because the process may be alive but not responding. `fall 2` / `rise 2` set how many failures or successes in a row are needed to change status (protection against flapping on a single random error).
- `weight -20`: if the check fails, priority drops by 20: lb1 goes 110 → 90, which is below lb2's 100, so the VIP moves. Without `weight`, the script puts the server into FAULT state (faulty: the server drops out of the MASTER election until the check passes again), and it gives up the VIP as well.
- By default, when lb1 recovers it takes the VIP back (preempt). That is a second short outage. To avoid it, set `state BACKUP` and `nopreempt` on both.

The bind trap: if HAProxy or nginx listens specifically on `10.0.0.100:443`, it will not start on the BACKUP, where the VIP is absent (`cannot bind socket`). Fixes: listen on `*:443`, or allow binding to a non-local address — `sysctl -w net.ipv4.ip_nonlocal_bind=1`. Then the service runs on both nodes in advance, and nothing needs restarting when the VIP moves.

### Pacemaker and Corosync: how it works
This is a full cluster stack (for example, RHEL — Red Hat Enterprise Linux — ships it as the High Availability Add-On). It manages not just an IP but any **resources**: a VIP, the HAProxy service, a disk mount, a database. It has two layers:

- **Corosync** is the transport and membership layer. Nodes constantly exchange messages (UDP port 5405 by default) and know who in the cluster is alive. It also computes **quorum** (the minimum number of votes the cluster needs to manage resources; usually a strict majority, more than half of the nodes). A 3-node cluster keeps working while at least 2 are visible. If the network splits the cluster, only the part with the majority keeps the resources, and the smaller part stops its own — otherwise both halves would bring up the VIP and services in parallel. That is why clusters have an odd number of nodes: with 4 nodes and a 2+2 split nobody has a majority, and the whole cluster stops.
- **Pacemaker** is the "brain", the CRM (Cluster Resource Manager). It stores the desired state in the CIB (Cluster Information Base, the cluster's XML config, identical on all nodes) and decides which node each resource should run on. If a resource fails, it first tries to restart it in place, then moves it to another node.
- **Resource agents** are the scripts Pacemaker uses to start, stop and monitor a resource: `ocf:heartbeat:IPaddr2` for a VIP (brings the address up and sends a gratuitous ARP), `systemd:haproxy` for the service.
- **Constraints** are placement rules: `colocation` (HAProxy must run where the VIP is), `order` (VIP first, then HAProxy), `location` (prefer node lb1).
- **Fencing** (literally "putting up a fence" — isolating a faulty node from shared resources) and its most common form **STONITH** (Shoot The Other Node In The Head, i.e. forcibly power it off). If a node stops responding, Pacemaker does not guess whether it is alive: the node may simply have hung or lost the network while still holding the VIP and writing to disk. So it is forcibly powered off or rebooted through a fence agent: via IPMI (Intelligent Platform Management Interface — a standard network interface for managing server hardware that bypasses the OS and can cut power) or iLO (Integrated Lights-Out — the same kind of controller on HPE servers; Dell calls it iDRAC), via the hypervisor API (vSphere, Proxmox) or the cloud API. Only after confirmation does it start the resources on another node. This guarantees there is no **split-brain**, where two nodes hold the VIP at the same time or write to the same disk. Quorum and fencing complement each other: quorum decides *which* part of the cluster is allowed to run, and fencing guarantees that the losing node has really stopped, not just gone quiet.

How it is usually configured (the `pcs` tool):

```bash
pcs resource create vip ocf:heartbeat:IPaddr2 ip=10.0.0.100 cidr_netmask=24 op monitor interval=10s
pcs resource create haproxy systemd:haproxy op monitor interval=5s
pcs constraint colocation add haproxy with vip INFINITY
pcs constraint order vip then haproxy
pcs status
```

`pcs status` shows where each resource is running: `vip (ocf:heartbeat:IPaddr2): Started lb1`. To move it manually (for example, before maintenance on lb1) use `pcs resource move vip lb2`, and afterwards always `pcs resource clear vip`: `move` leaves a permanent "do not run on lb1" rule that is easy to forget.

The two-node caveat: on a network split each half has one vote out of two, so nobody has a majority. That is why a two-node `corosync.conf` sets `two_node: 1` (quorum is kept with a single node, and fencing protects against split-brain) or adds a third vote — a **qdevice** (quorum device: a lightweight `corosync-qnetd` arbiter on a separate machine that gives its vote to one of the halves, so that half gets a majority).

### Docker Swarm, HAProxy and VIP
Swarm has its own **service VIP**, and it is easy to confuse with the external VIP. They are different things:
- **Swarm service VIP** is an internal address in the overlay network (a virtual network spanning the hosts). Every service gets one automatically, and IPVS (the load balancer in the Linux kernel) spreads requests to that address across the service's tasks (containers). It is not visible from outside the cluster.
- **Routing mesh (ingress)**: a port published with `--publish 443:443` is open on **every** Swarm node, even where no container runs; the request is forwarded to a node that has one.
- **External VIP** is the address for outside clients. Swarm cannot provide one by itself, so keepalived or Pacemaker is installed next to it on the hosts (not in containers), and the VIP floats between nodes.

The typical setup: HAProxy runs on several nodes (as a global Swarm service `--mode global` or as a plain systemd service), the external VIP floats between those nodes, and HAProxy proxies requests to the Swarm services. If Pacemaker runs on the same hosts as Swarm, there are two quorums: the Swarm managers' (they agree via Raft — a consensus algorithm where decisions are made by a majority of managers, which is why you run 3 or 5 managers) and Corosync's. They are independent, and during network problems their decisions can diverge: the VIP on one node, the Swarm leader on another. That is fine for the VIP; what matters is that HAProxy on the VIP node is alive.

### keepalived or Pacemaker: which to choose
| | keepalived | Pacemaker + Corosync |
|---|---|---|
| What it can do | VIP over VRRP + a service check script | any resources, dependencies, start order |
| Split-brain protection (two nodes both believing they are primary) | none: on a network split both become MASTER | quorum (only the part of the cluster with a majority of nodes runs) + fencing/STONITH (an unresponsive node is forcibly powered off via IPMI or the hypervisor) |
| Complexity | one config, set up in 10 minutes | several components, `pcs`, you need to understand quorum |
| When to use | a pair of HAProxy or nginx load balancers — the classic | when besides the VIP you need to move state: a shared disk, a database, DRBD (Distributed Replicated Block Device — network mirroring of a disk between nodes); enterprise environments with RHEL HA (Red Hat Enterprise Linux High Availability) |

For a load balancer, keepalived's split-brain is nearly harmless: HAProxy keeps no state, and two nodes with the same VIP only cause brief network oddities. For a database, split-brain means two primaries writing different data — that is why it needs Pacemaker with fencing.

### 🧪 Mini task

**Task.** Two load balancers with keepalived from the example above: lb1 (`priority 110`, `weight -20`) and lb2 (`priority 100`). HAProxy on lb1 was stopped. Work out where the VIP ends up, and verify it on the servers: which node has the address, what keepalived wrote to the journal, and who is sending VRRP advertisements now.

<details class="qa"><summary>Solution: command and expected result</summary>

Calculation: the `pidof haproxy` check on lb1 failed twice in a row (`fall 2`, ~4 seconds), lb1's priority became 110 − 20 = 90 < 100, so lb2 is now MASTER.

Commands on lb1 and lb2:

```bash
sudo systemctl stop haproxy          # lb1 only
ip -brief addr show eth0
sudo journalctl -u keepalived --since "5 min ago" | grep -E 'Entering (MASTER|BACKUP)'
sudo tcpdump -ni eth0 -c 2 vrrp
```

Expected result on lb2:

```text
eth0  UP  10.0.0.12/24 10.0.0.100/24
Oct 30 12:00:05 lb2 Keepalived_vrrp[812]: (VI_1) Entering MASTER STATE
IP 10.0.0.12 > 224.0.0.18: VRRPv2, Advertisement, vrid 51, prio 100, authtype simple, intvl 1s, length 20
IP 10.0.0.12 > 224.0.0.18: VRRPv2, Advertisement, vrid 51, prio 100, authtype simple, intvl 1s, length 20
```

Explanation: the VIP `10.0.0.100` is now the second address on lb2's interface, and it is gone from `ip addr` on lb1. lb1's journal will show `Entering BACKUP STATE`. Advertisements come from whoever is MASTER — `tcpdump` shows the sender and its priority. After `systemctl start haproxy` on lb1 its priority returns to 110, and without `nopreempt` the VIP moves back. If `tcpdump` on the BACKUP is empty and both nodes log `Entering MASTER STATE`, that is split-brain (both nodes hold the VIP at once): the firewall is blocking protocol 112 (VRRP) or multicast.

</details>

## Nginx and HAProxy in Kubernetes
- **Ingress controllers**: based on nginx — **ingress-nginx** (from the Kubernetes community) and NGINX Ingress Controller (from F5); based on HAProxy — HAProxy Kubernetes Ingress Controller. The controller watches Ingress and Service objects and generates the configuration itself.
- The Kubernetes community announced the **end of support for ingress-nginx** (in 2026). For new clusters people look at **Gateway API** implementations (Envoy Gateway, Cilium, Istio, NGINX Gateway Fabric, HAProxy) or at cloud controllers (AWS Load Balancer Controller). Worth knowing: migrating off ingress-nginx is a frequent interview topic right now.
- Settings that in plain nginx go into the config (`client_max_body_size`, timeouts, rate limits) are set in ingress-nginx through Ingress **annotations** (`nginx.ingress.kubernetes.io/proxy-body-size: "50m"`) or in the controller's shared ConfigMap.
- **Envoy** — the third important proxy: the foundation of Istio, Envoy Gateway and many API gateways. Its configuration arrives dynamically over an API (xDS — a set of gRPC protocols through which a control server hands out route, cluster and address lists to proxies on the fly) rather than from a file, which suits constantly changing environments like Kubernetes, where pods appear and disappear every minute.

## Nginx or HAProxy: which to choose
| | Nginx | HAProxy |
|---|---|---|
| Main role | General-purpose web server and reverse proxy | Specialised L4 and L7 load balancer |
| Static files, caching | A strong point: serving files, sendfile, proxy_cache | Does not serve files, minimal cache |
| Active health checks | Only in commercial NGINX Plus | Out of the box, flexible (HTTP, TCP, agent) |
| Queues and per-server maxconn | No queues: when `max_conns` is reached (open source) the request goes to another server or gets an error | A queue with a timeout — protects servers from overload |
| Changes on the fly | Only reload (dynamic API in Plus) | Runtime API: drain, maint, weights, state without reload |
| Statistics | `stub_status` — a few counters | A detailed page and a built-in Prometheus exporter |
| Rate limiting | `limit_req`, `limit_conn` | Stick tables: flexible counters on any key, synchronised between nodes |
| Model | Several worker processes | One process, many threads, shared state |
| Typical use | Site front, static files, TLS, cache, ingress | Balancing databases and services, TCP, complex rules, high loads |

In practice they are often used **together**: HAProxy accepts and distributes traffic, while nginx next to the application serves static files and proxies to the application (for example, PHP-FPM).

## Interview questions

**Why can nginx hold tens of thousands of connections with only a few processes?**
Each worker is a single thread with an event loop: non-blocking sockets and epoll, which reports which sockets have events. The thread never waits for any client; it handles ready events one after another, so an idle connection costs only a little memory, not a separate thread or process as in the old thread-per-connection model.

**How does nginx choose a location?**
First an exact match `=`; then the longest prefix — if it has `^~`, the search stops; then the regular expressions `~` and `~*` in config order, the first match wins; if none matches, the longest prefix found earlier is used.

**How does nginx choose a server for a request with an unknown Host?**
Among servers on that address and port it looks for a `server_name` match (exact, leading wildcard, trailing wildcard, regular expressions). If nothing matches, the server marked `default_server` is used, and without one the first server defined on that port. That is why it is worth defining an explicit default server that rejects such requests.

**What happens on `nginx -s reload`? Are connections dropped?**
The master validates the new configuration (on error it keeps the old one), starts new workers with the new configuration and gracefully shuts down the old ones: they stop accepting new connections, finish the current ones and exit. The listening sockets are never closed, so no connections are lost.

**What is the difference between `proxy_pass http://backend` and `proxy_pass http://backend/`?**
Without a URI part the request path is passed to the backend as is. With a URI (even a single slash) the matched location prefix is replaced by that URI: `location /api/` with `proxy_pass http://backend/` turns `/api/users` into `/users`.

**How do you enable keep-alive from nginx to backends?**
`keepalive N` in the upstream block, plus `proxy_http_version 1.1` and `proxy_set_header Connection ""` in the location. By default nginx uses HTTP/1.0 and closes the connection after every request.

**Where do 502 errors in nginx come from and how do you investigate them?**
Look at error.log: `Connection refused` — the backend is not listening on the port; `upstream prematurely closed connection` — the backend crashed during the request or closed a keep-alive connection before nginx did (the backend's keep-alive timeout must be longer than nginx's); `upstream sent too big header` — increase `proxy_buffer_size`; `no live upstreams` — all servers are marked down.

**How does 502 differ from 504, and what is 499?**
502 — the backend responded incorrectly or dropped the connection. 504 — the backend did not respond within `proxy_read_timeout`. 499 — a non-standard nginx code: the client itself closed the connection before getting the response (often a load balancer in front of nginx with its own, shorter timeout).

**Why do files larger than 1 MB fail to upload?**
`client_max_body_size` is 1 MB by default; when exceeded nginx returns 413. Raise the limit for the relevant location (in ingress-nginx — with the `proxy-body-size` annotation).

**How do you limit the number of requests per IP in nginx?**
`limit_req_zone $binary_remote_addr zone=... rate=10r/s` in http and `limit_req zone=... burst=20 nodelay` in the location, with `limit_req_status 429` as the response code. It works as a leaky bucket. Behind a load balancer, set up realip first, otherwise all clients look like one IP.

**Why does the application behind nginx see the proxy's IP and redirect to http://?**
The backend sees a connection from nginx. You need to pass `X-Real-IP` and `X-Forwarded-For` with the client address and `X-Forwarded-Proto` with the scheme, and the application must trust them (only from proxy addresses). If there is another load balancer in front of nginx, configure the realip module with `set_real_ip_from`.

**How do you proxy WebSocket through nginx?**
Pass the `Upgrade` and `Connection` headers explicitly (via a `map` on `$http_upgrade`), use `proxy_http_version 1.1` and increase `proxy_read_timeout`, otherwise an idle connection is closed after 60 seconds.

**What health checks do nginx and HAProxy have?**
Open-source nginx has only passive checks: a server is excluded after `max_fails` errors on real requests within `fail_timeout`. Active checks are in NGINX Plus. HAProxy checks servers periodically itself (`check`, `option httpchk`, `inter`, `fall`, `rise`) and removes them before real requests suffer.

**What does a server's maxconn do in HAProxy, and why have a queue?**
It limits the number of concurrent requests to a server. Excess requests queue inside HAProxy and wait up to `timeout queue`, after which they get 503. This keeps the server in its efficient regime instead of drowning under peak load, and overall throughput and latency are better than under overload.

**Which HAProxy timeouts should you know?**
`timeout connect` — connecting to the server; `timeout client` and `timeout server` — inactivity of each side; `timeout http-request` — receiving request headers (slowloris protection); `timeout queue` — waiting in the queue; `timeout tunnel` — for WebSocket and other long connections; `timeout http-keep-alive` — waiting for the next request on a keep-alive connection.

**How do you take a server out of balancing for an update without losing requests?**
Through the Runtime API: `set server backend/server state drain` — new requests stop, current ones finish; wait for zero active connections; `state maint`, update; make sure the health check passes; `state ready`. In nginx — mark the server `down` or remove it from the upstream and reload.

**How do you tell from an HAProxy log who is to blame for an error?**
By the termination flags and timers. `sH` — the server did not respond within timeout server (504), `SC` — the server refused the connection (503), `sQ` — the request waited too long in the queue (503), `SH` — the server dropped the connection (502), `CD` — the client left on its own. The `Tw` timer shows queue time, `Tc` — connecting to the server, `Tr` — server response time.

**How do you make the load balancer itself highly available?**
At least two nodes and a floating IP via keepalived (VRRP) or Pacemaker and Corosync with fencing to protect against split-brain. Alternatives are several addresses in DNS, anycast with BGP, and in the cloud a managed load balancer (ALB, NLB) in front of HAProxy or nginx.

**What are stick tables in HAProxy?**
In-memory counter tables keyed by anything (IP, cookie, header): request rate, number of connections, errors. They are used to pin a client to a server and to rate-limit and block abuse; they can be synchronised between nodes via peers.

**Nginx or HAProxy — which do you choose?**
Nginx — when you need static files, caching, web server features, TLS termination in front of an application. HAProxy — when balancing is the main job: active checks, queues and maxconn, managing servers on the fly, detailed statistics, TCP balancing of databases and services. They are often used together.

**What happened to ingress-nginx and what should be used instead?**
The Kubernetes community stopped developing and supporting ingress-nginx. New clusters choose Gateway API implementations (Envoy Gateway, Cilium, Istio, NGINX Gateway Fabric, HAProxy) or cloud controllers such as the AWS Load Balancer Controller; existing clusters need a migration plan.

**How does connecting to the backend through a Unix socket differ from TCP? When do you choose which?**
A Unix socket is a file through which processes exchange data on the same machine, bypassing the network stack: no TCP handshakes, ports or TIME_WAIT, less overhead, and access is restricted by file permissions. But it only works within one machine (or between containers of one pod via a shared volume). If nginx and the application are on the same host, like nginx and PHP-FPM, a socket is better; if the backend is on another server or there are several of them, only TCP.

**nginx returns 502 and the log says `connect() to unix:/run/php/php8.3-fpm.sock failed`. How do you investigate?**
By the error code in parentheses. `2: No such file or directory` — there is no socket: the application is not running, the path is wrong, or the socket is in an isolated `PrivateTmp`. `13: Permission denied` — permissions on the file or directory, the nginx user, SELinux. `111: Connection refused` — a file left over from a crashed process. `11: Resource temporarily unavailable` — the connection queue is full, all PHP-FPM processes are busy. Check with `ls -l` on the socket, `ss -xlp` and the application logs.

**WebSocket connections through nginx drop roughly once a minute. Why, and what do you do?**
It is almost always an idle timeout: by default nginx waits 60 seconds for data from the backend (`proxy_read_timeout`), the AWS ALB idle timeout is the same 60 seconds by default, and HAProxy has `timeout tunnel`. If nothing is sent over the connection for a minute, one of the links closes it. The fix is to raise the timeout at every link of the path and, most reliably, send a ping from the application every 20–30 seconds; the client must also be able to reconnect.
