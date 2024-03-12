package hbridge

import (
	"log"
	"log/slog"
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
	err := pA.Out(gpio.Low)
	if err != nil {
		log.Fatal("Failed to set pinA output to low")
	}

	pB := gpioreg.ByName(pinB)
	if pB == nil {
		log.Fatal("Failed to find pinB")
	}
	err = pB.Out(gpio.Low)
	if err != nil {
		log.Fatal("Failed to set pinB output to low")
	}

	eA := gpioreg.ByName(enablePinA)
	if eA == nil {
		log.Fatal("Failed to find enablePinA")
	}
	err = eA.Out(gpio.High)
	if err != nil {
		log.Fatal("Failed to set enablePinA output to low")
	}

	eB := gpioreg.ByName(enablePinB)
	if eB == nil {
		log.Fatal("Failed to find enablePinB")
	}
	err = eB.Out(gpio.Low)
	if err != nil {
		log.Fatal("Failed to set enablePinB output to low")
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
	h.mu.Lock()
	if !h.enabled {
		slog.Debug("control sent when hbridge not enabled", "percent", percent)
	}
	h.mu.Unlock()
	if percent < 0 {
		return h.Cool(-percent)
	} else if percent > 0 {
		return h.Heat(percent)
	} else {
		return h.Cool(0)
	}
}

func (h *HBridge) Cool(percent float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	err := h.set(h.pinA, 0.0)
	if err != nil {
		return err
	}
	return h.set(h.pinB, percent)
}

func (h *HBridge) Heat(percent float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	err := h.set(h.pinB, 0.0)
	if err != nil {
		return err
	}
	return h.set(h.pinA, percent)
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
	err := h.enPinA.Out(gpio.High)
	if err != nil {
		log.Fatal("Failed to set enablePinA output to high")
	}
	err = h.enPinB.Out(gpio.High)
	if err != nil {
		log.Fatal("Failed to set enablePinA output to high")
	}
}

func (h *HBridge) Disable() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.enabled = false
	h.enPinA.Out(gpio.Low)
	h.enPinB.Out(gpio.Low)
}

func (h *HBridge) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.enPinA.Out(gpio.Low)
	h.enPinB.Out(gpio.Low)
	err := h.set(h.pinA, 0.0)
	if err != nil {
		return err
	}
	return h.set(h.pinB, 0.0)
}

func (h *HBridge) HardStop() error {
	//h.enPinA.Out(gpio.Low)
	//h.enPinB.Out(gpio.Low)
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
