package main

import (
	"context"
	"fmt"
	"github.com/danthegoodman1/FireScroll/api"
	"github.com/danthegoodman1/FireScroll/gologger"
	"github.com/danthegoodman1/FireScroll/gossip"
	"github.com/danthegoodman1/FireScroll/internal"
	"github.com/danthegoodman1/FireScroll/log_consumer"
	"github.com/danthegoodman1/FireScroll/partitions"
	"github.com/danthegoodman1/FireScroll/utils"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"
)

var (
	logger = gologger.NewLogger()
)

func main() {
	logger.Info().Msg("starting FireScroll")
	if utils.Env_Profile {
		logger.Warn().Msg("profiling enabled!")
		f, err := os.Create("cpu.pprof")
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
		runtime.SetMutexProfileFraction(1)
		runtime.SetBlockProfileRate(1)
	}
	g := errgroup.Group{}
	g.Go(func() error {
		logger.Debug().Msg("starting internal server")
		return internal.StartServer()
	})

	partitionManager, err := partitions.NewPartitionManager(utils.Env_Namespace)
	if err != nil {
		logger.Fatal().Err(err).Msg("error creating partition manager")
	}
	var logConsumer *log_consumer.LogConsumer
	g.Go(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
		defer cancel()
		lc, err := log_consumer.NewLogConsumer(ctx, utils.Env_Namespace, fmt.Sprintf("%s__%s", utils.Env_Region, utils.Env_ReplicaGroupName), strings.Split(utils.Env_KafkaSeeds, ","), utils.Env_KafkaSessionMs, partitionManager)
		logConsumer = lc
		return err
	})

	var gm *gossip.Manager
	g.Go(func() error {
		var err error
		gm, err = gossip.NewGossipManager(partitionManager)
		return err
	})

	err = g.Wait()
	if err != nil {
		logger.Fatal().Err(err).Msg("Error starting services, exiting")
	}

	apiServer, err := api.StartServer(utils.Env_APIPort, partitionManager, logConsumer, gm)
	if err != nil {
		logger.Fatal().Err(err).Msg("error starting api server")
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(utils.Env_SleepSeconds))
	defer cancel()

	if gm != nil {
		err = gm.Shutdown()
		if err != nil {
			logger.Error().Err(err).Msg("error shutting down gossip manager, other nodes might take some extra time to evict this node but otherwise it's fine")
		}
	}

	g = errgroup.Group{}
	g.Go(func() error {
		return internal.Shutdown(ctx)
	})
	g.Go(func() error {
		return apiServer.Shutdown(ctx)
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
