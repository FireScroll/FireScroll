docker exec -it redpanda-0 rpk topic create fanoutdb_testns_mutations
docker exec -it redpanda-0 rpk topic add-partitions fanoutdb_testns_mutations --num 3
docker exec -it redpanda-0 rpk topic create fanoutdb_testns_partitions