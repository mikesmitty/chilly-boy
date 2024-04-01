package dewpoint

import (
	"log/slog"

	"github.com/mikesmitty/chilly-boy/pkg/swma"
)

type Dewpoint struct {
	ready bool
}

func NewDewpoint(tempChan <-chan float64) (<-chan float64, func() error) {
	c := make(chan float64, 1)
	d := swma.NewSlidingWindow(600)
	first := true
	return c, func() error {
		for temp := range tempChan {
			slog.Debug("dewpoint", "temp", temp)
			if first {
				first = false
				for i := 0; i < 600; i++ {
					d.Add(temp)
				}
			}
			c <- d.Add(temp)
		}
		return nil
	}
}

func (d *Dewpoint) Add(temp float64) float64 {
	return temp
}

func (d *Dewpoint) Pause() {
	d.ready = false
}

func (d *Dewpoint) Resume() {
	d.ready = true
}
