package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/data-api/logscores"
	"go.ntppool.org/data-api/ntpdb"
)

type historyMode uint8

const (
	historyModeUnknown historyMode = iota
	historyModeLog
	historyModeJSON
	historyModeMonitor
)

func paramHistoryMode(s string) historyMode {
	switch s {
	case "log":
		return historyModeLog
	case "json":
		return historyModeJSON
	case "monitor":
		return historyModeMonitor
	default:
		return historyModeUnknown
	}
}

func (srv *Server) getHistory(ctx context.Context, c echo.Context, server ntpdb.Server) (*logscores.LogScoreHistory, error) {
	log := logger.Setup()

	limit := 0
	if limitParam, err := strconv.Atoi(c.QueryParam("limit")); err == nil {
		limit = limitParam
	} else {
		limit = 50
	}
	if limit > 4000 {
		limit = 4000
	}

	since, _ := strconv.ParseInt(c.QueryParam("since"), 10, 64) // defaults to 0 so don't care if it parses

	monitorParam := c.QueryParam("monitor")

	if since > 0 {
		c.Request().Header.Set("Cache-Control", "s-maxage=300")
	}

	q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))

	var monitorID uint32 = 0
	switch monitorParam {
	case "":
		name := "recentmedian.scores.ntp.dev"
		monitor, err := q.GetMonitorByName(ctx, sql.NullString{Valid: true, String: name})
		if err != nil {
			log.Warn("could not find monitor", "name", name, "err", err)
		}
		monitorID = monitor.ID
	case "*":
		monitorID = 0 // don't filter on monitor ID
	}

	log.Info("monitor param", "monitor", monitorID)

	sinceTime := time.Unix(since, 0)
	if since > 0 {
		log.Warn("monitor data requested with since parameter, not supported", "since", sinceTime)
	}

	ls, err := logscores.GetHistory(ctx, srv.db, server.ID, monitorID, sinceTime, limit)

	return ls, err
}

func (srv *Server) history(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "history")
	defer span.End()

	// for errors and 404s, a shorter cache time
	c.Response().Header().Set("Cache-Control", "public,max-age=300")

	mode := paramHistoryMode(c.Param("mode"))
	if mode == historyModeUnknown {
		return c.String(http.StatusNotFound, "invalid mode")
	}

	server, err := srv.FindServer(ctx, c.Param("server"))
	if err != nil {
		log.Error("find server", "err", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}
	if server.ID == 0 {
		return c.String(http.StatusNotFound, "server not found")
	}

	history, err := srv.getHistory(ctx, c, server)
	if err != nil {
		log.Error("get history", "err", err)
		return c.String(http.StatusInternalServerError, "internal error")
	}

	if mode == historyModeLog {

		ctx, span := tracing.Tracer().Start(ctx, "history.csv")
		b := bytes.NewBuffer([]byte{})
		w := csv.NewWriter(b)

		ff := func(f float64) string {
			s := fmt.Sprintf("%.9f", f)
			s = strings.TrimRight(s, "0")
			s = strings.TrimRight(s, ".")
			return s
		}

		w.Write([]string{"ts_epoch", "ts", "offset", "step", "score", "monitor_id", "monitor_name", "leap", "error"})
		for _, l := range history.LogScores {
			// log.Debug("csv line", "id", l.ID, "n", i)

			var offset string
			if l.Offset.Valid {
				offset = ff(l.Offset.Float64)
			}

			step := ff(l.Step)
			score := ff(l.Score)
			var monName string
			if l.MonitorID.Valid {
				monName = history.Monitors[int(l.MonitorID.Int32)]
			}
			var leap string
			if l.Attributes.Leap != 0 {
				leap = fmt.Sprintf("%d", l.Attributes.Leap)
			}

			err := w.Write([]string{
				strconv.Itoa(int(l.Ts.Unix())),
				// l.Ts.Format(time.RFC3339),
				l.Ts.Format("2006-01-02 15:04:05"),
				offset,
				step,
				score,
				fmt.Sprintf("%d", l.MonitorID.Int32),
				monName,
				leap,
				l.Attributes.Error,
			})
			if err != nil {
				log.Warn("csv encoding error", "ls_id", l.ID, "err", err)
			}
		}
		w.Flush()
		if err := w.Error(); err != nil {
			log.ErrorContext(ctx, "could not flush csv", "err", err)
			span.End()
			return c.String(http.StatusInternalServerError, "csv error")
		}

		log.Info("entries", "count", len(history.LogScores), "out_bytes", b.Len())

		span.End()
		return c.Blob(http.StatusOK, "text/csv", b.Bytes())

	}

	return c.JSON(http.StatusOK, history)
}
