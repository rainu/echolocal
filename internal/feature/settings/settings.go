//go:build !board_biscuit

package settings

import (
	"slices"
	"sync"

	shareddrawer "github.com/ygelfand/libcountertop/pkg/display/drawer"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"
	"github.com/ygelfand/libcountertop/pkg/say"

	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/drawer"
	"github.com/ygelfand/echolocal/internal/feature/lock"
	"github.com/ygelfand/echolocal/internal/feature/shell"
)

func init() {
	component.Register(
		sharedcomponent.Device,
		Get,
		sharedcomponent.Order(36),
		component.Needs(board.Panel),
	)
}

type Settings struct{ SearchQuery string }

var (
	once   sync.Once
	shared *Settings
)

func Get() *Settings {
	once.Do(func() {
		shared = &Settings{}

		say.Use(config.Get().Screen.Language)

		drawer.Get().Add(shareddrawer.Entry{
			Name:  func() string { return say.T("settings.title") },
			Order: shareddrawer.OrderSettings,
			Icon:  "mdi:cog",
			Open:  func() { lock.Get().Gate().Open(Open) },
		})
	})
	return shared
}

func (s *Settings) Name() string { return "settings" }

func Open() { shell.Get().Push(root()) }

var pages = map[string]func() sharedshell.View{
	"settings": func() sharedshell.View { return root() },
	"smb":      func() sharedshell.View { return smbPage() },
	"youtube":  func() sharedshell.View { return youtubePage() },
	"features": func() sharedshell.View { return featuresPage() },
	"camera":   func() sharedshell.View { return cameraPage() },
	"openclaw": func() sharedshell.View { return openclawPage() },
}

var avatarPage func() sharedshell.View

// RegisterAvatar is called by the Athena GUI during device initialization.
func RegisterAvatar(page func() sharedshell.View) {
	avatarPage = page
	pages["avatar"] = page
	pages["vrm-test"] = page
}

func OpenPage(name string) bool {
	page, ok := pages[name]
	if ok {
		shell.Get().Push(page())
	}
	return ok
}

func Pages() []string {
	out := make([]string, 0, len(pages))
	for name := range pages {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
