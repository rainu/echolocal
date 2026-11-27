//go:build !board_biscuit

package gui

import (
	"net/http"
	"os"

	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/apps"
	"github.com/ygelfand/echolocal/internal/feature/drawer"
	"github.com/ygelfand/echolocal/internal/feature/playout"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/filepicker"
	"github.com/ygelfand/echolocal/internal/filesmedia"
	"github.com/ygelfand/echolocal/internal/hardware/display"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/echolocal/internal/ui/filesapp"
	"github.com/ygelfand/libcountertop/pkg/audio/resample"
	sharedapps "github.com/ygelfand/libcountertop/pkg/display/apps"
	shareddrawer "github.com/ygelfand/libcountertop/pkg/display/drawer"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/video"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/media/cast"
	"github.com/ygelfand/libcountertop/pkg/media/playback"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
)

type filesPlayer struct {
	*urlplayer.Player
	source *filesmedia.Transport
}

func (p *filesPlayer) Close() error { _ = p.Stop(); return p.source.Close() }

func playUserFile(f *os.File, name string) (filesapp.Player, error) {
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
	view := filesapp.New(filesapp.Options{Shell: shell.Get(), Locations: filepicker.UserLocations(filesapp.Words), Preferences: config.FilesSection, Helper: layout.AvatarRenderer, Play: playUserFile})
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
