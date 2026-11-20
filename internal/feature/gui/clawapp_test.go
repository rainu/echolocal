//go:build !board_biscuit

package gui

import (
	"github.com/ygelfand/echolocal/internal/openclaw/chatapp"
	"testing"
)

func TestChatAppHasNoSettingsTimeoutAndClosesItsController(t *testing.T) {
	a := chatapp.New(nil, func() {}, nil, nil, nil)
	v := &clawAppView{app: a}
	if !v.Covers() || v.Timeout() != 0 {
		t.Fatal("not a dedicated application")
	}
	// Closing a view which never started capture must be harmless.
	v.Close()
}
