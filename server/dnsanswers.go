package server

import (
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/labstack/echo/v4"
	"golang.org/x/sync/errgroup"

	"go.ntppool.org/common/logger"
	chdb "go.ntppool.org/data-api/chdb"
	"go.ntppool.org/data-api/ntpdb"
)

const pointBasis float64 = 10000
const pointSymbol = "‱"

// const pointBasis = 1000
// const pointSymbol = "‰"

func (srv *Server) dnsAnswers(c echo.Context) error {

	log := logger.Setup()

	ctx := c.Request().Context()

	c.Response().Header().Set("Cache-Control", "max-age=20")

	// conn, err := srv.chConn(ctx)
	// if err != nil {
	// 	slog.Error("could not connect to clickhouse", "err", err)
	// 	return c.String(http.StatusInternalServerError, "clickhouse error")
	// }

	ip, err := netip.ParseAddr(c.Param("server"))
	if err != nil {
		log.Warn("could not parse server parameter", "server", c.Param("server"), "err", err)
		return c.NoContent(http.StatusNotFound)
	}

	if ip.String() != c.Param("server") || len(c.QueryString()) > 0 {
		return c.Redirect(http.StatusPermanentRedirect, "https://www.ntppool.org/api/data/server/dns/answers/"+ip.String())
	}

	queryGroup, ctx := errgroup.WithContext(ctx)

	var zoneStats []ntpdb.GetZoneStatsV2Row
	var serverNetspeed uint32

	queryGroup.Go(func() error {
		var err error
		q := ntpdb.New(srv.db)
		zoneStats, err = q.GetZoneStatsV2(ctx, ip.String())
		if err != nil {
			slog.Error("GetZoneStatsV2", "err", err)
			return err
		}
		if zoneStats == nil {
			slog.Info("didn't get zoneStats")
		}

		serverNetspeed, err = q.GetServerNetspeed(ctx, ip.String())
		if err != nil {
			slog.Error("GetServerNetspeed", "err", err)
			return err
		}

		return nil
	})

	days := 3

	var serverData chdb.ServerQueries

	queryGroup.Go(func() error {
		var err error
		serverData, err = srv.ch.ServerAnswerCounts(c.Request().Context(), ip.String(), days)
		if err != nil {
			slog.Error("ServerUserCCData", "err", err)
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

		totalData, err = srv.ch.AnswerTotals(c.Request().Context(), qtype, days)
		if err != nil {
			slog.Error("AnswerTotals", "err", err)
		}
		return err
	})

	err = queryGroup.Wait()
	if err != nil {
		slog.Error("query error", "err", err)
		return c.String(http.StatusInternalServerError, err.Error())
	}

	zoneTotals := map[string]int32{}

	for _, z := range zoneStats {
		zn := z.ZoneName
		if zn == "@" {
			zn = ""
		}
		zoneTotals[zn] = z.NetspeedActive // binary.BigEndian.Uint64(...)
		// slog.Info("zone netspeed", "cc", z.ZoneName, "speed", z.NetspeedActive)
	}

	for _, cc := range serverData {
		cc.Points = (pointBasis / float64(totalData[cc.CC])) * float64(cc.Count)
		totalName := cc.CC
		if totalName == "gb" {
			totalName = "uk"
		}
		if zt, ok := zoneTotals[totalName]; ok {
			cc.Netspeed = (pointBasis / float64(zt)) * float64(serverNetspeed)
		}
		// log.Info("points", "cc", cc.CC, "points", cc.Points)
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

	c.Response().Header().Set("Cache-Control", "max-age=1800")

	return c.JSONPretty(http.StatusOK, r, "")

}
