// Package probe checks whether an HTTP target is up.
package probe

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Ready reports whether a single GET to url answers 200 or 302.
// Redirects are not followed, so a 302 counts as "up" (Jellyfin redirects
// its root to the web UI).
func Ready(ctx context.Context, client *http.Client, url string) bool {
	if client == nil {
		client = http.DefaultClient
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusFound
}

// ErrTimeout is returned by Wait when the target never became ready.
var ErrTimeout = errors.New("probe: target did not become ready in time")

// Wait polls url every interval until it is Ready, the timeout passes, or ctx ends.
func Wait(ctx context.Context, client *http.Client, url string, interval, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		pctx, pcancel := context.WithTimeout(ctx, interval+2*time.Second)
		ok := Ready(pctx, client, url)
		pcancel()
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrTimeout
			}
			return ctx.Err()
		case <-t.C:
		}
	}
}
