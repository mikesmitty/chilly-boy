package dutycycle

import (
	"log/slog"

	"github.com/mikesmitty/chilly-boy/pkg/cmhpid"
)

func NewDutyCycle(input <-chan cmhpid.ControllerState) func() error {
	return func() error {
		buf := make([]float64, 600)
		total := 0.0
		i := 0
		for v := range input {
			total := total - buf[i] + v.Signal
			slog.Info("duty cycle", "value", total/600)

			buf[i] = v.Signal
			i++
			if i == len(buf) {
				i = 0
			}
		}
		return nil
	}
}
