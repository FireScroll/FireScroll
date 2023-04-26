package partitions

import (
	"errors"
	"fmt"
	"github.com/danthegoodman1/FanoutDB/gologger"
	"github.com/danthegoodman1/FanoutDB/syncx"
	"github.com/danthegoodman1/FanoutDB/utils"
	"os"
)

var (
	logger                    = gologger.NewLogger()
	ErrPartitionAlreadyExists = errors.New("partition already exists")
	ErrRestoredDBTooOld       = errors.New("the restored database was too old")
)

type (
	PartitionManager struct {
		Partitions syncx.Map[int32, *Partition]
	}
)

func NewPartitionManager() (*PartitionManager, error) {
	logger.Debug().Msg("creating new partition manager")
	pm := &PartitionManager{
		Partitions: syncx.Map[int32, *Partition]{},
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
func (pm *PartitionManager) AddPartition(id int32) (int64, error) {
	logger.Debug().Msgf("adding partition %d", id)
	// Check if we already have it
	_, ok := pm.Partitions.Load(id)
	if ok {
		logger.Warn().Msgf("tried to add partition we already had: %d, no-op", id)
		return 0, ErrPartitionAlreadyExists
	}

	// Create the dir if we need
	os.MkdirAll(utils.Env_DBPath, 0777)

	part, err := newPartition(id)
	if err != nil {
		return 0, fmt.Errorf("error in newPartition: %w", err)
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

	err := os.Remove(getPartitionPath(id))
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

func (pm *PartitionManager) HandleMutation(partitionID int32, mutationBytes []byte) error {
	// TODO: Remove log line
	logger.Debug().Msg("got handle mutation")
	return nil
}
