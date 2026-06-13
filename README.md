# castle-verify-go

A Go implementation of the X.com (Twitter) web login flow, built on
[`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) with a custom
Chrome 147 TLS profile (correct ALPS, PSK resumption, stable concurrency). It
generates the request artifacts a real browser sends — `x-client-transaction-id`
and the XPFF header (AES-256-GCM) — and drives the JFAPI onboarding/login steps.

> **Disclaimer.** This project is published for research and educational purposes
> only — to document how modern anti-bot/TLS-fingerprinting flows work. Use it
> only against accounts and services you own or are authorized to test, and in
> accordance with the target's Terms of Service and applicable law. The authors
> accept no responsibility for misuse.

## Build

```bash
go build ./...
# or produce stripped binaries into ./bin
./build.bat   # Windows
```

## Configuration

The Castle token backend and the proxy are supplied from **outside** the code —
nothing sensitive is hard-coded. Copy `.env.example` to `.env` (or export the
variables) and fill in your own values.

| Setting | Flag | Environment variable | Default |
|---|---|---|---|
| Proxy | `--proxy` | `HTTP_PROXY` / `HTTPS_PROXY` | none (direct) |
| Castle API key | `--api-key` | `CASTLE_API_KEY` | none (you must supply your own) |

### Proxy formats

`--proxy` (or the `HTTP(S)_PROXY` env var) accepts any of these — the scheme
defaults to `http://` when omitted:

```
http://user:pass@host:port
https://user:pass@host:port
socks5://user:pass@host:port
host:port:user:pass
host:port
```

For rotating-session residential proxies (provider usernames containing
`-type-` / `-country-` / `-lifetime-` markers), a fresh `-session-XXX` segment is
injected per run (`castle-verify`) or per worker (`castle-verify-mt`) to force a
new exit IP. Plain `user:pass` proxies are passed through untouched.

## Run

```bash
# Single account
go run ./cmd/castle-verify --proxy http://user:pass@host:port --debug eloncr

# Multi-thread harness
go run ./cmd/castle-verify-mt --threads 200 --count 1000 --proxy host:port:user:pass

# Proxy can come from the environment instead of a flag
export HTTPS_PROXY=http://user:pass@host:port
go run ./cmd/castle-verify eloncr
```

Run `go run ./cmd/castle-verify -h` for the full flag list (country, browser,
platform, etc.).

## Tests

```bash
go test ./...
```

## Layout

```
internal/
  transaction/    # x-client-transaction-id
  crypto/         # XPFF AES-256-GCM
  config/         # browser info, headers, endpoints, proxy parsing
  httpclient/     # tls-client wrapper with the Chrome 147 profile
  profiles/       # local Chrome 147 ClientProfile
  castletoken/    # Castle token backend client
  jetfuel/        # JFAPI response parser
  flow/           # login orchestrator
cmd/
  castle-verify/      # single-account login CLI
  castle-verify-mt/   # multi-thread harness
```
