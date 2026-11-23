//go:build !board_biscuit

package clawtalk

import (
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/config"
	"testing"
)

type modeRenderer struct {
	fakeAvatar
	mode    bool
	options avatar.Options
	changes int
}

func (r *modeRenderer) SetTalkMode(full bool)      { r.mode = full; r.changes++ }
func (r *modeRenderer) Configure(o avatar.Options) { r.options = o }
func TestModeSwitchKeepsRendererAndIndependentFraming(t *testing.T) {
	r := &modeRenderer{}
	r.state.Running = true
	p := &AvatarPage{Renderer: r, composition: config.Avatar{Zoom: 150, Yaw: 20, X: 30, FullscreenZoom: 225, FullscreenYaw: -40, FullscreenX: -50}}
	for n := 0; n < 100; n++ {
		p.SwitchMode()
		if !r.mode || r.options.Zoom != 2.25 || r.options.Yaw != -40 || r.options.PanX != -.5 {
			t.Fatal("fullscreen framing", r.options)
		}
		p.SwitchMode()
		if r.mode || r.options.Zoom != 1.5 || r.options.Yaw != 20 || r.options.PanX != .3 {
			t.Fatal("split framing", r.options)
		}
	}
	if r.starts != 0 || !r.state.Running || r.changes != 200 {
		t.Fatal("renderer restarted", r)
	}
}
