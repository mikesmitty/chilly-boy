package cmhmax31865

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mikesmitty/max31865"
	"periph.io/x/conn/v3/physic"
)

func TemperatureChannel(ctx context.Context, dev *max31865.Dev, interval time.Duration) (<-chan float64, func() error) {
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
				var e physic.Env
				err := dev.Sense(&e)
				if err != nil {
					return fmt.Errorf("max31865: %w", err)
				}
				slog.Debug("publishing reading", "value", e.Temperature.Celsius(), "module", "max31865")
				c <- e.Temperature.Celsius()
			}
		}
	}
}

func TickerPrint(ctx context.Context, c <-chan physic.Env) func() error {
	ctx, cancelFunc := context.WithCancel(ctx)
	return func() error {
		defer cancelFunc()
		done := ctx.Done()
		for {
			select {
			case <-done:
				return nil
			case e := <-c:
				fmt.Printf("Temperature: %0.2f\n", e.Temperature.Celsius())
			}
		}
	}
}
