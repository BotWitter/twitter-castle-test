package transaction

import "fmt"

// Interpolate is a linear blend: from*(1-f) + to*f, element-wise.
// Panics on length mismatch.
func Interpolate(from, to []float64, f float64) []float64 {
	if len(from) != len(to) {
		panic(fmt.Sprintf("Mismatched interpolation arguments %v: %v", from, to))
	}
	out := make([]float64, len(from))
	for i := range from {
		out[i] = from[i]*(1-f) + to[i]*f
	}
	return out
}
