package partitions

type Operation string

const (
	OperationPut    Operation = "put"
	OperationGet    Operation = "get"
	OperationDelete Operation = "delete"
	OperationBatch  Operation = "batch"
)
