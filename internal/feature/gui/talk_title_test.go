//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
)

func TestSplitTalkTitleAndCompactPrivacyAllLanguages(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range say.Languages() {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			t.Run(fmt.Sprintf("%s-%d", lang, size[0]), func(t *testing.T) {
				rows := []menu.Row{{Label: say.T("clawtalk.state.listening"), Hint: say.T("clawtalk.end_hint")}, {Label: say.T("clawtalk.mic_live"), Hint: say.T("clawtalk.caption_hint")}, {Label: say.T("clawtalk.caption_user"), Hint: "Synthetic conversation"}}
				p := captionTestPage(func() ([]menu.Row, []func(int)) { return rows, nil }, func() {}, func() {}, avatar.State{Ready: true, Running: true, Notice: "running", ModelName: "Not a session title"})
				title := "Synthetic current session"
				mic := "mic_live"
				p.SessionTitle = func() string { return title }
				p.MicrophoneStatus = func() string { return mic }
				w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
				for _, value := range []string{title, strings.Repeat("Wide 界 ä ", 70), "", "Renamed session"} {
					title = value
					for _, privacy := range []string{"mic_live", "mic_wait", "mic_muted", "mic_off"} {
						mic = privacy
						root := w.TestRender(func(w *gogui.Window) gogui.View { return talkAvatarView(w, p) })
						heading := avatarCaptionShape(t, root, "talk-session-title")
						scroller := avatarCaptionShape(t, root, "talk-avatar-transcript")
						texts := strings.Join(avatarCaptionTexts(root), " ")
						if heading.X+heading.Width > float32(size[0]/2-19) || heading.Height > 42 || scroller.Height < 170 {
							t.Fatalf("crowded header or title overflow: %+v / %+v", heading, scroller)
						}
						if !strings.Contains(texts, avatar.Text(lang, mic)) {
							t.Fatal("privacy missing", texts)
						}

						fullTitle := value
						if fullTitle == "" {
							fullTitle = avatar.Text(lang, "session_unknown")
						}
						style := gogui.CurrentTheme().TextStyleDef
						style.Size = 24
						expected := fitTalkTitle(w, fullTitle, style, float32(size[0]/2-40))
						if heading.TC == nil || heading.TC.Text != expected {
							t.Fatal("wrong current title or fallback", expected)
						}
						if w.TextWidth(fullTitle, style) > float32(size[0]/2-40) && !strings.HasSuffix(heading.TC.Text, "…") {
							t.Fatal("long title not ellipsized")
						}
						if value != "Synthetic current session" && strings.Contains(texts, "Synthetic current session") {
							t.Fatal("old title retained")
						}
						for _, forbidden := range []string{say.T("clawtalk.end_hint"), say.T("clawtalk.caption_hint"), say.T("clawtalk.mic_live"), avatar.Text(lang, "talk_running"), "Not a session title", "OpenClaw"} {
							if forbidden != "" && strings.Contains(texts, forbidden) {
								t.Fatal("redundant prose retained", forbidden)
							}
						}
					}
				}
				p.Fullscreen = true
				root := w.TestRender(func(w *gogui.Window) gogui.View { return talkAvatarView(w, p) })
				if avatarCaptionFind(root, "talk-session-title") != nil {
					t.Fatal("title added to fullscreen")
				}
			})
		}
	}
}
