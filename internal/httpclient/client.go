// Package httpclient wraps bogdanfinn/tls-client
// with our config + fingerprint, and exposes a small Get/Post API.
//
// Uses the local Chrome_147 profile (in internal/profiles) — a real
// Chrome/147.0.0.0 (Windows) ClientHello with correct ALPS, PSK resumption,
// and stable multi-thread behavior. utls auto-folds the pre_shared_key extension when
// a session ticket is cached, matching real Chrome's mixed fresh/resumed
// fingerprint distribution.
package httpclient

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	"github.com/castle-verify-go/internal/config"
	localprofiles "github.com/castle-verify-go/internal/profiles"
)

type Client struct {
	inner       tls_client.HttpClient
	jar         tls_client.CookieJar
	browserInfo *config.BrowserInfo
	proxyURL    string
	randomize   bool
	requestN    atomic.Int64
}

type Options struct {
	Proxy            config.ProxyConfig
	Timeout          time.Duration
	RandomizePerCall bool
	BrowserMode      string // "static" or "random"
	BrowserVersion   int    // 0 = random
	Platform         string // empty = weighted random
	Browser          string // empty = chosen from platform
}

func New(opts Options) (*Client, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.BrowserMode == "" {
		opts.BrowserMode = "random"
	}

	bi := config.GenerateBrowserInfo(opts.BrowserMode, opts.Platform, opts.Browser, opts.BrowserVersion)

	jar := tls_client.NewCookieJar()
	profile := localprofiles.Chrome_147

	tlsOpts := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profile),
		tls_client.WithRandomTLSExtensionOrder(),
		tls_client.WithCookieJar(jar),
		tls_client.WithTimeoutSeconds(int(opts.Timeout.Seconds())),
		tls_client.WithNotFollowRedirects(),
	}
	proxyURL := opts.Proxy.URL()
	if proxyURL != "" {
		tlsOpts = append(tlsOpts, tls_client.WithProxyUrl(proxyURL))
	}

	inner, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), tlsOpts...)
	if err != nil {
		return nil, fmt.Errorf("tls_client init: %w", err)
	}

	return &Client{
		inner:       inner,
		jar:         jar,
		browserInfo: bi,
		proxyURL:    proxyURL,
		randomize:   opts.RandomizePerCall,
	}, nil
}

func (c *Client) BrowserInfo() *config.BrowserInfo { return c.browserInfo }
func (c *Client) ProxyURL() string                 { return c.proxyURL }
func (c *Client) Inner() tls_client.HttpClient     { return c.inner }
func (c *Client) Jar() tls_client.CookieJar        { return c.jar }

// Randomize regenerates BrowserInfo + recreates the underlying tls_client
// (cookie jar reset). Call between independent flows.
func (c *Client) Randomize(opts Options) error {
	new, err := New(opts)
	if err != nil {
		return err
	}
	c.inner = new.inner
	c.jar = new.jar
	c.browserInfo = new.browserInfo
	c.requestN.Store(0)
	return nil
}

// ApplyCustomHeaders merges custom headers — picked up by APIHeaders.
func (c *Client) ApplyCustomHeaders(h map[string]string) {
	if c.browserInfo.CustomHeaders == nil {
		c.browserInfo.CustomHeaders = map[string]string{}
	}
	for k, v := range h {
		c.browserInfo.CustomHeaders[k] = v
	}
}

// Do executes a request. Reads + closes the body, returns (resp, body).
func (c *Client) Do(req *fhttp.Request) (*fhttp.Response, []byte, error) {
	c.requestN.Add(1)
	resp, err := c.inner.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}
	return resp, body, nil
}

func (c *Client) Get(url string, headers fhttp.Header) (*fhttp.Response, []byte, error) {
	req, err := fhttp.NewRequest(fhttp.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header = cloneHeaderOrdered(headers)
	return c.Do(req)
}

func (c *Client) Post(url string, headers fhttp.Header, body []byte) (*fhttp.Response, []byte, error) {
	req, err := fhttp.NewRequest(fhttp.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header = cloneHeaderOrdered(headers)
	return c.Do(req)
}

func cloneHeaderOrdered(h fhttp.Header) fhttp.Header {
	if h == nil {
		return fhttp.Header{}
	}
	out := make(fhttp.Header, len(h))
	for k, v := range h {
		out[k] = append([]string{}, v...)
	}
	// Preserve insertion order via fhttp's HeaderOrderKey if present in source.
	if order, ok := h[fhttp.HeaderOrderKey]; ok {
		out[fhttp.HeaderOrderKey] = append([]string{}, order...)
	}
	return out
}

func (c *Client) Close() error {
	if closer, ok := c.inner.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
	return nil
}

// ExtractBetween returns the substring between two markers (used for guest_token).
func ExtractBetween(text, start, end string) string {
	si := indexOf(text, start)
	if si < 0 {
		return ""
	}
	si += len(start)
	ei := indexOfFrom(text, end, si)
	if ei < 0 {
		return ""
	}
	out := text[si:ei]
	return removeChars(out, " \n")
}

func ExtractGuestToken(html string) string {
	return ExtractBetween(html, "gt=", ";")
}

func indexOf(s, sub string) int       { return bytes.Index([]byte(s), []byte(sub)) }
func indexOfFrom(s, sub string, n int) int {
	if n >= len(s) {
		return -1
	}
	idx := bytes.Index([]byte(s[n:]), []byte(sub))
	if idx < 0 {
		return -1
	}
	return idx + n
}

func removeChars(s, chars string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		drop := false
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, s[i])
		}
	}
	return string(out)
}

var ErrNoCookie = errors.New("cookie not found")
