//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func greetingPromptTitle() string {
	switch say.Chosen() {
	case "de":
		return "Begrüßungsprompt"
	case "it":
		return "Prompt di saluto"
	case "vi":
		return "Lời nhắc chào hỏi"
	default:
		return "Greeting prompt"
	}
}
func greetingPromptPageFor(initial string, save func(string) error, redraw func()) *TalkPromptEditor {
	return &TalkPromptEditor{Page: talkPromptPageWithDefault(initial,
		func(text string) (string, error) { return text, talk.ValidateGreetingPrompt(text) },
		save, redraw, greetingPromptTitle(), func() string { return talk.GreetingForLanguage(say.Chosen()) }, false)}
}

// Both prompts share one screen and one atomic save, but remain separate
// persisted values and separate provider messages at their original times.
func talkConversationPromptPageFor(initial clawtalk.Slot, render func(string) (string, error), save func(string, string) error, redraw func()) *TalkPromptEditor {
	var commitErr error
	device := talkPromptPageFor(initial.DeviceContextTemplate, render, func(string) error { return commitErr }, redraw)
	greeting := greetingPromptPageFor(initial.GreetingPrompt, func(string) error { return commitErr }, redraw)
	page := &sharedshell.Page{Title: say.T("clawtalk.context")}
	page.Build = func() ([]menu.Row, []func(int)) {
		rows, acts := device.Build()
		rows[0].Label = promptText("title")
		more, moreActs := greeting.Build()
		more[0].Label = greetingPromptTitle()
		return append(more, rows...), append(moreActs, acts...)
	}
	page.Actions = []sharedshell.Action{
		{Label: promptText("reset"), MinHeight: 64, Do: func() { device.Actions[0].Do(); greeting.Actions[0].Do() }},
		{Label: say.T("openclaw.action.save"), MinHeight: 64, Role: sharedshell.Primary, Do: func() {
			dr, _ := device.Build()
			gr, _ := greeting.Build()
			d, g := dr[0].Value, gr[0].Value
			_, commitErr = render(d)
			if commitErr == nil {
				commitErr = talk.ValidateGreetingPrompt(g)
			}
			if commitErr == nil {
				if d == openclaw.DefaultDeviceContextTemplate() {
					d = ""
				}
				if g == talk.GreetingForLanguage(say.Chosen()) {
					g = ""
				}
				commitErr = save(d, g)
			}
			// Reflect the same transaction outcome in both draft editors. Their save
			// callbacks above never perform a second persistent write.
			device.Actions[1].Do()
			greeting.Actions[1].Do()
		}},
	}
	return &TalkPromptEditor{Page: page}
}
