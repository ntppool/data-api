package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/sync/errgroup"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"

	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"

	"go.ntppool.org/common/health"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/common/tracing"

	chdb "go.ntppool.org/data-api/chdb"
	"go.ntppool.org/data-api/ntpdb"
)

type Server struct {
	db *sql.DB
	ch *chdb.ClickHouse

	ctx context.Context

	metrics    *metricsserver.Metrics
	tpShutdown []tracing.TpShutdownFunc
}

func NewServer(ctx context.Context, configFile string) (*Server, error) {
	ch, err := chdb.New(ctx, configFile)
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}
	db, err := ntpdb.OpenDB(configFile)
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}

	srv := &Server{
		ch:      ch,
		db:      db,
		ctx:     ctx,
		metrics: metricsserver.New(),
	}

	tpShutdown, err := tracing.InitTracer(ctx, &tracing.TracerConfig{
		ServiceName: "data-api",
		Environment: "",
	})
	if err != nil {
		return nil, err
	}

	srv.tpShutdown = append(srv.tpShutdown, tpShutdown)
	// srv.tracer = tracing.Tracer()
	return srv, nil
}

func (srv *Server) Run() error {
	log := logger.Setup()

	ctx, cancel := context.WithCancel(srv.ctx)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		return srv.metrics.ListenAndServe(ctx, 9020)
	})

	g.Go(func() error {
		return health.HealthCheckListener(ctx, 9019, log.WithGroup("health"))
	})

	e := echo.New()
	e.Use(otelecho.Middleware("data-api"))
	e.Use(slogecho.New(log))

	srv.tpShutdown = append(srv.tpShutdown, e.Shutdown)

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{
			"http://localhost", "http://localhost:5173", "http://localhost:8080",
			"https://www.ntppool.org", "https://*.ntppool.org",
			"https://web.beta.grundclock.com", "https://manage.beta.grundclock.com",
			"https:/*.askdev.grundclock.com",
		},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	e.GET("/hello", func(c echo.Context) error {
		ctx := c.Request().Context()
		ctx, span := tracing.Tracer().Start(ctx, "hello")
		defer span.End()

		log.InfoContext(ctx, "hello log")
		return c.String(http.StatusOK, "Hello")
	})

	e.GET("/api/usercc", srv.userCountryData)

	e.GET("/api/server/dns/answers/:server", srv.dnsAnswers)

	g.Go(func() error {
		return e.Start(":8030")
	})

	return g.Wait()
}

func (srv *Server) Shutdown(ctx context.Context) error {
	logger.Setup().Info("Shutting down")
	errs := []error{}
	for _, fn := range srv.tpShutdown {
		err := fn(ctx)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (srv *Server) userCountryData(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "userCountryData")
	defer span.End()

	q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))
	zoneStats, err := ntpdb.GetZoneStats(ctx, q)
	if err != nil {
		log.ErrorContext(ctx, "GetZoneStats", "err", err)
		return c.String(http.StatusInternalServerError, err.Error())
	}
	if zoneStats == nil {
		log.InfoContext(ctx, "didn't get zoneStats")
	}

	data, err := srv.ch.UserCountryData(c.Request().Context())
	if err != nil {
		log.ErrorContext(ctx, "UserCountryData", "err", err)
		return c.String(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, struct {
		UserCountry *chdb.UserCountry
		ZoneStats   *ntpdb.ZoneStats
	}{
		UserCountry: data,
		ZoneStats:   zoneStats,
	})

}
