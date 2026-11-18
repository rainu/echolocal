package openclaw

import (
	"github.com/ygelfand/libcountertop/pkg/runtime/startup"
	"time"
)

// BootWait bounds startup readiness only. Background reconnection continues.
const BootWait = 8 * time.Second

func (c *Client) Startup() startup.Progress {
	return startupProgress(c.Status(), time.Since(c.born))
}
func startupProgress(s Status, elapsed time.Duration) startup.Progress {
	// Error text is a fixed whitelist: startup summaries are also logged.
	if s.State == "error" {
		messages := map[string]string{
			"storage": "Zugangsdaten nicht lesbar", "identity": "Geräteidentität ungültig",
			"authentication": "Zugangsdaten abgewiesen", "tls": "TLS-Prüfung fehlgeschlagen",
			"protocol": "Protokoll nicht kompatibel", "settings": "Einstellungen prüfen",
		}
		if text, ok := messages[s.Reason]; ok {
			return startup.Progress{Failed: true, Doing: text}
		}
	}
	if !s.Configured {
		return startup.Progress{Done: true, Doing: "Nicht eingerichtet"}
	}
	if !s.Enabled {
		return startup.Progress{Done: true, Doing: "Deaktiviert"}
	}
	if s.State == "connected" {
		return startup.Progress{Done: true, Doing: "Verbunden"}
	}
	if s.State == "pairing" {
		return startup.Progress{Failed: true, Doing: "Freigabe im Gateway erforderlich"}
	}
	if elapsed >= BootWait {
		return startup.Progress{Failed: true, Doing: "Offline; Verbindung läuft im Hintergrund"}
	}
	return startup.Progress{Doing: "Verbindung wird aufgebaut"}
}

// Close ends the lifecycle without changing the saved reconnect preference.
func (c *Client) Close() error {
	c.op.Lock()
	defer c.op.Unlock()
	c.stopLocked()
	return nil
}
