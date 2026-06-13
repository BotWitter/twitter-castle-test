package transaction

import "math"

// RotationMatrix returns the 4-element form [cos, -sin, sin, cos]. Input in degrees.
func RotationMatrix(degrees float64) [4]float64 {
	rad := degrees * math.Pi / 180
	c, s := math.Cos(rad), math.Sin(rad)
	return [4]float64{c, -s, s, c}
}
