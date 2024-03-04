package dutycycle

import (
	"log/slog"
	"math"
	"strconv"

	"github.com/mikesmitty/chilly-boy/pkg/cmhpid"
	"github.com/mikesmitty/chilly-boy/pkg/swma"
)

func NewDutyCycle(input <-chan cmhpid.ControllerState) (<-chan float64, func() error) {
	output := make(chan float64)
	return output, func() error {
		freq := 1
		size := 600
		swma := swma.NewSlidingWindow(size)

		avg := 0.0
		i := 0
		for v := range input {
			signal := math.Max(-math.MaxFloat64, math.Min(math.MaxFloat64, v.ControlSignal))
			if !math.IsInf(signal, 0) && !math.IsNaN(signal) {
				avg = swma.Add(signal)
			}
			if i%freq == 0 {
				slog.Debug("duty cycle", "average", strconv.FormatFloat(avg, 'f', 2, 64))
				output <- avg
			}
			i++
			i = i % size
		}
		return nil
	}
}
