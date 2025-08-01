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
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/data-api/logscores"
	"go.ntppool.org/data-api/ntpdb"
)

// sanitizeForCSV removes or replaces problematic characters for CSV output
func sanitizeForCSV(s string) string {
	// Replace NULL bytes and other control characters with a placeholder
	var result strings.Builder
	for _, r := range s {
		switch {
		case r == 0: // NULL byte
			result.WriteString("<NULL>")
		case r < 32 && r != '\t' && r != '\n' && r != '\r': // Other control chars except tab/newline/carriage return
			result.WriteString(fmt.Sprintf("<0x%02X>", r))
		default:
			result.WriteRune(r)
		}
	}
	return result.String()
}

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

type historyParameters struct {
	limit       int
	monitorID   int
	server      ntpdb.Server
	since       time.Time
	fullHistory bool
}

func (srv *Server) getHistoryParameters(ctx context.Context, c echo.Context, server ntpdb.Server) (historyParameters, error) {
	log := logger.FromContext(ctx)

	p := historyParameters{}

	limit := 0
	if limitParam, err := strconv.Atoi(c.QueryParam("limit")); err == nil {
		limit = limitParam
	} else {
		limit = 100
	}

	if limit > 10000 {
		limit = 10000
	}
	p.limit = limit

	q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))

	monitorParam := c.QueryParam("monitor")

	var monitorID uint32
	switch monitorParam {
	case "":
		name := "recentmedian.scores.ntp.dev"
		var ipVersion ntpdb.NullMonitorsIpVersion
		if server.IpVersion == ntpdb.ServersIpVersionV4 {
			ipVersion = ntpdb.NullMonitorsIpVersion{MonitorsIpVersion: ntpdb.MonitorsIpVersionV4, Valid: true}
		} else {
			ipVersion = ntpdb.NullMonitorsIpVersion{MonitorsIpVersion: ntpdb.MonitorsIpVersionV6, Valid: true}
		}
		monitor, err := q.GetMonitorByNameAndIPVersion(ctx, ntpdb.GetMonitorByNameAndIPVersionParams{
			TlsName:   sql.NullString{Valid: true, String: name},
			IpVersion: ipVersion,
		})
		if err != nil {
			log.Warn("could not find monitor", "name", name, "ip_version", server.IpVersion, "err", err)
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
				return p, echo.NewHTTPError(http.StatusNotFound, "monitor not found")
			}

			monitorParam = monitorParam + ".%"
			var ipVersion ntpdb.NullMonitorsIpVersion
			if server.IpVersion == ntpdb.ServersIpVersionV4 {
				ipVersion = ntpdb.NullMonitorsIpVersion{MonitorsIpVersion: ntpdb.MonitorsIpVersionV4, Valid: true}
			} else {
				ipVersion = ntpdb.NullMonitorsIpVersion{MonitorsIpVersion: ntpdb.MonitorsIpVersionV6, Valid: true}
			}
			monitor, err := q.GetMonitorByNameAndIPVersion(ctx, ntpdb.GetMonitorByNameAndIPVersionParams{
				TlsName:   sql.NullString{Valid: true, String: monitorParam},
				IpVersion: ipVersion,
			})
			if err != nil {
				if err == sql.ErrNoRows {
					return p, echo.NewHTTPError(http.StatusNotFound, "monitor not found").WithInternal(err)
				}
				log.WarnContext(ctx, "could not find monitor", "name", monitorParam, "ip_version", server.IpVersion, "err", err)
				return p, echo.NewHTTPError(http.StatusNotFound, "monitor not found (sql)")
			}
			monitorID = monitor.ID

		}
	}

	p.monitorID = int(monitorID)
	log.DebugContext(ctx, "monitor param", "monitor", monitorID, "ip_version", server.IpVersion)

	since, _ := strconv.ParseInt(c.QueryParam("since"), 10, 64) // defaults to 0 so don't care if it parses
	if since > 0 {
		p.since = time.Unix(since, 0)
	}

	clientIP, err := netip.ParseAddr(c.RealIP())
	if err != nil {
		return p, err
	}

	// log.DebugContext(ctx, "client ip", "client_ip", clientIP.String())

	if clientIP.IsPrivate() || clientIP.IsLoopback() { // don't allow this through the ingress or CDN
		if fullParam := c.QueryParam("full_history"); len(fullParam) > 0 {
			if t, _ := strconv.ParseBool(fullParam); t {
				p.fullHistory = true
			}
		}
	}

	return p, nil
}

func (srv *Server) getHistoryMySQL(ctx context.Context, _ echo.Context, p historyParameters) (*logscores.LogScoreHistory, error) {
	ls, err := logscores.GetHistoryMySQL(ctx, srv.db, p.server.ID, uint32(p.monitorID), p.since, p.limit)
	return ls, err
}

func (srv *Server) history(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "history")
	defer span.End()

	// set a reasonable default cache time; adjusted later for
	// happy path common responses
	c.Response().Header().Set("Cache-Control", "public,max-age=240")

	mode := paramHistoryMode(c.Param("mode"))
	if mode == historyModeUnknown {
		return echo.NewHTTPError(http.StatusNotFound, "invalid mode")
	}

	server, err := srv.FindServer(ctx, c.Param("server"))
	if err != nil {
		log.ErrorContext(ctx, "find server", "err", err)
		if he, ok := err.(*echo.HTTPError); ok {
			return he
		}
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

	p, err := srv.getHistoryParameters(ctx, c, server)
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he
		}
		log.ErrorContext(ctx, "get history parameters", "err", err)
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	p.server = server

	var history *logscores.LogScoreHistory

	sourceParam := c.QueryParam("source")
	switch sourceParam {
	case "m":
	case "c":
	default:
		sourceParam = os.Getenv("default_source")
	}

	if sourceParam == "m" {
		history, err = srv.getHistoryMySQL(ctx, c, p)
	} else {
		history, err = logscores.GetHistoryClickHouse(ctx, srv.ch, srv.db, p.server.ID, uint32(p.monitorID), p.since, p.limit, p.fullHistory)
	}
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
		Rtt       *float64 `json:"rtt,omitempty"`
	}

	type MonitorEntry struct {
		ID     uint32   `json:"id"`
		Name   string   `json:"name"`
		Type   string   `json:"type"`
		Ts     string   `json:"ts"`
		Score  float64  `json:"score"`
		Status string   `json:"status"`
		AvgRtt *float64 `json:"avg_rtt,omitempty"`
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

	monitorIDs := []uint32{}
	for k := range history.Monitors {
		monitorIDs = append(monitorIDs, uint32(k))
	}

	q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))
	logScoreMonitors, err := q.GetServerScores(ctx,
		ntpdb.GetServerScoresParams{
			MonitorIDs: monitorIDs,
			ServerID:   server.ID,
		},
	)
	if err != nil {
		span.RecordError(err)
		log.ErrorContext(ctx, "GetServerScores", "err", err)
		return c.String(http.StatusInternalServerError, "err")
	}

	// log.InfoContext(ctx, "got logScoreMonitors", "count", len(logScoreMonitors))

	// Calculate average RTT per monitor
	monitorRttSums := make(map[uint32]float64)
	monitorRttCounts := make(map[uint32]int)

	for _, ls := range history.LogScores {
		if ls.MonitorID.Valid && ls.Rtt.Valid {
			monitorID := uint32(ls.MonitorID.Int32)
			monitorRttSums[monitorID] += float64(ls.Rtt.Int32) / 1000.0
			monitorRttCounts[monitorID]++
		}
	}

	for _, lsm := range logScoreMonitors {
		score := math.Round(lsm.ScoreRaw*10) / 10 // round to one decimal

		tempMon := ntpdb.Monitor{
			//			Hostname: lsm.Hostname,
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

		// Add average RTT if available
		if count, exists := monitorRttCounts[lsm.ID]; exists && count > 0 {
			avgRtt := monitorRttSums[lsm.ID] / float64(count)
			me.AvgRtt = &avgRtt
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
		if ls.Rtt.Valid {
			rtt := float64(ls.Rtt.Int32) / 1000.0
			res.History[i].Rtt = &rtt
		}
	}

	setHistoryCacheControl(c, history)

	return c.JSON(http.StatusOK, res)
}

func (srv *Server) historyCSV(ctx context.Context, c echo.Context, history *logscores.LogScoreHistory) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "history.csv")
	defer span.End()

	b := bytes.NewBuffer([]byte{})
	w := csv.NewWriter(b)

	ff := func(f float64) string {
		s := fmt.Sprintf("%.9f", f)
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
		return s
	}

	err := w.Write([]string{"ts_epoch", "ts", "offset", "step", "score", "monitor_id", "monitor_name", "rtt", "leap", "error"})
	if err != nil {
		log.ErrorContext(ctx, "could not write csv header", "err", err)
		return err
	}
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

		var rtt string
		if l.Rtt.Valid {
			rtt = ff(float64(l.Rtt.Int32) / 1000.0)
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
			rtt,
			leap,
			sanitizeForCSV(l.Attributes.Error),
		})
		if err != nil {
			log.Warn("csv encoding error", "ls_id", l.ID, "err", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.ErrorContext(ctx, "could not flush csv", "err", err)
		return c.String(http.StatusInternalServerError, "csv error")
	}

	// log.Info("entries", "count", len(history.LogScores), "out_bytes", b.Len())

	setHistoryCacheControl(c, history)

	c.Response().Header().Set("Content-Disposition", "inline")
	// Chrome and Firefox force-download text/csv files, so use text/plain
	// https://bugs.chromium.org/p/chromium/issues/detail?id=152911
	return c.Blob(http.StatusOK, "text/plain", b.Bytes())
}

func setHistoryCacheControl(c echo.Context, history *logscores.LogScoreHistory) {
	hdr := c.Response().Header()
	if len(history.LogScores) == 0 ||
		// cache for longer if data hasn't updated for a while; or we didn't
		// find any.
		(time.Now().Add(-8 * time.Hour).After(history.LogScores[len(history.LogScores)-1].Ts)) {
		hdr.Set("Cache-Control", "s-maxage=260,max-age=360")
	} else {
		if len(history.LogScores) == 1 {
			hdr.Set("Cache-Control", "s-maxage=60,max-age=35")
		} else {
			hdr.Set("Cache-Control", "s-maxage=90,max-age=120")
		}
	}
}
