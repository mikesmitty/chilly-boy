package cmhmqtt

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/mikesmitty/chilly-boy/pkg/cmhpid"
)

type Client struct {
	client      mqtt.Client
	topicPrefix string
	qos         byte
	retained    bool
}

func NewClient(broker *url.URL) *Client {
	c := &Client{}

	var urls []*url.URL
	urls = append(urls, broker)

	hostname, _ := os.Hostname()
	hostname = strings.Split(hostname, ".")[0]
	clientID := hostname
	if clientID == "" {
		now := time.Now().UnixNano()
		sum := md5.New().Sum([]byte(strconv.FormatInt(now, 10)))
		clientID = string(sum)
	}

	c.qos = 1
	c.topicPrefix = "chilly-boy/" + hostname

	slog.Info("connecting to mqtt", "url", broker, "clientid", clientID)
	c.client = mqtt.NewClient(&mqtt.ClientOptions{
		Servers:        urls,
		ClientID:       clientID,
		ConnectTimeout: 30 * time.Second,
	})

	return c
}

func (c *Client) GetPublisher(tempChan <-chan float64, lightChan <-chan uint64, pidChan <-chan cmhpid.ControllerState) func() error {
	tempTopic := c.topicPrefix + "/mirror_temperature"
	lightTopic := c.topicPrefix + "/mirror_infrared_light"
	pidTopicDiffLight := c.topicPrefix + "/mirror_pid_light_diff"
	//pidTopicDiffTemp := c.topicPrefix + "/mirror_pid_temp_diff"
	pidTopicError := c.topicPrefix + "/mirror_pid_error"
	pidTopicIntegral := c.topicPrefix + "/mirror_pid_integral"
	pidTopicDerivative := c.topicPrefix + "/mirror_pid_derivative"
	pidTopicSignal := c.topicPrefix + "/mirror_pid_signal"
	return func() error {
		go func() {
			for {
				select {
				case temp := <-tempChan:
					slog.Debug("mqtt publishing", "field", "rtd", "value", temp, "topic", tempTopic, "module", "cmhmqtt")
					c.Publish(tempTopic, strconv.FormatFloat(temp, 'f', -1, 64))
				case light := <-lightChan:
					slog.Debug("mqtt publishing", "field", "light", "value", light, "topic", lightTopic, "module", "cmhmqtt")
					c.Publish(lightTopic, strconv.FormatUint(light, 10))
				case pid := <-pidChan:
					slog.Debug("mqtt publishing", "field", "pid state", "value", pid, "module", "cmhmqtt")
					c.Publish(pidTopicDiffLight, strconv.FormatFloat(pid.LightDiff, 'f', 2, 64))
					//c.Publish(pidTopicDiffTemp, strconv.FormatFloat(pid.TempDiff, 'f', 2, 64))
					c.Publish(pidTopicError, strconv.FormatFloat(pid.ControlError, 'f', 2, 64))
					c.Publish(pidTopicIntegral, strconv.FormatFloat(pid.ControlErrorIntegral, 'f', 2, 64))
					c.Publish(pidTopicDerivative, strconv.FormatFloat(pid.ControlErrorDerivative, 'f', 2, 64))
					c.Publish(pidTopicSignal, strconv.FormatFloat(pid.Signal, 'f', 2, 64))
				}
			}
		}()
		if token := c.client.Connect(); token.Wait() && token.Error() != nil {
			slog.Error("mqtt connection failed", "error", token.Error())
			return token.Error()
		}

		return nil
	}
}

func (p *Client) Publish(topic string, msg string) {
	t := p.client.Publish(topic, p.qos, p.retained, msg)
	go func() {
		_ = t.WaitTimeout(5 * time.Second)
		if t.Error() != nil {
			slog.Error("mqtt message publish failed", "error", t.Error())
		}
	}()
}

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
		if token := c.client.Subscribe(commandTopic, c.qos, func(client mqtt.Client, msg mqtt.Message) {
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
