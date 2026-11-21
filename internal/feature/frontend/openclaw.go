//go:build !board_biscuit

package frontend

import (
	"github.com/ygelfand/echolocal/internal/board"
	"github.com/ygelfand/echolocal/internal/component"
	"github.com/ygelfand/echolocal/internal/feature/assistantconfig"
	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	claw "github.com/ygelfand/echolocal/internal/feature/openclaw"
	"github.com/ygelfand/echolocal/internal/feature/openclawconfig"
	"github.com/ygelfand/echolocal/internal/feature/shell"
	"net/http"
)

func addOpenClaw(pages map[string]http.Handler) {
	if !component.Board().Has(board.Panel) {
		return
	}
	m := clawtalk.Get()
	pages["openclaw"] = openclawconfig.New(openclawconfig.Options{
		WakeChoices: assistantconfig.WakeChoices, WakeIDs: assistantconfig.WakeIDs, SaveWake: assistantconfig.SetWakeWord, Busy: m.Busy,
		Connections: claw.Get().Connections, Store: m.Store, Stop: m.Stop, Changed: shell.Get().Redraw,
		Preview: clawtalk.PreviewAudio,
		History: m.ConfigurationHistory, DeleteHistory: m.DeleteConfigurationHistory,
	}).Handler()
}
