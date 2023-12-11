package logscores

import (
	"context"
	"database/sql"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/data-api/ntpdb"
)

type LogScoreHistory struct {
	LogScores []ntpdb.LogScore
	Monitors  map[int]string
}

func GetHistory(ctx context.Context, db *sql.DB, serverID, monitorID uint32, since time.Time, count int) (*LogScoreHistory, error) {
	log := logger.Setup()

	if count == 0 {
		count = 200
	}

	log.Debug("GetHistory", "server", serverID, "monitor", monitorID, "since", since, "count", count)

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

	return &LogScoreHistory{
		LogScores: ls,
		Monitors:  monitors,
	}, nil
}

/*



 */
