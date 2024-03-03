package router

import (
	"log/slog"
	"sync"
)

type Fan[T any] struct {
	debug   bool
	name    string
	mu      sync.Mutex
	input   <-chan T
	outputs map[string]chan<- T
}

func NewFan[T any](name string, input <-chan T) *Fan[T] {
	return &Fan[T]{
		name:    name,
		input:   input,
		outputs: make(map[string](chan<- T)),
	}
}

func (f *Fan[T]) SetDebug(debug bool) {
	f.debug = debug
}

func (f *Fan[T]) Subscribe(client string) <-chan T {
	if f.debug {
		slog.Debug("subscribing to fan", "fan", f.name, "client", client)
	}
	f.mu.Lock()
	if _, ok := f.outputs[client]; ok {
		f.mu.Unlock()
		panic("client already subscribed")
	}
	defer f.mu.Unlock()
	c := make(chan T, 1)
	f.outputs[client] = c
	return c
}

func (f *Fan[T]) Unsubscribe(client string) {
	if f.debug {
		slog.Debug("unsubscribing from fan", "fan", f.name, "client", client)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.outputs[client]; !ok {
		panic("client not subscribed")
	}
	close(f.outputs[client])
	delete(f.outputs, client)
}

func (f *Fan[T]) Run() error {
	for v := range f.input {
		if f.debug {
			slog.Debug("fan received value", "fan", f.name, "value", v)
		}
		f.mu.Lock()
		for k, ch := range f.outputs {
			if f.debug {
				slog.Debug("fan sending value", "subscriber", k, "fan", f.name, "value", v)
			}
			ch <- v
			if f.debug {
				slog.Debug("fan sent value", "subscriber", k, "fan", f.name, "value", v)
			}
		}
		f.mu.Unlock()
	}
	return nil
}
