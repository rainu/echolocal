//go:build !board_biscuit

package settings

import (
	"strings"
	"sync"
	"testing"

	"github.com/ygelfand/echolocal/internal/feature/microphone"
	testfeature "github.com/ygelfand/echolocal/internal/feature/mictest"
	core "github.com/ygelfand/echolocal/internal/mictest"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	setting "github.com/ygelfand/libcountertop/pkg/settings"
)

type fakeMicTest struct {
	mu    sync.Mutex
	state core.State
	owner string
	calls []string
}

func (f *fakeMicTest) State(owner string) core.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.state
	if s.Phase == "" {
		s.Phase = "idle"
	}
	s.Owned = f.owner != "" && f.owner == owner
	return s
}
func (f *fakeMicTest) Command(owner, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, action)
	switch action {
	case "record":
		f.owner = owner
		f.state.Phase = "recording"
		f.state.Seconds = 0.5
	case "play":
		f.state.Phase = "playing"
	case "stop":
		f.state.Phase = "ready"
	case "discard":
		f.state = core.State{Phase: "idle"}
		f.owner = ""
	}
	return nil
}
func TestMicrophoneControlsAndTestShareOneConcretePage(t *testing.T) {
	m := &fakeMicTest{}
	sh := sharedshell.New()
	page := microphonePageWith(m, sh)
	var view sharedshell.View = page
	if _, ok := view.(*sharedshell.Page); !ok {
		t.Fatal("native renderer requires concrete page")
	}
	if page.OnOpen == nil || len(page.Actions) != 3 {
		t.Fatal("missing lifecycle or footer buttons")
	}
	want := []string{micText("Start recording", "Starten"), micText("Play", "Abspielen"), micText("Stop", "Stoppen")}
	for i, a := range page.Actions {
		if a.Label != want[i] || a.MinHeight < 64 {
			t.Fatal("unexpected footer action", a.Label)
		}
	}
	rows, taps := page.Build()
	if len(rows) != len(taps) {
		t.Fatal("row handlers")
	}
	sliders, toggles := 0, 0
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Label), "ram") {
			t.Fatal("RAM help must not appear on microphone settings")
		}
		if r.Kind == menu.Slider {
			sliders++
		}
		if r.Kind == menu.Toggle {
			toggles++
		}
		if r.Open != nil {
			t.Fatal("microphone test must not open a submenu")
		}
	}
	if sliders != 2 || toggles != 2 {
		t.Fatalf("sliders=%d toggles=%d", sliders, toggles)
	}
	start, play, stop := page.Actions[0], page.Actions[1], page.Actions[2]
	if !start.IsEnabled() || play.IsEnabled() || stop.IsEnabled() {
		t.Fatal("idle action enablement")
	}
	play.Run()
	stop.Run()
	if len(m.calls) != 0 {
		t.Fatal("disabled action dispatched")
	}
	start.Run()
	if start.IsEnabled() || play.IsEnabled() || !stop.IsEnabled() {
		t.Fatal("capture action enablement")
	}
	rows, _ = page.Build()
	for _, r := range rows[:5] {
		if !r.Dim {
			t.Fatal("processing knob enabled during capture")
		}
	}
	stop.Run()
	if !start.IsEnabled() || !play.IsEnabled() || stop.IsEnabled() {
		t.Fatal("ready action enablement")
	}
	play.Run()
	if start.IsEnabled() || play.IsEnabled() || !stop.IsEnabled() {
		t.Fatal("playback action enablement")
	}
	stop.Run()
	start.Run()
	if len(m.calls) != 5 {
		t.Fatal("repeat should record without a delete step", m.calls)
	}
}
func TestLeavingMicrophoneSettingsErasesTakeAndUnhooks(t *testing.T) {
	before := testfeature.Changed.Listeners()
	m := &fakeMicTest{}
	sh := sharedshell.New()
	page := microphonePageWith(m, sh)
	sh.Push(page)
	page.Actions[0].Run()
	page.Actions[2].Run()
	if testfeature.Changed.Listeners() != before+1 {
		t.Fatal("missing redraw subscription")
	}
	sh.Push(&sharedshell.Page{Title: "picker"})
	if m.State("").Phase != "idle" || testfeature.Changed.Listeners() != before {
		t.Fatal("leaving did not clear/unhook")
	}
	sh.Pop()
	if testfeature.Changed.Listeners() != before+1 {
		t.Fatal("return did not resubscribe")
	}
	page.Actions[0].Run()
	sh.Pop()
	if m.State("").Phase != "idle" || testfeature.Changed.Listeners() != before {
		t.Fatal("back did not clear/unhook")
	}
}

func TestMicrophoneNumericKnobsPreserveEveryStep(t *testing.T) {
	rows, _ := microphoneRows(sharedshell.New(), func() bool { return false })
	fields := microphone.Table(microphone.Mixings()).Rows()
	for i, f := range fields {
		if f.Kind != setting.Number {
			continue
		}
		for raw := f.Min; raw <= f.Max; raw++ {
			pct := f.Percent(raw)
			if got := rows[i].Snap(pct); got != pct {
				t.Fatalf("%s raw %d: percent %d snapped to %d", f.Name, raw, pct, got)
			}
			if f.Raw(pct) != raw {
				t.Fatalf("%s lost raw step %d", f.Name, raw)
			}
		}
	}
}
