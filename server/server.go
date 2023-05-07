package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"

	chdb "go.ntppool.org/data-api/chdb"
	"go.ntppool.org/data-api/ntpdb"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	otrace "go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
	"golang.org/x/sync/errgroup"
)

type Server struct {
	db *sql.DB
	ch *chdb.ClickHouse

	ctx    context.Context
	mr     *prometheus.Registry
	tracer otrace.Tracer
}

func NewServer(ctx context.Context) (*Server, error) {
	mr := prometheus.NewRegistry()

	ch, err := chdb.New("database.yaml")
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}
	db, err := ntpdb.OpenDB("database.yaml")
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}

	srv := &Server{
		ch:  ch,
		db:  db,
		ctx: ctx,
		mr:  mr,
	}

	err = srv.initTracer()
	if err != nil {
		return nil, err
	}

	srv.tracer = srv.NewTracer()
	return srv, nil
}

func (srv *Server) metricsHandler() http.Handler {
	return promhttp.HandlerFor(srv.mr, promhttp.HandlerOpts{
		ErrorLog:          log.Default(),
		Registry:          srv.mr,
		EnableOpenMetrics: true,
	})
}

func (srv *Server) Run() error {
	slog.Info("Run()")

	ctx, cancel := context.WithCancel(srv.ctx)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	metricsServer := &http.Server{
		Addr:    ":9000",
		Handler: srv.metricsHandler(),
	}

	g.Go(func() error {
		err := metricsServer.ListenAndServe()
		if err != nil {
			return fmt.Errorf("metrics server: %w", err)
		}
		return nil
	})

	e := echo.New()
	e.Use(otelecho.Middleware("data-api"))

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"http://localhost", "http://localhost:5173", "https://www.ntppool.org"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	e.GET("/hello", func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello")
	})

	e.GET("/api/usercc", srv.userCountryData)

	g.Go(func() error {
		return e.Start(":8000")
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
