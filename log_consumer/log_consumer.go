package log_consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/danthegoodman1/FanoutDB/gologger"
	"github.com/danthegoodman1/FanoutDB/syncx"
	"github.com/danthegoodman1/FanoutDB/utils"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
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
		ManagedPartitions syncx.Map[int32, *Partition]
		Client            *kgo.Client
		AdminClient       *kadm.Client
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

func NewLogConsumer(ctx context.Context, namespace, consumerGroup string, seeds []string, sessionMS int64) (*LogConsumer, error) {
	consumer := &LogConsumer{
		ConsumerGroup:     consumerGroup,
		Namespace:         namespace,
		ManagedPartitions: syncx.Map[int32, *Partition]{},
		Client:            nil,
	}
	mutationTopic := formatMutationLog(namespace)
	partitionTopic := formatPartitionTopic(namespace)
	logger.Debug().Msgf("using mutation log %s and partition log %s", mutationTopic, partitionTopic)
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(seeds...),
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

	// Get the actual partitions
	resp, err := consumer.AdminClient.DescribeGroups(ctx, consumerGroup)
	if err != nil {
		return nil, fmt.Errorf("error in AdminClient.DescribeGroups: %w", err)
	}
	fmt.Println(resp.AssignedPartitions(), resp.Names(), resp.Sorted())

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
