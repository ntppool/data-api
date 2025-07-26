package chdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/data-api/ntpdb"
)

func (d *ClickHouse) Logscores(ctx context.Context, serverID, monitorID int, since time.Time, limit int, fullHistory bool) ([]ntpdb.LogScore, error) {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "CH Logscores")
	defer span.End()

	recentFirst := true

	if since.IsZero() && !fullHistory {
		since = time.Now().Add(4 * -24 * time.Hour)
	} else {
		recentFirst = false
	}

	args := []interface{}{serverID}
	query := `select id,monitor_id,server_id,ts,
                toFloat64(score),toFloat64(step),offset,
                rtt,leap,warning,error
              from log_scores
              where
                server_id = ?`

	if monitorID > 0 {
		query = `select id,monitor_id,server_id,ts,
                toFloat64(score),toFloat64(step),offset,
                rtt,leap,warning,error
              from log_scores
              where
                server_id = ?
                and monitor_id = ?`
		args = []interface{}{serverID, monitorID}
	}

	if fullHistory {
		query += " order by ts"
		if recentFirst {
			query += " desc"
		}
	} else {
		query += " and ts > ? order by ts "
		if recentFirst {
			query += "desc "
		}
		query += "limit ?"
		args = append(args, since, limit)
	}

	log.DebugContext(ctx, "clickhouse query", "query", query, "args", args)

	rows, err := d.Scores.Query(
		clickhouse.Context(
			ctx, clickhouse.WithSpan(span.SpanContext()),
		),
		query, args...,
	)
	if err != nil {
		log.ErrorContext(ctx, "query error", "err", err)
		return nil, fmt.Errorf("database error")
	}

	rv := []ntpdb.LogScore{}

	for rows.Next() {

		row := ntpdb.LogScore{}

		var leap uint8

		if err := rows.Scan(
			&row.ID,
			&row.MonitorID,
			&row.ServerID,
			&row.Ts,
			&row.Score,
			&row.Step,
			&row.Offset,
			&row.Rtt,
			&leap,
			&row.Attributes.Warning,
			&row.Attributes.Error,
		); err != nil {
			log.Error("could not parse row", "err", err)
			continue
		}

		row.Attributes.Leap = int8(leap)

		rv = append(rv, row)

	}

	// log.InfoContext(ctx, "returning data", "rv", rv)

	return rv, nil
}

// LogscoresTimeRange queries log scores within a specific time range for Grafana integration
func (d *ClickHouse) LogscoresTimeRange(ctx context.Context, serverID, monitorID int, from, to time.Time, limit int) ([]ntpdb.LogScore, error) {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "CH LogscoresTimeRange")
	defer span.End()

	args := []interface{}{serverID, from, to}
	
	query := `select id,monitor_id,server_id,ts,
                toFloat64(score),toFloat64(step),offset,
                rtt,leap,warning,error
              from log_scores
              where
                server_id = ?
                and ts >= ?
                and ts <= ?`

	if monitorID > 0 {
		query += " and monitor_id = ?"
		args = append(args, monitorID)
	}

	// Always order by timestamp ASC for Grafana convention
	query += " order by ts ASC"
	
	// Apply limit to prevent memory issues
	if limit > 0 {
		query += " limit ?"
		args = append(args, limit)
	}

	log.DebugContext(ctx, "clickhouse time range query", 
		"query", query, 
		"args", args,
		"server_id", serverID,
		"monitor_id", monitorID,
		"from", from.Format(time.RFC3339),
		"to", to.Format(time.RFC3339),
		"limit", limit,
		"full_sql_with_params", func() string {
			// Build a readable SQL query with parameters substituted for debugging
			sqlDebug := query
			paramIndex := 0
			for strings.Contains(sqlDebug, "?") && paramIndex < len(args) {
				var replacement string
				switch v := args[paramIndex].(type) {
				case int:
					replacement = fmt.Sprintf("%d", v)
				case time.Time:
					replacement = fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05"))
				default:
					replacement = fmt.Sprintf("'%v'", v)
				}
				sqlDebug = strings.Replace(sqlDebug, "?", replacement, 1)
				paramIndex++
			}
			return sqlDebug
		}(),
	)

	rows, err := d.Scores.Query(
		clickhouse.Context(
			ctx, clickhouse.WithSpan(span.SpanContext()),
		),
		query, args...,
	)
	if err != nil {
		log.ErrorContext(ctx, "time range query error", "err", err)
		return nil, fmt.Errorf("database error")
	}

	rv := []ntpdb.LogScore{}

	for rows.Next() {
		row := ntpdb.LogScore{}
		var leap uint8

		if err := rows.Scan(
			&row.ID,
			&row.MonitorID,
			&row.ServerID,
			&row.Ts,
			&row.Score,
			&row.Step,
			&row.Offset,
			&row.Rtt,
			&leap,
			&row.Attributes.Warning,
			&row.Attributes.Error,
		); err != nil {
			log.Error("could not parse row", "err", err)
			continue
		}

		row.Attributes.Leap = int8(leap)
		rv = append(rv, row)
	}

	log.InfoContext(ctx, "time range query results", 
		"rows_returned", len(rv),
		"server_id", serverID,
		"monitor_id", monitorID,
		"time_range", fmt.Sprintf("%s to %s", from.Format(time.RFC3339), to.Format(time.RFC3339)),
		"limit", limit,
		"sample_rows", func() []map[string]interface{} {
			samples := make([]map[string]interface{}, 0, 3)
			for i, row := range rv {
				if i >= 3 { break }
				samples = append(samples, map[string]interface{}{
					"id": row.ID,
					"monitor_id": row.MonitorID,
					"ts": row.Ts.Format(time.RFC3339),
					"score": row.Score,
					"rtt_valid": row.Rtt.Valid,
					"offset_valid": row.Offset.Valid,
				})
			}
			return samples
		}(),
	)

	return rv, nil
}
