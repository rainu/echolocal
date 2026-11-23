package config

// AvatarZoomMin and AvatarZoomMax define the shared persisted/API percent range.
const (
	AvatarMouthIntensityDefault = 50
	AvatarMouthIntensityMax     = 100
	AvatarZoomMin               = 50
	AvatarZoomMax               = 999
	// A fitted model can extend half a viewport width from center. At 999% zoom,
	// 500% translation keeps every model point reachable without changing pan units.
	AvatarPositionLimit = 500
)

// Avatar contains the model and shared lab/conversation presentation.
// Integer units are degrees and percent of the square rendering viewport.
type Avatar struct {
	TalkMode             string `json:"talk_mode,omitempty"`
	FullscreenBackground string `json:"fullscreen_background,omitempty"`
	FullscreenZoom       int    `json:"fullscreen_zoom,omitempty"`
	FullscreenYaw        int    `json:"fullscreen_yaw,omitempty"`
	FullscreenPitch      int    `json:"fullscreen_pitch,omitempty"`
	FullscreenX          int    `json:"fullscreen_x,omitempty"`
	FullscreenY          int    `json:"fullscreen_y,omitempty"`
	// nil migrates old installations to 50%; explicit zero closes the mouth.
	MouthIntensity *int `json:"mouth_intensity,omitempty"`
	// Disabled flags preserve the legacy default: all three motions enabled.
	IdleDisabled      bool   `json:"idle_disabled,omitempty"`
	BlinkDisabled     bool   `json:"blink_disabled,omitempty"`
	AnimationDisabled bool   `json:"animation_disabled,omitempty"`
	Background        string `json:"background,omitempty"`
	Path              string `json:"path"`
	Zoom              int    `json:"zoom,omitempty"`
	Yaw               int    `json:"yaw,omitempty"`
	Pitch             int    `json:"pitch,omitempty"`
	X                 int    `json:"x,omitempty"`
	Y                 int    `json:"y,omitempty"`
}

// Presentation makes legacy zero zoom usable and bounds hand-edited storage.
func (a Avatar) Presentation() Avatar {
	if a.Zoom == 0 {
		a.Zoom = 100
	}
	a.Zoom = max(AvatarZoomMin, min(AvatarZoomMax, a.Zoom))
	a.Yaw = max(-180, min(180, a.Yaw))
	a.Pitch = max(-85, min(85, a.Pitch))
	a.X = max(-AvatarPositionLimit, min(AvatarPositionLimit, a.X))
	a.Y = max(-AvatarPositionLimit, min(AvatarPositionLimit, a.Y))
	return a
}

// MouthPercent preserves explicit zero while providing a safe legacy default.
func (a Avatar) MouthPercent() int {
	if a.MouthIntensity == nil {
		return AvatarMouthIntensityDefault
	}
	return max(0, min(AvatarMouthIntensityMax, *a.MouthIntensity))
}

// Mode preserves split-screen for old and invalid hand-edited configurations.
func (a Avatar) Mode() string {
	if a.TalkMode == "fullscreen" {
		return "fullscreen"
	}
	return "split_screen"
}

// Composition selects independent framing; expression and the model stay shared.
func (a Avatar) Composition(full bool) Avatar {
	if full {
		a.Background = a.FullscreenBackground
		a.Zoom = a.FullscreenZoom
		a.Yaw = a.FullscreenYaw
		a.Pitch = a.FullscreenPitch
		a.X = a.FullscreenX
		a.Y = a.FullscreenY
	}
	return a.Presentation()
}
