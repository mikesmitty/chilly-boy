package swma

type SlidingWindow struct {
	sum        float64
	window     []float64
	windowSize int
}

func NewSlidingWindow(windowSize int) *SlidingWindow {
	return &SlidingWindow{
		window:     make([]float64, windowSize),
		windowSize: windowSize,
	}
}

func (s *SlidingWindow) Add(value float64) float64 {
	s.sum += value
	s.sum -= s.window[0]
	s.window = append(s.window[1:], value)
	return s.sum / float64(s.windowSize)
}

func (s *SlidingWindow) Average() float64 {
	return s.sum / float64(s.windowSize)
}

func (s *SlidingWindow) Reset() {
	s.sum = 0
	s.window = make([]float64, 0, s.windowSize)
}
func (s *SlidingWindow) Sum() float64 {
	return s.sum
}

func (s *SlidingWindow) Window() []float64 {
	return s.window
}

func (s *SlidingWindow) WindowSize() int {
	return s.windowSize
}
