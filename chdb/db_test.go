package chdb

import (
	"context"
	"log/slog"
	"testing"
	"time"

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func TestVerifyClickhouseConnectionLogsBeforeHandshakeAndAfterPing(t *testing.T) {
	handler := newDBRecordHandler()
	log := slog.New(handler)
	started := make(chan struct{})
	release := make(chan struct{})
	result := make(chan error, 1)
	conn := &connectionVerifierStub{
		serverVersion: func() (*clickhousedriver.ServerVersion, error) {
			close(started)
			<-release
			return &clickhousedriver.ServerVersion{}, nil
		},
		ping: func(context.Context) error {
			return nil
		},
	}

	go func() {
		result <- verifyClickhouseConnection(
			context.Background(),
			log,
			"logs",
			conn,
			[]any{"addresses", []string{"clickhouse.example:9440"}, "database", "dns"},
		)
	}()

	<-started
	opening := nextDBRecord(t, handler.records)
	if opening.Message != "opening clickhouse connection" {
		t.Fatalf("opening log message = %q, want %q", opening.Message, "opening clickhouse connection")
	}
	assertDBRecordAttribute(t, opening, "connection", "logs")
	assertDBRecordAttribute(t, opening, "database", "dns")

	close(release)
	if err := <-result; err != nil {
		t.Fatalf("verifyClickhouseConnection() error = %v", err)
	}

	connected := nextDBRecord(t, handler.records)
	if connected.Message != "clickhouse connection" {
		t.Fatalf("completion log message = %q, want %q", connected.Message, "clickhouse connection")
	}
	assertDBRecordAttribute(t, connected, "connection", "logs")
	for _, key := range []string{"handshake_duration", "ping_duration", "total_duration"} {
		if value := dbRecordAttribute(connected, key); value == nil {
			t.Errorf("completion log has no %q attribute", key)
		}
	}
}

type connectionVerifierStub struct {
	serverVersion func() (*clickhousedriver.ServerVersion, error)
	ping          func(context.Context) error
}

func (c *connectionVerifierStub) ServerVersion() (*clickhousedriver.ServerVersion, error) {
	return c.serverVersion()
}

func (c *connectionVerifierStub) Ping(ctx context.Context) error {
	return c.ping(ctx)
}

type dbRecordHandler struct {
	records chan slog.Record
}

func newDBRecordHandler() *dbRecordHandler {
	return &dbRecordHandler{records: make(chan slog.Record, 10)}
}

func (h *dbRecordHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *dbRecordHandler) Handle(_ context.Context, record slog.Record) error {
	h.records <- record.Clone()
	return nil
}

func (h *dbRecordHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *dbRecordHandler) WithGroup(string) slog.Handler {
	return h
}

func nextDBRecord(t *testing.T, records <-chan slog.Record) slog.Record {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for log record")
		return slog.Record{}
	}
}

func assertDBRecordAttribute(t *testing.T, record slog.Record, key string, want any) {
	t.Helper()
	got := dbRecordAttribute(record, key)
	if got != want {
		t.Errorf("log attribute %q = %#v, want %#v", key, got, want)
	}
}

func dbRecordAttribute(record slog.Record, key string) any {
	var value any
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == key {
			value = attr.Value.Any()
			return false
		}
		return true
	})
	return value
}
