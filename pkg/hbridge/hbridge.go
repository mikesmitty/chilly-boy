package hbridge

import (
	"fmt"
	"log/slog"
	"math"
	"sync"

	"github.com/stianeikeland/go-rpio/v4"
)

const kHz = 1000

type HBridge struct {
	freq    int
	pinA    rpio.Pin
	pinB    rpio.Pin
	enPinA  rpio.Pin
	enPinB  rpio.Pin
	enabled bool
	mu      sync.Mutex
}

func NewHBridge(pinA, pinB, enablePinA, enablePinB int) (*HBridge, error) {
	err := rpio.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open rpio: %w", err)
	}

	hb := &HBridge{
		freq:   1 * kHz,
		pinA:   pinInit(pinA),
		pinB:   pinInit(pinB),
		enPinA: pinInit(enablePinA),
		enPinB: pinInit(enablePinB),
	}

	return hb, nil
}

func (h *HBridge) Control(percent float64) error {
	if !h.enabled {
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
	return h.control(uint32(math.Round(percent)), 0)
}

func (h *HBridge) Heat(percent float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.control(0, uint32(math.Round(percent)))
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
	h.enPinA.High()
	h.enPinB.High()
}

func (h *HBridge) Disable() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.disable()
}

func (h *HBridge) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.set(h.pinA, 0)
	h.set(h.pinB, 0)
}

func (h *HBridge) HardStop() error {
	h.set(h.pinA, 0)
	h.set(h.pinB, 0)
	return h.disable()
}

func (h *HBridge) control(cool, heat uint32) error {
	if cool > 0 && heat > 0 {
		return fmt.Errorf("invalid hbridge control: cool=%d, heat=%d", cool, heat)
	}
	errA := h.set(h.pinA, heat)
	errB := h.set(h.pinB, cool)
	if errA != nil || errB != nil {
		return fmt.Errorf("failed to set hbridge pwm: %v, %v", errA, errB)
	}
	return nil
}

func (h *HBridge) disable() error {
	h.enPinA.Low()
	h.enPinB.Low()
	if h.enPinA.Read() != rpio.Low || h.enPinB.Read() != rpio.Low {
		return fmt.Errorf("failed to disable hbridge")
	}
	h.enabled = false
	return nil
}

func (h *HBridge) set(pin rpio.Pin, percent uint32) error {
	if percent > 100 {
		return fmt.Errorf("pwm percent cannot be more than 100: %d", percent)
	}
	pin.Pwm()
	pin.DutyCycle(percent, 101-percent)
	pin.Freq(h.freq * 10)
	return nil
}

func pinInit(number int) rpio.Pin {
	p := rpio.Pin(number)
	p.Output()
	p.Low()
	return p
}
