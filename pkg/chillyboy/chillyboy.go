package chillyboy

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os"
	"os/signal"
	"time"

	tsl2591 "github.com/JenswBE/golang-tsl2591"
	max "github.com/mikesmitty/chilly-boy/pkg/cmhmax31865"
	mqtt "github.com/mikesmitty/chilly-boy/pkg/cmhmqtt"
	"github.com/mikesmitty/chilly-boy/pkg/cmhpid"
	sht "github.com/mikesmitty/chilly-boy/pkg/cmhsht4x"
	tsl "github.com/mikesmitty/chilly-boy/pkg/cmhtsl2591"
	"github.com/mikesmitty/chilly-boy/pkg/dutycycle"
	"github.com/mikesmitty/chilly-boy/pkg/env"
	"github.com/mikesmitty/chilly-boy/pkg/hbridge"
	"github.com/mikesmitty/chilly-boy/pkg/router"
	"github.com/mikesmitty/chilly-boy/pkg/watchdog"
	"github.com/mikesmitty/max31865"
	"github.com/mikesmitty/sht4x"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"periph.io/x/conn/v3/i2c/i2creg"
	"periph.io/x/conn/v3/spi/spireg"
	"periph.io/x/host/v3"
)

func Root() func(cmd *cobra.Command, args []string) {
	return func(cmd *cobra.Command, args []string) {
		slogOpts := slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
		if viper.GetBool("debug") {
			slogOpts.Level = slog.LevelDebug
		}
		log := slog.New(slog.NewTextHandler(os.Stderr, &slogOpts))
		slog.SetDefault(log)

		spiBus := viper.GetString("spibus")
		i2cBus := viper.GetString("i2cbus")
		pidInterval := viper.GetDuration("pid-interval")

		_, err := host.Init()
		errChk(err)

		ctx, cancelFunc := context.WithCancel(context.Background())
		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(-1)

		// HBridge
		hb := hbridge.NewHBridge("GPIO26", "GPIO19", "GPIO20", "GPIO21")
		hb.Stop()

		// MAX31865
		sb, err := spireg.Open(spiBus)
		errChk(err)

		rtdDev, err := max31865.New(sb, nil)
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
		lightFan := router.NewFan[float64]("light", lightCh)
		g.Go(lightFan.Run)

		// SHT4x
		ib, err := i2creg.Open(i2cBus)
		errChk(err)
		defer ib.Close()

		refDev, err := sht4x.New(ib, nil)
		errChk(err)

		refCh, refFn := sht.TemperatureChannel(ctx, refDev, pidInterval)
		slog.Debug("Starting sht4x")
		g.Go(refFn)
		refFan := router.NewFan[env.Env]("ref", refCh)
		g.Go(refFan.Run)

		// PID
		var pidTune bool
		var kp, ki, kd, ff, awg, tuneAmp, tuneBase float64
		switch {
		case viper.GetFloat64("pid-tune-kp") != 0:
			pidTune = true
			kp = viper.GetFloat64("pid-tune-kp")
			tuneAmp = viper.GetFloat64("pid-tune-amp")
			tuneBase = viper.GetFloat64("pid-tune-base")
		default:
			ku := viper.GetFloat64("pid-ku")
			tu := viper.GetDuration("pid-tu").Seconds()
			algorithm := viper.GetString("pid-algo")
			// Traditional PID gains
			kp = viper.GetFloat64("pid-kp")
			ki = viper.GetFloat64("pid-ki")
			kd = viper.GetFloat64("pid-kd")
			// Feed Forward, Anti-Windup Gain
			ff = viper.GetFloat64("pid-ff")
			awg = viper.GetFloat64("pid-awg")
			kp, ki, kd, err = cmhpid.CalculatePID(ku, tu, kp, ki, kd, algorithm)
			errChk(err)
		}

		// Ensure pidMin is always negative
		pidMin := math.Min(viper.GetFloat64("pid-min-output"), -1*viper.GetFloat64("pid-min-output"))
		pidMax := viper.GetFloat64("pid-max-output")
		maxLight := viper.GetFloat64("max-light")
		pidLP := viper.GetDuration("pid-lp")
		pidCtrl := cmhpid.NewController(pidTune, tuneAmp, tuneBase, kp, ki, kd, ff, awg, pidMin, pidMax, maxLight, pidLP, pidInterval)
		pidCh, pidReset, controller := pidCtrl.GetController(lightFan.Subscribe("pid"), rtdFan.Subscribe("pid"))
		slog.Debug("Starting PID controller")
		g.Go(controller)
		pidFan := router.NewFan[cmhpid.ControllerState]("pid", pidCh)
		g.Go(pidFan.Run)

		slog.Debug("Starting HBridge control loop")
		go func() {
			// Find max light level
			hb.Enable()
			maxLight := findMaxLight(lightFan, func() { hb.Control(pidMin) }, func() { hb.Control(pidMax) })
			pidCtrl.MaxLight = maxLight
			slog.Info("max light level determined", "maxLight", fmt.Sprintf("%0.0f", maxLight))

			threshold := maxLight * viper.GetFloat64("startup-light-ratio") / 100.0

			light := lightFan.Subscribe("hbridge")
			hb.Control(pidMin)
			slog.Info("waiting for light to reach threshold", "threshold", fmt.Sprintf("%0.2f", threshold))
			for l := range light {
				if l < threshold {
					slog.Debug("reached light threshold, stopping hbridge")
					hb.Cool(0.0)
					pidReset()
					break
				}
			}

			// Unsubscribe so we don't hang the pid loop
			lightFan.Unsubscribe("hbridge")

			slog.Info("light threshold reached, starting PID control")
			for control := range pidFan.Subscribe("hbridge") {
				slog.Debug("hbridge received control signal", "controlSignal", fmt.Sprintf("%0.3f", control.ControlSignal))
				hb.Control(control.ControlSignal)
			}
		}()

		// Duty Cycle
		dutyCh, dutyCycle := dutycycle.NewDutyCycle(pidFan.Subscribe("dutycycle"))
		dutyFan := router.NewFan[float64]("dutycycle", dutyCh)
		g.Go(dutyCycle)
		g.Go(dutyFan.Run)

		// MQTT
		mqttUrl, err := url.Parse(viper.GetString("mqtt-broker"))
		errChk(err)
		mc := mqtt.NewClient(mqttUrl)
		g.Go(mc.GetPublisher(rtdFan.Subscribe("mqtt"), lightFan.Subscribe("mqtt"), dutyFan.Subscribe("mqtt"), pidFan.Subscribe("mqtt"), refFan.Subscribe("mqtt")))
		// Publish/handle the mirror-enable switch
		g.Go(mc.SwitchFn("mirror-enable", hb.Enable, hb.Disable, hb.GetEnable))

		// Watchdog
		watchdogTimeout := viper.GetDuration("watchdog-timeout")
		g.Go(watchdog.NewWatchdog(watchdogTimeout, hb.Stop, lightFan.Subscribe("watchdog")))

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

func findMaxLight(lightFan *router.Fan[float64], chiller func(), heater func()) float64 {
	lightCh := lightFan.Subscribe("lightmax")
	defer lightFan.Unsubscribe("lightmax")
	initialLight := <-lightCh

	// Cool for up to 90 seconds then heat for 60 seconds to find peak light level
	chiller()
	lightMaxCooldown(90*time.Second, lightCh, initialLight)
	heater()
	return lightMaxHeatup(60*time.Second, lightCh)
}

func lightMaxCooldown(timeout time.Duration, lightCh <-chan float64, initialLight float64) {
	timer := time.NewTimer(timeout)
	for {
		select {
		case <-timer.C:
			return
		case l := <-lightCh:
			if l < (initialLight * 0.99) {
				return
			}
		}
	}
}

func lightMaxHeatup(timeout time.Duration, lightCh <-chan float64) float64 {
	var max float64
	timer := time.NewTimer(timeout)
	for {
		select {
		case <-timer.C:
			return max
		case l := <-lightCh:
			if l > max {
				max = l
			}
		}
	}
}
