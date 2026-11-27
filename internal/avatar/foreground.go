package avatar

import (
	"errors"
	"strings"
)

// Foregrounds use exactly the bounded, straight-alpha image snapshot path used
// by backgrounds. Distinct errors keep invalid overlays from hiding the scene.
func foregroundError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(strings.Replace(err.Error(), "background_", "foreground_", 1))
}
func ValidateForeground(v string) error {
	if isBackgroundVideo(v) {
		return foregroundError(errors.New("background_video_unsupported"))
	}
	return foregroundError(ValidateBackground(v))
}
func ResolveForeground(root, dir, name string) (string, func(), error) {
	if isBackgroundVideo(name) {
		return "", func() {}, foregroundError(errors.New("background_video_unsupported"))
	}
	p, release, err := ResolveBackground(root, dir, name)
	return p, release, foregroundError(err)
}
