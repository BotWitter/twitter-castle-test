// castle-verify-mt — multi-thread login harness.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/castle-verify-go/internal/cli"
	"github.com/castle-verify-go/internal/config"
	"github.com/castle-verify-go/internal/flow"
	"github.com/castle-verify-go/internal/httpclient"
)

type mtArgs struct {
	threads       int
	count         int
	username      string
	proxy         string
	debug         bool
	version       int
	noTransaction bool
	disableutils  bool
	country       string
	browserMode   string
	browser       string
	platform      string
	apiKey        string
}

func parseMTArgs() mtArgs {
	a := mtArgs{}
	fs := flag.NewFlagSet("castle-verify-mt", flag.ExitOnError)
	fs.IntVar(&a.threads, "threads", 50, "Worker count")
	fs.IntVar(&a.count, "count", 100, "Total accounts to process")
	fs.StringVar(&a.username, "username", "eloncr", "Username to use")
	fs.StringVar(&a.proxy, "proxy", "", "Proxy (http://user:pass@host:port, host:port:user:pass, or host:port). Falls back to HTTP(S)_PROXY env. Rotating residential proxies get a fresh -session-XXX per worker.")
	fs.BoolVar(&a.debug, "debug", false, "Debug logging")
	fs.IntVar(&a.version, "version", 148, "Browser version")
	fs.BoolVar(&a.noTransaction, "no-transaction", false, "Disable X-Client-Transaction-Id")
	fs.BoolVar(&a.disableutils, "disableutils", false, "Drop x-client-transaction-id")
	fs.StringVar(&a.country, "country", "us", "ISO 3166-1 alpha-2 country (proxy + Castle payload)")
	fs.StringVar(&a.browserMode, "browser-mode", "static", "static or random")
	fs.StringVar(&a.browser, "browser", "chrome", "Browser type")
	fs.StringVar(&a.platform, "platform", "Windows", "Platform")
	fs.StringVar(&a.apiKey, "api-key", "", "Castle API key (falls back to CASTLE_API_KEY env)")
	_ = fs.Parse(os.Args[1:])
	if a.apiKey == "" {
		a.apiKey = os.Getenv("CASTLE_API_KEY")
	}
	raw := a.proxy
	if raw == "" {
		raw = config.ProxyFromEnv().URL()
	}
	normalized, err := config.NormalizeProxy(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid --proxy: %v\n", err)
		os.Exit(2)
	}
	a.proxy = config.ApplyCountryToProxy(normalized, a.country)
	return a
}

type stats struct {
	Total   atomic.Int64
	Success atomic.Int64
	Failed  atomic.Int64
	Infra   atomic.Int64
}

func (s *stats) Snapshot() (total, success, failed, infra int64) {
	return s.Total.Load(), s.Success.Load(), s.Failed.Load(), s.Infra.Load()
}

type job struct {
	id int
}

func main() {
	args := parseMTArgs()
	level := zerolog.InfoLevel
	if args.debug {
		level = zerolog.DebugLevel
	}
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"}).
		Level(level).
		With().Timestamp().Logger()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		hard := make(chan os.Signal, 1)
		signal.Notify(hard, os.Interrupt, syscall.SIGTERM)
		<-hard // first signal — handled by NotifyContext
		<-hard // second signal — force exit
		fmt.Fprintln(os.Stderr, "second interrupt — forcing exit")
		os.Exit(130)
	}()

	jobs := make(chan job, args.threads*2)
	stats := &stats{}
	var wg sync.WaitGroup

	log.Info().
		Int("threads", args.threads).
		Int("count", args.count).
		Str("username", args.username).
		Bool("disableutils", args.disableutils).
		Msg("Starting multi-thread harness")

	start := time.Now()

	for i := 0; i < args.threads; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			workerLog := log.With().Int("w", workerID).Logger()
			for j := range jobs {
				if ctx.Err() != nil {
					return
				}
				stats.Total.Add(1)
				// One retry on infra failures (timeout / proxy / TLS / CDN edge block):
				// rotate to a fresh sticky-session → new residential exit IP → new ClientHello.
				var (
					ok    bool
					code  int
					infra bool
				)
				const maxAttempts = 2
				for attempt := 1; attempt <= maxAttempts; attempt++ {
					if ctx.Err() != nil {
						return
					}
					sessionID := config.RandomSessionID(8)
					rotated := config.ApplySessionToProxy(args.proxy, sessionID)
					pc := config.ProxyConfig{HTTP: rotated, HTTPS: rotated}
					ok, code, infra = runOne(args, pc, workerLog, j.id)
					if ok || !(infra || code == -1) {
						break
					}
					if attempt < maxAttempts {
						workerLog.Warn().Int("acct", j.id).Int("attempt", attempt).Int("code", code).Msg("Infra error — retrying with new session/proxy")
					}
				}
				switch {
				case ok:
					stats.Success.Add(1)
				case infra || code == -1:
					// Network/timeout/proxy issue — not a real auth failure.
					// Bucket separately so success rate isn't penalised.
					stats.Infra.Add(1)
				default:
					stats.Failed.Add(1)
				}
			}
		}(i)
	}

	go func() {
		for i := 0; i < args.count; i++ {
			select {
			case <-ctx.Done():
				close(jobs)
				return
			case jobs <- job{id: i}:
			}
		}
		close(jobs)
	}()

	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				total, ok, failed, infra := stats.Snapshot()
				eff := total - infra
				log.Info().
					Int64("total", total).Int64("success", ok).Int64("failed", failed).Int64("infra", infra).
					Float64("success_rate", pct(ok, eff)).
					Float64("rps", float64(total)/time.Since(start).Seconds()).
					Msg("Progress")
			}
		}
	}()

	wg.Wait()
	total, ok, failed, infra := stats.Snapshot()
	dur := time.Since(start)
	effective := total - infra // attempts that actually reached the auth endpoints
	fmt.Printf("\n=== Final Stats ===\n")
	fmt.Printf("Duration:    %s\n", dur.Round(time.Millisecond))
	fmt.Printf("Total:       %d\n", total)
	fmt.Printf("Effective:   %d (total - infra)\n", effective)
	fmt.Printf("Success:     %d (%.1f%% of effective, %.1f%% of total)\n",
		ok, pct(ok, effective), pct(ok, total))
	fmt.Printf("Failed:      %d\n", failed)
	fmt.Printf("Infra err:   %d (timeout / proxy / TLS / DNS)\n", infra)
	fmt.Printf("Throughput:  %.2f req/s\n", float64(total)/dur.Seconds())
}

func pct(n, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) * 100 / float64(total)
}

func runOne(args mtArgs, pc config.ProxyConfig, log zerolog.Logger, id int) (ok bool, code int, infra bool) {
	httpC, err := httpclient.New(httpclient.Options{
		Proxy:          pc,
		Timeout:        30 * time.Second,
		BrowserMode:    args.browserMode,
		Browser:        args.browser,
		Platform:       args.platform,
		BrowserVersion: args.version,
	})
	if err != nil {
		log.Error().Err(err).Msg("HTTP client init")
		return false, 0, true
	}
	defer httpC.Close()

	enableTx := !args.noTransaction && !args.disableutils
	orch := flow.New(httpC, flow.Options{
		CastleAPIKey:            args.apiKey,
		EnableClientTransaction: enableTx,
		DisableUtils:            args.disableutils,
		Country:                 args.country,
		Logger:                  log.With().Int("acct", id).Logger(),
	})
	success, c := orch.Execute(args.username)
	if orch.RateLimited() {
		cli.RateLimitExit(args.apiKey != "")
	}
	return success, c, c == -1
}
