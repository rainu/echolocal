package clawtalk

import (
	"github.com/ygelfand/libcountertop/pkg/say"
	"os"
	"path/filepath"
	"testing"
)

func TestSlotRoutingDefaultsAndContextPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")
	s := load(path)
	if s.Get(0).OpenClaw || s.Get(1).OpenClaw {
		t.Fatal("HA must remain the default")
	}
	if e := s.Update(0, func(v *Slot) { v.OpenClaw = true; v.Agent = "synthetic"; v.Resume = true; v.Timeout = 30 }); e != nil {
		t.Fatal(e)
	}
	a, e := s.Target(0)
	if e != nil {
		t.Fatal(e)
	}
	b, e := load(path).Target(0)
	if e != nil || a.Context != b.Context {
		t.Fatal("resume lost context across restart")
	}
	if s.Get(1).OpenClaw {
		t.Fatal("sibling slot changed")
	}
	if e = s.Update(0, func(v *Slot) { v.Resume = false }); e != nil {
		t.Fatal(e)
	}
	c, e := s.Target(0)
	if e != nil || c.Context == a.Context {
		t.Fatal("fresh policy reused context")
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("routing storage permissions")
	}
	s.Update(0, func(v *Slot) { v.Resume = true; v.Agent = "another" })
	if _, e = s.Target(0); e == nil {
		t.Fatal("wrong-agent context accepted")
	}
}
func TestCorruptRoutingFailsClosedWithoutHAFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing.json")
	os.WriteFile(path, []byte("invalid"), 0600)
	s := load(path)
	for n := 0; n < 2; n++ {
		if !s.Get(n).OpenClaw {
			t.Fatal("corrupt routing silently selected HA")
		}
		if _, e := s.Target(n); e == nil {
			t.Fatal("corrupt route activated")
		}
	}
	if e := s.Update(0, func(*Slot) {}); e == nil {
		t.Fatal("corrupt file overwritten")
	}
}

func TestVoiceModelChoicePersistsIndependentlyPerSlot(t *testing.T) {
	path := t.TempDir() + "/routing.json"
	s := load(path)
	if e := s.Update(0, func(v *Slot) { v.Provider = "synthetic-provider"; v.Model = "future-model-2099" }); e != nil {
		t.Fatal(e)
	}
	again := load(path)
	if again.Get(0).Model != "future-model-2099" || again.Get(1).Model != "" {
		t.Fatal("model selection lost or shared between slots")
	}
}

func TestTalkFeedbackPersistsWithoutChangingOtherSlot(t *testing.T) {
	path := t.TempDir() + "/routing.json"
	s := load(path)
	if e := s.Update(0, func(v *Slot) { v.StartTone = "none"; v.EndTone = "ding" }); e != nil {
		t.Fatal(e)
	}
	again := load(path)
	if again.Get(0).StartTone != "none" || again.Get(0).EndTone != "ding" || again.Get(1).StartTone != "" || again.Get(1).EndTone != "" {
		t.Fatal("feedback lost or changed another slot")
	}
}

func TestDeviceContextTemplateIsAtomicPersistentAndPerSlot(t *testing.T) {
	path := t.TempDir() + "/routes.json"
	s := load(path)
	if s.Get(0).DeviceContextTemplate != "" {
		t.Fatal("legacy settings lost default")
	}
	template := "Use {{node_id}} for {{device_name}}"
	if err := s.Update(0, func(v *Slot) { v.DeviceContextTemplate = template }); err != nil {
		t.Fatal(err)
	}
	if load(path).Get(0).DeviceContextTemplate != template || load(path).Get(1).DeviceContextTemplate != "" {
		t.Fatal("template lost or sibling changed")
	}
	if err := s.Update(0, func(v *Slot) { v.DeviceContextTemplate = "{{unknown}}" }); err == nil {
		t.Fatal("bad template persisted")
	}
	if load(path).Get(0).DeviceContextTemplate != template {
		t.Fatal("failed save changed previous template")
	}
	if err := s.Update(0, func(v *Slot) { v.DeviceContextTemplate = "" }); err != nil || load(path).Get(0).DeviceContextTemplate != "" {
		t.Fatal("reset did not persist")
	}
}

func TestEndSettingsMigrationPersistenceAndValidation(t *testing.T) {
	path := t.TempDir() + "/settings.json"
	os.WriteFile(path, []byte(`[{"openclaw":true,"agent":"synthetic","timeout":30},{}]`), 0600)
	s := load(path)
	if s.Get(0).Ending.Defaults("de").Phrase != "Gespräch beenden" {
		t.Fatal("missing migration default")
	}
	if err := s.Update(0, func(v *Slot) {
		v.Ending.Phrase = "My stop phrase"
		v.Ending.Position = "end"
		v.Ending.Timing = "immediate"
	}); err != nil {
		t.Fatal(err)
	}
	again := load(path)
	end := again.Get(0).Ending
	if end.Phrase != "My stop phrase" || end.Position != "end" || end.Timing != "immediate" || again.Get(0).Agent != "synthetic" || again.Get(1).Ending.Phrase != "" {
		t.Fatal("settings lost/route or sibling changed")
	}
	for _, change := range []func(*Slot){func(v *Slot) { v.Ending.Phrase = "!!!" }, func(v *Slot) { v.Ending.Position = "middle" }, func(v *Slot) { v.Ending.Timing = "later" }} {
		if again.Update(0, change) == nil || again.Get(0).Ending != end || load(path).Get(0).Ending != end {
			t.Fatal("invalid change persisted")
		}
	}
}

func TestEndDefaultIsPinnedAfterFirstSave(t *testing.T) {
	before := say.Chosen()
	defer say.Use(before)
	say.Use("de")
	path := t.TempDir() + "/settings.json"
	s := load(path)
	if err := s.Update(0, func(v *Slot) { v.Timeout = 30 }); err != nil {
		t.Fatal(err)
	}
	say.Use("en")
	if end := load(path).Get(0).Ending; end.Phrase != "Gespräch beenden" || end.Position != "exact" || end.Timing != "turn_end" {
		t.Fatal("saved default followed later language change")
	}
}
