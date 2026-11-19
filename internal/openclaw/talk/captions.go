package talk

import (
	"strings"
	"sync"
	"time"
)

// ConversationLog is a bounded, volatile display cache. It is never included
// in diagnostics, persisted, logged or sent to a second service.
type ConversationLog struct {
	mu    sync.Mutex
	calls []Conversation
}
type Caption struct{ Role, Text, ID string }
type Conversation struct {
	Generation uint64
	Started    time.Time
	Entries    []Caption
}

func (l *ConversationLog) Begin(generation uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.calls) == 6 {
		copy(l.calls, l.calls[1:])
		l.calls = l.calls[:5]
	}
	l.calls = append(l.calls, Conversation{Generation: generation, Started: time.Now()})
}
func (l *ConversationLog) Append(generation uint64, role, text, id string) {
	if role != "user" && role != "assistant" {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	r := []rune(text)
	if len(r) > 4096 {
		text = string(r[:4096]) + "…"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.calls) == 0 {
		return
	}
	c := &l.calls[len(l.calls)-1]
	if c.Generation != generation {
		return
	}
	if id != "" {
		for _, e := range c.Entries {
			if e.ID == id && e.Role == role {
				return
			}
		}
	}
	if len(c.Entries) == 32 {
		copy(c.Entries, c.Entries[1:])
		c.Entries = c.Entries[:31]
	}
	c.Entries = append(c.Entries, Caption{Role: role, Text: text, ID: id})
}
func (l *ConversationLog) Snapshot() []Conversation {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]Conversation(nil), l.calls...)
	for i := range out {
		out[i].Entries = append([]Caption(nil), out[i].Entries...)
	}
	return out
}

// Remove erases only the local display cache. A deleted active generation is
// not recreated by future captions; the next activation starts a new cache.
func (l *ConversationLog) Remove(generation uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.calls {
		if l.calls[i].Generation == generation {
			copy(l.calls[i:], l.calls[i+1:])
			l.calls[len(l.calls)-1] = Conversation{}
			l.calls = l.calls[:len(l.calls)-1]
			return
		}
	}
}
func (l *ConversationLog) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	clear(l.calls)
	l.calls = nil
}
