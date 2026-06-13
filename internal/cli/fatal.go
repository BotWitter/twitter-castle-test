// Package cli holds small shared helpers for the command-line binaries.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"sync"
)

var rateLimitOnce sync.Once

// RateLimitExit prints a Castle 429 message tailored to whether an API key was
// supplied, waits for Enter, then exits. Safe to call from multiple goroutines.
func RateLimitExit(hasAPIKey bool) {
	rateLimitOnce.Do(func() {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "  Castle API rate limit reached (HTTP 429).")
		if hasAPIKey {
			fmt.Fprintln(os.Stderr, "  Your request rate is too high — reduce --threads (or slow down) and try again.")
		} else {
			fmt.Fprintln(os.Stderr, "  No API key set: the free tier is rate limited. Purchase a Castle API key and")
			fmt.Fprintln(os.Stderr, "  pass it via --api-key or the CASTLE_API_KEY environment variable.")
		}
		fmt.Fprint(os.Stderr, "\n  Press Enter to exit...")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		os.Exit(1)
	})
	select {}
}
