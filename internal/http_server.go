package internal

import (
	"fmt"
	"github.com/FireScroll/FireScroll/gologger"
	"github.com/FireScroll/FireScroll/utils"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/net/context"
	"net/http"
	"net/http/pprof"
	"strings"
)

var (
	httpServer *http.Server
	logger     = gologger.NewLogger()
)

func StartServer() error {
	logger.Debug().Msgf("Starting internal http server on port %s", utils.Env_InternalPort)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))

	httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%s", utils.Env_InternalPort),
		Handler: mux,
	}
	go func() {
		err := httpServer.ListenAndServe()
		if strings.Contains(err.Error(), "address already in use") {
			logger.Fatal().Err(err).Msg("error starting server")
		}
	}()
	return nil
}

func Shutdown(ctx context.Context) error {
	if httpServer != nil {
		logger.Debug().Msg("Shutting down internal server")
		return httpServer.Shutdown(ctx)
	}
	return nil
}
