package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadyStatuses(t *testing.T) {
	cases := map[int]bool{200: true, 302: true, 404: false, 500: false, 503: false}
	for code, want := range cases {
		code := code
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if code == 302 {
				w.Header().Set("Location", "/web/")
			}
			w.WriteHeader(code)
		}))
		got := Ready(context.Background(), srv.Client(), srv.URL)
		srv.Close()
		if got != want {
			t.Errorf("status %d: Ready = %v, want %v", code, got, want)
		}
	}
}

func TestReadyUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if Ready(context.Background(), nil, url) {
		t.Error("closed server should not be ready")
	}
}

func TestWaitBecomesReady(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := Wait(context.Background(), srv.Client(), srv.URL, 10*time.Millisecond, 5*time.Second); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if h := atomic.LoadInt32(&hits); h != 3 {
		t.Errorf("hits = %d, want 3", h)
	}
}

func TestWaitTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	err := Wait(context.Background(), srv.Client(), srv.URL, 10*time.Millisecond, 100*time.Millisecond)
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
}

func TestWaitCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	err := Wait(ctx, srv.Client(), srv.URL, 10*time.Millisecond, 5*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
