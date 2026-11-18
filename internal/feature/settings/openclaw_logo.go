//go:build !board_biscuit

package settings

import (
	_ "embed"
	gogui "github.com/go-gui-org/go-gui/gui"
)

//go:embed openclaw-logo.svg
var openclawLogoSVG string

func openclawLogo() gogui.View {
	return gogui.Svg(gogui.SvgCfg{ID: "openclaw-menu-logo", SvgData: openclawLogoSVG, Width: 24, Height: 24})
}
