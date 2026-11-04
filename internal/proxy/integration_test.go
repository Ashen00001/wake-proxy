package proxy_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ashen00001/wake-proxy/internal/probe"
	"github.com/Ashen00001/wake-proxy/internal/proxy"
	"github.com/Ashen00001/wake-proxy/internal/wol"
)

var testMAC = net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}

// fakePC listens on loopback UDP and records every packet it receives.
func fakePC(t *testing.T) (addr string, packets <-chan []byte) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip("udp loopback unavailable:", err)
	}
	t.Cleanup(func() { pc.Close() })
	ch := make(chan []byte, 32)
	go func() {
		buf := make([]byte, 2048)
		for {
			n, _, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			ch <- append([]byte(nil), buf[:n]...)
		}
	}()
	return pc.LocalAddr().String(), ch
}

// fakeJellyfin answers 503 for the first downPolls requests, then 200.
func fakeJellyfin(t *testing.T, downPolls int32) (*httptest.Server, *int32) {
	t.Helper()
	var polls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&polls, 1) <= downPolls {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &polls
}

// startProxy wires the real wol, probe and proxy pieces together.
func startProxy(t *testing.T, target, udpAddr string, timeout time.Duration) *http.Client {
	t.Helper()
	probeClient := &http.Client{Timeout: 2 * time.Second}
	h := proxy.New(proxy.Config{
		Target:          target,
		Timeout:         timeout,
		MinWakeInterval: time.Minute,
		RefreshSeconds:  1,
	},
		func(ctx context.Context) bool { return probe.Ready(ctx, probeClient, target) },
		func() error { return wol.Send(udpAddr, testMAC) },
	)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: &rewriteTransport{base: srv.URL},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// rewriteTransport sends every request to the proxy under test.
type rewriteTransport struct{ base string }

func (rt *rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	u := *r.URL
	u.Scheme = "http"
	u.Host = strings.TrimPrefix(rt.base, "http://")
	r2 := r.Clone(r.Context())
	r2.URL = &u
	return http.DefaultTransport.RoundTrip(r2)
}

func get(t *testing.T, c *http.Client, path string) (*http.Response, string) {
	t.Helper()
	resp, err := c.Get("http://wake.test" + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func wantPacket(t *testing.T, packets <-chan []byte) {
	t.Helper()
	select {
	case p := <-packets:
		want, _ := wol.MagicPacket(testMAC)
		if len(p) != 102 || !bytes.Equal(p, want) {
			t.Fatalf("bad magic packet (len %d): %x", len(p), p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no magic packet arrived")
	}
}

func wantNoPacket(t *testing.T, packets <-chan []byte) {
	t.Helper()
	select {
	case p := <-packets:
		t.Fatalf("unexpected extra packet: %x", p)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestFlowWakeThenRedirect(t *testing.T) {
	udpAddr, packets := fakePC(t)
	jelly, polls := fakeJellyfin(t, 3)
	c := startProxy(t, jelly.URL, udpAddr, time.Minute)

	for i := 0; i < 3; i++ {
		resp, body := get(t, c, "/")
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, `http-equiv="refresh"`) {
			t.Fatalf("request %d: want wait page, got %d %q", i+1, resp.StatusCode, body)
		}
	}
	wantPacket(t, packets)
	wantNoPacket(t, packets) // repeated requests must not re-send within the rate limit

	resp, _ := get(t, c, "/")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != jelly.URL {
		t.Fatalf("want 302 to %s, got %d %q", jelly.URL, resp.StatusCode, resp.Header.Get("Location"))
	}
	if n := atomic.LoadInt32(polls); n != 4 {
		t.Errorf("target polled %d times, want 4", n)
	}
	wantNoPacket(t, packets)
}

func TestFlowAlreadyUp(t *testing.T) {
	udpAddr, packets := fakePC(t)
	jelly, _ := fakeJellyfin(t, 0)
	c := startProxy(t, jelly.URL, udpAddr, time.Minute)

	resp, _ := get(t, c, "/")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != jelly.URL {
		t.Fatalf("want immediate 302 to %s, got %d %q", jelly.URL, resp.StatusCode, resp.Header.Get("Location"))
	}
	wantNoPacket(t, packets)
}

func TestFlowNeverComesUp(t *testing.T) {
	udpAddr, packets := fakePC(t)
	jelly, _ := fakeJellyfin(t, 1<<30)
	c := startProxy(t, jelly.URL, udpAddr, 300*time.Millisecond)

	resp, body := get(t, c, "/")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Waking up") {
		t.Fatalf("want wait page first, got %d %q", resp.StatusCode, body)
	}
	wantPacket(t, packets)

	time.Sleep(450 * time.Millisecond)
	resp, body = get(t, c, "/")
	if resp.StatusCode != http.StatusGatewayTimeout || !strings.Contains(body, "didn't come up") {
		t.Fatalf("want failure page after timeout, got %d %q", resp.StatusCode, body)
	}
	if !strings.Contains(body, "/?retry=1") {
		t.Error("failure page should offer a retry link")
	}
	wantNoPacket(t, packets)
}
