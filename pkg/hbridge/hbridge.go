package hbridge

import (
	"fmt"
	"log"
	"log/slog"
	"strconv"
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
		log.Fatal("Failed to set pinA output low")
	}

	pB := gpioreg.ByName(pinB)
	if pB == nil {
		log.Fatal("Failed to find pinB")
	}
	err = pB.Out(gpio.Low)
	if err != nil {
		log.Fatal("Failed to set pinB output low")
	}

	eA := gpioreg.ByName(enablePinA)
	if eA == nil {
		log.Fatal("Failed to find enablePinA")
	}
	err = eA.Out(gpio.High)
	if err != nil {
		log.Fatal("Failed to set enablePinA output low")
	}

	eB := gpioreg.ByName(enablePinB)
	if eB == nil {
		log.Fatal("Failed to find enablePinB")
	}
	err = eB.Out(gpio.Low)
	if err != nil {
		log.Fatal("Failed to set enablePinB output low")
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
	if !h.GetEnable() {
		slog.Debug("control sent when hbridge not enabled", "percent", percent)
	}
	if percent < 0 {
		// Convert the negative percentage to positive
		percent = -percent
		return h.Cool(percent)
	} else if percent > 0 {
		return h.Heat(percent)
	} else {
		return h.control(0, 0)
	}
}

func (h *HBridge) Cool(percent float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.control(percent, 0)
}

func (h *HBridge) Heat(percent float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.control(0, percent)
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
	h.disable()
}

func (h *HBridge) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.control(0, 0)
}

func (h *HBridge) HardStop() error {
	errA := h.disable()
	errB := h.control(0, 0)
	if errA != nil {
		return errA
	}
	if errB != nil {
		return errB
	}
	return nil
}

func (h *HBridge) control(cool, heat float64) error {
	errA := h.set(h.pinA, heat)
	errB := h.set(h.pinB, cool)
	if cool > 0 && heat > 0 || cool == 0 && heat == 0 {
		slog.Info("hbridge control", "cool", strconv.FormatFloat(cool, 'f', 2, 64), "heat", strconv.FormatFloat(heat, 'f', 2, 64))
	}
	if errA != nil || errB != nil {
		return fmt.Errorf("failed to set hbridge pwm: %v, %v", errA, errB)
	}
	return nil
}

func (h *HBridge) disable() error {
	errA := h.enPinA.Out(gpio.Low)
	errB := h.enPinB.Out(gpio.Low)
	if errA != nil || errB != nil {
		return fmt.Errorf("failed to disable hbridge: %v, %v", errA, errB)
	}
	h.enabled = false
	return nil
}

func (h *HBridge) set(pin gpio.PinOut, percent float64) error {
	dutyCycle := gpio.DutyMax * gpio.Duty(percent/100.0)
	if err := pin.PWM(dutyCycle, h.freq); err != nil {
		return err
	}
	return nil
}
