package main

import (
	"testing"
	"time"
)

func TestParseConfigFlags(t *testing.T) {
	c, err := parseConfig([]string{
		"-mac", "AA:BB:CC:DD:EE:FF", "-target", "http://192.0.2.10:8096/",
		"-broadcast", "192.0.2.255", "-timeout", "90s", "-rate-limit", "10s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.broadcast != "192.0.2.255:9" {
		t.Errorf("broadcast = %q, want port 9 appended", c.broadcast)
	}
	if c.timeout != 90*time.Second || c.rateLimit != 10*time.Second || c.listen != ":8080" {
		t.Errorf("unexpected config: %+v", c)
	}
}

func TestParseConfigEnv(t *testing.T) {
	t.Setenv("WAKE_MAC", "AA:BB:CC:DD:EE:FF")
	t.Setenv("WAKE_TARGET", "http://192.0.2.10:8096/")
	t.Setenv("WAKE_TIMEOUT", "120")
	t.Setenv("WAKE_LISTEN", "127.0.0.1:9000")
	c, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.timeout != 120*time.Second || c.listen != "127.0.0.1:9000" {
		t.Errorf("unexpected config: %+v", c)
	}
}

func TestParseConfigErrors(t *testing.T) {
	t.Setenv("WAKE_MAC", "")
	t.Setenv("WAKE_TARGET", "")
	for _, args := range [][]string{
		nil,
		{"-mac", "AA:BB:CC:DD:EE:FF"},
		{"-target", "http://192.0.2.10/"},
		{"-mac", "AA:BB:CC:DD:EE:FF", "-target", "ftp://192.0.2.10/"},
		{"-mac", "AA:BB:CC:DD:EE:FF", "-target", "http://192.0.2.10/", "-timeout", "0s"},
	} {
		if _, err := parseConfig(args); err == nil {
			t.Errorf("parseConfig(%v) expected error", args)
		}
	}
}
