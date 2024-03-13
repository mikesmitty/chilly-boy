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
	sht "github.com/mikesmitty/chilly-boy/pkg/cmhsht4x"
	tsl "github.com/mikesmitty/chilly-boy/pkg/cmhtsl2591"
	"github.com/mikesmitty/chilly-boy/pkg/dewpoint"
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
		slog.Debug("starting rtd")
		g.Go(rtdFn)
		rtdFan := router.NewFan[float64]("rtd", rtdCh)
		g.Go(rtdFan.Run)

		dewptCh, dewptFn := dewpoint.NewDewpoint(rtdFan.Subscribe("dewpoint"))
		slog.Debug("starting dewpoint")
		g.Go(dewptFn)
		dewptFan := router.NewFan[float64]("dewpoint", dewptCh)
		g.Go(dewptFan.Run)

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
			tuneBase = math.Min(viper.GetFloat64("pid-tune-base"), -1*viper.GetFloat64("pid-tune-base"))
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

		maxLight := viper.GetFloat64("max-light")
		lowLight := maxLight * 0.4
		highLight := maxLight * 0.8

		maxCool := viper.GetFloat64("max-cool")
		maxHeat := viper.GetFloat64("max-heat")
		sigExp := viper.GetFloat64("signal-exponent")
		pidLP := viper.GetDuration("pid-lp")
		pidCtrl := cmhpid.NewController(pidTune, tuneAmp, tuneBase, kp, ki, kd, ff, awg, -1*maxCool, maxHeat, sigExp, maxLight, pidLP, pidInterval)
		pidCh, pidReset, controller := pidCtrl.GetController(lightFan.Subscribe("pid"), rtdFan.Subscribe("pid"))
		slog.Debug("starting pid controller")
		g.Go(controller)
		pidFan := router.NewFan[cmhpid.ControllerState]("pid", pidCh)
		g.Go(pidFan.Run)

		hb.Enable()
		slog.Debug("starting mirror control loop")
		go mirrorLoop(ctx, hb, maxCool, lowLight, highLight, lightFan, pidFan, pidReset)

		// Duty Cycle
		dutyCh, dutyCycle := dutycycle.NewDutyCycle(pidFan.Subscribe("dutycycle"))
		dutyFan := router.NewFan[float64]("dutycycle", dutyCh)
		g.Go(dutyCycle)
		g.Go(dutyFan.Run)

		// MQTT
		mqttUrl, err := url.Parse(viper.GetString("mqtt-broker"))
		mqttSampleInterval := viper.GetInt("mqtt-sample-interval")
		errChk(err)
		mc := mqtt.NewClient(mqttUrl, mqttSampleInterval, pidInterval)
		g.Go(mc.GetPublisher(rtdFan.Subscribe("mqtt"), dewptFan.Subscribe("mqtt"), lightFan.Subscribe("mqtt"), dutyFan.Subscribe("mqtt"), pidFan.Subscribe("mqtt"), refFan.Subscribe("mqtt")))
		// Publish/handle the mirror-enable switch
		g.Go(mc.SwitchFn("mirror-enable", hb.Enable, hb.Disable, hb.GetEnable))

		// Watchdog
		watchdogTimeout := viper.GetDuration("watchdog-timeout")
		g.Go(watchdog.NewWatchdog(watchdogTimeout, hb.HardStop, lightFan.Subscribe("watchdog")))

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
			hb.HardStop()
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

func heatCycle(ctx context.Context, heatGoal, lightGoal float64, lightFan, tempFan *router.Fan[float64], refFan *router.Fan[env.Env], coolFn, stopFn, heatFn func() error) error {
	tempCh := tempFan.Subscribe("maxlight")
	defer tempFan.Unsubscribe("maxlight")
	lightCh := lightFan.Subscribe("maxlight")
	defer lightFan.Unsubscribe("maxlight")
	refCh := refFan.Subscribe("maxlight")
	defer refFan.Unsubscribe("maxlight")

	r := <-refCh
	dewpoint := r.Dewpoint
	t := <-tempCh
	if t > dewpoint {
		slog.Info("begining heat cycle, cooling to dewpoint", "mirror-temp", t, "dewpoint", dewpoint)
		if err := coolFn(); err != nil {
			return err
		}
	}

	var cooling bool
	var heating bool
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case r := <-refCh:
			dewpoint = r.Dewpoint
		case t := <-tempCh:
			if !cooling && !heating && t < dewpoint {
				slog.Info("mirror temperature is below dewpoint, beginning heating", "temp", t, "dewpoint", dewpoint)
				heating = true
				if err := heatFn(); err != nil {
					stopFn()
					return err
				}
				return nil
			} else if heating && t > heatGoal {
				slog.Info("mirror reached goal temperature, cooling back down", "temp", t, "goal", heatGoal)
				cooling = true
				heating = false
				if err := coolFn(); err != nil {
					return err
				}
			}
		case l := <-lightCh:
			if cooling && l < lightGoal {
				slog.Info("light level below goal, heatcycle complete", "light", l, "goal", lightGoal)
				stopFn()
				return nil
			}
		}
	}
}

func cooldown(ctx context.Context, lightGoal float64, lightFan *router.Fan[float64]) {
	lightCh := lightFan.Subscribe("cooldown")
	defer lightFan.Unsubscribe("cooldown")
	for {
		select {
		case <-ctx.Done():
			return
		case l := <-lightCh:
			if l < lightGoal {
				slog.Info("light level below goal, cooldown complete", "light", l, "goal", lightGoal)
				return
			}
		}
	}
}

func mirrorLoop(ctx context.Context, hb *hbridge.HBridge, maxCool, lowLight, highLight float64, lightFan *router.Fan[float64], pidFan *router.Fan[cmhpid.ControllerState], pidReset func()) {
	for {
		slog.Info("cooling mirror down", "goal", lowLight)
		hb.Cool(maxCool)
		cooldown(ctx, lowLight, lightFan)
		hb.Cool(0.0)
		pidReset()
		slog.Info("desired light level reached, resuming pid control")
		runPID(ctx, hb, highLight, lightFan, pidFan)
	}
}

func runPID(ctx context.Context, hb *hbridge.HBridge, highLight float64, lightFan *router.Fan[float64], pidFan *router.Fan[cmhpid.ControllerState]) {
	lightCh := lightFan.Subscribe("runpid")
	defer lightFan.Unsubscribe("runpid")
	pidCh := pidFan.Subscribe("runpid")
	defer pidFan.Unsubscribe("runpid")
	for {
		select {
		case <-ctx.Done():
			return
		case control := <-pidCh:
			slog.Debug("hbridge received control signal", "controlSignal", fmt.Sprintf("%0.3f", control.ControlSignal))
			hb.Control(control.ControlSignal)
		case l := <-lightCh:
			if l > highLight {
				slog.Info("light level above upper threshold, stopping pid control", "light", l, "goal", highLight)
				return
			}
		}
	}
}
