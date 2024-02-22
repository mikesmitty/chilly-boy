package cmhpid

import (
	"encoding/json"
	"log/slog"
	"math"
	"time"

	"github.com/VividCortex/ewma"
	"go.einride.tech/pid"
)

type Controller struct {
	Enabled bool
	c       pid.Controller
	peak    float64
}

type ControllerState struct {
	LightDiff float64
	TempDiff  float64

	ControlError           float64
	ControlErrorIntegral   float64
	ControlErrorDerivative float64
	ControlSignal          float64
	Signal                 float64
}

func NewController(kp, ki, kd, peak float64, interval time.Duration, historyOffset time.Duration) *Controller {
	return &Controller{
		Enabled: true,
		c: pid.Controller{
			Config: pid.ControllerConfig{
				ProportionalGain: kp,
				IntegralGain:     ki,
				DerivativeGain:   kd,
			},
		},
		peak: peak,
	}
}

func (c *Controller) GetController(lightChan <-chan uint64) (<-chan ControllerState, func(), func() error) {
	stateOutput := make(chan ControllerState, 1)

	return stateOutput, c.c.Reset, func() error {
		emaScaleFactor := 100.0
		lightScaleFactor := 10.0
		lightScale := 100.0 / c.peak
		lastReading := 0.0
		lastTime := time.Now()

		// Exponential moving average
		// alpha = 2/(N+1), 30 samples = 0.064516129
		ema := ewma.NewMovingAverage(30)

		slog.Debug("starting PID controller loop", "kp", c.c.Config.ProportionalGain, "ki", c.c.Config.IntegralGain, "kd", c.c.Config.DerivativeGain, "module", "cmhpid")
		for light := range lightChan {
			slog.Debug("pid received light reading", "light", light, "module", "cmhpid")

			now := time.Now()
			elapsed := now.Sub(lastTime)
			lastTime = now
			slog.Debug("elapsed time since last cycle", "elapsed", elapsed, "module", "cmhpid")

			reading := float64(light)
			diff := (reading - lastReading) * lightScale * lightScaleFactor
			slog.Debug("light differences", "current", reading, "last", lastReading, "module", "cmhpid")
			slog.Debug("total difference", "diff", diff, "module", "cmhpid")
			lastReading = reading
			ema.Add(diff)
			slog.Debug("exponential moving average", "ema", ema.Value(), "scaleFactor", emaScaleFactor, "module", "cmhpid")

			signalInput := (ema.Value() * emaScaleFactor)

			c.c.Update(pid.ControllerInput{
				// Target value
				ReferenceSignal:  0.0,
				ActualSignal:     signalInput,
				SamplingInterval: elapsed,
			})

			// Limit output to -100% and +3% to avoid overheating
			signal := math.Max(-100.0, math.Min(3.0, c.c.State.ControlSignal))
			slog.Debug("pid control signal", "control", c.c.State.ControlSignal, "signal", signal, "module", "cmhpid")
			stateOutput <- ControllerState{
				LightDiff:              diff,
				ControlError:           c.c.State.ControlError,
				ControlErrorIntegral:   c.c.State.ControlErrorIntegral,
				ControlErrorDerivative: c.c.State.ControlErrorDerivative,
				ControlSignal:          c.c.State.ControlSignal,
				Signal:                 signal,
			}
			slog.Debug("pid control signal published", "module", "cmhpid")
		}
		return nil
	}
}

func (p *ControllerState) String() string {
	out, err := json.Marshal(p)
	if err != nil {
		slog.Error("json marshal error", "error", err, "module", "cmhpid", "state", p)
	}
	return string(out)
}
