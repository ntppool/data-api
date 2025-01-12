package server

import (
	"database/sql"
	"errors"
	"net/http"
	"net/netip"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	chdb "go.ntppool.org/data-api/chdb"
	"go.ntppool.org/data-api/ntpdb"
)

const pointBasis float64 = 10000
const pointSymbol = "‱"

// const pointBasis = 1000
// const pointSymbol = "‰"

func (srv *Server) dnsAnswers(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "dnsanswers")
	defer span.End()

	// for errors and 404s, a shorter cache time
	c.Response().Header().Set("Cache-Control", "public,max-age=300")

	// conn, err := srv.chConn(ctx)
	// if err != nil {
	// 	log.Error("could not connect to clickhouse", "err", err)
	// 	return c.String(http.StatusInternalServerError, "clickhouse error")
	// }

	log = log.With("server_param", c.Param("server"))
	span.SetAttributes(attribute.String("server_param", c.Param("server")))

	ip, err := netip.ParseAddr(c.Param("server"))
	if err != nil {
		log.Warn("could not parse server parameter", "server", c.Param("server"), "err", err)
		return c.NoContent(http.StatusBadRequest)
	}

	if ip.String() != c.Param("server") || len(c.QueryString()) > 0 {
		// better URLs are forever
		c.Response().Header().Set("Cache-Control", "public,max-age=10400")
		return c.Redirect(http.StatusPermanentRedirect, "https://www.ntppool.org/api/data/server/dns/answers/"+ip.String())
	}

	queryGroup, ctx := errgroup.WithContext(ctx)

	var zoneStats []ntpdb.GetZoneStatsV2Row
	var serverNetspeed uint32

	queryGroup.Go(func() error {
		var err error
		q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))

		serverNetspeed, err = q.GetServerNetspeed(ctx, ip.String())
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				log.Error("GetServerNetspeed", "err", err)
			}
			return err // this will return if the server doesn't exist
		}

		zoneStats, err = q.GetZoneStatsV2(ctx, ip.String())
		if err != nil {
			// if we had a netspeed we expect rows here, too.
			log.Error("GetZoneStatsV2", "err", err)
			return err
		}
		if zoneStats == nil {
			log.Warn("didn't get zoneStats")
		}

		return nil
	})

	days := 3

	var serverData chdb.ServerQueries

	queryGroup.Go(func() error {
		var err error
		serverData, err = srv.ch.ServerAnswerCounts(ctx, ip.String(), days)
		if err != nil {
			log.Error("ServerUserCCData", "err", err)
			return err
		}
		return nil
	})

	var totalData chdb.ServerTotals

	queryGroup.Go(func() error {
		var err error

		qtype := "A"
		if ip.Is6() {
			qtype = "AAAA"
		}

		totalData, err = srv.ch.AnswerTotals(ctx, qtype, days)
		if err != nil {
			log.Error("AnswerTotals", "err", err)
		}
		return err
	})

	err = queryGroup.Wait()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "Not found")
		}
		log.Error("query error", "err", err)
		return c.String(http.StatusInternalServerError, err.Error())
	}

	zoneTotals := map[string]int32{}

	for _, z := range zoneStats {
		zn := z.ZoneName
		if zn == "@" {
			zn = ""
		}
		zoneTotals[zn] = z.NetspeedActive // binary.BigEndian.Uint64(...)
		// log.Info("zone netspeed", "cc", z.ZoneName, "speed", z.NetspeedActive)
	}

	for _, cc := range serverData {
		cc.Points = (pointBasis / float64(totalData[cc.CC])) * float64(cc.Count)
		totalName := cc.CC
		if totalName == "gb" {
			totalName = "uk"
		}
		if zt, ok := zoneTotals[totalName]; ok {
			// log.InfoContext(ctx, "netspeed data", "pointBasis", pointBasis, "zt", zt, "server netspeed", serverNetspeed)
			if zt == 0 {
				// if the recorded netspeed for the zone was zero, assume it's at least
				// this servers worth instead. Otherwise the Netspeed gets to be 'infinite'.
				zt = int32(serverNetspeed)
			}
			cc.Netspeed = (pointBasis / float64(zt)) * float64(serverNetspeed)
		}
		// log.DebugContext(ctx, "points", "cc", cc.CC, "points", cc.Points)
	}

	r := struct {
		Server interface{}
		// Totals interface{}
		PointSymbol string
	}{
		Server:      serverData,
		PointSymbol: pointSymbol,
		// Totals: totalData,
	}

	c.Response().Header().Set("Cache-Control", "public,max-age=1800")

	return c.JSONPretty(http.StatusOK, r, "")

}
