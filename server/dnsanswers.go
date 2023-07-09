package server

import (
	"net/http"
	"net/netip"

	"github.com/labstack/echo/v4"
	"go.ntppool.org/common/logger"
	"golang.org/x/exp/slog"
)

const pointBasis float64 = 10000
const pointSymbol = "‱"

// const pointBasis = 1000
// const pointSymbol = "‰"

func (srv *Server) dnsAnswers(c echo.Context) error {

	log := logger.Setup()

	ctx := c.Request().Context()

	conn, err := srv.chConn(ctx)
	if err != nil {
		slog.Error("could not connect to clickhouse", "err", err)
		return c.String(http.StatusInternalServerError, "clickhouse error")
	}

	ip, err := netip.ParseAddr(c.Param("server"))
	if err != nil {
		log.Warn("could not parse server parameter", "server", c.Param("server"), "err", err)
		return c.NoContent(http.StatusNotFound)
	}

	// q := ntpdb.New(srv.db)
	// zoneStats, err := q.GetZoneStats(ctx)
	// if err != nil {
	// 	slog.Error("GetZoneStats", "err", err)
	// 	return c.String(http.StatusInternalServerError, err.Error())
	// }
	// if zoneStats == nil {
	// 	slog.Info("didn't get zoneStats")
	// }

	days := 4

	serverData, err := srv.ch.ServerAnswerCounts(c.Request().Context(), conn, ip.String(), days)
	if err != nil {
		slog.Error("ServerUserCCData", "err", err)
		return c.String(http.StatusInternalServerError, err.Error())
	}

	qtype := "A"
	if ip.Is6() {
		qtype = "AAAA"
	}

	totalData, err := srv.ch.AnswerTotals(c.Request().Context(), conn, qtype, days)
	if err != nil {
		slog.Error("AnswerTotals", "err", err)
		return c.String(http.StatusInternalServerError, err.Error())
	}

	for _, cc := range serverData {
		cc.Points = (pointBasis / float64(totalData[cc.CC])) * float64(cc.Count)
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

	return c.JSONPretty(http.StatusOK, r, "")

}
