package openclaw

import (
	"strings"
	"testing"
	"time"

	"github.com/ygelfand/libcountertop/pkg/say"
)

func TestStartupIsBoundedAndTruthful(t *testing.T) {
	cases := []struct {
		name         string
		status       Status
		elapsed      time.Duration
		done, failed bool
		text         string
	}{
		{"not configured", Status{}, 0, true, false, "openclaw.boot.not_configured"},
		{"disabled", Status{Configured: true}, 0, true, false, "openclaw.boot.disabled"},
		{"connecting", Status{Configured: true, Enabled: true, State: "connecting"}, 0, false, false, "openclaw.boot.connecting"},
		{"before deadline", Status{Configured: true, Enabled: true, State: "reconnecting"}, BootWait - time.Nanosecond, false, false, "openclaw.boot.connecting"},
		{"offline deadline", Status{Configured: true, Enabled: true, State: "reconnecting"}, BootWait, false, true, "openclaw.boot.offline"},
		{"connected", Status{Configured: true, Enabled: true, State: "connected"}, BootWait + time.Second, true, false, "openclaw.boot.connected"},
		{"pending approval", Status{Configured: true, Enabled: true, State: "pairing"}, 0, false, true, "openclaw.boot.pairing"},
		{"bad storage", Status{State: "error", Reason: "storage"}, 0, false, true, "openclaw.boot.storage"},
		{"TLS failure", Status{Configured: true, Enabled: true, State: "error", Reason: "tls"}, 0, false, true, "openclaw.boot.tls"},
		{"auth failure", Status{Configured: true, Enabled: true, State: "error", Reason: "authentication"}, 0, false, true, "openclaw.boot.authentication"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			p := startupProgress(tt.status, tt.elapsed)
			if p.Done != tt.done || p.Failed != tt.failed || p.Doing != tt.text {
				t.Fatalf("unexpected startup classification: %+v", p)
			}
		})
	}
}
func TestStartupNeverLeaksUnknownErrorText(t *testing.T) {
	p := startupProgress(Status{Configured: true, Enabled: true, State: "error", Reason: "wss://private.example.test/?token=synthetic-secret"}, BootWait)
	if !p.Settled() || strings.Contains(p.Doing, "private") || strings.Contains(p.Doing, "secret") {
		t.Fatal("unsafe or unbounded startup message")
	}
}

func TestStartupUsesSelectedLanguage(t *testing.T) {
	before := say.Chosen()
	t.Cleanup(func() { say.Use(before) })
	c := &Client{born: time.Now(), state: "connected", saved: credentials{URL: "wss://gateway.example.test", Enabled: true}}
	for _, tt := range []struct{ language, text string }{{"en", "Connected"}, {"de", "Verbunden"}, {"it", "Connesso"}, {"vi", "Đã kết nối"}} {
		say.Use(tt.language)
		if got := c.Startup().Doing; got != tt.text {
			t.Errorf("%s startup: got %q, want %q", tt.language, got, tt.text)
		}
	}
}
