package clawtalk

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Slot struct {
	StartTone    string `json:"startTone,omitempty"`
	EndTone      string `json:"endTone,omitempty"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	OpenClaw     bool   `json:"openclaw"`
	Agent        string `json:"agent,omitempty"`
	AgentLabel   string `json:"agentLabel,omitempty"`
	Context      string `json:"context,omitempty"`
	ContextLabel string `json:"contextLabel,omitempty"`
	Resume       bool   `json:"resume"`
	Timeout      int    `json:"timeout"`
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
	change(&next[slot])
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
