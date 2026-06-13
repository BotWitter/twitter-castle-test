// Package castletoken is a thin client for a self-hosted Castle token backend
// (NOT Castle.io directly). The base URL is config.CastleBaseURL.
//
// This package uses the Go stdlib net/http (NOT bogdanfinn/tls-client) for this
// internal backend endpoint.
package castletoken

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	cryptoRand "crypto/rand"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	"github.com/castle-verify-go/internal/config"
)

const cacheDuration = 60 * time.Second

var ErrRateLimited = errors.New("castle: rate limited (HTTP 429)")

type Generator struct {
	jar         tls_client.CookieJar // shared with main HTTP client; we set __cuid here
	apiKey      string
	country     string // default country sent in payload (ISO 3166-1 alpha-2, uppercase)
	browserInfo *config.BrowserInfo
	httpStdlib  *stdhttp.Client

	cachedToken    string
	cachedCUID     string
	tokenTimestamp time.Time
}

func New(jar tls_client.CookieJar, apiKey, country string, bi *config.BrowserInfo) *Generator {
	if country == "" {
		country = "US"
	}
	return &Generator{
		jar:         jar,
		apiKey:      apiKey,
		country:     strings.ToUpper(country),
		browserInfo: bi,
		httpStdlib:  &stdhttp.Client{Timeout: 15 * time.Second},
	}
}

func (g *Generator) Country() string { return g.country }

func (g *Generator) HasAPIKey() bool { return g.apiKey != "" }

func (g *Generator) CUID() string {
	return g.cachedCUID
}

func (g *Generator) SetOverride(token, cuid string) {
	g.cachedToken = token
	g.cachedCUID = cuid
	g.tokenTimestamp = time.Now()
	g.applyCUIDCookie(cuid)
}

func generateCUID() string {
	var b [16]byte
	if _, err := cryptoRand.Read(b[:]); err != nil {
		// Fall back deterministic on rare failure
		copy(b[:], []byte(time.Now().Format(time.RFC3339Nano)))
	}
	// Set version 4 + variant bits per RFC 4122
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return hex.EncodeToString(b[:])
}

func (g *Generator) applyCUIDCookie(cuid string) {
	if g.jar == nil {
		return
	}
	for _, host := range []string{"https://x.com", "https://twitter.com"} {
		u, _ := url.Parse(host)
		c := &fhttp.Cookie{
			Name:   "__cuid",
			Value:  cuid,
			Domain: "." + u.Host,
			Path:   "/",
		}
		g.jar.SetCookies(u, []*fhttp.Cookie{c})
	}
}

// GenerateToken POSTs to the Castle wrapper backend.
// If force=false and a cached token exists, returns the cached one.
// country overrides the generator default; empty = use g.country.
func (g *Generator) GenerateToken(force bool, country string) (string, error) {
	if !force && g.cachedToken != "" {
		return g.cachedToken, nil
	}
	g.cachedCUID = generateCUID()
	cuid := g.cachedCUID
	g.applyCUIDCookie(cuid)

	cc := strings.ToUpper(country)
	if cc == "" {
		cc = g.country
	}

	payload := map[string]any{
		"userAgent": g.browserInfo.UserAgent(),
		"cuid":      cuid,
		"country":   cc,
		"sec-ch-ua": g.browserInfo.SecChUA(),
	}
	currentUA := g.browserInfo.UserAgent()

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := stdhttp.NewRequest(http.MethodPost, config.CastleBaseURL+config.CastleTokenPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", currentUA)
	// Authorization comes from the user-supplied Castle API key (--api-key flag
	// or CASTLE_API_KEY env). No key → no header (backend will reject).
	if g.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}

	resp, err := g.httpStdlib.Do(req)
	if err != nil {
		return "", fmt.Errorf("castle POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", ErrRateLimited
	}

	var out struct {
		Token string `json:"token"`
		CUID  string `json:"cuid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("castle decode: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("castle returned empty token (status %d)", resp.StatusCode)
	}

	effectiveCUID := cuid
	if out.CUID != "" {
		effectiveCUID = out.CUID
	}

	g.cachedToken = out.Token
	g.cachedCUID = effectiveCUID
	g.tokenTimestamp = time.Now()
	g.applyCUIDCookie(effectiveCUID)

	return out.Token, nil
}

// GetToken returns cached token if still fresh, else regenerates with stored country.
func (g *Generator) GetToken() (string, error) {
	if g.cachedToken != "" && time.Since(g.tokenTimestamp) <= cacheDuration {
		return g.cachedToken, nil
	}
	return g.GenerateToken(false, "")
}
