package clawtalk

import (
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
