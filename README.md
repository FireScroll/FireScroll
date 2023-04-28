# Firescroll 

The unkillable KV database with unlimited read throughput. 

Perfect for configuration management at scale where you want global low-latency reads without caching or cold first reads.

<!-- TOC -->
* [Firescroll](#firescroll)
  * [Quick Note](#quick-note)
  * [API](#api)
    * [Put Record(s) `POST /records/put`](#put-records-post-recordsput)
    * [Get Record(s) `POST /records/get`](#get-records-post-recordsget)
    * [Delete Record(s) `POST /records/delete`](#delete-records-post-recordsdelete)
    * [List Records `POST /records/list`](#list-records-post-recordslist)
    * [Batch Put and Delete Records `POST /records/batch`](#batch-put-and-delete-records-post-recordsbatch)
  * [Configuration](#configuration)
  * [The `If` statement](#the-if-statement)
    * [Examples:](#examples)
      * [Check existence of a record](#check-existence-of-a-record)
  * [Scaling read throughput](#scaling-read-throughput)
    * [Scaling the replica group](#scaling-the-replica-group)
    * [Scaling the number of replica groups](#scaling-the-number-of-replica-groups)
    * [Scaling resources for the nodes](#scaling-resources-for-the-nodes)
    * [Scaling the log](#scaling-the-log)
  * [Scaling writes throughput (the log)](#scaling-writes-throughput-the-log)
  * [Topic Management](#topic-management)
    * [Mutations](#mutations)
    * [Partitions](#partitions)
  * [Backups and Snapshotting](#backups-and-snapshotting)
  * [Recommended Redpanda/Kafka Settings](#recommended-redpandakafka-settings)
  * [Architecture](#architecture)
    * [Storage engine](#storage-engine)
    * [Tables](#tables)
    * [Gossip](#gossip)
    * [Mapping log topic partitions to nodes](#mapping-log-topic-partitions-to-nodes)
    * [Partition initialization process](#partition-initialization-process)
  * [Performance and Benchmarking](#performance-and-benchmarking)
<!-- TOC -->

## Quick Note

Redpanda and Kafka are used pretty interchangeably here. They mean the same thing, and any differences (e.g. configuration properties) will be noted.

## API

The API is HTTP/1.1 & 2 compatible, with all operations as a `POST` request and JSON bodies.

### Put Record(s) `POST /records/put`

Can add an `If` condition on it for conditional

Can check for existence with `pk ≠ null`

### Get Record(s) `POST /records/get`

Get record(s) by their `pk` and `sk` pairs. Multiple records can be fetched at the same time.

### Delete Record(s) `POST /records/delete`

Delete record(s) by pk and sk, can put an IF condition on it

### List Records `POST /records/list`

Can have starts_after or ends_before to determine the direction, and prefix

Can specify a pk or a partition number. If pk then sk is the filter. If partition number then pk and sk are filters. Use `eq`, `gt` and `lt` in sub-object.

Can use IF to filter rows, not on partition number

### Batch Put and Delete Records `POST /records/batch`

Multiple `Put` and `Delete` operations can be sent in a single request, which will result in all operations being atomic.

If any condition fails, then all operations will be aborted

## Configuration

All environment variables are imported and validated in files called `env.go`. They exist in the various packages based on scope. You can also visit the test files to see what needs to be specified.

| Env Var              | Description                                                                                                 | Type              |
|----------------------|-------------------------------------------------------------------------------------------------------------|-------------------|
| `NAMESPACE`          | global, should be the same for all regions                                                                  | string            |
| `REPLICA_GROUP`      | per replica in a region                                                                                     | string            |
| `INSTANCE_ID`        | unique to the node                                                                                          | string            |
| `TOPIC_RETENTION_MS` | configured in Kafka, but Firescroll needs to know for backup management!                                    | int               |
| `PARTITIONS`         | default 256, but must be the same as Kafka, and cannot change!                                              | int (optional)    |
| `PEERS`              | CSV of (domain/ip):port for gossip peers. If omitted then GET request for other partitions will be ignored. | string (optional) |



## The `If` statement

`Put` and `Delete` can all take an optional `If` condition that will determine whether the operation is applied at the time that a node consumes the mutation.

The `If` condition must evaluate to a boolean (`true` or `false`), and is in [expr syntax](https://github.com/antonmedv/expr).

The available top-level keys are:
1. `pk`
2. `sk`
3. `data` (the top level JSON object, e.g. `{"key": "val"}` could be checked like `data.key == "val"`)
4. `_created_at` - an internal column created when the record is first inserted, in unix ms. This is the time that the log received the mutation.
5. `_updated_at` - an internal coluimn that is updated any time the record is updated, in unix ms. This is the time that the log received the mutation.

If an `If` statement exists, the row will be loaded into the query engine.

### Examples:

#### Check existence of a record

If you only want to `Put` if a record does not already exist, then you can use the `If` query:

```expr
pk == null
```

This checks that the primary key does not already exist.

_Note: `null` and `nil` can be used interchangeably_

## Scaling read throughput

Nodes are always members of a single replica group, but may contain multiple partitions. The partition mapping is managed by the log (Redpanda/Kafka), but you determine the replica group on boot with the `REPLICA_GROUP={replica group name}` env var.

Each replica group will have a single copy of a partition.

More details are available in [this section](#mapping-log-topic-partitions-to-nodes).

You can scale a region in one of three ways:

1. Scaling the number of nodes within a replica group
2. Scaling the number of replica groups
3. Scaling resources for the nodes
4. Scaling the log

### Scaling the replica group

My increasing the number of nodes in a replica group, you spread out the partitions among more nodes. With fewer partitions to manage, a node will generally receive less traffic.

Simply increase the number of nodes with the same `REPLICA_GROUP` env var to scale up, or decrease to scale down. The cluster will automatically adjust and rebalance.

Note: Scaling down with only a single replica group will result in temporary downtime for the partitions that were on the terminated node.

### Scaling the number of replica groups

Within the same region, you can also add replica groups to increase the number of replicas for partitions. By addition an addition replica group, you are adding a copy of all partitions for the namespace.

This becomes a second method for scaling read performance in the cluster, as adding more replicas means more read throughput. `READ` and `LIST` operations will be routed randomly to a known replica of a partition, so scaling the replicas provides linear scaling of read throughput.

### Scaling resources for the nodes

Nodes will utilize all resources for read performance, so more cores and memory = faster performance.

### Scaling the log

If you saturate the resources of the log (Redpanda/Kafka) cluster, you may need to scale up those nodes as well. Monitor the network traffic and CPU usage to determine this.

## Scaling writes throughput (the log)

You can increase write performance by giving your log cluster more resources. Monitor network and CPU usage to determine when this is right for you.

All writes will pass through the log cluster.

## Topic Management

> ⚠️ **DO NOT TOUCH ANY TOPICS WITH THE PREFIX `firescroll_`**
> 
> This will break the namespace if you do not know **exactly** what you are doing.

When a namespace is created, 2 topic within Redpanda are created: `firescroll_{namespace}_mutations` and `firescroll_{namespace}_partitions`.

### Mutations

The `mutations` topic is used for streaming the KV operations to the storage nodes. This should have a configured retention that is far beyond what a disaster recovery scenario might look like (e.g. 7 days).

### Partitions

The `partition` topic is used to record the number of partitions when the namespace is created. **This topic must have unlimited retention**, and only a single record is placed in (optimistically). This ensures that we can enforce that the number of partitions cannot change.

On start, a node will read from this topic and check against its configured number of partitions. If they differ for any reason, then the node will refuse to start. If a node inserts a record (due to the topic not existing or being empty), then it will verify by waiting until it can consume the first record in the topic. If a node fails to create a topic because it already exists, but there are no records yet, it will insert its partition count in. If a race condition occurs where multiple nodes start with different partition counts, only one partition count will be established, and other nodes will fail to initialize.

Since the number of partitions cannot be changed, by default it is a high value of `256`. This should cover all the way up to the most absurd scales, as you can continue to increase replicas and per-node resources.


## Backups and Snapshotting

Nodes will elect a single instance of a replica within a region to manage backups for a partition. Backups are made using litestream.io, and sent to an S3 compatible storage.

Backups are also used as snapshots for partition-shuffling. When a partition is remapped to another node (either via scaling or recovery), the node will first restore the partition from the litestream backup. It will then reset the partition checkpoint to the time of the backup, and consume the log from there. This results in a faster recovery, and the topic can expire records over time, preventing unbound log growth.

Backups can be explicitly enabled with `BACKUP=true` env var. Backups should be enabled at the replica-group level, and all nodes within the same replica group should have the same `BACKUP` value. Backups are named via the `REPLICA_GROUP` env var, so distinct regions should use distinct buckets if you plan on having multiple backup replica groups.

According to litestream [colliding backups will not corrupt, but are also just a bad idea.](https://litestream.io/how-it-works/#:~:text=This%20approach%20also%20has%20the%20benefit%20that%20two%20servers%20that%20accidentally%20share%20the%20same%20replica%20destination%20will%20not%20overwrite%20each%20other%E2%80%99s%20data.%20However%2C%20note%20that%20it%20is%20not%20recommended%20to%20replicate%20two%20databases%20to%20the%20same%20exact%20replica%20path.)

This allows for multiple backup strategies such as:

1. A single replica group in a single region is marked for backing up, and other groups and regions point to that S3 path for restore. This results in less redundancy, but less bandwidth and storage used
2. A single replica group in each region is marked, and the local region points to that. This results in more redundancy and faster restores, but increased storage usage and bandwidth.

You should choose a strategy depending on your needs.

## Recommended Redpanda/Kafka Settings

Make sure that the `group_max_session_timeout_ms` and `group_min_session_timeout_ms` range in Redpanda (Kafka uses `.` instead of `_`) allows for your `KAFKA_SESSION_MS` env var value (default `60000`). Redpanda uses a `30000` max so it will need to be adjusted.

TODO

Set session timeout high enough for restart of nodes.

## Architecture

TL,DR: Firescroll is different by delegating the distributed nature to the log, and playing dumb about materializing the log to snapshots. It them provides a nice API for conditional querying.

Even more TL,DR: it's a fancy wrapper around a log for fanning-out reads.

By centralizing the writes to a single log cluster we can ensure low-latency durability of mutations, while downstream nodes pull those mutations at their own pace.

Like other eventually consistent databases, this means that read-after-write is not guaranteed is determined by how quickly the mutation is propagated. This use case is acceptable for most KV requirements like serving configurations (DNS records, feature flags, etc.)

Firescroll optimizes for low-latency high-throughput reads from all points of presence, with the worst case performance being one network hop in the local cluster to serve the read.

We are effectively decoupling the local WAL and the compaction to pages as would be found in a traditional DB.

This also means that no caching is needed, since updates are propagated down to the nodes as fast as they can be consumed.

### Storage engine

SQLite is used as the underlying storage engine. Each partition is a single SQLite database.

Continuous incremental snapshots are covered in the [Backups and Snapshotting section](#backups-and-snapshotting), and used for both disaster recovery and partition remapping.

### Tables

2 tables are created:

1. For storing the materialized mutation log (where your data is stored): `log_data`
2. For keeping track of the latest value from Kafka (using the Kafka write timestamp): `offsets`

### Gossip

Gossip is used within a local region to

### Mapping log topic partitions to nodes

In log terms, nodes always belong to a single topic and a single consumer group.

A single node could (and most likely will) be responsible for multiple partitions of a topic. This is determined by strategy used by the log cluster. Nodes simply react to the partitions they are mapped to, and respond accordingly.

When a node is unmapped from a partition, it will first stop the backup process (to ensure no in-progress backups are lost). It will then delete the local DB from disk and reclaim the space. It is important to ensure that at least one replica group performs backups, or data will be permanently lost once the retention period of the topic is passed.

### Partition initialization process

When a partition is initialized (node starting) it follows this order:

1. Will use the local DB if it exists and has a timestamp >= snapshot timestamp. Continue consuming log from latest mutation in local DB.
2. Will use the snapshot in S3 if it is newer than local DB or local DB does not exist. Continue consuming log from latest mutation in snapshot.
3. Will start consuming from Kafka, and throw warning log that it is creating a local DB.

## Performance and Benchmarking

TODO