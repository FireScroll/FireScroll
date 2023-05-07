package partitions

import "errors"

type Operation string

const (
	OperationPut    Operation = "put"
	OperationGet    Operation = "get"
	OperationDelete Operation = "delete"
	OperationList   Operation = "list"
	OperationBatch  Operation = "batch"
)

var (
	ErrUnknownOperation = errors.New("unknown operation")
)
