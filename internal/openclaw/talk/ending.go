package talk

import (
	"errors"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// EndSettings is snapshotted at activation; changing settings never changes an
// already-running conversation. Empty fields retain conservative defaults.
type EndSettings struct {
	Phrase   string `json:"phrase,omitempty"`
	Position string `json:"position,omitempty"`
	Timing   string `json:"timing,omitempty"`
}

func DefaultEndPhrase(language string) string {
	switch strings.Split(strings.ToLower(language), "-")[0] {
	case "de":
		return "Gespräch beenden"
	case "it":
		return "Termina conversazione"
	case "vi":
		return "Kết thúc cuộc trò chuyện"
	default:
		return "End conversation"
	}
}

func (s EndSettings) Defaults(language string) EndSettings {
	if s.Phrase == "" {
		s.Phrase = DefaultEndPhrase(language)
	}
	if s.Position == "" {
		s.Position = "exact"
	}
	if s.Timing == "" {
		s.Timing = "turn_end"
	}
	return s
}

func (s EndSettings) Validate() error {
	if !utf8.ValidString(s.Phrase) || utf8.RuneCountInString(s.Phrase) > 256 || normalizeEnd(s.Phrase) == "" {
		return errors.New("talk_end_phrase_invalid")
	}
	hasWord := false
	for _, r := range s.Phrase {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			hasWord = true
		}
		if unicode.IsControl(r) {
			return errors.New("talk_end_phrase_invalid")
		}
	}
	if !hasWord {
		return errors.New("talk_end_phrase_invalid")
	}
	if s.Position != "exact" && s.Position != "start" && s.Position != "end" {
		return errors.New("talk_end_position_invalid")
	}
	if s.Timing != "immediate" && s.Timing != "turn_end" {
		return errors.New("talk_end_timing_invalid")
	}
	return nil
}

func normalizeEnd(text string) string {
	// Preserve marks (Vietnamese/decomposed Unicode) and underscore as word
	// constituents; punctuation cannot hide an inside-word match.
	text = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' {
			return unicode.ToLower(r)
		}
		return ' '
	}, norm.NFC.String(text))
	return strings.Join(strings.Fields(text), " ")
}

func (s EndSettings) Matches(text string) bool {
	text, phrase := normalizeEnd(text), normalizeEnd(s.Phrase)
	if phrase == "" {
		return false
	}
	switch s.Position {
	case "exact":
		// Keep the stored key for existing slots; match the complete phrase
		// anywhere, with word boundaries on both sides.
		return strings.Contains(" "+text+" ", " "+phrase+" ")
	case "start":
		return text == phrase || strings.HasPrefix(text, phrase+" ")
	case "end":
		return text == phrase || strings.HasSuffix(text, " "+phrase)
	default:
		return false
	}
}

// endTranscript owns a separate utterance cache, not the paragraph display cache.
// A delta appends; a snapshot replaces (including empty revisions); finals reset.
// No speech text is retained after a final or logged/persisted here.
type endTranscript struct {
	mu       sync.Mutex
	text, id string
	overflow bool
}

func (e *endTranscript) reset(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.text, e.id, e.overflow = "", id, false
}

func (s *Session) userTranscript(role, text, id string, snapshot, final bool) bool {
	if role != "user" {
		return false
	}
	e := &s.endTranscript
	e.mu.Lock()
	if id != "" && e.id != id {
		e.text, e.id, e.overflow = "", id, false
	}
	if snapshot || final {
		e.text, e.overflow = "", false
	}
	if len(e.text)+len(text) > 16384 {
		e.text, e.overflow = "", true
	} else if !e.overflow {
		e.text += text
	}
	settings := s.Ending.Defaults("en")
	match := !e.overflow && (final || settings.Timing == "immediate") && settings.Matches(e.text)
	if final {
		e.text, e.id, e.overflow = "", "", false
	}
	e.mu.Unlock()
	if match {
		s.requestEnd()
	}
	return match
}

func (s *Session) requestEnd() {
	s.endOnce.Do(func() {
		if s.End != nil {
			go s.End()
		}
	})
}
