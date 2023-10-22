package chdb

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
)

type ClickHouse struct {
	conn clickhouse.Conn
}

func New(ctx context.Context, dbConfigPath string) (*ClickHouse, error) {
	conn, err := setupClickhouse(ctx)
	if err != nil {
		return nil, err
	}
	return &ClickHouse{conn: conn}, nil
}

func setupClickhouse(ctx context.Context) (driver.Conn, error) {

	log := logger.Setup()

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
	log.Info("clickhouse connection", "version", v)

	err = conn.Ping(ctx)
	if err != nil {
		log.Error("clickhouse ping", "err", err)
		return nil, err
	}

	return conn, nil
}
