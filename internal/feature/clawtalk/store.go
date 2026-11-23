package clawtalk

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/ygelfand/echolocal/internal/openclaw"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"github.com/ygelfand/libcountertop/pkg/say"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type Slot struct {
	Ending                talk.EndSettings `json:"ending,omitempty"`
	DeviceContextTemplate string           `json:"deviceContextTemplate,omitempty"`
	GreetingPrompt        string           `json:"greetingPrompt,omitempty"`
	ProgressTone          string           `json:"progressTone,omitempty"`
	StartTone             string           `json:"startTone,omitempty"`
	EndTone               string           `json:"endTone,omitempty"`
	Provider              string           `json:"provider,omitempty"`
	Model                 string           `json:"model,omitempty"`
	OpenClaw              bool             `json:"openclaw"`
	Agent                 string           `json:"agent,omitempty"`
	AgentLabel            string           `json:"agentLabel,omitempty"`
	Context               string           `json:"context,omitempty"`
	ContextLabel          string           `json:"contextLabel,omitempty"`
	Resume                bool             `json:"resume"`
	Timeout               int              `json:"timeout"`
}

// GreetingText resolves only the empty default sentinel; custom text is verbatim.
func (s Slot) GreetingText(language string) string {
	if s.GreetingPrompt != "" {
		return s.GreetingPrompt
	}
	return talk.GreetingForLanguage(language)
}

type Store struct {
	mu      sync.Mutex
	path    string
	slots   [2]Slot
	invalid bool
}

func load(path string) *Store {
	s := &Store{path: path}
	b, e := os.ReadFile(path)
	if e == nil {
		if json.Unmarshal(b, &s.slots) != nil {
			s.invalid = true
		}
	} else if !os.IsNotExist(e) {
		s.invalid = true
	}
	return s
}
func (s *Store) Get(slot int) Slot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot < 0 || slot >= len(s.slots) {
		return Slot{}
	}
	v := s.slots[slot]
	if s.invalid {
		v.OpenClaw = true
	}
	if v.Timeout < 15 || v.Timeout > 900 {
		v.Timeout = 60
	}
	return v
}
func (s *Store) Update(slot int, change func(*Slot)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.invalid || slot < 0 || slot >= len(s.slots) {
		return errors.New("talk_storage")
	}
	next := s.slots
	// Materialize missing language-specific defaults on the next successful save.
	next[slot].Ending = next[slot].Ending.Defaults(say.Chosen())
	change(&next[slot])
	next[slot].Ending = next[slot].Ending.Defaults(say.Chosen())
	if err := ValidateSlot(next[slot]); err != nil {
		return err
	}
	if err := next[slot].Ending.Validate(); err != nil {
		return err
	}
	next[slot].Ending.Phrase = strings.TrimSpace(next[slot].Ending.Phrase)
	if err := openclaw.ValidateDeviceContextTemplate(next[slot].DeviceContextTemplate); err != nil {
		return err
	}
	if err := talk.ValidateGreetingPrompt(next[slot].GreetingPrompt); err != nil {
		return err
	}
	if next[slot].Timeout < 15 {
		next[slot].Timeout = 60
	}
	b, e := json.Marshal(next)
	if e != nil {
		return errors.New("talk_storage")
	}
	if os.MkdirAll(filepath.Dir(s.path), 0700) != nil {
		return errors.New("talk_storage")
	}
	f, e := os.CreateTemp(filepath.Dir(s.path), ".talk-*")
	if e != nil {
		return errors.New("talk_storage")
	}
	defer os.Remove(f.Name())
	e = f.Chmod(0600)
	if e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	end := f.Close()
	if e == nil {
		e = end
	}
	if e == nil {
		e = os.Rename(f.Name(), s.path)
	}
	if e != nil {
		return errors.New("talk_storage")
	}
	s.slots = next
	return nil
}
func (s *Store) Target(slot int) (Slot, error) {
	v := s.Get(slot)
	if s.invalid || !v.OpenClaw || v.Agent == "" || strings.ContainsAny(v.Agent, ":/\\ \t\r\n") {
		return v, errors.New("talk_target_missing")
	}
	if v.Resume && v.Context != "" && !strings.HasPrefix(v.Context, "agent:"+v.Agent+":") {
		return v, errors.New("talk_target_mismatch")
	}
	if !v.Resume || v.Context == "" {
		raw := make([]byte, 16)
		if _, e := rand.Read(raw); e != nil {
			return v, errors.New("talk_identity")
		}
		v.Context = "agent:" + v.Agent + ":echolocal-" + hex.EncodeToString(raw)
		v.ContextLabel = ""
		if e := s.Update(slot, func(w *Slot) { w.Context = v.Context; w.ContextLabel = "" }); e != nil {
			return v, e
		}
	}
	return v, nil
}
func label(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 48 {
		r = r[:48]
	}
	return string(r)
}

// ValidateSlot is shared by native and owner web saves. Dynamic catalog choices
// are checked at selection time, not on unrelated edits or future catalog changes.
func ValidateSlot(v Slot) error {
	if err := v.Ending.Defaults(say.Chosen()).Validate(); err != nil {
		return err
	}
	if err := openclaw.ValidateDeviceContextTemplate(v.DeviceContextTemplate); err != nil {
		return err
	}
	if err := talk.ValidateGreetingPrompt(v.GreetingPrompt); err != nil {
		return err
	}
	if v.Timeout != 0 && (v.Timeout < 15 || v.Timeout > 900) {
		return errors.New("talk_timeout_invalid")
	}
	for _, text := range []string{v.Agent, v.Context, v.Provider, v.Model, v.AgentLabel, v.ContextLabel, v.ProgressTone, v.StartTone, v.EndTone} {
		if !utf8.ValidString(text) || len(text) > 1024 {
			return errors.New("talk_selection_invalid")
		}
		for _, r := range text {
			if unicode.IsControl(r) {
				return errors.New("talk_selection_invalid")
			}
		}
	}
	if strings.ContainsAny(v.Agent, ":/\\ \t\r\n") {
		return errors.New("talk_target_missing")
	}
	return nil
}

// NewStore opens the same private profile store used by the device UI.
func NewStore(path string) *Store { return load(path) }
