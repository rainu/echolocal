//go:build !board_biscuit

package gui

import (
	"bytes"
	"context"
	"image/png"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"

	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/apps"
	"github.com/ygelfand/echolocal/internal/feature/drawer"
	"github.com/ygelfand/echolocal/internal/feature/media"
	"github.com/ygelfand/echolocal/internal/feature/playout"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/filepicker"
	"github.com/ygelfand/echolocal/internal/filesmedia"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	"github.com/ygelfand/echolocal/internal/hardware/gpu"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/echolocal/internal/ui/filesapp"
	"github.com/ygelfand/libcountertop/pkg/audio/resample"
	sharedapps "github.com/ygelfand/libcountertop/pkg/display/apps"
	shareddrawer "github.com/ygelfand/libcountertop/pkg/display/drawer"
	"github.com/ygelfand/libcountertop/pkg/display/photosview"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/video"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/media/cast"
	"github.com/ygelfand/libcountertop/pkg/media/photos"
	"github.com/ygelfand/libcountertop/pkg/media/playback"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
)

type filesPlayer struct {
	*urlplayer.Player
	source *filesmedia.Transport
}

func (p *filesPlayer) Close() error { _ = p.Stop(); return p.source.Close() }

func playUserFile(f *os.File, name string) (*filesPlayer, error) {
	transport, err := filesmedia.NewTransport(f, name)
	if err != nil {
		return nil, err
	}
	var native *urlplayer.Native
	if st, e := os.Stat(layout.LegacyDecoder); e == nil && st.Mode().IsRegular() {
		native = &urlplayer.Native{Helper: layout.LegacyDecoder}
	}
	p := urlplayer.New(urlplayer.Options{
		HTTP: &http.Client{Transport: transport}, Output: playout.New(), Screen: true, Native: native,
		Target: func() playback.Target { return video.Target(display.Get(), component.Board().MaxFPS) },
		Resample: func(from int) func([]int16) []int16 {
			return resample.NewRational(from, speaker.Rate, speaker.Channels).Run
		},
	})
	if err = p.Load(cast.Media{ContentID: filesmedia.URL, StreamType: cast.StreamBuffered, Metadata: cast.Metadata{Title: name}}, 0, true); err != nil {
		transport.Close()
		return nil, err
	}
	return &filesPlayer{p, transport}, nil
}
func extendFiles(_ *widgets.UI, o *screens.Options, _ func() *screens.App) {
	var photo atomic.Pointer[photosview.View]
	openMedia := func(ctx context.Context, f *os.File, name string) (io.Closer, error) {
		if filesmedia.Kind(name) == "image" {
			defer f.Close()
			img, err := filesmedia.ImageContext(ctx, f, 1280, 800)
			if err != nil {
				return nil, err
			}
			var data bytes.Buffer
			if err = png.Encode(&data, img); err != nil {
				return nil, err
			}
			// Reuse the actual Photos application with a private one-file source.
			// No import into the library, no persisted photo/picker preferences.
			pv := photosview.New(photosview.Options{
				Shell: shell.Get(), Photos: &filePhoto{data: data.Bytes()},
				Settings: filePhotoSettings{}, GPU: gpu.Get(), Display: display.Get(),
			})
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			photo.Store(pv)
			pv.App().Open()
			page := shell.Get().Top()
			return &mediaCloser{close: func() { photo.CompareAndSwap(pv, nil); shell.Get().Remove(page) }}, nil
		}
		if ctx.Err() != nil {
			f.Close()
			return nil, ctx.Err()
		}
		player, err := playUserFile(f, name)
		if err != nil {
			f.Close()
			return nil, err
		}
		if ctx.Err() != nil {
			player.Close()
			return nil, ctx.Err()
		}
		if filesmedia.Kind(name) == "audio" {
			shell.Get().Push(media.Page())
		}
		return player, nil
	}
	o.Views = append(o.Views, func(v sharedshell.View) *screens.Screen {
		if pv := photo.Load(); pv != nil {
			return pv.Screen(v)
		}
		return nil
	})
	view := filesapp.New(filesapp.Options{Shell: shell.Get(), Locations: filepicker.UserLocations(filesapp.Words), Preferences: config.FilesSection, Helper: layout.AvatarRenderer, Open: openMedia})
	open := func() { shell.Get().Push(view) }
	title := func() string { return filesapp.Words("Files", "Dateien") }
	apps.Get().Add(sharedapps.App{ID: "files", Name: title, Icon: "mdi:folder", Open: open})
	drawer.Get().Add(shareddrawer.Entry{Order: 33, Name: title, Icon: "mdi:folder", Open: open})
	o.Views = append(o.Views, func(v sharedshell.View) *screens.Screen {
		if v != view {
			return nil
		}
		return &screens.Screen{View: view, Fixed: true, Build: view.Build}
	})
}

// One immutable, bounded photo; the Photos app owns rendering and navigation.
type filePhoto struct{ data []byte }

func (p *filePhoto) Next() (photos.Picture, bool) {
	if p.data == nil {
		return photos.Picture{}, false
	}
	data := p.data
	p.data = nil
	return photos.Picture{ID: "local-file", Data: data}, true
}

type filePhotoSettings struct{}

func (filePhotoSettings) Read() schema.Photos {
	return schema.Photos{AppEvery: schema.DelayNever, AppFit: schema.FitContain}
}
func (filePhotoSettings) Update(func(*schema.Photos)) error { return nil }

type mediaCloser struct {
	once  sync.Once
	close func()
}

func (c *mediaCloser) Close() error { c.once.Do(c.close); return nil }
