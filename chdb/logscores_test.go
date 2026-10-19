package chdb

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// queryRecorder records the query and args of each Query call and fails it.
type queryRecorder struct {
	clickhouse.Conn
	query string
	args  []any
}

func (q *queryRecorder) Query(_ context.Context, query string, args ...any) (clickhousedriver.Rows, error) {
	q.query = query
	q.args = args
	return nil, errors.New("recorded")
}

func countWeekPredicates(query string) int {
	return strings.Count(query, "toStartOfWeek(ts)")
}

func TestLogscoresWeekBound(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	// Sunday 05:00 in Tokyo is still Saturday in UTC.
	since := time.Date(2026, 9, 13, 5, 0, 0, 0, tokyo)

	for _, monitorID := range []int{0, 24} {
		rec := &queryRecorder{}
		ch := &ClickHouse{Scores: rec}
		_, _ = ch.Logscores(context.Background(), 21950, monitorID, since, 100, false)

		if n := countWeekPredicates(rec.query); n != 1 {
			t.Errorf("monitor %d: %d toStartOfWeek(ts) predicates, want 1: %s", monitorID, n, rec.query)
		}
		if !strings.Contains(rec.query, "toStartOfWeek(ts) >= toStartOfWeek(toDateTime(?))") {
			t.Errorf("monitor %d: week lower bound missing: %s", monitorID, rec.query)
		}
		if got, want := strings.Count(rec.query, "?"), len(rec.args); got != want {
			t.Fatalf("monitor %d: %d placeholders, %d args", monitorID, got, want)
		}
		// args end with since, since as Unix seconds, limit
		n := len(rec.args)
		if got := rec.args[n-3]; got != since {
			t.Errorf("monitor %d: ts arg = %v, want %v", monitorID, got, since)
		}
		if got := rec.args[n-2]; got != since.Unix() {
			t.Errorf("monitor %d: week arg = %#v, want int64 %d", monitorID, got, since.Unix())
		}
	}
}

func TestLogscoresFullHistoryHasNoWeekBound(t *testing.T) {
	rec := &queryRecorder{}
	ch := &ClickHouse{Scores: rec}
	_, _ = ch.Logscores(context.Background(), 21950, 24, time.Time{}, 0, true)

	if n := countWeekPredicates(rec.query); n != 0 {
		t.Errorf("%d toStartOfWeek(ts) predicates, want 0: %s", n, rec.query)
	}
}

func TestLogscoresTimeRangeWeekBounds(t *testing.T) {
	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 9, 12, 20, 0, 0, 0, la)
	to := time.Date(2026, 9, 19, 20, 0, 0, 0, la)

	for _, monitorID := range []int{0, 24} {
		rec := &queryRecorder{}
		ch := &ClickHouse{Scores: rec}
		_, _ = ch.LogscoresTimeRange(context.Background(), 21950, monitorID, from, to, 1000)

		if n := countWeekPredicates(rec.query); n != 2 {
			t.Errorf("monitor %d: %d toStartOfWeek(ts) predicates, want 2: %s", monitorID, n, rec.query)
		}
		if got, want := strings.Count(rec.query, "?"), len(rec.args); got != want {
			t.Fatalf("monitor %d: %d placeholders, %d args", monitorID, got, want)
		}
		// server_id, from, to, from as Unix seconds, to as Unix seconds, ...
		want := []any{21950, from, to, from.Unix(), to.Unix()}
		for i, w := range want {
			if rec.args[i] != w {
				t.Errorf("monitor %d: arg %d = %#v, want %#v", monitorID, i, rec.args[i], w)
			}
		}
	}
}
