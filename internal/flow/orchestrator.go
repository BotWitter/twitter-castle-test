// Package flow drives the new JFAPI onboarding login flow.
package flow

import (
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/rs/zerolog"

	"github.com/castle-verify-go/internal/castletoken"
	"github.com/castle-verify-go/internal/config"
	"github.com/castle-verify-go/internal/httpclient"
	"github.com/castle-verify-go/internal/jetfuel"
	"github.com/castle-verify-go/internal/transaction"
)

type Orchestrator struct {
	HTTP         *httpclient.Client
	CastleAPIKey string
	Logger       zerolog.Logger

	EnableClientTransaction bool
	DisableUtils            bool
	CastleTokenOverride     string
	CUIDOverride            string
	Country                 string
	Timezone                string

	castle *castletoken.Generator

	GuestID           string
	GuestToken        string
	SessionToken      string // from begin_login response
	NextAction        string // from begin_login response
	clientTransaction *transaction.ClientTransaction
	infraError        bool
	rateLimited       bool
}

type Options struct {
	CastleAPIKey            string
	EnableClientTransaction bool
	DisableUtils            bool
	CastleTokenOverride     string
	CUIDOverride            string
	Country                 string // ISO 3166-1 alpha-2; default "US"
	Timezone                string // IANA timezone; if empty derived from Country
	Logger                  zerolog.Logger
}

func New(http *httpclient.Client, opts Options) *Orchestrator {
	enableTx := opts.EnableClientTransaction && !opts.DisableUtils
	castle := castletoken.New(http.Jar(), opts.CastleAPIKey, opts.Country, http.BrowserInfo())
	tz := opts.Timezone
	if tz == "" {
		tz = config.CountryToTimezone(opts.Country)
	}
	return &Orchestrator{
		HTTP:                    http,
		CastleAPIKey:            opts.CastleAPIKey,
		Logger:                  opts.Logger,
		EnableClientTransaction: enableTx,
		DisableUtils:            opts.DisableUtils,
		CastleTokenOverride:     opts.CastleTokenOverride,
		CUIDOverride:            opts.CUIDOverride,
		Country:                 opts.Country,
		Timezone:                tz,
		castle:                  castle,
	}
}

func (o *Orchestrator) Castle() *castletoken.Generator { return o.castle }

func (o *Orchestrator) recordErr(err error) bool {
	if errors.Is(err, castletoken.ErrRateLimited) {
		o.rateLimited = true
	}
	if IsNetworkError(err) {
		o.infraError = true
	}
	return false
}

// RateLimited reports whether a Castle call returned HTTP 429 during this run.
func (o *Orchestrator) RateLimited() bool { return o.rateLimited }

func (o *Orchestrator) cookieValue(rawURL, name string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, c := range o.HTTP.Jar().Cookies(u) {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func (o *Orchestrator) Step1FetchLoginPage() bool {
	o.Logger.Info().Msg("Step 1: Fetching login page...")
	headers := config.NavigationHeaders(o.HTTP.BrowserInfo(), "none", "", true)
	resp, body, err := o.HTTP.Get(config.LoginPage, headers)
	if err != nil {
		o.Logger.Error().Err(err).Msg("Step 1 failed (login page)")
		return o.recordErr(err)
	}
	if resp.StatusCode >= 400 {
		if IsEdgeBlock(resp.StatusCode, body) {
			o.Logger.Error().Int("status", resp.StatusCode).Msg("Step 1 edge block (CDN/proxy)")
			o.infraError = true
			return false
		}
		o.Logger.Error().Int("status", resp.StatusCode).Msg("Step 1 login page bad status")
		return false
	}

	o.GuestID = o.cookieValue("https://x.com", "guest_id")
	if o.GuestID == "" {
		// Empty cookie jar likely means CDN/edge served a degraded page —
		// classify as infra so the retry loop rotates session and tries again.
		o.infraError = true
		o.Logger.Error().Msg("Failed to extract guest_id from cookies (infra)")
		return false
	}
	o.GuestToken = httpclient.ExtractGuestToken(string(body))
	if o.GuestToken == "" {
		// HTML missing gt token = same infra class (page incomplete/blocked).
		o.infraError = true
		o.Logger.Error().Msg("Failed to extract guest_token from HTML (infra)")
		return false
	}
	o.Logger.Info().Str("guest_id", o.GuestID).Msg("OK Guest ID")
	o.Logger.Info().Str("guest_token", truncate(o.GuestToken, 20)).Msg("OK Guest Token")

	if !o.EnableClientTransaction {
		return true
	}

	o.Logger.Info().Msg("Fetching x.com home page for transaction-id")
	homeHeaders := config.NavigationHeaders(o.HTTP.BrowserInfo(), "same-origin", "https://x.com/i/flow/login", false)
	_, homeBody, err := o.HTTP.Get(config.HomeURL, homeHeaders)
	if err != nil {
		o.Logger.Error().Err(err).Msg("home page fetch failed")
		return o.recordErr(err)
	}
	odURL, err := transaction.GetOndemandFileURL(string(homeBody))
	if err != nil {
		o.Logger.Error().Err(err).Msg("get ondemand url")
		return false
	}
	o.Logger.Info().Str("url", odURL).Msg("Fetching ondemand.s file")
	scriptHeaders := config.ScriptHeaders(o.HTTP.BrowserInfo(), "cross-site", "")
	_, odBody, err := o.HTTP.Get(odURL, scriptHeaders)
	if err != nil {
		o.Logger.Error().Err(err).Msg("ondemand fetch failed")
		return o.recordErr(err)
	}
	ct, err := transaction.New(homeBody, string(odBody))
	if err != nil {
		o.Logger.Error().Err(err).Msg("init ClientTransaction")
		return false
	}
	o.clientTransaction = ct
	o.Logger.Info().Msg("OK ClientTransaction initialized")
	return true
}

func (o *Orchestrator) Step1_5FetchCastleToken() bool {
	o.Logger.Info().Msg("Step 1.5: Fetching Castle token...")
	if o.CastleTokenOverride != "" {
		o.castle.SetOverride(o.CastleTokenOverride, o.CUIDOverride)
		o.Logger.Info().Str("token", truncate(o.CastleTokenOverride, 50)).Msg("OK Pre-generated castle token")
		if o.CUIDOverride != "" {
			o.Logger.Info().Str("cuid", o.CUIDOverride).Msg("OK Pre-generated CUID")
		}
		return true
	}
	tok, err := o.castle.GenerateToken(true, o.Country)
	if err != nil {
		o.Logger.Error().Err(err).Msg("Castle generate failed")
		return o.recordErr(err)
	}
	o.Logger.Info().Str("token", truncate(tok, 50)).Str("cuid", o.castle.CUID()).Msg("OK Castle token")
	return true
}

// Step2WarmupOnboarding warms the JFAPI onboarding state machine: GET
// /landing then GET /?mode=login; both fire before
// the begin_login POST. Response bodies (gzipped binary form schema) are
// not parsed; we only check status.
func (o *Orchestrator) Step2WarmupOnboarding() bool {
	o.Logger.Info().Msg("Step 2: Warming onboarding state (landing + login)...")
	if o.GuestToken == "" {
		o.Logger.Error().Msg("Guest token not available")
		return false
	}

	landingHeaders := config.JFAPIHeaders(
		o.HTTP.BrowserInfo(), "GET", o.GuestToken,
		o.transactionID("GET", "/i/jfapi/onboarding/web/landing"),
		o.Timezone,
		"https://x.com/i/flow/login",
	)
	resp, body, err := o.HTTP.Get(config.OnboardingLanding, landingHeaders)
	if err != nil {
		o.Logger.Error().Err(err).Msg("Step 2 landing GET")
		return o.recordErr(err)
	}
	if resp.StatusCode >= 400 {
		if IsEdgeBlock(resp.StatusCode, body) {
			o.Logger.Error().Int("status", resp.StatusCode).Msg("Step 2 landing edge block")
			o.infraError = true
			return false
		}
		o.Logger.Error().Int("status", resp.StatusCode).Bytes("body", body).Msg("Step 2 landing bad status")
		return false
	}
	o.Logger.Info().Int("bytes", len(body)).Msg("OK landing state")

	loginHeaders := config.JFAPIHeaders(
		o.HTTP.BrowserInfo(), "GET", o.GuestToken,
		o.transactionID("GET", "/i/jfapi/onboarding/web"),
		o.Timezone,
		"https://x.com/i/flow/login",
	)
	resp, body, err = o.HTTP.Get(config.OnboardingLogin, loginHeaders)
	if err != nil {
		o.Logger.Error().Err(err).Msg("Step 2 login GET")
		return o.recordErr(err)
	}
	if resp.StatusCode >= 400 {
		if IsEdgeBlock(resp.StatusCode, body) {
			o.Logger.Error().Int("status", resp.StatusCode).Msg("Step 2 login edge block")
			o.infraError = true
			return false
		}
		o.Logger.Error().Int("status", resp.StatusCode).Bytes("body", body).Msg("Step 2 login bad status")
		return false
	}
	o.Logger.Info().Int("bytes", len(body)).Msg("OK login state")
	return true
}

// Step4BeginLogin POSTs the begin_login action with the username + Castle
// token in the form body. Returns (success, statusCode). The new flow carries
// $castle_token in the body (NOT the x-castle-token header).
func (o *Orchestrator) Step4BeginLogin(username string) (bool, int) {
	o.Logger.Info().Str("username", username).Msg("Step 4: POST begin_login...")

	castleTok, err := o.castle.GetToken()
	if err != nil {
		o.Logger.Error().Err(err).Msg("Castle token")
		return o.recordErr(err), 0
	}
	if castleTok == "" {
		o.Logger.Error().Msg("Empty castle token")
		return false, 0
	}

	form := url.Values{}
	form.Set("username_or_email", username)
	form.Set("$castle_token", castleTok)
	body := []byte(form.Encode())

	headers := config.JFAPIHeaders(
		o.HTTP.BrowserInfo(), "POST", o.GuestToken,
		o.transactionID("POST", "/i/jfapi/onboarding/web/actions/begin_login"),
		o.Timezone,
		"https://x.com/i/jf/onboarding/web?mode=login",
	)

	resp, respBody, err := o.HTTP.Post(config.BeginLoginAction, headers, body)
	if err != nil {
		o.Logger.Error().Err(err).Msg("Step 4 POST")
		if IsNetworkError(err) {
			o.infraError = true
			return false, -1
		}
		return false, 0
	}

	o.Logger.Debug().Int("status", resp.StatusCode).Int("body_bytes", len(respBody)).Bytes("body", respBody).Msg("Step 4 response")

	switch resp.StatusCode {
	case 200:
		// JFAPI always returns 200; decode body to classify outcome.
		// session_token in body = Twitter accepted the request (castle
		// token passed whatever validation runs here). user_not_found is
		// orthogonal to castle and must not be reported as token failure.
		result, decErr := jetfuel.ParseResponse(respBody)
		if decErr != nil {
			o.Logger.Error().Err(decErr).Msg("Step 4 decode body")
			return false, 0
		}
		switch {
		case result.SessionToken != "":
			o.SessionToken = result.SessionToken
			o.NextAction = result.NextAction
			o.Logger.Info().
				Str("session_token", result.SessionToken).
				Str("next_action", result.NextAction).
				Msg("Step 4 OK")
			return true, 200
		case strings.HasPrefix(result.ErrorMessage, "user_not_found:"):
			o.Logger.Warn().Str("error", result.ErrorMessage).Str("next_action", result.NextAction).Msg("Step 4 user not found")
			return false, 200
		case strings.Contains(result.ErrorMessage, "temporarily limited"):
			// X detected/blocked the Castle token — explicit castle failure.
			o.Logger.Error().Str("error", result.ErrorMessage).Msg("Step 4 CASTLE DETECTED")
			return false, 200
		default:
			// Unknown 200 outcome — neither success nor known error.
			// Could be a future castle/challenge response; surface raw.
			o.Logger.Warn().Str("error", result.ErrorMessage).Str("next_action", result.NextAction).Msg("Step 4 unknown 200 outcome")
			return false, 200
		}
	default:
		if IsEdgeBlock(resp.StatusCode, respBody) {
			o.Logger.Error().Int("status", resp.StatusCode).Msg("Step 4 edge block (CDN/proxy)")
			o.infraError = true
			return false, -1
		}
		o.Logger.Warn().Int("status", resp.StatusCode).Bytes("body", respBody).Msg("Step 4 unexpected status")
		return false, resp.StatusCode
	}
}

// response_code = -1 → infra/network error (retryable).
func (o *Orchestrator) Execute(username string) (bool, int) {
	steps := []struct {
		name string
		fn   func() bool
	}{
		{"Fetch Login Page", o.Step1FetchLoginPage},
		{"Fetch Castle Token", o.Step1_5FetchCastleToken},
		{"Warmup Onboarding", o.Step2WarmupOnboarding},
	}
	for i, s := range steps {
		if i > 0 {
			d := time.Duration(100+rand.Intn(300)) * time.Millisecond
			time.Sleep(d)
		}
		if !s.fn() {
			o.Logger.Error().Str("step", s.name).Msg("Login flow failed")
			code := 0
			if o.infraError {
				code = -1
			}
			return false, code
		}
	}
	ok, code := o.Step4BeginLogin(username)
	if !ok {
		return false, code
	}
	o.Logger.Info().Msg("OK Login flow completed")
	return true, code
}

func (o *Orchestrator) transactionID(method, path string) string {
	if o.clientTransaction == nil {
		return ""
	}
	id, err := o.clientTransaction.Generate(method, path)
	if err != nil {
		o.Logger.Warn().Err(err).Msg("transaction id generate")
		return ""
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Compile-time check that fhttp is referenced (for tooling).
var _ = fhttp.MethodGet
var _ = errors.New
var _ = fmt.Errorf
