//go:build !board_biscuit

package localvideo

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"
	"github.com/ygelfand/libcountertop/pkg/audio/resample"
	"github.com/ygelfand/libcountertop/pkg/display/video"
	"github.com/ygelfand/libcountertop/pkg/media/cast"
	shared "github.com/ygelfand/libcountertop/pkg/media/localvideo"
	"github.com/ygelfand/libcountertop/pkg/media/playback"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"

	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/media"
	"github.com/ygelfand/echolocal/internal/feature/playout"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
)

func init() {
	component.Register(sharedcomponent.Device, Get, sharedcomponent.Order(61), component.Needs(board.Panel))
}

type Volume interface {
	Level() int
	SetLevel(int)
	Muted() bool
	Mute(bool)
}

type Receiver struct {
	player  shared.Player
	entity  *esphome.MediaPlayer
	volume  Volume
	handler http.Handler
}

var Get = sync.OnceValue(func() *Receiver {
	output := playout.New()
	p := urlplayer.New(urlplayer.Options{
		Output: output, Screen: true,
		Target: func() playback.Target { return video.Target(display.Get(), component.Board().MaxFPS) },
		Resample: func(from int) func([]int16) []int16 {
			return resample.NewRational(from, speaker.Rate, speaker.Channels).Run
		},
	})
	return newReceiver(p, mediaVolume{})
})

func newReceiver(p shared.Player, volume Volume) *Receiver {
	r := &Receiver{player: p, volume: volume, handler: shared.New(p)}
	r.entity = &esphome.MediaPlayer{
		Base:          esphome.Base{ObjectID: "video", Name: "Video", Icon: "mdi:video", DeviceID: component.DevicePlayback},
		SupportsPause: true,
		// No default audio format: HA must pass the original video URL, not its WAV proxy.
		Features: esphome.MediaPlayerFeaturePlayMedia | esphome.MediaPlayerFeaturePlay |
			esphome.MediaPlayerFeaturePause | esphome.MediaPlayerFeatureStop |
			esphome.MediaPlayerFeatureVolumeSet | esphome.MediaPlayerFeatureVolumeMute,
		OnCommand: r.command,
	}
	r.syncState()
	return r
}

func Handler() http.Handler                    { return Get().handler }
func (r *Receiver) Name() string               { return "video player" }
func (r *Receiver) Entities() []esphome.Entity { return []esphome.Entity{r.entity} }
func (r *Receiver) Run(ctx context.Context) error {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			r.syncState()
		}
	}
}

func (r *Receiver) syncState() {
	state := esphome.MediaPlayerIdle
	switch r.player.Snapshot().State {
	case "loading", "playing":
		state = esphome.MediaPlayerPlaying
	case "paused":
		state = esphome.MediaPlayerPaused
	}
	if r.entity.State() != state {
		r.entity.SetState(state)
	}
	level := float32(r.volume.Level()) / 100
	if r.entity.Volume() != level {
		r.entity.SetVolume(level)
	}
	muted := r.volume.Muted()
	if r.entity.Muted() != muted {
		r.entity.SetMuted(muted)
	}
}

func (r *Receiver) command(c esphome.MediaCommand) {
	if c.HasVolume {
		r.volume.SetLevel(int(math.Round(float64(c.Volume) * 100)))
	}
	var err error
	if c.HasMediaURL && c.MediaURL != "" {
		// Announcements stay on the existing speaker entity and its dedicated voice path.
		if c.Announcement || !shared.ValidURL(c.MediaURL) {
			slog.Warn("video player rejected an unsupported media request")
			return
		}
		err = r.player.Load(cast.Media{ContentID: c.MediaURL, StreamType: cast.StreamBuffered, Metadata: cast.Metadata{Title: "Video"}}, 0, true)
	}
	if c.HasCommand {
		switch c.Command {
		case esphome.MediaPlayerPlay:
			err = r.player.Play()
		case esphome.MediaPlayerPause:
			err = r.player.Pause()
		case esphome.MediaPlayerStop:
			err = r.player.Stop()
		case esphome.MediaPlayerToggle:
			if r.player.Snapshot().State == "paused" {
				err = r.player.Play()
			} else {
				err = r.player.Pause()
			}
		case esphome.MediaPlayerMute:
			r.volume.Mute(true)
		case esphome.MediaPlayerUnmute:
			r.volume.Mute(false)
		}
	}
	if err != nil {
		slog.Warn("video player could not apply a media command")
	}
	// Do not wait for decoder metadata in the native API command callback.
	// The component loop publishes asynchronous open/error and native/web controls.
}

type mediaVolume struct{}

func (mediaVolume) Level() int     { return media.Get().Levels.Level(config.StreamMedia) }
func (mediaVolume) SetLevel(v int) { media.Get().Levels.Set(config.StreamMedia, max(0, min(100, v))) }
func (mediaVolume) Muted() bool    { return media.Get().Levels.Muted(config.StreamMedia) }
func (mediaVolume) Mute(v bool)    { media.Get().Levels.Mute(config.StreamMedia, v) }
