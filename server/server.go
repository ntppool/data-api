package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	otrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/common/health"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"

	chdb "go.ntppool.org/data-api/chdb"
	"go.ntppool.org/data-api/ntpdb"
)

type Server struct {
	db *sql.DB
	ch *chdb.ClickHouse

	ctx context.Context

	metrics *metricsserver.Metrics
	tracer  otrace.Tracer
}

func NewServer(ctx context.Context, configFile string) (*Server, error) {
	ch, err := chdb.New(configFile)
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

	err = srv.initTracer()
	if err != nil {
		return nil, err
	}

	srv.tracer = srv.NewTracer()
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

	e.Use(middleware.Logger())

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost", "http://localhost:5173", "https://www.ntppool.org"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	e.GET("/hello", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello")
	})

	e.GET("/api/usercc", srv.userCountryData)

	e.GET("/api/server/dns/answers/:server", srv.dnsAnswers)

	g.Go(func() error {
		return e.Start(":8030")
	})

	return g.Wait()
}

func (srv *Server) userCountryData(c echo.Context) error {

	ctx := c.Request().Context()

	conn, err := srv.chConn(ctx)
	if err != nil {
		slog.Error("could not connect to clickhouse", "err", err)
		return c.String(http.StatusInternalServerError, "clickhouse error")
	}

	q := ntpdb.New(srv.db)
	zoneStats, err := q.GetZoneStats(ctx)
	if err != nil {
		slog.Error("GetZoneStats", "err", err)
		return c.String(http.StatusInternalServerError, err.Error())
	}
	if zoneStats == nil {
		slog.Info("didn't get zoneStats")
	}

	data, err := srv.ch.UserCountryData(c.Request().Context(), conn)
	if err != nil {
		slog.Error("UserCountryData", "err", err)
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
