// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.26.0

package ntpdb

import (
	"context"
	"database/sql"
)

type Querier interface {
	GetMonitorByName(ctx context.Context, tlsName sql.NullString) (Monitor, error)
	GetMonitorsByID(ctx context.Context, monitorids []uint32) ([]Monitor, error)
	GetServerByID(ctx context.Context, id uint32) (Server, error)
	GetServerByIP(ctx context.Context, ip string) (Server, error)
	GetServerLogScores(ctx context.Context, arg GetServerLogScoresParams) ([]LogScore, error)
	GetServerLogScoresByMonitorID(ctx context.Context, arg GetServerLogScoresByMonitorIDParams) ([]LogScore, error)
	GetServerNetspeed(ctx context.Context, ip string) (uint32, error)
	GetServerScores(ctx context.Context, arg GetServerScoresParams) ([]GetServerScoresRow, error)
	GetZoneByName(ctx context.Context, name string) (Zone, error)
	GetZoneCounts(ctx context.Context, zoneID uint32) ([]ZoneServerCount, error)
	GetZoneStatsData(ctx context.Context) ([]GetZoneStatsDataRow, error)
	GetZoneStatsV2(ctx context.Context, ip string) ([]GetZoneStatsV2Row, error)
}

var _ Querier = (*Queries)(nil)
