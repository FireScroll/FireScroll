package log_consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/danthegoodman1/FanoutDB/gologger"
	"github.com/danthegoodman1/FanoutDB/partition_manager"
	"github.com/danthegoodman1/FanoutDB/utils"
	"github.com/samber/lo"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"log"
	"math/rand"
	"time"
)

var (
	ErrPartitionMismatch = errors.New("partition mismatch")
	ErrNameBadChars      = errors.New("bad namespace name, cannot have underscores in name")
	ErrClientClosed      = errors.New("client is closed")

	logger = gologger.NewLogger()
)

type (
	// LogConsumer is a single consumer of log, belonging to a single consumer group.
	// It also manages gossip participation
	LogConsumer struct {
		ConsumerGroup, Namespace string

		// ManagedPartitions are the partitions that are managed on this node
		PartitionManager *partition_manager.PartitionManager
		Client           *kgo.Client
		AdminClient      *kadm.Client
		AdminTicker      *time.Ticker
		NumPartitions    int64
	}

	PartitionMessage struct {
		NumPartitions int64
	}

	PartitionError struct {
		Topic     string
		Partition int32
		Err       error
	}
)

func NewLogConsumer(ctx context.Context, namespace, consumerGroup string, seeds []string, sessionMS int64, partMan *partition_manager.PartitionManager) (*LogConsumer, error) {
	consumer := &LogConsumer{
		ConsumerGroup:    consumerGroup,
		Namespace:        namespace,
		NumPartitions:    utils.Env_NumPartitions,
		PartitionManager: partMan,
	}
	mutationTopic := formatMutationLog(namespace)
	partitionTopic := formatPartitionTopic(namespace)
	logger.Debug().Msgf("using mutation log %s and partition log %s", mutationTopic, partitionTopic)
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.ClientID("a"),
		kgo.InstanceID(utils.Env_InstanceID),
		kgo.ConsumerGroup(consumerGroup),
		kgo.ConsumeTopics(mutationTopic),
		kgo.SessionTimeout(time.Millisecond*time.Duration(sessionMS)),
		//kgo.DisableAutoCommit(), // TODO: See comment, need listeners
	)
	if err != nil {
		return nil, fmt.Errorf("error in kgo.NewClient (mutations): %w", err)
	}
	consumer.Client = cl
	consumer.AdminClient = kadm.NewClient(cl)
	consumer.AdminTicker = time.NewTicker(time.Second * 2)

	// Verify the partitions
	// First we should try to read from it
	partitionsClient, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
		kgo.ConsumeTopics(partitionTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), // always consume the first record
	)
	if err != nil {
		return nil, fmt.Errorf("error in kgo.NewClient (partitions): %w", err)
	}

	logger.Debug().Msg("sleeping to let consumer group to register")
	time.Sleep(time.Second)

	go func() {
		// Get the actual partitions
		for {
			_, open := <-consumer.AdminTicker.C
			if !open {
				logger.Debug().Msg("ticker channel closed, stopping")
				break
			}
			resp, err := consumer.AdminClient.DescribeGroups(context.Background(), consumerGroup)
			if err != nil {
				logger.Error().Err(err).Msg("error describing groups")
				continue
			}
			memberID, _ := consumer.Client.GroupMetadata()
			if len(resp.Sorted()) == 0 {
				logger.Warn().Msg("did not get any groups yet for group metadata")
				continue
			}
			member, ok := lo.Find(resp.Sorted()[0].Members, func(item kadm.DescribedGroupMember) bool {
				return item.MemberID == memberID
			})
			if !ok {
				logger.Warn().Msg("did not find myself in group metadata, cannot continue with partition mappings until I know what partitions I have")
				continue
			}
			//spew.Dump(resp.Sorted())
			log.Println("I am", member.MemberID)
			var partitionCount int64 = 0
			resp.AssignedPartitions().Each(func(t string, p int32) {
				partitionCount++
			})
			//currentVal := atomic.LoadInt64(&consumer.NumPartitions)
			//if currentVal != partitionCount {
			//	// We can't continue now
			//	logger.Fatal().Msgf("number of partitions changed in Kafka topic! I have %d, but topic has %d aborting so it's not longer safe!!!!!", consumer.NumPartitions, partitionCount)
			//	atomic.StoreInt64(&consumer.NumPartitions, partitionCount)
			//}
			log.Println("number of partitions:", partitionCount, open)
		}
	}()

	records, err := pollRecords(ctx, partitionsClient)

	if errors.Is(err, context.DeadlineExceeded) || len(records) == 0 {
		logger.Info().Msg("did not find existing records topic, creating")
		// Create the records and poll again
		pm := PartitionMessage{NumPartitions: utils.Env_NumPartitions}
		pmBytes, err := json.Marshal(pm)
		if err != nil {
			return nil, fmt.Errorf("error in json.Marshal of partitions message: %w", err)
		}
		pCtx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		// Publish
		if err := partitionsClient.ProduceSync(pCtx, &kgo.Record{Topic: partitionTopic, Value: pmBytes}).FirstErr(); err != nil {
			return nil, fmt.Errorf("error in partitionsClient.ProduceSync: %w", err)
		}
		logger.Debug().Msg("produced partition message, checking")
		records, err = pollRecords(ctx, partitionsClient)
		if err != nil {
			return nil, fmt.Errorf("error in pollRecords (after publish): %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("error in pollRecords: %w", err)
	}

	if len(records) == 0 {
		logger.Fatal().Msg("partition message not found after producing then checking, exiting")
	}

	var pm PartitionMessage
	err = json.Unmarshal(records[0].Value, &pm)
	if err != nil {
		return nil, fmt.Errorf("error in json.Unmarshal: %w", err)
	}

	if pm.NumPartitions != utils.Env_NumPartitions {
		return nil, ErrPartitionMismatch
	}
	logger.Debug().Msgf("got matching partitions %d", pm.NumPartitions)

	return consumer, nil
}

func pollRecords(ctx context.Context, client *kgo.Client) ([]*kgo.Record, error) {
	pCtx, cancel := context.WithTimeout(ctx, time.Millisecond*100*time.Duration(int64(rand.Intn(3)+2)))
	defer cancel()
	logger.Debug().Msg("polling for records")
	fetches := client.PollFetches(pCtx)
	if fetches.IsClientClosed() {
		return nil, ErrClientClosed
	}

	var errs []PartitionError
	fetches.EachError(func(topic string, partition int32, err error) {
		errs = append(errs, PartitionError{
			Topic:     topic,
			Partition: partition,
			Err:       err,
		})
	})
	if len(errs) > 0 {
		return nil, errs[0].Err
	}

	var records []*kgo.Record
	fetches.EachRecord(func(record *kgo.Record) {
		records = append(records, record)
	})
	return records, nil
}

func formatMutationLog(namespace string) string {
	// TODO: check for underscores and panic
	return fmt.Sprintf("fanoutdb_%s_mutations", namespace)
}

func formatPartitionTopic(namespace string) string {
	// TODO: check for underscores and panic
	return fmt.Sprintf("fanoutdb_%s_partitions", namespace)
}

func (lc *LogConsumer) Shutdown() error {
	logger.Info().Msg("shutting down log consumer")
	lc.AdminTicker.Stop()
	lc.AdminClient.Close()
	lc.Client.CloseAllowingRebalance() // TODO: Maybe we want to manually mark something as going away if we are killing like this?
	return nil
}
