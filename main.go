package main

import (
	"context"
	"github.com/danthegoodman1/FanoutDB/api"
	"github.com/danthegoodman1/FanoutDB/gologger"
	"github.com/danthegoodman1/FanoutDB/internal"
	"github.com/danthegoodman1/FanoutDB/log_consumer"
	"github.com/danthegoodman1/FanoutDB/partitions"
	"github.com/danthegoodman1/FanoutDB/utils"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	logger = gologger.NewLogger()
)

func main() {
	logger.Info().Msg("starting FanoutDB")
	g := errgroup.Group{}
	g.Go(func() error {
		logger.Debug().Msg("starting internal server")
		return internal.StartServer()
	})
	g.Go(func() error {
		logger.Debug().Msg("starting api server")
		return api.StartServer()
	})

	partitionManager, err := partitions.NewPartitionManager()
	if err != nil {
		logger.Fatal().Err(err).Msg("error creating partition manager")
	}
	var logConsumer *log_consumer.LogConsumer
	g.Go(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		lc, err := log_consumer.NewLogConsumer(ctx, utils.Env_Namespace, utils.Env_ReplicaGroupName, strings.Split("localhost:19092,localhost:29092,localhost:39092", ","), utils.Env_KafkaSessionMs, partitionManager)
		logConsumer = lc
		return err
	})

	err = g.Wait()
	if err != nil {
		logger.Fatal().Err(err).Msg("Error starting services, exiting")
	}
	logger.Info().Msg("all services started")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	logger.Info().Msg("received shutdown signal!")

	// Provide time for load balancers to de-register before stopping accepting connections
	if utils.Env_SleepSeconds > 0 {
		logger.Info().Msgf("sleeping for %ds before exiting", utils.Env_SleepSeconds)
		time.Sleep(time.Second * time.Duration(utils.Env_SleepSeconds))
		logger.Info().Msgf("slept for %ds, exiting", utils.Env_SleepSeconds)
	}

	logger.Debug().Msg("closing kafka client")
	logConsumer.Client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(utils.Env_SleepSeconds))
	defer cancel()

	g = errgroup.Group{}
	g.Go(func() error {
		return internal.Shutdown(ctx)
	})
	g.Go(func() error {
		return api.Shutdown(ctx)
	})
	g.Go(func() error {
		return partitionManager.Shutdown()
	})
	g.Go(func() error {
		return logConsumer.Shutdown()
	})

	if err := g.Wait(); err != nil {
		logger.Error().Err(err).Msg("error shutting down servers")
		os.Exit(1)
	}
	logger.Info().Msg("shutdown servers, exiting")
}
