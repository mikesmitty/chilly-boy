package stats

import (
	"math"

	"gonum.org/v1/gonum/stat"
)

type Stats struct {
	offsetNegative bool
	offset         int
	size           int
	values         []float64
	x              []float64
	y              []float64
}

func NewStats(size, offset int) *Stats {
	offsetNegative := offset < 0
	if offsetNegative {
		offset = -offset
	}
	x := make([]float64, size)
	for i := range x {
		x[i] = float64(i + 1)
	}
	return &Stats{
		offset:         offset,
		offsetNegative: offsetNegative,
		size:           size,
		values:         make([]float64, size+offset),
		x:              x,
		y:              make([]float64, size),
	}
}

func (p *Stats) getValues() []float64 {
	if p.offsetNegative {
		return p.values[p.offset:]
	}
	return p.values[:p.size]
}

func (p *Stats) LinearRegression() (float64, float64) {
	return stat.LinearRegression(p.x, p.getValues(), nil, false)
}

func (p *Stats) Add(value float64) {
	p.values = append(p.values[1:], value)
}

func (p *Stats) StdDev() float64 {
	return stat.StdDev(p.getValues(), nil)
}

func (p *Stats) ResidualStandardDeviation(m float64) float64 {
	// Get the average difference between the actual and predicted values to account for sensor calibration
	val := p.getValues()
	sum := 0.0
	for i := range val {
		p.y[i] = m * p.x[i]
		sum += math.Abs(val[i] - p.y[i])
	}
	avgDiff := sum / float64(p.size)

	// Calculate the residual standard deviation minus the average difference
	sum = 0.0
	for i, v := range val {
		y := v - avgDiff
		sum += math.Pow(y-p.y[i], 2)
	}
	return math.Sqrt(sum / float64(p.size-2))
}

func (p *Stats) QuantileSpread(pct float64) float64 {
	// Calculate the residuals between the actual and predicted values
	b, m := p.LinearRegression()
	for i, v := range p.getValues() {
		y := m*p.x[i] + b
		p.y[i] = math.Abs(v - y)
	}
	return stat.Quantile(pct, stat.Empirical, p.y, nil)
}
