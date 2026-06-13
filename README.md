# twitter castle test
The API solution for generating valid Castle.io(castle_token) tokens for X (Twitter) automation
https://castle.botwitter.com/

## Build

```bash
go build ./...
# or produce stripped binaries into ./bin
./build.bat   # Windows
```


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
