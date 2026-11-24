package avatarlab

import (
	"github.com/ygelfand/echolocal/internal/config"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFullscreenIndependentPersistedComposition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	config.Use(path)
	table := NewTable()
	a := config.Avatar{Zoom: 230, Yaw: 20, Pitch: 10, X: 30, Y: 40, Background: "split.png"}
	if a.Mode() != "split_screen" || a.Composition(true).Zoom != 100 {
		t.Fatal("migration default")
	}
	for _, row := range table.In("fullscreen") {
		if row.Name == "talk_mode" {
			if row.Write(&a, "fullscreen") != nil {
				t.Fatal("default write")
			}
			continue
		}
		if row.Name == "fullscreen_background" {
			continue
		}
		before := a
		for _, bad := range []string{strconv.Itoa(row.Min - 1), strconv.Itoa(row.Max + 1), "NaN", "1.5"} {
			if row.Write(&a, bad) == nil || a != before {
				t.Fatal("invalid write", row.Name)
			}
		}
		if row.Write(&a, strconv.Itoa(row.Max)) != nil {
			t.Fatal("range", row.Name)
		}
	}
	split := a.Composition(false)
	if e := config.AvatarSection.Update(func(v *config.Avatar) { *v = a }); e != nil {
		t.Fatal(e)
	}
	o := Options(a.Composition(true))
	o.Zoom = 1.75
	o.Yaw = -30
	o.PanY = -.5
	if e := SaveComposition(o, true); e != nil {
		t.Fatal(e)
	}
	config.Use(path)
	got := config.AvatarSection.Read()
	if got.Composition(false).Zoom != split.Zoom || got.Yaw != 20 || got.Pitch != 10 || got.X != 30 || got.Y != 40 || got.Background != "split.png" {
		t.Fatal("fullscreen overwrote split", got)
	}
	if got.Mode() != "fullscreen" || got.FullscreenZoom != 175 || got.FullscreenYaw != -30 || got.FullscreenY != -50 {
		t.Fatal("persistence", got)
	}
	before := got
	if table.MustRow("talk_mode").Write(&got, "invalid") == nil || got != before {
		t.Fatal("invalid mode")
	}
	o = Options(got)
	o.Zoom = 2.5
	if e := SaveComposition(o, false); e != nil {
		t.Fatal(e)
	}
	got = config.AvatarSection.Read()
	if got.FullscreenZoom != 175 {
		t.Fatal("split overwrote fullscreen")
	}
}
