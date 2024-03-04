package cmhpid

import (
	"fmt"

	"github.com/mikesmitty/pidcalc"
)

func CalculatePID(ku, tu, kp, ki, kd float64, algorithm string) (float64, float64, float64, error) {
	var algo int
	switch algorithm {
	case "classic":
		fallthrough
	case "ziegler-nichols":
		algo = pidcalc.ZieglerNichols
	case "pessen-integral":
		algo = pidcalc.PessenIntegral
	case "some-overshoot":
		algo = pidcalc.SomeOvershoot
	case "no-overshoot":
		algo = pidcalc.NoOvershoot
	case "tyreus-luyben":
		algo = pidcalc.TyreusLuyben
	//case "ciancone-marlin":
	//	algo = pidcalc.CianconeMarlin
	default:
		return 0, 0, 0, fmt.Errorf("unknown PID algorithm: %s", algorithm)
	}
	var err error
	kp, ki, kd, err = pidcalc.Calculate(ku, tu, kp, ki, kd, algo)
	return kp, ki, kd, err
}
