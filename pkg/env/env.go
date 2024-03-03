package env

import (
	"math"
)

type Env struct {
	Temperature float64
	Humidity    float64
	Dewpoint    float64
}

func New(temp, humidity float64) Env {
	return Env{
		Temperature: temp,
		Humidity:    humidity,
		Dewpoint:    dewpoint(temp, humidity),
	}
}

func dewpoint(t, rh float64) float64 {
	return (243.04 * (math.Log(float64(rh)/100) + (17.625*t)/(243.04+t)) / (17.625 - math.Log(float64(rh)/100) - (17.625*t)/(243.04+t)))
}
