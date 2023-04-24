package main

import (
	"context"
	"github.com/danthegoodman1/FanoutDB/gologger"
	"github.com/danthegoodman1/FanoutDB/internal"
	"github.com/danthegoodman1/FanoutDB/utils"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
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

	err := g.Wait()
	if err != nil {
		logger.Error().Err(err).Msg("Error starting services")
	}

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

	g = errgroup.Group{}
	g.Go(func() error {
		return internal.Shutdown(ctx)
	})

	if err := g.Wait(); err != nil {
		logger.Error().Err(err).Msg("error shutting down servers")
		os.Exit(1)
	}
	logger.Info().Msg("shutdown servers, exiting")
}
