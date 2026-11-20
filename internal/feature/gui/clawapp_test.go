//go:build !board_biscuit

package gui

import (
	"encoding/json"
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/openclaw/chatapp"
	"github.com/ygelfand/echolocal/internal/openclaw/talk"
	"strings"
	"testing"
	"time"
)

func TestChatAppHasNoSettingsTimeoutAndClosesItsController(t *testing.T) {
	a := chatapp.New(nil, func() {}, nil, nil, nil)
	v := &clawAppView{app: a}
	if !v.Covers() || v.Timeout() <= 0 {
		t.Fatal("not a dedicated application")
	}
	// Closing a view which never started capture must be harmless.
	v.Close()
}

func TestChatMarkdownMaterializesLegacyThemeFontRoles(t *testing.T) {
	s := appMarkdownStyle()
	for _, size := range []float32{s.Text.Size, s.Bold.Size, s.Italic.Size, s.BoldItalic.Size, s.Code.Size, s.H1.Size, s.H2.Size, s.H3.Size, s.H4.Size, s.H5.Size, s.H6.Size} {
		if size < 18 {
			t.Fatal("invisible document role", size)
		}
	}
}

// Exercise the parser-to-native adapter: replacing block rendering alone still
// leaves the upstream document clipboard toolbar in the generated tree.
func TestChatNativeMarkdownHasContentWithoutClipboardControls(t *testing.T) {
	w := &gogui.Window{}
	v := &clawAppView{}
	l := v.markdown(w, "document", "# Heading\n\nReadable **bold** body.\n\n```go\ncode()\n```\n\n- item").GenerateLayout(w)
	var texts []string
	var walk func(gogui.Layout)
	walk = func(n gogui.Layout) {
		if n.Shape != nil {
			if strings.Contains(n.Shape.ID, "copy") {
				t.Fatal("clipboard control leaked into native chat", n.Shape.ID)
			}
			if n.Shape.TC != nil {
				texts = append(texts, n.Shape.TC.Text)
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(l)
	text := strings.Join(texts, " ")
	for _, want := range []string{"Heading", "Readable", "bold", "code()", "item"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing rendered content %q", want)
		}
	}
}

func TestChatSessionEllipsisFitsMeasuredWidth(t *testing.T) {
	measure := func(s string) float32 {
		width := float32(0)
		for _, r := range s {
			switch {
			case r == '.':
				width += 2
			case r > 127:
				width += 14
			default:
				width += 7
			}
		}
		return width
	}
	for _, tt := range []struct {
		text  string
		width float32
		want  string
	}{
		{"Short", 35, "Short"},
		{"A very long session title", 55, "A very..."},
		{"日本語の長い題名", 48, "日本語..."},
		{"Title", 5, ""},
		{"Title", 6, "..."},
		{"one\ntwo", 100, "one two"},
	} {
		got := appEllipsize(tt.text, tt.width, measure)
		if got != tt.want || measure(got) > tt.width {
			t.Errorf("%q width %v: got %q, want %q", tt.text, tt.width, got, tt.want)
		}
	}
}

func TestChatHistoryBeforeLiveCaptionsAndTimestampLabels(t *testing.T) {
	stamp := time.Date(2026, 11, 20, 12, 34, 0, 0, time.UTC)
	history := []chatapp.Message{{Role: "user", Content: json.RawMessage(`"old"`)}, {Role: "tool", Content: json.RawMessage(`"hidden"`)}, {Role: "assistant", Content: json.RawMessage(`"reply"`)}}
	entries := appMessageEntries(history, []talk.Caption{{Role: "user", Text: "new speech", Timestamp: stamp}, {Role: "assistant", Text: "live reply", Timestamp: stamp}})
	if len(entries) != 4 {
		t.Fatalf("entries: %v", entries)
	}
	for i, want := range []string{"old", "reply", "new speech", "live reply"} {
		if entries[i].text != want {
			t.Fatalf("entry %d: %q", i, entries[i].text)
		}
	}
	loc := time.FixedZone("device", 3600)
	if got := appMessageLabel("user", stamp, loc); !strings.HasSuffix(got, "20.11.2026 13:34") {
		t.Fatal(got)
	}
	if got := appMessageLabel("user", time.Time{}, loc); got != appText("user") {
		t.Fatal(got)
	}
}

func TestSessionStatusColorsAndWorkingPriority(t *testing.T) {
	normal := gogui.Hex(0xEEEEEE)
	if appSessionColor(chatapp.Session{}, false, normal) != normal {
		t.Fatal("ordinary row changed")
	}
	unread := appSessionColor(chatapp.Session{Unread: true}, false, normal)
	active := appSessionColor(chatapp.Session{HasActiveRun: true}, false, normal)
	if unread == normal || active == normal || active == unread {
		t.Fatal("statuses indistinguishable")
	}
	if appSessionColor(chatapp.Session{HasActiveRun: true, Unread: true}, false, normal) != active {
		t.Fatal("active must win")
	}
	if appSessionColor(chatapp.Session{Unread: true}, true, normal) != active {
		t.Fatal("history active status lost")
	}
	if appSessionColor(chatapp.Session{Unread: true}, false, normal) != unread {
		t.Fatal("unread lost when work stops")
	}
}
