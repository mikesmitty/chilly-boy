package mqtt

import (
	"crypto/md5"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/mikesmitty/chilly-boy/pkg/cmhpid"
	"github.com/mikesmitty/chilly-boy/pkg/env"
)

type Client struct {
	client      paho.Client
	clientID    string
	topicPrefix string
	qos         byte
	retained    bool
	sampleRate  int
	hassSensors map[string]HassSensor
	mu          sync.Mutex
}

func NewClient(broker *url.URL, sampleRate int, pidInterval time.Duration) *Client {
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
	c.clientID = clientID
	c.hassSensors = make(map[string]HassSensor)

	slog.Info("connecting to mqtt", "url", broker, "clientid", clientID)
	c.client = paho.NewClient(&paho.ClientOptions{
		Servers:        urls,
		ClientID:       clientID,
		ConnectRetry:   true,
		ConnectTimeout: 30 * time.Second,
	})

	c.sampleRate = sampleRate

	return c
}

func (c *Client) Connect() error {
	if token := c.client.Connect(); token.Wait() && token.Error() != nil {
		slog.Error("mqtt connection failed", "error", token.Error())
		return token.Error()
	}
	return nil
}

func (c *Client) Subscribe(topic string, handler paho.MessageHandler) error {
	if token := c.client.Subscribe(topic, c.qos, handler); token.Wait() && token.Error() != nil {
		slog.Error("mqtt subscription failed", "error", token.Error())
		return token.Error()
	}
	return nil
}

func (c *Client) GetPublisher(tempChan, dewptChan, lightChan, dutyChan <-chan float64, pidChan <-chan cmhpid.ControllerState, refChan <-chan env.Env) func() error {
	tempSensor := c.RegisterHassSensor(c.NewHassSensor("Mirror Temperature", HassSensorTemperature))
	dewpointSensor := c.RegisterHassSensor(c.NewHassSensor("Dewpoint", HassSensorTemperature))
	lightSensor := c.RegisterHassSensor(c.NewHassSensor("Infrared Light", HassSensorIlluminance))
	dutyCycleSensor := c.RegisterHassSensor(c.NewHassSensor("Duty Cycle", HassSensorIlluminance))
	pidLightDiff := c.RegisterHassSensor(c.NewHassSensor("PID Light Diff", HassSensorIlluminance))
	pidTempDiff := c.RegisterHassSensor(c.NewHassSensor("PID Temp Diff", HassSensorIlluminance))
	pidFeedForward := c.RegisterHassSensor(c.NewHassSensor("PID Feed Forward", HassSensorIlluminance))
	pidError := c.RegisterHassSensor(c.NewHassSensor("PID Error", HassSensorIlluminance))
	pidIntegral := c.RegisterHassSensor(c.NewHassSensor("PID Integral", HassSensorIlluminance))
	pidDerivative := c.RegisterHassSensor(c.NewHassSensor("PID Derivative", HassSensorIlluminance))
	pidSignal := c.RegisterHassSensor(c.NewHassSensor("PID Signal", HassSensorIlluminance))
	pidSignalInput := c.RegisterHassSensor(c.NewHassSensor("PID Signal Input", HassSensorIlluminance))
	pidSetpoint := c.RegisterHassSensor(c.NewHassSensor("PID Setpoint", HassSensorIlluminance))
	pidLinear := c.RegisterHassSensor(c.NewHassSensor("PID Linear", HassSensorIlluminance))
	refTemp := c.RegisterHassSensor(c.NewHassSensor("Reference Temperature", HassSensorTemperature))
	refHumidity := c.RegisterHassSensor(c.NewHassSensor("Reference Humidity", HassSensorHumidity))
	refDewpoint := c.RegisterHassSensor(c.NewHassSensor("Reference Dewpoint", HassSensorTemperature))

	dewpointSample := NewSample(c.sampleRate)
	tempSample := NewSample(c.sampleRate)
	lightSample := NewSample(c.sampleRate)
	dutySample := NewSample(c.sampleRate)
	pidSample := NewSample(c.sampleRate)
	refSample := NewSample(c.sampleRate)

	return func() error {
		for {
			select {
			case dewpt := <-dewptChan:
				if !dewpointSample.Ready() {
					continue
				}
				slog.Debug("mqtt publishing", "field", "dewpoint", "value", dewpt)
				c.HassPublishSensor(dewpointSensor, strconv.FormatFloat(dewpt, 'f', 5, 64))
			case temp := <-tempChan:
				if !tempSample.Ready() {
					continue
				}
				slog.Debug("mqtt publishing", "field", "rtd", "value", temp)
				c.HassPublishSensor(tempSensor, strconv.FormatFloat(temp, 'f', 5, 64))
			case light := <-lightChan:
				if !lightSample.Ready() {
					continue
				}
				slog.Debug("mqtt publishing", "field", "light", "value", light)
				c.HassPublishSensor(lightSensor, strconv.FormatFloat(light, 'f', 2, 64))
			case duty := <-dutyChan:
				if !dutySample.Ready() {
					continue
				}
				slog.Debug("mqtt publishing", "field", "duty", "value", duty)
				c.HassPublishSensor(dutyCycleSensor, strconv.FormatFloat(duty, 'f', 2, 64))
			case pid := <-pidChan:
				if !pidSample.Ready() {
					continue
				}
				slog.Debug("mqtt publishing", "field", "pid state", "value", pid)
				c.HassPublishSensor(pidLightDiff, strconv.FormatFloat(pid.LightDiff, 'f', 2, 64))
				c.HassPublishSensor(pidTempDiff, strconv.FormatFloat(pid.TempDiff, 'f', 2, 64))
				c.HassPublishSensor(pidFeedForward, strconv.FormatFloat(pid.FeedForward, 'f', 2, 64))
				c.HassPublishSensor(pidError, strconv.FormatFloat(pid.ControlError, 'f', 2, 64))
				c.HassPublishSensor(pidIntegral, strconv.FormatFloat(pid.ControlErrorIntegral, 'f', 2, 64))
				c.HassPublishSensor(pidDerivative, strconv.FormatFloat(pid.ControlErrorDerivative, 'f', 2, 64))
				c.HassPublishSensor(pidSignal, strconv.FormatFloat(pid.ControlSignal, 'f', 2, 64))
				c.HassPublishSensor(pidSignalInput, strconv.FormatFloat(pid.SignalInput, 'f', 2, 64))
				c.HassPublishSensor(pidSetpoint, strconv.FormatFloat(pid.SetPoint, 'f', 4, 64))
				c.HassPublishSensor(pidLinear, strconv.FormatFloat(pid.Linear, 'f', 4, 64))
			case ref := <-refChan:
				if !refSample.Ready() {
					continue
				}
				c.HassPublishSensor(refTemp, strconv.FormatFloat(ref.Temperature, 'f', 2, 64))
				c.HassPublishSensor(refHumidity, strconv.FormatFloat(ref.Humidity, 'f', 2, 64))
				c.HassPublishSensor(refDewpoint, strconv.FormatFloat(ref.Dewpoint, 'f', 2, 64))
			}
		}
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
