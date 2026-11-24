//go:build !board_biscuit

package localvideo

import (
	esphome "github.com/ygelfand/go-esphome-device"
	"github.com/ygelfand/libcountertop/pkg/media/cast"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
	"testing"
	"time"
)

type fakePlayer struct {
	state  string
	loaded string
	calls  []string
}

func (p *fakePlayer) Load(m cast.Media, _ time.Duration, _ bool) error {
	p.loaded = m.URL()
	p.state = "playing"
	p.calls = append(p.calls, "load")
	return nil
}
func (p *fakePlayer) Play() error { p.state = "playing"; p.calls = append(p.calls, "play"); return nil }
func (p *fakePlayer) Pause() error {
	p.state = "paused"
	p.calls = append(p.calls, "pause")
	return nil
}
func (p *fakePlayer) Stop() error                  { p.state = "idle"; p.calls = append(p.calls, "stop"); return nil }
func (p *fakePlayer) Seek(time.Duration) error     { return nil }
func (p *fakePlayer) Snapshot() urlplayer.Snapshot { return urlplayer.Snapshot{State: p.state} }

type fakeVolume struct {
	level int
	muted bool
}

func (v *fakeVolume) Level() int     { return v.level }
func (v *fakeVolume) SetLevel(n int) { v.level = n }
func (v *fakeVolume) Muted() bool    { return v.muted }
func (v *fakeVolume) Mute(b bool)    { v.muted = b }
func TestVideoEntityPassesOriginalURLWithoutAudioProxy(t *testing.T) {
	p := &fakePlayer{state: "idle"}
	v := &fakeVolume{level: 23}
	r := newReceiver(p, v)
	if r.entity.ObjectID != "video" || len(r.entity.SupportedFormats) != 0 {
		t.Fatal("video entity must not request audio transcoding")
	}
	raw := "https://example.com/opaque?id=1"
	r.entity.OnCommand(esphome.MediaCommand{HasMediaURL: true, MediaURL: raw})
	r.syncState()
	if p.loaded != raw || r.entity.State() != esphome.MediaPlayerPlaying {
		t.Fatal("original URL not routed or state not published")
	}
	r.command(esphome.MediaCommand{HasCommand: true, Command: esphome.MediaPlayerPause})
	r.syncState()
	if r.entity.State() != esphome.MediaPlayerPaused {
		t.Fatal("pause missing")
	}
	r.command(esphome.MediaCommand{HasCommand: true, Command: esphome.MediaPlayerPlay})
	r.syncState()
	if r.entity.State() != esphome.MediaPlayerPlaying {
		t.Fatal("resume missing")
	}
	r.command(esphome.MediaCommand{HasCommand: true, Command: esphome.MediaPlayerStop})
	r.syncState()
	if r.entity.State() != esphome.MediaPlayerIdle {
		t.Fatal("stop missing")
	}
}
func TestVideoEntityRejectsAnnouncementAndNonHTTPURL(t *testing.T) {
	p := &fakePlayer{state: "idle"}
	r := newReceiver(p, &fakeVolume{})
	for _, c := range []esphome.MediaCommand{{HasMediaURL: true, MediaURL: "file:///private"}, {HasMediaURL: true, MediaURL: "https://example.com/a.wav", Announcement: true}} {
		r.command(c)
	}
	if len(p.calls) != 0 {
		t.Fatal("invalid request reached video player")
	}
}
func TestVideoEntityTracksWebControlsErrorsAndMediaVolume(t *testing.T) {
	p := &fakePlayer{state: "playing"}
	v := &fakeVolume{level: 23}
	r := newReceiver(p, v)
	p.state = "paused"
	r.syncState()
	if r.entity.State() != esphome.MediaPlayerPaused {
		t.Fatal("external pause missing")
	}
	p.state = "error"
	r.syncState()
	if r.entity.State() != esphome.MediaPlayerIdle {
		t.Fatal("error left entity playing")
	}
	r.command(esphome.MediaCommand{HasVolume: true, Volume: .37, HasCommand: true, Command: esphome.MediaPlayerMute})
	r.syncState()
	if v.level != 37 || r.entity.Volume() != .37 || !r.entity.Muted() {
		t.Fatal("media volume/mute missing")
	}
}
