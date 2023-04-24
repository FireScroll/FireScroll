# FanoutDB

The unkillable KV database with unlimited read throughput.

<!-- TOC -->
* [FanoutDB](#fanoutdb)
  * [Operations](#operations)
    * [PUT](#put)
    * [GET](#get)
    * [DELETE](#delete)
    * [LIST](#list)
    * [BATCH](#batch)
  * [Topic Management](#topic-management)
  * [Backups and Snapshotting](#backups-and-snapshotting)
  * [Architecture](#architecture)
    * [Storage engine](#storage-engine)
    * [Raft](#raft)
    * [Gossip](#gossip)
<!-- TOC -->

## Operations

### PUT

Can add an `IF` condition on it for conditional

Can check for existence with `pk ≠ null`

### GET

Get record(s) by their `pk` and `sk` pairs. Multiple records can be fetched at the same time.

### DELETE

Delete a single record by pk and sk, can put an IF condition on it

### LIST

Can have starts_after or ends_before to determine the direction, and prefix

Can specify a pk or a partition number. If pk then sk is the filter. If partition number then pk and sk are filters. Use `eq`, `gt` and `lt` in sub-object.

Can use IF to filter rows, not on partition number

### BATCH

Multiple `PUT` and `DELETE` operations can be sent in a single request, which will result in all operations being atomic.

If any condition fails, then all operations will be aborted.

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

> ⚠️ **DO NOT TOUCH ANY TOPICS WITH THE PREFIX `fanountdb_`**
> 
> This will break the namespace if you do not know **exactly** what you are doing.

When a namespace is created, 2 topic within Redpanda are created: `fanoutdb_{namespace}_mutations` and `fanountdb_{namespace}_partitions`.

### Mutations

The `mutations` topic is used for streaming the KV operations to the storage nodes. This should have a configured retention that is far beyond what a disaster recovery scenario might look like (e.g. 7 days).

### Partitions

The `partition` topic is used to record the number of partitions when the namespace is created. **This topic must have unlimited retention**, and only a single record is placed in (optimistically). This ensures that we can enforce that the number of partitions cannot change.

On start, a node will read from this topic and check against its configured number of partitions. If they differ for any reason, then the node will refuse to start. If a node inserts a record (due to the topic not existing or being empty), then it will verify by waiting until it can consume the first record in the topic. If a node fails to create a topic because it already exists, but there are no records yet, it will insert its partition count in. If a race condition occurs where multiple nodes start with different partition counts, only one partition count will be established, and other nodes will fail to initialize.

Since the number of partitions cannot be changed, by default it is a high value of `256`. This should cover all the way up to the most absurd scales, as you can continue to increase replicas and per-node resources.


## Backups and Snapshotting

Nodes will elect a single instance of a replica within a region to manage backups for a partition. Backups are made using litestream.io, and sent to an S3 compatible storage.

Backups are also used as snapshots for partition-shuffling. When a partition is remapped to another node (either via scaling or recovery), the node will first restore the partition from the litestream backup. It will then reset the partition checkpoint to the time of the backup, and consume the log from there. This results in a faster recovery, and the topic can expire records over time, preventing unbound log growth.

Backups can be disabled with the `BACKUPS_DISABLED=true` env var if you only want to have a single region manage backups, and other regions can pull from that single S3 bucket. This will also disable raft. By default, all regions are expected to have their own local S3 bucket for faster upload and download times, as well as increased availability.

You can find more details in the [Architecture](#architecture) section.

## Architecture

### Storage engine

SQLite is used as the underlying storage engine. Each partition is a single SQLite database.

Continuous incremental snapshots are 

### Raft

Raft is used within a local region to elect a single replica of a partition to manage backups. No messages are sent over raft, it is purely used for leader election.

### Gossip

Gossip is used within a local region to

### Mapping log topic partitions to nodes