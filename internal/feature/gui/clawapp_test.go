//go:build !board_biscuit

package gui

import (
	"github.com/ygelfand/echolocal/internal/openclaw/chatapp"
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
