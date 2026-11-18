package talk

import (
	"strings"
	"testing"
)

func TestCaptionCacheBoundedIsolatedAndCopied(t *testing.T) {
	var log ConversationLog
	for gen := uint64(1); gen <= 8; gen++ {
		log.Begin(gen)
		for i := 0; i < 40; i++ {
			log.Append(gen, "user", "synthetic text", string(rune('A'+i)))
		}
	}
	calls := log.Snapshot()
	if len(calls) != 6 || calls[0].Generation != 3 || len(calls[5].Entries) != 32 {
		t.Fatal("cache bound failed")
	}
	log.Append(7, "user", "retired", "old")
	log.Append(8, "tool", "hidden", "tool")
	log.Append(8, "user", "duplicate", calls[5].Entries[0].ID)
	if len(log.Snapshot()[5].Entries) != 32 || log.Snapshot()[5].Entries[31].Text != "synthetic text" {
		t.Fatal("old/invalid/duplicate caption accepted")
	}
	calls[5].Entries[0].Text = "changed"
	if log.Snapshot()[5].Entries[0].Text == "changed" {
		t.Fatal("mutable snapshot")
	}
	log.Append(8, "assistant", strings.Repeat("ü", 5000), "long")
	if len([]rune(log.Snapshot()[5].Entries[31].Text)) != 4097 {
		t.Fatal("caption text unbounded")
	}
}
