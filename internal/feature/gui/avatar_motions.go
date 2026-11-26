//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/libcountertop/pkg/say"
)

type avatarMotionController interface {
	Motions() avatar.MotionState
	RefreshMotions()
	SelectMotion(string)
	PlayMotion(bool)
}

func (v *avatarView) motionControls() []gogui.View {
	c, ok := v.app.(avatarMotionController)
	if !ok {
		return nil
	}
	s := c.Motions()
	text := func(key string) string { return avatar.Text(say.Chosen(), key) }
	options := []gogui.SelectOption{gogui.NewSelectOption(text("motion_none"), "")}
	present := map[string]bool{"": true}
	for _, entry := range s.Entries {
		options = append(options, gogui.NewSelectOption(entry.Name, entry.File))
		present[entry.File] = true
	}
	// A playing snapshot survives removal. Refresh must not falsely label it as
	// another entry or stop an unrelated preview.
	if s.Selected != "" && !present[s.Selected] {
		options = append(options, gogui.NewSelectOption(s.Selected+" — "+text("motion_removed"), s.Selected))
		present[s.Selected] = true
	}
	style := gogui.CurrentTheme().Cfg.TextStyleDef
	style.Size = 36
	style.Color = gogui.Hex(0xEEF3F4)
	choose := gogui.Select(gogui.SelectCfg{
		ID: "avatar-motion", A11YCfg: gogui.A11YCfg{A11YLabel: text("motion_label")},
		Options: options, Selected: []string{s.Selected}, Placeholder: text("motion_choose"),
		Sizing: gogui.FillFit, TextStyle: style, PlaceholderStyle: style, Padding: gogui.PadVH(6, 12),
		Color: gogui.Hex(0x283C42), ColorSelectSubtle: gogui.Hex(0x3E706D), FloatZIndex: 100,
		OnSelect: func(values []string, e gogui.EventCtx) {
			if len(values) != 1 || !present[values[0]] || s.Pending {
				return
			}
			v.leavePose()
			c.SelectMotion(values[0])
			if state := c.Motions(); state.Pending {
				if v.app.Snapshot().Notice == "stopping" {
					v.resume.Store(true)
				} else {
					v.app.Start()
				}
			}
			e.Window.InvalidateLayout()
		},
	})
	body := []gogui.View{avatarHeading(text("motion_label")), choose,
		avatarRow(avatarButton("avatar-motion-refresh", text("motion_refresh"), false, false, c.RefreshMotions),
			avatarButton("avatar-motion-play", text("motion_play"), s.Playing, s.Pending || s.Status != "motion_ready", func() { c.PlayMotion(true) }),
			avatarButton("avatar-motion-pause", text("motion_pause"), !s.Playing, s.Pending || s.Status != "motion_ready", func() { c.PlayMotion(false) }))}
	if s.Notice != "" {
		body = append(body, avatarText(text(s.Notice)))
	}
	if s.Status != "" {
		body = append(body, avatarText(text(s.Status)))
	}
	body = append(body, avatarText(text("motion_help")))
	return body
}
