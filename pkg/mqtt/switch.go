package mqtt

import (
	"bytes"
	"fmt"
	"log/slog"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

func (c *Client) SwitchFn(name string, onFn func(), offFn func(), stateFn func() bool) func() error {
	topicPrefix := fmt.Sprintf("%s/switch/%s/", c.topicPrefix, name)
	commandTopic := topicPrefix + "command"
	stateTopic := topicPrefix + "state"

	return func() error {
		t := time.NewTicker(5 * time.Second)
		go func() {
			for range t.C {
				state := "OFF"
				if stateFn() {
					state = "ON"
				}
				if !c.client.IsConnected() {
					slog.Error("mqtt client not connected", "switch", name)
					continue
				}
				c.Publish(stateTopic, state)
			}
		}()

		for !c.client.IsConnected() {
			time.Sleep(1 * time.Second)
		}

		slog.Debug("subscribing to mqtt switch", "switch", name, "topic", commandTopic)
		if token := c.client.Subscribe(commandTopic, c.qos, func(client paho.Client, msg paho.Message) {
			slog.Debug("mqtt switch command received", "switch", name, "command", msg.Payload(), "topic", commandTopic)
			if bytes.Equal(msg.Payload(), []byte("ON")) {
				onFn()
			} else {
				offFn()
			}
		}); token.Wait() && token.Error() != nil {
			slog.Error("mqtt subscription failed", "switch", name, "error", token.Error())
		}
		return nil
	}
}
