package cmhpid

import (
	"encoding/json"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/mikesmitty/chilly-boy/pkg/swma"
	"go.einride.tech/pid"
)

type Controller struct {
	Enabled         bool
	MaxLight        float64
	c               pid.AntiWindupController
	feedForwardGain float64
	interval        time.Duration
	tuning          bool
	tuningAmp       float64
	tuningBase      float64
}

type ControllerState struct {
	FeedForward float64
	LightDiff   float64
	TempDiff    float64

	ControlError           float64
	ControlErrorIntegral   float64
	ControlErrorDerivative float64
	ControlSignal          float64
	SignalInput            float64
}

func NewController(tuning bool, tuneAmp, tuneBase, kp, ki, kd, ff, awg, min, max, maxLight float64, lp, interval time.Duration) *Controller {
	if tuning {
		ki, kd = 0, 0
	}

	return &Controller{
		Enabled:  true,
		MaxLight: maxLight,
		c: pid.AntiWindupController{
			Config: pid.AntiWindupControllerConfig{
				ProportionalGain:    kp,
				IntegralGain:        ki,
				DerivativeGain:      kd,
				AntiWindUpGain:      awg,
				LowPassTimeConstant: lp,
				MaxOutput:           max,
				MinOutput:           min,
			},
		},
		feedForwardGain: ff,
		interval:        interval,
		tuning:          tuning,
		tuningAmp:       tuneAmp,
		tuningBase:      tuneBase,
	}
}

func (c *Controller) GetController(lightChan <-chan float64, tempChan <-chan float64) (<-chan ControllerState, func(), func() error) {
	stateOutput := make(chan ControllerState, 1)

	return stateOutput, c.c.Reset, func() error {
		lastReading := 0.0
		lastTemp := 0.0
		lastTime := time.Now()

		// Average light
		l := swma.NewSlidingWindow(3)

		// Average temperature
		t := swma.NewSlidingWindow(3)

		// Tuning
		periods := make([]float64, 0, 10)
		tuningPeak := 0.0
		peakTime := time.Now()
		lastPeakTime := time.Now()

		firstLoop := true

		slog.Info("starting PID controller loop", "kp", c.c.Config.ProportionalGain, "ki", c.c.Config.IntegralGain, "kd", c.c.Config.DerivativeGain, "module", "cmhpid")
		for light := range lightChan {
			slog.Debug("pid received light reading", "light", light, "module", "cmhpid")
			temp := <-tempChan
			slog.Debug("pid received temperature reading", "temp", temp, "module", "cmhpid")

			if firstLoop {
				lastReading = float64(light)
				lastTemp = temp
				tuningPeak = temp
			}

			now := time.Now()
			elapsed := now.Sub(lastTime)
			lastTime = now
			slog.Debug("elapsed time since last cycle", "elapsed", elapsed, "module", "cmhpid")

			// Light sliding window moving average
			l.Add(light)
			reading := l.Average()

			diff := (reading - lastReading) / (c.MaxLight / 100.0)
			slog.Debug("light difference", "current", reading, "last", lastReading, "scale", (c.MaxLight / 100.0), "diff", diff, "module", "cmhpid")
			lastReading = reading

			// Temperature sliding window moving average
			t.Add(temp)
			temp = t.Average()

			tempDiff := (temp - lastTemp)
			slog.Debug("temp difference", "tempDiff", tempDiff, "module", "cmhpid")
			lastTemp = temp
			feedForward := c.feedForwardGain * tempDiff / float64(elapsed.Seconds())
			slog.Debug("feed forward", "feedForward", feedForward, "feedForwardGain", c.feedForwardGain, "elapsed", elapsed.Seconds(), "module", "cmhpid")

			if c.tuning && c.tuningAmp > 0 {
				if temp < tuningPeak {
					tuningPeak = temp
					peakTime = now
				}
				if temp-tuningPeak > c.tuningAmp && !firstLoop {
					tuningPeriod := peakTime.Sub(lastPeakTime)
					periods = append(periods, float64(tuningPeriod.Seconds()))
					lastPeakTime = peakTime
					slog.Info("found tuning peak", "median", median(periods), "Tu", tuningPeriod.Seconds(), "Ku", c.c.Config.ProportionalGain, "peak", tuningPeak, "temp", temp, "module", "cmhpid")
					tuningPeak = 50.0
				}
			}

			sign := 1.0
			if diff < 0 {
				sign = -1.0
			}
			diff = diff * 10

			signalInput := math.Pow(1+math.Abs(diff), 2) * sign
			slog.Debug("pid signal input", "diff", diff, "signalInput", signalInput, "module", "cmhpid")

			c.c.Update(pid.AntiWindupControllerInput{
				// Reference (target) is 0 change in light output
				ReferenceSignal: 0.0,
				ActualSignal:    signalInput,
				// Feed Forward is intended to compensate for the delay in response by predicting future signal behavior
				// Mirror temperature leads light output by a few seconds so using the temperature derivative here
				// helps compensate, allowing the PID to react to the change in light output before it happens
				FeedForwardSignal: feedForward,
				SamplingInterval:  elapsed,
			})

			// Add base-level cooling required to keep a stable temperature during tuning
			controlSignal := c.c.State.ControlSignal
			if c.tuning {
				controlSignal += c.tuningBase
			}

			slog.Debug("pid control signal", "controlSignal", c.c.State.ControlSignal, "tuningBase", c.tuningBase, "module", "cmhpid")
			stateOutput <- ControllerState{
				LightDiff:              diff,
				TempDiff:               tempDiff,
				ControlError:           c.c.State.ControlError,
				ControlErrorIntegral:   c.c.State.ControlErrorIntegral,
				ControlErrorDerivative: c.c.State.ControlErrorDerivative,
				ControlSignal:          controlSignal,
				FeedForward:            feedForward,
				SignalInput:            signalInput,
			}
			slog.Debug("pid control signal published", "module", "cmhpid")

			firstLoop = false
		}
		return nil
	}
}

func (p *Controller) SetIntegral(integral float64) {
	p.c.State.ControlErrorIntegral = integral
}

func (p *ControllerState) String() string {
	out, err := json.Marshal(p)
	if err != nil {
		slog.Error("json marshal error", "error", err, "module", "cmhpid", "state", p)
	}
	return string(out)
}

func median(data []float64) float64 {
	dataCopy := make([]float64, len(data))
	copy(dataCopy, data)

	sort.Float64s(dataCopy)

	var median float64
	l := len(dataCopy)
	if l == 0 {
		return 0
	} else if l%2 == 0 {
		median = (dataCopy[l/2-1] + dataCopy[l/2]) / 2
	} else {
		median = dataCopy[l/2]
	}

	return median
}
