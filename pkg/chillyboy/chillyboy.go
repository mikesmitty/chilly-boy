package chillyboy

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os"
	"os/signal"

	tsl2591 "github.com/JenswBE/golang-tsl2591"
	max "github.com/mikesmitty/chilly-boy/pkg/cmhmax31865"
	mqtt "github.com/mikesmitty/chilly-boy/pkg/cmhmqtt"
	"github.com/mikesmitty/chilly-boy/pkg/cmhpid"
	tsl "github.com/mikesmitty/chilly-boy/pkg/cmhtsl2591"
	"github.com/mikesmitty/chilly-boy/pkg/hbridge"
	"github.com/mikesmitty/chilly-boy/pkg/router"
	"github.com/mikesmitty/chilly-boy/pkg/watchdog"
	"github.com/mikesmitty/max31865"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

func Root() func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			//Level: slog.LevelDebug,
			Level: slog.LevelInfo,
		}))
		slog.SetDefault(log)

		spiBus := viper.GetString("spibus")
		i2cBus := viper.GetString("i2cbus")
		pidInterval := viper.GetDuration("pid-interval")
		pidHistoryOffset := viper.GetDuration("pid-history-offset")

		_, err := host.Init()
		errChk(err)

		ctx, cancelFunc := context.WithCancel(context.Background())
		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(-1)

		// MAX31865
		sb, err := spireg.Open(spiBus)
		errChk(err)

		rtdDev, err := max31865.New(sb, max31865.AdafruitPT1000())
		errChk(err)

		rtdCh, rtdFn := max.TemperatureChannel(ctx, rtdDev, pidInterval)
		slog.Debug("Starting RTD")
		g.Go(rtdFn)
		rtdFan := router.NewFan[float64]("rtd", rtdCh)
		g.Go(rtdFan.Run)

		// TSL2591
		opts := &tsl2591.Opts{
			Bus:    i2cBus,
			Gain:   tsl2591.GainLow,
			Timing: tsl2591.IntegrationTime100MS,
		}
		tslDev, err := tsl2591.NewTSL2591(opts)
		errChk(err)
		defer func() {
			if disableErr := tslDev.Disable(); disableErr != nil {
				errChk(disableErr)
			}
		}()

		lightCh, lightFn := tsl.LightChannel(ctx, tslDev, pidInterval)
		slog.Debug("Starting light sensor")
		g.Go(lightFn)
		lightFan := router.NewFan[uint64]("light", lightCh)
		g.Go(lightFan.Run)

		// PID
		kp := viper.GetFloat64("pid-kp")
		ki := viper.GetFloat64("pid-ki")
		kd := viper.GetFloat64("pid-kd")
		peak := viper.GetFloat64("peak-light")
		pidCtrl := cmhpid.NewController(kp, ki, kd, peak, pidInterval, pidHistoryOffset)
		//pidCh, pidReset, controller := pidCtrl.GetController(lightFan.Subscribe("pid"), rtdFan.Subscribe("pid"))
		pidCh, pidReset, controller := pidCtrl.GetController(lightFan.Subscribe("pid"))
		slog.Debug("Starting PID controller")
		g.Go(controller)
		pidFan := router.NewFan[cmhpid.ControllerState]("pid", pidCh)
		g.Go(pidFan.Run)

		// HBridge
		hb := hbridge.NewHBridge("GPIO26", "GPIO19", "GPIO20", "GPIO21")
		slog.Debug("Starting HBridge control loop")
		go func() {
			light := lightFan.Subscribe("hbridge")

			peak := viper.GetFloat64("peak-light")
			threshold := uint64(math.Round(viper.GetFloat64("startup-light-ratio") * peak / 100.0))
			slog.Info("waiting for light to reach threshold", "threshold", fmt.Sprintf("%d", threshold))

			hb.Enable()
			hb.Control(-100.0)
			for l := range light {
				if l < threshold {
					slog.Debug("reached light threshold, stopping hbridge")
					hb.Cool(0.0)
					pidReset()
					break
				}
				slog.Debug("light is above threshold", "light", fmt.Sprintf("%d", l))
			}
			// Unsubscribe so we don't hang the pid loop
			lightFan.Unsubscribe("hbridge")
			for control := range pidFan.Subscribe("hbridge") {
				slog.Debug("hbridge received control signal", "control", fmt.Sprintf("%0.3f", control.ControlSignal), "signal", fmt.Sprintf("%0.3f", control.Signal))
				hb.Control(control.Signal)
			}
		}()

		// MQTT
		mqttUrl, err := url.Parse(viper.GetString("mqtt-broker"))
		errChk(err)
		mc := mqtt.NewClient(mqttUrl)
		g.Go(mc.GetPublisher(rtdFan.Subscribe("mqtt"), lightFan.Subscribe("mqtt"), pidFan.Subscribe("mqtt")))
		// Publish/handle the mirror-enable switch
		g.Go(mc.SwitchFn("mirror-enable", hb.Enable, hb.Disable, hb.GetEnable))

		// Watchdog
		watchdogTimeout := viper.GetDuration("watchdog-timeout")
		g.Go(watchdog.NewWatchdog(watchdogTimeout, hb.Stop, lightFan.Subscribe("watchdog")))

		// Duty Cycle
		//g.Go(dutycycle.NewDutyCycle(pidFan.Subscribe("dutycycle")))

		// Signal handling
		chanSignal := make(chan os.Signal, 1)
		signal.Notify(chanSignal, os.Interrupt)

		g.Go(func() error {
			defer cancelFunc()
			select {
			case <-ctx.Done():
			case <-chanSignal:
			}
			slog.Info("shutting down...")
			slog.Info("stopping hbridge...")
			hb.Stop()
			os.Exit(0)
			return nil
		})

		slog.Debug("waiting for goroutines to finish")
		err = g.Wait()
		errChk(err)
	}
}

func errChk(err error) {
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
