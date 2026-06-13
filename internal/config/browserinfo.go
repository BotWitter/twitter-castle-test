// Package config holds the browser identity, proxy parsing, auth, endpoints,
// and request headers used across the login flow.
package config

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
)

type BrowserInfo struct {
	BrowserType     string
	Version         int
	Patch           int
	Build           int
	Platform        string
	PlatformVersion string
	Architecture    string
	Bitness         string
	Mobile          bool
	DeviceModel     string
	Mode            string // "static" or "random"
	CustomHeaders   map[string]string
}

func (b *BrowserInfo) AcceptLanguage() string {
	return "en-US,en;q=0.9"
}

func (b *BrowserInfo) IsIOS() bool {
	return b.Platform == "iOS"
}

const (
	StaticUserAgent           = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"
	StaticSecChUA             = `"Google Chrome";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`
	StaticSecChUAArch         = `"x86"`
	StaticSecChUAMobile       = "?0"
	StaticSecChUAModel        = `""`
	StaticSecChUAPlatform     = `"Windows"`
	StaticSecChUAFullPlatform = `"19.0.0"`
)

var browserBrands = map[string]string{
	"chrome": "Google Chrome",
	"brave":  "Brave",
	"edge":   "Microsoft Edge",
}

var androidDevices = []struct{ Device, Version string }{
	{"Pixel 9", "15"}, {"Pixel 8", "14"}, {"Pixel 8 Pro", "14"},
	{"SM-S936B", "15"}, {"SM-S926B", "14"}, {"SM-S928B", "15"},
	{"SM-A556B", "14"}, {"SM-G991B", "13"},
}

var (
	iosVersions     = []string{"18_3", "18_2", "18_1", "17_7", "17_6"}
	macosVersions   = []string{"10_15_7"}
	windowsVersions = []string{"10.0.0", "15.0.0", "19.0.0"}
)

var platformBrowsers = map[string][]string{
	"Windows": {"chrome", "brave", "edge"},
	"macOS":   {"chrome", "safari", "brave"},
	"iOS":     {"safari", "chrome"},
	"Android": {"chrome", "brave"},
}

func GenerateBrowserInfo(mode, platform, browser string, version int) *BrowserInfo {
	if mode == "static" {
		v := version
		if v == 0 {
			v = 147
		}
		plat := platform
		if plat == "" {
			plat = "Windows"
		}
		bt := browser
		if bt == "" {
			if plat == "iOS" {
				bt = "safari"
			} else {
				bt = "chrome"
			}
		}
		return &BrowserInfo{
			BrowserType:     bt,
			Version:         v,
			Patch:           0,
			Build:           0,
			Platform:        plat,
			PlatformVersion: "19.0.0",
			Architecture:    "x86",
			Bitness:         "64",
			Mode:            "static",
		}
	}

	versions := []int{141, 142, 143, 144, 145, 146, 147}
	bi := &BrowserInfo{
		BrowserType: chooseStr([]string{"chrome", "brave", "edge"}),
		Version:     versions[rand.Intn(len(versions))],
		Patch:       rand.Intn(100),
		Build:       6000 + rand.Intn(1000),
		// Weighted: Windows 0.55, iOS 0.45
		Platform: weightedPlatform(),
		Mode:     "random",
	}
	if platform != "" {
		bi.Platform = platform
	}
	if browser != "" {
		bi.BrowserType = browser
	}
	if version != 0 {
		bi.Version = version
	}
	bi.fillPlatformDefaults()
	return bi
}

func weightedPlatform() string {
	if rand.Float64() < 0.55 {
		return "Windows"
	}
	return "iOS"
}

func chooseStr(s []string) string { return s[rand.Intn(len(s))] }

func (b *BrowserInfo) fillPlatformDefaults() {
	if b.Mode == "static" {
		return
	}
	valid, ok := platformBrowsers[b.Platform]
	if !ok {
		valid = []string{"chrome"}
	}
	if !contains(valid, b.BrowserType) {
		b.BrowserType = chooseStr(valid)
	}
	switch b.Platform {
	case "Windows":
		if b.Architecture == "" {
			b.Architecture = "x86"
		}
		if b.Bitness == "" {
			b.Bitness = "64"
		}
		if b.PlatformVersion == "" {
			b.PlatformVersion = chooseStr(windowsVersions)
		}
		b.Mobile = false
	case "macOS":
		if b.Architecture == "" {
			b.Architecture = chooseStr([]string{"x86", "arm"})
		}
		if b.Bitness == "" {
			b.Bitness = "64"
		}
		if b.PlatformVersion == "" {
			b.PlatformVersion = chooseStr(macosVersions)
		}
		b.Mobile = false
	case "iOS":
		if b.PlatformVersion == "" {
			b.PlatformVersion = chooseStr(iosVersions)
		}
		b.Mobile = true
		b.Architecture = ""
		b.Bitness = ""
	case "Android":
		dev := androidDevices[rand.Intn(len(androidDevices))]
		if b.DeviceModel == "" {
			b.DeviceModel = dev.Device
		}
		if b.PlatformVersion == "" {
			b.PlatformVersion = dev.Version
		}
		b.Mobile = true
		b.Architecture = ""
		b.Bitness = ""
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (b *BrowserInfo) BrandName() string {
	if name, ok := browserBrands[b.BrowserType]; ok {
		return name
	}
	return "Google Chrome"
}

func (b *BrowserInfo) SecChUAMobile() string {
	if b.Mobile {
		return "?1"
	}
	return "?0"
}

func (b *BrowserInfo) UserAgent() string {
	if b.Mode == "static" && b.Version == 147 && b.Platform == "Windows" && b.BrowserType == "chrome" {
		return StaticUserAgent
	}
	switch b.Platform {
	case "Windows":
		base := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko)"
		if b.BrowserType == "edge" {
			return fmt.Sprintf("%s Chrome/%d.0.0.0 Safari/537.36 Edg/%d.0.0.0", base, b.Version, b.Version)
		}
		return fmt.Sprintf("%s Chrome/%d.0.0.0 Safari/537.36", base, b.Version)
	case "macOS":
		if b.BrowserType == "safari" {
			safariVer := chooseStr([]string{"18.3", "17.6"})
			return fmt.Sprintf("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/%s Safari/605.1.15", safariVer)
		}
		base := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko)"
		if b.BrowserType == "edge" {
			return fmt.Sprintf("%s Chrome/%d.0.0.0 Safari/537.36 Edg/%d.0.0.0", base, b.Version, b.Version)
		}
		return fmt.Sprintf("%s Chrome/%d.0.0.0 Safari/537.36", base, b.Version)
	case "iOS":
		iosVer := b.PlatformVersion
		if b.BrowserType == "chrome" {
			return fmt.Sprintf("Mozilla/5.0 (iPhone; CPU iPhone OS %s like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/%d.0.6917.56 Mobile/15E148 Safari/604.1", iosVer, b.Version)
		}
		safariVer := strings.ReplaceAll(iosVer, "_", ".")
		return fmt.Sprintf("Mozilla/5.0 (iPhone; CPU iPhone OS %s like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/%s Mobile/15E148 Safari/604.1", iosVer, safariVer)
	case "Android":
		base := fmt.Sprintf("Mozilla/5.0 (Linux; Android %s; %s) AppleWebKit/537.36 (KHTML, like Gecko)", b.PlatformVersion, b.DeviceModel)
		if b.BrowserType == "edge" {
			return fmt.Sprintf("%s Chrome/%d.0.6917.65 Mobile Safari/537.36 EdgA/%d.0.0.0", base, b.Version, b.Version)
		}
		return fmt.Sprintf("%s Chrome/%d.0.6917.65 Mobile Safari/537.36", base, b.Version)
	}
	return fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Safari/537.36", b.Version)
}

var (
	greaseChars    = []string{" ", "(", ":", "-", ".", "/", ")", ";", "=", "?", "_"}
	greaseVersions = []string{"8", "99", "24"}
)

func (b *BrowserInfo) greaseSeed() int {
	if b.Version < 0 {
		return 0
	}
	return b.Version
}

func (b *BrowserInfo) greasedBrandVersion(versionType string) (string, string) {
	seed := b.greaseSeed()
	greasedMajor := greaseVersions[seed%len(greaseVersions)]
	brand := "Not" + greaseChars[seed%len(greaseChars)] + "A" + greaseChars[(seed+1)%len(greaseChars)] + "Brand"
	if versionType == "full" {
		return brand, greasedMajor + ".0.0.0"
	}
	return brand, greasedMajor
}

func getRandomOrder(seed, size int) []int {
	if size == 2 {
		return []int{seed % size, (seed + 1) % size}
	}
	if size == 3 {
		orders := [][]int{
			{0, 1, 2}, {0, 2, 1}, {1, 0, 2},
			{1, 2, 0}, {2, 0, 1}, {2, 1, 0},
		}
		return append([]int{}, orders[seed%len(orders)]...)
	}
	if size == 4 {
		orders := [][]int{
			{0, 1, 2, 3}, {0, 1, 3, 2}, {0, 2, 1, 3}, {0, 2, 3, 1}, {0, 3, 1, 2}, {0, 3, 2, 1},
			{1, 0, 2, 3}, {1, 0, 3, 2}, {1, 2, 0, 3}, {1, 2, 3, 0}, {1, 3, 0, 2}, {1, 3, 2, 0},
			{2, 0, 1, 3}, {2, 0, 3, 1}, {2, 1, 0, 3}, {2, 1, 3, 0}, {2, 3, 0, 1}, {2, 3, 1, 0},
			{3, 0, 1, 2}, {3, 0, 2, 1}, {3, 1, 0, 2}, {3, 1, 2, 0}, {3, 2, 0, 1}, {3, 2, 1, 0},
		}
		return append([]int{}, orders[seed%len(orders)]...)
	}
	out := make([]int, size)
	for i := range out {
		out[i] = i
	}
	return out
}

type brandTuple struct{ Name, Version string }

func (b *BrowserInfo) shuffleBrandList(brands []brandTuple) []brandTuple {
	seed := b.greaseSeed()
	order := getRandomOrder(seed, len(brands))
	out := make([]brandTuple, len(brands))
	for i, pos := range order {
		out[pos] = brands[i]
	}
	return out
}

func (b *BrowserInfo) SecChUA() string {
	if b.Mode == "static" && b.Version == 147 && b.BrowserType == "chrome" {
		return StaticSecChUA
	}
	if b.BrowserType == "safari" || b.Platform == "iOS" {
		return ""
	}
	greaseBrand, greaseVer := b.greasedBrandVersion("major")
	brands := []brandTuple{
		{greaseBrand, greaseVer},
		{"Chromium", strconv.Itoa(b.Version)},
	}
	if name := b.BrandName(); name != "" {
		brands = append(brands, brandTuple{name, strconv.Itoa(b.Version)})
	}
	brands = b.shuffleBrandList(brands)
	parts := make([]string, len(brands))
	for i, br := range brands {
		parts[i] = fmt.Sprintf(`"%s";v="%s"`, br.Name, br.Version)
	}
	return strings.Join(parts, ", ")
}

func (b *BrowserInfo) SecChUAFullVersionList() string {
	if b.Mode == "static" && b.Version == 147 && b.BrowserType == "chrome" {
		return StaticSecChUA
	}
	if b.BrowserType == "safari" || b.Platform == "iOS" {
		return ""
	}
	fullVer := fmt.Sprintf("%d.0.%d.%d", b.Version, b.Build, b.Patch)
	greaseBrand, greaseFullVer := b.greasedBrandVersion("full")
	brands := []brandTuple{
		{greaseBrand, greaseFullVer},
		{"Chromium", fullVer},
	}
	if name := b.BrandName(); name != "" {
		brands = append(brands, brandTuple{name, fullVer})
	}
	brands = b.shuffleBrandList(brands)
	parts := make([]string, len(brands))
	for i, br := range brands {
		parts[i] = fmt.Sprintf(`"%s";v="%s"`, br.Name, br.Version)
	}
	return strings.Join(parts, ", ")
}

func (b *BrowserInfo) SecChUAPlatform() string {
	return fmt.Sprintf(`"%s"`, b.Platform)
}

var (
	uaChromeVerRe  = regexp.MustCompile(`Chrome/(\d+)\.`)
	uaCriOSVerRe   = regexp.MustCompile(`CriOS/(\d+)\.`)
	uaEdgVerRe     = regexp.MustCompile(`Edg(?:A)?/(\d+)\.`)
	uaSafariVerRe  = regexp.MustCompile(`Version/(\d+)\.`)
	uaIOSVersionRe = regexp.MustCompile(`iPhone OS (\d+_\d+(?:_\d+)?)`)
	uaMacVersionRe = regexp.MustCompile(`Mac OS X (\d+[_\.]\d+(?:[_\.]\d+)?)`)
	uaAndroidVerRe = regexp.MustCompile(`Android (\d+(?:\.\d+)*)`)
	uaAndroidDevRe = regexp.MustCompile(`Android [^;]+;\s*([^)]+?)\)`)
)

// ApplyUserAgent parses a real UA string and rewrites this BrowserInfo so
// UserAgent() + SecChUA() emit values consistent with that UA. Switches Mode
// to "random" so derived getters honour the parsed fields (static mode hard-codes
// Chrome 147 Windows). Returns false if UA couldn't be parsed.
func (b *BrowserInfo) ApplyUserAgent(ua string) bool {
	ua = strings.TrimSpace(ua)
	if ua == "" {
		return false
	}
	platform := ""
	switch {
	case strings.Contains(ua, "Windows NT"):
		platform = "Windows"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		platform = "iOS"
	case strings.Contains(ua, "Android"):
		platform = "Android"
	case strings.Contains(ua, "Macintosh") || strings.Contains(ua, "Mac OS X"):
		platform = "macOS"
	default:
		return false
	}

	browser := ""
	version := 0
	switch {
	case uaEdgVerRe.MatchString(ua):
		browser = "edge"
		if m := uaEdgVerRe.FindStringSubmatch(ua); len(m) > 1 {
			version, _ = strconv.Atoi(m[1])
		}
	case strings.Contains(ua, "OPR/"):
		browser = "chrome"
	case strings.Contains(ua, "Brave"):
		browser = "brave"
	case uaCriOSVerRe.MatchString(ua):
		browser = "chrome"
		if m := uaCriOSVerRe.FindStringSubmatch(ua); len(m) > 1 {
			version, _ = strconv.Atoi(m[1])
		}
	case uaChromeVerRe.MatchString(ua):
		browser = "chrome"
		if m := uaChromeVerRe.FindStringSubmatch(ua); len(m) > 1 {
			version, _ = strconv.Atoi(m[1])
		}
	case uaSafariVerRe.MatchString(ua) && (platform == "macOS" || platform == "iOS"):
		browser = "safari"
		if m := uaSafariVerRe.FindStringSubmatch(ua); len(m) > 1 {
			version, _ = strconv.Atoi(m[1])
		}
	default:
		return false
	}

	b.Mode = "random"
	b.Platform = platform
	b.BrowserType = browser
	if version > 0 {
		b.Version = version
	}
	b.Patch = 0
	b.Build = 0
	b.PlatformVersion = ""
	b.Architecture = ""
	b.Bitness = ""
	b.Mobile = false
	b.DeviceModel = ""

	switch platform {
	case "iOS":
		if m := uaIOSVersionRe.FindStringSubmatch(ua); len(m) > 1 {
			b.PlatformVersion = m[1]
		}
	case "macOS":
		if m := uaMacVersionRe.FindStringSubmatch(ua); len(m) > 1 {
			b.PlatformVersion = strings.ReplaceAll(m[1], ".", "_")
		}
	case "Android":
		if m := uaAndroidVerRe.FindStringSubmatch(ua); len(m) > 1 {
			b.PlatformVersion = m[1]
		}
		if m := uaAndroidDevRe.FindStringSubmatch(ua); len(m) > 1 {
			b.DeviceModel = strings.TrimSpace(m[1])
		}
	case "Windows":
		b.PlatformVersion = "19.0.0"
		b.Architecture = "x86"
		b.Bitness = "64"
	}
	b.fillPlatformDefaults()
	return true
}
