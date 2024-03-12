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
	"github.com/mikesmitty/chilly-boy/pkg/env"
	"github.com/mikesmitty/chilly-boy/pkg/swma"
)

type Client struct {
	client         mqtt.Client
	topicPrefix    string
	qos            byte
	retained       bool
	tempAvg        *swma.SlidingWindow
	sampleInterval int
}

func NewClient(broker *url.URL, sampleInterval int, pidInterval time.Duration) *Client {
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

	c.sampleInterval = sampleInterval

	return c
}

func (c *Client) GetPublisher(tempChan, dewptChan, lightChan, dutyChan <-chan float64, pidChan <-chan cmhpid.ControllerState, refChan <-chan env.Env) func() error {
	tempTopic := c.topicPrefix + "/mirror_temperature"
	dewpointTopic := c.topicPrefix + "/dewpoint"
	lightTopic := c.topicPrefix + "/mirror_infrared_light"
	dutyCycleTopic := c.topicPrefix + "/mirror_duty_cycle"
	pidTopicDiffLight := c.topicPrefix + "/mirror_pid_light_diff"
	pidTopicDiffTemp := c.topicPrefix + "/mirror_pid_temp_diff"
	pidTopicFeedForward := c.topicPrefix + "/mirror_pid_feed_forward"
	pidTopicError := c.topicPrefix + "/mirror_pid_error"
	pidTopicIntegral := c.topicPrefix + "/mirror_pid_integral"
	pidTopicDerivative := c.topicPrefix + "/mirror_pid_derivative"
	pidTopicSignal := c.topicPrefix + "/mirror_pid_signal"
	pidTopicSignalInput := c.topicPrefix + "/mirror_pid_signal_input"
	refTopicTemp := c.topicPrefix + "/mirror_reference_temperature"
	refTopicHumidity := c.topicPrefix + "/mirror_reference_humidity"
	refTopicDewpoint := c.topicPrefix + "/mirror_reference_dewpoint"

	i, j, k, l, m, n := 0, 0, 0, 0, 0, 0
	return func() error {
		go func() {
			for {
				select {
				case dewpt := <-dewptChan:
					i++
					if i%c.sampleInterval != 0 {
						continue
					}
					i = 0
					slog.Debug("mqtt publishing", "field", "dewpoint", "value", dewpt, "topic", dewpointTopic)
					c.Publish(dewpointTopic, strconv.FormatFloat(dewpt, 'f', -1, 64))
				case temp := <-tempChan:
					j++
					if j%c.sampleInterval != 0 {
						continue
					}
					j = 0
					slog.Debug("mqtt publishing", "field", "rtd", "value", temp, "topic", tempTopic)
					c.Publish(tempTopic, strconv.FormatFloat(temp, 'f', -1, 64))
				case light := <-lightChan:
					k++
					if k%c.sampleInterval != 0 {
						continue
					}
					k = 0
					slog.Debug("mqtt publishing", "field", "light", "value", light, "topic", lightTopic)
					c.Publish(lightTopic, strconv.FormatFloat(light, 'f', 2, 64))
				case duty := <-dutyChan:
					l++
					if l%c.sampleInterval != 0 {
						continue
					}
					l = 0
					slog.Debug("mqtt publishing", "field", "duty", "value", duty, "topic", dutyCycleTopic)
					c.Publish(dutyCycleTopic, strconv.FormatFloat(duty, 'f', 2, 64))
				case pid := <-pidChan:
					m++
					if m%c.sampleInterval != 0 {
						continue
					}
					m = 0
					slog.Debug("mqtt publishing", "field", "pid state", "value", pid)
					c.Publish(pidTopicDiffLight, strconv.FormatFloat(pid.LightDiff, 'f', 2, 64))
					c.Publish(pidTopicDiffTemp, strconv.FormatFloat(pid.TempDiff, 'f', 2, 64))
					c.Publish(pidTopicFeedForward, strconv.FormatFloat(pid.FeedForward, 'f', 2, 64))
					c.Publish(pidTopicError, strconv.FormatFloat(pid.ControlError, 'f', 2, 64))
					c.Publish(pidTopicIntegral, strconv.FormatFloat(pid.ControlErrorIntegral, 'f', 2, 64))
					c.Publish(pidTopicDerivative, strconv.FormatFloat(pid.ControlErrorDerivative, 'f', 2, 64))
					c.Publish(pidTopicSignal, strconv.FormatFloat(pid.ControlSignal, 'f', 2, 64))
					c.Publish(pidTopicSignalInput, strconv.FormatFloat(pid.SignalInput, 'f', 2, 64))
				case ref := <-refChan:
					n++
					if n%c.sampleInterval != 0 {
						continue
					}
					n = 0
					c.Publish(refTopicTemp, strconv.FormatFloat(ref.Temperature, 'f', 2, 64))
					c.Publish(refTopicHumidity, strconv.FormatFloat(ref.Humidity, 'f', 2, 64))
					c.Publish(refTopicDewpoint, strconv.FormatFloat(ref.Dewpoint, 'f', 2, 64))
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
