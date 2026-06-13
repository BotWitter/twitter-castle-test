// castle-verify — single-account X.com login CLI.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/castle-verify-go/internal/cli"
	"github.com/castle-verify-go/internal/config"
	"github.com/castle-verify-go/internal/flow"
	"github.com/castle-verify-go/internal/httpclient"
)

type cliArgs struct {
	username      string
	proxy         string
	debug         bool
	apiKey        string
	version       int
	noTransaction bool
	disableutils  bool
	userAgent     string
	country       string
	browserMode   string
	browser       string
	platform      string
}

func parseArgs() cliArgs {
	a := cliArgs{}
	fs := flag.NewFlagSet("castle-verify", flag.ExitOnError)
	fs.StringVar(&a.proxy, "proxy", "", "Proxy (http://user:pass@host:port, host:port:user:pass, or host:port). Falls back to HTTP(S)_PROXY env when empty.")
	fs.BoolVar(&a.debug, "debug", false, "Enable debug logging")
	fs.StringVar(&a.apiKey, "api-key", "", "Optional Castle API key")
	fs.IntVar(&a.version, "version", 148, "Browser version")
	fs.BoolVar(&a.noTransaction, "no-transaction", false, "Disable X-Client-Transaction-Id")
	fs.BoolVar(&a.disableutils, "disableutils", false, "Drop x-client-transaction-id")
	fs.StringVar(&a.userAgent, "user-agent", "", "Override navigator.userAgent (sec-ch-ua derived from this)")
	fs.StringVar(&a.country, "country", "us", "ISO 3166-1 alpha-2 country (rewrites proxy -country-XX + Castle payload)")
	fs.StringVar(&a.browserMode, "browser-mode", "static", "static (default chrome 147 Windows) or random")
	fs.StringVar(&a.browser, "browser", "chrome", "Browser type (chrome/brave/edge/safari)")
	fs.StringVar(&a.platform, "platform", "Windows", "Platform (Windows/macOS/iOS/Android)")
	_ = fs.Parse(os.Args[1:])
	if fs.NArg() > 0 {
		a.username = fs.Arg(0)
	} else {
		a.username = "eloncr"
	}
	return a
}

func setupLogger(debug bool) zerolog.Logger {
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}
	zerolog.TimeFieldFormat = time.RFC3339
	return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"}).
		Level(level).
		With().Timestamp().Logger()
}

func resolveProxy(raw string, log zerolog.Logger) config.ProxyConfig {
	if raw == "" {
		raw = config.ProxyFromEnv().URL()
	}
	if raw == "" {
		log.Warn().Msg("No proxy set (pass --proxy or set HTTP(S)_PROXY) — connecting directly")
		return config.ProxyConfig{}
	}
	pc, err := config.ParseProxy(raw)
	if err != nil {
		log.Error().Err(err).Msg("Invalid proxy — connecting directly")
		return config.ProxyConfig{}
	}
	log.Info().Str("proxy", pc.URL()).Msg("Using proxy")
	return pc
}

func run() int {
	args := parseArgs()
	log := setupLogger(args.debug)

	pc := resolveProxy(args.proxy, log)
	enableTx := !args.noTransaction && !args.disableutils

	log.Info().Str("country", strings.ToUpper(args.country)).Msg("Starting login flow")
	log.Info().Bool("transaction_id", enableTx).Bool("disableutils", args.disableutils).Msg("Settings")

	httpC, err := httpclient.New(httpclient.Options{
		Proxy:          pc,
		Timeout:        30 * time.Second,
		BrowserMode:    args.browserMode,
		Browser:        args.browser,
		Platform:       args.platform,
		BrowserVersion: args.version,
	})
	if err != nil {
		log.Error().Err(err).Msg("HTTPClient init")
		return 1
	}
	defer httpC.Close()

	bi := httpC.BrowserInfo()
	if args.userAgent != "" {
		if ok := bi.ApplyUserAgent(args.userAgent); ok {
			log.Info().Str("ua", args.userAgent).Msg("Browser identity overridden")
		} else {
			log.Warn().Str("ua", args.userAgent).Msg("UA did not match known pattern — keeping default identity")
		}
	}
	log.Info().
		Str("platform", bi.Platform).
		Str("browser", bi.BrowserType).
		Int("version", bi.Version).
		Str("user_agent", bi.UserAgent()).
		Msg("Browser identity")

	apiKey := args.apiKey
	if apiKey == "" {
		apiKey = os.Getenv("CASTLE_API_KEY")
	}
	log.Info().Str("castle_base_url", config.CastleBaseURL).Msg("Castle backend")
	orch := flow.New(httpC, flow.Options{
		CastleAPIKey:            apiKey,
		EnableClientTransaction: enableTx,
		DisableUtils:            args.disableutils,
		Country:                 args.country,
		Logger:                  log,
	})

	ok, code := orch.Execute(args.username)
	if orch.RateLimited() {
		cli.RateLimitExit(apiKey != "")
	}
	if ok {
		log.Info().Msg("Authentication completed successfully")
		return 0
	}
	log.Error().Int("status", code).Msg("Authentication failed")
	return 1
}

func installSignalHandler() {
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		fmt.Fprintln(os.Stderr, "interrupt — exiting")
		os.Exit(130)
	}()
}

func main() {
	installSignalHandler()
	os.Exit(run())
}
