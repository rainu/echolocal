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
type avatarRenderer interface {
	Snapshot() avatar.State
	Start()
	Stop()
	SetViewport(avatar.Viewport)
}

type AvatarPage struct {
	*CaptionPage
	Renderer    avatarRenderer
	started     bool
	epoch       uint64
	mu          sync.Mutex
	closed      bool
	Fullscreen  bool
	composition config.Avatar
}

func (p *AvatarPage) Keeps() bool { return true }

// Close is a visibility notification, not conversation teardown: a temporary
// cover must release its surface but must not permanently disable this page.
func (p *AvatarPage) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.started = false
	p.epoch++
	p.Renderer.Stop()
}
func (p *AvatarPage) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.started = false
	p.epoch++
	p.Renderer.Stop()
}
func (p *AvatarPage) Start(v avatar.Viewport) {
	p.mu.Lock()
	if p.closed || p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.epoch++
	epoch := p.epoch
	p.mu.Unlock()
	go func() {
		// A previously covered native process may still be retiring. Do not overlap
		// its surface or consume the only start attempt while Stop is asynchronous.
		deadline := time.Now().Add(3 * time.Second)
		for p.Renderer.Snapshot().Running {
			p.mu.Lock()
			retired := p.closed || p.epoch != epoch
			p.mu.Unlock()
			if retired || time.Now().After(deadline) {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.closed || p.epoch != epoch {
			return
		}
		p.Renderer.SetViewport(v)
		p.Renderer.Start()
	}()
}
func (m *Manager) standalonePage() sharedshell.View {
	captions := m.conversationPage()
	if component.Board().Codename != "athena" {
		return captions
	}
	c := avatar.New(layout.AvatarDir, nil, func() bool { return false }, shellfeature.Get().Redraw)
	c.SetSource(func() string { return config.AvatarSection.Read().Path })
	c.SetBackground(func() string { return config.AvatarSection.Read().Background })
	saved := config.AvatarSection.Read()
	c.EnableTalkLayout(func() string { return saved.FullscreenBackground })
	full := saved.Mode() == "fullscreen"
	c.SetTalkMode(full)
	c.Configure(avatarlab.Options(saved.Composition(full)))
	c.SetOutput(speaker.Get().VoiceLevel, 24*time.Hour)
	m.mu.Lock()
	m.avatarRenderer = c
	m.mu.Unlock()
	return &AvatarPage{CaptionPage: captions, Renderer: c, Fullscreen: full, composition: saved}
}

// SwitchMode is presentation-only: it never calls the Talk manager or restarts
// its renderer. Saved framing is frozen for this conversation in both modes.
func (p *AvatarPage) SwitchMode() {
	p.Fullscreen = !p.Fullscreen
	if c, ok := p.Renderer.(interface {
		SetTalkMode(bool)
		Configure(avatar.Options)
	}); ok {
		c.Configure(avatarlab.Options(p.composition.Composition(p.Fullscreen)))
		c.SetTalkMode(p.Fullscreen)
	}
}
