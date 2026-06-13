package transaction

// FloatToHex converts a float into the hex string the client expects.
// The algorithm is custom — DO NOT substitute strconv.FormatFloat.
//
// Integer truncation is toward zero, not floor, which the Go int conversion does.
func FloatToHex(x float64) string {
	var head []byte
	quotient := int(x)
	fraction := x - float64(quotient)

	// Integer part: emit hex digits MSB-first by prepending.
	// Loop terminates when quotient hits 0.
	for quotient > 0 {
		quotient = int(x / 16)
		remainder := int(x - float64(quotient)*16)
		var digit byte
		if remainder > 9 {
			digit = byte(remainder + 55) // 'A' = 65 = 10+55
		} else {
			digit = byte('0' + remainder)
		}
		head = append([]byte{digit}, head...)
		x = float64(quotient)
	}

	if fraction == 0 {
		return string(head)
	}

	out := append(head, '.')
	// Fractional part: bounded loop — float64 mantissa exhausts within ~14 hex
	// digits, so there is no natural termination guard. Cap at 32 for safety.
	for i := 0; i < 32 && fraction > 0; i++ {
		fraction *= 16
		integer := int(fraction)
		fraction -= float64(integer)
		if integer > 9 {
			out = append(out, byte(integer+55))
		} else {
			out = append(out, byte('0'+integer))
		}
	}

	return string(out)
}
