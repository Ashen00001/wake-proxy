// Package proxy implements the HTTP handler: wake the target, show a
// waiting page, redirect once it answers.
package proxy

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/http"
	"sync"
	"time"
)

// Config holds the handler's settings.
type Config struct {
	// Target is the URL the browser is redirected to (and which is probed).
	Target string
	// Timeout is how long to wait for the target after a wake attempt starts.
	Timeout time.Duration
	// MinWakeInterval is the minimum gap between two magic packets.
	MinWakeInterval time.Duration
	// ProbeTimeout bounds a single readiness probe.
	ProbeTimeout time.Duration
	// RefreshSeconds is the waiting page's auto-refresh interval.
	RefreshSeconds int
}

// Handler serves the wake proxy.
type Handler struct {
	cfg   Config
	probe func(ctx context.Context) bool
	wake  func() error
	now   func() time.Time
	logf  func(format string, args ...any)

	mu        sync.Mutex
	lastWake  time.Time // last time a packet was sent
	attemptAt time.Time // when the current waiting period began (zero = none)
}

// New returns a Handler. probe reports whether the target is up; wake sends a
// magic packet.
func New(cfg Config, probe func(ctx context.Context) bool, wake func() error) *Handler {
	if cfg.RefreshSeconds <= 0 {
		cfg.RefreshSeconds = 3
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = 2 * time.Second
	}
	return &Handler{cfg: cfg, probe: probe, wake: wake, now: time.Now, logf: log.Printf}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		fmt.Fprintln(w, "ok")
		return
	case "/":
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.ProbeTimeout)
	up := h.probe(ctx)
	cancel()
	if up {
		h.mu.Lock()
		h.attemptAt = time.Time{}
		h.mu.Unlock()
		http.Redirect(w, r, h.cfg.Target, http.StatusFound)
		return
	}

	retry := r.URL.Query().Get("retry") != ""
	state, elapsed := h.advance(retry)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if state == stateFailed {
		w.WriteHeader(http.StatusGatewayTimeout)
		fmt.Fprintf(w, failedPage, html.EscapeString(h.cfg.Timeout.Round(time.Second).String()))
		return
	}
	fmt.Fprintf(w, wakingPage, h.cfg.RefreshSeconds, int(elapsed.Seconds()), int(h.cfg.Timeout.Seconds()))
}

type state int

const (
	stateWaking state = iota
	stateFailed
)

// advance updates the wake state for a request that found the target down.
func (h *Handler) advance(retry bool) (state, time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()

	if !h.attemptAt.IsZero() && now.Sub(h.attemptAt) > h.cfg.Timeout {
		if !retry {
			return stateFailed, now.Sub(h.attemptAt)
		}
		h.attemptAt = time.Time{}
	}
	if h.attemptAt.IsZero() {
		h.attemptAt = now
	}
	if h.lastWake.IsZero() || now.Sub(h.lastWake) >= h.cfg.MinWakeInterval {
		if err := h.wake(); err != nil {
			h.logf("wake failed: %v", err)
		} else {
			h.logf("magic packet sent")
			h.lastWake = now
		}
	}
	return stateWaking, now.Sub(h.attemptAt)
}

const pageStyle = `<style>body{font-family:system-ui,sans-serif;background:#111;color:#eee;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;text-align:center}a{color:#8cf}</style>`

const wakingPage = `<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="%d"><title>Waking up...</title>` + pageStyle + `</head>
<body><div><h1>Waking up...</h1><p>The server is starting. This page refreshes on its own and will send you through once it's ready.</p><p>%ds of %ds</p></div></body></html>
`

const failedPage = `<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Didn't wake</title>` + pageStyle + `</head>
<body><div><h1>Server didn't come up</h1><p>No answer after %s. It may be unplugged, off the network, or still booting.</p><p><a href="/?retry=1">Try again</a></p></div></body></html>
`
