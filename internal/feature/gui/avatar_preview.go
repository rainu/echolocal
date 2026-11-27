//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
	"time"
)

// Preview owns only a local renderer, never a Talk manager or audio provider.
type avatarPreview struct {
	renderer   *avatar.Controller
	lifecycle  *clawtalk.AvatarPage
	lab        *avatarView
	options    avatar.Options
	running    bool
	fullscreen bool
}

func (v *avatarView) openPreview() {
	s := v.app.Snapshot()
	c := avatar.New(layout.AvatarDir, nil, func() bool { return false }, featureshell.Get().Redraw)
	c.SetSource(func() string { return config.AvatarSection.Read().Path })
	saved := config.AvatarSection.Read()
	c.SetBackground(func() string { return saved.Background })
	c.EnableTalkLayout(func() string { return saved.FullscreenBackground })
	c.SetForeground(func() string { return saved.Foreground }, func() string { return saved.FullscreenForeground })
	c.SetTalkMode(v.fullscreen)
	c.SetOverlay(talkVolumeBounds)
	o := s.Options
	o.Mouth = avatar.MouthClosed
	c.Configure(o)
	c.SetOutput(func() float64 { return 0 }, 5*time.Minute)
	p := &avatarPreview{renderer: c, lab: v, options: s.Options, running: s.Running && s.Notice != "stopping", fullscreen: v.fullscreen}
	p.lifecycle = &clawtalk.AvatarPage{Renderer: c}
	featureshell.Get().Push(p)
}
func (*avatarPreview) Covers() bool           { return true }
func (*avatarPreview) Timeout() time.Duration { return 5 * time.Minute }
func (p *avatarPreview) Close() {
	p.lifecycle.Finish()
	p.lab.app.Configure(p.options)
	p.lab.loaded.Store(true)
	p.lab.retiring.Store(p.renderer)
	p.lab.resume.Store(p.running)
}
func extendAvatarPreview(_ *widgets.UI, o *screens.Options, _ func() *screens.App) {
	o.Views = append(o.Views, func(view sharedshell.View) *screens.Screen {
		p, ok := view.(*avatarPreview)
		if !ok {
			return nil
		}
		return &screens.Screen{View: p, Fixed: true, Build: p.build}
	})
}
func (p *avatarPreview) build(w *gogui.Window) gogui.View {
	width, height := w.WindowSize()
	// Lab Stop is asynchronous. Never overlap native display surfaces.
	if !p.lab.app.Snapshot().Running {
		p.lifecycle.Start(avatar.Viewport{Width: width, Height: height})
	}
	if p.fullscreen {
		return fullscreenComposition(width, height, p.switchMode, func() { featureshell.Get().Pop() })
	}
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	s := p.renderer.Snapshot()
	note := text("preview_only")
	if s.ForegroundNotice != "" {
		note += " " + avatar.Text(say.Chosen(), s.ForegroundNotice)
	}
	if s.BackgroundNotice != "" {
		note += " " + text(s.BackgroundNotice)
	}
	if !s.Running && s.Notice != "ready" {
		note += " " + text(s.Notice)
	}
	left := gogui.Column(gogui.ContainerCfg{Width: float32(width / 2), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.PadAll(20), Spacing: gogui.SpacingSmall, Content: []gogui.View{
		avatarText(text("split_screen")),
		talkAvatarAction("avatar-preview-fullscreen", text("switch_fullscreen"), p.switchMode, false),
		talkAvatarAction("avatar-preview-exit", text("preview_exit"), func() { featureshell.Get().Pop() }, true),
		gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding, Scrollable: true, DragScroll: true, ScrollMode: gogui.ScrollVerticalOnly, Content: []gogui.View{talkAvatarHint(note)}}),
	}})
	return avatarComposition(width, height, left)
}

func (p *avatarPreview) switchMode() {
	p.fullscreen = !p.fullscreen
	options := avatarlab.Options(config.AvatarSection.Read().Composition(p.fullscreen))
	if p.fullscreen == p.lab.fullscreen {
		options = p.options
	}
	options.Mouth = avatar.MouthClosed
	p.renderer.Configure(options)
	p.renderer.SetTalkMode(p.fullscreen)
}
