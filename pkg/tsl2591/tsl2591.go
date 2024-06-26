package tsl2591

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"time"

	tsl "github.com/JenswBE/golang-tsl2591"
)

func LightChannel(ctx context.Context, dev *tsl.TSL2591, interval time.Duration) (<-chan float64, func() error) {
	c := make(chan float64, 1)
	ctx, cancelFunc := context.WithCancel(ctx)
	return c, func() error {
		defer cancelFunc()
		done := ctx.Done()
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-done:
				return nil
			case <-ticker.C:
				ir, err := dev.Infrared()
				if err != nil {
					return fmt.Errorf("tsl2591: %w", err)
				}
				slog.Debug("publishing reading", "ir", ir, "module", "tsl2591")
				c <- float64(ir)
			}
		}
	}
}

func TickerPrint(ctx context.Context, tsl *tsl.TSL2591, lightInterval time.Duration) func() error {
	ctx, cancelFunc := context.WithCancel(ctx)
	return func() error {
		defer cancelFunc()
		ticker := time.NewTicker(lightInterval)
		done := ctx.Done()
		for {
			select {
			case <-done:
				return nil
			case <-ticker.C:
				ir, err := tsl.Infrared()
				if err != nil {
					return err
				}
				log.Printf("Infrared light: %d\n", ir)
			}
		}
	}
}
