package mqtt

type Sample struct {
	count int
	rate  int
}

func NewSample(rate int) *Sample {
	return &Sample{rate: rate}
}

func (s *Sample) Ready() bool {
	s.count++
	if s.count%s.rate == 0 {
		s.count = 0
		return true
	}
	return false
}
