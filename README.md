# Firescroll 

An unkillable distributed KV database with unlimited read throughput. Have replicas in any number of regions without impacting write or read performance of other nodes in the cluster. No maintenance or repairs required.

Perfect for configuration management at scale where you want global low-latency reads without caching or cold first reads.

Useful for low-latency cases that can tolerate short cache-like behavior such as:
- DNS providers (mapping a domain name to records)
- Webhost routing (e.g. Vercel mapping your domain to your code)
- Feature flagging
- SSL certificate serving
- CDN configs
- and many more!

<!-- TOC -->
* [Firescroll](#firescroll-)
  * [Features](#features)
  * [Quick Notes](#quick-notes)
  * [API](#api)
    * [Put Record(s) `POST /records/put`](#put-records-post-recordsput)
    * [Get Record(s) `POST /records/get`](#get-records-post-recordsget)
    * [Delete Record(s) `POST /records/delete`](#delete-records-post-recordsdelete)
    * [(WIP) List Records `POST /records/list`](#wip-list-records-post-recordslist)
    * [(WIP) Batch Put and Delete Records `POST /records/batch`](#wip-batch-put-and-delete-records-post-recordsbatch)
  * [Setup](#setup)
    * [Mutations Topic](#mutations-topic)
    * [Partitions Topic](#partitions-topic)
  * [Running Locally](#running-locally)
  * [Configuration](#configuration)
  * [(WIP) The `If` statement](#wip-the-if-statement)
    * [Examples:](#examples)
      * [Check existence of a record](#check-existence-of-a-record)
  * [Scaling the cluster](#scaling-the-cluster)
    * [Scaling the replica group](#scaling-the-replica-group)
    * [Scaling the number of replica groups](#scaling-the-number-of-replica-groups)
    * [Scaling resources for the nodes](#scaling-resources-for-the-nodes)
    * [Scaling the log](#scaling-the-log)
  * [Topic Management](#topic-management)
    * [Mutations](#mutations)
    * [Partitions](#partitions)
  * [Backups and Snapshotting](#backups-and-snapshotting)
  * [Recommended Redpanda/Kafka Settings](#recommended-redpandakafka-settings)
  * [Architecture](#architecture)
<!-- TOC -->

## Features

- Quick durability of writes with decoupled read throughput
- Arbitrary number of read replicas supported
- Designed to serve global low latency reads without sacrificing write performance
- Remote partition proxying of Get requests
- (WIP) Atomic mutation batches
- (WIP) Conditional (If) statements checked at mutation time
- Support for arbitrary number of regions with varying latencies without impacting write or read performance

## Quick Notes

This Readme is still a WIP, and so is this project. If you see any areas of improvement please open an issue!

I would not call this "production ready" in the traditional sense, as it lacks extensive automated testing.

Redpanda and Kafka are used pretty interchangeably here. They mean the same thing, and any differences (e.g. configuration properties) will be noted.

## API

The API is HTTP/1.1 & 2 compatible, with all operations as a `POST` request and JSON bodies.

See examples in [records.http](api/records.http)

### Put Record(s) `POST /records/put`

### Get Record(s) `POST /records/get`

### Delete Record(s) `POST /records/delete`

### (WIP) List Records `POST /records/list`

Can have starts_after or ends_before to determine the direction, and prefix

Can specify a pk or a partition number. If pk then sk is the filter. If partition number then pk and sk are filters. Use `eq`, `gt` and `lt` in sub-object.

Can use IF to filter rows, not on partition number

### (WIP) Batch Put and Delete Records `POST /records/batch`

Multiple `Put` and `Delete` operations can be sent in a single request, which will result in all operations being atomic.

If any condition fails, then all operations will be aborted

## Setup

The first step is to create the following topics in Kafka:

1. `firescroll_{namespace}_mutations`
2. `firescroll_{namespace}_partitions`

### Mutations Topic

The mutations topic is used to manage all mutations (Put and Delete) that are applies to the data. It is important to set this to a partition count that is sufficient for your scale, as this cannot be changed later (nodes will refuse to start as they no longer know where data exists in). With Redpanda a partition is pretty powerful, so using are relatively high number (a few partitions per core) will cover you for a long time. For production at scale put as many topics as cores you ever intend to have in the Redpanda cluster.

For example, if you are using Redpanda it might look like:

```
rpk topic create firescroll_testns_mutations
rpk topic add-partitions firescroll_testns_mutations --num 3 # using 4 total partitions
rpk topic create firescroll_testns_partitions
```

### Partitions Topic

The partitions topic is used to record the first found topic count, and allows nodes to check against this count so that they can crash if something changes (because now we don't know where data is). While the actual partitions are checked against in real time, this serves as an extra dummy check in case you change the partition count and update the env vars (since data is not currently re-partitioned). So it's just an extra redundancy check and only ever (intentionally) writes one record, and otherwise is read on node startup.

## Running Locally

To run locally:

```
bash up.sh
```

This will start Redpanda and Minio, and set up the topics with the namespace `testns`.

From there go to `http://localhost:9001` and create a key pair in Minio to use with the bucket `testbucket` that was automatically created. 

## Configuration

For all options, see [env.go](utils/env.go). Here are some notable ones:

| Env Var                 | Description                                                                                                                                             | Type              |
|-------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------|
| `NAMESPACE`             | global, should be the same for all regions                                                                                                              | string            |
| `REPLICA_GROUP`         | per replica in a region                                                                                                                                 | string            |
| `INSTANCE_ID`           | unique to the node                                                                                                                                      | string            |
| `TOPIC_RETENTION_MS`    | configured in Kafka, but Firescroll needs to know for backup management! (WIP for warnings)                                                             | int               |
| `PARTITIONS`            | do not change this once you start consuming mutations. Firescroll will do it's best to refuse to work if it detects a change so data is not lost        | int               |
| `GOSSIP_PEERS`          | CSV of (domain/ip):port for gossip peers. If omitted then GET request for other partitions will be ignored.                                             | string (optional) |
| `GOSSIP_BROADCAST_MS`   | How frquently to broadcast gossip messages. Default `500`.                                                                                              | int               |
| `KAFKA_SEEDS`           | CSV of (domain/ip):port for the Kafka node seeds.                                                                                                       | string            |
| `ADVERTISE_ADDR`        | The address to advertise to gossip peers                                                                                                                | string            |
| `BACKUP`                | Whether to enable backups. Should choose at most 1 replica group per region. Should be same for all nodes in replica group. Set to `1` to enable.       | int               |
| `S3_RESTORE`            | Whether to restore from S3 backups. Set to `1` to enable.                                                                                               | int               |
| `AWS_ACCESS_KEY_ID`     | For S3                                                                                                                                                  | string            |
| `AWS_SECRET_ACCESS_KEY` | For S3                                                                                                                                                  | string            |
| `AWS_REGION`            | For S3. Use `us-east-1` for local Minio                                                                                                                 | string            |
| `S3_ENDPOINT`           | The HTTP endpoint for S3. Like `https://s3.us-east-1.amazonaws.com` or `https://ny3.digitaloceanspace.com`. If `http://` SSL is automatically disabled. | string            |
| `BACKUP_INTERVAL_SEC`   | How long in seconds between backups. Default 12 hours.                                                                                                  | int               |
| `DB_PATH`               | Where to store Badger DB files. Default `/var/firescroll/dbs`                                                                                           | string            |

You can also see example values used for local development in [.env.local](.env.local), and overrides for a second node in [Taskfile.yaml](Taskfile.yaml).

## (WIP) The `If` statement

`Put` and `Delete` can all take an optional `If` condition that will determine whether the operation is applied at the time that a node consumes the mutation. If in a batch, then any failing If statement will revoke the whole batch.

The `If` condition must evaluate to a boolean (`true` or `false`), and is in [expr syntax](https://github.com/antonmedv/expr).

The available top-level keys are:
1. `pk`
2. `sk`
3. `data` (the top level JSON object, e.g. `{"key": "val"}` could be checked like `data.key == "val"`)
4. `_created_at` - an internal column created when the record is first inserted, in unix ms. This is the time that the log received the mutation.
5. `_updated_at` - an internal coluimn that is updated any time the record is updated, in unix ms. This is the time that the log received the mutation.

If an `If` statement exists, the row will be loaded into the query engine.

The best way to check for whether a mutation applied is to have some random ID that you update for every put, and poll the lowest latency region to check if that change is applied. For deletes you can obviously check for the absence of the record.

### Examples:

#### Check existence of a record

If you only want to `Put` if a record does not already exist, then you can use the `If` query:

```expr
pk == null
```

This checks that the primary key does not already exist.

_Note: `null` and `nil` can be used interchangeably_

## Scaling the cluster

Nodes are always members of a single replica group, but may contain multiple partitions. The partition mapping is managed by the log (Redpanda/Kafka), but you determine the replica group on boot with the `REPLICA_GROUP={replica group name}` env var.

Each replica group will have a single copy of all partitions.

More details are available in [this section](#topic-management).

You can scale in multiple ways, depending on what dimension is the bottleneck:

1. Scaling the number of nodes within a replica group
2. Scaling the number of replica groups
3. Scaling resources for the nodes
4. Scaling the log

The suggested order of operations is to first create replicas (benefit of HA), then scale the nodes up, then create more nodes to spread out partitions. If your workload only ever reads from a single parititon at a time, you can look to add more nodes before scaling nodes up.

### Scaling the replica group

My increasing the number of nodes in a replica group, you spread out the partitions among more nodes. With fewer partitions to manage, a node will generally receive less traffic.

Simply increase the number of nodes with the same `REPLICA_GROUP` env var to scale up, or decrease to scale down. The cluster will automatically adjust and rebalance.

Note: Scaling down with only a single replica group will result in temporary downtime for the partitions that were on the terminated node. Having multiple replicas avoids unavailablity.

### Scaling the number of replica groups

Within the same region, you can also add replica groups to increase the number of replicas for partitions. By addition an addition replica group, you are adding a copy of all partitions for the namespace.

This becomes a second method for scaling read performance in the cluster, as adding more replicas means more read throughput. `READ` and `LIST` operations will be routed randomly to a known replica of a partition, so scaling the replicas provides linear scaling of read throughput.

It also increases the availability of the cluster as any replica of a partition can serve a read.

### Scaling resources for the nodes

Nodes will utilize all resources for read performance, so more cores and memory = faster performance.

### Scaling the log

If you saturate the resources of the log (Redpanda/Kafka) cluster, you may need to scale up those nodes as well. Monitor the network traffic and CPU usage to determine this.

## Topic Management

> ⚠️ **DO NOT MODIFY ANY TOPICS WITH THE PREFIX `firescroll_`**
> 
> This will break the namespace if you do not know **exactly** what you are doing.

When a namespace is created, 2 topic within Kafka are created: `firescroll_{namespace}_mutations` and `firescroll_{namespace}_partitions`.

### Mutations

The `mutations` topic is used for streaming the KV operations to the storage nodes. This should have a configured retention that is far beyond what a disaster recovery scenario might look like (e.g. 7 days).

### Partitions

The `partition` topic is used to record the number of partitions when the namespace is created. **This topic must have unlimited retention**, and only a single record is placed in (optimistically). This ensures that we can enforce that the number of partitions cannot change.

On start, a node will read from this topic and check against its configured number of partitions. If they differ for any reason, then the node will refuse to start. If a node inserts a record (due to the topic not existing or being empty), then it will verify by waiting until it can consume the first record in the topic. If a node fails to create a topic because it already exists, but there are no records yet, it will insert its partition count in. If a race condition occurs where multiple nodes start with different partition counts, only one partition count will be established, and other nodes will fail to initialize.

Since the number of partitions cannot be changed, by default it is a high value of `256`. This should cover all the way up to the most absurd scales, as you can continue to increase replicas and per-node resources.


## Backups and Snapshotting

Backups are also used as snapshots for partition-shuffling. When a partition is remapped to another node (either via scaling or recovery), the node will first restore the partition from a remote S3 backup. It will then reset the partition checkpoint to the time of the backup, and consume the log from there. This results in a faster recovery, and the topic can expire records over time, preventing unbound log growth.

Backups can be explicitly enabled with `BACKUP=true` env var. Backups should be enabled at the replica-group level, and all nodes within the same replica group should have the same `BACKUP` value. Backups are named via the partition, so distinct regions should use distinct buckets if you plan on having multiple backup replica groups.

This allows for multiple backup strategies such as:

1. A single replica group in a single region is marked for backing up, and other groups and regions point to that S3 path for restore. This results in less redundancy, but less bandwidth and storage used
2. A single replica group in each region is marked, and the local region points to that. This results in more redundancy and faster restores, but increased storage usage and bandwidth.

You should choose a strategy depending on your needs.

There are multiple env vars to use for this functionality such as:
```
BACKUP_INTERVAL_SEC
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
S3_ENDPOINT
BACKUP
AWS_REGION
S3_BUCKET
BACKUP_TIMEOUT_SEC
S3_RESTORE
```

## Recommended Redpanda/Kafka Settings

Make sure that the `group_max_session_timeout_ms` and `group_min_session_timeout_ms` range in Redpanda (Kafka uses `.` instead of `_`) allows for your `KAFKA_SESSION_MS` env var value (default `60000`). Redpanda uses a `30000` max so it will need to be adjusted.

## Architecture

See my blog post here for a more detailed look at the architecture. (LINK BLOG POST).

Briefly, this database turns the traditional DB inside-out: Rather than having multiple nodes with each their own WAL, a distributed central WAL cluster is used (Kafka/Redpanda) and nodes consume from that, materializing (truncating) to disk and backing that snapshot of the log to S3 so that they can be restored on other nodes (during partition remapping) without needing to consume the entire WAL history (this is specifically important for allowing us to have a really short retention period on the WAL!)

This allows us to decouple reads and writes, meaning that nodes in different regions can consume at their own pace. For example the latencies of servers in Japan do not affect the write performance of servers in North Virginia. This also removes the issue found in Cassandra of the possibility of entering a permanently inconsistent state between replicas of partitions, requiring repairs. It also means that nodes are not responsible for coordinating replication, meaning they can focus on serving reads fast.

It uses Kafka or Redpanda as the distributed WAL (prefer Redpanda), and Badger as the local KV db. A single node can easily serve reads in the hundreds of thousands per second.

This arch also enables arbitrary number of read replicas for increased performance, or adding more nodes to spread out the partitions.

It's also extremely easy to manage. By having a 2-tier architecture (Kafka -> Nodes) there is no complex cascading replication.

Backups to S3 are also used to aid in partition remapping and bringing new replicas online, allowing the Kafka log to be keep a low retention without losing data.