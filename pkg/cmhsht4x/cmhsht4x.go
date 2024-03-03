package cmhsht4x

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mikesmitty/chilly-boy/pkg/env"
	"github.com/mikesmitty/sht4x"
	"periph.io/x/conn/v3/physic"
)

func TemperatureChannel(ctx context.Context, dev *sht4x.Dev, interval time.Duration) (<-chan env.Env, func() error) {
	c := make(chan env.Env, 1)
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
					return fmt.Errorf("sht4x: %w", err)
				}
				slog.Debug("publishing reading", "temp", e.Temperature.Celsius(), "humidity", e.Humidity, "module", "sht4x")
				en := env.New(e.Temperature.Celsius(), float64(e.Humidity)/float64(physic.PercentRH))
				c <- en
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
