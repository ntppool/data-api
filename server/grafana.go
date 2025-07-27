package server

import (
	"context"
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

// ColumnDef represents a Grafana table column definition
type ColumnDef struct {
	Text string `json:"text"`
	Type string `json:"type"`
	Unit string `json:"unit,omitempty"`
}

// GrafanaTableSeries represents a single table series in Grafana format
type GrafanaTableSeries struct {
	Target  string            `json:"target"`
	Tags    map[string]string `json:"tags"`
	Columns []ColumnDef       `json:"columns"`
	Values  [][]interface{}   `json:"values"`
}

// GrafanaTimeSeriesResponse represents the complete Grafana table response
type GrafanaTimeSeriesResponse []GrafanaTableSeries

// timeRangeParams extends historyParameters with time range support
type timeRangeParams struct {
	historyParameters // embed existing struct
	from              time.Time
	to                time.Time
	maxDataPoints     int
	interval          string // for future downsampling
}

// parseTimeRangeParams parses and validates time range parameters
func (srv *Server) parseTimeRangeParams(ctx context.Context, c echo.Context, server ntpdb.Server) (timeRangeParams, error) {
	log := logger.FromContext(ctx)

	// Start with existing parameter parsing logic
	baseParams, err := srv.getHistoryParameters(ctx, c, server)
	if err != nil {
		return timeRangeParams{}, err
	}

	trParams := timeRangeParams{
		historyParameters: baseParams,
		maxDataPoints:     50000, // default
	}

	// Parse from timestamp (required)
	fromParam := c.QueryParam("from")
	if fromParam == "" {
		return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "from parameter is required")
	}

	fromSec, err := strconv.ParseInt(fromParam, 10, 64)
	if err != nil {
		return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "invalid from timestamp format")
	}
	trParams.from = time.Unix(fromSec, 0)

	// Parse to timestamp (required)
	toParam := c.QueryParam("to")
	if toParam == "" {
		return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "to parameter is required")
	}

	toSec, err := strconv.ParseInt(toParam, 10, 64)
	if err != nil {
		return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "invalid to timestamp format")
	}
	trParams.to = time.Unix(toSec, 0)

	// Validate time range
	if trParams.from.Equal(trParams.to) || trParams.from.After(trParams.to) {
		return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "from must be before to")
	}

	// Check minimum range (1 second)
	if trParams.to.Sub(trParams.from) < time.Second {
		return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "time range must be at least 1 second")
	}

	// Check maximum range (90 days)
	if trParams.to.Sub(trParams.from) > 90*24*time.Hour {
		return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "time range cannot exceed 90 days")
	}

	// Parse maxDataPoints (optional)
	if maxDataPointsParam := c.QueryParam("maxDataPoints"); maxDataPointsParam != "" {
		maxDP, err := strconv.Atoi(maxDataPointsParam)
		if err != nil {
			return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "invalid maxDataPoints format")
		}
		if maxDP > 50000 {
			return timeRangeParams{}, echo.NewHTTPError(http.StatusBadRequest, "maxDataPoints cannot exceed 50000")
		}
		if maxDP > 0 {
			trParams.maxDataPoints = maxDP
		}
	}

	// Parse interval (optional, for future downsampling)
	trParams.interval = c.QueryParam("interval")

	log.DebugContext(ctx, "parsed time range params",
		"from", trParams.from,
		"to", trParams.to,
		"maxDataPoints", trParams.maxDataPoints,
		"interval", trParams.interval,
		"monitor", trParams.monitorID,
	)

	return trParams, nil
}

// sanitizeMonitorName sanitizes monitor names for Grafana target format
func sanitizeMonitorName(name string) string {
	// Replace problematic characters for Grafana target format
	result := strings.ReplaceAll(name, " ", "_")
	result = strings.ReplaceAll(result, ".", "-")
	result = strings.ReplaceAll(result, "/", "-")
	return result
}

// transformToGrafanaTableFormat converts LogScoreHistory to Grafana table format
func transformToGrafanaTableFormat(history *logscores.LogScoreHistory, monitors []ntpdb.Monitor) GrafanaTimeSeriesResponse {
	// Group data by monitor_id (one series per monitor)
	monitorData := make(map[int][]ntpdb.LogScore)
	monitorInfo := make(map[int]ntpdb.Monitor)

	// Group log scores by monitor ID
	skippedInvalidMonitors := 0
	for _, ls := range history.LogScores {
		if !ls.MonitorID.Valid {
			skippedInvalidMonitors++
			continue
		}
		monitorID := int(ls.MonitorID.Int32)
		monitorData[monitorID] = append(monitorData[monitorID], ls)
	}

	// Debug logging for transformation
	logger.Setup().Info("transformation grouping debug",
		"total_log_scores", len(history.LogScores),
		"skipped_invalid_monitors", skippedInvalidMonitors,
		"grouped_monitor_ids", func() []int {
			keys := make([]int, 0, len(monitorData))
			for k := range monitorData {
				keys = append(keys, k)
			}
			return keys
		}(),
		"monitor_data_counts", func() map[int]int {
			counts := make(map[int]int)
			for k, v := range monitorData {
				counts[k] = len(v)
			}
			return counts
		}(),
	)

	// Index monitors by ID for quick lookup
	for _, monitor := range monitors {
		monitorInfo[int(monitor.ID)] = monitor
	}

	var response GrafanaTimeSeriesResponse

	// Create one table series per monitor
	logger.Setup().Info("creating grafana series",
		"monitor_data_entries", len(monitorData),
	)

	for monitorID, logScores := range monitorData {
		if len(logScores) == 0 {
			logger.Setup().Info("skipping monitor with no data", "monitor_id", monitorID)
			continue
		}

		logger.Setup().Info("processing monitor series",
			"monitor_id", monitorID,
			"log_scores_count", len(logScores),
		)

		// Get monitor name from history.Monitors map or from monitor info
		monitorName := "unknown"
		if name, exists := history.Monitors[monitorID]; exists && name != "" {
			monitorName = name
		} else if monitor, exists := monitorInfo[monitorID]; exists {
			monitorName = monitor.DisplayName()
		}

		// Build target name and tags
		sanitizedName := sanitizeMonitorName(monitorName)
		target := "monitor{name=" + sanitizedName + "}"

		tags := map[string]string{
			"monitor_id":   strconv.Itoa(monitorID),
			"monitor_name": monitorName,
			"type":         "monitor",
		}

		// Add status (we'll use active as default since we have data for this monitor)
		tags["status"] = "active"

		// Define table columns
		columns := []ColumnDef{
			{Text: "time", Type: "time"},
			{Text: "score", Type: "number"},
			{Text: "rtt", Type: "number", Unit: "ms"},
			{Text: "offset", Type: "number", Unit: "s"},
		}

		// Build values array
		var values [][]interface{}
		for _, ls := range logScores {
			// Convert timestamp to milliseconds
			timestampMs := ls.Ts.Unix() * 1000

			// Create row: [time, score, rtt, offset]
			row := []interface{}{
				timestampMs,
				ls.Score,
			}

			// Add RTT (convert from microseconds to milliseconds, handle null)
			if ls.Rtt.Valid {
				rttMs := float64(ls.Rtt.Int32) / 1000.0
				row = append(row, rttMs)
			} else {
				row = append(row, nil)
			}

			// Add offset (handle null)
			if ls.Offset.Valid {
				row = append(row, ls.Offset.Float64)
			} else {
				row = append(row, nil)
			}

			values = append(values, row)
		}

		// Create table series
		series := GrafanaTableSeries{
			Target:  target,
			Tags:    tags,
			Columns: columns,
			Values:  values,
		}

		response = append(response, series)

		logger.Setup().Info("created series for monitor",
			"monitor_id", monitorID,
			"target", series.Target,
			"values_count", len(series.Values),
		)
	}

	logger.Setup().Info("transformation complete",
		"final_response_count", len(response),
		"response_is_nil", response == nil,
	)

	return response
}

// scoresTimeRange handles Grafana time range requests for NTP server scores
func (srv *Server) scoresTimeRange(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "scoresTimeRange")
	defer span.End()

	// Set reasonable default cache time; adjusted later based on data
	c.Response().Header().Set("Cache-Control", "public,max-age=240")

	// Validate mode parameter
	mode := c.Param("mode")
	if mode != "json" {
		return echo.NewHTTPError(http.StatusNotFound, "invalid mode - only json supported")
	}

	// Find and validate server first
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

	// Parse and validate time range parameters
	params, err := srv.parseTimeRangeParams(ctx, c, server)
	if err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he
		}
		log.ErrorContext(ctx, "parse time range parameters", "err", err)
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	// Query ClickHouse for time range data
	log.InfoContext(ctx, "executing clickhouse time range query",
		"server_id", server.ID,
		"server_ip", server.Ip,
		"monitor_id", params.monitorID,
		"from", params.from,
		"to", params.to,
		"max_data_points", params.maxDataPoints,
		"time_range_duration", params.to.Sub(params.from).String(),
	)

	logScores, err := srv.ch.LogscoresTimeRange(ctx, int(server.ID), params.monitorID, params.from, params.to, params.maxDataPoints)
	if err != nil {
		log.ErrorContext(ctx, "clickhouse time range query", "err", err,
			"server_id", server.ID,
			"monitor_id", params.monitorID,
			"from", params.from,
			"to", params.to,
		)
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	log.InfoContext(ctx, "clickhouse query results",
		"server_id", server.ID,
		"rows_returned", len(logScores),
		"first_few_ids", func() []uint64 {
			ids := make([]uint64, 0, 3)
			for i, ls := range logScores {
				if i >= 3 {
					break
				}
				ids = append(ids, ls.ID)
			}
			return ids
		}(),
	)

	// Build LogScoreHistory structure for compatibility with existing functions
	history := &logscores.LogScoreHistory{
		LogScores: logScores,
		Monitors:  make(map[int]string),
	}

	// Get monitor names for the returned data
	monitorIDs := []uint32{}
	for _, ls := range logScores {
		if ls.MonitorID.Valid {
			monitorID := uint32(ls.MonitorID.Int32)
			if _, exists := history.Monitors[int(monitorID)]; !exists {
				history.Monitors[int(monitorID)] = ""
				monitorIDs = append(monitorIDs, monitorID)
			}
		}
	}

	log.InfoContext(ctx, "monitor processing",
		"unique_monitor_ids", monitorIDs,
		"monitor_count", len(monitorIDs),
	)

	// Get monitor details from database for status and display names
	var monitors []ntpdb.Monitor
	if len(monitorIDs) > 0 {
		q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))
		logScoreMonitors, err := q.GetServerScores(ctx, ntpdb.GetServerScoresParams{
			MonitorIDs: monitorIDs,
			ServerID:   server.ID,
		})
		if err != nil {
			log.ErrorContext(ctx, "get monitor details", "err", err)
			// Don't fail the request, just use basic info
		} else {
			for _, lsm := range logScoreMonitors {
				// Create monitor entry for transformation (we mainly need the display name)
				tempMon := ntpdb.Monitor{
					TlsName:  lsm.TlsName,
					Location: lsm.Location,
					ID:       lsm.ID,
				}
				monitors = append(monitors, tempMon)

				// Update monitor name in history
				history.Monitors[int(lsm.ID)] = tempMon.DisplayName()
			}
		}
	}

	// Transform to Grafana table format
	log.InfoContext(ctx, "starting grafana transformation",
		"log_scores_count", len(logScores),
		"monitors_count", len(monitors),
		"history_monitors", history.Monitors,
	)

	grafanaResponse := transformToGrafanaTableFormat(history, monitors)

	log.InfoContext(ctx, "grafana transformation complete",
		"response_series_count", len(grafanaResponse),
		"response_preview", func() interface{} {
			if len(grafanaResponse) == 0 {
				return "empty_response"
			}
			first := grafanaResponse[0]
			return map[string]interface{}{
				"target":        first.Target,
				"tags":          first.Tags,
				"columns_count": len(first.Columns),
				"values_count":  len(first.Values),
				"first_few_values": func() [][]interface{} {
					if len(first.Values) == 0 {
						return [][]interface{}{}
					}
					count := 2
					if len(first.Values) < count {
						count = len(first.Values)
					}
					return first.Values[:count]
				}(),
			}
		}(),
	)

	// Set cache control headers based on data characteristics
	setHistoryCacheControl(c, history)

	// Set CORS headers
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Content-Type", "application/json")

	log.InfoContext(ctx, "time range response final",
		"server_id", server.ID,
		"server_ip", server.Ip,
		"monitor_id", params.monitorID,
		"time_range", params.to.Sub(params.from).String(),
		"raw_data_points", len(logScores),
		"grafana_series_count", len(grafanaResponse),
		"max_data_points", params.maxDataPoints,
		"response_is_null", grafanaResponse == nil,
		"response_is_empty", len(grafanaResponse) == 0,
	)

	return c.JSON(http.StatusOK, grafanaResponse)
}

// testGrafanaTable returns sample data in Grafana table format for validation
func (srv *Server) testGrafanaTable(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "testGrafanaTable")
	defer span.End()

	log.InfoContext(ctx, "serving test Grafana table format",
		"remote_ip", c.RealIP(),
		"user_agent", c.Request().UserAgent(),
	)

	// Generate sample data with realistic NTP Pool values
	now := time.Now()
	sampleData := GrafanaTimeSeriesResponse{
		{
			Target: "monitor{name=zakim1-yfhw4a}",
			Tags: map[string]string{
				"monitor_id":   "126",
				"monitor_name": "zakim1-yfhw4a",
				"type":         "monitor",
				"status":       "active",
			},
			Columns: []ColumnDef{
				{Text: "time", Type: "time"},
				{Text: "score", Type: "number"},
				{Text: "rtt", Type: "number", Unit: "ms"},
				{Text: "offset", Type: "number", Unit: "s"},
			},
			Values: [][]interface{}{
				{now.Add(-10*time.Minute).Unix() * 1000, 20.0, 18.865, -0.000267},
				{now.Add(-20*time.Minute).Unix() * 1000, 20.0, 18.96, -0.000390},
				{now.Add(-30*time.Minute).Unix() * 1000, 20.0, 18.073, -0.000768},
				{now.Add(-40*time.Minute).Unix() * 1000, 20.0, 18.209, nil}, // null offset example
			},
		},
		{
			Target: "monitor{name=nj2-mon01}",
			Tags: map[string]string{
				"monitor_id":   "84",
				"monitor_name": "nj2-mon01",
				"type":         "monitor",
				"status":       "active",
			},
			Columns: []ColumnDef{
				{Text: "time", Type: "time"},
				{Text: "score", Type: "number"},
				{Text: "rtt", Type: "number", Unit: "ms"},
				{Text: "offset", Type: "number", Unit: "s"},
			},
			Values: [][]interface{}{
				{now.Add(-10*time.Minute).Unix() * 1000, 19.5, 22.145, 0.000123},
				{now.Add(-20*time.Minute).Unix() * 1000, 19.8, 21.892, 0.000089},
				{now.Add(-30*time.Minute).Unix() * 1000, 20.0, 22.034, 0.000156},
			},
		},
	}

	// Add CORS header for browser testing
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Set("Content-Type", "application/json")

	// Set cache control similar to other endpoints
	c.Response().Header().Set("Cache-Control", "public,max-age=60")

	log.InfoContext(ctx, "test Grafana table response sent",
		"series_count", len(sampleData),
		"response_size_approx", "~1KB",
	)

	return c.JSON(http.StatusOK, sampleData)
}
