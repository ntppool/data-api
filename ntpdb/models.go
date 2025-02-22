// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.28.0

package ntpdb

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"

	"go.ntppool.org/common/types"
)

type MonitorsIpVersion string

const (
	MonitorsIpVersionV4 MonitorsIpVersion = "v4"
	MonitorsIpVersionV6 MonitorsIpVersion = "v6"
)

func (e *MonitorsIpVersion) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = MonitorsIpVersion(s)
	case string:
		*e = MonitorsIpVersion(s)
	default:
		return fmt.Errorf("unsupported scan type for MonitorsIpVersion: %T", src)
	}
	return nil
}

type NullMonitorsIpVersion struct {
	MonitorsIpVersion MonitorsIpVersion `json:"monitors_ip_version"`
	Valid             bool              `json:"valid"` // Valid is true if MonitorsIpVersion is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullMonitorsIpVersion) Scan(value interface{}) error {
	if value == nil {
		ns.MonitorsIpVersion, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.MonitorsIpVersion.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullMonitorsIpVersion) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.MonitorsIpVersion), nil
}

type MonitorsStatus string

const (
	MonitorsStatusPending MonitorsStatus = "pending"
	MonitorsStatusTesting MonitorsStatus = "testing"
	MonitorsStatusActive  MonitorsStatus = "active"
	MonitorsStatusPaused  MonitorsStatus = "paused"
	MonitorsStatusDeleted MonitorsStatus = "deleted"
)

func (e *MonitorsStatus) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = MonitorsStatus(s)
	case string:
		*e = MonitorsStatus(s)
	default:
		return fmt.Errorf("unsupported scan type for MonitorsStatus: %T", src)
	}
	return nil
}

type NullMonitorsStatus struct {
	MonitorsStatus MonitorsStatus `json:"monitors_status"`
	Valid          bool           `json:"valid"` // Valid is true if MonitorsStatus is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullMonitorsStatus) Scan(value interface{}) error {
	if value == nil {
		ns.MonitorsStatus, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.MonitorsStatus.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullMonitorsStatus) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.MonitorsStatus), nil
}

type MonitorsType string

const (
	MonitorsTypeMonitor MonitorsType = "monitor"
	MonitorsTypeScore   MonitorsType = "score"
)

func (e *MonitorsType) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = MonitorsType(s)
	case string:
		*e = MonitorsType(s)
	default:
		return fmt.Errorf("unsupported scan type for MonitorsType: %T", src)
	}
	return nil
}

type NullMonitorsType struct {
	MonitorsType MonitorsType `json:"monitors_type"`
	Valid        bool         `json:"valid"` // Valid is true if MonitorsType is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullMonitorsType) Scan(value interface{}) error {
	if value == nil {
		ns.MonitorsType, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.MonitorsType.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullMonitorsType) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.MonitorsType), nil
}

type ServerScoresStatus string

const (
	ServerScoresStatusNew     ServerScoresStatus = "new"
	ServerScoresStatusTesting ServerScoresStatus = "testing"
	ServerScoresStatusActive  ServerScoresStatus = "active"
)

func (e *ServerScoresStatus) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = ServerScoresStatus(s)
	case string:
		*e = ServerScoresStatus(s)
	default:
		return fmt.Errorf("unsupported scan type for ServerScoresStatus: %T", src)
	}
	return nil
}

type NullServerScoresStatus struct {
	ServerScoresStatus ServerScoresStatus `json:"server_scores_status"`
	Valid              bool               `json:"valid"` // Valid is true if ServerScoresStatus is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullServerScoresStatus) Scan(value interface{}) error {
	if value == nil {
		ns.ServerScoresStatus, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.ServerScoresStatus.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullServerScoresStatus) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.ServerScoresStatus), nil
}

type ServersIpVersion string

const (
	ServersIpVersionV4 ServersIpVersion = "v4"
	ServersIpVersionV6 ServersIpVersion = "v6"
)

func (e *ServersIpVersion) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = ServersIpVersion(s)
	case string:
		*e = ServersIpVersion(s)
	default:
		return fmt.Errorf("unsupported scan type for ServersIpVersion: %T", src)
	}
	return nil
}

type NullServersIpVersion struct {
	ServersIpVersion ServersIpVersion `json:"servers_ip_version"`
	Valid            bool             `json:"valid"` // Valid is true if ServersIpVersion is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullServersIpVersion) Scan(value interface{}) error {
	if value == nil {
		ns.ServersIpVersion, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.ServersIpVersion.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullServersIpVersion) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.ServersIpVersion), nil
}

type ZoneServerCountsIpVersion string

const (
	ZoneServerCountsIpVersionV4 ZoneServerCountsIpVersion = "v4"
	ZoneServerCountsIpVersionV6 ZoneServerCountsIpVersion = "v6"
)

func (e *ZoneServerCountsIpVersion) Scan(src interface{}) error {
	switch s := src.(type) {
	case []byte:
		*e = ZoneServerCountsIpVersion(s)
	case string:
		*e = ZoneServerCountsIpVersion(s)
	default:
		return fmt.Errorf("unsupported scan type for ZoneServerCountsIpVersion: %T", src)
	}
	return nil
}

type NullZoneServerCountsIpVersion struct {
	ZoneServerCountsIpVersion ZoneServerCountsIpVersion `json:"zone_server_counts_ip_version"`
	Valid                     bool                      `json:"valid"` // Valid is true if ZoneServerCountsIpVersion is not NULL
}

// Scan implements the Scanner interface.
func (ns *NullZoneServerCountsIpVersion) Scan(value interface{}) error {
	if value == nil {
		ns.ZoneServerCountsIpVersion, ns.Valid = "", false
		return nil
	}
	ns.Valid = true
	return ns.ZoneServerCountsIpVersion.Scan(value)
}

// Value implements the driver Valuer interface.
func (ns NullZoneServerCountsIpVersion) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return string(ns.ZoneServerCountsIpVersion), nil
}

type LogScore struct {
	ID         uint64                   `db:"id" json:"id"`
	MonitorID  sql.NullInt32            `db:"monitor_id" json:"monitor_id"`
	ServerID   uint32                   `db:"server_id" json:"server_id"`
	Ts         time.Time                `db:"ts" json:"ts"`
	Score      float64                  `db:"score" json:"score"`
	Step       float64                  `db:"step" json:"step"`
	Offset     sql.NullFloat64          `db:"offset" json:"offset"`
	Rtt        sql.NullInt32            `db:"rtt" json:"rtt"`
	Attributes types.LogScoreAttributes `db:"attributes" json:"attributes"`
}

type Monitor struct {
	ID            uint32                `db:"id" json:"id"`
	Type          MonitorsType          `db:"type" json:"type"`
	UserID        sql.NullInt32         `db:"user_id" json:"user_id"`
	AccountID     sql.NullInt32         `db:"account_id" json:"account_id"`
	Name          string                `db:"name" json:"name"`
	Location      string                `db:"location" json:"location"`
	Ip            sql.NullString        `db:"ip" json:"ip"`
	IpVersion     NullMonitorsIpVersion `db:"ip_version" json:"ip_version"`
	TlsName       sql.NullString        `db:"tls_name" json:"tls_name"`
	ApiKey        sql.NullString        `db:"api_key" json:"api_key"`
	Status        MonitorsStatus        `db:"status" json:"status"`
	Config        string                `db:"config" json:"config"`
	ClientVersion string                `db:"client_version" json:"client_version"`
	LastSeen      sql.NullTime          `db:"last_seen" json:"last_seen"`
	LastSubmit    sql.NullTime          `db:"last_submit" json:"last_submit"`
	CreatedOn     time.Time             `db:"created_on" json:"created_on"`
}

type Server struct {
	ID             uint32           `db:"id" json:"id"`
	Ip             string           `db:"ip" json:"ip"`
	IpVersion      ServersIpVersion `db:"ip_version" json:"ip_version"`
	UserID         sql.NullInt32    `db:"user_id" json:"user_id"`
	AccountID      sql.NullInt32    `db:"account_id" json:"account_id"`
	Hostname       sql.NullString   `db:"hostname" json:"hostname"`
	Stratum        sql.NullInt16    `db:"stratum" json:"stratum"`
	InPool         uint8            `db:"in_pool" json:"in_pool"`
	InServerList   uint8            `db:"in_server_list" json:"in_server_list"`
	Netspeed       uint32           `db:"netspeed" json:"netspeed"`
	NetspeedTarget uint32           `db:"netspeed_target" json:"netspeed_target"`
	CreatedOn      time.Time        `db:"created_on" json:"created_on"`
	UpdatedOn      time.Time        `db:"updated_on" json:"updated_on"`
	ScoreTs        sql.NullTime     `db:"score_ts" json:"score_ts"`
	ScoreRaw       float64          `db:"score_raw" json:"score_raw"`
	DeletionOn     sql.NullTime     `db:"deletion_on" json:"deletion_on"`
	Flags          string           `db:"flags" json:"flags"`
}

type Zone struct {
	ID          uint32         `db:"id" json:"id"`
	Name        string         `db:"name" json:"name"`
	Description sql.NullString `db:"description" json:"description"`
	ParentID    sql.NullInt32  `db:"parent_id" json:"parent_id"`
	Dns         bool           `db:"dns" json:"dns"`
}

type ZoneServerCount struct {
	ID              uint32                    `db:"id" json:"id"`
	ZoneID          uint32                    `db:"zone_id" json:"zone_id"`
	IpVersion       ZoneServerCountsIpVersion `db:"ip_version" json:"ip_version"`
	Date            time.Time                 `db:"date" json:"date"`
	CountActive     uint32                    `db:"count_active" json:"count_active"`
	CountRegistered uint32                    `db:"count_registered" json:"count_registered"`
	NetspeedActive  uint32                    `db:"netspeed_active" json:"netspeed_active"`
}
