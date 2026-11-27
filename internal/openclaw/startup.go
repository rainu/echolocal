package openclaw

import (
	"time"

	"github.com/ygelfand/libcountertop/pkg/runtime/startup"
	"github.com/ygelfand/libcountertop/pkg/say"
)

// BootWait bounds startup readiness only. Background reconnection continues.
const BootWait = 8 * time.Second

func (c *Client) Startup() startup.Progress {
	p := startupProgress(c.Status(), time.Since(c.born))
	p.Doing = say.T(p.Doing)
	return p
}
func startupProgress(s Status, elapsed time.Duration) startup.Progress {
	// Error text is a fixed whitelist: startup summaries are also logged.
	if s.State == "error" {
		messages := map[string]string{
			"storage": "openclaw.boot.storage", "identity": "openclaw.boot.identity",
			"authentication": "openclaw.boot.authentication", "tls": "openclaw.boot.tls",
			"protocol": "openclaw.boot.protocol", "settings": "openclaw.boot.settings",
		}
		if text, ok := messages[s.Reason]; ok {
			return startup.Progress{Failed: true, Doing: text}
		}
	}
	if !s.Configured {
		return startup.Progress{Done: true, Doing: "openclaw.boot.not_configured"}
	}
	if !s.Enabled {
		return startup.Progress{Done: true, Doing: "openclaw.boot.disabled"}
	}
	if s.State == "connected" {
		return startup.Progress{Done: true, Doing: "openclaw.boot.connected"}
	}
	if s.State == "pairing" {
		return startup.Progress{Failed: true, Doing: "openclaw.boot.pairing"}
	}
	if elapsed >= BootWait {
		return startup.Progress{Failed: true, Doing: "openclaw.boot.offline"}
	}
	return startup.Progress{Doing: "openclaw.boot.connecting"}
}

// Close ends the lifecycle without changing the saved reconnect preference.
func (c *Client) Close() error {
	c.op.Lock()
	defer c.op.Unlock()
	c.stopLocked()
	return nil
}

// ClockWaitText contains no clock value, endpoint or provider error.
func ClockWaitText() string {
	texts := map[string]string{
		"en": "Waiting for a valid device clock",
		"de": "Warte auf eine gültige Gerätezeit",
		"it": "In attesa di un orologio dispositivo valido",
		"vi": "Đang chờ đồng hồ thiết bị hợp lệ",
	}
	if text := texts[say.Chosen()]; text != "" {
		return text
	}
	return texts["en"]
}
