package ntpdb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.ntppool.org/common/logger"
	"gopkg.in/yaml.v3"
)

type Config struct {
	MySQL DBConfig `yaml:"mysql"`
}

type DBConfig struct {
	DSN  string `default:"" flag:"dsn" usage:"Database DSN"`
	User string `default:"" flag:"user"`
	Pass string `default:"" flag:"pass"`
}

func OpenDB(ctx context.Context, configFile string) (*sql.DB, error) {
	log := logger.FromContext(ctx)

	dbconn := sql.OpenDB(Driver{CreateConnectorFunc: createConnector(ctx, configFile)})

	dbconn.SetConnMaxLifetime(time.Minute * 3)
	dbconn.SetMaxOpenConns(8)
	dbconn.SetMaxIdleConns(3)

	err := dbconn.Ping()
	if err != nil {
		log.DebugContext(ctx, "could not connect to database: %s", "err", err)
		return nil, err
	}

	return dbconn, nil
}

func createConnector(ctx context.Context, configFile string) CreateConnectorFunc {
	log := logger.FromContext(ctx)
	return func() (driver.Connector, error) {
		log.DebugContext(ctx, "opening db config file", "filename", configFile)

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

		// log.Printf("db cfg: %+v", cfg)

		dsn := cfg.MySQL.DSN
		if len(dsn) == 0 {
			return nil, fmt.Errorf("--database.dsn flag or DATABASE_DSN environment variable required")
		}

		dbcfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, err
		}

		if user := cfg.MySQL.User; len(user) > 0 {
			dbcfg.User = user
		}

		if pass := cfg.MySQL.Pass; len(pass) > 0 {
			dbcfg.Passwd = pass
		}

		return mysql.NewConnector(dbcfg)
	}
}
