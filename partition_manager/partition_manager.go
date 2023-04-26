package partition_manager

import (
	"context"
	"github.com/danthegoodman1/FanoutDB/gologger"
	"github.com/danthegoodman1/FanoutDB/syncx"
)

var (
	logger = gologger.NewLogger()
)

type (
	PartitionManager struct {
		Partitions syncx.Map[int32, string]
	}
)

func NewPartitionManager() (*PartitionManager, error) {
	logger.Debug().Msg("creating new partition manager")
	pm := &PartitionManager{
		Partitions: syncx.Map[int32, string]{},
	}

	return pm, nil
}

func (pm *PartitionManager) Shutdown(ctx context.Context) error {
	logger.Info().Msg("shutting down partition manager")
	return nil
}

func (pm *PartitionManager) GetPartitionIDs() (ids []int32) {
	pm.Partitions.Range(func(id int32, _ string) bool {
		ids = append(ids, id)
		return true
	})
	return
}
