//go:build !board_biscuit

package localvideo

import (
	"github.com/ygelfand/libcountertop/pkg/audio/resample"
	"github.com/ygelfand/libcountertop/pkg/display/video"
	shared "github.com/ygelfand/libcountertop/pkg/media/localvideo"
	"github.com/ygelfand/libcountertop/pkg/media/playback"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
	"net/http"
	"sync"

	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/playout"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
)

var handler = sync.OnceValue(func() http.Handler {
	output := playout.New()
	p := urlplayer.New(urlplayer.Options{
		Output: output, Screen: true,
		Target: func() playback.Target { return video.Target(display.Get(), component.Board().MaxFPS) },
		Resample: func(from int) func([]int16) []int16 {
			return resample.NewRational(from, speaker.Rate, speaker.Channels).Run
		},
	})
	return shared.New(p)
})

func Handler() http.Handler { return handler() }
