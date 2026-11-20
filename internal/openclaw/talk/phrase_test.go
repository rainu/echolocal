package talk

import (
	"context"
	"encoding/json"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConfiguredEndMatching(t *testing.T) {
	for _, tc := range []struct {
		text              string
		exact, start, end bool
	}{
		{"Gespräch beenden", true, true, true},
		{"  „GESPRÄCH BEENDEN!“  ", true, true, true},
		{"Gespra\u0308ch  beenden", true, true, true},
		{"Gespräch beenden und danke", false, true, false},
		{"Danke, Gespräch beenden", false, false, true},
		{"Bitte Gespräch beenden und danke", false, false, false},
		{"Gespräch beendende", false, false, false},
		{"Vorgespräch beenden", false, false, false},
		{"Gespräch beenden_weiter", false, false, false},
		{"Vor_Gespräch beenden", false, false, false},
		{"Gespräch nicht beenden", false, false, false},
		{"", false, false, false},
	} {
		for position, want := range map[string]bool{"exact": tc.exact, "start": tc.start, "end": tc.end} {
			s := EndSettings{Phrase: "Gespräch beenden", Position: position, Timing: "turn_end"}
			if s.Matches(tc.text) != want {
				t.Errorf("%s: %q", position, tc.text)
			}
		}
	}
	for _, lang := range []string{"en", "de", "it", "vi", "de-DE", "unknown"} {
		s := (EndSettings{}).Defaults(lang)
		if s.Validate() != nil || !s.Matches(strings.ToUpper(s.Phrase)+"!") || s.Timing != "turn_end" || s.Position != "exact" {
			t.Fatal("invalid default", lang)
		}
	}
	for _, phrase := range []string{"", "...", "_", "\nstop", strings.Repeat("a", 257)} {
		s := EndSettings{Phrase: phrase, Position: "exact", Timing: "turn_end"}
		if s.Validate() == nil {
			t.Fatal("invalid phrase accepted")
		}
	}
}

func TestEndTimingPositionAndRoles(t *testing.T) {
	for _, position := range []string{"exact", "start", "end"} {
		for _, timing := range []string{"immediate", "turn_end"} {
			t.Run(position+"/"+timing, func(t *testing.T) {
				s := NewSession(nil, "synthetic", nil)
				s.Ending = EndSettings{Phrase: "Gespräch beenden", Position: position, Timing: timing}
				var closed atomic.Int32
				s.End = func() { closed.Add(1) }
				if s.userTranscript("assistant", "Gespräch beenden", "", true, true) || s.userTranscript("tool", "Gespräch beenden", "", true, false) {
					t.Fatal("wrong role")
				}
				text := "Gespräch beenden"
				if position == "start" {
					text += " und danke"
				}
				if position == "end" {
					text = "Danke, " + text
				}
				// Fragments alone are not phrases. Only their accumulated utterance matches.
				cut := len(text) / 2
				if s.userTranscript("user", text[:cut], "u", false, false) {
					t.Fatal("fragment matched")
				}
				if got := s.userTranscript("user", text[cut:], "u", false, false); got != (timing == "immediate") {
					t.Fatal("timing ignored")
				}
				if timing == "immediate" {
					eventually(t, func() bool { return closed.Load() == 1 })
				} else if closed.Load() != 0 {
					t.Fatal("interim closed turn-end mode")
				}
				if !s.userTranscript("user", text, "u", true, true) {
					t.Fatal("final missed")
				}
				var wg sync.WaitGroup
				for i := 0; i < 16; i++ {
					wg.Add(1)
					go func() { defer wg.Done(); s.userTranscript("user", text, "u", true, true); s.requestEnd() }()
				}
				wg.Wait()
				eventually(t, func() bool { return closed.Load() == 1 })
			})
		}
	}
}

func TestEndInterimRevisionsAndTurnReset(t *testing.T) {
	s := NewSession(nil, "synthetic", nil)
	s.Ending = EndSettings{Phrase: "stop now", Position: "exact", Timing: "immediate"}
	if s.userTranscript("user", "stop no", "1", false, false) {
		t.Fatal("premature")
	}
	if s.userTranscript("user", "keep talking", "1", true, false) {
		t.Fatal("revision appended")
	}
	if s.userTranscript("user", "stop ", "2", false, false) {
		t.Fatal("new turn")
	}
	if !s.userTranscript("user", "now", "2", false, false) {
		t.Fatal("new turn retained old text")
	}
	s.endTranscript.reset("")
	if s.userTranscript("user", "stop ", "", false, false) {
		t.Fatal("fragment")
	}
	if s.userTranscript("user", "", "", true, false) {
		t.Fatal("empty snapshot")
	}
	if s.userTranscript("user", "now", "", false, false) {
		t.Fatal("empty revision not applied")
	}
	s.userTranscript("user", "keep talking", "", true, true)
	if !s.userTranscript("user", "stop now", "", false, false) {
		t.Fatal("final did not clear cache")
	}
	s.endTranscript.reset("")
	if s.userTranscript("user", strings.Repeat("x", 17000), "", false, false) || s.userTranscript("user", "stop now", "", false, false) {
		t.Fatal("overflow created false match")
	}
	if !s.userTranscript("user", "stop now", "", true, false) {
		t.Fatal("snapshot did not recover")
	}
	s.Ending.Timing = "turn_end"
	if s.userTranscript("user", "stop now", "", true, false) || s.userTranscript("user", "keep talking", "", true, true) {
		t.Fatal("revised interim closed final-only mode")
	}
}

func TestNativeEndFromInterimAndSnapshotRevision(t *testing.T) {
	for _, snapshot := range []bool{false, true} {
		t.Run(map[bool]string{false: "deltas", true: "revision"}[snapshot], func(t *testing.T) {
			d := &nativeDevice{fakeDevice: fakeDevice{frames: make(chan []int16, 8)}}
			s := NewSession(nil, "synthetic", d)
			s.Ending = EndSettings{Phrase: "stop now", Position: "exact", Timing: "immediate"}
			var closed atomic.Int32
			s.End = func() { closed.Add(1) }
			ctx, cancel := context.WithCancel(context.Background())
			go s.nativeLoop(ctx)
			defer func() { cancel(); <-s.native.done }()
			emit(s, `{"type":"session.output_transcript.delta","delta":"stop now"}`)
			emit(s, `{"type":"session.input_transcript.delta","delta":"stop "}`)
			if snapshot {
				emit(s, `{"type":"input_transcript.added","item":{"text":"keep talking"}}`)
				emit(s, `{"type":"input_transcript.added","item":{"text":"stop now"}}`)
			} else {
				emit(s, `{"type":"session.input_transcript.delta","delta":"now"}`)
			}
			eventually(t, func() bool { return closed.Load() == 1 })
			if s.utterances.Load() != 0 {
				t.Fatal("waited for turn.done")
			}
		})
	}
}

func TestGatewayEndTimingAndFinalRevision(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, timing := range []string{"immediate", "turn_end"} {
			s := NewSession(nil, "synthetic", &fakeDevice{})
			s.Ending = EndSettings{Phrase: "stop now", Position: "exact", Timing: timing}
			s.nativeEnabled.Store(native)
			var closed atomic.Int32
			s.End = func() { closed.Add(1) }
			events := make(chan connection.TalkEvent, 16)
			done := make(chan struct{})
			ctx, cancel := context.WithCancel(context.Background())
			barrier := make(chan struct{})
			s.Caption = func(role, text, id string) {
				if role == "user" {
					select {
					case <-barrier:
					default:
						close(barrier)
					}
				}
			}
			go func() { defer close(done); s.control(ctx, events) }()
			send := func(typ, role, text string) {
				var e connection.TalkEvent
				e.VoiceSessionID = s.id
				e.Event.Type = typ
				e.Event.Payload, _ = json.Marshal(map[string]string{"role": role, "text": text})
				events <- e
			}
			send("transcript.done", "assistant", "stop now")
			send("transcript.delta", "user", "stop ")
			send("transcript.updated", "user", "keep talking")
			send("transcript.updated", "user", "stop now")
			send("transcript.done", "user", "keep talking")
			if timing == "immediate" && !native {
				eventually(t, func() bool { return closed.Load() == 1 })
				if s.utterances.Load() != 0 {
					t.Fatal("not immediate")
				}
			} else {
				<-barrier
				if closed.Load() != 0 {
					t.Fatal("assistant/interim/revised final ended session")
				}
				send("transcript.done", "user", "stop now")
				eventually(t, func() bool { return closed.Load() == 1 })
			}
			cancel()
			<-done
		}
	}
}

func TestInvalidEndSettingsRejectBeforeConnecting(t *testing.T) {
	s := NewSession(nil, "synthetic", nil)
	s.Ending = EndSettings{Phrase: "valid", Position: "middle", Timing: "immediate"}
	if s.Open(context.Background()) == nil {
		t.Fatal("invalid settings connected")
	}
}

func TestNativeFinalOnlyUsesConfiguredPhrase(t *testing.T) {
	s, _, _ := nativeHarness(t)
	s.Ending = EndSettings{Phrase: "custom stop", Position: "exact", Timing: "turn_end"}
	var ended atomic.Int32
	s.End = func() { ended.Add(1) }
	emit(s, `{"type":"session.input_transcript.delta","delta":"custom stop"}`)
	emit(s, `{"type":"turn.done","turn":{"id":"u1","role":"user","transcript":"keep talking"}}`)
	emit(s, `{"type":"turn.done","turn":{"id":"a1","role":"assistant","transcript":"custom stop"}}`)
	eventually(t, func() bool { return len(s.native.transcripts) == 2 })
	if ended.Load() != 0 {
		t.Fatal("interim or assistant closed conversation")
	}
	emit(s, `{"type":"turn.done","turn":{"id":"u2","role":"user","transcript":"CUSTOM STOP!"}}`)
	eventually(t, func() bool { return ended.Load() == 1 })
}
