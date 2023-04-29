docker exec -it redpanda-0 rpk topic create firescroll_testns_mutations
docker exec -it redpanda-0 rpk topic add-partitions firescroll_testns_mutations --num 1
docker exec -it redpanda-0 rpk topic create firescroll_testns_partitions