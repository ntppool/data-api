package chdb

import (
	"context"
	"os"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gopkg.in/yaml.v3"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
)

type Config struct {
	ClickHouse struct {
		Scores DBConfig `yaml:"scores"`
		Logs   DBConfig `yaml:"logs"`
	} `yaml:"clickhouse"`
}

type DBConfig struct {
	Host     string
	Database string
}

type ClickHouse struct {
	Logs   clickhouse.Conn
	Scores clickhouse.Conn
}

func New(ctx context.Context, dbConfigPath string) (*ClickHouse, error) {
	ch, err := setupClickhouse(ctx, dbConfigPath)
	if err != nil {
		return nil, err
	}
	return ch, nil
}

func setupClickhouse(ctx context.Context, configFile string) (*ClickHouse, error) {
	log := logger.FromContext(ctx)

	log.DebugContext(ctx, "opening ch config", "file", configFile)

	dbFile, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}

	dec := yaml.NewDecoder(dbFile)

	cfg := Config{}

	err = dec.Decode(&cfg)
	if err != nil {
		return nil, err
	}

	ch := &ClickHouse{}

	ch.Logs, err = open(ctx, cfg.ClickHouse.Logs)
	if err != nil {
		return nil, err
	}
	ch.Scores, err = open(ctx, cfg.ClickHouse.Scores)
	if err != nil {
		return nil, err
	}

	return ch, nil
}

func open(ctx context.Context, cfg DBConfig) (clickhouse.Conn, error) {
	log := logger.Setup()

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:     []string{cfg.Host + ":9000"},
		Protocol: clickhouse.Native,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
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
		MaxOpenConns:         8,
		MaxIdleConns:         3,
		ConnMaxLifetime:      5 * time.Minute,
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
