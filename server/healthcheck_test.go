package server

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestRunHealthCheckDoesNotLogFastSuccess(t *testing.T) {
	handler := newRecordHandler()
	log := slog.New(handler)

	err := runHealthCheck(context.Background(), log, "clickhouse.logs", time.Hour, nil, func(context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("runHealthCheck() error = %v", err)
	}

	select {
	case record := <-handler.records:
		t.Fatalf("runHealthCheck() unexpectedly logged %q", record.Message)
	default:
	}
}

func TestRunHealthCheckLogsWhileSlowAndWhenFinished(t *testing.T) {
	handler := newRecordHandler()
	log := slog.New(handler)
	release := make(chan struct{})
	result := make(chan error, 1)

	go func() {
		result <- runHealthCheck(context.Background(), log, "clickhouse.scores", time.Millisecond, func() []any {
			return []any{"open_connections", 8, "idle_connections", 0}
		}, func(context.Context) error {
			<-release
			return nil
		})
	}()

	slow := nextRecord(t, handler.records)
	if slow.Message != "health check slow" {
		t.Fatalf("slow log message = %q, want %q", slow.Message, "health check slow")
	}
	assertRecordAttribute(t, slow, "check", "clickhouse.scores")
	assertRecordAttribute(t, slow, "open_connections", int64(8))
	assertRecordAttribute(t, slow, "idle_connections", int64(0))

	close(release)
	if err := <-result; err != nil {
		t.Fatalf("runHealthCheck() error = %v", err)
	}

	finished := nextRecord(t, handler.records)
	if finished.Message != "health check completed after delay" {
		t.Fatalf("completion log message = %q, want %q", finished.Message, "health check completed after delay")
	}
	assertRecordAttribute(t, finished, "check", "clickhouse.scores")
}

func TestRunHealthCheckLogsFailure(t *testing.T) {
	handler := newRecordHandler()
	log := slog.New(handler)
	wantErr := errors.New("mesh connection reset")

	err := runHealthCheck(context.Background(), log, "clickhouse.logs", time.Hour, func() []any {
		return []any{"open_connections", 1, "idle_connections", 0}
	}, func(context.Context) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("runHealthCheck() error = %v, want %v", err, wantErr)
	}

	failed := nextRecord(t, handler.records)
	if failed.Message != "health check failed" {
		t.Fatalf("failure log message = %q, want %q", failed.Message, "health check failed")
	}
	assertRecordAttribute(t, failed, "check", "clickhouse.logs")
	assertRecordAttribute(t, failed, "err", wantErr.Error())
	assertRecordAttribute(t, failed, "context_err", "")
	assertRecordAttribute(t, failed, "open_connections", int64(1))
	assertRecordAttribute(t, failed, "idle_connections", int64(0))
	if duration := recordAttribute(failed, "duration"); duration == nil {
		t.Fatal("failure log has no duration attribute")
	}
}

func TestHealthCheckWasSlowUsesElapsedDurationWhenTimerHasNotFired(t *testing.T) {
	if !healthCheckWasSlow(3*time.Second, 2*time.Second, false) {
		t.Fatal("healthCheckWasSlow() = false for a check exceeding the slow threshold")
	}
	if healthCheckWasSlow(time.Second, 2*time.Second, false) {
		t.Fatal("healthCheckWasSlow() = true for a check below the slow threshold")
	}
}

type recordHandler struct {
	records chan slog.Record
}

func newRecordHandler() *recordHandler {
	return &recordHandler{records: make(chan slog.Record, 10)}
}

func (h *recordHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *recordHandler) Handle(_ context.Context, record slog.Record) error {
	h.records <- record.Clone()
	return nil
}

func (h *recordHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h *recordHandler) WithGroup(string) slog.Handler {
	return h
}

func nextRecord(t *testing.T, records <-chan slog.Record) slog.Record {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for log record")
		return slog.Record{}
	}
}

func assertRecordAttribute(t *testing.T, record slog.Record, key string, want any) {
	t.Helper()
	got := recordAttribute(record, key)
	if got != want {
		t.Errorf("log attribute %q = %#v, want %#v", key, got, want)
	}
}

func recordAttribute(record slog.Record, key string) any {
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
