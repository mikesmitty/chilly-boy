package watchdog

import (
	"log/slog"
	"time"
)

func NewWatchdog[T any](interval time.Duration, shutdown func() error, input <-chan T) func() error {
	return func() error {
		t := time.NewTimer(interval)
		awake := true
		slog.Debug("watchdog started", "timeout", interval)
		for {
			select {
			case <-input:
				awake = true
			case <-t.C:
				if !awake {
					slog.Error("watchdog timeout, shutting down chiller", "timeout", interval)
					if err := shutdown(); err != nil {
						return err
					}
				}
				awake = false
			}
		}
	}
}
