package server

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.ntppool.org/common/version"
	"golang.org/x/exp/slog"
)

func (srv *Server) chConn(ctx context.Context) (driver.Conn, error) {

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{"10.43.207.123:9000"},
		Auth: clickhouse.Auth{
			Database: "geodns3",
			Username: "default",
			Password: "",
		},
		// Debug: true,
		// Debugf: func(format string, v ...interface{}) {
		// 	slog.Info("debug format", "format", format)
		// 	fmt.Printf(format+"\n", v)
		// },
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		DialTimeout:          time.Second * 5,
		MaxOpenConns:         5,
		MaxIdleConns:         5,
		ConnMaxLifetime:      time.Duration(10) * time.Minute,
		ConnOpenStrategy:     clickhouse.ConnOpenInOrder,
		BlockBufferSize:      10,
		MaxCompressionBuffer: 10240,
		ClientInfo: clickhouse.ClientInfo{
			Products: []struct {
				Name    string
				Version string
			}{
				{Name: "data-api", Version: version.Version()},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	v, err := conn.ServerVersion()
	if err != nil {
		return nil, err
	}
	slog.Info("clickhouse connection", "version", v)

	err = conn.Ping(ctx)
	if err != nil {
		slog.Error("clickhouse ping", "err", err)
		return nil, err
	}

	return conn, nil
}
