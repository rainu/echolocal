// Package networkmedia implements an owner-configured, read-only SMB media source.
package networkmedia

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

var ErrSettings = errors.New("Invalid SMB settings: provide a server, port 1–65535, single share name, and username; use a dedicated read-only account")

type Settings struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Share    string `json:"share"`
	Username string `json:"username"`
	Domain   string `json:"domain"`
}
type Update struct {
	Settings
	Password      *string `json:"password,omitempty"`
	ClearPassword bool    `json:"clear_password,omitempty"`
}
type saved struct {
	Settings
	Password string `json:"password"`
}
type View struct {
	Settings
	PasswordSet bool              `json:"password_set"`
	Labels      map[string]string `json:"labels"`
}
type Store struct {
	mu      sync.Mutex
	path    string
	current saved
	loadErr error
	Changed func()
}

func New(path string) *Store {
	s := &Store{path: path, current: saved{Settings: Settings{Port: 445}}}
	b, e := os.ReadFile(path)
	if e == nil {
		e = json.Unmarshal(b, &s.current)
	} else if errors.Is(e, os.ErrNotExist) {
		e = nil
	}
	if e != nil {
		s.loadErr = errors.New("Could not read SMB configuration; save valid settings to recover")
	}
	return s
}
func (s *Store) View() View {
	s.mu.Lock()
	defer s.mu.Unlock()
	return View{s.current.Settings, s.current.Password != "", Labels()}
}
func (s *Store) snapshot() (saved, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current, s.loadErr
}
func clean(v string) bool {
	if len(v) > 255 {
		return false
	}
	for _, c := range v {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}
func valid(c Settings) bool {
	if c.Port < 1 || c.Port > 65535 || !clean(c.Host) || !clean(c.Share) || !clean(c.Username) || !clean(c.Domain) {
		return false
	}
	if c.Host != "" {
		if strings.ContainsAny(c.Host, "/\\@%?# \t") {
			return false
		}
		if net.ParseIP(c.Host) == nil {
			for _, c := range c.Host {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-') {
					return false
				}
			}
		}
	}
	if c.Share != "" && (strings.ContainsAny(c.Share, "/\\:*?\"<>|") || c.Share == "." || c.Share == "..") {
		return false
	}
	return !c.Enabled || c.Host != "" && c.Share != "" && c.Username != ""
}
func (s *Store) Save(u Update) error {
	if !valid(u.Settings) || u.Password != nil && (len(*u.Password) == 0 || len(*u.Password) > 1024 || u.ClearPassword) {
		return ErrSettings
	}
	s.mu.Lock()
	next := saved{Settings: u.Settings, Password: s.current.Password}
	if u.Password != nil {
		next.Password = *u.Password
	}
	if u.ClearPassword {
		next.Password = ""
	}
	if next.Enabled && next.Password == "" {
		s.mu.Unlock()
		return ErrSettings
	}
	b, e := json.Marshal(next)
	if e == nil {
		e = os.MkdirAll(filepath.Dir(s.path), 0700)
	}
	if e == nil {
		var f *os.File
		f, e = os.CreateTemp(filepath.Dir(s.path), ".smb-*")
		if e == nil {
			name := f.Name()
			defer os.Remove(name)
			if e = f.Chmod(0600); e == nil {
				_, e = f.Write(b)
			}
			if e == nil {
				e = f.Sync()
			}
			if closeErr := f.Close(); e == nil {
				e = closeErr
			}
			if e == nil {
				e = os.Rename(name, s.path)
			}
		}
	}
	if e == nil {
		s.current = next
		s.loadErr = nil
	}
	changed := s.Changed
	s.mu.Unlock()
	if e != nil {
		return errors.New("Could not save SMB configuration")
	}
	if changed != nil {
		changed()
	}
	return nil
}
