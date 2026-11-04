# RabbitMQ

A thorough RabbitMQ primer: the "exchange → binding → queue" model, exchange types and routing, queue types (classic, quorum, streams), acknowledgements and prefetch, error handling via dead letter, clustering and high availability, linking data centres, resources and performance, security, monitoring, running it in Kubernetes and the cloud (Amazon MQ) and a comparison with Kafka, SQS and SNS. Each topic: what it is, why, how it works, an example with explanations and typical mistakes. At the end — interview questions with answers.

How to read: bring up a broker from the first section and run the examples as you go. The output is shown in comments after `# →`. The detailed comparison with Kafka is in the "RabbitMQ and other brokers" section and in the [Kafka](#19-kafka.md/s14) topic.

## How to learn without a course

RabbitMQ runs as a single container, together with the management web UI:
```bash
docker run -d --name rabbit -p 5672:5672 -p 15672:15672 rabbitmq:4-management
# 5672  — AMQP: applications connect here
# 15672 — the web UI and HTTP API: http://localhost:15672, login and password guest / guest (localhost only)
docker exec rabbit rabbitmq-diagnostics check_running
# → Checking if RabbitMQ is running on node rabbit@...
# → RabbitMQ on node rabbit@... is fully booted and running
```
The web UI is handy for looking at queues, exchanges and the links between them, and for publishing test messages by hand. For code — Python and the `pika` library: `pip install pika`.

For experiments with a cluster and failures you need 3 nodes: docker compose with three containers and a shared Erlang cookie (the secret by which nodes recognise "their own") or a kind cluster with the RabbitMQ Cluster Operator (see the "RabbitMQ in Kubernetes and the cloud" section).

The plan: a queue and a message → exchanges and routing → acknowledgements and prefetch → dead letter and retries → a quorum queue and a node failure → streams → monitoring.

## What RabbitMQ is and when you need it

**RabbitMQ** is a message broker: applications send messages to the broker, the broker distributes them into **queues** by routing rules, and consumers take messages from the queues and **acknowledge** processing, after which the message is deleted. It's written in Erlang (a language and platform for fault-tolerant systems from the telecom industry). The main protocol is **AMQP 0-9-1**, since version 4.0 also built-in AMQP 1.0, plus MQTT (for IoT devices) and STOMP via plugins.

The "smart broker, simple client" model: routing, retries, TTL, priorities and dead letter are done by the broker. Kafka is the other way round: the broker just stores the log, and the clients hold the logic.

When RabbitMQ is a good choice:
- **task queues**: sending emails, generating reports, processing uploaded files — each task must be processed by one worker and acknowledged;
- **complex routing**: the event `order.created.eu` must reach the European delivery service, analytics and audit by different rules;
- **priorities and delayed delivery**, limiting the lifetime of messages;
- **request-reply** (RPC over queues) and integrating heterogeneous systems via standard protocols;
- **IoT** via MQTT.

When Kafka is better: you need an event log with retention and re-reading of history, many independent consumers of the same data, hundreds of thousands and millions of messages per second, CDC and stream analytics. In detail — the "RabbitMQ and other brokers" section.

## Core concepts

- **Producer** — sends messages. Important: in RabbitMQ a producer writes **not to a queue but to an exchange**.
- **Exchange** — a "sorting office": it receives a message and decides by rules which queues to put it into. It doesn't store messages itself.
- **Binding** — a rule linking an exchange to a queue ("put messages with routing key `order.*` from exchange `events` into queue `billing`").
- **Routing key** — an address string the producer specifies when sending; the exchange compares it with the bindings.
- **Queue** — stores messages until they're taken and acknowledged.
- **Consumer** — subscribes to a queue and receives messages.
- **Connection and channel** — an application opens one TCP connection and inside it lightweight logical channels for different streams of work. Opening a connection per message is expensive; the rule is a long-lived connection and channels.
- **Virtual host (vhost)** — an isolated space inside the broker with its own exchanges, queues and permissions: for example, a separate vhost per environment or team.

```text
producer ──(routing key: order.created)──▶ exchange "events" (topic)
                                              │ binding "order.*"    ──▶ queue "billing"   ──▶ consumer billing
                                              │ binding "#"          ──▶ queue "audit"     ──▶ consumer audit
                                              └ binding "payment.*"  ──▶ queue "payments"  (won't get there)
```

## Exchanges and routing

Four exchange types:
| Type | How it picks queues | Example |
|---|---|---|
| **direct** | the routing key must exactly match the binding key | `pdf` → the PDF generation queue, `email` → the email queue |
| **topic** | a pattern over dot-separated words: `*` — exactly one word, `#` — zero or more words | `order.*.eu` catches `order.created.eu`; `order.#` catches everything about orders |
| **fanout** | to all bound queues, the routing key is ignored | the "user deleted" event — each service gets its own copy |
| **headers** | by message headers (`x-match: all` or `any`) | routing by several attributes at once; rarely used |

The **default exchange** is a nameless direct exchange to which every queue is automatically bound by its name. So "sending straight to a queue" is really sending to the default exchange with routing key = the queue's name.

What happens to a message that didn't get into any queue (no matching binding)? By default **it's silently lost**. The protection: the `mandatory` flag on publish (the broker returns the message to the producer) or an **alternate exchange** (all unroutable messages go to a fallback exchange and queue where they're visible).

A Python example: a topic exchange and two queues with different patterns.
```python
import pika

conn = pika.BlockingConnection(pika.ConnectionParameters("localhost"))
ch = conn.channel()

ch.exchange_declare(exchange="events", exchange_type="topic", durable=True)
for queue, pattern in (("billing", "order.*"), ("audit", "#")):
    ch.queue_declare(queue=queue, durable=True, arguments={"x-queue-type": "quorum"})
    ch.queue_bind(queue=queue, exchange="events", routing_key=pattern)

ch.confirm_delivery()                     # publisher confirms: basic_publish waits for the broker's confirmation
for key in ("order.created", "order.paid", "payment.refunded"):
    ch.basic_publish(exchange="events", routing_key=key, body=key.encode(),
                     properties=pika.BasicProperties(delivery_mode=2),   # persistent: to disk
                     mandatory=True)      # a message without a matching queue is returned rather than lost
conn.close()
```
```bash
docker exec rabbit rabbitmqctl list_queues name type messages
# → name     type    messages
# → audit    quorum  3          — all three messages got here (pattern #)
# → billing  quorum  2          — only order.created and order.paid
docker exec rabbit rabbitmqctl list_bindings source_name destination_name routing_key
```
The message counters in `list_queues` and the web UI are updated every few seconds: right after publishing they may show zeros.

## Queues: classic, quorum, streams

**Queue properties:**
- **durable** — the queue survives a broker restart (its definition is saved). The messages themselves survive a restart only if they're also **persistent** (`delivery_mode=2`);
- **exclusive** — the queue belongs to one connection and is deleted with it (temporary queues for RPC replies);
- **auto-delete** — deleted when the last consumer unsubscribes;
- limits: `x-max-length` or `x-max-length-bytes` (what to do on overflow — `x-overflow`: drop the oldest or reject new ones), `x-message-ttl` (the lifetime of messages), `x-expires` (delete an unused queue).

**Three queue types:**
| | Classic | Quorum | Stream |
|---|---|---|---|
| Storage | on one node | replicated to several nodes using the Raft protocol | a replicated log on disk |
| After reading | deleted | deleted | **stays** until retention, can be read from any position |
| Node failure | the queue is unavailable until the node returns | a new leader is elected from the replicas, work continues | as with quorum |
| Specifics | fast, for temporary and non-critical data | **the standard for important data**, a redelivery limit (poison message handling), priorities (2 levels) | huge volumes, many consumers of the same data, re-reading — "Kafka inside RabbitMQ" |

**Classic mirrored queues** (the old way of HA by mirroring classic queues) **were removed in RabbitMQ 4.0**. In an interview, the right answer about HA in RabbitMQ is quorum queues.

A quorum queue is declared with an argument:
```python
ch.queue_declare(queue="orders", durable=True, arguments={
    "x-queue-type": "quorum",
    "x-delivery-limit": 5,                # after 5 failed deliveries — to dead letter instead of circling forever
    "x-dead-letter-exchange": "dlx",
})
```

**Streams** are for "like Kafka" scenarios, but inside RabbitMQ: a log that many consumers read independently, specifying where to start (`first`, `last`, `next`, an offset or a time). **Super streams** are a stream split into partitions for parallelism (the analogue of a Kafka topic with partitions). They're fastest via a separate stream protocol (port 5552, the `rabbitmq_stream` plugin), but they can also be read and written via ordinary AMQP.

## Acknowledgements and prefetch

**Consumer acknowledgements** — how the broker learns that a message has been processed:
- **manual ack** — after processing the consumer sends `basic.ack`, and only then is the message deleted. If the consumer crashed or closed the channel without acknowledging, the message **goes back to the queue** and goes to someone else (at-least-once);
- `basic.nack` / `basic.reject` — "couldn't process it": with `requeue=true` return it to the queue, with `requeue=false` — drop it or send it to dead letter;
- **auto ack** (`auto_ack=True`) — the message counts as acknowledged at the moment it's sent to the consumer. Fast, but a consumer crash = the loss of the messages it has already received (at-most-once).

**Prefetch** (`basic.qos`, prefetch count) — how many unacknowledged messages the broker may give one consumer at a time. This is the main performance and fairness knob:
- no prefetch set (unlimited) — the broker dumps all the queue's messages on the first consumer, it takes up memory, and the others sit idle;
- prefetch = 1 — fair (a slow worker doesn't hoard tasks), but slow because of waiting for each ack over the network;
- usually 10–300: less for long tasks, more for fast small messages.

```python
import pika

def handle(ch, method, properties, body):
    try:
        process(body)                                      # processing must be idempotent
        ch.basic_ack(delivery_tag=method.delivery_tag)
    except TemporaryError:
        ch.basic_nack(delivery_tag=method.delivery_tag, requeue=True)    # try again
    except Exception:
        ch.basic_nack(delivery_tag=method.delivery_tag, requeue=False)   # to dead letter

conn = pika.BlockingConnection(pika.ConnectionParameters("localhost"))
ch = conn.channel()
ch.basic_qos(prefetch_count=20)                            # no more than 20 unacknowledged per consumer
ch.basic_consume(queue="billing", on_message_callback=handle)
ch.start_consuming()
```

**Publisher confirms** — acknowledgements on the producer side: the broker reports that the message has been accepted and (for persistent and quorum) written to disk and the replicas. Without them the producer doesn't know whether a message arrived if the broker crashed at the moment of sending. Together: publisher confirms + persistent messages + a quorum queue + manual ack = messages aren't lost at any step. Duplicates are still possible (a resend after a lost confirmation, a redelivery after a consumer crash), so handlers must be **idempotent** — just as in Kafka.

**Message order** is preserved within a queue, but only while it's read by one consumer and there are no redeliveries: several competing consumers process messages in parallel, and `nack` with `requeue` returns a message to the queue. If order matters — a **single active consumer** (`x-single-active-consumer: true`: only one consumer reads, the others are hot standbys) or splitting by key into several queues (the consistent hash exchange).

## Errors and retries: dead letter, TTL, delayed delivery

A **poison message** is a message the handler can never process (a broken format, a bug). With `nack(requeue=true)` it returns to the head of the queue and keeps coming back again and again, loading the CPU and blocking the rest.

A **dead letter exchange (DLX)** is an exchange to which the broker moves a message if it was:
- rejected (`nack`/`reject` with `requeue=false`);
- expired by TTL;
- pushed out of an overflowing queue;
- in a quorum queue, over the `x-delivery-limit` (20 by default since 4.0).

```python
ch.exchange_declare(exchange="dlx", exchange_type="fanout", durable=True)
ch.queue_declare(queue="orders.dead", durable=True, arguments={"x-queue-type": "quorum"})
ch.queue_bind(queue="orders.dead", exchange="dlx")
# on the main queue: "x-dead-letter-exchange": "dlx" (see the quorum queue example above)
```
In the `x-death` header the broker records where and why the message went to dead letter. The `orders.dead` queue is monitored (an alert on growth) and sorted out by hand or by a script: fixed the bug → moved the messages back (a shovel or the web UI).

**A delayed retry** (retry with backoff) without plugins — via TTL and DLX: a failed message is sent to the `orders.retry` queue with `x-message-ttl: 30000` and an `x-dead-letter-exchange` pointing back to the main exchange. After 30 seconds the message "dies" and returns to work. Several queues with different TTLs (10 s, 1 min, 10 min) give an increasing delay.

**Delayed delivery** ("send in an hour") — either the same TTL + DLX scheme or the `rabbitmq_delayed_message_exchange` plugin (an exchange with an `x-delay` header). The plugin is third-party and stores delayed messages on one node without replication, so it's avoided for critical scenarios.

**Priorities**: a classic queue with `x-max-priority` (up to 255 levels, practically up to 10), a quorum queue — two levels (high and normal, since 4.0).

## Clustering and high availability

A RabbitMQ **cluster** is several nodes (usually **3 or 5**, an odd number for quorum) that share metadata: users, vhosts, exchanges, queue definitions. A client can connect to any node and work with any queue — the node forwards requests to where the queue lives.

What's important to understand:
- **queue data isn't distributed by itself**: a classic queue lives on one node, a quorum queue on several replicas (on 3 nodes of the cluster by default), and it has a leader;
- when a node fails, a quorum queue elects a new leader from the replicas as long as a **majority** of the replicas is alive (2 of 3, 3 of 5). If fewer than half the replicas are alive, the queue is unavailable, but the data isn't lost;
- a **network partition** (the nodes are alive but can't see each other) is a dangerous situation for the old classic mirrored queues; quorum queues solve this with the Raft protocol — writes are possible only on the majority side;
- in newer versions the cluster metadata is stored in **Khepri** (a Raft-based store) instead of the old Mnesia, which also improves behaviour during partitions;
- nodes communicate on port 25672 and find each other via port 4369 (epmd — the Erlang name service); both must be open between the nodes and closed from outside;
- a cluster **isn't stretched across data centres or regions**: Raft and the Erlang cluster are sensitive to latency. Between sites — federation and shovel (the next section). Across the AZs of one region — fine.

**Upgrading without downtime**: nodes are upgraded one at a time. Before stopping a node — `rabbitmq-upgrade drain` (the node stops being the leader of quorum queues and stops accepting clients), and a check with `rabbitmq-queues check_if_node_is_quorum_critical` (whether any quorum queue would lose quorum if this node is stopped).

```bash
docker exec rabbit rabbitmq-diagnostics cluster_status
docker exec rabbit rabbitmq-queues quorum_status orders        # the queue's leader and replicas
docker exec rabbit rabbitmq-queues check_if_node_is_quorum_critical
```

Clients must be able to **reconnect**: a list of all the nodes' addresses (or a load balancer in front of them) and automatic recovery of the connection and subscriptions. Most libraries can do this, but it has to be enabled and tested explicitly.

## Between data centres: federation and shovel

- **Federation** — an exchange or queue on one broker receives messages from another broker over ordinary AMQP. It suits linking sites with high latency and an unreliable network: each site is its own independent cluster, and federation carries over the needed messages. An example: regional clusters forward events to a central one.
- **Shovel** — a simple "mover": it reads from a source queue and publishes to a destination exchange (on the same or another broker). It's used for migrating between clusters, sorting out dead letter queues, moving data when upgrading to a new version via a new cluster (blue-green).

Both work asynchronously, so during DR some messages not yet moved to the second site may be lost or duplicated — just as with Kafka replication via MirrorMaker.

## Performance and resources

**Protecting the broker from overload:**
- **memory alarm** — when RabbitMQ uses more than `vm_memory_high_watermark` (60% of memory by default), it **blocks all producers** until memory is freed. Consumers keep working;
- **disk alarm** — when free space drops below `disk_free_limit`, producers are blocked too. The default is very small (about 50 MB); in prod it's raised — for example, to the node's memory size;
- **flow control** — the broker slows down connections that are too fast if its internal processes can't keep up.

A real-life symptom: "the producers froze and send nothing, there are no errors" — almost always a memory or disk alarm went off because the consumers fell behind and the queues grew to millions of messages.
```bash
docker exec rabbit rabbitmq-diagnostics alarms
docker exec rabbit rabbitmq-diagnostics memory_breakdown      # what the memory went to
```

**Long queues are bad**. RabbitMQ works best when queues are almost empty: messages quickly go to consumers. Millions of messages in a queue mean higher memory, a slow restart, long replica synchronisation. If you need to store a large volume, that's a job for streams or Kafka.

What else affects performance:
- **connections and channels**: long-lived connections, separate connections for publishing and consuming (then blocking the producers won't stop the consumers on the same connection), no "a connection per message" (connection churn — frequently opening and closing connections loads the broker);
- **prefetch** — see above;
- **message size**: large files aren't sent through the broker; they go to S3, and a reference is sent;
- **the number of queues**: tens of thousands of queues load the metadata and memory;
- **persistent + quorum** is slower than transient + classic, but that's the price of durability.

A load test — the official **PerfTest** tool:
```bash
docker run --rm --network container:rabbit pivotalrabbitmq/perf-test:latest \
  --uri amqp://guest:guest@localhost:5672 --producers 2 --consumers 4 --queue perf --quorum-queue --time 60
# every few seconds — messages sent and received per second, delivery latency
```

## Security

- **The `guest` user** can log in only from localhost — leave it that way; in prod create your own users and delete guest.
- **Permissions per vhost**: on each vhost a user is given three regular expressions — what they may configure (configure: create and delete queues and exchanges), where they may write (write) and where they may read from (read):
```bash
docker exec rabbit rabbitmqctl add_vhost billing
docker exec rabbit rabbitmqctl add_user billing-svc 'S3cr3t-pass'
docker exec rabbit rabbitmqctl set_permissions -p billing billing-svc '^billing\..*' '^billing\..*|events' '^billing\..*'
docker exec rabbit rabbitmqctl delete_user guest
```
- **TLS** for clients (port 5671) and between nodes; mandatory in the cloud.
- **Authentication** via external systems: OAuth 2 (tokens from Keycloak, Entra ID), LDAP.
- **Network**: AMQP ports only from the applications' networks, the web UI port not on the internet, inter-node ports only between the nodes.
- **The Erlang cookie** is the shared secret of the cluster's nodes: whoever knows it can connect to the cluster as a node and get full control. Store it as a secret.

## Monitoring

What to watch:
| Metric | Why it matters | Alert |
|---|---|---|
| **Queue depth** (messages ready) | consumers can't keep up | grows for N minutes in a row |
| **Unacknowledged** (messages unacknowledged) | consumers took messages but hung | grows with a steady incoming flow |
| Publish, deliver, ack rates | the ratio of incoming to processed | the incoming flow is steadily higher than processing |
| The number of consumers on a queue | the workers went down | 0 consumers on a working queue |
| **Memory and disk alarms** | the producers are blocked | any alarm |
| Messages in dead letter queues | processing errors | growth |
| Connection restarts (connection churn) | badly written clients load the broker | a sharp rise in connection opens |
| The state of quorum replicas | a queue without quorum is unavailable | fewer replicas than a majority |

The built-in `rabbitmq_prometheus` plugin serves metrics on port 15692 (`/metrics`), and there are official Grafana dashboards for it. The management UI is convenient for people, but you shouldn't build monitoring on its API — with thousands of queues its requests are heavy.
```bash
docker exec rabbit rabbitmq-plugins enable rabbitmq_prometheus   # usually already enabled in the management image
docker exec rabbit rabbitmqctl list_queues name messages_ready messages_unacknowledged consumers
```

## RabbitMQ in Kubernetes and the cloud

- **RabbitMQ Cluster Kubernetes Operator** — the official operator: the cluster is described by a `RabbitmqCluster` resource, and the operator creates a StatefulSet, services and secrets and does upgrades one node at a time. The **Messaging Topology Operator** lets you describe queues, exchanges, bindings, users and permissions as resources in git too.
```yaml
apiVersion: rabbitmq.com/v1beta1
kind: RabbitmqCluster
metadata:
  name: events
  namespace: messaging
spec:
  replicas: 3
  resources:
    requests: { cpu: "1", memory: 4Gi }
    limits: { memory: 4Gi }                 # the memory high watermark is computed from the container limit
  persistence:
    storageClassName: gp3
    storage: 50Gi
  rabbitmq:
    additionalConfig: |
      disk_free_limit.absolute = 4GB
      default_queue_type = quorum
---
apiVersion: rabbitmq.com/v1beta1
kind: Queue
metadata:
  name: orders
  namespace: messaging
spec:
  name: orders
  type: quorum
  durable: true
  rabbitmqClusterReference:
    name: events
```
- **Amazon MQ for RabbitMQ** — managed RabbitMQ in AWS: a single node (for dev) or a cluster of 3 nodes in different AZs, patches and upgrades in maintenance windows, metrics in CloudWatch. Limitations: not all plugins and settings are available, new versions arrive with a delay, no access to the nodes. Amazon MQ also offers ActiveMQ for old JMS-based systems.
- **CloudAMQP** — a SaaS provider of RabbitMQ in different clouds.

How to choose: Amazon MQ — if everything is in AWS, the team is small and standard RabbitMQ features are enough. The operator in Kubernetes — if the platform is already on Kubernetes, you need special plugins and settings or the same installation in different clouds and on-prem. If all you need from RabbitMQ is simple queues and pub/sub in AWS — perhaps you don't need it at all, and SQS and SNS without a broker are enough.

## RabbitMQ and other brokers: Kafka, SQS, SNS

The main difference is in the model:
- **RabbitMQ is queues and routing**: the broker distributes messages into queues by rules, the consumer acknowledges each one, and acknowledged messages are deleted. "A smart broker, a simple client".
- **Kafka is a log**: messages are kept for a set time, consumers move their own offset and can re-read history, many independent groups read the same thing. "A simple broker, a smart client". In detail — in the [Kafka](#19-kafka.md/s1) topic.

| | RabbitMQ | Kafka | Amazon SQS | Amazon SNS |
|---|---|---|---|---|
| Model | queues + exchanges | a log with partitions | a queue | pub/sub broadcast |
| After processing | deleted after ack (except streams) | stays until retention | deleted | not stored |
| Routing | direct, topic by patterns, fanout, headers | by topic and key | none | subscription filters |
| Acknowledgement | per message (ack/nack) | committing the offset (position) | deleting the message after processing | — |
| Retries and errors | requeue, DLX, delivery limit | your own retry topic and DLT | visibility timeout, DLQ via a redrive policy | delivery retries, DLQ |
| Priorities, delayed delivery | yes | no | a delay of up to 15 minutes | no |
| Order | within a queue (one consumer or single active consumer) | within a partition | FIFO — within a message group | FIFO topics |
| Volumes | tens to hundreds of thousands of messages/s | millions of messages/s | practically unlimited | high |
| Protocols | AMQP 0-9-1 and 1.0, MQTT, STOMP | its own binary protocol | the AWS HTTP API | the AWS HTTP API |
| Operations | medium (or Amazon MQ) | heavy (or MSK) | none | none |

**Analogies that help in an interview:**
- a RabbitMQ queue with several consumers ≈ an SQS queue with several workers ≈ a Kafka consumer group: the work is shared between instances;
- a fanout exchange with several queues ≈ an SNS topic with subscribed SQS queues ≈ several consumer groups on one Kafka topic: each service gets its own copy;
- a topic exchange with patterns ≈ SNS with subscription filters; Kafka has no such routing — you create separate topics or filter on the consumer side;
- DLX ≈ a DLQ in SQS (a redrive policy) ≈ a dead letter topic in Kafka, which the consumer maintains itself;
- `x-delivery-limit` in a quorum queue ≈ `maxReceiveCount` in an SQS redrive policy;
- an unacked message that comes back when a consumer crashes ≈ the visibility timeout in SQS: the message is hidden while it's being processed and becomes visible again if the handler didn't delete it in time;
- RabbitMQ Streams and super streams ≈ a Kafka topic with partitions;
- Amazon MQ for RabbitMQ ≈ Amazon MSK: the same broker with an open protocol, but as a managed service.

**How to choose in one sentence:** task queues, complex routing, priorities, acknowledgement of each task, request-reply and standard protocols — RabbitMQ (or SQS and SNS if everything is in AWS and you don't want to run a broker). High-volume event streams, many independent consumers and re-reading history — Kafka.

## Typical mistakes

- **No prefetch** — the first consumer takes the whole queue into memory, the others sit idle.
- **Auto ack for important tasks** — a consumer crash loses all the received but unprocessed messages.
- **`nack(requeue=true)` without a limit** — a poison message circles forever; you need a DLX and `x-delivery-limit`.
- **Classic queues for important data** — on a node failure the queue is unavailable, and without persistent messages the data disappears on restart. For important data — quorum queues.
- **No publisher confirms** — the producer "sent" it, but the broker crashed before writing, and the message is lost without a trace.
- **Unroutable messages** — a typo in the routing key, and messages silently disappear. `mandatory` or an alternate exchange.
- **A connection per message** — connection churn loads the broker more than the messages themselves.
- **RabbitMQ as storage** — millions of messages in queues, a memory alarm, blocked producers. For storage and re-reading — streams or Kafka.
- **A cluster stretched across regions** — latency breaks quorum and performance; between sites — federation or shovel.
- **The default `disk_free_limit`** — the disk fills up almost completely before the protection kicks in.
- **The guest user and the web UI on the internet.**
- **Non-idempotent handlers** — duplicates after redeliveries and reconnections.

## Interview questions

**What is RabbitMQ and how does it work?**
A message broker written in Erlang with the AMQP protocol. A producer sends a message to an exchange with a routing key, the exchange distributes it into queues by bindings, and consumers receive messages from the queues and acknowledge processing, after which the message is deleted. Routing, retries, TTL, priorities and dead letter are on the broker's side.

**What exchange types are there?**
Direct — an exact match of the routing key and the binding key. Topic — patterns over dot-separated words (`*` — one word, `#` — any number). Fanout — to all bound queues regardless of the key. Headers — by message headers. Plus the default exchange: a nameless direct exchange to which every queue is bound by its name.

**What happens to a message that didn't get into any queue?**
By default it's silently dropped. To prevent this: publish with the `mandatory` flag (the broker returns the message to the producer) or set up an alternate exchange where all unroutable messages go.

**How do you guarantee that messages aren't lost?**
At every step: a durable queue of the quorum type (replicated to several nodes), persistent messages, publisher confirms on the producer, manual ack on the consumer after processing. Plus monitoring of alarms and dead letter. Duplicates are still possible, so handlers are made idempotent.

**How does a classic queue differ from a quorum queue and a stream?**
A classic queue lives on one node and is fast, but unavailable when the node fails. A quorum queue is replicated via Raft to several nodes, elects a new leader on failure and supports a redelivery limit — the standard for important data. A stream is a replicated log: messages aren't deleted after reading, and many consumers read from any position, as in Kafka. Classic mirrored queues were removed in RabbitMQ 4.0.

**What is prefetch and how do you choose it?**
The maximum number of unacknowledged messages the broker gives one consumer. Without it one consumer takes the whole queue; with prefetch 1 it's fair but slow. Usually 10–300: less for long tasks, more for fast small messages. It's tuned with a load test for throughput and even distribution.

**What is a dead letter exchange and when does a message get there?**
An exchange to which the broker moves "dead" messages: rejected with `requeue=false`, expired by TTL, pushed out of an overflowing queue, or over the delivery limit in a quorum queue. The `x-death` header stores the reason. The dead letter queue is monitored and sorted out, and after a fix the messages are returned to work.

**How do you make a delayed retry?**
Without plugins — a waiting queue with `x-message-ttl` and a dead letter exchange pointing back to the working exchange: the message waits for the set time and returns. Several such queues with increasing TTLs give an exponential delay. The alternative is the delayed message exchange plugin, but it doesn't replicate delayed messages.

**How does RabbitMQ provide high availability?**
A cluster of 3 or 5 nodes and quorum queues replicated via Raft: when a node fails, a new queue leader is elected as long as a majority of the replicas is alive. Clients connect to several nodes or via a load balancer and can reconnect. Upgrades go one node at a time with `rabbitmq-upgrade drain`. A cluster isn't stretched across data centres — federation or shovel is used.

**What happens during a network partition of the cluster?**
The nodes are alive but can't see each other. Quorum queues keep working only on the side with a majority of replicas; on the minority they're unavailable — so two diverging versions of the data don't appear. The old classic mirrored queues could diverge and lose messages in this situation, which is one of the reasons they were removed.

**The producers stopped sending messages, there are no errors. What do you check?**
`rabbitmq-diagnostics alarms`: almost always a memory or disk alarm went off and the broker blocked publishing. The cause is usually lagging or crashed consumers and grown queues. Next: queue depth, the number of consumers, `memory_breakdown`, free disk. The fix is to restore consumption, limit queue lengths, raise the limits, and separate publishing and consuming onto different connections.

**How do you preserve the order of message processing?**
Order exists within one queue, but competing consumers and redeliveries break it. Options: a single active consumer (one reads, the others are on standby), splitting by key into several queues via the consistent hash exchange (order within a key, parallelism across keys), processing without `requeue` to the head of the queue. If strict per-key order is needed at large scale, that's an argument for Kafka.

**RabbitMQ or Kafka — which would you choose?**
RabbitMQ — task queues, complex routing, priorities and delayed delivery, acknowledgement of each task, request-reply, standard protocols (AMQP, MQTT). Kafka — high-volume event streams, many independent consumers of the same data, retention and re-reading of history, CDC and stream analytics. RabbitMQ is simpler to operate at small and medium volumes, Kafka scales better in throughput.

**RabbitMQ or SQS and SNS in AWS?**
SQS and SNS don't need operating: they scale by themselves, billing is per request, DLQs and FIFO are there. But they lack flexible exchange routing, priorities and standard protocols (only the AWS API), and delivery latency is higher. RabbitMQ (or Amazon MQ) is chosen when you need AMQP or MQTT, complex routing, portability across clouds or migrating an existing RabbitMQ-based system.

**Amazon MQ or your own RabbitMQ in Kubernetes?**
Amazon MQ is a managed cluster of 3 nodes in different AZs with patches and CloudWatch metrics, but not all plugins and settings, and versions arrive with a delay. Your own RabbitMQ via the Cluster Operator gives any plugins and settings, queues and permissions in git via the Topology Operator, the same in any cloud, but upgrades, disks and monitoring are on you. A small team in AWS — Amazon MQ; a Kubernetes platform across several clouds — the operator.

**A queue is growing, and there are consumers. How do you investigate?**
I'd compare the publish and ack rates: is processing slower than the incoming flow, or have the consumers hung (lots of unacked). I'd check prefetch (isn't one consumer taking everything), errors and `nack` with `requeue` (poison messages going in circles), slow dependencies (a database, an external API). Solutions: add consumers, tune prefetch, move poison messages to a DLX with a delivery limit, limit the queue length so as not to reach a memory alarm.
