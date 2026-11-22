//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/apps"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
	"github.com/ygelfand/echolocal/internal/feature/lock"
	"github.com/ygelfand/echolocal/internal/feature/privacy"
	"github.com/ygelfand/echolocal/internal/feature/settings"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/hardware/mic"
	"github.com/ygelfand/echolocal/internal/layout"
	sharedapps "github.com/ygelfand/libcountertop/pkg/display/apps"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
	"math"
	"sync/atomic"
	"time"
)

type avatarLabController interface {
	Snapshot() avatar.State
	Configure(avatar.Options)
	Start()
	Stop()
	SetViewport(avatar.Viewport)
}
type avatarView struct {
	note           string
	app            avatarLabController
	dragEpoch      atomic.Uint64
	load           func()
	loaded         atomic.Bool
	touchWindow    *gogui.Window
	touchMulti     bool
	touchSeenReset uint64
	touchReset     atomic.Uint64
}

func (*avatarView) Covers() bool           { return true }
func (*avatarView) Timeout() time.Duration { return 6 * time.Minute }
func (v *avatarView) Close() {
	v.loaded.Store(false)
	v.dragEpoch.Add(1)
	v.touchReset.Add(1)
	v.app.Stop()
}
func extendAvatar(_ *widgets.UI, o *screens.Options, _ func() *screens.App) {
	if component.Board().Codename != "athena" {
		return
	}
	c := avatar.New(layout.AvatarDir, func() (<-chan []int16, func()) { return mic.Get().ListenUnleveled("avatar") }, privacy.Get().MicMuted, featureshell.Get().Redraw)
	c.SetSource(func() string { return config.AvatarSection.Read().Path })
	avatarlab.Controls()
	v := &avatarView{app: c, load: func() { c.Configure(avatarlab.Options(config.AvatarSection.Read())) }}
	open := func() {
		o := avatarlab.Options(config.AvatarSection.Read())
		c.Configure(o)
		featureshell.Get().Push(v)
	}
	apps.Get().Add(sharedapps.App{ID: "vrm-test", Name: func() string { return avatarWords("Avatar Lab", "Avatar-Labor") }, Icon: "mdi:flask", Open: open})
	settings.RegisterAvatar(func() sharedshell.View { return v })
	o.Views = append(o.Views, func(view sharedshell.View) *screens.Screen {
		if view != v {
			return nil
		}
		return &screens.Screen{View: v, Fixed: true, Build: v.build}
	})
}
func avatarWords(en, de string) string {
	if say.Chosen() == "de" {
		return de
	}
	return en
}
func avatarText(text string) gogui.View {
	st := gogui.CurrentTheme().Cfg.TextStyleDef
	st.Size = 19
	return gogui.Text(gogui.TextCfg{Text: text, TextStyle: st, Mode: gogui.TextModeWrap})
}
func avatarButton(id, label string, selected, disabled bool, do func()) gogui.View {
	color := gogui.Hex(0x283C42)
	if selected {
		color = gogui.Hex(0x3E706D)
	}
	st := gogui.CurrentTheme().Cfg.TextStyleDef
	st.Size = 20
	st.Color = gogui.Hex(0xEEF3F4)
	return gogui.Button(gogui.ButtonCfg{ID: id, MinHeight: 48, Height: 48, Sizing: gogui.FillFit, Color: color, Disabled: disabled, Content: []gogui.View{gogui.Text(gogui.TextCfg{Text: label, TextStyle: st})}, OnClick: func(e gogui.EventCtx) { do(); e.Window.InvalidateLayout() }})
}
func avatarRow(v ...gogui.View) gogui.View {
	return gogui.Row(gogui.ContainerCfg{Sizing: gogui.FillFit, Padding: gogui.NoPadding, Spacing: gogui.SpacingSmall, Content: v})
}
func (v *avatarView) build(w *gogui.Window) gogui.View {
	if v.load != nil && v.loaded.CompareAndSwap(false, true) {
		v.load()
	}
	v.attachAvatarTouch(w)
	if epoch := v.touchReset.Load(); v.touchSeenReset != epoch {
		v.touchSeenReset = epoch
		v.touchMulti = false
	}
	s := v.app.Snapshot()
	width, height := w.WindowSize()
	size := min(width/2, height-100)
	supported := width >= 640 && height >= 480
	v.app.SetViewport(avatar.Viewport{X: width / 2, Y: 80, Size: size})
	option := func(update func(*avatar.Options)) { o := v.app.Snapshot().Options; update(&o); v.app.Configure(o) }
	toggle := func(id, en, de string, on bool, update func(*avatar.Options)) gogui.View {
		mark := "○ "
		if on {
			mark = "● "
		}
		return avatarButton(id, mark+avatarWords(en, de), on, false, func() { option(update) })
	}
	modes := []string{avatarWords("Closed", "Geschlossen"), avatarWords("Fixed", "Manuell"), avatarWords("Synthetic", "Synthetisch"), avatarWords("Microphone", "Mikrofon")}
	modeButton := func(n int) gogui.View {
		return avatarButton(fmt.Sprintf("vrm-mouth-%d", n), modes[n], s.Options.Mouth == n, false, func() { option(func(o *avatar.Options) { o.Mouth = n }) })
	}
	notes := map[string]string{"timeout": avatarWords("5-minute limit reached.", "5-Minuten-Limit erreicht."), "renderer_timeout": avatarWords("Renderer did not become ready.", "Renderer wurde nicht bereit."), "assets_unavailable": avatarWords("Import a VRM with echolocal avatar import first.", "Zuerst ein VRM mit echolocal avatar import auswählen."), "starting": avatarWords("Loading avatar…", "Avatar wird geladen…"), "model_invalid": avatarWords("VRM asset rejected.", "VRM-Datei abgelehnt."), "motion_invalid": avatarWords("Animation rejected.", "Animationsdatei abgelehnt."), "gpu_failed": avatarWords("Graphics renderer unavailable.", "Grafik-Renderer nicht verfügbar."), "protocol_error": avatarWords("Renderer protocol mismatch.", "Renderer-Schnittstelle inkompatibel."), "ready": avatarWords("Ready — maximum 5 minutes per run.", "Bereit – maximal 5 Minuten pro Lauf."), "running": avatarWords("Running — controls apply live.", "Läuft – Schalter wirken sofort."), "stopping": avatarWords("Stopping…", "Wird beendet…"), "finished": avatarWords("Stopped. Start to run again.", "Beendet. Start für einen neuen Lauf."), "missing_renderer": avatarWords("Renderer is missing. Repair the EchoLocal installation.", "Renderer fehlt. EchoLocal-Installation reparieren."), "start_failed": avatarWords("Could not start renderer.", "Rendererstart fehlgeschlagen."), "renderer_failed": avatarWords("Renderer failed. Restart manually.", "Rendererfehler. Manuell neu starten.")}
	note := notes[s.Notice]
	if translated := avatar.Text(say.Chosen(), s.Notice); translated != "" {
		note = translated
	}
	if !supported {
		note = avatarWords("Preview needs at least 640×480.", "Vorschau benötigt mindestens 640×480.")
	}
	body := []gogui.View{
		avatarRow(avatarButton("vrm-back", avatarWords("Back", "Zurück"), false, false, func() { v.app.Stop(); featureshell.Get().Pop() }), avatarText(avatarWords("Avatar Lab", "Avatar-Labor"))),
		avatarText(note),
		avatarRow(avatarButton("avatar-file", avatar.Text(say.Chosen(), "model"), false, false, func() {
			v.app.Stop()
			lock.Get().Gate().Open(func() { featureshell.Get().Push(settings.AvatarModelPage()) })
		})),
		avatarText(avatar.Text(say.Chosen(), "selection")),
		avatarRow(avatarButton("avatar-save", avatar.Text(say.Chosen(), "save_presentation"), false, false, func() {
			if err := avatarlab.SavePresentation(v.app.Snapshot().Options); err != nil {
				v.note = avatar.Text(say.Chosen(), "save_failed")
			} else {
				v.note = avatar.Text(say.Chosen(), "saved")
			}
		}), avatarButton("avatar-presentation", avatar.Text(say.Chosen(), "presentation"), false, false, func() { v.app.Stop(); featureshell.Get().Push(settings.AvatarPresentationPage()) })),
		avatarText(v.note),
		avatarRow(avatarButton("vrm-start", "Start", false, s.Running || !supported, v.app.Start), avatarButton("vrm-stop", "Stop", false, !s.Running, v.app.Stop)),
		avatarText(fmt.Sprintf("%s %.0f° / %.0f° · %.0f %%", avatarWords("Rotation", "Drehung"), s.Options.Yaw, s.Options.Pitch, avatar.EffectiveZoom(s.Options.Zoom)*100)),
		avatarText(fmt.Sprintf("X %+.0f %% · Y %+.0f %%", s.Options.PanX*100, s.Options.PanY*100)),
		avatarRow(avatarButton("avatar-rotate-left", "← 15°", false, false, func() { option(func(o *avatar.Options) { o.Yaw -= 15 }) }), avatarButton("avatar-rotate-reset", avatarWords("Front", "Vorne"), false, false, func() {
			option(func(o *avatar.Options) {
				o.Yaw = 0
				o.Pitch = 0
				o.Zoom = 1
				o.PanX = 0
				o.PanY = 0
				v.dragEpoch.Add(1)
				v.touchReset.Add(1)
			})
			if err := avatarlab.SavePresentation(v.app.Snapshot().Options); err != nil {
				v.note = avatar.Text(say.Chosen(), "save_failed")
			} else {
				v.note = avatar.Text(say.Chosen(), "saved")
			}
		}), avatarButton("avatar-rotate-right", "15° →", false, false, func() { option(func(o *avatar.Options) { o.Yaw += 15 }) })),
		toggle("vrm-idle", "Idle movement", "Idle-Bewegung", s.Options.Idle, func(o *avatar.Options) { o.Idle = !o.Idle }),
		toggle("vrm-blink", "Blink", "Blinzeln", s.Options.Blink, func(o *avatar.Options) { o.Blink = !o.Blink }),
		toggle("vrm-animation", "VRMA animation", "VRMA-Animation", s.Options.Animation, func(o *avatar.Options) { o.Animation = !o.Animation }),
		avatarText(avatarWords("Mouth source", "Mundsteuerung")), avatarRow(modeButton(0), modeButton(1)), avatarRow(modeButton(2), modeButton(3)),
		avatarRow(avatarButton("vrm-fixed-down", "−", false, s.Options.Mouth != 1, func() { option(func(o *avatar.Options) { o.Fixed -= .1 }) }), avatarText(fmt.Sprintf("%.0f %%", s.Options.Fixed*100)), avatarButton("vrm-fixed-up", "+", false, s.Options.Mouth != 1, func() { option(func(o *avatar.Options) { o.Fixed += .1 }) })),
		avatarText(fmt.Sprintf("FPS %.1f · Frames %d · %.0f s\n%s %.0f %% · RMS %.4f\n%s %d · %s %d", s.FPS, s.Frames, s.Seconds, avatarWords("Mouth", "Mund"), s.Mouth*100, s.RMS, avatarWords("Mic blocks", "Mikrofonblöcke"), s.Blocks, avatarWords("Gaps >100ms", "Lücken >100ms"), s.Gaps)),
		avatarText(avatarWords("Local level only; no recording or upload. Mic: stay quiet for 1 second. Hardware mute is respected.", "Nur lokaler Pegel; keine Aufnahme oder Übertragung. Mikrofon: 1 Sekunde ruhig bleiben. Hardware-Mute bleibt wirksam.")),
	}
	if s.Muted {
		body = append(body, avatarText(avatarWords("Microphone muted", "Mikrofon stummgeschaltet")))
	}
	left := gogui.Column(gogui.ContainerCfg{ID: "vrm-controls", Width: float32(width)/2 - 16, Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.PadAll(14), Spacing: gogui.SpacingSmall, Scrollable: true, DragScroll: true, ScrollMode: gogui.ScrollVerticalOnly, Content: body})
	right := gogui.Column(gogui.ContainerCfg{ID: "avatar-preview-touch", OnMouseDown: func(e gogui.EventCtx) { v.beginAvatarDrag(e, avatar.Viewport{X: width / 2, Y: 80, Size: size}) }, Width: float32(width)/2 + 16, Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.PadAll(16), Content: []gogui.View{avatarText(s.ModelName + " · " + s.Credit), avatarText(avatarWords("One finger: rotate. Two: move. Pinch: zoom.", "Ein Finger: drehen. Zwei: verschieben. Pinch: zoomen."))}})
	return gogui.Row(gogui.ContainerCfg{ID: "avatar-app", Width: float32(width), Height: float32(height), Sizing: gogui.FixedFixed, Padding: gogui.NoPadding, Content: []gogui.View{left, right}})
}

func (v *avatarView) beginAvatarDrag(e gogui.EventCtx, vp avatar.Viewport) {
	if v.touchMulti || e.Event == nil || e.Window == nil || e.Layout == nil || e.Layout.Shape == nil || e.Event.MouseButton != gogui.MouseLeft || vp.Size <= 0 {
		return
	}
	// Widget callbacks receive local coordinates; captured moves are window-absolute.
	x, y := e.Event.MouseX+e.Layout.Shape.X, e.Event.MouseY+e.Layout.Shape.Y
	if x < float32(vp.X) || x >= float32(vp.X+vp.Size) || y < float32(vp.Y) || y >= float32(vp.Y+vp.Size) {
		return
	}
	state := v.app.Snapshot()
	if !state.Running || !state.Ready || !featureshell.Get().Visible(v) {
		return
	}
	epoch := v.dragEpoch.Add(1)
	startYaw, startPitch := state.Options.Yaw, state.Options.Pitch
	decided := false
	end := func() { v.dragEpoch.CompareAndSwap(epoch, epoch+1) }
	e.Consume()
	move := func(m gogui.EventCtx) {
		if m.Event == nil || v.dragEpoch.Load() != epoch || !featureshell.Get().Visible(v) {
			return
		}
		current := v.app.Snapshot()
		if !current.Running || !current.Ready {
			return
		}
		dx, dy := float64(m.Event.MouseX-x), float64(m.Event.MouseY-y)
		if math.IsNaN(dx) || math.IsInf(dx, 0) || math.IsNaN(dy) || math.IsInf(dy, 0) {
			return
		}
		if !decided {
			if math.Hypot(dx, dy) < 6 {
				return
			}
			decided = true
		}
		options := current.Options
		options.Yaw = startYaw + dx*180/float64(vp.Size)
		options.Pitch = math.Max(-85, math.Min(85, startPitch+dy*180/float64(vp.Size)))
		v.app.Configure(options)
		m.Window.InvalidateLayout()
	}
	e.Window.MouseLock(gogui.MouseLockCfg{MouseMove: move,
		MouseUp: func(m gogui.EventCtx) { move(m); m.Window.MouseUnlock(); end() },
		Cancel:  func(*gogui.Window) { end() },
	})
}
