//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
	"time"
)

func TestTalkActivityBothLayoutsLocalizedDotsAndClear(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		for _, size := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s-%d-%t", lang, size[0], full), func(t *testing.T) {
					say.Use(lang)
					rows := []menu.Row{{Label: say.T("clawtalk.state.listening")}, {Label: say.T("clawtalk.mic_live")}, {Label: say.T("clawtalk.caption_user"), Hint: strings.Repeat("Synthetic caption. ", 80)}}
					stops := 0
					p := captionTestPage(func() ([]menu.Row, []func(int)) { return rows, nil }, func() {}, func() { stops++ }, avatar.State{})
					p.Fullscreen = full
					working := true
					p.AgentActivity = func() talk.AgentActivity { return talk.AgentActivity{Available: true, Working: working} }
					w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
					root := w.TestRender(func(w *gogui.Window) gogui.View { return talkAvatarView(w, p) })
					badge := avatarCaptionShape(t, root, "talk-agent-activity")
					expectedWidth := float32(64)
					if badge.Width != expectedWidth || badge.Height != 44 {
						t.Fatal("badge geometry", badge)
					}
					if talkActivityAccessibility(p).A11YLabel != avatar.Text(lang, "agent_working") {
						t.Fatal("accessible translation absent")
					}
					if strings.Contains(strings.Join(avatarCaptionTexts(root), "|"), avatar.Text(lang, "agent_working")) {
						t.Fatal("visible text bar retained")
					}
					if err := w.TestClick(avatarCaptionID(t, w, "talk-agent-activity")); err != nil {
						t.Fatal(err)
					}
					if !p.ActivityPaused() || p.ActivityDots() != 3 || !working || stops != 0 {
						t.Fatal("pause changed task or failed static alternative")
					}
					if talkActivityAccessibility(p).A11YDescription != avatar.Text(lang, "activity_resume") {
						t.Fatal("resume help absent")
					}
					if err := w.TestClick(avatarCaptionID(t, w, "talk-agent-activity")); err != nil {
						t.Fatal(err)
					}
					if p.ActivityPaused() {
						t.Fatal("resume failed")
					}
					root = w.TestRender(nil)
					badge = avatarCaptionShape(t, root, "talk-agent-activity")
					if full {
						if badge.X != float32(size[0]-20)-expectedWidth || badge.Y != float32(size[1]-78) {
							t.Fatal("native aperture parity", badge)
						}
						mode := avatarCaptionShape(t, root, "talk-fullscreen-switch")
						if badge.X <= mode.X+168 {
							t.Fatal("badge covers primary control")
						}
						for _, id := range []string{"talk-fullscreen-stop", "talk-fullscreen-switch"} {
							if err := w.TestClick(avatarCaptionID(t, w, id)); err != nil {
								t.Fatal(err)
							}
						}

					} else {
						stop := avatarCaptionShape(t, root, "talk-avatar-stop")
						if badge.Y+badge.Height > stop.Y || stop.Y+stop.Height > float32(size[1]) {
							t.Fatal("badge hides controls", badge, stop)
						}
						if err := w.TestClick(avatarCaptionID(t, w, "talk-avatar-stop")); err != nil {
							t.Fatal(err)
						}
					}
					if stops != 1 {
						t.Fatal("stop inaccessible")
					}
					working = false
					root = w.TestRender(nil)
					if avatarCaptionFind(root, "talk-agent-activity") != nil {
						t.Fatal("idle retains indicator")
					}
				})
			}
		}
	}
}

func TestActivityDotsRepaintWithoutRebuildingCaptions(t *testing.T) {
	p := &clawtalk.AvatarPage{CaptionPage: &clawtalk.CaptionPage{Page: &sharedshell.Page{Title: "Synthetic"}}, Renderer: &captionTestAvatar{}, Fullscreen: true}
	defer p.Finish()
	working := true
	p.AgentActivity = func() talk.AgentActivity { return talk.AgentActivity{Available: true, Working: working} }
	w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: 1280, Height: 800})
	builds := 0
	w.TestRender(func(w *gogui.Window) gogui.View { builds++; return talkAvatarView(w, p) })
	now := time.Unix(100, 0)
	p.TickActivity(now, true)
	w.FrameFn()
	baseline := builds
	triangles := func() int {
		n := 0
		for _, r := range w.Renderers() {
			n += len(r.Triangles)
		}
		return n
	}
	one := triangles()
	if one == 0 {
		t.Fatal("dots were not painted")
	}
	for step, want := range []int{1, 2, 3, 2, 1} {
		p.TickActivity(now.Add(time.Duration(step)*350*time.Millisecond), true)
		w.FrameFn()
		if p.ActivityDots() != want || triangles() != one*want {
			t.Fatal("paint-only dot frame", step, p.ActivityDots(), triangles(), one*want)
		}
		if builds != baseline {
			t.Fatal("animation rebuilt page/captions", builds, baseline)
		}
	}
	p.ToggleActivityMotion()
	w.InvalidateLayout()
	w.FrameFn()
	baseline = builds
	for step := 5; step < 10; step++ {
		p.TickActivity(now.Add(time.Duration(step)*350*time.Millisecond), true)
		w.FrameFn()
	}
	if p.ActivityDots() != 3 || triangles() != one*3 || builds != baseline {
		t.Fatal("static alternative still animates")
	}
	working = false
	w.InvalidateLayout()
	w.FrameFn()
	if triangles() != 0 {
		t.Fatal("idle retains painted dots")
	}
}
