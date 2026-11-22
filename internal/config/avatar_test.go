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
