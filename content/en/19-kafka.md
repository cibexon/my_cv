# Kafka

A thorough Apache Kafka primer: how it works (brokers, topics, partitions, offsets), producers and consumers, delivery guarantees, replication and durability, retention and compaction, scaling, the ecosystem (Connect, Schema Registry, MirrorMaker), security, monitoring, running it in the cloud (Amazon MSK, Strimzi in Kubernetes) and a comparison with RabbitMQ, SQS, SNS and Kinesis. Each topic: what it is, why, how it works, an example with explanations and typical mistakes. At the end — interview questions with answers.

How to read: bring up a local broker from the first section and run the commands as you go. The output is shown in comments after `# →`. The detailed comparison with RabbitMQ is in the "Kafka and other brokers" section and in the [RabbitMQ](#21-rabbitmq.md/s14) topic.

## How to learn without a course

Kafka is easiest to learn on a single broker in Docker. Since version 4.0 it doesn't need ZooKeeper, so a single container is enough:
```bash
docker run -d --name kafka -p 9092:9092 apache/kafka:4.1.0     # a single broker in KRaft mode, listening on localhost:9092
docker exec -it kafka bash
cd /opt/kafka/bin                                               # all the tools are here: kafka-topics.sh, kafka-console-producer.sh...
./kafka-topics.sh --bootstrap-server localhost:9092 --list
```
`--bootstrap-server` is the address of any broker the client starts from: it gets the list of all the cluster's brokers from it and then talks to the right ones directly.

For experiments with replication and failures you need a cluster of three brokers: docker compose with three `apache/kafka` services or a kind cluster with Strimzi (see the "Kafka in the cloud and Kubernetes" section).

The plan: a topic → a producer and consumer in the console → keys and partitions → consumer groups and lag → replication and a broker failure → retention and compaction → a Python client → monitoring.

## What Kafka is and when you need it

**Kafka** is a distributed **event log**: messages are appended to the end, kept for a set time (days, weeks or forever) and can be read by many independent consumers, each at its own pace and from any position.

The main difference from a classic queue (RabbitMQ, SQS): a message **isn't deleted after it's read**. A consumer only moves its pointer (offset — the position number in the log). Therefore:
- several systems independently read the same event: billing, analytics, search, audit;
- you can **re-read history**: you fixed a bug in a handler — rewind the offset and process again;
- a new service can read the events of the last 7 days that happened before it existed.

When you need Kafka:
- **event streams** between microservices (an event-driven architecture: "order created", "payment went through");
- **collecting logs, metrics, clicks** at high volume — hundreds of thousands and millions of messages per second;
- **CDC** (Change Data Capture — a stream of changes from a database: every row insert and update becomes an event, for example via Debezium);
- **real-time stream processing** (Kafka Streams, Flink): aggregates, enrichment, fraud detection;
- **a buffer** between fast producers and slow consumers.

When Kafka is overkill: a dozen background jobs a minute, routing each message by complex rules, priorities, delayed delivery, request-reply (RPC). RabbitMQ or SQS is simpler here. Kafka is a heavy system: a cluster, disks, monitoring, upgrades.

## Core concepts

- **Broker** — a Kafka server. A cluster is several brokers (at least 3 in prod).
- **Topic** — a named stream of messages of one meaning: `orders`, `payments`, `clickstream`.
- **Partition** — a topic is split into partitions, each a separate ordered log on one broker's disk. Partitions are the unit of parallelism and scaling: different partitions live on different brokers and are read by different consumers in parallel.
- **Offset** — the number of a message within a partition (0, 1, 2…). Order is guaranteed **only within one partition**; there's no order across partitions.
- **Message (record)** — a key (optional), a value (the data itself, bytes), headers, a timestamp.
- **Segments** — on disk a partition is stored as segment files (up to 1 GB by default). Old segments are deleted whole according to the retention rules.
- **Replication** — each partition has copies on several brokers (the replication factor, usually 3). One copy is the **leader** (all writes and reads go through it), the others are **followers** that copy the data from the leader.
- **ISR (In-Sync Replicas)** — the replicas that don't lag behind the leader. Only they can become the new leader without data loss.
- **Controller** — a broker (or a dedicated node) that stores the cluster metadata (which topics exist, where the leaders are) and assigns a new leader on failure. In **KRaft** mode (Kafka Raft) the Kafka nodes do this themselves using the Raft consensus protocol. This used to require a separate **ZooKeeper** cluster; Kafka 4.0 removed ZooKeeper support entirely.

```text
topic orders, 3 partitions, replication factor 3, brokers 1–3

partition 0:  leader on broker 1, copies on 2 and 3   [0][1][2][3][4][5] ← writes go to the end
partition 1:  leader on broker 2, copies on 3 and 1   [0][1][2][3]
partition 2:  leader on broker 3, copies on 1 and 2   [0][1][2][3][4]
```

Try it:
```bash
./kafka-topics.sh --bootstrap-server localhost:9092 --create --topic orders --partitions 3 --replication-factor 1
./kafka-topics.sh --bootstrap-server localhost:9092 --describe --topic orders
# → Topic: orders  TopicId: rt0U7cmORHS8gulvm0MiqQ  PartitionCount: 3  ReplicationFactor: 1  Configs: min.insync.replicas=1,segment.bytes=1073741824
# →   Topic: orders  Partition: 0  Leader: 1  Replicas: 1  Isr: 1  Elr:   LastKnownElr:
# →   Topic: orders  Partition: 1  Leader: 1  Replicas: 1  Isr: 1  Elr:   LastKnownElr:
# →   Topic: orders  Partition: 2  Leader: 1  Replicas: 1  Isr: 1  Elr:   LastKnownElr:
```
`Leader` is the number of the partition's leader broker, `Replicas` — where the copies are, `Isr` — which of them are in sync (see "Durability"). `Elr` (eligible leader replicas) is a list that appeared in Kafka 4.x of replicas that can safely become leader even after dropping out of the ISR.

## The producer: how a message gets into a partition

A **producer** is a client that writes messages. Which partition a message goes to:
- **with a key** — by the key's hash: `hash(key) % number_of_partitions`. All messages with the same key (for example, `order_id` or `user_id`) always go to the same partition — which means they're processed **in order**;
- **without a key** — the producer spreads messages across partitions in batches (the sticky partitioner: it fills a batch for one partition, then moves to the next) — evenly, but without order.

```bash
# a producer with keys: input in the key:value format
./kafka-console-producer.sh --bootstrap-server localhost:9092 --topic orders \
  --property parse.key=true --property key.separator=:
> order-1:created
> order-2:created
> order-1:paid
> order-1:shipped
```
All `order-1` events end up in one partition and are read in the order created → paid → shipped. A check with a consumer that prints the partition number:
```text
Partition:0	order-2	created
Partition:1	order-1	created
Partition:1	order-1	paid
Partition:1	order-1	shipped
```

**The main producer settings:**
| Parameter | What it does | Practice |
|---|---|---|
| `acks` | when a write counts as successful: `0` — don't wait for a reply, `1` — the leader wrote it, `all` — all replicas in the ISR wrote it | `all` for important data (the default since Kafka 3.0) |
| `enable.idempotence` | the broker drops duplicates on resend (each message has a producer and sequence number) | `true` (the default since 3.0) — retries don't create duplicates |
| `retries`, `delivery.timeout.ms` | how many times and how long to retry on network errors and leader changes | by default retries for up to 2 minutes |
| `linger.ms`, `batch.size` | how long to wait while filling a batch of messages before sending | `linger.ms=5–20` — bigger batches, higher throughput, a few ms more latency |
| `compression.type` | batch compression: `lz4`, `zstd`, `snappy`, `gzip` | `lz4` or `zstd` — less traffic and disk |
| `max.in.flight.requests.per.connection` | how many unacknowledged requests at once | with idempotence up to 5 — order is preserved |

A situation: the producer sent a message, the leader wrote it, but the reply was lost in the network. Without idempotence the producer resends, and the topic ends up with a **duplicate**. With `enable.idempotence=true` the broker recognises the retry by the sequence number and doesn't write it a second time.

## Consumers and consumer groups

A **consumer** reads messages, a **consumer group** is several instances of one service that share the work. The rules:
- each partition in a group is read by **exactly one** consumer;
- one consumer can read several partitions;
- **different groups read a topic independently**: the `billing` group and the `analytics` group each get all the messages.

```text
topic orders: 3 partitions

group billing (3 consumers):  P0 → c1   P1 → c2   P2 → c3
group billing (5 consumers): P0 → c1   P1 → c2   P2 → c3   c4, c5 sit idle
group analytics (1 consumer): P0, P1, P2 → a1
```
The conclusion that gets asked about: **having more consumers in a group than partitions is useless** — the extra ones sit idle. A group's maximum parallelism = the number of partitions.

**Offsets and commits.** A group stores how far it has read each partition in the internal topic `__consumer_offsets`. A consumer periodically **commits** the offset. After a restart it continues from the last committed position.
```bash
./kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic orders \
  --group billing --from-beginning --property print.key=true --property print.partition=true

./kafka-consumer-groups.sh --bootstrap-server localhost:9092 --describe --group billing
# → Consumer group 'billing' has no active members.     — the consumer has already stopped
# → GROUP    TOPIC   PARTITION  CURRENT-OFFSET  LOG-END-OFFSET  LAG  CONSUMER-ID  HOST  CLIENT-ID
# → billing  orders  0          1               1               0    -            -     -
# → billing  orders  1          3               3               0    -            -     -
# → billing  orders  2          0               0               0    -            -     -
```
- `CURRENT-OFFSET` — how far the group has read;
- `LOG-END-OFFSET` — the last message in the partition;
- **`LAG`** = the difference. A growing lag is the main symptom: the consumers can't keep up.

A **rebalance** is the redistribution of partitions among a group's consumers when someone joins, leaves or hangs longer than `max.poll.interval.ms` (5 minutes between `poll` calls by default). During the old "eager" rebalance the whole group stopped reading. Modern ways to reduce the pain: **cooperative rebalancing** (only the necessary partitions move), **static membership** (`group.instance.id` — a restarted pod comes back with its partitions without a rebalance), and the new group protocol from Kafka 4.0 (KIP-848: the broker computes the partition assignment rather than the clients).

A situation: a consumer processes a batch of messages for 7 minutes (heavy DB queries) → `max.poll.interval.ms` is exceeded → the broker considers it dead → rebalance → the partitions go to another consumer → that one processes the same messages → duplicates and endless rebalances. Solutions: lower `max.poll.records`, speed up processing or raise `max.poll.interval.ms`.

**Rewinding offsets** — reprocessing or skipping:
```bash
# the group must be stopped; first look (--dry-run), then apply (--execute)
./kafka-consumer-groups.sh --bootstrap-server localhost:9092 --group billing --topic orders \
  --reset-offsets --to-datetime 2026-10-28T09:00:00.000 --dry-run
./kafka-consumer-groups.sh --bootstrap-server localhost:9092 --group billing --topic orders \
  --reset-offsets --to-earliest --execute
```

## Delivery guarantees

The guarantee depends on **when the consumer commits the offset** relative to processing:
| Guarantee | How you get it | Risk |
|---|---|---|
| **At most once** | commit the offset, then process | crashed after the commit — the message is lost |
| **At least once** | process, then commit | crashed after processing but before the commit — the message is processed again |
| **Exactly once** | Kafka transactions: writing the results and committing offsets atomically | works only for "Kafka → processing → Kafka" chains |

In practice it's almost always **at least once + an idempotent handler**: processing the same message again doesn't change the result. Techniques: a unique event key and a table of processed IDs, `INSERT ... ON CONFLICT DO NOTHING`, an upsert by key instead of an increment.

**Exactly-once semantics (EOS)** in Kafka is the idempotent producer + **transactions**: the application reads from a topic, writes the result to another topic and commits the offsets in one transaction; consumers with `isolation.level=read_committed` don't see the results of aborted transactions. Kafka Streams turns this on with a single setting, `processing.guarantee=exactly_once_v2`. If the result goes to an external system (a database, an HTTP API), Kafka no longer gives the "exactly once" guarantee — the receiver has to be idempotent.

An at-least-once consumer example in Python (the `confluent-kafka` library):
```python
from confluent_kafka import Consumer

consumer = Consumer({
    "bootstrap.servers": "localhost:9092",
    "group.id": "billing",
    "enable.auto.commit": False,          # we commit ourselves, after processing
    "auto.offset.reset": "earliest",      # a new group starts from the beginning of the topic
})
consumer.subscribe(["orders"])

while True:
    msg = consumer.poll(1.0)
    if msg is None:
        continue
    if msg.error():
        print("error:", msg.error())
        continue
    handle(msg.key(), msg.value())        # processing must be idempotent
    consumer.commit(message=msg, asynchronous=False)   # commit after successful processing
```

## Durability: replication, ISR and a broker failure

Three settings that together determine whether data can be lost:
- **`replication.factor=3`** — each partition has 3 copies on different brokers;
- **`min.insync.replicas=2`** — a write with `acks=all` is accepted only if at least 2 replicas from the ISR confirmed it;
- **`acks=all`** on the producer.

What happens on failures in this setup:
- 1 of 3 brokers goes down → a new leader is chosen from the ISR for its partitions, writes continue (2 left in the ISR ≥ 2), no data is lost;
- 2 brokers go down → 1 replica left in the ISR < 2 → producers with `acks=all` get a `NotEnoughReplicas` error. Kafka **prefers to refuse a write rather than accept data that might be lost**. Reads keep working.

**`unclean.leader.election.enable`** — whether a replica outside the ISR may become leader if all in-sync copies are unavailable. `true` — the partition comes back faster but loses the messages the lagging replica didn't have. By default `false`: availability is sacrificed for durability.

**Rack awareness** — brokers are given `broker.rack` (for example, the AZ name), and Kafka spreads a partition's replicas across different racks or zones. Then losing a whole AZ doesn't take all the copies. Consumers with `client.rack` can read from the nearest replica (fetch from follower) rather than from the leader in another zone — in the cloud this saves money on cross-zone traffic.

Checking replication health:
```bash
./kafka-topics.sh --bootstrap-server localhost:9092 --describe --under-replicated-partitions
./kafka-topics.sh --bootstrap-server localhost:9092 --describe --unavailable-partitions
# empty output — all is well
```

## Retention, compaction, tiered storage

**Retention** — how long messages are kept:
- `retention.ms` — by time (7 days by default);
- `retention.bytes` — by size per partition (unlimited by default);
- **whole segments** are deleted, so messages may live a bit longer than specified.

```bash
./kafka-configs.sh --bootstrap-server localhost:9092 --alter --entity-type topics --entity-name orders \
  --add-config retention.ms=259200000            # 3 days
./kafka-configs.sh --bootstrap-server localhost:9092 --describe --entity-type topics --entity-name orders
```

**Log compaction** (`cleanup.policy=compact`) — instead of deleting by time, Kafka keeps **the latest value for each key**. Older versions of the same key are cleaned out over time. A message with a key and an empty value (a **tombstone**) means "the key is deleted".

Why: a topic as a table of current state. For example, `user-profiles` keyed by `user_id` — a new service reads the topic from the beginning and gets each user's current profile without going through the whole change history. This is also how Kafka stores its own `__consumer_offsets`.

**Tiered storage** (production-ready since Kafka 3.9) — fresh segments sit on the brokers' local disks, old ones are offloaded to object storage (S3). Keeping months of history becomes cheaper, and brokers with small disks recover faster. In MSK it's enabled with a storage setting.

**How much disk is needed** (an example calculation): 5,000 messages per second × 1 KB = 5 MB/s ≈ 430 GB a day. With 7 days of retention and a replication factor of 3: 430 × 7 × 3 ≈ 9 TB per cluster, plus a 30–50% margin for spikes and rebalancing.

## How many partitions and how to scale

Partitions set the ceiling on parallelism, but they have a cost: each means open files, memory on the broker, work for the controller, and time to elect leaders on failure.

How to choose the number of partitions:
- from the required throughput: if one consumer processes 1,000 messages per second and the stream is 20,000, you need at least 20 partitions (and 20 consumers);
- with headroom for growth, because **the number of partitions can't be decreased**, and increasing it **breaks the key distribution**: `hash(key) % N` changes when N changes, and new messages for the key `order-1` go to a different partition — order for that key is broken during the transition;
- typical values are from 6 to a few dozen per topic; thousands of partitions per topic is almost always a design mistake.

```bash
./kafka-topics.sh --bootstrap-server localhost:9092 --alter --topic orders --partitions 6   # increase only
```

**A hot partition** — one key generates disproportionately many messages (for example, a large customer), and its partition with a single consumer becomes the bottleneck while the others sit idle. Solutions: a different key (a composite `customer_id + order_id` if strict per-customer order isn't needed), a separate topic for the large customer.

**Adding brokers** doesn't move data by itself: new brokers stay empty until partitions are reassigned to them (`kafka-reassign-partitions.sh` or Cruise Control — a tool for automatic load balancing across brokers; it's built into Strimzi). Reassignment copies data over the network, so it's rate-limited (throttled) to avoid interfering with production traffic.

## The ecosystem: Connect, Schema Registry, Streams, MirrorMaker

- **Kafka Connect** — a framework for moving data between Kafka and external systems without your own code. **Source** connectors read from a source into Kafka (Debezium for CDC from PostgreSQL and MySQL, JDBC), **sink** connectors write from Kafka outward (S3, Elasticsearch/OpenSearch, ClickHouse). A connector is described by a JSON config and runs in a cluster of Connect workers.
- **Schema Registry** — a store of message schemas (Avro, Protobuf, JSON Schema). The producer registers the schema, only its ID goes into the message, and the consumer gets the schema by the ID. The key point is **compatibility checking**: you can't roll out a producer that removed a required field and will break all the consumers. Modes: `BACKWARD` (new consumers read old data), `FORWARD`, `FULL`. Options — Confluent Schema Registry, Apicurio, AWS Glue Schema Registry.
- **Kafka Streams** — a Java stream-processing library inside an ordinary application: filtering, aggregates over time windows, joining streams; state is kept in a local RocksDB and in Kafka changelog topics. **Apache Flink** is a separate cluster engine for the same at a larger scale.
- **MirrorMaker 2** — replication of topics between clusters (built on Connect): DR in another region, migration to the cloud, an aggregating cluster. It also translates consumer group offsets so consumers can switch to the second cluster.

```json
{
  "name": "orders-to-s3",
  "config": {
    "connector.class": "io.confluent.connect.s3.S3SinkConnector",
    "topics": "orders",
    "s3.bucket.name": "acme-events-archive",
    "s3.region": "eu-central-1",
    "storage.class": "io.confluent.connect.s3.storage.S3Storage",
    "format.class": "io.confluent.connect.s3.format.parquet.ParquetFormat",
    "flush.size": "10000",
    "tasks.max": "3"
  }
}
```
The connector archives the `orders` topic to S3 in Parquet format in batches of 10,000 messages. For MSK the same connector is run in **MSK Connect** (managed Connect workers).

## Security

By default a local broker accepts everything without a password or encryption. In prod:
- **encryption**: TLS between clients and brokers and between brokers;
- **authentication**: mTLS (client certificates), **SASL/SCRAM** (a login and password with a secure exchange), SASL/OAUTHBEARER (tokens from an IdP — an identity management system), **IAM** in Amazon MSK (the client signs the request with its IAM role, as when calling the AWS API);
- **authorization**: ACLs — who can read, write, create topics:
```bash
./kafka-acls.sh --bootstrap-server kafka:9093 --command-config admin.properties \
  --add --allow-principal User:billing --operation Read --topic orders --group billing
./kafka-acls.sh --bootstrap-server kafka:9093 --command-config admin.properties --list --topic orders
```
- **network**: brokers in private subnets, access only from the needed Security Groups; in MSK across accounts — via multi-VPC connectivity (PrivateLink).
- **data**: disk encryption (KMS in MSK); personal data in events — minimise it or encrypt it at the field level, because it can't be "deleted" from an immutable log except via retention or compaction with a tombstone.

## Monitoring

What to watch first:
| Metric | Why it matters | Alert |
|---|---|---|
| **Consumer lag** per group | consumers can't keep up, data is processed late | lag grows for N minutes in a row or the time lag > SLO |
| **UnderReplicatedPartitions** | replicas lag or a broker is down — reduced durability margin | > 0 for more than a couple of minutes |
| **OfflinePartitionsCount** | a partition has no leader — neither writes nor reads | > 0 immediately |
| **ActiveControllerCount** | how many controllers are active | ≠ 1 |
| ISR size and **IsrShrinksPerSec** | replicas often drop out of the ISR — network, disk or overload | growth |
| Produce and Fetch request latency | the brokers are overloaded | p99 above normal |
| Broker disk | a full disk = a stopped broker | > 75–80% |
| Producer errors | `NotEnoughReplicas`, timeouts | > 0 |

Tools: broker metrics via JMX (the Java standard for metrics) and the JMX exporter for Prometheus; **lag** — kafka-exporter or Burrow (it evaluates not only the number but also the trend); web UIs for people — Kafka UI (Provectus), AKHQ, Redpanda Console. In MSK — CloudWatch metrics and open monitoring with Prometheus; in Strimzi — ready-made Grafana dashboards.

Lag in messages is deceptive: a lag of 10,000 messages with a stream of 100,000 per second is 0.1 seconds, while at 10 per second it's 17 minutes. For alerts, a **time lag** (the age of the last processed message) is better.

## Kafka in the cloud and Kubernetes: Amazon MSK, Strimzi, Confluent

Options:
- **Amazon MSK** (provisioned) — AWS manages the brokers: patches, replacing failed ones, monitoring, encryption. You choose the type and number of brokers, the disk, the version. **MSK Express brokers** — a new type of broker with AWS-managed storage (they recover and scale faster). **MSK Serverless** — no brokers, billing for traffic and partitions, but with limits on the number of partitions and settings. In addition: **MSK Connect** (managed Connect workers), IAM authentication, tiered storage.
- **Strimzi** — a Kafka operator for Kubernetes (a CNCF project): the cluster, topics and users are described as CRDs, and the operator deploys the brokers as pods, does rolling updates and manages certificates.
- **Confluent Cloud** — SaaS from Kafka's creators with extra features (Schema Registry, connectors, ksqlDB) — expensive, but minimal operations.
- Compatible alternatives: **Redpanda** (written in C++, no JVM, protocol-compatible), **WarpStream** and similar (stateless brokers, data goes straight to S3 — cheaper, but higher latency).

```yaml
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaNodePool
metadata:
  name: brokers
  labels:
    strimzi.io/cluster: events          # which cluster the pool belongs to
spec:
  replicas: 3
  roles: [controller, broker]          # KRaft: the same nodes store metadata and data
  storage:
    type: persistent-claim
    size: 500Gi
    class: gp3
---
apiVersion: kafka.strimzi.io/v1beta2
kind: Kafka
metadata:
  name: events
spec:
  kafka:
    version: 4.0.0
    listeners:
      - name: tls
        port: 9093
        type: internal
        tls: true
        authentication:
          type: tls                    # clients log in with certificates
    config:
      default.replication.factor: 3
      min.insync.replicas: 2           # a write is acknowledged when 2 of the 3 replicas have it
  entityOperator:
    topicOperator: {}                  # topics as KafkaTopic resources in git
    userOperator: {}                   # users and ACLs as KafkaUser resources
```
A topic is also described as a resource and lives in git together with the rest of the configuration:
```yaml
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaTopic
metadata:
  name: orders
  labels:
    strimzi.io/cluster: events
spec:
  partitions: 12
  replicas: 3
  config:
    retention.ms: 604800000            # 7 days
    min.insync.replicas: 2
```

Arguments for the choice:
| | Amazon MSK | Strimzi in EKS |
|---|---|---|
| Operations | AWS replaces failed brokers, patches the OS and Kafka | the operator automates, but disks, nodes, Strimzi and Kafka upgrades are on you |
| Cost | more expensive per broker; **replication between brokers within the cluster is free** | cheaper in hardware, but the cross-zone replication traffic (between AZs) is billed as ordinary EC2 traffic — with large flows this is noticeable money |
| Versions and settings | only the versions MSK supports and part of the parameters | any versions and parameters, any plugins |
| Access and security | IAM authentication of clients, VPC integration, PrivateLink for other accounts | TLS and SCRAM, ACLs via KafkaUser, everything in manifests |
| Portability | AWS only | the same in any cloud and on-prem |
| Observability | CloudWatch metrics and open monitoring with Prometheus | Prometheus metrics and ready-made Strimzi dashboards |

How to answer briefly: **MSK** — when you need Kafka as a service, the team is small and the platform lives only in AWS. **Strimzi** — when the platform is already on Kubernetes, you need the same installation in several clouds or on-prem, special versions and settings, and there are people ready to own Kafka's disks and upgrades. In both cases it's worth mentioning in an interview: a replication factor of 3 with `min.insync.replicas=2` and `acks=all` on the producers, spreading brokers across three AZs (rack awareness) and monitoring consumer lag.

**Upgrading a cluster without downtime** (in MSK and Strimzi the service or operator does this, by hand it's the same): brokers are restarted **one at a time**, and before each next one you wait until under-replicated partitions are back to 0. Clients survive this thanks to retries and switching to the new leaders. So clients should have reasonable `retries` and `delivery.timeout.ms`, and topics should have RF 3.

## Kafka and other brokers: RabbitMQ, SQS, SNS, Kinesis

The main difference is in the model:
- **Kafka is a log**: it keeps messages for a set time, consumers move their own offset, messages can be re-read, many independent groups read the same thing. "A smart client, a simple broker".
- **RabbitMQ is a routing broker with queues**: a message is delivered to a queue by routing rules, the consumer acknowledges processing, and the message is **deleted**. "A smart broker, a simple client". In detail — in the [RabbitMQ](#21-rabbitmq.md/s1) topic.

| | Kafka | RabbitMQ | Amazon SQS | Amazon SNS | Amazon Kinesis Data Streams |
|---|---|---|---|---|---|
| Model | a log with partitions | queues + routing via exchanges | a task queue | broadcast (pub/sub) | a log with shards |
| After reading | stays until retention | deleted after ack | deleted after processing | delivered to subscribers, not stored | stays until retention (up to 365 days) |
| Re-reading history | yes, by rewinding the offset | no (except RabbitMQ Streams) | no | no | yes |
| Order | within a partition | within a queue with a single consumer | FIFO queues — within a message group | FIFO topics | within a shard |
| Scaling reads | consumers ≤ partitions | any number of competing consumers per queue | any number | — | consumers per shard |
| Routing | by topic and key | flexible: direct, topic by pattern, fanout, headers | none | subscription filters | none |
| Priorities, delayed delivery | no | yes (priorities, TTL + DLX, the delayed plugin) | a delay of up to 15 minutes | no | no |
| Throughput | millions of messages/s per cluster | tens to hundreds of thousands of messages/s | practically unlimited (Standard) | high | 1 MB/s or 1000 records/s per shard for writes |
| Operations | heavy (or MSK) | medium (or Amazon MQ) | none, serverless | none | minimal |

**Analogies that help in an interview:**
- a Kafka consumer group ≈ several competing consumers on one RabbitMQ or SQS queue: the work is shared between instances;
- several consumer groups on one topic ≈ a fanout exchange in RabbitMQ (or SNS → several SQS queues): each system gets its own copy;
- a Kafka partition and key ≈ a message group in SQS FIFO: order within a key and parallelism across keys;
- retention and re-reading ≈ RabbitMQ Streams or Kinesis;
- a dead letter queue: Kafka has no built-in one — you create a separate topic `orders.DLT`, to which the consumer itself writes the "bad" messages; in RabbitMQ it's a DLX, in SQS — a redrive policy;
- Kinesis Data Streams ≈ a "simpler" managed Kafka: a shard ≈ a partition, but fewer settings, no Connect ecosystem, and throughput is counted per shard.

**How to choose in one sentence:** you need high-volume event streams, many independent consumers and re-reading history — Kafka (or MSK, Kinesis). You need task queues, complex routing, priorities, acknowledgement of each task and request-reply — RabbitMQ (or SQS and SNS if everything is in AWS and you don't want to operate a broker).

## Typical mistakes

- **More consumers than partitions** — the extra ones sit idle, and scaling "doesn't help".
- **Too few partitions at the start** — increasing later breaks per-key order; too many — load on the cluster and slow leader elections.
- **`acks=1` or RF=1 for important data** — the failure of one broker loses acknowledged messages.
- **`min.insync.replicas=1` with RF=3** — `acks=all` is formally there, but a write is acknowledged by one replica.
- **Non-idempotent handlers** with at-least-once — duplicates after every rebalance and restart.
- **Long processing inside the poll loop** — exceeding `max.poll.interval.ms`, endless rebalances.
- **Auto-committing offsets before processing** (`enable.auto.commit=true` with asynchronous processing) — message loss on a crash.
- **Kafka as a database for queries** — you can't quickly find a message by a field in a topic; for that the data is exported to a DB or a search engine.
- **Huge messages** (tens of megabytes) — the `message.max.bytes` limit is about 1 MB by default; large objects go to S3, and a reference goes to Kafka.
- **No lag monitoring** — the problem is noticed when users complain about stale data.
- **No schemas** — one producer changes the format and breaks all the consumers; you need a Schema Registry with compatibility checks.
- **All replicas' brokers in one AZ** — losing the zone takes all the copies; you need `broker.rack`.

## Interview questions

**What is Kafka and how does it differ from an ordinary message queue?**
A distributed event log: messages are appended to the end of a topic's partitions and kept for a set time rather than deleted after reading. Each consumer group stores its own offset and reads independently, and messages can be re-read. In an ordinary queue (RabbitMQ, SQS) a message is deleted after acknowledgement and goes to one consumer. Kafka is designed for large event streams and many independent consumers, a queue — for distributing tasks.

**What are a topic, a partition and an offset?**
A topic is a named stream of messages. It's split into partitions — ordered logs that are distributed across brokers and provide parallelism. An offset is the number of a message within a partition; a consumer group stores the offset up to which it has read each partition. Order is guaranteed only within a partition.

**How do you guarantee the order of message processing?**
Order exists only within a partition, so all related messages are sent with the same key (for example, `order_id`) — they go to one partition and are processed in order by one consumer. On the producer — idempotence (then retries don't change the order with `max.in.flight` up to 5). Global order across the whole topic is possible only with a single partition, which kills parallelism.

**How do consumer groups work? What happens if there are more consumers than partitions?**
A topic's partitions are distributed among the group's consumers so that each is read by exactly one consumer. Different groups read the topic independently. If there are more consumers than partitions, the extra ones sit idle — a group's maximum parallelism equals the number of partitions.

**What is consumer lag and how do you deal with it?**
The difference between the last offset in a partition and the group's committed offset — how many messages haven't been processed yet. A growing lag means the consumers can't keep up. It's monitored via kafka-exporter or Burrow, and alerting is better on the time lag. Solutions: more consumers (up to the number of partitions), more partitions, faster processing (batches, asynchronous I/O), finding a slow consumer or a hot partition.

**What delivery guarantees does Kafka have?**
At most once — commit before processing, loss is possible. At least once — commit after processing, duplicates are possible; this is the standard together with idempotent handlers. Exactly once — the idempotent producer plus transactions, it works for Kafka → Kafka chains (Kafka Streams with `exactly_once_v2`); when writing to external systems they have to be idempotent.

**How do you configure Kafka so that data isn't lost?**
Replication factor 3, `min.insync.replicas=2`, `acks=all` and idempotence on the producers, `unclean.leader.election.enable=false`, replicas in different AZs via `broker.rack`, committing offsets only after processing, monitoring under-replicated partitions. Then the failure of one broker neither loses nor stops writes, and if two fail Kafka refuses writes but doesn't lose acknowledged data.

**What is the ISR and why min.insync.replicas?**
The ISR is the replicas of a partition that don't lag behind the leader; only they become the new leader without data loss. `min.insync.replicas` is the minimum number of replicas in the ISR that must confirm a write with `acks=all`. If the ISR is smaller than this, the write is rejected with `NotEnoughReplicas`: Kafka prefers write unavailability to data loss.

**What is a rebalance and how do you reduce its impact?**
The redistribution of partitions among a group's consumers when a consumer joins, leaves or hangs. While a classic rebalance is in progress, the group doesn't read. What helps: cooperative rebalancing (only the necessary partitions move), static membership (`group.instance.id`), the new group protocol in Kafka 4.0, and short processing inside `poll` (`max.poll.records`, `max.poll.interval.ms`) so the consumer isn't considered dead.

**What is log compaction and when do you use it?**
A cleanup policy under which Kafka keeps the latest message for each key instead of deleting by time; a tombstone (a key with an empty value) deletes the key. It's used for state topics: user profiles, settings, reference data, Kafka Streams changelogs — a new consumer reads the topic and gets the current state for each key.

**How do you choose the number of partitions? Can it be changed?**
From the required throughput and the speed of one consumer, with headroom for growth. It can be increased but not decreased. Increasing it changes the key distribution (`hash(key) % N`), so for topics with per-key order it's done deliberately, or a new topic is created with a migration. Too many partitions load the brokers and the controller.

**Why was ZooKeeper needed and what is KRaft?**
ZooKeeper used to store the cluster metadata and elect the controller. KRaft is consensus built into Kafka on the Raft protocol: the Kafka nodes themselves store the metadata in the controller role. It's simpler to operate (one system instead of two), recovers faster and supports more partitions. Kafka 4.0 removed ZooKeeper support; ZooKeeper-based clusters first migrate to KRaft on version 3.x.

**How do you move Kafka to another region or set up DR?**
MirrorMaker 2 (or Confluent Cluster Linking, MSK Replicator in AWS) replicates topics and translates consumer group offsets to the second cluster. Producers and consumers switch via DNS or configuration. You need to account for the asynchrony (RPO — the replication lag), duplicates after switching (idempotent handlers) and the fact that offsets differ between clusters — without offset translation consumers start from the wrong place.

**Why do you need a Schema Registry?**
It stores message schemas (Avro, Protobuf, JSON Schema) and checks the compatibility of new versions. A producer won't be able to register a schema that breaks consumers (for example, removing a required field in BACKWARD mode). Only the schema ID is sent in a message, so messages are more compact than JSON with field names.

**Kafka or RabbitMQ — which would you choose?**
Kafka — for high-volume event streams, several independent consumers of the same data, re-reading history, CDC and stream analytics. RabbitMQ — for task queues, complex rule-based routing, priorities and delayed delivery, request-reply and when you need acknowledgement of each message. If everything is in AWS and volumes are moderate — SQS and SNS without operating a broker.

**Kafka or Kinesis Data Streams in AWS?**
Kinesis is fully managed, billed per shard, integrates with Lambda and Firehose, but has strict per-shard limits, fewer settings, no Kafka ecosystem (Connect, Streams) and AWS lock-in. Kafka (MSK) has higher throughput and flexibility, standard clients and tools, portability, but more operations and usually a higher minimum price. Small and medium streams in an AWS-only platform — Kinesis; large streams and the Kafka ecosystem — MSK.

**Amazon MSK or Strimzi — which would you choose and why?**
MSK is Kafka as a service: AWS replaces brokers and patches, IAM authentication, inter-broker replication isn't billed; the downsides are limited versions and parameters, AWS only, a higher broker price. Strimzi is a Kafka operator in Kubernetes: everything in manifests and git, any versions and settings, the same in any cloud; the downsides are that disks, nodes and upgrades are on you, and cross-zone replication traffic is billed. A small team in one AWS — MSK; multi-cloud or on-prem on Kubernetes with an experienced team — Strimzi. In both: RF=3, `min.insync.replicas=2`, `acks=all`, brokers in three AZs, consumer lag monitoring.

**A Kafka broker went down. What happens and what do you check?**
For the partitions where it was the leader, the controller elects new leaders from the ISR, and clients reconnect using the updated metadata — writes and reads continue within seconds. I'd check: OfflinePartitions (should be 0), UnderReplicatedPartitions (will be > 0 until the broker returns), `NotEnoughReplicas` producer errors (if the ISR dropped below `min.insync.replicas`), the cause of the failure (disk, memory, JVM, network). After it returns, the broker catches up with the leaders; if it doesn't return — replace it and reassign the partitions.

**Consumers can't keep up and lag is growing. How do you investigate?**
I'd look at whether the lag grows on all partitions or on one: on one — a hot key or a stuck consumer. I'd check the number of consumers against partitions, rebalances in the logs (hangs because of `max.poll.interval.ms`), the processing speed and what slows it down (a database, an external API). Solutions: add consumers up to the number of partitions, add partitions, process in batches and asynchronously, move the slow part to a separate topic, fix the key distribution.
