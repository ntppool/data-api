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

func (d *ClickHouse) Logscores(ctx context.Context, serverID, monitorID int, since time.Time, limit int) ([]ntpdb.LogScore, error) {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "CH Logscores")
	defer span.End()

	if since.IsZero() {
		since = time.Now().Add(4 * -24 * time.Hour)
	}

	args := []interface{}{serverID, since, limit}
	query := `select id,monitor_id,server_id,ts,
                toFloat64(score),toFloat64(step),offset,
                rtt,leap,warning,error
              from log_scores
              where
                server_id = ?
                and ts > ?
              order by ts desc
              limit ?;`

	if monitorID > 0 {
		query = `select id,monitor_id,server_id,ts,
                toFloat64(score),toFloat64(step),offset,
                rtt,leap,warning,error
              from log_scores
              where
                server_id = ?
                and monitor_id = ?
                and ts > ?
              order by ts desc
              limit ?;`
		args = []interface{}{serverID, monitorID, since, limit}
	}

	rows, err := d.Scores.Query(clickhouse.Context(ctx, clickhouse.WithSpan(span.SpanContext())),
		query, args...,
	)
	if err != nil {
		log.ErrorContext(ctx, "query error", "err", err)
		return nil, fmt.Errorf("database error")
	}

	rv := []ntpdb.LogScore{}

	for rows.Next() {

		row := ntpdb.LogScore{}

		if err := rows.Scan(
			&row.ID,
			&row.MonitorID,
			&row.ServerID,
			&row.Ts,
			&row.Score,
			&row.Step,
			&row.Offset,
			&row.Rtt,
			&row.Attributes.Leap,
			&row.Attributes.Warning,
			&row.Attributes.Error,
		); err != nil {
			log.Error("could not parse row", "err", err)
			continue
		}

		rv = append(rv, row)

	}

	// log.InfoContext(ctx, "returning data", "rv", rv)

	return rv, nil
}
