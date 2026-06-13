package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type ProxyConfig struct {
	HTTP  string
	HTTPS string
}

// ProxyFromEnv reads a proxy URL from the standard HTTP(S)_PROXY environment
// variables. Used as a fallback when no --proxy flag is supplied.
func ProxyFromEnv() ProxyConfig {
	c := ProxyConfig{
		HTTP:  firstEnv("HTTP_PROXY", "http_proxy"),
		HTTPS: firstEnv("HTTPS_PROXY", "https_proxy"),
	}
	return c
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// URL returns the preferred proxy URL (http takes precedence over https).
func (p ProxyConfig) URL() string {
	if p.HTTP != "" {
		return p.HTTP
	}
	return p.HTTPS
}

// ParseProxy normalizes a user-supplied proxy string into a ProxyConfig.
// An empty input returns an empty config and no error (proxy disabled).
//
// Accepted forms (the scheme defaults to http:// when omitted):
//
//	http://user:pass@host:port      // standard URL with auth
//	http://host:port                // standard URL, no auth
//	socks5://user:pass@host:port    // http, https, socks5 schemes supported
//	user:pass@host:port             // scheme prepended automatically
//	host:port:user:pass             // colon form (common with proxy providers)
//	host:port                       // colon form, no auth
func ParseProxy(raw string) (ProxyConfig, error) {
	normalized, err := NormalizeProxy(raw)
	if err != nil {
		return ProxyConfig{}, err
	}
	if normalized == "" {
		return ProxyConfig{}, nil
	}
	return ProxyConfig{HTTP: normalized, HTTPS: normalized}, nil
}

func NormalizeProxy(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	if strings.Contains(raw, "://") {
		return buildProxyURL(raw)
	}

	// "user:pass@host:port" without a scheme → prepend the default.
	if strings.Contains(raw, "@") {
		return buildProxyURL("http://" + raw)
	}

	// Colon forms: "host:port" or "host:port:user:pass".
	switch strings.Count(raw, ":") {
	case 1:
		hp := strings.SplitN(raw, ":", 2)
		return assembleProxyURL("http", hp[0], hp[1], "", "")
	default: // host:port:user:pass (password may itself contain ':')
		parts := strings.SplitN(raw, ":", 4)
		if len(parts) < 4 {
			return "", fmt.Errorf("invalid proxy %q: expected host:port or host:port:user:pass", raw)
		}
		return assembleProxyURL("http", parts[0], parts[1], parts[2], parts[3])
	}
}

func buildProxyURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid proxy %q: %w", raw, err)
	}
	if err := checkScheme(u.Scheme); err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid proxy %q: missing host:port", raw)
	}
	return u.String(), nil
}

func assembleProxyURL(scheme, host, port, user, pass string) (string, error) {
	if err := checkScheme(scheme); err != nil {
		return "", err
	}
	if host == "" || port == "" {
		return "", fmt.Errorf("invalid proxy: missing host or port")
	}
	u := &url.URL{Scheme: scheme, Host: host + ":" + port}
	if user != "" {
		u.User = url.UserPassword(user, pass)
	}
	return u.String(), nil
}

func checkScheme(scheme string) error {
	switch strings.ToLower(scheme) {
	case "http", "https", "socks5", "socks5h":
		return nil
	default:
		return fmt.Errorf("unsupported proxy scheme %q (use http, https or socks5)", scheme)
	}
}
