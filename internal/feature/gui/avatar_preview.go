//go:build !board_biscuit

package gui

import (
	"fmt"
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
	"image"
	"math"
	"sync/atomic"
	"time"
)

// Preview owns only a local renderer, never a Talk manager or audio provider.
type avatarPreview struct {
	renderer               *avatar.Controller
	lifecycle              *clawtalk.AvatarPage
	lab                    *avatarView
	options                avatar.Options
	running                bool
	fullscreen             bool
	editor                 *avatarView
	controls               bool
	note                   string
	bounds                 atomic.Pointer[image.Rectangle]
	hiddenControlsContains func(float64, float64) bool
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

	o := s.Options
	o.Mouth = avatar.MouthClosed
	c.Configure(o)
	c.SetOutput(func() float64 { return 0 }, 5*time.Minute)
	p := &avatarPreview{renderer: c, lab: v, options: s.Options, running: s.Running && s.Notice != "stopping", fullscreen: v.fullscreen}
	p.controls = true
	p.initEditor()
	c.SetOverlay(func() image.Rectangle {
		if r := p.bounds.Load(); r != nil {
			return *r
		}
		return image.Rectangle{}
	})
	p.lifecycle = &clawtalk.AvatarPage{Renderer: c}
	featureshell.Get().Push(p)
}
func (*avatarPreview) Covers() bool           { return true }
func (*avatarPreview) Timeout() time.Duration { return 5 * time.Minute }
func (p *avatarPreview) Close() {
	p.lifecycle.Finish()
	if p.editor != nil {
		p.editor.dragEpoch.Add(1)
		p.editor.touchReset.Add(1)
		if p.editor.touchDetach != nil {
			p.editor.touchDetach()
		}
	}
	p.lab.fullscreen = p.fullscreen
	p.lab.app.Configure(p.options)
	p.lab.keepDraft()
	p.lab.note = p.note
	if p.lab.note == "" && !samePlacement(p.options, avatarlab.Options(config.AvatarSection.Read().Composition(p.fullscreen))) {
		p.lab.note = "placement_pending"
	}
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
	if p.editor == nil {
		p.initEditor()
	}
	p.editor.attachAvatarTouch(w)
	r := image.Rectangle{}
	if p.fullscreen && p.controls {
		r = image.Rect(0, height-previewEditorHeight, width, height)
	}
	p.bounds.Store(&r)
	// Lab Stop is asynchronous. Never overlap native display surfaces.
	if !p.lab.app.Snapshot().Running {
		p.lifecycle.Start(avatar.Viewport{Width: width, Height: height})
	}
	if p.fullscreen {
		return p.fullscreenEditor(width, height)
	}
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	s := p.renderer.Snapshot()
	note := text("placement_help")
	if p.note != "" {
		note += " " + text(p.note)
	}
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
		talkAvatarAction("avatar-preview-save", text("placement_save"), p.save, false),
		gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding, Scrollable: true, DragScroll: true, ScrollMode: gogui.ScrollVerticalOnly, Content: []gogui.View{talkAvatarHint(note)}}),
	}})
	return p.touchCanvas(width, height, avatarComposition(width, height, left))
}

func (p *avatarPreview) switchMode() {
	p.lab.drafts[p.fullscreen] = p.options
	p.fullscreen = !p.fullscreen
	p.options = p.lab.draft(p.fullscreen)
	p.Configure(p.options)
	p.editor.dragEpoch.Add(1)
	p.editor.touchReset.Add(1)
	p.note = ""
	p.renderer.SetTalkMode(p.fullscreen)
}

// The editor adapts the existing Lab gesture path. Its renderer remains silent,
// and persisted composition settings are identical to those read by Talk.
func (p *avatarPreview) initEditor() {
	if p.lab.drafts == nil {
		p.lab.drafts = map[bool]avatar.Options{}
	}
	p.editor = &avatarView{app: p, gestureView: p}
	p.editor.gestureViewport = func(w, h int) avatar.Viewport {
		if p.fullscreen {
			return avatar.Viewport{Size: min(w, h), Width: w, Height: h}
		}
		vp := avatar.ConversationViewport(w, h)
		return avatar.Viewport{X: vp.X, Y: vp.Y, Size: min(vp.Width, vp.Height), Width: vp.Width, Height: vp.Height}
	}
	p.editor.gestureContains = func(x, y float64) bool {
		r := p.bounds.Load()
		if p.fullscreen {
			if p.controls && r != nil && y >= float64(r.Min.Y) {
				return false
			}
			// Exclude native Talk control apertures even when editor chrome is hidden.
			if !p.controls && r != nil {
				// Window dimensions are retained separately by the editor viewport.
				return p.hiddenControlsContains == nil || !p.hiddenControlsContains(x, y)
			}
		}
		return true
	}
}
func (p *avatarPreview) Snapshot() avatar.State {
	s := p.renderer.Snapshot()
	s.Options = p.options
	return s
}
func (p *avatarPreview) Configure(o avatar.Options) {
	p.options = o
	p.lab.drafts[p.fullscreen] = o
	p.note = ""
	o.Mouth = avatar.MouthClosed
	p.renderer.Configure(o)
}

// Gesture-controller adapter: AvatarPage alone owns renderer lifecycle.
func (p *avatarPreview) Start()                      {}
func (p *avatarPreview) Stop()                       {}
func (p *avatarPreview) SetViewport(avatar.Viewport) {}
func (p *avatarPreview) save() {
	if err := avatarlab.SaveComposition(p.options, p.fullscreen); err != nil {
		p.note = "save_failed"
	} else {
		p.note = "saved"
	}
}
func (p *avatarPreview) reset() {
	o := p.options
	o.Zoom, o.Yaw, o.Pitch, o.PanX, o.PanY = 1, 0, 0, 0, 0
	p.Configure(o)
	p.editor.dragEpoch.Add(1)
	p.editor.touchReset.Add(1)
}

const previewEditorHeight = 244

func (p *avatarPreview) fullscreenEditor(width, height int) gogui.View {
	p.hiddenControlsContains = func(x, y float64) bool {
		return y >= float64(height-98) && x >= float64(width/2-90) && x <= float64(width/2+90)
	}
	if !p.controls {
		base := p.hiddenEditor(width, height)
		return p.touchCanvas(width, height, base)
	}
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	o := p.options
	status := text("placement_pending")
	saved := avatarlab.Options(config.AvatarSection.Read().Composition(p.fullscreen))
	if samePlacement(o, saved) {
		status = text("placement_saved")
	}
	if p.note != "" {
		status = text(p.note)
	}
	change := func(update func(*avatar.Options)) func() {
		return func() {
			o := p.options
			update(&o)
			p.Configure(o)
			p.editor.touchReset.Add(1)
			p.editor.dragEpoch.Add(1)
		}
	}
	buttons := func(id, label string, do func()) gogui.View {
		return avatarButton("avatar-preview-"+id, label, false, false, do)
	}
	bar := gogui.Column(gogui.ContainerCfg{ID: "avatar-preview-editor", Height: previewEditorHeight, Sizing: gogui.FillFixed, Padding: gogui.PadAll(8), Spacing: gogui.SpacingSmall, Content: []gogui.View{
		avatarRow(buttons("save", text("placement_save"), p.save), buttons("reset", text("placement_reset"), p.reset), buttons("split", text("split_screen"), p.switchMode), buttons("exit", text("placement_back"), func() { featureshell.Get().Pop() })),
		avatarRow(buttons("zoom-out", "−", change(func(o *avatar.Options) { o.Zoom = math.Max(avatar.MinZoom, avatar.EffectiveZoom(o.Zoom)/1.1) })), buttons("zoom-in", "+", change(func(o *avatar.Options) { o.Zoom = math.Min(avatar.MaxZoom, avatar.EffectiveZoom(o.Zoom)*1.1) })), buttons("left", "← 15°", change(func(o *avatar.Options) { o.Yaw -= 15 })), buttons("right", "15° →", change(func(o *avatar.Options) { o.Yaw += 15 })), buttons("hide", text("placement_hide"), func() { p.controls = false; p.editor.touchReset.Add(1) })),
		talkAvatarHint(fmt.Sprintf("%s · X %+.0f%% Y %+.0f%% · %.0f%% · %.0f° / %.0f°", status, o.PanX*100, o.PanY*100, avatar.EffectiveZoom(o.Zoom)*100, o.Yaw, o.Pitch)),
		previewEditorHint("avatar-preview-help", text("placement_help")),
	}})
	return p.touchCanvas(width, height, gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Content: []gogui.View{
		gogui.Column(gogui.ContainerCfg{Height: float32(height - previewEditorHeight), Sizing: gogui.FillFixed, Padding: gogui.NoPadding, SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius}), bar,
	}}))
}
func samePlacement(a, b avatar.Options) bool {
	return math.Abs(avatar.EffectiveZoom(a.Zoom)-avatar.EffectiveZoom(b.Zoom)) < .005 && math.Abs(a.PanX-b.PanX) < .005 && math.Abs(a.PanY-b.PanY) < .005 && math.Abs(a.Yaw-b.Yaw) < .5 && math.Abs(a.Pitch-b.Pitch) < .5
}
func (p *avatarPreview) touchCanvas(width, height int, content gogui.View) gogui.View {
	return gogui.Column(gogui.ContainerCfg{ID: "avatar-preview-edit-canvas", Width: float32(width), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, OnMouseDown: func(e gogui.EventCtx) { p.editor.beginAvatarDrag(e, p.editor.gestureViewport(width, height)) }, Content: []gogui.View{content}})
}

func (p *avatarPreview) hiddenEditor(width, height int) gogui.View {
	text := func(k string) string { return avatar.Text(say.Chosen(), k) }
	return gogui.Column(gogui.ContainerCfg{Sizing: gogui.FillFill, Padding: gogui.NoPadding, Spacing: gogui.NoSpacing, SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Content: []gogui.View{
		gogui.Column(gogui.ContainerCfg{Height: float32(height - 92), Sizing: gogui.FillFixed, Padding: gogui.NoPadding, SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius}),
		gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingPx(24), SizeBorder: gogui.NoBorder, Radius: gogui.NoRadius, Content: []gogui.View{
			gogui.Column(gogui.ContainerCfg{Width: float32(width/2 - 108), Height: 72, Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, SizeBorder: gogui.NoBorder}),
			talkModeButton("avatar-preview-show", gogui.IconExpand, text("placement_show"), false, func() { p.controls = true; p.editor.touchReset.Add(1) }),
			talkModeButton("avatar-preview-exit", gogui.IconColumns, text("placement_back"), false, func() { featureshell.Get().Pop() }),
		}}),
	}})
}

func previewEditorHint(id, text string) gogui.View {
	st := gogui.CurrentTheme().TextStyleDef
	st.Size = 15
	return gogui.Text(gogui.TextCfg{ID: id, Text: text, TextStyle: st, Mode: gogui.TextModeWrap})
}
