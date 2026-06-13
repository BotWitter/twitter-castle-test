package transaction

import "regexp"

const (
	DefaultKeyword         = "obfiowerehiring"
	AdditionalRandomNumber = 3
	OnDemandFileURL        = "https://abs.twimg.com/responsive-web/client-web/ondemand.s.{filename}a.js"
	timeEpochMs            = 1682924400000 // 2023-05-01T07:00:00Z
)

var (
	IndicesRE          = regexp.MustCompile(`(\(\w{1}\[(\d{1,2})\],\s*16\))+`)
	OnDemandFileRE     = regexp.MustCompile(`,(\d+):["']ondemand\.s["']`)
	OnDemandHashFormat = `,%s:"([0-9a-f]+)"`
	NonDigitRE         = regexp.MustCompile(`[^\d]+`)
	StripDotDashRE     = regexp.MustCompile(`[.-]`)
)
