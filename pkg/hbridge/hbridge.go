package hbridge

import (
	"log"
	"sync"

	"periph.io/x/conn/v3/gpio"
	"periph.io/x/conn/v3/gpio/gpioreg"
	"periph.io/x/conn/v3/physic"
)

type HBridge struct {
	freq    physic.Frequency
	pinA    gpio.PinOut
	pinB    gpio.PinOut
	enPinA  gpio.PinOut
	enPinB  gpio.PinOut
	enabled bool
	mu      sync.Mutex
}

func NewHBridge(pinA, pinB, enablePinA, enablePinB string) *HBridge {
	pA := gpioreg.ByName(pinA)
	if pA == nil {
		log.Fatal("Failed to find pinA")
	}

	pB := gpioreg.ByName(pinB)
	if pB == nil {
		log.Fatal("Failed to find pinB")
	}

	eA := gpioreg.ByName(enablePinA)
	if eA == nil {
		log.Fatal("Failed to find enablePinA")
	}

	eB := gpioreg.ByName(enablePinB)
	if eB == nil {
		log.Fatal("Failed to find enablePinB")
	}

	return &HBridge{
		freq:   1 * physic.KiloHertz,
		pinA:   pA,
		pinB:   pB,
		enPinA: eA,
		enPinB: eB,
	}
}

func (h *HBridge) Control(percent float64) error {
	if percent > 0 {
		return h.Cool(percent)
	} else if percent < 0 {
		return h.Heat(-percent)
	} else {
		return h.Stop()
	}
}

func (h *HBridge) Cool(percent float64) error {
	h.set(h.pinB, 0.0)
	return h.set(h.pinA, percent)
}

func (h *HBridge) Heat(percent float64) error {
	h.set(h.pinA, 0.0)
	return h.set(h.pinB, percent)
}

func (h *HBridge) GetEnable() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.enabled
}

func (h *HBridge) Enable() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.enabled = true
	h.enPinA.Out(gpio.High)
	h.enPinB.Out(gpio.High)
}

func (h *HBridge) Disable() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.enabled = false
	h.enPinA.Out(gpio.Low)
	h.enPinB.Out(gpio.Low)
}

func (h *HBridge) Stop() error {
	h.enPinA.Out(gpio.Low)
	h.enPinB.Out(gpio.Low)
	h.set(h.pinA, 0.0)
	return h.set(h.pinB, 0.0)
}

func (h *HBridge) set(pin gpio.PinOut, percent float64) error {
	dutyCycle := gpio.DutyMax * gpio.Duty(percent/100.0)
	if err := pin.PWM(dutyCycle, h.freq); err != nil {
		return err
	}
	return nil
}
