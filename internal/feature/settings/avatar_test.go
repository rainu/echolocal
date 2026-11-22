//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"testing"
)

func TestAvatarNativeEditorReadsSameSettingAndResets(t *testing.T) {
	value := config.Avatar{}
	c := &setting.Controls[config.Avatar]{Table: avatarlab.NewTable(), Read: func() config.Avatar { return value }, Save: func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) }}
	page := avatarModelPageFor(c)
	rows, _ := page.Build()
	rows[0].Save("/data/local/tmp/avatar.vrm")
	if value.Path != "/data/local/tmp/avatar.vrm" {
		t.Fatal(value)
	}
	c.ChangeRaw("path", "/data/local/tmp/other.vrm")
	rows, _ = page.Build()
	if rows[0].Value != value.Path {
		t.Fatal("external update missing")
	}
	rows[0].Save("../bad.vrm")
	rows, _ = page.Build()
	if value.Path != "/data/local/tmp/other.vrm" || len(rows) != 4 || rows[3].Label == "" {
		t.Fatal("invalid input lost selection")
	}
	_, acts := page.Build()
	acts[1](0)
	rows, _ = page.Build()
	if value.Path != "" || rows[0].Value != "" {
		t.Fatal("reset failed")
	}
}
