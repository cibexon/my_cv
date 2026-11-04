# Observability and SRE

Observability and SRE practices in depth: metrics, logs and traces; Prometheus and PromQL with queries explained; monitoring methodologies; SLIs, SLOs and the error budget; good alerting; incident management and postmortems; reliability patterns. Each topic: what it is, why, how it works, examples with explanations.

## Monitoring and observability

**Monitoring** answers **questions known in advance**: "is CPU usage above 80%?", "does the service respond?". You decided in advance what to measure and built dashboards and alerts.

**Observability** is a property of a system that lets you answer **new, unforeseen** questions from its telemetry: "why is checkout slow for users from one region on the mobile app after yesterday's release?". This requires rich data with details (which user, version, region, request path) and the ability to slice it arbitrarily.

In a distributed system of dozens of microservices, a failure rarely looks like "the service is down". More often it's a degradation that no dashboard anticipated. That's why monitoring alone is not enough.

### 🧪 Mini task

**Task.** Run Prometheus in Docker and check it two ways: as a black box (does the health check answer and how fast) and as a white box (what the service reports about itself on `/metrics`).

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run -d --name prom -p 9090:9090 prom/prometheus > /dev/null; sleep 15
curl -s -o /dev/null -w 'black-box: %{http_code} in %{time_total}s\n' http://localhost:9090/-/healthy
curl -s http://localhost:9090/metrics | grep -E '^(prometheus_tsdb_head_series|go_goroutines|process_open_fds) '
docker rm -f prom > /dev/null
```

Expected output:

```text
black-box: 200 in 0.002871s
go_goroutines 41
process_open_fds 31
prometheus_tsdb_head_series 532
```

Breakdown: an outside check answers "does the service work for the user" but doesn't explain why it broke. Metrics from inside show the state: goroutines, open descriptors, series in memory. Monitoring means known checks and alerts for known problems. Observability is the ability to understand, from telemetry (metrics, logs, traces), a problem nobody foresaw. Prometheus scrapes itself, so it also stores these metrics. Your numbers will differ.

</details>

## Telemetry signals

### Metrics
Numeric values over time: the number of requests, the error rate, latency, CPU usage.
- **Upsides:** cheap to store (one number per interval regardless of the number of requests), fast queries, ideal for **alerts, dashboards and trends**.
- **Downsides:** aggregated, no details about an individual request. A metric shows that 2% of requests fail but not which ones or why.
- Tools: Prometheus, VictoriaMetrics (a Prometheus-compatible metrics database, more frugal with memory and disk), CloudWatch, Datadog (a commercial SaaS monitoring platform).

### Logs
Records of events with details.
- **Upsides:** the maximum of detail about a specific event (a stack trace, parameters, the user ID).
- **Downsides:** expensive to store and search at large volumes.
- **Structured logs** (JSON, not free text) are a must: you can filter and aggregate by fields.
  ```json
  {"ts":"2026-10-12T10:00:01Z","level":"error","service":"checkout","trace_id":"4bf92f35","user_id":"u-123","msg":"payment declined","provider":"stripe","latency_ms":2300}
  ```
- **A trace_id in every line** links logs to traces, and the logs of different services handling one request to each other.
- Tools: Loki, Elasticsearch/OpenSearch (ELK — Elasticsearch, Logstash, Kibana), CloudWatch Logs. Collectors (agents that read logs on hosts and ship them to storage): Fluent Bit, Vector (a Rust collector from Datadog), Promtail.

### Traces (distributed tracing)
The path of one request through all the services. A **trace** consists of **spans** — operations with a start time, duration and attributes: "a request to API Gateway 340 ms → the orders service 300 ms → a database query 250 ms → a call to the payment service 30 ms".
- They answer the question "**where** is time lost and where does an error occur" in a chain of services.
- The trace context is passed between services in a header (the W3C `traceparent` standard).
- Usually not all traces are kept but a sample (**sampling**): 1–10% plus all failed and slow requests (tail sampling — the decision "keep this trace or not" is made at the end, when it's already known whether the request was slow or failed; as opposed to head sampling, where the coin is tossed at the start of the request).
- Tools: Jaeger, Grafana Tempo (storage and UIs for traces), AWS X-Ray, Datadog APM (Application Performance Monitoring).

### Profiling
Continuous profiling (Pyroscope, Parca — they constantly profile running processes with low overhead) shows which code functions consume CPU and memory — the next level of detail after a trace.

### OpenTelemetry
**OpenTelemetry (OTel)** is an open standard and a set of SDKs and tools for collecting metrics, logs and traces. An application is instrumented once with the OTel SDK (much of it automatically: HTTP clients, database drivers), and the **OTel Collector** receives the telemetry, processes it (filters, adds attributes, samples) and sends it to any backend. Switching monitoring vendors doesn't require rewriting the instrumentation.

### 🧪 Mini task

**Task.** You have structured JSON logs from three services with a shared `trace_id`. Find the request that ended in an error and reconstruct its path through all services with each step's duration.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > logs.jsonl <<'EOF'
{"ts":"10:00:01.000","svc":"gateway","trace_id":"a1","level":"info","msg":"POST /orders","ms":12}
{"ts":"10:00:01.010","svc":"gateway","trace_id":"b2","level":"info","msg":"POST /orders","ms":2040}
{"ts":"10:00:01.020","svc":"orders","trace_id":"b2","level":"info","msg":"create order","ms":2025}
{"ts":"10:00:01.030","svc":"orders","trace_id":"a1","level":"info","msg":"create order","ms":8}
{"ts":"10:00:03.040","svc":"payments","trace_id":"b2","level":"error","msg":"bank timeout","ms":2000}
EOF
bad=$(jq -r 'select(.level == "error") | .trace_id' logs.jsonl)
echo "failed trace: $bad"
jq -r --arg t "$bad" 'select(.trace_id == $t) | "\(.svc)\t\(.ms)ms\t\(.msg)"' logs.jsonl
```

Expected output:

```text
failed trace: b2
gateway	2040ms	POST /orders
orders	2025ms	create order
payments	2000ms	bank timeout
```

Breakdown: with plain text logs you'd search by time and hope for a match. Structured logs with a `trace_id` let one query collect the request's path through all services and show that 2 of the 2.04 seconds were spent waiting for the bank in `payments`. That's exactly what a trace in Jaeger or Tempo shows as a diagram. Metrics would say "/orders latency went up", logs "what happened", traces "where exactly". OpenTelemetry ties all three signals together with one `trace_id`.

</details>

## Prometheus

### How it works
- **The pull model:** Prometheus itself periodically (the scrape interval, usually 15–30 s) polls the `/metrics` HTTP endpoint of every monitored target. The upsides: Prometheus knows which targets should exist, and `up == 0` immediately shows an unavailable target; the application doesn't need to know the address of the monitoring system.
- **Service discovery** — where Prometheus gets its list of targets: from the Kubernetes API (pods, services), from AWS EC2, Consul (HashiCorp's service registry), files. In Kubernetes with the **Prometheus Operator**, targets are described by `ServiceMonitor` and `PodMonitor` objects.
- **Exporters** — programs that turn the metrics of third-party systems into the Prometheus format: `node_exporter` (Linux host metrics), `kube-state-metrics` (the state of Kubernetes objects), `blackbox_exporter` (HTTP, TCP, DNS availability checks from outside), `postgres_exporter`, `redis_exporter`.
- **Pushgateway** — for short tasks (batch, cron) that don't live long enough to be scraped. Not for ordinary services.
- **Storage** — a local time-series database (TSDB) on disk, usually for 15–30 days. For long-term storage and combining several Prometheus servers — **Thanos** (an add-on to Prometheus: it puts data into S3 and provides a single query across all servers), **Grafana Mimir**, **VictoriaMetrics**, Amazon Managed Prometheus.
- **Alertmanager** — a separate component that receives fired alerts: it groups them (one alert instead of a hundred identical ones), deduplicates, applies silences (temporary muting), inhibition (if the whole cluster is down, don't send an alert about every service) and routes them (Slack, PagerDuty, Opsgenie — on-call services that call and message the on-call engineer and escalate if they don't answer; Telegram) by labels.

### The data model
Each **time series** is defined by a metric name and a set of **labels**:
```
http_requests_total{service="checkout", method="POST", status="500"}  1027
```
Every unique combination of labels is a **separate time series**.

### Metric types
**Counter** — only grows (and resets to 0 when the process restarts): the number of requests, errors, processed bytes. The value itself is useless ("there have been 1,027 errors since startup"). What matters is the **rate of growth** — the `rate()` function.

**Gauge** — a current value that can go up and down: memory usage, temperature, queue length, the number of active connections.

**Histogram** — the distribution of values across **buckets**: how many requests completed within 0.1 s, 0.25 s, 0.5 s, 1 s and so on. Percentiles are computed from it (`histogram_quantile`). It creates several series: `_bucket{le="..."}`, `_sum`, `_count`.

**Summary** — percentiles are computed in the application itself. They **can't be aggregated** across instances (you can't average percentiles), which is why histograms are usually preferred.

### Cardinality — Prometheus's main threat
The number of time series = the product of the numbers of values of all labels. A `user_id` label with a million users turns one metric into a million series: Prometheus eats all its memory and crashes.

**You must not use in labels** values with unbounded variety: user, order and request IDs, emails, full URLs with parameters (`/orders/12345`), error text. Normalise them (`/orders/{id}`) or send such data to logs and traces.

### PromQL — the query language explained
**The request rate (RPS) per service:**
```promql
sum by (service) (rate(http_requests_total[5m]))
```
- `http_requests_total[5m]` — the counter's values over the last 5 minutes (a range vector);
- `rate(...)` — the average per-second rate of growth over those 5 minutes, correctly handling counter resets on restart;
- `sum by (service)` — add up the series of all pods and methods, keeping the breakdown by service only.

**The error rate:**
```promql
sum by (service) (rate(http_requests_total{status=~"5.."}[5m]))
  /
sum by (service) (rate(http_requests_total[5m]))
```
`status=~"5.."` is a regular expression: all statuses starting with 5. The result is a fraction from 0 to 1.

**The 99th percentile of latency:**
```promql
histogram_quantile(0.99,
  sum by (le, service) (rate(http_request_duration_seconds_bucket[5m]))
)
```
The `le` label (the bucket's upper bound) must be kept in `by`: the function reconstructs the distribution from it. "p99 = 0.8 s" means that 99% of requests are faster than 0.8 seconds and 1% are slower.

**CPU consumption by pods:**
```promql
sum by (pod) (rate(container_cpu_usage_seconds_total{namespace="prod", container!=""}[5m]))
```
The result is in cores (0.5 = half a core). `container!=""` removes the pod-level aggregated series so it isn't counted twice.

**The share of CPU throttling** (how much a container hits its limit):
```promql
sum by (pod) (rate(container_cpu_cfs_throttled_periods_total[5m]))
  /
sum by (pod) (rate(container_cpu_cfs_periods_total[5m]))
```

**Memory usage relative to the limit:**
```promql
sum by (pod) (container_memory_working_set_bytes{container!=""})
  /
sum by (pod) (kube_pod_container_resource_limits{resource="memory"})
```
`working_set` is the memory the OOM killer acts on, which is why it's the one compared.

**Pods that restart often:**
```promql
increase(kube_pod_container_status_restarts_total[1h]) > 3
```
`increase` — how much the counter grew over an hour (the same as `rate` multiplied by the window length).

**A forecast: will the disk run out of space within a day:**
```promql
predict_linear(node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"}[6h], 24 * 3600) < 0
```
A linear extrapolation of the 6-hour trend 24 hours ahead. Such an alert fires **before** the problem, not when the disk is already full.

**Unavailable targets:**
```promql
up == 0
```

**Why always `rate()` and not the counter's value.** A counter grows monotonically from process startup, its absolute value says nothing about the current state, and it resets to 0 on restart. `rate` shows the current rate and handles resets correctly.

### An alerting rule
```yaml
groups:
  - name: checkout
    rules:
      - alert: CheckoutHighErrorRate
        expr: |
          sum(rate(http_requests_total{service="checkout", status=~"5.."}[5m]))
            /
          sum(rate(http_requests_total{service="checkout"}[5m])) > 0.05
        for: 10m                      # the condition must hold for 10 minutes — protection against short spikes
        labels:
          severity: critical          # Alertmanager decides by this label whether to wake the on-call engineer
          team: payments
        annotations:
          summary: "Checkout: more than 5% of 5xx errors"
          description: "5xx share {{ $value | humanizePercentage }} over the last 10 minutes"
          runbook_url: https://wiki.example.com/runbooks/checkout-errors
          dashboard: https://grafana.example.com/d/checkout
```

### 🧪 Mini task

**Task.** Run Prometheus and use the HTTP API to execute three PromQL queries: which targets are up (`up`), how many requests per second its own HTTP server handles (`rate`), and how many series the most "multiplying" metric has.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
docker run -d --name prom -p 9090:9090 prom/prometheus > /dev/null; sleep 15
q() { curl -s http://localhost:9090/api/v1/query --data-urlencode "query=$1" | jq -r '.data.result[] | "\(.metric.job // .metric.__name__ // "-") \(.value[1])"'; }
q 'up'
q 'sum(rate(prometheus_http_requests_total[1m]))'
q 'topk(1, count by (__name__) ({__name__=~"prometheus_.+"}))'
docker rm -f prom > /dev/null
```

Expected output:

```text
prometheus 1
- 0.06666666666666667
prometheus_http_request_duration_seconds_bucket 140
```

Breakdown: `up` is a synthetic metric Prometheus creates for every scrape target: 1 means it answered, 0 means it didn't. The most basic alert is built on it. `rate()` turns a growing counter into a per-second rate and handles resets on restart; a raw counter value is meaningless. A histogram metric yields many series: every `le` bucket × every label combination. That's how cardinality grows — Prometheus's main threat — which is why user_id or URLs with parameters must never go into labels. Your numbers will differ.

</details>

## The telemetry pipeline: OTel Collector, Fluent Bit, Mimir, ClickHouse
In a large platform, telemetry isn't sent straight from the application to storage but passed through a **pipeline**: agents on the hosts collect, enrich and buffer the data, and the storage backends scale separately.

### OpenTelemetry Collector
It receives metrics, logs and traces (OTLP — the native OpenTelemetry protocol over gRPC or HTTP — and dozens of other formats), processes them and sends them to several backends. The configuration is pipelines of **receivers → processors → exporters**:
```yaml
# otelcol-contrib: hostmetrics, prometheusremotewrite and clickhouse are in the contrib distribution
receivers:
  otlp:
    protocols:
      grpc: { endpoint: 0.0.0.0:4317 }
      http: { endpoint: 0.0.0.0:4318 }
  hostmetrics:
    collection_interval: 15s
    scrapers: { cpu: {}, memory: {}, disk: {}, network: {} }

processors:
  memory_limiter:                    # first: protects the collector itself from OOM during a data spike
    check_interval: 1s
    limit_percentage: 80
    spike_limit_percentage: 20
  resource:
    attributes:
      - { key: deployment.environment, value: prod, action: upsert }
  batch:                             # last: sending in batches saves network and CPU
    timeout: 5s
    send_batch_size: 8192

exporters:
  prometheusremotewrite:
    endpoint: https://mimir.internal/api/v1/push
    headers: { X-Scope-OrgID: platform }   # the tenant in Mimir
  otlphttp/tempo:
    endpoint: https://tempo.internal:4318
  clickhouse:
    endpoint: tcp://clickhouse.internal:9000
    database: otel
    ttl: 720h                        # keep for 30 days

service:
  pipelines:
    metrics: { receivers: [otlp, hostmetrics], processors: [memory_limiter, resource, batch], exporters: [prometheusremotewrite] }
    traces:  { receivers: [otlp], processors: [memory_limiter, resource, batch], exporters: [otlphttp/tempo] }
    logs:    { receivers: [otlp], processors: [memory_limiter, resource, batch], exporters: [clickhouse] }
```
The typical layout: an **agent** (the Collector in agent mode) on every host and a **gateway** — a shared cluster of collectors where tail sampling of traces, filtering and routing happen.

### Fluent Bit
A lightweight log collector written in C: it reads files and the systemd journal, parses, filters and sends to Loki, Elasticsearch/OpenSearch, S3, OpenTelemetry and others. It consumes little memory and CPU — which is why it's often put on hosts where every percent of CPU counts.
```ini
# /etc/fluent-bit/fluent-bit.conf
[SERVICE]
    Flush        1
    storage.path /var/lib/fluent-bit/buffer    # an on-disk buffer: logs aren't lost if the storage is unavailable

[INPUT]
    Name           systemd
    Tag            host.*
    Systemd_Filter _SYSTEMD_UNIT=runtime.service

[FILTER]
    Name   modify
    Match  *
    Add    env prod

[OUTPUT]
    Name   opentelemetry
    Match  *
    Host   otel-gateway.internal
    Port   4318
```
On latency-sensitive hosts the agents (Fluent Bit, the Collector, node_exporter) are restricted to the system cores and a CPU quota in the unit file (`CPUAffinity=0-1`, `CPUQuota=50%`) so that telemetry collection doesn't create tails in the application.

### Grafana Mimir
A Prometheus-compatible metrics store (PromQL, remote_write — the protocol by which Prometheus forwards collected metrics to external storage) for long retention and large volumes. Prometheus or the Collector sends data to it via `remote_write`, the data lives in object storage (S3), and the components scale horizontally: the **distributor** accepts writes, the **ingester** keeps fresh data in memory, the **querier** and **store-gateway** run queries over fresh and old data, the **compactor** compacts blocks in S3. It supports **multi-tenancy** (data of different teams or customers is kept in one system but isolated from each other): the `X-Scope-OrgID` header separates the data of teams or customers. Related Grafana projects: **Loki** (logs), **Tempo** (traces) — together they're called the LGTM stack (Loki, Grafana, Tempo, Mimir).

### ClickHouse for logs and traces
**ClickHouse** is a columnar analytical database. It stores each column separately and compresses heavily, so it aggregates billions of rows in seconds. For telemetry this means you can store **all** events with high cardinality (request, client, host IDs) and compute arbitrary aggregates over them, which Prometheus doesn't allow. Many modern observability platforms are built on ClickHouse (SigNoz, ClickStack, Uptrace), and the OTel Collector has a ClickHouse exporter.
```sql
-- P99 and request count per client over the last hour
SELECT
    tenant_id,
    count() AS requests,
    quantile(0.99)(duration_ms) AS p99_ms
FROM request_log
WHERE timestamp > now() - INTERVAL 1 HOUR
GROUP BY tenant_id
ORDER BY p99_ms DESC
LIMIT 10;
```
`quantile` in ClickHouse is approximate (sample-based); for an exact value there's `quantileExact`, and for merging partial results — `quantileTDigest`.

Schema basics: a table on the **MergeTree** engine (ClickHouse's main table engine: data is written in sorted chunks that are merged in the background), `ORDER BY (service, timestamp)` — the storage order and the primary index filtering works by; `PARTITION BY toDate(timestamp)` and `TTL timestamp + INTERVAL 30 DAY` — automatic deletion of old data.

**What to use when:** metrics for alerts and dashboards — Prometheus/Mimir (cheap, fast, but low cardinality); detailed events and lookups by ID — ClickHouse or Loki; a request's path — traces in Tempo or ClickHouse.

### 🧪 Mini task

**Task.** Write an OpenTelemetry Collector config with an OTLP receiver, the `batch` processor and the `debug` exporter, and check it with `validate`. Then reference an undeclared exporter in the pipeline and look at the error.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > otel.yaml <<'EOF'
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
processors:
  batch: {}
exporters:
  debug: {}
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug]
EOF
validate() { docker run --rm -v "$PWD/otel.yaml:/cfg.yaml" otel/opentelemetry-collector-contrib validate --config=/cfg.yaml 2>&1 | tail -1; }
validate && echo "config is valid"
sed -i.bak 's/exporters: \[debug\]/exporters: [debug, otlp\/tempo]/' otel.yaml
validate
```

Expected output:

```text
config is valid
Error: invalid configuration: service::pipelines::traces: references exporter "otlp/tempo" which is not configured
```

Breakdown: a collector is built from three parts: receivers accept telemetry (OTLP from apps, Prometheus scraping, log files), processors handle it (batching, sampling, dropping sensitive fields, memory limits), exporters send it on (Tempo, Mimir, Loki, ClickHouse). A component works only if it's declared in its section *and* wired into `service.pipelines`. `validate` catches such mistakes before deploy — it goes into CI for collector configs, like `nginx -t` for nginx. `validate` passes silently, so we print `config is valid` ourselves.

</details>

## Methodologies: what to monitor

### The four golden signals (Google SRE)
For any user-facing service:
1. **Latency** — the time it takes to serve requests. Separately for successful and failed ones: fast 500 errors can "improve" the average latency.
2. **Traffic** — the load: requests per second.
3. **Errors** — the share of failed requests (explicit 5xx, as well as wrong responses and responses that are too slow).
4. **Saturation** — how close a resource is to its limit: CPU and memory usage, queue length, connection pool utilisation.

### RED — for services
**R**ate (requests per second), **E**rrors, **D**uration (the latency distribution). A condensed version of the golden signals for microservices. If every service has a RED dashboard, that's already good.

### USE — for resources
For every resource (CPU, memory, disk, network) check **U**tilization, **S**aturation (the queue of waiting work) and **E**rrors. Used for troubleshooting infrastructure (see the Linux topic).

### 🧪 Mini task

**Task.** From a service's access.log for one minute compute the three RED metrics: Rate (requests per second), Errors (share of 5xx) and Duration (p95 response time from the last field of the line).

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
for i in $(seq 1 20); do
  status=200; [ $((i % 10)) -eq 0 ] && status=503
  echo "10.0.0.$i GET /api/orders $status 0.$(printf '%03d' $((i * 15)))"
done > access.log
awk '{n++; if ($4 >= 500) e++} END {printf "rate=%.2f rps errors=%.0f%%\n", n / 60, e / n * 100}' access.log
awk '{print $5}' access.log | sort -n | awk '{a[NR] = $1} END {print "p95=" a[int(NR * 0.95 + 0.999)] "s"}'
```

Expected output:

```text
rate=0.33 rps errors=10%
p95=0.285s
```

Breakdown: RED is three questions for any service: how many requests (Rate), how many failed (Errors), how long they took (Duration). 2 errors out of 20 is 10%. p95 is the value faster than which 95% of requests finished: the 19th of 20 sorted values. For resources (CPU, disks, network) you use USE — Utilization, Saturation, Errors — and Google's four golden signals are RED plus Saturation. In Prometheus the same metrics come from `rate()`, the ratio of errors to the total, and `histogram_quantile()`.

</details>

## SLIs, SLOs, SLAs and the error budget

An **SLI (Service Level Indicator)** is a measurable quality indicator from the user's point of view. Usually expressed as a share of "good" events:
- the share of successful requests: `successful / all`;
- the share of requests faster than a threshold: `requests faster than 300 ms / all`.

An **SLO (Service Level Objective)** is a target for an SLI over a period: "99.9% of requests to checkout succeed over a rolling 30 days".

An **SLA (Service Level Agreement)** is a contractual commitment to a customer with **consequences** (compensation) when breached. It's usually looser than the internal SLO to leave a margin.

### Why not 100%
- 100% is unattainable: the network, providers, hardware and the client itself are unreliable.
- Every extra "nine" costs many times more (redundancy, complexity, slower releases).
- A user can't tell 99.99% from 100%: their own internet connection is less reliable.

### Error budget
**Error budget = 100% − SLO** — the acceptable amount of unreliability:
| SLO | Allowed downtime per 30 days |
|---|---|
| 99% | ~7.2 hours |
| 99.5% | ~3.6 hours |
| 99.9% | ~43 minutes |
| 99.95% | ~22 minutes |
| 99.99% | ~4.3 minutes |

Why a budget: it turns the "ship features faster" versus "stability" argument into a **measurable rule**:
- there's budget left — the team ships features, experiments, takes risks;
- the budget is spent — **a freeze on risky changes**, reliability is the priority until the budget recovers.

### Alerting on the budget burn rate
An alert "error rate above 0.1%" is noisy on short spikes and reacts late to slow degradation. It's better to alert on the **burn rate** — how many times faster than normal the budget is being spent. A burn rate of 1 means the budget runs out exactly at the end of the period; 14.4 — in 2 days instead of 30.

The recommended scheme (multi-window, multi-burn-rate):
- **page the on-call engineer immediately**: burn rate > 14.4 in both a 1-hour and a 5-minute window (2% of the monthly budget is spent in an hour);
- **a ticket for working hours**: burn rate > 1 over a 3-day window (slow degradation).

The short window paired with the long one is needed so that the alert **resolves** quickly once the problem is gone.

### A latency SLO with a hard threshold
For a target like "P99 ≤ 5 ms" the most convenient SLI is a ratio: **what share of requests completed within 5 ms**. Then the SLO reads: "99% of requests faster than 5 ms over 30 days", and the error budget and burn rate apply to it just as to errors.
```promql
# the share of requests faster than 5 ms over 5 minutes
sum(rate(request_duration_seconds_bucket{le="0.005"}[5m]))
/
sum(rate(request_duration_seconds_count[5m]))
```
This works only if **a bucket boundary of exactly 0.005** exists in the histogram. Hence the rules:
- **buckets are chosen for the SLO**, not left at the defaults (the standard Prometheus buckets start at 5 ms, and everything faster falls into one bucket). For a 5 ms target something like `0.0005, 0.001, 0.002, 0.003, 0.004, 0.005, 0.0075, 0.01, 0.025, 0.05, 0.1, 0.5` is reasonable;
- **`histogram_quantile` interpolates linearly within a bucket**: if the bucket spans 5 to 10 ms, "P99 = 7.3 ms" is an estimate, and the real value can be anywhere within those bounds. Accuracy is determined by the width of the buckets around the value of interest;
- **native histograms** (available in newer Prometheus versions and in Mimir) use high-resolution exponential buckets and remove this problem at the cost of client and server support;
- **aggregate buckets, not percentiles**: the cluster-wide P99 is `histogram_quantile` of the sum of all instances' buckets, not the average of the instances' P99 (averaging percentiles is mathematically wrong);
- **where to measure** — it's important to agree: in the application (processing time), at the load balancer, or at the client (including network and queues). For a product that itself sits in the traffic path, you measure the latency it adds: from receiving a request to sending it on, including waiting in a queue;
- **exemplars** — a specific slow request's `trace_id` is attached to a histogram bucket. In Grafana you can open that request's trace straight from a point on the P99 graph and see where the time went.

Alerts on such an SLO use the same burn rate, except that the "bad event" is a request slower than the threshold.

### 🧪 Mini task

**Task.** The availability SLO is 99.9% over 30 days. In the first 10 days there were 2,100,000 requests, 3,150 of them errors. Compute the error budget in minutes of downtime and in requests, the current SLI, how much of the budget has been spent and the burn rate.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
slo, days, elapsed = 0.999, 30, 10
total, errors = 2_100_000, 3_150
print(f"budget: {(1 - slo) * days * 24 * 60:.1f} min of downtime per {days} days")
allowed = total * (1 - slo)
print(f"SLI: {1 - errors / total:.4%}, allowed errors so far: {allowed:.0f}")
print(f"spent: {errors / allowed:.0%} of the budget for these {elapsed} days")
print(f"burn rate: {errors / total / (1 - slo):.1f}x -> 30-day budget gone in {days / (errors / total / (1 - slo)):.0f} days")
EOF
```

Expected output:

```text
budget: 43.2 min of downtime per 30 days
SLI: 99.8500%, allowed errors so far: 2100
spent: 150% of the budget for these 10 days
burn rate: 1.5x -> 30-day budget gone in 20 days
```

Breakdown: 99.9% leaves 0.1% for errors — 43 minutes a month, or 1 error per 1,000 requests. The SLI is now 99.85%, with 1.5 times more errors than allowed, i.e. a burn rate of 1.5: at this pace the monthly budget is gone in 20 days. The error budget turns reliability into a management decision: while budget remains, the team ships features; once it's gone, releases freeze and stability work comes first. An SLA is an external promise to the customer with penalties, and it's set looser than the internal SLO.

</details>

## Alerting: good practices
- **Alert on symptoms, not causes.** "Users are getting errors" (a symptom) — page. "CPU at 90% on one node" (a possible cause) — don't page: if users aren't suffering, there's no problem, and if they are, the symptom alert will fire.
- **Every alert must require action.** If nothing can be done, it's not an alert but a line on a dashboard.
- **Every alert has a runbook**: what it means, how to check, how to fix.
- **Severity levels**: critical (wake people at night) — only when users are suffering right now; warning — to a work chat or a ticket.
- **Fighting alert fatigue:** `for:` to filter out spikes, grouping in Alertmanager, regular reviews of noisy alerts (delete or fix them). If the on-call engineer is woken 10 times a night by false alerts, they'll stop reacting to the real one.
- **Monitoring from outside**: blackbox checks (synthetic requests) show the problem the way the user sees it, even if the whole internal monitoring system went down together with the cluster.

### 🧪 Mini task

**Task.** Write an alert rule "5xx share above 5% for 5 minutes" and a `promtool` unit test for it: at 10% errors the alert must not fire after 3 minutes and must fire after 10. Run the test using the Prometheus image.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
cd "$(mktemp -d)"
cat > rules.yml <<'EOF'
groups:
  - name: api
    rules:
      - alert: HighErrorRate
        expr: sum(rate(http_requests_total{code=~"5.."}[1m])) / sum(rate(http_requests_total[1m])) > 0.05
        for: 5m
        labels:
          severity: page
EOF
cat > tests.yml <<'EOF'
rule_files: [rules.yml]
evaluation_interval: 1m
tests:
  - interval: 1m
    input_series:
      - series: 'http_requests_total{code="200"}'
        values: '0+90x15'
      - series: 'http_requests_total{code="500"}'
        values: '0+10x15'
    alert_rule_test:
      - eval_time: 3m
        alertname: HighErrorRate
        exp_alerts: []
      - eval_time: 10m
        alertname: HighErrorRate
        exp_alerts:
          - exp_labels: {severity: page}
EOF
docker run --rm -v "$PWD:/w" -w /w --entrypoint promtool prom/prometheus test rules tests.yml
```

Expected output:

```text
Unit Testing:  tests.yml
  SUCCESS
```

Breakdown: `0+90x15` is a series growing by 90 every minute, so the error share is exactly 10%. At minute 3 the condition holds, but `for: 5m` hasn't elapsed: the alert is pending and not sent. That's how `for` filters out short spikes. At minute 10 it's firing. A good alert wakes a human only when users suffer and action is needed: on symptoms (errors, latency, SLO burn rate), not causes like "CPU 80%". Every alert needs a runbook link. Rule tests in CI catch PromQL typos before an alert silently stops working.

</details>

## Incident management

### During an incident
- **The top priority is restoring service**, not finding the root cause: roll back the last deployment, shift traffic, turn the feature off with a flag, add resources. The investigation comes later.
- **Roles** (for major incidents):
  - **Incident Commander** — coordinates, makes decisions, watches the time;
  - **Communications** — reports status to the business and users (a status page — a public page like status.example.com with the service's current state);
  - **Operations/experts** — do the actual fixing.
- A separate incident channel; the timeline of actions is recorded as you go.
- Severity levels (SEV1–SEV4: SEV1 — the core service is down for all users, SEV4 — a minor degradation without noticeable impact) define whom to involve and how fast.

### The postmortem (post-incident review)
A review after an incident. **Blameless** — without looking for someone to blame: if one person could take down production with a single command, the problem is in the system (there was no safeguard, review or check), not in the person. Otherwise people start hiding mistakes.

Contents:
- **the timeline**: when it started, when it was noticed, which actions were taken and when service was restored;
- **the impact**: how many users, for how long, how much error budget was spent;
- **the root cause and contributing factors** (the "5 whys" method: keep asking "why" until you reach a systemic cause);
- **what worked well and what got in the way** (for example, the alert came 20 minutes late, the runbook was outdated);
- **action items** — specific tasks with owners and deadlines: add an alert, automate a check, change a process.

### Metrics
- **MTTD** (mean time to detect) — how quickly we detect; **MTTA** (acknowledge) — how quickly we react; **MTTR** (restore/recover) — how quickly we restore; **MTBF** (between failures) — how often things break.
- **DORA metrics** (delivery performance, from the DevOps Research and Assessment studies):
  - **deployment frequency** — how often we deploy;
  - **lead time for changes** — the time from commit to production;
  - **change failure rate** — the share of deployments that caused problems;
  - **time to restore service** — the time to recover after a failure.

  The best teams deploy often and in small portions, and at the same time have a **lower** change failure rate.

### Toil
**Toil** is manual, repetitive, automatable work without long-term value that grows together with the service: manual deployments, restarts, granting access, cleaning disks. According to the Google SRE model, toil should take less than 50% of the time, and the rest should be engineering work that eliminates that toil (automation, self-service).

### On-call
On-call duty: a rotation, clear escalations, a limit on the load (if there are more than a couple of incidents per shift, the system needs fixing rather than tolerating), compensation, a handover with context.

### 🧪 Mini task

**Task.** From an incident timeline (degradation start, alert fired, on-call acknowledged, fix applied, recovery) compute the time to detect, to acknowledge, to recover and the total user impact duration.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
from datetime import datetime
t = {name: datetime.fromisoformat(f"2026-10-25T{hhmm}") for name, hhmm in [
    ("impact_start", "14:02"), ("alert_fired", "14:09"), ("acknowledged", "14:12"),
    ("fix_applied", "14:31"), ("recovered", "14:34")]}
minutes = lambda a, b: int((t[b] - t[a]).total_seconds() // 60)
print("time to detect:     ", minutes("impact_start", "alert_fired"), "min")
print("time to acknowledge:", minutes("alert_fired", "acknowledged"), "min")
print("time to recover:    ", minutes("alert_fired", "recovered"), "min")
print("user impact:        ", minutes("impact_start", "recovered"), "min")
EOF
```

Expected output:

```text
time to detect:      7 min
time to acknowledge: 3 min
time to recover:     25 min
user impact:         32 min
```

Breakdown: users suffered for 32 minutes, and for 7 of them nobody knew. That's a postmortem question: why did the alert fire so late (too long a `for`, an alert on a cause rather than a symptom)? That's why the timeline is a mandatory part of a postmortem: without it you can't tell which stage to improve. Another 25 minutes went to diagnosis and the fix — runbooks, clear dashboards and fast rollback help there. Averages of these intervals across incidents are MTTD, MTTA and MTTR.

</details>

## Reliability patterns
- **Timeouts** on every network call. Without a timeout, a hung dependency holds threads and connections until the whole service goes down.
- **Retries with exponential backoff and jitter.** Retry only idempotent operations. Limit the number of retries: during a mass failure, retries from all clients multiply the load and finish off the recovering service (a **retry storm**). Jitter (random spread) keeps clients from retrying in sync.
- **Circuit breaker** — after a series of errors, stop calling the dependency for a while and immediately return an error or a fallback response. It gives the dependency time to recover and doesn't waste resources on requests that are bound to fail.
- **Bulkhead** — resource isolation: separate thread and connection pools for different dependencies, so that a problem with one doesn't exhaust the resources for all.
- **Rate limiting and load shedding** — limit incoming load and drop part of the requests under overload so as to serve the rest rather than going down entirely.
- **Graceful degradation** — when a secondary feature fails (recommendations, reviews), the main scenario (making a purchase) keeps working.
- **Idempotency** — processing the same request or message again doesn't create duplicates (an idempotency key in payments).
- **Redundancy** — several replicas across several AZs, with no single point of failure.
- **Chaos engineering** — deliberately inject failures (kill a pod, add network latency, take down a zone) and check that the system holds up: Chaos Mesh, Litmus (chaos experiment tools for Kubernetes), AWS Fault Injection Service (the same for AWS resources: stop instances, "drop" a zone).
- **Capacity planning and load testing** (k6, Gatling — load generators that describe user scenarios in code): know how much the system can take and when scaling will be needed.

### 🧪 Mini task

**Task.** Model two patterns. First, exponential retry backoff capped at 1 second. Second, a circuit breaker that opens after 3 consecutive failures, rejects calls immediately, and after 5 seconds lets one trial call through.

<details class="qa"><summary>Solution: command and expected output</summary>

Paste the whole block into a terminal:

```bash
python3 - <<'EOF'
print("backoff:", [min(0.1 * 2 ** n, 1.0) for n in range(6)])

class Breaker:
    def __init__(self, threshold=3, cooldown=5):
        self.failures, self.opened_at, self.threshold, self.cooldown = 0, None, threshold, cooldown
    def call(self, now, ok):
        if self.opened_at is not None:
            if now - self.opened_at < self.cooldown:
                return "rejected (open)"
            self.opened_at = None
            if not ok:
                self.opened_at = now
                return "trial failed -> open again"
            self.failures = 0
            return "trial ok -> closed"
        if ok:
            self.failures = 0
            return "ok"
        self.failures += 1
        if self.failures >= self.threshold:
            self.opened_at = now
            return "failed -> OPEN"
        return "failed"

b = Breaker()
for now, ok in [(0, False), (1, False), (2, False), (3, True), (4, True), (8, True), (9, True)]:
    print(f"t={now}s", b.call(now, ok))
EOF
```

Expected output:

```text
backoff: [0.1, 0.2, 0.4, 0.8, 1.0, 1.0]
t=0s failed
t=1s failed
t=2s failed -> OPEN
t=3s rejected (open)
t=4s rejected (open)
t=8s trial ok -> closed
t=9s ok
```

Breakdown: exponential backoff gives an overloaded service time to recover, and the cap keeps the pause from growing forever. Production adds random jitter so thousands of clients don't retry in sync. After three failures the circuit breaker stops calling the sick service: calls at seconds 3 and 4 are rejected instantly, without waiting for timeouts, and the service isn't hammered with retries. After 5 seconds (half-open) one trial call succeeded and the circuit closed. Alongside them work timeouts, bulkheads (separate resource pools), graceful degradation and rate limiting.

</details>

## Interview questions

**How does monitoring differ from observability?**
Monitoring answers questions known in advance: dashboards and alerts on chosen metrics. Observability lets you investigate new, unforeseen problems using rich telemetry (metrics, structured logs, traces with attributes).

**What telemetry signals are there, and what is each for?**
Metrics — cheap aggregates for alerts and trends. Logs — the details of individual events. Traces — a request's path through the services and where time is lost. They're linked via the trace_id.

**Why use `rate()` rather than the counter's value?**
A counter grows from startup and resets on restart: its absolute value says nothing about the current state. `rate()` gives the current per-second rate and handles resets correctly.

**Why look at p99 rather than the average latency?**
The average hides the tail of the distribution: 99% of fast requests mask 1% of very slow ones. With millions of requests that 1% is tens of thousands of unhappy users, and often they're the most active customers with the biggest baskets.

**Prometheus ate all its memory. What's the cause, and what do you do?**
Almost always a cardinality explosion — a label with an unbounded number of values (user ID, full URL). To find it: `topk(10, count by (__name__)({__name__=~".+"}))` and the TSDB status page (`/api/v1/status/tsdb`). To fix it: remove the label in the application or drop it via `metric_relabel_configs` (Prometheus rules that rewrite or discard metrics and labels right after the scrape, before writing to the database), normalise paths.

**What are an SLI, an SLO, an SLA and the error budget?**
An SLI is a measurable indicator (the share of successful requests). An SLO is a target for it (99.9% over 30 days). An SLA is a contractual commitment with consequences. Error budget = 100% − SLO — the acceptable unreliability; while there's budget left you can take risks with releases, and once it's spent reliability takes priority.

**How do you choose an SLO?**
From the user experience (what the user considers "working") and historical data, agreed with the business. Not 100%. Start with an achievable value and tighten it.

**Why is an alert on "error rate > 1%" worse than one on the burn rate?**
A threshold is noisy on short spikes and notices slow degradation late. The burn rate shows how fast the budget is being spent: a fast burn — an urgent alert, a slow one — a ticket. Dual windows (long and short) give both reliable firing and quick resolution of the alert.

**How do you fight alert fatigue?**
Alert on symptoms, not causes; every alert has an action and a runbook; `for:` to filter out spikes; grouping and inhibition in Alertmanager; regular reviews and removal of noisy alerts; critical only for what requires a reaction at night.

**What do you do first during an incident?**
Restore service (roll back, shift traffic, turn off a feature, scale), communicate the status, keep a timeline. Look for the root cause after recovery.

**What is a blameless postmortem and why is it needed?**
A review of an incident without looking for someone to blame: we look for systemic causes (why the system allowed the mistake to happen and why it wasn't detected earlier) and come up with specific actions with owners. Without blame, people openly describe what happened, and the review really improves the system.

**What is toil?**
Manual, repetitive, automatable work without long-term value that grows together with the system. The goal of SRE is to limit its share and automate it.

**What are the DORA metrics?**
Deployment frequency, lead time from commit to production, change failure rate, time to restore. They measure delivery performance; in the best teams high speed is combined with high stability.

**How do you protect a service from a dependency failing?**
Timeouts, limited retries with backoff and jitter, a circuit breaker, bulkheads (isolated pools), caching and fallback responses (graceful degradation), asynchronous processing via queues.

**What is a retry storm and how do you avoid it?**
During a failure all clients retry their requests at the same time, multiplying the load and preventing the service from recovering. Protection: limiting the number of retries, exponential backoff with jitter, a circuit breaker, a retry budget (no more than a certain share of normal traffic).

**Tell me about an incident you resolved.**
Prepare 2–3 stories using the STAR method (see the "Cases" topic): what happened, how it was detected, how service was restored, what the root cause was, what was changed afterwards.

**How do you define an SLO if the target is P99 no more than 5 ms?**
The SLI is the share of requests faster than 5 ms: the ratio of the `rate` of the `le="0.005"` bucket to the `rate` of the total counter. The SLO is, for example, 99% of such requests over 30 days, followed by the error budget and burn rate alerts. The histogram must have a boundary of exactly 5 ms and dense buckets around it; agree on where exactly latency is measured.

**Why can't you average the P99 of different instances?**
Percentiles don't add up or average: the cluster's P99 depends on the whole distribution, not on the P99 of its parts. The right way is to sum the histogram buckets of all instances and compute `histogram_quantile` of the sum. That's why histograms rather than summaries are used for aggregation.

**Why do you need the OpenTelemetry Collector if the application can send data straight to storage?**
It decouples the application from the backend: buffering and retries when the backend is unavailable, batching, enrichment with attributes, filtering, trace sampling, sending to several systems at once and switching backends without changing the application's code. The agent-on-host plus gateway layout enables tail sampling and centralised rules.

**When do you use ClickHouse for telemetry, and when Prometheus?**
Prometheus and Mimir — for metrics with a limited set of labels: alerts, dashboards, SLOs. ClickHouse — for detailed events with high cardinality (every request with the client, host and version IDs), arbitrary analytics and long-term storage of logs and traces. Both are often used.
