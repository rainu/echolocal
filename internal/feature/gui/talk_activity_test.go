//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
)

func TestTalkActivityBothLayoutsLocalizedStaticAndClear(t *testing.T) {
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
					expectedWidth := float32(244)
					if full {
						expectedWidth = float32(min(244, size[0]/2-116))
					}
					if badge.Width != expectedWidth || badge.Height != 40 {
						t.Fatal("badge geometry", badge)
					}
					if !strings.Contains(strings.Join(avatarCaptionTexts(root), "|"), avatar.Text(lang, "agent_working")) {
						t.Fatal("translation absent")
					}
					if full {
						if badge.X != float32(size[0]-20)-expectedWidth || badge.Y != float32(size[1]-76) {
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
