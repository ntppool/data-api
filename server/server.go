package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	slogecho "github.com/samber/slog-echo"

	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go.ntppool.org/common/health"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/metricsserver"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/common/version"
	"go.ntppool.org/common/xff/fastlyxff"

	"go.ntppool.org/api/config"

	chdb "go.ntppool.org/data-api/chdb"
	"go.ntppool.org/data-api/ntpdb"
)

type Server struct {
	db     *sql.DB
	ch     *chdb.ClickHouse
	config *config.Config

	ctx context.Context

	metrics    *metricsserver.Metrics
	tpShutdown []tracing.TpShutdownFunc
}

func NewServer(ctx context.Context, configFile string) (*Server, error) {
	log := logger.Setup()

	ch, err := chdb.New(ctx, configFile)
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}
	db, err := ntpdb.OpenDB(ctx, configFile)
	if err != nil {
		return nil, fmt.Errorf("mysql open: %w", err)
	}

	conf := config.New()
	if !conf.Valid() {
		log.Error("invalid ntppool config")
	}

	srv := &Server{
		ch:      ch,
		db:      db,
		ctx:     ctx,
		config:  conf,
		metrics: metricsserver.New(),
	}

	tpShutdown, err := tracing.InitTracer(ctx, &tracing.TracerConfig{
		ServiceName: "data-api",
		Environment: conf.DeploymentMode(),
	})
	if err != nil {
		return nil, fmt.Errorf("tracing init: %w", err)
	}

	srv.tpShutdown = append(srv.tpShutdown, tpShutdown)
	return srv, nil
}

func (srv *Server) Run() error {
	log := logger.Setup()

	ntpconf := config.New()

	ctx, cancel := context.WithCancel(srv.ctx)
	defer cancel()

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		version.RegisterMetric("dataapi", srv.metrics.Registry())
		return srv.metrics.ListenAndServe(ctx, 9020)
	})

	g.Go(func() error {
		hclog := log.WithGroup("health")
		hc := health.NewServer(healthHandler(srv, hclog))
		hc.SetLogger(hclog)
		return hc.Listen(ctx, 9019)
	})

	e := echo.New()
	srv.tpShutdown = append(srv.tpShutdown, e.Shutdown)

	e.Debug = false

	trustOptions := []echo.TrustOption{
		echo.TrustLoopback(true),
		echo.TrustLinkLocal(false),
		echo.TrustPrivateNet(true),
	}

	if fileName := os.Getenv("FASTLY_IPS"); len(fileName) > 0 {
		xff, err := fastlyxff.New(fileName)
		if err != nil {
			return err
		}
		cdnTrustRanges, err := xff.EchoTrustOption()
		if err != nil {
			return err
		}
		trustOptions = append(trustOptions, cdnTrustRanges...)
	} else {
		log.Warn("Fastly IPs not configured (FASTLY_IPS)")
	}

	e.IPExtractor = echo.ExtractIPFromXFFHeader(trustOptions...)

	e.Use(echoprometheus.NewMiddlewareWithConfig(echoprometheus.MiddlewareConfig{
		Registerer: srv.metrics.Registry(),
	}))
	e.Use(otelecho.Middleware("data-api"))

	e.Use(slogecho.NewWithConfig(log,
		slogecho.Config{
			WithTraceID:      false, // done by logger already
			DefaultLevel:     slog.LevelInfo,
			ClientErrorLevel: slog.LevelWarn,
			ServerErrorLevel: slog.LevelError,
			// WithRequestHeader: true,
		},
	))

	e.Use(
		func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				request := c.Request()

				span := trace.SpanFromContext(request.Context())
				span.SetAttributes(attribute.String("http.real_ip", c.RealIP()))

				// since the Go library (temporarily?) isn't including this
				span.SetAttributes(attribute.String("url.path", c.Request().RequestURI))
				if q := c.QueryString(); len(q) > 0 {
					span.SetAttributes(attribute.String("url.query", q))
				}

				c.Response().Header().Set("Traceparent", span.SpanContext().TraceID().String())

				return next(c)
			}
		},
	)

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		vinfo := version.VersionInfo()
		v := "data-api/" + vinfo.Version + "+" + vinfo.GitRevShort
		return func(c echo.Context) error {
			c.Response().Header().Set(echo.HeaderServer, v)
			return next(c)
		}
	})

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{
			"http://localhost", "http://localhost:5173", "http://localhost:5174", "http://localhost:8080",
			"https://www.ntppool.org", "https://*.ntppool.org",
			"https://web.beta.grundclock.com", "https://manage.beta.grundclock.com",
			"https:/*.askdev.grundclock.com",
		},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			log.ErrorContext(c.Request().Context(), err.Error(), "stack", string(stack))
			fmt.Println(string(stack))
			return err
		},
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
	e.GET("/api/server/scores/:server/:mode", srv.history)
	e.GET("/api/dns/counts", srv.dnsQueryCounts)
	e.GET("/api/v2/test/grafana-table", srv.testGrafanaTable)
	e.GET("/api/v2/server/scores/:server/:mode", srv.scoresTimeRange)

	if len(ntpconf.WebHostname()) > 0 {
		e.POST("/api/server/scores/:server/:mode", func(c echo.Context) error {
			// POST requests used to work, so make them not error out
			mode := c.Param("mode")
			server := c.Param("server")
			query := c.Request().URL.Query()
			return c.Redirect(
				http.StatusSeeOther,
				ntpconf.WebURL(
					fmt.Sprintf("/scores/%s/%s", server, mode),
					&query,
				),
			)
		})
	}
	e.GET("/graph/:server/:type", srv.graphImage)

	e.GET("/api/zone/counts/:zone_name", srv.zoneCounts)

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

	data, err := srv.ch.UserCountryData(ctx)
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

func (srv *Server) dnsQueryCounts(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "dnsQueryCounts")
	defer span.End()

	data, err := srv.ch.DNSQueries(ctx)
	if err != nil {
		log.ErrorContext(ctx, "dnsQueryCounts", "err", err)
		return c.String(http.StatusInternalServerError, err.Error())
	}

	hdr := c.Response().Header()
	hdr.Set("Cache-Control", "s-maxage=30,max-age=60")

	return c.JSON(http.StatusOK, data)
}

func healthHandler(srv *Server, log *slog.Logger) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		g, ctx := errgroup.WithContext(ctx)

		stats := srv.db.Stats()
		if stats.OpenConnections > 3 {
			log.InfoContext(ctx, "health requests", "url", req.URL.String(), "stats", stats)
		}

		if resetParam := req.URL.Query().Get("reset"); resetParam != "" {
			reset, err := strconv.ParseBool(resetParam)
			log.InfoContext(ctx, "db reset request", "err", err, "reset", reset)

			if err == nil && reset {
				// this feature was to debug some specific problem
				log.InfoContext(ctx, "setting idle db conns to zero")
				srv.db.SetConnMaxLifetime(30 * time.Second)
				srv.db.SetMaxIdleConns(0)
				srv.db.SetMaxIdleConns(4)
			}
		}

		g.Go(func() error {
			err := srv.ch.Scores.Ping(ctx)
			if err != nil {
				log.WarnContext(ctx, "ch scores ping", "err", err)
				return err
			}
			return nil
		})

		g.Go(func() error {
			err := srv.ch.Logs.Ping(ctx)
			if err != nil {
				log.WarnContext(ctx, "ch logs ping", "err", err)
				return err
			}
			return nil
		})

		g.Go(func() error {
			err := srv.db.PingContext(ctx)
			if err != nil {
				log.WarnContext(ctx, "db ping", "err", err)
				return err
			}
			return nil
		})

		err := g.Wait()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, err = w.Write([]byte("db ping err"))
			if err != nil {
				log.ErrorContext(ctx, "could not write response", "err", err)
			}
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err = w.Write([]byte("ok"))
		if err != nil {
			log.ErrorContext(ctx, "could not write response", "err", err)
		}
	}
}
