//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
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
func greetingPromptSummary(text string) string {
	if text == "" {
		return promptText("default")
	}
	return promptText("custom")
}
func greetingPromptPageFor(initial string, save func(string) error, redraw func()) *TalkPromptEditor {
	return &TalkPromptEditor{Page: talkPromptPageWithDefault(initial,
		func(text string) (string, error) { return text, talk.ValidateGreetingPrompt(text) },
		save, redraw, greetingPromptTitle(), func() string { return talk.GreetingForLanguage(say.Chosen()) }, false)}
}
func talkGreetingPromptPage(slot int) *TalkPromptEditor {
	return greetingPromptPageFor(clawtalk.Get().Store.Get(slot).GreetingPrompt,
		func(value string) error { return saveTalk(slot, func(s *clawtalk.Slot) { s.GreetingPrompt = value }) },
		func() { shell.Get().Redraw() })
}
