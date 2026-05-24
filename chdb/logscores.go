package chdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/data-api/ntpdb"
)

// scanLogScore reads one ClickHouse log_scores row into an ntpdb.LogScore.
// The ClickHouse columns are unsigned (id UInt64, monitor_id/server_id UInt32,
// rtt UInt32), but the PostgreSQL-generated model uses signed/pgtype fields, so
// the row is scanned into locals matching the ClickHouse types and then converted.
func scanLogScore(rows interface{ Scan(...any) error }) (ntpdb.LogScore, error) {
	row := ntpdb.LogScore{}

	var (
		id        uint64
		monitorID uint32
		serverID  uint32
		ts        time.Time
		offset    sql.NullFloat64
		rtt       sql.NullInt32
		leap      uint8
	)

	if err := rows.Scan(
		&id,
		&monitorID,
		&serverID,
		&ts,
		&row.Score,
		&row.Step,
		&offset,
		&rtt,
		&leap,
		&row.Attributes.Warning,
		&row.Attributes.Error,
	); err != nil {
		return row, err
	}

	row.ID = int64(id)
	row.MonitorID = pgtype.Int8{Int64: int64(monitorID), Valid: true}
	row.ServerID = int64(serverID)
	row.Ts = pgtype.Timestamptz{Time: ts, Valid: true}
	row.Offset = pgtype.Float8{Float64: offset.Float64, Valid: offset.Valid}
	row.Rtt = pgtype.Int4{Int32: rtt.Int32, Valid: rtt.Valid}
	row.Attributes.Leap = int8(leap)

	return row, nil
}

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
		row, err := scanLogScore(rows)
		if err != nil {
			log.Error("could not parse row", "err", err)
			continue
		}
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

	log.DebugContext(
		ctx, "clickhouse time range query",
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
		row, err := scanLogScore(rows)
		if err != nil {
			log.Error("could not parse row", "err", err)
			continue
		}
		rv = append(rv, row)
	}

	log.InfoContext(
		ctx, "time range query results",
		"rows_returned", len(rv),
		"server_id", serverID,
		"monitor_id", monitorID,
		"time_range", fmt.Sprintf("%s to %s", from.Format(time.RFC3339), to.Format(time.RFC3339)),
		"limit", limit,
		"sample_rows", func() []map[string]interface{} {
			samples := make([]map[string]interface{}, 0, 3)
			for i, row := range rv {
				if i >= 3 {
					break
				}
				samples = append(samples, map[string]interface{}{
					"id":           row.ID,
					"monitor_id":   row.MonitorID,
					"ts":           row.Ts.Time.Format(time.RFC3339),
					"score":        row.Score,
					"rtt_valid":    row.Rtt.Valid,
					"offset_valid": row.Offset.Valid,
				})
			}
			return samples
		}(),
	)

	return rv, nil
}
