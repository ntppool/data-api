package server

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
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