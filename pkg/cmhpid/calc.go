package cmhpid

import (
	"fmt"
	"log/slog"
)

func CalculatePID(ku, tu, kp, ki, kd float64, algorithm string) (float64, float64, float64, error) {
	var err error
	if tu == 0 && kp != 0 {
		return 0, 0, 0, fmt.Errorf("tu cannot be calculated from ku and kp")
	}
	slog.Info("calculating PID constants", "ku", ku, "tu", tu, "algorithm", algorithm, "module", "cmhpid")
	switch algorithm {
	case "classic":
		fallthrough
	case "ziegler-nichols":
		kp, ki, kd, err = classic(ku, tu, kp, ki, kd)
	case "pessen-integral":
		kp, ki, kd, err = pessenIntegral(ku, tu, kp, ki, kd)
	case "some-overshoot":
		kp, ki, kd, err = someOvershoot(ku, tu, kp, ki, kd)
	case "no-overshoot":
		kp, ki, kd, err = noOvershoot(ku, tu, kp, ki, kd)
	default:
		kp, ki, kd = 0, 0, 0
		err = fmt.Errorf("unknown PID algorithm: %s", algorithm)
	}

	return kp, ki, kd, err
}

func classic(ku, tu, kp, ki, kd float64) (float64, float64, float64, error) {
	switch {
	case tu == 0:
		switch {
		case ki != 0:
			ku, tu = classicFromKuKi(ku, ki)
		case kd != 0:
			ku, tu = classicFromKuKd(ku, kd)
		case kp != 0:
			return 0, 0, 0, fmt.Errorf("tu cannot be calculated from ku and kp")
		}
	case ku == 0:
		switch {
		case kp != 0:
			ku, tu = classicFromTuKp(tu, kp)
		case ki != 0:
			ku, tu = classicFromTuKi(tu, ki)
		case kd != 0:
			ku, tu = classicFromTuKd(tu, kd)
		}
	}
	kp, ki, kd = classicFromKuTu(ku, tu)
	return kp, ki, kd, nil
}

func classicFromKuTu(ku, tu float64) (float64, float64, float64) {
	kp := 0.6 * ku
	ki := 1.2 * ku / tu
	kd := 0.075 * ku * tu
	return kp, ki, kd
}

func classicFromKuKi(ku, ki float64) (float64, float64) {
	// ki := 1.2 * ku / tu
	// ki / 1.2 := ku / tu
	// tu * ki / 1.2 := ku
	tu := ku * ki / 1.2
	return ku, tu
}

func classicFromKuKd(ku, kd float64) (float64, float64) {
	tu := kd / 0.075 / ku
	return ku, tu
}

func classicFromTuKp(tu, kp float64) (float64, float64) {
	ku := kp / 0.6
	return ku, tu
}

func classicFromTuKi(tu, ki float64) (float64, float64) {
	// ki := 1.2 * ku / tu
	// ki / 1.2 := ku / tu
	// tu * ki / 1.2 := ku
	ku := tu * ki / 1.2
	return ku, tu
}

func classicFromTuKd(tu, kd float64) (float64, float64) {
	// kd := 0.075 * ku * tu
	// kd / 0.075 := ku * tu
	// kd / 0.075 / tu := ku
	ku := kd / 0.075 / tu
	return ku, tu
}

func pessenIntegral(ku, tu, kp, ki, kd float64) (float64, float64, float64, error) {
	switch {
	case tu == 0:
		switch {
		case ki != 0:
			ku, tu = pessenIntegralFromKuKi(ku, ki)
		case kd != 0:
			ku, tu = pessenIntegralFromKuKd(ku, kd)
		case kp != 0:
			return 0, 0, 0, fmt.Errorf("tu cannot be calculated from ku and kp")
		}
	case ku == 0:
		switch {
		case kp != 0:
			ku, tu = pessenIntegralFromTuKp(tu, kp)
		case ki != 0:
			ku, tu = pessenIntegralFromTuKi(tu, ki)
		case kd != 0:
			ku, tu = pessenIntegralFromTuKd(tu, kd)
		}
	}
	kp, ki, kd = pessenIntegralFromKuTu(ku, tu)
	return kp, ki, kd, nil
}

func pessenIntegralFromKuTu(ku, tu float64) (float64, float64, float64) {
	kp := 0.7 * ku
	ki := 1.75 * ku / tu
	kd := 0.105 * ku * tu
	return kp, ki, kd
}

func pessenIntegralFromKuKi(ku, ki float64) (float64, float64) {
	tu := ku * ki / 1.75
	return ku, tu
}

func pessenIntegralFromKuKd(ku, kd float64) (float64, float64) {
	tu := kd / 0.105 / ku
	return ku, tu
}

func pessenIntegralFromTuKp(tu, kp float64) (float64, float64) {
	ku := kp / 0.7
	return ku, tu
}

func pessenIntegralFromTuKi(tu, ki float64) (float64, float64) {
	// ki = 1.75 * ku / tu
	// ki / ku / 1.75 = 1 / tu
	// tu * ki / ku / 1.75 = tu / tu
	// tu * ki / ku / 1.75 = 1
	// tu * ki / 1.75 = ku
	ku := tu * ki / 1.75
	return ku, tu
}

func pessenIntegralFromTuKd(tu, kd float64) (float64, float64) {
	ku := kd / 0.105 / tu
	return ku, tu
}

func someOvershootFromKuTu(ku, tu float64) (float64, float64, float64) {
	kp := 1.0 / 3.0 * ku
	ki := 2.0 / 3.0 * ku / tu
	kd := 1.0 / 9.0 * ku * tu
	return kp, ki, kd
}

func someOvershoot(ku, tu, kp, ki, kd float64) (float64, float64, float64, error) {
	switch {
	case tu == 0:
		switch {
		case ki != 0:
			ku, tu = someOvershootFromKuKi(ku, ki)
		case kd != 0:
			ku, tu = someOvershootFromKuKd(ku, kd)
		case kp != 0:
			return 0, 0, 0, fmt.Errorf("tu cannot be calculated from ku and kp")
		}
	case ku == 0:
		switch {
		case kp != 0:
			ku, tu = someOvershootFromTuKp(tu, kp)
		case ki != 0:
			ku, tu = someOvershootFromTuKi(tu, ki)
		case kd != 0:
			ku, tu = someOvershootFromTuKd(tu, kd)
		}
	}
	kp, ki, kd = someOvershootFromKuTu(ku, tu)
	return kp, ki, kd, nil
}

// Double-check from here on
func someOvershootFromKuKi(ku, ki float64) (float64, float64) {
	tu := ku * ki / (2.0 / 3.0)
	return ku, tu
}

func someOvershootFromKuKd(ku, kd float64) (float64, float64) {
	tu := kd / (1.0 / 9.0) / ku
	return ku, tu
}

func someOvershootFromTuKp(tu, kp float64) (float64, float64) {
	ku := kp / (1.0 / 3.0)
	return ku, tu
}

func someOvershootFromTuKi(tu, ki float64) (float64, float64) {
	ku := tu * ki / (2.0 / 3.0)
	return ku, tu
}

func someOvershootFromTuKd(tu, kd float64) (float64, float64) {
	ku := kd / (1.0 / 9.0) / tu
	return ku, tu
}

func noOvershoot(ku, tu, kp, ki, kd float64) (float64, float64, float64, error) {
	switch {
	case tu == 0:
		switch {
		case ki != 0:
			ku, tu = noOvershootFromKuKi(ku, ki)
		case kd != 0:
			ku, tu = noOvershootFromKuKd(ku, kd)
		case kp != 0:
			return 0, 0, 0, fmt.Errorf("tu cannot be calculated from ku and kp")
		}
	case ku == 0:
		switch {
		case kp != 0:
			ku, tu = noOvershootFromTuKp(tu, kp)
		case ki != 0:
			ku, tu = noOvershootFromTuKi(tu, ki)
		case kd != 0:
			ku, tu = noOvershootFromTuKd(tu, kd)
		}
	}
	kp, ki, kd = noOvershootFromKuTu(ku, tu)
	return kp, ki, kd, nil
}

func noOvershootFromKuTu(ku, tu float64) (float64, float64, float64) {
	kp := 0.2 * ku
	ki := 0.4 * ku / tu
	kd := 0.2 / 3 * kp * tu
	return kp, ki, kd
}

func noOvershootFromKuKi(ku, ki float64) (float64, float64) {
	tu := ku * ki / 0.4
	return ku, tu
}

func noOvershootFromKuKd(ku, kd float64) (float64, float64) {
	tu := kd / (0.2 / 3) / ku
	return ku, tu
}

func noOvershootFromTuKp(tu, kp float64) (float64, float64) {
	ku := kp / 0.2
	return ku, tu
}

func noOvershootFromTuKi(tu, ki float64) (float64, float64) {
	ku := tu * ki / 0.4
	return ku, tu
}

func noOvershootFromTuKd(tu, kd float64) (float64, float64) {
	ku := kd / (0.2 / 3) / tu
	return ku, tu
}
