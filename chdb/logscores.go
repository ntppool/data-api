package chdb

import (
	"context"
	"fmt"
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
		log.WarnContext(ctx, "resetting since to 4 days ago")
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
