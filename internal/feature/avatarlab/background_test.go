package avatarlab

import (
	"github.com/ygelfand/echolocal/internal/config"
	"github.com/ygelfand/libcountertop/pkg/say"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestBackgroundSettingPreservesPresentationAndPersists(t *testing.T) {
	root := t.TempDir()
	config.Use(filepath.Join(root, "settings.json"))
	p := filepath.Join(root, "image.png")
	f, _ := os.Create(p)
	png.Encode(f, image.NewGray(image.Rect(0, 0, 4, 8)))
	f.Close()
	table := NewTable()
	row := table.MustRow("background")
	a := config.Avatar{Zoom: 999, Yaw: 30, Pitch: 20, X: 250, Y: 400, IdleDisabled: true, Path: "/avatar.vrm"}
	before := a
	if e := row.Write(&a, p); e != nil {
		t.Fatal(e)
	}
	if a.Background != p {
		t.Fatal(a)
	}
	a.Background = ""
	if a != before {
		t.Fatal("selection changed framing")
	}
	row.Write(&a, p)
	before = a
	if row.Write(&a, "/missing.png") == nil || a != before {
		t.Fatal("invalid selection changed state")
	}
	config.AvatarSection.Update(func(c *config.Avatar) { *c = a })
	if e := SavePresentation(Options(a)); e != nil {
		t.Fatal(e)
	}
	config.Use(filepath.Join(root, "settings.json"))
	if config.AvatarSection.Read() != a {
		t.Fatal("roundtrip")
	}
	if e := row.Write(&a, ""); e != nil {
		t.Fatal(e)
	}
	a.Background = p
	if a != before {
		t.Fatal("clear changed framing")
	}
}

func TestBackgroundPickerMetadataIsLocalizedAndUsesDeviceDirectory(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		c := &setting.Controls[config.Avatar]{Table: NewTable(), Read: func() config.Avatar { return config.Avatar{Background: "/images/saved.png"} }}
		for _, field := range c.Fields() {
			if field.Name != "background" {
				continue
			}
			if field.FilePicker == nil || field.FilePicker.StartDir != "/images" || len(field.FilePicker.Extensions) != 3 || field.Help != "" {
				t.Fatal(field)
			}
			for key, text := range field.FilePicker.Text {
				if text == "" {
					t.Fatal(lang, key)
				}
			}
			if len(field.FilePicker.Text) != 14 {
				t.Fatal(field.FilePicker)
			}
		}
	}
}
