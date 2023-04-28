package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/danthegoodman1/Firescroll/gologger"
	"github.com/danthegoodman1/Firescroll/log_consumer"
	"github.com/danthegoodman1/Firescroll/partitions"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/net/http2"
	"net"
	"net/http"
	"os"
	"time"
)

var (
	logger = gologger.NewLogger()
)

type CustomValidator struct {
	validator *validator.Validate
}

func (cv *CustomValidator) Validate(i interface{}) error {
	if err := cv.validator.Struct(i); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}

func ValidateRequest(c echo.Context, s interface{}) error {
	if err := c.Bind(s); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := c.Validate(s); err != nil {
		return err
	}
	return nil
}

type HTTPServer struct {
	e  *echo.Echo
	pm *partitions.PartitionManager
	lc *log_consumer.LogConsumer
}

func StartServer(port string, pm *partitions.PartitionManager, lc *log_consumer.LogConsumer) (*HTTPServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return nil, fmt.Errorf("error in net.Listen: %w", err)
	}
	e := echo.New()
	s := &HTTPServer{
		e:  e,
		pm: pm,
		lc: lc,
	}
	e.HideBanner = true
	e.HidePort = true
	logConfig := middleware.LoggerConfig{
		//Skipper: skipHealthCheck,
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}",` +
			`"host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}",` +
			`"status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}",` +
			`"bytes_in":${bytes_in},"bytes_out":${bytes_out},"proto":"${protocol}",` +
			// fake request headers for getting info into http logs
			`"userID":"${header:loguserid}","reqID":"${header:reqID}","wasCached":"${header:wasCached}"}` + "\n",
		CustomTimeFormat: "2006-01-02 15:04:05.00000",
		Output:           os.Stdout, // logger or os.Stdout
	}
	e.Use(middleware.LoggerWithConfig(logConfig))
	e.Use(middleware.CORS())
	e.Use(middleware.Gzip())
	e.Use(NewTimeoutMiddleware(30 * time.Second)) // 30s timeout on request
	val := validator.New()
	//urlCompat := regexp.MustCompile("^[0-9a-zA-Z_-]+$")
	//_ = val.RegisterValidation("urlCompat", func(fl validator.FieldLevel) bool {
	//	return urlCompat.MatchString(fl.Field().String())
	//})
	e.Validator = &CustomValidator{validator: val}

	if err != nil {
		logger.Error().Err(err).Msg("Failed initial connection to gubernator")
	}

	e.Listener = listener
	go func() {
		logger.Info().Msg("starting h2c server on " + listener.Addr().String())
		err := e.StartH2CServer("", &http2.Server{})
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("failed to start h2c server, exiting")
		}
	}()

	e.GET("/up", Up)
	e.POST("/records/:op", s.operationHandler)

	return s, nil
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	logger.Info().Msg("shutting down api server")
	return s.e.Shutdown(ctx)
	return nil
}

func NewTimeoutMiddleware(timeout time.Duration) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx, cancelFunc := context.WithTimeout(c.Request().Context(), timeout)
			defer cancelFunc()
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

func Up(c echo.Context) error {
	return c.String(http.StatusOK, "ok")
}
