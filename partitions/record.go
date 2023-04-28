package partitions

import (
	"encoding/json"
	"time"
)

type RecordKey struct {
	Pk string `validate:"require"`
	Sk string `validate:"require"`
}

type StoredRecord struct {
	Data map[string]any

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (sr StoredRecord) Record(pk, sk string) Record {
	return Record{
		Pk:        pk,
		Sk:        sk,
		Data:      sr.Data,
		CreatedAt: sr.CreatedAt,
		UpdatedAt: sr.UpdatedAt,
	}
}

// Serialize will serialize a record, panicking on failure
func (sr StoredRecord) Serialize() []byte {
	b, err := json.Marshal(sr)
	if err != nil {
		panic(err)
	}
	return b
}

type Record struct {
	Pk   string
	Sk   string
	Data map[string]any

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (r Record) StoredRecord() StoredRecord {
	return StoredRecord{
		Data:      r.Data,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

type RecordMutation struct {
	Pk       string `validate:"require"`
	Sk       string `validate:"require"`
	Data     *map[string]any
	Mutation Operation
}
