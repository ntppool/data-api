package logscores

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/data-api/chdb"
	"go.ntppool.org/data-api/ntpdb"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type LogScoreHistory struct {
	LogScores []ntpdb.LogScore
	Monitors  map[int]string
	// MonitorIDs []uint32
}

func GetHistoryClickHouse(ctx context.Context, ch *chdb.ClickHouse, db *pgxpool.Pool, serverID, monitorID int64, since time.Time, count int, fullHistory bool) (*LogScoreHistory, error) {
	log := logger.FromContext(ctx)
	ctx, span := tracing.Tracer().Start(ctx, "logscores.GetHistoryClickHouse",
		trace.WithAttributes(
			attribute.Int64("server", serverID),
			attribute.Int64("monitor", monitorID),
			attribute.Bool("full_history", fullHistory),
		),
	)
	defer span.End()

	log.DebugContext(ctx, "GetHistoryCH", "server", serverID, "monitor", monitorID, "since", since, "count", count, "full_history", fullHistory)

	ls, err := ch.Logscores(ctx, int(serverID), int(monitorID), since, count, fullHistory)
	if err != nil {
		log.ErrorContext(ctx, "clickhouse logscores", "err", err)
		return nil, err
	}

	q := ntpdb.NewWrappedQuerier(ntpdb.New(db))

	monitors, err := getMonitorNames(ctx, ls, q)
	if err != nil {
		return nil, err
	}

	return &LogScoreHistory{
		LogScores: ls,
		Monitors:  monitors,
	}, nil
}

func GetHistoryPostgres(ctx context.Context, db *pgxpool.Pool, serverID, monitorID int64, since time.Time, count int) (*LogScoreHistory, error) {
	log := logger.FromContext(ctx)
	ctx, span := tracing.Tracer().Start(ctx, "logscores.GetHistoryPostgres")
	defer span.End()

	span.SetAttributes(
		attribute.Int64("server", serverID),
		attribute.Int64("monitor", monitorID),
	)

	log.Debug("GetHistoryPostgres", "server", serverID, "monitor", monitorID, "since", since, "count", count)

	q := ntpdb.NewWrappedQuerier(ntpdb.New(db))

	var ls []ntpdb.LogScore
	var err error
	if monitorID > 0 {
		ls, err = q.GetServerLogScoresByMonitorID(ctx, ntpdb.GetServerLogScoresByMonitorIDParams{
			ServerID:  int64(serverID),
			MonitorID: pgtype.Int8{Int64: int64(monitorID), Valid: true},
			Limit:     int32(count),
		})
	} else {
		ls, err = q.GetServerLogScores(ctx, ntpdb.GetServerLogScoresParams{
			ServerID: int64(serverID),
			Limit:    int32(count),
		})
	}
	if err != nil {
		return nil, err
	}

	monitors, err := getMonitorNames(ctx, ls, q)
	if err != nil {
		return nil, err
	}

	return &LogScoreHistory{
		LogScores: ls,
		Monitors:  monitors,
		// MonitorIDs: monitorIDs,
	}, nil
}

func getMonitorNames(ctx context.Context, ls []ntpdb.LogScore, q ntpdb.QuerierTx) (map[int]string, error) {
	monitors := map[int]string{}
	monitorIDs := []int64{}
	for _, l := range ls {
		if !l.MonitorID.Valid {
			continue
		}
		mID := l.MonitorID.Int64
		if _, ok := monitors[int(mID)]; !ok {
			monitors[int(mID)] = ""
			monitorIDs = append(monitorIDs, mID)
		}
	}

	dbmons, err := q.GetMonitorsByID(ctx, monitorIDs)
	if err != nil {
		return nil, err
	}
	for _, m := range dbmons {
		monitors[int(m.ID)] = m.DisplayName()
	}
	return monitors, nil
}
