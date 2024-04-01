package mqtt

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	paho "github.com/eclipse/paho.mqtt.golang"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	HassSensorGeneric HassSensorType = iota
	HassSensorIlluminance
	HassSensorTemperature
	HassSensorHumidity
)

type HassSensorType int

type HassSensor struct {
	configTopic       string
	Name              string     `json:"name"`
	UniqueID          string     `json:"unique_id"`
	Device            HassDevice `json:"device,omitempty"`
	DeviceClass       string     `json:"device_class,omitempty"`
	StateTopic        string     `json:"state_topic"`
	UnitOfMeasurement string     `json:"unit_of_measurement,omitempty"`
	Icon              string     `json:"icon,omitempty"`
}

type HassDevice struct {
	Name        string   `json:"name,omitempty"`
	Identifiers []string `json:"identifiers,omitempty"`
	Model       string   `json:"model,omitempty"`
}

func (c *Client) HomeAssistant() error {
	c.HassAnnounceAll()
	topic := "homeassistant/status"
	return c.Subscribe(topic, func(client paho.Client, msg paho.Message) {
		payload := string(msg.Payload())
		slog.Info("homeassistant status watcher", "status", payload)
		if payload == "online" {
			c.HassAnnounceAll()
		}
	})
}

func (c *Client) HassAnnounceAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	slog.Info("announcing homeassistant sensors")
	for _, sensor := range c.hassSensors {
		c.HassAnnounceSensor(sensor)
	}
}

func (c *Client) NewHassSensor(name string, sensorType HassSensorType) HassSensor {
	var deviceClass string
	var unit string
	switch sensorType {
	case HassSensorIlluminance:
		deviceClass = "illuminance"
		unit = "#"
	case HassSensorTemperature:
		deviceClass = "temperature"
		unit = "°C"
	case HassSensorHumidity:
		deviceClass = "humidity"
		unit = "%"
	}
	return HassSensor{
		Name: name,
		Device: HassDevice{
			Name: cases.Title(language.English).String(c.clientID),
		},
		StateTopic:        c.topicPrefix + "/sensor/" + name,
		DeviceClass:       deviceClass,
		UnitOfMeasurement: unit,
	}
}

func (c *Client) RegisterHassSensor(sensor HassSensor) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sensor.UniqueID == "" {
		sensor.UniqueID = slugify(sensor.Device.Name + "_" + sensor.Name)
	}
	if len(sensor.Device.Identifiers) == 0 {
		sensor.Device.Identifiers = []string{slugify(sensor.Device.Name)}
	}
	sensor.configTopic = "homeassistant/sensor/" + sensor.UniqueID + "/config"
	c.hassSensors[sensor.UniqueID] = sensor
	return sensor.UniqueID
}

func (c *Client) HassAnnounceSensor(sensor HassSensor) {
	payload, err := json.Marshal(sensor)
	if err != nil {
		slog.Error("json marshal error", "error", err, "module", "mqtt", "sensor", sensor)
		return
	}
	c.Publish(sensor.configTopic, string(payload))
}

func (c *Client) HassPublishSensor(uniqueID, state string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	sensor, ok := c.hassSensors[uniqueID]
	if !ok {
		return fmt.Errorf("sensor not found: %s", uniqueID)
	}
	c.Publish(sensor.StateTopic, state)
	return nil
}

func slugify(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "_")
}
