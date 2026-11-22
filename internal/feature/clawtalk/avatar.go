//go:build !board_biscuit

package clawtalk

import (
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
	shellfeature "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"github.com/ygelfand/echolocal/internal/layout"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"sync"
	"time"
)

// AvatarPage owns only rendering. It reuses the one existing Talk controller,
// audio lane and status page, never a second transport or microphone listener.
type AvatarPage struct {
	*CaptionPage
	Renderer *avatar.Controller
	once     sync.Once
	mu       sync.Mutex
	closed   bool
}

func (p *AvatarPage) Keeps() bool { return true }
func (p *AvatarPage) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.Renderer.Stop()
}
func (p *AvatarPage) Start(v avatar.Viewport) {
	p.once.Do(func() {
		go func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.closed {
				return
			}
			p.Renderer.SetViewport(v)
			p.Renderer.Start()
		}()
	})
}
func (m *Manager) standalonePage() sharedshell.View {
	captions := m.conversationPage()
	if component.Board().Codename != "athena" {
		return captions
	}
	c := avatar.New(layout.AvatarDir, nil, func() bool { return false }, shellfeature.Get().Redraw)
	c.SetSource(func() string { return config.AvatarSection.Read().Path })
	c.Configure(avatarlab.Options(config.AvatarSection.Read()))
	c.SetOutput(speaker.Get().VoiceLevel, 24*time.Hour)
	m.mu.Lock()
	m.avatarRenderer = c
	m.mu.Unlock()
	return &AvatarPage{CaptionPage: captions, Renderer: c}
}
