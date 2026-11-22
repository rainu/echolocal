//go:build !board_biscuit

package settings

import (
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
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

func TestAvatarNativeMotionTogglesAndReset(t *testing.T) {
	value := config.Avatar{Path: "/model.vrm", Zoom: 150}
	c := &setting.Controls[config.Avatar]{Table: avatarlab.NewTable(), Read: func() config.Avatar { return value }, Save: func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) }}
	page := avatarPresentationPageFor(c)
	rows, acts := page.Build()
	if len(rows) != 9 {
		t.Fatal("missing presentation controls", len(rows))
	}
	for i, name := range []string{"idle", "blink", "animation"} {
		index := 5 + i
		if rows[index].Kind != menu.Toggle || !rows[index].On {
			t.Fatal("native toggle", name)
		}
		acts[index](0)
		a := value
		if c.Table.MustRow(name).Read(&a) != "off" {
			t.Fatal("native toggle not saved", name)
		}
	}
	rows, acts = page.Build()
	for i := 5; i < 8; i++ {
		if rows[i].On {
			t.Fatal("off state not restored")
		}
	}
	// A settings reset restores framing and motion defaults, not model selection.
	acts[len(acts)-1](0)
	o := avatarlab.Options(value)
	if !o.Idle || !o.Blink || !o.Animation || o.Zoom != 1 || value.Path != "/model.vrm" {
		t.Fatal("reset", value)
	}
}

func TestNativeCloseUpZoomSaveAndValidation(t *testing.T) {
	value := config.Avatar{Path: "/model.vrm", Yaw: 20, Y: 75}
	c := &setting.Controls[config.Avatar]{Table: avatarlab.NewTable(), Read: func() config.Avatar { return value }, Save: func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) }}
	page := avatarPresentationPageFor(c)
	for _, zoom := range []string{"300", "500", "999"} {
		rows, _ := page.Build()
		rows[0].Save(zoom)
		rows, _ = page.Build()
		if rows[0].Value != zoom || value.Yaw != 20 || value.Y != 75 {
			t.Fatal("native framing", value)
		}
	}
	before := value
	for _, bad := range []string{"49", "1000", "NaN", "999.1"} {
		rows, _ := page.Build()
		rows[0].Save(bad)
		rows, _ = page.Build()
		if value != before || len(rows) != 10 || rows[9].Label == "" {
			t.Fatal("invalid native zoom", bad, value)
		}
	}
}
