package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	h     *Handler
	up    bool
	wakes int
	now   time.Time
}

func newFixture() *fixture {
	f := &fixture{now: time.Unix(1_700_000_000, 0)}
	f.h = New(Config{
		Target:          "http://192.0.2.10:8096/",
		Timeout:         60 * time.Second,
		MinWakeInterval: 20 * time.Second,
	}, func(context.Context) bool { return f.up }, func() error { f.wakes++; return nil })
	f.h.now = func() time.Time { return f.now }
	f.h.logf = func(string, ...any) {}
	return f
}

func (f *fixture) get(path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestAlreadyUpRedirectsWithoutWake(t *testing.T) {
	f := newFixture()
	f.up = true
	rec := f.get("/")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "http://192.0.2.10:8096/" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if f.wakes != 0 {
		t.Errorf("wakes = %d, want 0", f.wakes)
	}
}

func TestDownSendsWakeAndShowsWaitingPage(t *testing.T) {
	f := newFixture()
	rec := f.get("/")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `http-equiv="refresh"`) || !strings.Contains(body, "Waking up") {
		t.Errorf("unexpected body: %s", body)
	}
	if f.wakes != 1 {
		t.Errorf("wakes = %d, want 1", f.wakes)
	}
}

func TestRateLimitsWakePackets(t *testing.T) {
	f := newFixture()
	for i := 0; i < 10; i++ {
		f.get("/")
		f.now = f.now.Add(time.Second)
	}
	if f.wakes != 1 {
		t.Fatalf("wakes after 10s of requests = %d, want 1", f.wakes)
	}
	f.now = f.now.Add(15 * time.Second) // 25s since first send
	f.get("/")
	if f.wakes != 2 {
		t.Errorf("wakes after interval = %d, want 2", f.wakes)
	}
}

func TestRedirectsOnceTargetComesUp(t *testing.T) {
	f := newFixture()
	f.get("/")
	f.now = f.now.Add(5 * time.Second)
	f.up = true
	rec := f.get("/")
	if rec.Code != http.StatusFound {
		t.Fatalf("code = %d, want 302", rec.Code)
	}
	if f.wakes != 1 {
		t.Errorf("wakes = %d, want 1", f.wakes)
	}
}

func TestTimeoutReportsFailureThenRetry(t *testing.T) {
	f := newFixture()
	f.get("/")
	f.now = f.now.Add(61 * time.Second)
	rec := f.get("/")
	if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), "didn't come up") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if f.wakes != 1 {
		t.Errorf("failed page must not send packets, wakes = %d", f.wakes)
	}
	rec = f.get("/?retry=1")
	if rec.Code != http.StatusOK || f.wakes != 2 {
		t.Errorf("retry: code %d wakes %d", rec.Code, f.wakes)
	}
	// and the new attempt shows the waiting page again, not failure
	if rec = f.get("/"); rec.Code != http.StatusOK {
		t.Errorf("after retry code = %d", rec.Code)
	}
}

func TestWakeErrorDoesNotBreakPageAndRetriesNextRequest(t *testing.T) {
	f := newFixture()
	calls := 0
	f.h.wake = func() error { calls++; return errors.New("boom") }
	if rec := f.get("/"); rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	f.get("/")
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (failed sends are not rate limited)", calls)
	}
}

func TestOtherPathsAndMethods(t *testing.T) {
	f := newFixture()
	if rec := f.get("/healthz"); rec.Code != 200 {
		t.Errorf("healthz = %d", rec.Code)
	}
	if rec := f.get("/nope"); rec.Code != 404 {
		t.Errorf("/nope = %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d", rec.Code)
	}
	if f.wakes != 0 {
		t.Errorf("wakes = %d, want 0", f.wakes)
	}
}
