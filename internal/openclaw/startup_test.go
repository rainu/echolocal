package openclaw

import (
	"strings"
	"testing"
	"time"
)

func TestStartupIsBoundedAndTruthful(t *testing.T) {
	cases := []struct {
		name         string
		status       Status
		elapsed      time.Duration
		done, failed bool
		text         string
	}{
		{"not configured", Status{}, 0, true, false, "Nicht eingerichtet"},
		{"disabled", Status{Configured: true}, 0, true, false, "Deaktiviert"},
		{"connecting", Status{Configured: true, Enabled: true, State: "connecting"}, 0, false, false, "Verbindung wird aufgebaut"},
		{"before deadline", Status{Configured: true, Enabled: true, State: "reconnecting"}, BootWait - time.Nanosecond, false, false, "Verbindung wird aufgebaut"},
		{"offline deadline", Status{Configured: true, Enabled: true, State: "reconnecting"}, BootWait, false, true, "Offline; Verbindung läuft im Hintergrund"},
		{"connected", Status{Configured: true, Enabled: true, State: "connected"}, BootWait + time.Second, true, false, "Verbunden"},
		{"pending approval", Status{Configured: true, Enabled: true, State: "pairing"}, 0, false, true, "Freigabe im Gateway erforderlich"},
		{"bad storage", Status{State: "error", Reason: "storage"}, 0, false, true, "Zugangsdaten nicht lesbar"},
		{"TLS failure", Status{Configured: true, Enabled: true, State: "error", Reason: "tls"}, 0, false, true, "TLS-Prüfung fehlgeschlagen"},
		{"auth failure", Status{Configured: true, Enabled: true, State: "error", Reason: "authentication"}, 0, false, true, "Zugangsdaten abgewiesen"},
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
