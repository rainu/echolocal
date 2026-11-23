package avatarlab

import (
	"github.com/ygelfand/echolocal/internal/config"
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
