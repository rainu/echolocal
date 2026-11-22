package config

import (
	"path/filepath"
	"testing"
)

func TestAvatarSelectionPersistsAndEmptyRestoresDefault(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	if s.Get().Avatar.Path != "" {
		t.Fatal("default changed")
	}
	if e = s.Update(func(c *Config) { c.Avatar.Path = "/data/local/tmp/Custom.vrm" }); e != nil {
		t.Fatal(e)
	}
	s, e = Load(p)
	if e != nil || s.Get().Avatar.Path != "/data/local/tmp/Custom.vrm" {
		t.Fatal("persistence", e)
	}
	s.Update(func(c *Config) { c.Avatar.Path = "" })
	s, e = Load(p)
	if e != nil || s.Get().Avatar.Path != "" {
		t.Fatal("reset", e)
	}
}

func TestAvatarPresentationPersistsAndBoundsLegacyStorage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s, e := Load(p)
	if e != nil {
		t.Fatal(e)
	}
	want := Avatar{Path: "/model.vrm", Zoom: 175, Yaw: -45, Pitch: 20, X: 15, Y: -30}
	if e = s.Update(func(c *Config) { c.Avatar = want }); e != nil {
		t.Fatal(e)
	}
	s, e = Load(p)
	if e != nil || s.Get().Avatar != want {
		t.Fatal(e, s.Get().Avatar)
	}
	b := Avatar{Zoom: 999, Yaw: -999, Pitch: 999, X: -999, Y: 999}.Presentation()
	if b.Zoom != 300 || b.Yaw != -180 || b.Pitch != 85 || b.X != -100 || b.Y != 100 {
		t.Fatal(b)
	}
	if (Avatar{}).Presentation().Zoom != 100 {
		t.Fatal("legacy zoom")
	}
}
