package transaction

import (
	"encoding/base64"
	"math"
)

// JSRound replicates JavaScript's Math.round: half-away-from-zero, sign preserved.
// This differs from banker's rounding — DO NOT substitute a round-half-to-even helper.
func JSRound(num float64) float64 {
	x := math.Floor(num)
	if (num - x) >= 0.5 {
		x = math.Ceil(num)
	}
	return math.Copysign(x, num)
}

// IsOdd returns -1.0 for odd, 0.0 for even.
// Takes an int because the caller passes an enumeration counter.
func IsOdd(n int) float64 {
	if n%2 != 0 {
		return -1.0
	}
	return 0.0
}

// Base64Encode returns standard padded base64.
func Base64Encode(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

// Base64StripPad — final transaction-id encoding strips trailing '='.
func Base64StripPad(b []byte) string {
	return base64.RawStdEncoding.EncodeToString(b)
}
