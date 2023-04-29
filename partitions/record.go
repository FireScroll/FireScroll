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

func (sr StoredRecord) MustSerialize() []byte {
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

func (r Record) WithErr(err error) RecordWithError {
	rwe := RecordWithError{
		Pk:        r.Pk,
		Sk:        r.Sk,
		Data:      r.Data,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if err != nil {
		rwe.Error = err.Error()
	}
	return rwe
}

type RecordMutation struct {
	Pk       string `validate:"require"`
	Sk       string `validate:"require"`
	Data     *map[string]any
	TsMs     int64
	Mutation Operation
}

type RecordWithError struct {
	Pk   string
	Sk   string
	Data map[string]any `json:",omitempty"`

	CreatedAt time.Time `json:",omitempty"`
	UpdatedAt time.Time `json:",omitempty"`
	Error     string    `json:",omitempty"`
}
