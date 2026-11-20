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
type Caption struct {
	Role, Text, ID string
	Provisional    bool
	Timestamp      time.Time
}
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
	// A final may cover several inferred same-role segments. Replace those
	// together, never publish the provisional text as authoritative history.
	position := -1
	timestamp := time.Now()
	kept := c.Entries[:0]
	for _, e := range c.Entries {
		if e.Role == role && e.Provisional {
			if position < 0 {
				position = len(kept)
				timestamp = e.Timestamp
			}
			continue
		}
		kept = append(kept, e)
	}
	c.Entries = kept
	final := Caption{Role: role, Text: text, ID: id, Timestamp: timestamp}
	if position >= 0 {
		c.Entries = append(c.Entries, Caption{})
		copy(c.Entries[position+1:], c.Entries[position:])
		c.Entries[position] = final
	} else {
		c.Entries = append(c.Entries, final)
	}
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

// Delta appends verbatim fragments. Role changes form inferred display paragraphs,
// not provider-final contributions. The cache remains bounded and volatile.
func (l *ConversationLog) Delta(generation uint64, role, delta string) {
	if (role != "user" && role != "assistant") || delta == "" {
		return
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
	n := len(c.Entries)
	if n == 0 || !c.Entries[n-1].Provisional || c.Entries[n-1].Role != role {
		if n == 32 {
			copy(c.Entries, c.Entries[1:])
			c.Entries = c.Entries[:31]
		}
		c.Entries = append(c.Entries, Caption{Role: role, Provisional: true, Timestamp: time.Now()})
	}
	e := &c.Entries[len(c.Entries)-1]
	// Cap before concatenation, including oversized/untrusted fragments.
	r := []rune(e.Text)
	if len(r) < 4096 {
		d := []rune(delta)
		if len(d) > 4096-len(r) {
			d = d[:4096-len(r)]
		}
		e.Text += string(d)
	}
}
func (l *ConversationLog) DropPending(generation uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.calls {
		c := &l.calls[i]
		if c.Generation != generation {
			continue
		}
		kept := c.Entries[:0]
		for _, e := range c.Entries {
			if !e.Provisional {
				kept = append(kept, e)
			}
		}
		clear(c.Entries[len(kept):])
		c.Entries = kept
	}
}
