package avatarlab

import (
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestPresentationRangesAndSharedOptions(t *testing.T) {
	table := NewTable()
	a := config.Avatar{Path: "/model.vrm"}
	if table.MustRow("zoom").Read(&a) != "100" {
		t.Fatal("legacy default")
	}
	for _, s := range table.In("presentation") {
		if s.Name == "idle" || s.Name == "blink" || s.Name == "animation" {
			continue
		}
		for _, n := range []int{s.Min, s.Max} {
			if err := s.Write(&a, strconv.Itoa(n)); err != nil {
				t.Fatal(s.Name, err)
			}
			if s.Read(&a) != strconv.Itoa(n) {
				t.Fatal(s.Name, a)
			}
		}
		before := a
		for _, bad := range []string{strconv.Itoa(s.Min - 1), strconv.Itoa(s.Max + 1), "NaN", "1.5", "Inf"} {
			if s.Write(&a, bad) == nil || a != before {
				t.Fatal(s.Name, bad)
			}
		}
		for _, lang := range []string{"en", "de", "it", "vi"} {
			if avatar.Text(lang, s.Name) == "" || avatar.Text(lang, s.Name+"_help") == "" {
				t.Fatal("translation", lang, s.Name)
			}
		}
	}
	o := Options(a)
	if o.Zoom != 9.99 || o.Yaw != 180 || o.Pitch != 85 || o.PanX != 5 || o.PanY != 5 {
		t.Fatal(o)
	}
	if a.Path != "/model.vrm" {
		t.Fatal("presentation touched selection")
	}
}

func TestMotionSettingsRoundTripAndSave(t *testing.T) {
	table := NewTable()
	for mask := 0; mask < 8; mask++ {
		a := config.Avatar{Path: "/model.vrm", Zoom: 125, Yaw: 15}
		for bit, name := range []string{"idle", "blink", "animation"} {
			row := table.MustRow(name)
			if row.Read(&a) != "on" {
				t.Fatal("legacy default", name)
			}
			value := "off"
			if mask&(1<<bit) != 0 {
				value = "on"
			}
			if err := row.Write(&a, value); err != nil || row.Read(&a) != value {
				t.Fatal(name, err)
			}
			before := a
			if row.Write(&a, "invalid") == nil || a != before {
				t.Fatal("invalid toggle accepted", name)
			}
			for _, lang := range []string{"en", "de", "it", "vi"} {
				if avatar.Text(lang, name) == "" || avatar.Text(lang, name+"_help") == "" {
					t.Fatal("translation", lang, name)
				}
			}
		}
		o := Options(a)
		if o.Idle != (mask&1 != 0) || o.Blink != (mask&2 != 0) || o.Animation != (mask&4 != 0) {
			t.Fatal(mask, o)
		}
		saved := config.Avatar{Path: a.Path}
		applyPresentation(&saved, o)
		if !reflect.DeepEqual(saved, a) {
			t.Fatal("save lost motion or framing", mask, saved, a)
		}
	}
}

func TestSavedMotionReloadedByLabAndTalk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	config.Use(path)
	config.AvatarSection.Update(func(a *config.Avatar) { a.Path = "/model.vrm" })
	for mask := 0; mask < 8; mask++ {
		preview := avatar.Options{Zoom: 1.25, Yaw: 15, Idle: mask&1 != 0, Blink: mask&2 != 0, Animation: mask&4 != 0}
		if err := SavePresentation(preview); err != nil {
			t.Fatal(err)
		}
		config.Use(path)
		got := Options(config.AvatarSection.Read())
		if got.Idle != preview.Idle || got.Blink != preview.Blink || got.Animation != preview.Animation || got.Zoom != preview.Zoom || got.Yaw != preview.Yaw {
			t.Fatal("restart lost saved preview", mask, got)
		}
		if config.AvatarSection.Read().Path != "/model.vrm" {
			t.Fatal("save changed model")
		}
	}
}

func TestCloseUpZoomSaveReloadAndSharedRenderer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "avatar.json")
	config.Use(path)
	table := NewTable()
	row := table.MustRow("zoom")
	if row.Min != 50 || row.Max != 999 {
		t.Fatal("API metadata range", row)
	}
	for _, percent := range []int{300, 500, 999} {
		a := config.Avatar{Path: "/model.vrm", Yaw: 30, Pitch: -15, X: -120, Y: 400, BlinkDisabled: true}
		if err := row.Write(&a, strconv.Itoa(percent)); err != nil {
			t.Fatal(err)
		}
		preview := Options(a)
		if err := config.AvatarSection.Update(func(saved *config.Avatar) { *saved = a }); err != nil {
			t.Fatal(err)
		}
		if err := SavePresentation(preview); err != nil {
			t.Fatal(err)
		}
		config.Use(path)
		saved := config.AvatarSection.Read()
		if !reflect.DeepEqual(saved, a) || row.Read(&saved) != strconv.Itoa(percent) {
			t.Fatal("save/reload", saved, a)
		}
		c := avatar.New(t.TempDir(), nil, func() bool { return false }, func() {})
		c.Configure(Options(saved))
		if got := c.Snapshot().Options; got != preview {
			t.Fatal("runtime clamped framing", got, preview)
		}
		for _, bad := range []string{"49", "1000", "999.1", "NaN", "+Inf", "-1"} {
			if row.Write(&saved, bad) == nil || !reflect.DeepEqual(saved, a) {
				t.Fatal("invalid changed stored framing", bad, saved)
			}
		}
	}
}

func TestMouthIntensityMigrationZeroRestartAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	config.Use(path)
	row := NewTable().MustRow("mouth_intensity")
	a := config.Avatar{}
	if row.Read(&a) != "50" || Options(a).MouthIntensity != .5 {
		t.Fatal("legacy default")
	}
	for _, n := range []int{0, 10, 50, 100} {
		if err := row.Write(&a, strconv.Itoa(n)); err != nil {
			t.Fatal(err)
		}
		if err := config.AvatarSection.Update(func(v *config.Avatar) { *v = a }); err != nil {
			t.Fatal(err)
		}
		config.Use(path)
		saved := config.AvatarSection.Read()
		if saved.MouthIntensity == nil || *saved.MouthIntensity != n || Options(saved).MouthIntensity != float64(n)/100 {
			t.Fatal("restart", saved)
		}
		if err := SavePresentation(Options(saved)); err != nil {
			t.Fatal(err)
		}
		config.Use(path)
		saved = config.AvatarSection.Read()
		if row.Read(&saved) != strconv.Itoa(n) {
			t.Fatal("lab save")
		}
	}
	defaults := config.Avatar{}
	if err := row.Write(&a, row.Read(&defaults)); err != nil || a.MouthPercent() != 50 {
		t.Fatal("reset")
	}
	for _, bad := range []string{"-1", "101", "1.5", "NaN", "Inf", ""} {
		before := a
		if row.Write(&a, bad) == nil || a != before {
			t.Fatal("invalid", bad)
		}
	}
}
