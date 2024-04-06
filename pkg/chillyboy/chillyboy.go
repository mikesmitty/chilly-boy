package chillyboy

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	tsl2591 "github.com/JenswBE/golang-tsl2591"
	"github.com/mikesmitty/chilly-boy/pkg/cmhpid"
	"github.com/mikesmitty/chilly-boy/pkg/dewpoint"
	"github.com/mikesmitty/chilly-boy/pkg/dutycycle"
	"github.com/mikesmitty/chilly-boy/pkg/env"
	"github.com/mikesmitty/chilly-boy/pkg/hbridge"
	max "github.com/mikesmitty/chilly-boy/pkg/max31865"
	"github.com/mikesmitty/chilly-boy/pkg/mqtt"
	"github.com/mikesmitty/chilly-boy/pkg/router"
	sht "github.com/mikesmitty/chilly-boy/pkg/sht4x"
	tsl "github.com/mikesmitty/chilly-boy/pkg/tsl2591"
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

		hostState, err := host.Init()
		errChk(err)
		for i := range hostState.Loaded {
			slog.Debug("loaded", "module", hostState.Loaded[i])
		}
		for i := range hostState.Failed {
			slog.Error("failed", "module", hostState.Failed[i])
		}
		for i := range hostState.Skipped {
			slog.Debug("skipped", "module", hostState.Skipped[i])
		}

		ctx, cancelFunc := context.WithCancel(context.Background())
		g, ctx := errgroup.WithContext(ctx)
		g.SetLimit(-1)

		// HBridge
		hb, err := hbridge.NewHBridge(12, 13, 5, 6)
		errChk(err)

		// MAX31865
		sb, err := spireg.Open(spiBus)
		errChk(err)

		tempDev, err := max31865.New(sb, nil)
		errChk(err)

		tempCh, tempFn := max.TemperatureChannel(ctx, tempDev, pidInterval)
		slog.Debug("starting temp")
		g.Go(tempFn)
		tempFan := router.NewFan[float64]("temp", tempCh)
		g.Go(tempFan.Run)

		dewptCh, dewptFn := dewpoint.NewDewpoint(tempFan.Subscribe("dewpoint"))
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
			algorithm := viper.GetString("pid-algorithm")
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
		lightRatio := viper.GetFloat64("target-light-ratio") / 100
		maxLightRatio := viper.GetFloat64("target-max-light-ratio") / 100
		lowLight := maxLight * lightRatio
		highLight := maxLight * maxLightRatio

		initCool := viper.GetFloat64("initial-cool")
		initHeat := viper.GetFloat64("initial-heat")
		maxCool := viper.GetFloat64("max-cool")
		maxHeat := viper.GetFloat64("max-heat")
		if initCool == 0.0 {
			initCool = maxCool
		}
		if initHeat == 0.0 {
			initHeat = maxHeat
		}
		pidCtrl := cmhpid.NewController(
			pidTune,
			tuneAmp,
			tuneBase,
			kp,
			ki,
			kd,
			ff,
			awg,
			-1*maxCool,
			maxHeat,
			viper.GetFloat64("signal-exponent"),
			viper.GetFloat64("signal-cap"),
			viper.GetFloat64("pid-startup-integral"),
			maxLight,
			viper.GetDuration("pid-lp"),
			pidInterval,
			viper.GetInt("pid-input-average"),
			viper.GetInt("pid-output-average"),
		)
		pidCtrl.SetpointGain(
			viper.GetFloat64("setpoint-gain"),
			viper.GetFloat64("setpoint-floor"),
			viper.GetFloat64("linear-setpoint-gain"),
			viper.GetFloat64("linear-setpoint-deadband"),
			viper.GetFloat64("setpoint-step-limit"),
		)
		pidCh, pidReset, controller := pidCtrl.GetController(lightFan.Subscribe("pid"), tempFan.Subscribe("pid"), refFan.Subscribe("pid"))
		slog.Debug("starting pid controller")
		g.Go(controller)
		pidFan := router.NewFan[cmhpid.ControllerState]("pid", pidCh)
		g.Go(pidFan.Run)

		hb.Enable()
		slog.Debug("starting mirror control loop")
		go mirrorLoop(ctx, hb, initCool, initHeat, lowLight, highLight, lightFan, tempFan, refFan, pidFan, pidReset, pidCtrl)

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
		errChk(mc.Connect())
		g.Go(mc.GetPublisher(tempFan.Subscribe("mqtt"), dewptFan.Subscribe("mqtt"), lightFan.Subscribe("mqtt"), dutyFan.Subscribe("mqtt"), pidFan.Subscribe("mqtt"), refFan.Subscribe("mqtt")))
		errChk(mc.HomeAssistant())
		// Publish/handle the mirror-enable switch
		g.Go(mc.SwitchFn("mirror-enable", hb.Enable, hb.Disable, hb.GetEnable))

		// Watchdog
		watchdogTimeout := viper.GetDuration("watchdog-timeout")
		g.Go(watchdog.NewWatchdog(watchdogTimeout, hb.HardStop, lightFan.Subscribe("watchdog")))

		// Signal handling
		chanSignal := make(chan os.Signal, 1)
		signal.Notify(chanSignal, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)

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

func heatUp(ctx context.Context, heatGoal float64, tempFan *router.Fan[float64], stopFn, heatFn func() error) error {
	tempCh := tempFan.Subscribe("maxlight")
	defer tempFan.Unsubscribe("maxlight")

	temp := <-tempCh
	if temp > heatGoal {
		slog.Info("mirror above goal temperature, cooling back down", "temp", temp, "goal", heatGoal)
		return nil
	}

	slog.Info("mirror below goal temperature, heating up", "temp", temp, "goal", heatGoal)
	err := heatFn()
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case temp = <-tempCh:
			if temp > heatGoal {
				slog.Info("mirror reached goal temperature, cooling back down", "temp", temp, "goal", heatGoal)
				return stopFn()
			}
		}
	}
}

func cooldown(ctx context.Context, refFan *router.Fan[env.Env], lightGoal float64, lightFan, tempFan *router.Fan[float64], cool func()) {
	/*
		lightCh := lightFan.Subscribe("cooldown")
		defer lightFan.Unsubscribe("cooldown")
		l := <-lightCh
		if l > lightGoal {
			cool()
		}
	*/

	tempCh := tempFan.Subscribe("cooldown")
	defer tempFan.Unsubscribe("cooldown")
	refCh := refFan.Subscribe("cooldown")
	defer refFan.Unsubscribe("cooldown")
	ref := <-refCh
	temp := <-tempCh
	offset := 0.0
	if ref.Dewpoint < 3 && ref.Dewpoint > -3 {
		offset = 3.0
	}
	if temp > ref.Dewpoint {
		cool()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case temp = <-tempCh:
			goal := math.Min(ref.Dewpoint-offset, -offset)
			if temp < goal {
				slog.Info("temp below reference dewpoint, cooldown complete", "temp", temp, "ref", ref.Dewpoint)
				return
			}
		case ref = <-refCh:
		}
	}
}

func mirrorLoop(ctx context.Context, hb *hbridge.HBridge, initCool, initHeat, lowLight, highLight float64, lightFan, tempFan *router.Fan[float64], refFan *router.Fan[env.Env], pidFan *router.Fan[cmhpid.ControllerState], pidReset func(), pidCtrl *cmhpid.Controller) {
	for {
		// Make sure to clear frost before starting PID control
		heatUp(ctx, 20.0, tempFan,
			func() error { hb.Control(0); return nil },
			func() error { hb.Heat(initHeat); return nil },
		)
		time.Sleep(10 * time.Second)
		cooldown(ctx, refFan, lowLight, lightFan, tempFan, func() { hb.Cool(initCool) })
		hb.Control(0.0)
		time.Sleep(10 * time.Second)
		pidReset()
		slog.Info("desired light level reached, resuming pid control")
		runPID(ctx, hb, highLight, lightFan, tempFan, pidFan, pidCtrl)
	}
}

func runPID(ctx context.Context, hb *hbridge.HBridge, highLight float64, lightFan, tempFan *router.Fan[float64], pidFan *router.Fan[cmhpid.ControllerState], pidCtrl *cmhpid.Controller) {
	lightCh := lightFan.Subscribe("runpid")
	defer lightFan.Unsubscribe("runpid")
	tempCh := tempFan.Subscribe("runpid")
	defer tempFan.Unsubscribe("runpid")
	pidCh := pidFan.Subscribe("runpid")
	defer pidFan.Unsubscribe("runpid")
	thresh := 1000.0
	for {
		select {
		case <-ctx.Done():
			return
		case control := <-pidCh:
			switch {
			case thresh == 0:
			case control.ControlErrorIntegral > thresh:
				slog.Debug("integral error above threshold, capping", "threshold", thresh, "integral", control.ControlErrorIntegral)
				pidCtrl.SetIntegral(thresh)
			case control.ControlErrorIntegral < -thresh:
				slog.Debug("integral error below threshold, capping", "threshold", thresh, "integral", control.ControlErrorIntegral)
				pidCtrl.SetIntegral(-thresh)
			}
			slog.Debug("hbridge received control signal", "controlSignal", fmt.Sprintf("%0.3f", control.ControlSignal))
			hb.Control(control.ControlSignal)
		case l := <-lightCh:
			if l > highLight {
				slog.Info("light level above upper threshold, stopping pid control", "light", l, "goal", highLight)
				return
			}
		case t := <-tempCh:
			if t > 40.0 {
				hb.Control(0)
				slog.Info("mirror temperature above 40C, stopping pid control", "temp", t)
				return
			}
			if t < -20.0 {
				hb.Control(0)
				slog.Info("mirror temperature below -20C, stopping pid control", "temp", t)
				return
			}
		}
	}
}
