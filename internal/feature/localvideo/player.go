//go:build !board_biscuit

package localvideo

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"
	"github.com/ygelfand/libcountertop/pkg/audio/resample"
	"github.com/ygelfand/libcountertop/pkg/display/video"
	"github.com/ygelfand/libcountertop/pkg/media/cast"
	"github.com/ygelfand/libcountertop/pkg/media/cast/protocols/webreceiver"
	shared "github.com/ygelfand/libcountertop/pkg/media/localvideo"
	"github.com/ygelfand/libcountertop/pkg/media/playback"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
	"github.com/ygelfand/libcountertop/pkg/media/youtubeurl"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"

	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/chromecast"
	"github.com/ygelfand/echolocal/internal/feature/media"
	"github.com/ygelfand/echolocal/internal/feature/playout"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/echolocal/internal/networkmedia"
	"github.com/ygelfand/libcountertop/pkg/fetch"
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
	youtube *webreceiver.YouTube
	network *networkmedia.Player
}

var Get = sync.OnceValue(func() *Receiver {
	output := playout.New()
	client := fetch.Client(0)
	transport := networkmedia.NewTransport(Source(), client.Transport)
	client.Transport = transport
	p := urlplayer.New(urlplayer.Options{
		Output: output, Screen: true, HTTP: client,
		Target: func() playback.Target { return video.Target(display.Get(), component.Board().MaxFPS) },
		Resample: func(from int) func([]int16) []int16 {
			return resample.NewRational(from, speaker.Rate, speaker.Channels).Run
		},
	})
	y := webreceiver.NewYouTube(cast.Env{
		Output: output,
		Video:  func() playback.Target { return video.Target(display.Get(), component.Board().MaxFPS) },
		Resample: func(from int) func([]int16) []int16 {
			return resample.NewRational(from, speaker.Rate, speaker.Channels).Run
		},
	}, webreceiver.Options{Engine: chromecast.Engine()})
	network := networkmedia.Wrap(shared.Route(p, y), Source(), transport)
	r := newReceiver(network, mediaVolume{})
	r.network = network
	r.youtube = y
	Source().Changed = func() { _ = r.player.Stop() }
	media.Get().SetURLHandler(r.speaker)
	return r
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

func Account() *webreceiver.YouTubeAccount { return Get().youtube.Account() }

func Handler() http.Handler                    { return Get().handler }
func (r *Receiver) Name() string               { return "video player" }
func (r *Receiver) Entities() []esphome.Entity { return []esphome.Entity{r.entity} }
func (r *Receiver) Run(ctx context.Context) error {
	if r.youtube != nil {
		go func() { _ = r.youtube.Run(ctx) }()
	}
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
	if err := r.Command(c); err != nil {
		slog.Warn("video player rejected a media request", "reason", err)
	}
}

func (r *Receiver) Command(c esphome.MediaCommand) error {
	if c.HasMediaURL && c.MediaURL != "" {
		if c.Announcement || !shared.ValidURL(c.MediaURL) {
			return errors.New("use an HTTP(S) media URL or smb://media/path; video announcements are not supported")
		}
		if _, _, err := youtubeurl.Parse(c.MediaURL); err != nil {
			return err
		}
	}

	if c.HasVolume {
		r.volume.SetLevel(int(math.Round(float64(c.Volume) * 100)))
	}
	var err error
	if c.HasMediaURL && c.MediaURL != "" {
		err = r.player.Load(cast.Media{ContentID: c.MediaURL, StreamType: cast.StreamBuffered, Metadata: cast.Metadata{Title: "Video"}}, 0, true)
	}
	if err != nil {
		return err
	}
	if c.HasCommand && !(c.HasMediaURL && c.MediaURL != "" && c.Command == esphome.MediaPlayerPlay) {
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
		case esphome.MediaPlayerVolumeUp:
			r.volume.SetLevel(min(100, r.volume.Level()+5))
		case esphome.MediaPlayerVolumeDown:
			r.volume.SetLevel(max(0, r.volume.Level()-5))
		case esphome.MediaPlayerMute:
			r.volume.Mute(true)
		case esphome.MediaPlayerUnmute:
			r.volume.Mute(false)
		}
	}

	// Do not wait for decoder metadata in the native API command callback.
	// The component loop publishes asynchronous open/error and native/web controls.
	return err
}

type mediaVolume struct{}

func (mediaVolume) Level() int     { return media.Get().Levels.Level(config.StreamMedia) }
func (mediaVolume) SetLevel(v int) { media.Get().Levels.Set(config.StreamMedia, max(0, min(100, v))) }
func (mediaVolume) Muted() bool    { return media.Get().Levels.Muted(config.StreamMedia) }
func (mediaVolume) Mute(v bool)    { media.Get().Levels.Mute(config.StreamMedia, v) }

// Speaker URL requests reuse the same YouTube player as the web/API Video action.
// Non-YouTube audio and announcements retain their original implementation.
func (r *Receiver) speaker(c esphome.MediaCommand) (bool, error) {
	if c.HasMediaURL && c.MediaURL != "" {
		if strings.HasPrefix(c.MediaURL, "smb:") {
			return true, r.Command(c)
		}
		_, recognized, err := youtubeurl.Parse(c.MediaURL)
		if recognized {
			if err != nil {
				return true, err
			}
			if c.Announcement {
				return true, errors.New("announcements with YouTube URLs are not supported")
			}
			return true, r.Command(c)
		}
		if !c.Announcement && ((r.youtube != nil && r.youtube.Active()) || (r.network != nil && r.network.Active())) {
			_ = r.player.Stop()
		}
		return false, nil
	}
	if ((r.youtube != nil && r.youtube.Active()) || (r.network != nil && r.network.Active())) && c.HasCommand {
		switch c.Command {
		case esphome.MediaPlayerPlay, esphome.MediaPlayerPause, esphome.MediaPlayerStop, esphome.MediaPlayerToggle:
			return true, r.Command(c)
		}
	}
	return false, nil
}

func (r *Receiver) Snapshot() urlplayer.Snapshot { return r.player.Snapshot() }

var source = sync.OnceValue(func() *networkmedia.Store { return networkmedia.New(layout.StateDir + "/smb-media.json") })

func Source() *networkmedia.Store { return source() }
