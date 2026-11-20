//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/openclaw/chatapp"
	"strings"
	"testing"
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
