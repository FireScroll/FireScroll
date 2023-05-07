package partitions

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/danthegoodman1/FireScroll/gologger"
	"github.com/danthegoodman1/FireScroll/syncx"
	"github.com/danthegoodman1/FireScroll/utils"
	"os"
)

var (
	logger                    = gologger.NewLogger()
	ErrPartitionAlreadyExists = errors.New("partition already exists")
	ErrRestoredDBTooOld       = errors.New("the restored database was too old")
	ErrPartitionNotFound      = errors.New("partition not found")
)

type (
	PartitionManager struct {
		Partitions syncx.Map[int32, *Partition]
		Namespace  string
	}
)

func NewPartitionManager(namespace string) (*PartitionManager, error) {
	logger.Debug().Msg("creating new partition manager")
	pm := &PartitionManager{
		Partitions: syncx.Map[int32, *Partition]{},
		Namespace:  namespace,
	}

	return pm, nil
}

func (pm *PartitionManager) GetPartitionIDs() (ids []int32) {
	pm.Partitions.Range(func(id int32, _ *Partition) bool {
		ids = append(ids, id)
		return true
	})
	return
}

// AddPartition will return the timestamp in ms of the last seen row, if 0 it is a new partition
func (pm *PartitionManager) AddPartition(id int32) (*int64, error) {
	logger.Debug().Msgf("adding partition %d", id)
	// Check if we already have it
	_, ok := pm.Partitions.Load(id)
	if ok {
		logger.Warn().Msgf("tried to add partition we already had: %d, no-op", id)
		return nil, ErrPartitionAlreadyExists
	}

	// Create the dir if we need
	_ = os.MkdirAll(utils.Env_DBPath, 0777)

	part, err := newPartition(pm.Namespace, id)
	if err != nil {
		return nil, fmt.Errorf("error in newPartition: %w", err)
	}

	//if part.LastOffset > 0 {
	//	// If too old, then throw it away
	//	diff := time.Now().Sub(time.Unix(int64(part.LastEpoch), 0))
	//	logger.Debug().Msgf("got diff of %s for partition %d", diff, id)
	//	if diff.Milliseconds() > utils.Env_TopicRetentionMS {
	//		return 0, fmt.Errorf("the restored partition %d was too old, including the remote backup it may have tried to restore, do you need to use a remote backup?: %w", id, ErrRestoredDBTooOld)
	//	}
	//}

	pm.Partitions.Store(id, part)
	logger.Debug().Msgf("added partition %d", id)
	return part.LastOffset, nil
}

func (pm *PartitionManager) RemovePartition(id int32) error {
	logger.Debug().Msgf("removing partition %d", id)

	// check if we have it
	part, ok := pm.Partitions.LoadAndDelete(id)
	if !ok {
		logger.Warn().Msgf("tried to remove a partition we did not have: %d, will still try to remove from disk", id)
	} else {
		err := part.Shutdown()
		if err != nil {
			return fmt.Errorf("error in part.Shutdown(): %w", err)
		}
	}

	err := part.delete()
	if err != nil {
		return fmt.Errorf("error in os.Remove: %w", err)
	}

	logger.Debug().Msgf("removed partition %d at %s", id, getPartitionPath(id))

	return nil
}

func (pm *PartitionManager) Shutdown() error {
	logger.Info().Msg("shutting down partition manager")
	var parts []*Partition
	pm.Partitions.Range(func(_ int32, part *Partition) bool {
		parts = append(parts, part)
		return true
	})
	for _, part := range parts {
		err := part.Shutdown()
		if err != nil {
			return fmt.Errorf("error shutting down partition %d: %w", part.ID, err)
		}
	}
	return nil
}

func (pm *PartitionManager) HandleMutation(partitionID int32, mutationBytes []byte, offset int64) error {
	var mutation RecordMutation
	err := json.Unmarshal(mutationBytes, &mutation)
	if err != nil {
		return fmt.Errorf("error in json.Unmarshal: %w", err)
	}
	part, exists := pm.Partitions.Load(partitionID)
	if !exists {
		return ErrPartitionNotFound
	}
	err = part.HandleMutation(mutation, offset)
	if err != nil {
		return fmt.Errorf("error in part.HandleMutation: %w", err)
	}
	return nil
}

func (pm *PartitionManager) ReadRecords(partMap map[int32][]RecordKey) (records []Record, err error) {
	for partition, keys := range partMap {
		part, exists := pm.Partitions.Load(partition)
		if !exists {
			continue
		}

		res, err := part.ReadRecords(keys)
		if err != nil {
			err = fmt.Errorf("error in ReadRecord for part %d: %w", part, err)
		}
		records = append(records, res...)
	}
	return
}

func (pm *PartitionManager) ListRecords(partition int32, pk, skAfter string, limit int64) (records []Record, err error) {
	part, exists := pm.Partitions.Load(partition)
	if !exists {
		return nil, ErrPartitionNotFound
	}

	res, err := part.ListRecords(pk, skAfter, limit)
	if err != nil {
		err = fmt.Errorf("error in ListRecords for part %d: %w", part, err)
	}
	records = append(records, res...)
	return
}
