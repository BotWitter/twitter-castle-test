package config

import (
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
)

// webBearerToken is the public X.com web bearer token — the same value every
// web client sends, so it is safe to ship and never changes.
const webBearerToken = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"

// applyChromiumChHints adds the sec-ch-ua trio when sec_ch_ua is non-empty.
// Safari iOS legitimately sends sec-ch-ua: "" → trio dropped.
func applyChromiumChHints(h fhttp.Header, bi *BrowserInfo) {
	if ua := bi.SecChUA(); ua != "" {
		h.Set("sec-ch-ua", ua)
		h.Set("sec-ch-ua-mobile", bi.SecChUAMobile())
		h.Set("sec-ch-ua-platform", bi.SecChUAPlatform())
	}
}

// setHeaderOrder writes the iOS Safari header order so tls_client emits the
// HEADERS frame in real-Safari sequence. Lowercase keys.
func setHeaderOrder(h fhttp.Header, order []string) {
	delete(h, fhttp.HeaderOrderKey)
	for _, k := range order {
		h[fhttp.HeaderOrderKey] = append(h[fhttp.HeaderOrderKey], k)
	}
}

var iosNavigationOrder = []string{
	"sec-fetch-dest",
	"user-agent",
	"upgrade-insecure-requests",
	"accept",
	"sec-fetch-site",
	"sec-fetch-mode",
	"accept-language",
	"priority",
	"accept-encoding",
	"referer",
}

var iosScriptOrder = []string{
	"sec-fetch-dest",
	"accept-language",
	"accept-encoding",
	"sec-fetch-mode",
	"user-agent",
	"accept",
	"referer",
	"sec-fetch-site",
	"priority",
}

// Lowercase, used for both GET (state) and POST (action).
var jfapiOrder = []string{
	"x-jf-client-theme",
	"authorization",
	"sec-ch-ua-platform",
	"accept-language",
	"timezone",
	"sec-ch-ua",
	"sec-ch-ua-mobile",
	"x-twitter-active-user",
	"x-client-transaction-id",
	"x-guest-token",
	"user-agent",
	"x-jf-v",
	"content-type",
	"accept",
	"accept-encoding",
	"referer",
	"sec-fetch-dest",
	"sec-fetch-mode",
	"sec-fetch-site",
}

var iosAPIOrder = []string{
	"sec-fetch-dest",
	"accept-language",
	"accept-encoding",
	"sec-fetch-mode",
	"x-twitter-active-user",
	"x-twitter-client-language",
	"x-csrf-token",
	"x-guest-token",
	"x-client-transaction-id",
	"user-agent",
	"authorization",
	"accept",
	"content-type",
	"referer",
	"sec-fetch-site",
}

// is_first_request only changes cache-control (no-cache vs max-age=0) — Chrome only.
// Safari iOS drops cache-control + pragma + sec-fetch-user, sends a different accept.
func NavigationHeaders(bi *BrowserInfo, site string, referer string, isFirstRequest bool) fhttp.Header {
	h := fhttp.Header{}
	if bi.IsIOS() {
		h.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		h.Set("accept-language", bi.AcceptLanguage())
		h.Set("accept-encoding", "gzip, deflate, br, zstd")
		h.Set("priority", "u=0, i")
		h.Set("sec-fetch-dest", "document")
		h.Set("sec-fetch-mode", "navigate")
		h.Set("sec-fetch-site", site)
		h.Set("upgrade-insecure-requests", "1")
		h.Set("user-agent", bi.UserAgent())
		if referer != "" {
			h.Set("referer", referer)
		}
		setHeaderOrder(h, iosNavigationOrder)
		return h
	}
	h.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	h.Set("accept-language", bi.AcceptLanguage())
	h.Set("accept-encoding", "gzip, deflate, br, zstd")
	if isFirstRequest {
		h.Set("cache-control", "no-cache")
	} else {
		h.Set("cache-control", "max-age=0")
	}
	h.Set("pragma", "no-cache")
	h.Set("priority", "u=0, i")
	h.Set("sec-fetch-dest", "document")
	h.Set("sec-fetch-mode", "navigate")
	h.Set("sec-fetch-site", site)
	h.Set("sec-fetch-user", "?1")
	h.Set("upgrade-insecure-requests", "1")
	h.Set("user-agent", bi.UserAgent())
	if referer != "" {
		h.Set("referer", referer)
	}
	if bi.BrowserType == "brave" {
		h.Set("dnt", "1")
		h.Set("sec-gpc", "1")
	}
	applyChromiumChHints(h, bi)
	return h
}

func ScriptHeaders(bi *BrowserInfo, site, referer string) fhttp.Header {
	if site == "" {
		site = "cross-site"
	}
	if referer == "" {
		referer = "https://x.com/"
	}
	h := fhttp.Header{}
	h.Set("accept", "*/*")
	h.Set("accept-language", bi.AcceptLanguage())
	h.Set("accept-encoding", "gzip, deflate, br, zstd")
	h.Set("priority", "u=2")
	h.Set("referer", referer)
	h.Set("sec-fetch-dest", "script")
	h.Set("sec-fetch-mode", "no-cors")
	h.Set("sec-fetch-site", site)
	h.Set("user-agent", bi.UserAgent())
	if bi.IsIOS() {
		setHeaderOrder(h, iosScriptOrder)
		return h
	}
	if bi.BrowserType == "brave" {
		h.Set("dnt", "1")
		h.Set("sec-gpc", "1")
	}
	applyChromiumChHints(h, bi)
	return h
}

// CountryToTimezone maps an ISO 3166-1 alpha-2 country to an IANA timezone
// sent in the JFAPI `timezone` header. Falls back to "Etc/UTC".
func CountryToTimezone(country string) string {
	switch strings.ToUpper(country) {
	case "TR":
		return "Europe/Istanbul"
	case "US":
		return "America/New_York"
	case "GB", "UK":
		return "Europe/London"
	case "DE":
		return "Europe/Berlin"
	case "FR":
		return "Europe/Paris"
	case "ES":
		return "Europe/Madrid"
	case "IT":
		return "Europe/Rome"
	case "NL":
		return "Europe/Amsterdam"
	case "JP":
		return "Asia/Tokyo"
	case "BR":
		return "America/Sao_Paulo"
	case "IN":
		return "Asia/Kolkata"
	case "AU":
		return "Australia/Sydney"
	case "CA":
		return "America/Toronto"
	case "MX":
		return "America/Mexico_City"
	case "RU":
		return "Europe/Moscow"
	default:
		return "Etc/UTC"
	}
}

// JFAPIHeaders builds the header set for x.com/i/jfapi/onboarding/web/* (new
// onboarding flow). `method` is "GET" or "POST". `referer` defaults to the
// login page. `timezone` is the IANA TZ string (see CountryToTimezone).
//
// Differences vs APIHeaders:
//   - x-jf-client-theme: light
//   - x-jf-v: JP-5
//   - timezone: <IANA>
//   - accept-language: en   (NOT x-twitter-client-language)
//   - NO x-csrf-token, NO x-twitter-client-language
//   - content-type: application/x-www-form-urlencoded (POST)
func JFAPIHeaders(bi *BrowserInfo, method, guestToken, transactionID, timezone, referer string) fhttp.Header {
	if referer == "" {
		referer = "https://x.com/i/jf/onboarding/web?mode=login"
	}
	if timezone == "" {
		timezone = "Etc/UTC"
	}
	h := fhttp.Header{}
	h.Set("accept", "*/*")
	h.Set("accept-language", "en")
	h.Set("accept-encoding", "gzip, deflate, br, zstd")
	h.Set("authorization", "Bearer "+webBearerToken)
	if strings.EqualFold(method, "POST") {
		h.Set("content-type", "application/x-www-form-urlencoded")
	}
	h.Set("referer", referer)
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-site", "same-origin")
	h.Set("timezone", timezone)
	h.Set("user-agent", bi.UserAgent())
	h.Set("x-guest-token", guestToken)
	h.Set("x-jf-client-theme", "light")
	h.Set("x-jf-v", JFAPIVersion)
	h.Set("x-twitter-active-user", "yes")
	if transactionID != "" {
		h.Set("x-client-transaction-id", transactionID)
	}
	if bi.BrowserType == "brave" {
		h.Set("dnt", "1")
		h.Set("sec-gpc", "1")
	}
	applyChromiumChHints(h, bi)
	for k, v := range bi.CustomHeaders {
		h.Set(k, v)
	}
	setHeaderOrder(h, jfapiOrder)
	return h
}

func APIHeaders(bi *BrowserInfo, guestToken, transactionID string) fhttp.Header {
	h := fhttp.Header{}
	h.Set("accept", "*/*")
	h.Set("accept-language", bi.AcceptLanguage())
	h.Set("accept-encoding", "gzip, deflate, br, zstd")
	h.Set("authorization", "Bearer "+webBearerToken)
	h.Set("content-type", "application/json")
	h.Set("referer", "https://x.com/")
	h.Set("sec-fetch-dest", "empty")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-site", "same-site")
	h.Set("user-agent", bi.UserAgent())
	h.Set("x-guest-token", guestToken)
	h.Set("x-twitter-active-user", "yes")
	h.Set("x-twitter-client-language", "en")
	if transactionID != "" {
		h.Set("x-client-transaction-id", transactionID)
	}
	if bi.IsIOS() {
		for k, v := range bi.CustomHeaders {
			h.Set(k, v)
		}
		setHeaderOrder(h, iosAPIOrder)
		return h
	}
	if bi.BrowserType == "brave" {
		h.Set("dnt", "1")
		h.Set("sec-gpc", "1")
	}
	applyChromiumChHints(h, bi)
	for k, v := range bi.CustomHeaders {
		h.Set(k, v)
	}
	return h
}
