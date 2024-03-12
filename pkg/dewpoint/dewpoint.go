package dewpoint

import (
	"log/slog"

	"github.com/mikesmitty/chilly-boy/pkg/swma"
)

func NewDewpoint(tempChan <-chan float64) (<-chan float64, func() error) {
	c := make(chan float64, 1)
	d := swma.NewSlidingWindow(600)
	return c, func() error {
		for temp := range tempChan {
			slog.Debug("dewpoint", "temp", temp)
			c <- d.Add(temp)
		}
		return nil
	}
}
