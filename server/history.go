package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
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
	default:
		mID, err := strconv.ParseUint(monitorParam, 10, 32)
		if err == nil {
			monitorID = uint32(mID)
		} else {
			// only accept the name prefix; no wildcards; trust the database
			// to filter out any other crazy
			if strings.ContainsAny(monitorParam, "_%. \t\n") {
				return nil, echo.NewHTTPError(http.StatusNotFound, "monitor not found")
			}

			if err != nil {
				monitorParam = monitorParam + ".%"
				monitor, err := q.GetMonitorByName(ctx, sql.NullString{Valid: true, String: monitorParam})
				if err != nil {
					log.Warn("could not find monitor", "name", monitorParam, "err", err)
					return nil, echo.NewHTTPError(http.StatusNotFound, "monitor not found")
				}
				monitorID = monitor.ID
			}
		}
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

	// just cache for a short time by default
	c.Response().Header().Set("Cache-Control", "public,max-age=240")

	mode := paramHistoryMode(c.Param("mode"))
	if mode == historyModeUnknown {
		return echo.NewHTTPError(http.StatusNotFound, "invalid mode")
	}

	server, err := srv.FindServer(ctx, c.Param("server"))
	if err != nil {
		log.Error("find server", "err", err)
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}
	if server.DeletionAge(30 * 24 * time.Hour) {
		span.AddEvent("server deleted")
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}
	if server.ID == 0 {
		span.AddEvent("server not found")
		return echo.NewHTTPError(http.StatusNotFound, "server not found")
	}

	history, err := srv.getHistory(ctx, c, server)
	if err != nil {
		var httpError *echo.HTTPError
		if errors.As(err, &httpError) {
			if httpError.Code >= 500 {
				log.Error("get history", "err", err)
				span.RecordError(err)
			}
			return httpError
		} else {
			log.Error("get history", "err", err)
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "internal error")
		}
	}

	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	switch mode {
	case historyModeLog:
		return srv.historyCSV(ctx, c, history)
	case historyModeJSON:
		return srv.historyJSON(ctx, c, server, history)
	default:
		return c.String(http.StatusNotFound, "not implemented")
	}

}

func (srv *Server) historyJSON(ctx context.Context, c echo.Context, server ntpdb.Server, history *logscores.LogScoreHistory) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "history.json")
	defer span.End()

	type ScoresEntry struct {
		TS        int64    `json:"ts"`
		Offset    *float64 `json:"offset,omitempty"`
		Step      float64  `json:"step"`
		Score     float64  `json:"score"`
		MonitorID int      `json:"monitor_id"`
	}

	type MonitorEntry struct {
		ID     uint32  `json:"id"`
		Name   string  `json:"name"`
		Type   string  `json:"type"`
		Ts     string  `json:"ts"`
		Score  float64 `json:"score"`
		Status string  `json:"status"`
	}
	res := struct {
		History  []ScoresEntry  `json:"history"`
		Monitors []MonitorEntry `json:"monitors"`
		Server   struct {
			IP string `json:"ip"`
		} `json:"server"`
	}{
		History: make([]ScoresEntry, len(history.LogScores)),
	}
	res.Server.IP = server.Ip

	// log.InfoContext(ctx, "monitor id list", "ids", history.MonitorIDs)

	q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))
	logScoreMonitors, err := q.GetServerScores(ctx,
		ntpdb.GetServerScoresParams{
			MonitorIDs: history.MonitorIDs,
			ServerID:   server.ID,
		},
	)
	if err != nil {
		span.RecordError(err)
		log.ErrorContext(ctx, "GetServerScores", "err", err)
		return c.String(http.StatusInternalServerError, "err")
	}

	// log.InfoContext(ctx, "got logScoreMonitors", "count", len(logScoreMonitors))

	for _, lsm := range logScoreMonitors {
		score := math.Round(lsm.ScoreRaw*10) / 10 // round to one decimal

		tempMon := ntpdb.Monitor{
			Name:     lsm.Name,
			TlsName:  lsm.TlsName,
			Location: lsm.Location,
			ID:       lsm.ID,
		}
		name := tempMon.DisplayName()

		me := MonitorEntry{
			ID:     lsm.ID,
			Name:   name,
			Type:   string(lsm.Type),
			Ts:     lsm.ScoreTs.Time.Format(time.RFC3339),
			Score:  score,
			Status: string(lsm.Status),
		}
		res.Monitors = append(res.Monitors, me)
	}

	for i, ls := range history.LogScores {
		x := float64(1000000000000)
		score := math.Round(ls.Score*x) / x
		res.History[i] = ScoresEntry{
			TS:        ls.Ts.Unix(),
			MonitorID: int(ls.MonitorID.Int32),
			Step:      ls.Step,
			Score:     score,
		}
		if ls.Offset.Valid {
			offset := ls.Offset.Float64
			res.History[i].Offset = &offset
		}
	}

	if len(history.LogScores) == 0 ||
		history.LogScores[len(history.LogScores)-1].Ts.After(time.Now().Add(-8*time.Hour)) {
		// cache for longer if data hasn't updated for a while
		c.Request().Header.Set("Cache-Control", "s-maxage=3600,max-age=1800")
	} else {
		c.Request().Header.Set("Cache-Control", "s-maxage=300,max-age=240")
	}

	return c.JSON(http.StatusOK, res)

}

func (srv *Server) historyCSV(ctx context.Context, c echo.Context, history *logscores.LogScoreHistory) error {
	log := logger.Setup()
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

	// log.Info("entries", "count", len(history.LogScores), "out_bytes", b.Len())

	c.Request().Header.Set("Cache-Control", "s-maxage=120,max-age=120")

	return c.Blob(http.StatusOK, "text/csv", b.Bytes())

}
