package transaction

import "math"

// Cubic is a cubic-bezier solver.
// curves is [a0, a1, a2, a3] where animate() builds it as solve(frames[7+i], is_odd(i), 1, false).
type Cubic struct {
	Curves []float64
}

func NewCubic(curves []float64) *Cubic { return &Cubic{Curves: curves} }

// Value solves for the curve value via bisection with 1e-5 tolerance.
func (c *Cubic) Value(t float64) float64 {
	var startGradient, endGradient float64
	start, mid, end := 0.0, 0.0, 1.0

	if t <= 0.0 {
		if c.Curves[0] > 0.0 {
			startGradient = c.Curves[1] / c.Curves[0]
		} else if c.Curves[1] == 0.0 && c.Curves[2] > 0.0 {
			startGradient = c.Curves[3] / c.Curves[2]
		}
		return startGradient * t
	}
	if t >= 1.0 {
		if c.Curves[2] < 1.0 {
			endGradient = (c.Curves[3] - 1.0) / (c.Curves[2] - 1.0)
		} else if c.Curves[2] == 1.0 && c.Curves[0] < 1.0 {
			endGradient = (c.Curves[1] - 1.0) / (c.Curves[0] - 1.0)
		}
		return 1.0 + endGradient*(t-1.0)
	}

	for start < end {
		mid = (start + end) / 2
		xEst := cubicCalc(c.Curves[0], c.Curves[2], mid)
		if math.Abs(t-xEst) < 0.00001 {
			return cubicCalc(c.Curves[1], c.Curves[3], mid)
		}
		if xEst < t {
			start = mid
		} else {
			end = mid
		}
	}
	return cubicCalc(c.Curves[1], c.Curves[3], mid)
}

// cubicCalc: 3a(1-m)²m + 3b(1-m)m² + m³
func cubicCalc(a, b, m float64) float64 {
	return 3.0*a*(1-m)*(1-m)*m + 3.0*b*(1-m)*m*m + m*m*m
}
