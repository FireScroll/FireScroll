package log_consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/FireScroll/FireScroll/gologger"
	"github.com/FireScroll/FireScroll/internal"
	"github.com/FireScroll/FireScroll/partitions"
	"github.com/FireScroll/FireScroll/utils"
	"github.com/samber/lo"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"github.com/twmb/tlscfg"
	"golang.org/x/sync/errgroup"
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
		ConsumerGroup, Namespace, MutationTopic string

		// ManagedPartitions are the partitions that are managed on this node
		PartitionManager *partitions.PartitionManager
		Client           *kgo.Client
		AdminClient      *kadm.Client
		AdminTicker      *time.Ticker
		NumPartitions    int64
		Ready            bool

		shuttingDown bool
		closeChan    chan struct{}
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

func NewLogConsumer(ctx context.Context, namespace, consumerGroup string, seeds []string, sessionMS int64, partMan *partitions.PartitionManager) (*LogConsumer, error) {
	consumer := &LogConsumer{
		ConsumerGroup:    consumerGroup,
		Namespace:        namespace,
		NumPartitions:    utils.Env_NumPartitions,
		PartitionManager: partMan,
		MutationTopic:    formatMutationTopic(namespace),
		closeChan:        make(chan struct{}, 1),
	}
	partitionTopic := formatPartitionTopic(namespace)
	logger.Debug().Msgf("using mutation log %s and partition log %s for seeds %+v", consumer.MutationTopic, partitionTopic, seeds)
	opts := []kgo.Opt{
		kgo.SeedBrokers(seeds...),
		kgo.ClientID("firescroll"),
		kgo.InstanceID(utils.Env_InstanceID),
		kgo.ConsumerGroup(consumerGroup),
		kgo.ConsumeTopics(consumer.MutationTopic),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)), // force murmur2, same as in utils
		kgo.SessionTimeout(time.Millisecond * time.Duration(sessionMS)),
		//kgo.DisableAutoCommit(), // TODO: See comment, need listeners
	}
	if utils.Env_KafkaUsername != "" && utils.Env_KafkaPassword != "" {
		logger.Debug().Msg("using kafka auth")
		opts = append(opts, kgo.SASL(scram.Auth{
			User: utils.Env_KafkaUsername,
			Pass: utils.Env_KafkaPassword,
		}.AsSha256Mechanism()))
	}
	if utils.Env_KafkaTLS {
		logger.Debug().Msg("using kafka TLS")
		tlsCfg, err := tlscfg.New(
			tlscfg.MaybeWithDiskCA(utils.Env_KafkaTLSCAPath, tlscfg.ForClient),
		)
		if err != nil {
			return nil, fmt.Errorf("error in kgo.NewClient (mutations.tls): %w", err)
		}
		opts = append(opts, kgo.DialTLSConfig(tlsCfg))
	}
	cl, err := kgo.NewClient(
		opts...,
	)
	if err != nil {
		return nil, fmt.Errorf("error in kgo.NewClient (mutations): %w", err)
	}
	consumer.Client = cl
	consumer.AdminClient = kadm.NewClient(cl)
	consumer.AdminTicker = time.NewTicker(time.Second * 2)

	// Verify the partitions
	// First we should try to read from it
	partOpts := []kgo.Opt{
		kgo.SeedBrokers(seeds...),
		kgo.ConsumeTopics(partitionTopic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), // always consume the first record
	}
	if utils.Env_KafkaUsername != "" && utils.Env_KafkaPassword != "" {
		partOpts = append(partOpts, kgo.SASL(scram.Auth{
			User: utils.Env_KafkaUsername,
			Pass: utils.Env_KafkaPassword,
		}.AsSha256Mechanism()))
	}
	if utils.Env_KafkaTLS {
		logger.Debug().Msg("using kafka TLS")
		tlsCfg, err := tlscfg.New(
			tlscfg.MaybeWithDiskCA(utils.Env_KafkaTLSCAPath, tlscfg.ForClient),
		)
		if err != nil {
			return nil, fmt.Errorf("error in kgo.NewClient (partitions.tls): %w", err)
		}
		partOpts = append(partOpts, kgo.DialTLSConfig(tlsCfg))
	}
	partitionsClient, err := kgo.NewClient(
		partOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf("error in kgo.NewClient (partitions): %w", err)
	}

	logger.Debug().Msg("sleeping to let consumer group to register")
	time.Sleep(time.Second)

	go consumer.pollTopicInfo()
	go consumer.launchPollRecordLoop()

	records, err := simplePollRecords(ctx, partitionsClient)

	if errors.Is(err, context.DeadlineExceeded) || len(records) == 0 {
		logger.Info().Msg("did not find existing partitions topic record, creating")
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
		records, err = simplePollRecords(ctx, partitionsClient)
		if err != nil {
			return nil, fmt.Errorf("error in simplePollRecords (after publish): %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("error in simplePollRecords: %w", err)
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

func simplePollRecords(ctx context.Context, client *kgo.Client) ([]*kgo.Record, error) {
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

func formatMutationTopic(namespace string) string {
	return fmt.Sprintf("firescroll_%s_mutations", namespace)
}

func formatPartitionTopic(namespace string) string {
	return fmt.Sprintf("firescroll_%s_partitions", namespace)
}

func (lc *LogConsumer) Shutdown() error {
	logger.Info().Msg("shutting down log consumer")
	lc.shuttingDown = true
	lc.AdminTicker.Stop()
	lc.closeChan <- struct{}{}
	lc.AdminClient.Close()
	lc.Client.CloseAllowingRebalance() // TODO: Maybe we want to manually mark something as going away if we are killing like this?
	return nil
}

func (consumer *LogConsumer) pollTopicInfo() {
	// Get the actual partitions
	for {
		select {
		case <-consumer.AdminTicker.C:
			consumer.topicInfoLoop()
		case <-consumer.closeChan:
			logger.Debug().Msg("poll topic info received on close chan, exiting")
			return
		}
	}
}

func (consumer *LogConsumer) topicInfoLoop() {
	resp, err := consumer.AdminClient.DescribeGroups(context.Background(), consumer.ConsumerGroup)
	if err != nil {
		logger.Error().Err(err).Msg("error describing groups")
		return
	}
	memberID, _ := consumer.Client.GroupMetadata()
	if len(resp.Sorted()) == 0 {
		logger.Warn().Msg("did not get any groups yet for group metadata")
		return
	}
	member, ok := lo.Find(resp.Sorted()[0].Members, func(item kadm.DescribedGroupMember) bool {
		return item.MemberID == memberID
	})
	if !ok {
		logger.Debug().Interface("resp", resp).Msg("Got admin describe groups response")
		logger.Warn().Msg("did not find myself in group metadata, cannot continue with partition mappings until I know what partitions I have")
		return
	}

	var partitionCount int64 = 0
	resp.AssignedPartitions().Each(func(t string, p int32) {
		partitionCount++
	})

	// TODO: Add topic change abort back in
	//currentVal := atomic.LoadInt64(&consumer.NumPartitions)
	//if currentVal != partitionCount {
	//	// We can't continue now
	//	logger.Fatal().Msgf("number of partitions changed in Kafka topic! I have %d, but topic has %d aborting so it's not longer safe!!!!!", consumer.NumPartitions, partitionCount)
	//	atomic.StoreInt64(&consumer.NumPartitions, partitionCount)
	//}

	assigned, _ := member.Assigned.AsConsumer()
	if len(assigned.Topics) == 0 {
		consumer.Ready = false
		logger.Warn().Interface("assigned", assigned).Msg("did not find any assigned topics, can't make changes")
		return
	}
	myPartitions := assigned.Topics[0].Partitions
	news, gones := lo.Difference(myPartitions, consumer.PartitionManager.GetPartitionIDs())
	logger.Debug().Msgf("total partitions (%d),  my partitions (%d)", partitionCount, len(myPartitions))
	if len(news) > 0 {
		logger.Info().Msgf("got new partitions: %+v", news)
		resetPartitions := make([]struct {
			ID     int32
			Offset *int64
		}, len(news))
		logger.Debug().Msgf("pausing partitions %+v", news)
		mutationTopic := formatMutationTopic(consumer.Namespace)
		consumer.Client.PauseFetchPartitions(map[string][]int32{
			mutationTopic: news,
		})
		g := errgroup.Group{}
		for i, np := range news {
			// variable re-use protection
			newPart := np
			ind := i
			g.Go(func() error {
				offset, err := consumer.PartitionManager.AddPartition(newPart)
				if err != nil {
					return err
				}
				if offset == nil {
					logger.Info().Msgf("new partition: %d", newPart)
				}
				resetPartitions[ind] = struct {
					ID     int32
					Offset *int64
				}{ID: newPart, Offset: offset}
				return nil
			})
		}
		err := g.Wait()
		if err != nil {
			logger.Fatal().Err(err).Msg("error in adding a partition")
		}
		// Reset partition offsets
		offsetMap := map[int32]kgo.EpochOffset{}
		for _, resetPart := range resetPartitions {
			var resetOffset int64 = 0
			if resetPart.Offset != nil {
				resetOffset = (*resetPart.Offset) + 1
			}
			logger.Debug().Msgf("resuming partition %d at offset %d", resetPart.ID, resetOffset)
			offsetMap[resetPart.ID] = kgo.EpochOffset{
				Offset: resetOffset,
			}
		}
		consumer.Client.SetOffsets(map[string]map[int32]kgo.EpochOffset{
			mutationTopic: offsetMap,
		})
		// Resume partitions
		consumer.Client.ResumeFetchPartitions(map[string][]int32{
			mutationTopic: news,
		})
		logger.Debug().Msgf("resumed partitions %+v", news)
		consumer.Ready = true
	}
	if len(gones) > 0 {
		logger.Info().Msgf("dropped partitions: %+v", gones)
	}

	for _, gonePart := range gones {
		err := consumer.PartitionManager.RemovePartition(gonePart)
		if err != nil {
			logger.Fatal().Err(err).Msg("error removing partition, exiting")
		}
	}

	// Set the current partitions
	internal.Metric_Partitions.Set(float64(len(myPartitions)))
}

// launchPollRecordLoop is launched in a goroutine
func (lc *LogConsumer) launchPollRecordLoop() {
	for !lc.shuttingDown {
		err := lc.pollRecords(context.Background())
		if err != nil {
			logger.Error().Err(err).Msg("error polling for records")
		}
	}
}

var ErrPollFetches = errors.New("error polling fetches")

func (lc *LogConsumer) pollRecords(c context.Context) error {
	// maybe use PollRecords?
	ctx, cancel := context.WithTimeout(c, time.Second*5)
	defer cancel()
	fetches := lc.Client.PollFetches(ctx)
	if errs := fetches.Errors(); len(errs) > 0 {
		if len(errs) == 1 {
			if errors.Is(errs[0].Err, context.DeadlineExceeded) {
				logger.Debug().Msg("got no records")
				return nil
			}
			if errors.Is(errs[0].Err, kgo.ErrClientClosed) {
				return nil
			}
		}
		return fmt.Errorf("got errors when fetching: %+v :: %w", errs, ErrPollFetches)
	}

	g := errgroup.Group{}
	fetches.EachPartition(func(part kgo.FetchTopicPartition) {
		g.Go(func() error {
			for _, record := range part.Records {
				r := record
				logger.Debug().Msgf("got mutation for part %d at offset %d", r.Partition, r.Offset)
				err := lc.PartitionManager.HandleMutation(part.Partition, r.Value, r.Offset)
				if err != nil {
					return fmt.Errorf("error in HandleMutation: %w", err)
				}
			}
			return nil
		})
	})
	err := g.Wait()
	return err
}
