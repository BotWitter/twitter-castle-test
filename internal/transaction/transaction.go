package transaction

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ClientTransaction generates the x-client-transaction-id header.
// All bulky DOM refs are dropped after construction — only derived state is kept.
type ClientTransaction struct {
	RandomKeyword   string
	RandomNumber    int
	RowIndex        int
	KeyBytesIndices []int
	Key             string
	KeyBytes        []byte
	AnimationKey    string
}

// Option lets callers override defaults at construction.
type Option func(*ClientTransaction)

func WithRandomKeyword(s string) Option { return func(c *ClientTransaction) { c.RandomKeyword = s } }
func WithRandomNumber(n int) Option     { return func(c *ClientTransaction) { c.RandomNumber = n } }

// New builds a ClientTransaction from the x.com home HTML and the ondemand.s.*.js body.
func New(homeHTML []byte, ondemandJS string, opts ...Option) (*ClientTransaction, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(homeHTML))
	if err != nil {
		return nil, fmt.Errorf("parse home html: %w", err)
	}

	c := &ClientTransaction{
		RandomKeyword: DefaultKeyword,
		RandomNumber:  AdditionalRandomNumber,
	}
	for _, o := range opts {
		o(c)
	}

	c.RowIndex, c.KeyBytesIndices, err = extractIndices(ondemandJS)
	if err != nil {
		return nil, err
	}
	c.Key, err = extractKey(doc)
	if err != nil {
		return nil, err
	}
	c.KeyBytes, err = base64.StdEncoding.DecodeString(c.Key)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	c.AnimationKey, err = c.computeAnimationKey(doc)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// solve maps a value into [minVal, maxVal].
// rounding=true → math.floor; rounding=false → round to 2 decimals (banker's rounding).
func solve(value, minVal, maxVal float64, rounding bool) float64 {
	r := value*(maxVal-minVal)/255 + minVal
	if rounding {
		return math.Floor(r)
	}
	return pyRound2(r)
}

// pyRound2: round(x, 2) using banker's rounding (round-half-to-even).
// Go's math.RoundToEven handles half-to-even but works on integers; scale first.
func pyRound2(x float64) float64 {
	scaled := x * 100
	return math.RoundToEven(scaled) / 100
}

// computeAnimationKey derives the animation key from the home page document.
func (c *ClientTransaction) computeAnimationKey(doc *goquery.Document) (string, error) {
	const totalTime = 4096

	rowIndex := int(c.KeyBytes[c.RowIndex]) % 16

	frameTime := 1
	for _, idx := range c.KeyBytesIndices {
		frameTime *= int(c.KeyBytes[idx]) % 16
	}
	frameTime = int(JSRound(float64(frameTime)/10) * 10)

	frames := extractFrames(doc)
	arr, err := extract2DArray(frames, c.KeyBytes)
	if err != nil {
		return "", err
	}
	if rowIndex >= len(arr) {
		return "", fmt.Errorf("row_index %d out of range (have %d rows)", rowIndex, len(arr))
	}
	frameRow := arr[rowIndex]
	targetTime := float64(frameTime) / float64(totalTime)
	return c.animate(frameRow, targetTime)
}

// animate computes the animation key string from a frame row and target time.
func (c *ClientTransaction) animate(frames []int, targetTime float64) (string, error) {
	if len(frames) < 7 {
		return "", fmt.Errorf("animate: frames too short: %d", len(frames))
	}
	fromColor := []float64{float64(frames[0]), float64(frames[1]), float64(frames[2]), 1}
	toColor := []float64{float64(frames[3]), float64(frames[4]), float64(frames[5]), 1}
	fromRotation := []float64{0.0}
	toRotation := []float64{solve(float64(frames[6]), 60.0, 360.0, true)}

	rest := frames[7:]
	curves := make([]float64, len(rest))
	for i, v := range rest {
		curves[i] = solve(float64(v), IsOdd(i), 1.0, false)
	}
	cubic := NewCubic(curves)
	val := cubic.Value(targetTime)

	color := Interpolate(fromColor, toColor, val)
	for i, v := range color {
		if v < 0 {
			color[i] = 0
		} else if v > 255 {
			color[i] = 255
		}
	}
	rotation := Interpolate(fromRotation, toRotation, val)
	matrix := RotationMatrix(rotation[0])

	var sb []string
	// First N-1 color channels → '%x' (lowercase hex of rounded int).
	// The round uses banker's rounding (round-half-to-even).
	for i := 0; i < len(color)-1; i++ {
		n := int(math.RoundToEven(color[i]))
		sb = append(sb, strconv.FormatInt(int64(n), 16))
	}
	for _, v := range matrix {
		rounded := pyRound2(v)
		if rounded < 0 {
			rounded = -rounded
		}
		hexVal := FloatToHex(rounded)
		// Leading-'.' values are prefixed with '0' and lowercased; empty becomes "0".
		switch {
		case len(hexVal) > 0 && hexVal[0] == '.':
			sb = append(sb, "0"+lowercase(hexVal))
		case hexVal == "":
			sb = append(sb, "0")
		default:
			sb = append(sb, hexVal)
		}
	}
	sb = append(sb, "0", "0")

	joined := ""
	for _, s := range sb {
		joined += s
	}
	return StripDotDashRE.ReplaceAllString(joined, ""), nil
}

func lowercase(s string) string {
	out := []byte(s)
	for i, b := range out {
		if b >= 'A' && b <= 'Z' {
			out[i] = b + 32
		}
	}
	return string(out)
}

// Generate produces an x-client-transaction-id with current time + crypto/rand mask byte.
func (c *ClientTransaction) Generate(method, path string) (string, error) {
	timeNow := (time.Now().UnixMilli() - timeEpochMs) / 1000
	var mask [1]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return c.GenerateAt(method, path, timeNow, mask[0])
}

// GenerateAt is the deterministic test seam: caller supplies time + mask byte.
func (c *ClientTransaction) GenerateAt(method, path string, timeNow int64, randomNum byte) (string, error) {
	if c.AnimationKey == "" {
		return "", errors.New("ClientTransaction not initialized (animation_key empty)")
	}

	timeNowBytes := []byte{
		byte(timeNow & 0xFF),
		byte((timeNow >> 8) & 0xFF),
		byte((timeNow >> 16) & 0xFF),
		byte((timeNow >> 24) & 0xFF),
	}

	hashInput := method + "!" + path + "!" + strconv.FormatInt(timeNow, 10) + c.RandomKeyword + c.AnimationKey
	sum := sha256.Sum256([]byte(hashInput))

	bytesArr := make([]byte, 0, len(c.KeyBytes)+4+16+1)
	bytesArr = append(bytesArr, c.KeyBytes...)
	bytesArr = append(bytesArr, timeNowBytes...)
	bytesArr = append(bytesArr, sum[:16]...)
	bytesArr = append(bytesArr, byte(c.RandomNumber))

	out := make([]byte, 1+len(bytesArr))
	out[0] = randomNum
	for i, b := range bytesArr {
		out[1+i] = b ^ randomNum
	}
	return Base64StripPad(out), nil
}

