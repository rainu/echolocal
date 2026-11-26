//go:build !board_biscuit

package settings

import (
	"testing"

	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"

	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/echolocal/internal/feature/avatarlab"
)

func TestAvatarNativeEditorReadsSameSettingAndResets(t *testing.T) {
	value := config.Avatar{}
	c := &setting.Controls[config.Avatar]{
		Table: avatarlab.NewTable(),
		Read:  func() config.Avatar { return value },
		Save:  func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) },
	}
	page := avatarModelPageFor(c)
	rows, _ := page.Build()
	rows[0].Save("/data/local/tmp/avatar.vrm")
	if value.Path != "/data/local/tmp/avatar.vrm" {
		t.Fatal(value)
	}
	if err := c.ChangeRaw("path", "/data/local/tmp/other.vrm"); err != nil {
		t.Fatal(err)
	}
	rows, _ = page.Build()
	if rows[0].Hint != value.Path {
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
	if value.Path != "" || rows[0].Hint != "" {
		t.Fatal("reset failed")
	}
}

func TestAvatarNativeMotionTogglesAndReset(t *testing.T) {
	value := config.Avatar{Path: "/model.vrm", Zoom: 150}
	c := &setting.Controls[config.Avatar]{
		Table: avatarlab.NewTable(),
		Read:  func() config.Avatar { return value },
		Save:  func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) },
	}
	page := avatarPresentationPageFor(c)
	rows, acts := page.Build()
	if len(rows) != 10 {
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
	c := &setting.Controls[config.Avatar]{
		Table: avatarlab.NewTable(),
		Read:  func() config.Avatar { return value },
		Save:  func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) },
	}
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
		if value != before || len(rows) != 11 || rows[10].Label == "" {
			t.Fatal("invalid native zoom", bad, value)
		}
	}
}

func TestNativeMouthIntensityEditAndReset(t *testing.T) {
	value := config.Avatar{}
	c := &setting.Controls[config.Avatar]{
		Table: avatarlab.NewTable(),
		Read:  func() config.Avatar { return value },
		Save:  func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) },
	}
	page := avatarPresentationPageFor(c)
	for _, v := range []string{"0", "15", "100"} {
		rows, _ := page.Build()
		rows[8].Save(v)
		rows, _ = page.Build()
		if rows[8].Value != v {
			t.Fatal(rows[8])
		}
	}
	rows, _ := page.Build()
	rows[8].Save("101")
	rows, _ = page.Build()
	if rows[8].Value != "100" || len(rows) != 11 {
		t.Fatal("validation", rows)
	}
	_, acts := page.Build()
	acts[9](0)
	if value.MouthPercent() != 50 {
		t.Fatal("reset", value)
	}
}

func TestAllAvatarAssetRowsUsePickerAndKeepValidation(t *testing.T) {
	value := config.Avatar{Path: "/model.vrm", Zoom: 150}
	c := &setting.Controls[config.Avatar]{
		Table: avatarlab.NewTable(),
		Read:  func() config.Avatar { return value },
		Save:  func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) },
	}
	for _, page := range []*sharedshell.Page{avatarModelPageFor(c), avatarBackgroundPageFor(c), avatarFullscreenPageFor(c)} {
		rows, _ := page.Build()
		count := 0
		for _, row := range rows {
			if row.Open == nil {
				continue
			}
			count++
			if row.Kind != menu.Chevron || row.Value != "" {
				t.Fatal("not a picker", row)
			}
			before := value
			p := sharedshell.SettingsPage(row.Open())
			if p == nil {
				t.Fatal("picker lost settings-page recognition")
			}
			if contents, _ := p.Build(); len(contents) != 0 {
				t.Fatal("constructing picker browsed disk")
			}
			if value != before {
				t.Fatal("opening changed settings")
			}
			row.Save("/missing/not-an-asset.txt")
			if value != before {
				t.Fatal("bypassed existing validation")
			}
		}
		if count != 1 {
			t.Fatal("expected one picker per page", count)
		}
	}
}

func TestFullscreenBackgroundClearPreservesComposition(t *testing.T) {
	value := config.Avatar{
		Path:                 "/model.vrm",
		Background:           "/normal.png",
		FullscreenBackground: "/selected.png",
		FullscreenZoom:       468,
		FullscreenYaw:        -10,
		FullscreenPitch:      1,
		FullscreenX:          -5,
		FullscreenY:          168,
		TalkMode:             "fullscreen",
	}
	before := value
	c := &setting.Controls[config.Avatar]{
		Table: avatarlab.NewTable(),
		Read:  func() config.Avatar { return value },
		Save:  func(s setting.Setting[config.Avatar], v string) error { return s.Write(&value, v) },
	}
	p := avatarFullscreenPageFor(c)
	rows, actions := p.Build()
	found := false
	for i, r := range rows {
		if r.Label == avatar.Text(say.Chosen(), "background_reset") {
			actions[i](0)
			found = true
		}
	}
	if !found {
		t.Fatal("missing per-background clear action")
	}
	before.FullscreenBackground = ""
	if value != before {
		t.Fatal("image clear reset another setting", value, before)
	}
}
