package filepicker

import "github.com/ygelfand/echolocal/internal/layout"

// UserLocations is the single trusted root list for both on-device modes.
// Labels may be localized; callers cannot turn application state into a root.
func UserLocations(words func(string, string) string) []Location {
	return []Location{
		{words("User files", "Benutzerdateien"), layout.StateDir + "/user-files"},
		{words("Local files", "Lokale Dateien"), "/data/local"},
		{words("Shared storage", "Gemeinsamer Speicher"), "/data/media/0"},
	}
}
