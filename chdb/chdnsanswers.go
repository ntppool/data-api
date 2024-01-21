package chdb

import (
	"context"
	"fmt"
	"sort"

	"github.com/ClickHouse/clickhouse-go/v2"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
)

type ccCount struct {
	CC       string
	Count    uint64
	Points   float64
	Netspeed float64
}

type ServerQueries []*ccCount

type ServerTotals map[string]uint64

func (s ServerQueries) Len() int {
	return len(s)
}
func (s ServerQueries) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s ServerQueries) Less(i, j int) bool {
	return s[i].Count > s[j].Count
}

func (d *ClickHouse) ServerAnswerCounts(ctx context.Context, serverIP string, days int) (ServerQueries, error) {

	ctx, span := tracing.Tracer().Start(ctx, "ServerAnswerCounts")
	defer span.End()

	conn := d.Logs

	log := logger.Setup().With("server", serverIP)

	// queries by UserCC / Qtype for the ServerIP
	rows, err := conn.Query(clickhouse.Context(ctx,
		clickhouse.WithSpan(span.SpanContext()),
	), `
	select UserCC,Qtype,sum(queries) as queries
	from by_server_ip_1d
	where
		ServerIP = ? AND dt > now() - INTERVAL ? DAY
		group by rollup(Qtype,UserCC)
		order by UserCC,Qtype`,
		serverIP, days,
	)
	if err != nil {
		log.Error("query error", "err", err)
		return nil, fmt.Errorf("database error")
	}

	rv := ServerQueries{}

	for rows.Next() {
		var (
			UserCC, Qtype string
			queries       uint64
		)
		if err := rows.Scan(
			&UserCC,
			&Qtype,
			&queries,
		); err != nil {
			log.Error("could not parse row", "err", err)
			continue
		}

		if UserCC == "" && Qtype != "" {
			// we get the total from the complete rollup
			continue
		}

		// log.Info("usercc counts", "cc", UserCC, "counts", c)

		rv = append(rv, &ccCount{
			CC:    UserCC,
			Count: queries,
		})

		// slog.Info("set c", "c", c)
		// slog.Info("totals", "totals", totals)
	}

	sort.Sort(rv)

	return rv, nil
}

func (d *ClickHouse) AnswerTotals(ctx context.Context, qtype string, days int) (ServerTotals, error) {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "AnswerTotals")
	defer span.End()

	// queries by UserCC / Qtype for the ServerIP
	rows, err := d.Logs.Query(clickhouse.Context(ctx,
		clickhouse.WithSpan(span.SpanContext()),
	), `
	select UserCC,Qtype,sum(queries) as queries
	from by_server_ip_1d
	where
		Qtype = ? AND dt > now() - INTERVAL ? DAY
		group by rollup(Qtype,UserCC)
		order by UserCC,Qtype`,
		qtype, days,
	)
	if err != nil {
		log.Error("query error", "err", err)
		return nil, fmt.Errorf("database error")
	}

	rv := ServerTotals{}

	for rows.Next() {
		var (
			UserCC, Qtype string
			queries       uint64
		)
		if err := rows.Scan(
			&UserCC,
			&Qtype,
			&queries,
		); err != nil {
			log.Error("could not parse row", "err", err)
			continue
		}

		if UserCC == "" && Qtype != "" {
			// we get the total from the complete rollup
			continue
		}

		// log.Info("usercc counts", "cc", UserCC, "counts", c)

		if rv[UserCC] > 0 {
			log.Warn("duplicate UserCC row", "usercc", UserCC)
		}

		rv[UserCC] = queries

		// slog.Info("set c", "c", c)
		// slog.Info("totals", "totals", totals)
	}

	return rv, nil
}
