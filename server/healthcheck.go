package server

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const healthCheckSlowThreshold = 2 * time.Second

func runHealthCheck(
	ctx context.Context,
	log *slog.Logger,
	name string,
	slowThreshold time.Duration,
	details func() []any,
	check func(context.Context) error,
) error {
	started := time.Now()

	var state struct {
		sync.Mutex
		finished bool
		slow     bool
	}

	timer := time.AfterFunc(slowThreshold, func() {
		state.Lock()
		defer state.Unlock()

		if state.finished {
			return
		}
		state.slow = true
		log.WarnContext(ctx, "health check slow", healthCheckLogArgs(ctx, name, time.Since(started), nil, details)...)
	})

	err := check(ctx)
	duration := time.Since(started)

	state.Lock()
	state.finished = true
	wasSlow := state.slow
	timer.Stop()
	state.Unlock()

	if err != nil {
		log.WarnContext(ctx, "health check failed", healthCheckLogArgs(ctx, name, duration, err, details)...)
	} else if healthCheckWasSlow(duration, slowThreshold, wasSlow) {
		log.InfoContext(ctx, "health check completed after delay", healthCheckLogArgs(ctx, name, duration, nil, details)...)
	}

	return err
}

func healthCheckWasSlow(duration, threshold time.Duration, warningLogged bool) bool {
	return warningLogged || duration >= threshold
}

func healthCheckLogArgs(ctx context.Context, name string, duration time.Duration, err error, details func() []any) []any {
	args := []any{
		"check", name,
		"duration", duration,
		"context_err", contextError(ctx),
	}
	if err != nil {
		args = append(args, "err", err.Error())
	}
	if details != nil {
		args = append(args, details()...)
	}
	return args
}

func contextError(ctx context.Context) string {
	if err := ctx.Err(); err != nil {
		return err.Error()
	}
	return ""
}
