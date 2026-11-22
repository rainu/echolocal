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

func TestAvatarMotionPersistence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for mask := 0; mask < 8; mask++ {
		want := Avatar{Zoom: 125, Yaw: 15, IdleDisabled: mask&1 != 0, BlinkDisabled: mask&2 != 0, AnimationDisabled: mask&4 != 0}
		if err = s.Update(func(c *Config) { c.Avatar = want }); err != nil {
			t.Fatal(err)
		}
		s, err = Load(p)
		if err != nil || s.Get().Avatar != want {
			t.Fatal("motion persistence", mask, err, s.Get().Avatar)
		}
	}
}
