package internal

import (
	"fmt"
	"github.com/danthegoodman1/FanoutDB/gologger"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context"
	"net/http"
)

var (
	httpServer *http.Server
	logger     = gologger.NewLogger()
)

func StartServer() error {
	logger.Debug().Msgf("Starting internal http server on port %s", Env_InternalPort)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%s", Env_InternalPort),
		Handler: mux,
	}
	go httpServer.ListenAndServe()
	return nil
}

func Shutdown(ctx context.Context) error {
	if httpServer != nil {
		logger.Debug().Msg("Shutting down internal server")
		return httpServer.Shutdown(ctx)
	}
	return nil
}
