package talk

import (
	"context"
	"strings"
	"testing"
)

func TestStreamingParagraphsAndFinalReconciliation(t *testing.T) {
	var l ConversationLog
	l.Begin(1)
	l.Delta(1, "user", "Hello")
	l.Delta(1, "user", " world")
	l.Delta(1, "assistant", "Yes")
	l.Delta(1, "user", " again")
	es := l.Snapshot()[0].Entries
	if len(es) != 3 || es[0].Text != "Hello world" || !es[2].Provisional {
		t.Fatalf("paragraphs: %#v", es)
	}
	l.Append(1, "user", "Hello world again", "u")
	es = l.Snapshot()[0].Entries
	if len(es) != 2 || es[0].Provisional || es[0].Text != "Hello world again" || es[1].Role != "assistant" {
		t.Fatalf("final: %#v", es)
	}
	l.Append(1, "user", "duplicate", "u")
	if len(l.Snapshot()[0].Entries) != 2 {
		t.Fatal("duplicate final")
	}
	l.DropPending(1)
	es = l.Snapshot()[0].Entries
	if len(es) != 1 || es[0].Provisional {
		t.Fatal("pending survived closure")
	}
	l.Begin(2)
	l.Delta(1, "user", "stale")
	l.Delta(2, "assistant", strings.Repeat("ü", 5000))
	l.Delta(2, "assistant", "overflow")
	if len([]rune(l.Snapshot()[1].Entries[0].Text)) != 4096 {
		t.Fatal("unbounded")
	}
	l.Remove(2)
	l.Delta(2, "user", "late")
	if len(l.Snapshot()) != 1 {
		t.Fatal("resurrected deleted call")
	}
}
func TestPartialsNeverPublishHistory(t *testing.T) {
	s := NewSession(nil, "test", nil)
	var deltas []string
	s.PartialCaption = func(role, delta string) { deltas = append(deltas, role+":"+delta) }
	s.partial("assistant", "first")
	s.partial("assistant", " second")
	s.partial("tool", "hidden")
	if len(deltas) != 2 || len(s.native.transcripts) != 0 {
		t.Fatal("partial entered history queue")
	}
	cleared := false
	s.ClearPartial = func() { cleared = true }
	s.clearPartial()
	if !cleared {
		t.Fatal("missing cleanup")
	}
}

func TestNativeLiveDeltasAndClosure(t *testing.T) {
	d := &nativeDevice{fakeDevice: fakeDevice{frames: make(chan []int16, 8)}}
	s := NewSession(nil, "synthetic", d)
	var l ConversationLog
	l.Begin(1)
	s.PartialCaption = func(role, text string) { l.Delta(1, role, text) }
	s.Caption = func(role, text, id string) { l.Append(1, role, text, id) }
	s.ClearPartial = func() { l.DropPending(1) }
	ctx, cancel := context.WithCancel(context.Background())
	go s.nativeLoop(ctx)
	defer func() { cancel(); <-s.native.done }()
	emit(s, `{"type":"session.input_transcript.delta","delta":"synthetic "}`)
	emit(s, `{"type":"session.input_transcript.delta","delta":"question"}`)
	emit(s, `{"type":"session.output_transcript.delta","delta":"synthetic answer"}`)
	eventually(t, func() bool { return s.Diagnostics().OutputDeltas == 1 })
	es := l.Snapshot()[0].Entries
	if len(es) != 2 || es[0].Text != "synthetic question" || !es[1].Provisional {
		t.Fatalf("not live: %#v", es)
	}
	if len(s.native.transcripts) != 0 {
		t.Fatal("partial persisted")
	}
	emit(s, `{"type":"turn.done","turn":{"role":"user","id":"q","transcript":"synthetic question"}}`)
	emit(s, `{"type":"turn.done","turn":{"role":"assistant","id":"a","transcript":"synthetic answer"}}`)
	eventually(t, func() bool { return len(s.native.transcripts) == 2 })
	es = l.Snapshot()[0].Entries
	if len(es) != 2 || es[0].Provisional || es[1].Provisional {
		t.Fatal("final duplication")
	}
	emit(s, `{"type":"session.output_transcript.delta","delta":"unfinished"}`)
	eventually(t, func() bool { return s.Diagnostics().OutputDeltas == 2 })
	emit(s, `{"type":"output_audio_buffer.cleared"}`)
	eventually(t, func() bool { return len(l.Snapshot()[0].Entries) == 2 })
}
