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
