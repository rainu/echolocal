package avatarlab

import (
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
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
	if o.Zoom != 3 || o.Yaw != 180 || o.Pitch != 85 || o.PanX != 1 || o.PanY != 1 {
		t.Fatal(o)
	}
	if a.Path != "/model.vrm" {
		t.Fatal("presentation touched selection")
	}
}
