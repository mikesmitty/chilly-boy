package cmhpid

import (
	"encoding/json"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/mikesmitty/chilly-boy/pkg/env"
	"github.com/mikesmitty/chilly-boy/pkg/stats"
	"github.com/mikesmitty/chilly-boy/pkg/swma"
	"go.einride.tech/pid"
)

type Controller struct {
	Enabled                bool
	c                      pid.AntiWindupController
	feedForwardGain        float64
	inputAverage           int
	interval               time.Duration
	lightScale             float64
	linearSetpointDeadband float64
	linearSetpointGain     float64
	maxLight               float64
	outputAverage          int
	signalExponent         float64
	signalCap              float64
	setpointFloor          float64
	setpointGain           float64
	setpointStepLimit      float64
	startupIntegral        float64
	tuning                 bool
	tuningAmp              float64
	tuningBase             float64
}

type ControllerState struct {
	FeedForward float64
	LightDiff   float64
	TempDiff    float64

	ControlError           float64
	ControlErrorIntegral   float64
	ControlErrorDerivative float64
	ControlSignal          float64
	Linear                 float64
	SetPoint               float64
	SignalInput            float64
	Volatility             float64
}

func NewController(
	tuning bool,
	tuneAmp,
	tuneBase,
	kp, ki, kd,
	ff,
	awg,
	min,
	max,
	sigExp,
	sigCap,
	startupIntegral,
	maxLight float64,
	lp,
	interval time.Duration,
	inputAverage,
	outputAverage int,
) *Controller {
	if tuning {
		ki, kd = 0, 0
	}

	return &Controller{
		Enabled: true,
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
		inputAverage:    inputAverage,
		interval:        interval,
		maxLight:        maxLight,
		outputAverage:   outputAverage,
		signalCap:       sigCap,
		signalExponent:  sigExp,
		startupIntegral: startupIntegral,
		tuning:          tuning,
		tuningAmp:       tuneAmp,
		tuningBase:      tuneBase,
	}
}

func (c *Controller) GetController(lightChan, tempChan <-chan float64, refChan <-chan env.Env) (<-chan ControllerState, func(), func() error) {
	stateOutput := make(chan ControllerState, 1)

	reset := func() {
		c.c.Reset()
		c.c.State.ControlErrorIntegral = c.startupIntegral
	}

	return stateOutput, reset, func() error {
		lastLight := 0.0
		lastTemp := 0.0
		lastTime := time.Now()

		lightStats := stats.NewStats(c.period(90*time.Second), 0)

		inputAvg := swma.NewSlidingWindow(c.inputAverage)
		outputAvg := swma.NewSlidingWindow(c.outputAverage)

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
			ref := <-refChan
			slog.Debug("pid received reference reading", "refTemp", ref.Temperature, "module", "cmhpid")

			if firstLoop {
				lastLight = float64(light)
				lastTemp = temp
				tuningPeak = temp
			}

			now := time.Now()
			elapsed := now.Sub(lastTime)
			lastTime = now
			slog.Debug("elapsed time since last cycle", "elapsed", elapsed, "module", "cmhpid")

			lightDiff := (light - lastLight) / c.lightScale
			lastLight = light
			slog.Debug("light difference", "current", light, "last", lastLight, "scale", c.lightScale, "diff", lightDiff)

			tempDiff := (temp - lastTemp)
			lastTemp = temp
			slog.Debug("temp difference", "tempDiff", tempDiff)

			setPoint, vol, lin := c.getSetpoint(light, lightStats)
			signalInput := c.getSignal(tempDiff, inputAvg)
			feedForward := c.getFeedForward(tempDiff, elapsed)

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

			c.c.Update(pid.AntiWindupControllerInput{
				// Reference (target) is 0 change in light output
				ReferenceSignal: setPoint,
				ActualSignal:    signalInput,
				// Feed Forward is intended to compensate for the delay in response by predicting future signal behavior
				// Mirror temperature leads light output by a few seconds so using the temperature derivative here
				// helps compensate, allowing the PID to react to the change in light output before it happens
				FeedForwardSignal: c.getFeedForward(tempDiff, elapsed),
				SamplingInterval:  elapsed,
			})

			// Add base-level cooling required to keep a stable temperature during tuning
			controlSignal := c.c.State.ControlSignal
			if c.tuning {
				controlSignal += c.tuningBase
			}

			controlSignal = outputAvg.Add(controlSignal)

			slog.Debug("pid control signal", "stateControlSignal", c.c.State.ControlSignal, "controlSignal", controlSignal, "tuningBase", c.tuningBase, "module", "cmhpid")
			stateOutput <- ControllerState{
				LightDiff:              lightDiff,
				TempDiff:               tempDiff,
				ControlError:           c.c.State.ControlError,
				ControlErrorIntegral:   c.c.State.ControlErrorIntegral,
				ControlErrorDerivative: c.c.State.ControlErrorDerivative,
				ControlSignal:          controlSignal,
				FeedForward:            feedForward,
				SetPoint:               setPoint,
				SignalInput:            signalInput,
				Linear:                 lin,
				Volatility:             vol,
			}
			slog.Debug("pid control signal published", "module", "cmhpid")

			firstLoop = false
		}
		return nil
	}
}

func (p *Controller) SetpointGain(setpointGain, setpointFloor, linearSetpointGain, linearSetpointDeadband, setpointStepLimit float64) {
	p.setpointGain = setpointGain
	p.setpointFloor = setpointFloor
	p.linearSetpointGain = linearSetpointGain
	p.linearSetpointDeadband = linearSetpointDeadband
	p.setpointStepLimit = setpointStepLimit
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

func (c *Controller) period(d time.Duration) int {
	return int(d / c.interval)
}

func (c *Controller) getSetpoint(light float64, lightStats *stats.Stats) (float64, float64, float64) {
	// Calculate the linear regression (algebraic slope) and residual standard deviation of the light output
	lightStats.Add(light / (c.maxLight / 100.0))
	_, m := lightStats.LinearRegression()
	lightRsd := lightStats.ResidualStandardDeviation(m)

	// Volatility gain pushes the setpoint up to thin out the dew layer. Thicker dew layers are less stable,
	// tending to have large oscillating swings in temperature, and thus less accurate, so we try to minimize
	// that volatility by thinning out the dew layer.
	vol := c.setpointGain * lightRsd / 1e3
	setPoint := math.Max(0, vol-c.setpointFloor)

	// Linear gain counteracts the tendency for the light output to gradually drift either up or down over time.
	// It can be tuned out in one environment, but can return when temps and dewpoints change significantly.
	lin := c.linearSetpointGain * -m
	if lin < 0 {
		setPoint += math.Min(0, lin+c.linearSetpointDeadband)
	} else if lin > 0 {
		setPoint += math.Max(0, lin-c.linearSetpointDeadband)
	}
	setPoint = math.Min(setPoint, c.setpointStepLimit)
	setPoint = math.Max(setPoint, -c.setpointStepLimit)
	slog.Debug("setpoint adjustments", "volatility", vol, "linear", lin, "setPoint", setPoint, "setpointGain", c.setpointGain)
	return setPoint, vol, lin
}

func (c *Controller) getSignal(diff float64, inputAvg *swma.SlidingWindow) float64 {
	// Exponentially amplify the signal input to the PID controller in order to track the true dewpoint
	sign := 1.0
	if diff < 0 {
		sign = -1.0
	}
	signalInput := math.Pow(1+math.Abs(diff), c.signalExponent) - 1
	if c.signalCap > 0 {
		signalInput = math.Min(c.signalCap, signalInput)
	}
	signalInput *= sign
	signalInput = inputAvg.Add(signalInput)
	slog.Debug("pid signal input", "diff", diff, "signalInput", signalInput, "module", "cmhpid")
	return signalInput
}

func (c *Controller) getFeedForward(tempDiff float64, elapsed time.Duration) float64 {
	feedForward := c.feedForwardGain * tempDiff / float64(elapsed.Seconds())
	slog.Debug("feed forward", "feedForward", feedForward, "feedForwardGain", c.feedForwardGain, "elapsed", elapsed.Seconds(), "module", "cmhpid")
	return 0
}

func (c *Controller) SetMaxLight(maxLight float64) {
	c.maxLight = maxLight
	c.lightScale = maxLight / 100.0
}
