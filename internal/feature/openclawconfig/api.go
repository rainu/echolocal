//go:build !board_biscuit

// Package openclawconfig implements owner configuration, not Node commands.
// Mount Handler ONLY through webapi.Options.Pages (existing owner auth).
package openclawconfig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/ygelfand/echolocal/internal/feature/clawtalk"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/say"
)

type Options struct {
	WakeChoices   func() []clawtalk.Choice
	WakeIDs       func() []string
	SaveWake      func(int, string) error
	Busy          func() bool
	Caller        clawtalk.TalkCaller
	Connections   *openclaw.Connections
	Store         *clawtalk.Store
	Stop          func() bool
	Changed       func()
	Preview       func(cue, choice string) []byte
	History       func() []talk.Conversation
	DeleteHistory func(uint64)
}
type API struct{ o Options }

func New(o Options) *API { return &API{o: o} }
func (a *API) caller() clawtalk.TalkCaller {
	if a.o.Caller != nil {
		return a.o.Caller
	}
	return a.o.Connections.UI
}
func (a *API) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /{$}", a.view)
	m.HandleFunc("GET /status", a.status)
	m.HandleFunc("PUT /connection", a.connection)
	m.HandleFunc("POST /actions", a.action)
	m.HandleFunc("PUT /permissions/operator", a.operator)
	m.HandleFunc("PUT /permissions/node", a.node)
	m.HandleFunc("GET /slots/{slot}", a.readSlot)
	m.HandleFunc("PUT /slots/{slot}", a.saveSlot)
	m.HandleFunc("GET /choices/{kind}", a.choices)
	m.HandleFunc("POST /preview", a.preview)
	m.HandleFunc("GET /history", a.history)
	m.HandleFunc("PUT /wake/{slot}", a.wake)
	m.HandleFunc("DELETE /history/{id}", a.deleteHistory)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// The shared Page mount strips the prefix; normalize its empty root.
		if r.URL.Path == "" {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		m.ServeHTTP(w, r)
	})
}
func output(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, err error, status int) {
	// Never echo request values or raw Gateway/provider/storage errors.
	code := "settings"
	if err != nil {
		switch err.Error() {
		case "invalid_endpoint", "invalid_name", "invalid_scopes", "permission_unknown", "talk_timeout_invalid", "talk_target_missing", "talk_target_mismatch", "talk_selection_invalid", "talk_end_phrase_invalid", "talk_end_position_invalid", "talk_end_timing_invalid", "talk_context_too_long", "talk_context_placeholder", "talk_context_invalid", "talk_greeting_too_long", "talk_greeting_invalid", "confirmation_required", "selection_unavailable", "busy", "bad_request":
			code = err.Error()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	output(w, map[string]string{"error": code})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		failure(w, errors.New("bad_request"), 400)
		return false
	}
	return true
}
func (a *API) changed() {
	if a.o.Changed != nil {
		a.o.Changed()
	}
}

type connectionView struct {
	URL             string          `json:"url"`
	UIName          string          `json:"uiName"`
	NodeName        string          `json:"nodeName"`
	TokenConfigured bool            `json:"tokenConfigured"`
	UI              openclaw.Status `json:"ui"`
	Node            openclaw.Status `json:"node"`
}

func (a *API) view(w http.ResponseWriter, r *http.Request) {
	g := a.o.Connections
	c := g.Editable()
	grants := map[string]bool{}
	for _, v := range openclaw.PermissionGroups {
		grants[v] = g.Node.NodeAccess().Granted(v)
	}
	slots := []clawtalk.Slot{a.o.Store.Get(0), a.o.Store.Get(1)}
	for i := range slots {
		slots[i].Ending = slots[i].Ending.Defaults(say.Chosen())
	}
	output(w, map[string]any{
		"connection":      connectionView{c.URL, c.UIName, c.NodeName, c.Token != "", g.UI.Status(), g.Node.Status()},
		"requestedScopes": g.UI.RequestedOperatorScopes(), "grantedScopes": g.UI.GrantedOperatorScopes(), "supportedScopes": openclaw.SupportedOperatorScopes(),
		"grants": grants, "groups": openclaw.PermissionGroups, "slots": slots,
		"tones": clawtalk.ToneChoices(), "language": say.Chosen(), "labels": Labels(),
		"defaults":     map[string]any{"deviceContextTemplate": openclaw.DefaultDeviceContextTemplate(), "greetingPrompt": talk.GreetingForLanguage(say.Chosen()), "ending": talk.EndSettings{}.Defaults(say.Chosen())},
		"placeholders": openclaw.DeviceContextPlaceholders,
		"wake":         a.wakeView(),
		"events":       map[string]any{"ui": g.UI.Events(), "node": g.Node.Events()},
	})
}
func (a *API) connection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL      string  `json:"url"`
		UIName   string  `json:"uiName"`
		NodeName string  `json:"nodeName"`
		Token    *string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	// A missing token keeps it. An explicit empty string clears the shared
	// credential, never the role-scoped identities/device tokens.
	next := a.o.Connections.Editable()
	next.URL = body.URL
	next.UIName = body.UIName
	next.NodeName = body.NodeName
	if body.Token != nil {
		next.Token = *body.Token
	}
	if err := a.o.Connections.Save(next); err != nil {
		failure(w, err, 422)
		return
	}
	a.changed()
	a.view(w, r)
}
func (a *API) action(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Action  string `json:"action"`
		Role    string `json:"role"`
		Confirm bool   `json:"confirm"`
	}
	if !decode(w, r, &b) {
		return
	}
	g := a.o.Connections
	var err error
	if b.Role != "" && b.Role != "both" && b.Role != "ui" && b.Role != "node" {
		failure(w, errors.New("bad_request"), 400)
		return
	}
	if (b.Action == "pair_again" || b.Action == "forget") && b.Role != "" && b.Role != "both" {
		failure(w, errors.New("bad_request"), 400)
		return
	}
	role := g.UI
	if b.Role == "node" {
		role = g.Node
	}
	switch b.Action {
	case "connect":
		if b.Role == "" || b.Role == "both" {
			err = g.Connect()
		} else {
			err = role.Connect()
		}
	case "disconnect":
		if b.Role == "" || b.Role == "both" {
			err = g.Disconnect()
		} else {
			err = role.Disconnect()
		}
	case "pair_again", "forget":
		if !b.Confirm {
			failure(w, errors.New("confirmation_required"), 422)
			return
		}
		if b.Action == "forget" {
			err = g.Forget()
		} else {
			err = g.PairAgain()
		}
	case "stop_talk":
		if a.o.Stop != nil {
			a.o.Stop()
		}
	case "clear_events":
		if b.Role == "" || b.Role == "both" {
			g.UI.ClearEvents()
			g.Node.ClearEvents()
		} else {
			role.ClearEvents()
		}
	default:
		failure(w, errors.New("bad_request"), 400)
		return
	}
	if err != nil {
		failure(w, err, 422)
		return
	}
	a.changed()
	a.view(w, r)
}
func (a *API) operator(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Scopes *[]string `json:"scopes"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Scopes == nil {
		failure(w, errors.New("bad_request"), 400)
		return
	}
	if err := a.o.Connections.UI.SetOperatorScopes(*b.Scopes); err != nil {
		failure(w, err, 422)
		return
	}
	a.changed()
	a.view(w, r)
}
func (a *API) node(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Grants map[string]bool `json:"grants"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Grants == nil {
		failure(w, errors.New("bad_request"), 400)
		return
	}
	if err := a.o.Connections.Node.SetNodeGrants(b.Grants); err != nil {
		failure(w, err, 422)
		return
	}
	a.changed()
	a.view(w, r)
}
func slotNumber(r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("slot"))
	return n - 1, err == nil && n >= 1 && n <= 2
}
func (a *API) readSlot(w http.ResponseWriter, r *http.Request) {
	n, ok := slotNumber(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	output(w, a.o.Store.Get(n))
}
func hasChoice(list []clawtalk.Choice, value string) bool {
	for _, v := range list {
		if v.Value == value {
			return true
		}
	}
	return false
}
func (a *API) saveSlot(w http.ResponseWriter, r *http.Request) {
	n, ok := slotNumber(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var next clawtalk.Slot
	if !decode(w, r, &next) {
		return
	}
	old := a.o.Store.Get(n)
	if next.Agent != old.Agent {
		if next.Context == old.Context {
			next.Context = ""
			next.ContextLabel = ""
		}
		next.AgentLabel = ""
	}
	if next.Context != "" && (next.Agent == "" || len(next.Context) <= len("agent:"+next.Agent+":") || !prefix(next.Context, "agent:"+next.Agent+":")) {
		failure(w, errors.New("talk_target_mismatch"), 422)
		return
	}
	if err := clawtalk.ValidateSlot(next); err != nil {
		failure(w, err, 422)
		return
	}
	if _, err := a.o.Connections.Node.ConversationDeviceContextWithTemplate(next.DeviceContextTemplate); err != nil {
		failure(w, err, 422)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	unavailable := func() { failure(w, errors.New("selection_unavailable"), 422) }
	if next.Agent != "" && next.Agent != old.Agent {
		list, err := clawtalk.AgentChoices(ctx, a.caller())
		if err != nil || !hasChoice(list, next.Agent) {
			unavailable()
			return
		}
		for _, v := range list {
			if v.Value == next.Agent {
				next.AgentLabel = v.Label
			}
		}
	} else {
		next.AgentLabel = old.AgentLabel
	}
	if next.Context == "" {
		next.ContextLabel = ""
	}
	if next.Context != "" && next.Context != old.Context {
		list, err := clawtalk.SessionChoices(ctx, a.caller(), next.Agent)
		if err != nil || !hasChoice(list, next.Context) {
			unavailable()
			return
		}
		for _, v := range list {
			if v.Value == next.Context {
				next.ContextLabel = v.Label
			}
		}
	} else if next.Context != "" && next.Context == old.Context {
		next.ContextLabel = old.ContextLabel
	}
	if next.Provider != old.Provider || next.Model != old.Model {
		if next.Provider != "" || next.Model != "" {
			list, err := clawtalk.LiveModelChoices(ctx, a.caller(), next.Context)
			b, _ := json.Marshal(clawtalk.ModelSelection{Provider: next.Provider, Model: next.Model})
			if err != nil || !hasChoice(list, string(b)) {
				unavailable()
				return
			}
		}
	}
	tones := clawtalk.ToneChoices()
	for _, pair := range [][2]string{{next.StartTone, old.StartTone}, {next.ProgressTone, old.ProgressTone}, {next.EndTone, old.EndTone}} {
		if pair[0] != pair[1] && !hasChoice(tones, pair[0]) {
			unavailable()
			return
		}
	}
	if next.OpenClaw && !old.OpenClaw {
		if err := a.o.Connections.UI.EnableTalk(); err != nil {
			failure(w, err, 422)
			return
		}
	}
	// Stop only when changing live routing, matching the native agent/model/session pickers.
	if a.o.Stop != nil && (next.OpenClaw != old.OpenClaw || next.Agent != old.Agent || next.Context != old.Context || next.Model != old.Model || next.Provider != old.Provider) {
		a.o.Stop()
	}
	if err := a.o.Store.Update(n, func(v *clawtalk.Slot) { *v = next }); err != nil {
		failure(w, err, 422)
		return
	}
	a.changed()
	a.readSlot(w, r)
}
func prefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }
func (a *API) choices(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("slot"))
	if err != nil || n < 1 || n > 2 {
		failure(w, errors.New("bad_request"), 400)
		return
	}
	s := a.o.Store.Get(n - 1)
	if agent, ok := r.URL.Query()["agent"]; ok && len(agent) > 0 {
		s.Agent = agent[0]
		if err := clawtalk.ValidateSlot(clawtalk.Slot{Agent: s.Agent}); err != nil {
			failure(w, err, 422)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	var list []clawtalk.Choice
	switch r.PathValue("kind") {
	case "agents":
		list, err = clawtalk.AgentChoices(ctx, a.caller())
	case "models":
		list, err = clawtalk.LiveModelChoices(ctx, a.caller(), s.Context)
	case "sessions":
		list, err = clawtalk.SessionChoices(ctx, a.caller(), s.Agent)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		failure(w, errors.New("selection_unavailable"), 503)
		return
	}
	output(w, list)
}
func (a *API) preview(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Cue      string `json:"cue"`
		Choice   string `json:"choice"`
		Template string `json:"template"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Cue == "context" {
		text, err := a.o.Connections.Node.ConversationDeviceContextWithTemplate(b.Template)
		if err != nil {
			failure(w, err, 422)
			return
		}
		output(w, map[string]any{"text": text, "bytes": len(text), "maxBytes": openclaw.DeviceContextMaxBytes})
		return
	}
	if b.Cue != "ready" && b.Cue != "progress" && b.Cue != "end" || !hasChoice(clawtalk.ToneChoices(), b.Choice) {
		failure(w, errors.New("bad_request"), 400)
		return
	}
	// Browser audition returns audio only: it must never enqueue device output.
	if a.o.Preview == nil {
		failure(w, errors.New("selection_unavailable"), 503)
		return
	}
	output(w, map[string]any{"audio": a.o.Preview(b.Cue, b.Choice)})
}
func (a *API) history(w http.ResponseWriter, r *http.Request) {
	out := []talk.Conversation{}
	if a.o.History != nil {
		out = a.o.History()
	}
	output(w, out)
}
func (a *API) deleteHistory(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Confirm bool `json:"confirm"`
	}
	if !decode(w, r, &b) {
		return
	}
	if !b.Confirm {
		failure(w, errors.New("confirmation_required"), 422)
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		failure(w, errors.New("bad_request"), 400)
		return
	}
	if a.o.DeleteHistory != nil {
		a.o.DeleteHistory(id)
	}
	a.changed()
	w.WriteHeader(204)
}

func (a *API) wakeView() any {
	if a.o.WakeIDs == nil || a.o.WakeChoices == nil {
		return nil
	}
	return map[string]any{"ids": a.o.WakeIDs(), "choices": a.o.WakeChoices()}
}
func (a *API) wake(w http.ResponseWriter, r *http.Request) {
	n, ok := slotNumber(r)
	if !ok || a.o.SaveWake == nil {
		http.NotFound(w, r)
		return
	}
	if !a.o.Store.Get(n).OpenClaw {
		failure(w, errors.New("talk_selection_invalid"), 422)
		return
	}
	var b struct {
		Value *string `json:"value"`
	}
	if !decode(w, r, &b) {
		return
	}
	if b.Value == nil || !hasChoice(a.o.WakeChoices(), *b.Value) {
		failure(w, errors.New("talk_selection_invalid"), 422)
		return
	}
	if err := a.o.SaveWake(n, *b.Value); err != nil {
		failure(w, err, 422)
		return
	}
	a.changed()
	a.view(w, r)
}

func (a *API) status(w http.ResponseWriter, r *http.Request) {
	g := a.o.Connections
	output(w, map[string]any{"connection": map[string]any{"ui": g.UI.Status(), "node": g.Node.Status()}, "grantedScopes": g.UI.GrantedOperatorScopes(), "events": map[string]any{"ui": g.UI.Events(), "node": g.Node.Events()}})
}
