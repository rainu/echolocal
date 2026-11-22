//go:build !board_biscuit

package gui

import (
	"fmt"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
)

type captionTestAvatar struct{ state avatar.State }

func (a *captionTestAvatar) Snapshot() avatar.State    { return a.state }
func (*captionTestAvatar) Start()                      {}
func (*captionTestAvatar) Stop()                       {}
func (*captionTestAvatar) SetViewport(avatar.Viewport) {}

func avatarCaptionShape(t *testing.T, root *gogui.Layout, id string) *gogui.Shape {
	t.Helper()
	if root.Shape != nil && root.Shape.ID == id {
		return root.Shape
	}
	for i := range root.Children {
		if s := avatarCaptionFind(&root.Children[i], id); s != nil {
			return s
		}
	}
	t.Fatal("missing layout", id)
	return nil
}
func avatarCaptionFind(root *gogui.Layout, id string) *gogui.Shape {
	if root.Shape != nil && root.Shape.ID == id {
		return root.Shape
	}
	for i := range root.Children {
		if s := avatarCaptionFind(&root.Children[i], id); s != nil {
			return s
		}
	}
	return nil
}
func avatarCaptionTexts(root *gogui.Layout) []string {
	var out []string
	var walk func(*gogui.Layout)
	walk = func(n *gogui.Layout) {
		if n.Shape != nil && n.Shape.TC != nil {
			out = append(out, n.Shape.TC.Text)
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(root)
	return out
}
func avatarCaptionID(t *testing.T, w *gogui.Window, leaf string) string {
	t.Helper()
	ids := w.ResolveID(leaf)
	if len(ids) != 1 {
		t.Fatal(leaf, ids)
	}
	return ids[0]
}
func captionTestPage(build func() ([]menu.Row, []func(int)), history, stop func(), state avatar.State) *clawtalk.AvatarPage {
	p := &clawtalk.AvatarPage{CaptionPage: &clawtalk.CaptionPage{Page: &sharedshell.Page{
		Title: "OpenClaw", Build: build, Actions: []sharedshell.Action{
			{Label: say.T("clawtalk.history"), Do: history},
			{Label: say.T("clawtalk.end"), Do: stop},
		},
	}}, Renderer: &captionTestAvatar{state: state}}
	// Layout tests must not start a native process or an audio conversation.
	p.Finish()
	return p
}
func TestTalkAvatarTranscriptScrollKeepsActionsVisible(t *testing.T) {
	before := say.Chosen()
	defer say.Use(before)
	for _, lang := range say.Languages() {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}} {
			t.Run(fmt.Sprintf("%s-%dx%d", lang, size[0], size[1]), func(t *testing.T) {
				history, stops := 0, 0
				rows := []menu.Row{
					{Label: say.T("clawtalk.state.listening"), Hint: say.T("clawtalk.end_hint")},
					{Label: say.T("clawtalk.mic_live"), Hint: say.T("clawtalk.caption_hint")},
				}
				for i := 0; i < 32; i++ {
					role := "clawtalk.caption_assistant"
					if i%2 == 1 {
						role = "clawtalk.caption_user"
					}
					rows = append(rows, menu.Row{Label: say.T(role), Hint: fmt.Sprintf("Synthetic turn %d: %s", i, strings.Repeat("A long spoken contribution with Unicode ä 日本語. ", 12))})
				}
				p := captionTestPage(func() ([]menu.Row, []func(int)) { return rows, nil }, func() { history++ }, func() { stops++ }, avatar.State{Ready: true, Running: true, Notice: "running", ModelName: "synthetic model"})
				w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
				root := w.TestRender(func(w *gogui.Window) gogui.View { return talkAvatarView(w, p) })
				scroller := avatarCaptionShape(t, root, "talk-avatar-transcript")
				actions := avatarCaptionShape(t, root, "talk-avatar-actions")
				stop := avatarCaptionShape(t, root, "talk-avatar-stop")
				historyButton := avatarCaptionShape(t, root, "talk-avatar-history")
				stopY := stop.Y
				if !scroller.Scrollable || scroller.Height < 70 || scroller.X+scroller.Width > float32(size[0])/2 || scroller.Y+scroller.Height > actions.Y+.5 {
					t.Fatalf("transcript/actions overlap or unusable: %+v / %+v", scroller, actions)
				}
				if stop.Height < 64 || stop.Y+stop.Height > float32(size[1]) || stop.Y != historyButton.Y || actions.Scrollable {
					t.Fatal("unreachable or unpinned controls")
				}
				texts := strings.Join(avatarCaptionTexts(root), " ")
				for _, i := range []int{0, 1, 31} {
					if !strings.Contains(texts, rows[i+2].Hint) {
						t.Fatal("caption omitted", i)
					}
				}
				if err := w.TestScroll(avatarCaptionID(t, w, "talk-avatar-transcript"), 0, -200); err != nil {
					t.Fatal(err)
				}
				_, y, err := w.TestScrollOffset(avatarCaptionID(t, w, "talk-avatar-transcript"))
				if err != nil || y >= 0 {
					t.Fatal("transcript did not scroll", y, err)
				}
				root = w.TestRender(nil)
				if avatarCaptionShape(t, root, "talk-avatar-stop").Y != stopY {
					t.Fatal("stop scrolled with transcript")
				}
				if err := w.TestClick(avatarCaptionID(t, w, "talk-avatar-history")); err != nil {
					t.Fatal(err)
				}
				if err := w.TestClick(avatarCaptionID(t, w, "talk-avatar-stop")); err != nil {
					t.Fatal(err)
				}
				if history != 1 || stops != 1 {
					t.Fatal("wrong action routing", history, stops)
				}
			})
		}
	}
}
func TestTalkAvatarPartialFinalAndFailureCaptions(t *testing.T) {
	text := "Synthetic partial"
	rows := func() ([]menu.Row, []func(int)) {
		return []menu.Row{
			{Label: "status"}, {Label: "microphone"},
			{Label: say.T("clawtalk.caption_user"), Hint: "Synthetic user speech"},
			{Label: say.T("clawtalk.caption_assistant"), Hint: text},
		}, nil
	}
	p := captionTestPage(rows, func() {}, func() {}, avatar.State{Notice: "custom_unavailable"})
	w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: 1280, Height: 800})
	root := w.TestRender(func(w *gogui.Window) gogui.View { return talkAvatarView(w, p) })
	if got := strings.Join(avatarCaptionTexts(root), " "); !strings.Contains(got, text) || !strings.Contains(got, "Synthetic user speech") || !strings.Contains(got, avatar.Text(say.Chosen(), "talk_fallback")) {
		t.Fatal("failure view lost transcript or recovery", got)
	}
	text = "Synthetic final answer"
	root = w.TestRender(nil)
	got := strings.Join(avatarCaptionTexts(root), " ")
	if !strings.Contains(got, text) || strings.Contains(got, "Synthetic partial") {
		t.Fatal("caption did not update", got)
	}
}
