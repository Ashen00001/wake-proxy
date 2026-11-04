// Command wake-proxy sends a Wake-on-LAN packet when its link is opened,
// shows a waiting page, and redirects to the target once it is up.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/Ashen00001/wake-proxy/internal/probe"
	"github.com/Ashen00001/wake-proxy/internal/proxy"
	"github.com/Ashen00001/wake-proxy/internal/wol"
)

type config struct {
	mac, broadcast, target, listen string
	timeout, rateLimit             time.Duration
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return def
}

func parseConfig(args []string) (config, error) {
	var c config
	fs := flag.NewFlagSet("wake-proxy", flag.ContinueOnError)
	fs.StringVar(&c.mac, "mac", envOr("WAKE_MAC", ""), "MAC address of the machine to wake (env WAKE_MAC)")
	fs.StringVar(&c.broadcast, "broadcast", envOr("WAKE_BROADCAST", "255.255.255.255:9"), "broadcast address, host or host:port (env WAKE_BROADCAST)")
	fs.StringVar(&c.target, "target", envOr("WAKE_TARGET", ""), "URL to probe and redirect to (env WAKE_TARGET)")
	fs.StringVar(&c.listen, "listen", envOr("WAKE_LISTEN", ":8080"), "listen address (env WAKE_LISTEN)")
	fs.DurationVar(&c.timeout, "timeout", envDuration("WAKE_TIMEOUT", 3*time.Minute), "how long to wait for the target (env WAKE_TIMEOUT)")
	fs.DurationVar(&c.rateLimit, "rate-limit", envDuration("WAKE_RATE_LIMIT", 30*time.Second), "minimum gap between magic packets (env WAKE_RATE_LIMIT)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if c.mac == "" {
		return c, errors.New("a MAC address is required (-mac or WAKE_MAC)")
	}
	if c.target == "" {
		return c, errors.New("a target URL is required (-target or WAKE_TARGET)")
	}
	u, err := url.Parse(c.target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return c, fmt.Errorf("target %q must be an http(s) URL", c.target)
	}
	if c.timeout <= 0 || c.rateLimit < 0 {
		return c, errors.New("timeout must be positive and rate-limit non-negative")
	}
	if _, _, err := net.SplitHostPort(c.broadcast); err != nil {
		c.broadcast = net.JoinHostPort(c.broadcast, "9")
	}
	return c, nil
}

func main() {
	c, err := parseConfig(os.Args[1:])
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "wake-proxy:", err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	mac, err := wol.ParseMAC(c.mac)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wake-proxy:", err)
		os.Exit(2)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	h := proxy.New(proxy.Config{
		Target:          c.target,
		Timeout:         c.timeout,
		MinWakeInterval: c.rateLimit,
	},
		func(ctx context.Context) bool { return probe.Ready(ctx, client, c.target) },
		func() error { return wol.Send(c.broadcast, mac) },
	)

	srv := &http.Server{
		Addr:              c.listen,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("listening on %s, target %s, broadcast %s", c.listen, c.target, c.broadcast)
	log.Fatal(srv.ListenAndServe())
}
