package flow

import "strings"

// networkErrorKeywords lists substrings that mark a network/transport error,
// extended with Go-runtime specific error fragments observed in production
// (HTTPS-vs-HTTP proxy responses, context cancellations, i/o timeouts).
var networkErrorKeywords = []string{
	"timeout", "request canceled", "client.timeout",
	"connection refused", "connection reset", "connection aborted",
	"proxy", "eof", "reset by peer",
	"no route to host", "ssl", "tls handshake",
	"name resolution", "failed to do request",
	"remote end closed", "max retries", "read timed out",
	// Go-runtime / net package fragments
	"context deadline", "context canceled",
	"i/o timeout", "broken pipe",
	// Proxy returning a plaintext HTTP error page on an HTTPS request
	"http response to https", "server gave http response",
	// HTTP/2 stream errors during in-flight request
	"http2: ", "stream error", "goaway",
	// DNS / dial issues
	"dial tcp", "no such host", "network is unreachable",
}

func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, k := range networkErrorKeywords {
		if strings.Contains(msg, k) {
			return true
		}
	}
	return false
}

// edgeBlockBodySignals identify edge-network blocks served as HTML or short
// strings instead of the JSON body the auth flow expects. These responses
// carry a real status code (403/503/429/...) so they don't surface as a Go
// error, but they're still infra issues — TLS fingerprint, IP reputation,
// rate-limiting at the CDN layer — and unrelated to Castle / X.com auth.
var edgeBlockBodySignals = []string{
	"cloudflare", "attention required", "sorry, you have been blocked",
	"please enable cookies", "cf-ray", "ray id:",
	"akamai reference",
	"<title>error",
	"the request could not be satisfied", // CloudFront
	"perimeterx", "px-captcha",
	"distil", "imperva incapsula",
}

// IsEdgeBlock returns true when the (status, body) pair looks like an edge /
// CDN block rather than an application-level auth failure. Treat such hits
// as infra so they don't penalise the Castle success rate.
func IsEdgeBlock(status int, body []byte) bool {
	switch status {
	case 0:
		return false
	case 502, 503, 504, 521, 522, 523, 524, 525, 526, 527, 530, 568:
		// Bad-gateway / origin-unreachable / proxy-side errors — always infra.
		return true
	}
	if status == 403 || status == 429 {
		// Forbidden / rate-limit on landing pages: confirm via body fingerprint.
		// Auth POSTs return JSON; an HTML body here is the CDN block page.
		lo := strings.ToLower(string(body))
		for _, sig := range edgeBlockBodySignals {
			if strings.Contains(lo, sig) {
				return true
			}
		}
	}
	return false
}
