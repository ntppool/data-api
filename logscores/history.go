package logscores

import (
	"context"
	"database/sql"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/data-api/chdb"
	"go.ntppool.org/data-api/ntpdb"
	"go.opentelemetry.io/otel/attribute"
)

type LogScoreHistory struct {
	LogScores []ntpdb.LogScore
	Monitors  map[int]string
	// MonitorIDs []uint32
}

func GetHistoryClickHouse(ctx context.Context, ch *chdb.ClickHouse, db *sql.DB, serverID, monitorID uint32, since time.Time, count int) (*LogScoreHistory, error) {
	log := logger.FromContext(ctx)
	ctx, span := tracing.Tracer().Start(ctx, "logscores.GetHistoryClickHouse")
	defer span.End()

	span.SetAttributes(
		attribute.Int("server", int(serverID)),
		attribute.Int("monitor", int(monitorID)),
	)

	log.Debug("GetHistoryCH", "server", serverID, "monitor", monitorID, "since", since, "count", count)

	ls, err := ch.Logscores(ctx, int(serverID), int(monitorID), since, count)

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
		// MonitorIDs: monitorIDs,
	}, nil
}

func GetHistoryMySQL(ctx context.Context, db *sql.DB, serverID, monitorID uint32, since time.Time, count int) (*LogScoreHistory, error) {
	log := logger.FromContext(ctx)
	ctx, span := tracing.Tracer().Start(ctx, "logscores.GetHistoryMySQL")
	defer span.End()

	span.SetAttributes(
		attribute.Int("server", int(serverID)),
		attribute.Int("monitor", int(monitorID)),
	)

	log.Debug("GetHistoryMySQL", "server", serverID, "monitor", monitorID, "since", since, "count", count)

	q := ntpdb.NewWrappedQuerier(ntpdb.New(db))

	var ls []ntpdb.LogScore
	var err error
	if monitorID > 0 {
		ls, err = q.GetServerLogScoresByMonitorID(ctx, ntpdb.GetServerLogScoresByMonitorIDParams{
			ServerID:  serverID,
			MonitorID: sql.NullInt32{Int32: int32(monitorID), Valid: true},
			Limit:     int32(count),
		})
	} else {
		ls, err = q.GetServerLogScores(ctx, ntpdb.GetServerLogScoresParams{
			ServerID: serverID,
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
	monitorIDs := []uint32{}
	for _, l := range ls {
		if !l.MonitorID.Valid {
			continue
		}
		mID := uint32(l.MonitorID.Int32)
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
