package config

const (
	BaseURL   = "https://api.x.com/1.1"
	LoginPage = "https://x.com/i/flow/login"
	HomeURL   = "https://x.com"

	// New JFAPI onboarding flow (Twitter 2026-05 rewrite).
	OnboardingLanding = "https://x.com/i/jfapi/onboarding/web/landing"
	OnboardingLogin   = "https://x.com/i/jfapi/onboarding/web?mode=login"
	BeginLoginAction  = "https://x.com/i/jfapi/onboarding/web/actions/begin_login"

	// JFAPI client version sent as x-jf-v header.
	JFAPIVersion = "JP-5"

	// Castle backend: base URL + the token path appended to it.
	CastleBaseURL   = "https://castle.botwitter.com"
	CastleTokenPath = "/generate/web"
)
