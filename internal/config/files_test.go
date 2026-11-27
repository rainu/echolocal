package config

import (
	"github.com/ygelfand/libcountertop/pkg/settings/storage"
	"path/filepath"
	"testing"
)

func TestFilesPreferencesPersistAndMigrateWithoutTouchingPickerSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	defaults := s.Get().Files.Presentation()
	if defaults.View != "list" || defaults.Tile != 192 || defaults.Sort != "name" || defaults.Hidden {
		t.Fatal("legacy defaults")
	}
	before := s.Get().Avatar
	binding := storage.Field(s.Store, func(c *Config) *Files { return &c.Files })
	want := Files{View: "grid", Tile: 256, Sort: "modified", Descending: true, Hidden: true}
	if e = binding.Update(func(c *Files) { *c = want }); e != nil {
		t.Fatal(e)
	}
	s, e = Load(path)
	if e != nil || s.Get().Files != want {
		t.Fatal("persistence", e)
	}
	if s.Get().Avatar != before {
		t.Fatal("picker settings changed")
	}
	bad := Files{View: "bad", Sort: "bad", Tile: 9999}.Presentation()
	if bad != defaults {
		t.Fatal("invalid preferences", bad)
	}
}
