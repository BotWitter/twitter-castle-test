package config

import (
	"crypto/rand"
	"regexp"
	"strings"
)

// proxyCountryRE matches Geonode-style `-country-XX` segments in user/pass.
var proxyCountryRE = regexp.MustCompile(`-country-[a-zA-Z]{2}`)

// proxySessionRE matches existing `-session-XXX` segment.
var proxySessionRE = regexp.MustCompile(`-session-[A-Za-z0-9]+`)

// proxyRotatingRE detects rotating-session residential proxies (provider-style
// usernames carrying `-type-`, `-country-` or `-lifetime-` markers). Session
// injection only applies to these; plain `user:pass` proxies are left untouched.
var proxyRotatingRE = regexp.MustCompile(`-type-|-country-|-lifetime-`)

// proxyUserPassRE captures the userinfo part: scheme://USER:PASS@host:port
var proxyUserPassRE = regexp.MustCompile(`^([a-z]+://)([^:@]+)(:[^@]*@)`)

const sessionAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// ApplyCountryToProxy rewrites any `-country-XX` substring in proxyURL to
// `-country-<lowercased country>`. No-op if no match.
func ApplyCountryToProxy(proxyURL, country string) string {
	if proxyURL == "" || country == "" {
		return proxyURL
	}
	cc := strings.ToLower(country)
	return proxyCountryRE.ReplaceAllString(proxyURL, "-country-"+cc)
}

// ApplySessionToProxy injects/replaces a `-session-<id>` segment in the
// username portion of a Geonode-style proxy URL. Used per-worker to force
// unique exit IPs across the residential pool.
//
// Example:
//
//	"http://geonode_USER-type-residential-country-tr:PASS@host:port", "ab12cd"
//	→ "http://geonode_USER-type-residential-country-tr-session-ab12cd:PASS@host:port"
func ApplySessionToProxy(proxyURL, sessionID string) string {
	if proxyURL == "" || sessionID == "" {
		return proxyURL
	}
	// If a session segment already exists, replace it.
	if proxySessionRE.MatchString(proxyURL) {
		return proxySessionRE.ReplaceAllString(proxyURL, "-session-"+sessionID)
	}
	// Only rotating-session residential proxies support this; leave plain
	// user:pass proxies untouched so their auth isn't corrupted.
	if !proxyRotatingRE.MatchString(proxyURL) {
		return proxyURL
	}
	// Otherwise append `-session-<id>` to the end of the username.
	return proxyUserPassRE.ReplaceAllString(proxyURL, "${1}${2}-session-"+sessionID+"${3}")
}

func RandomSessionID(n int) string {
	if n <= 0 {
		n = 8
	}
	raw := make([]byte, n)
	_, _ = rand.Read(raw)
	out := make([]byte, n)
	for i, b := range raw {
		out[i] = sessionAlphabet[int(b)%len(sessionAlphabet)]
	}
	return string(out)
}
