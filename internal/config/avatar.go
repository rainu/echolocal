package config

// Avatar selects one device-local model. Empty preserves the imported default.
type Avatar struct {
	Path string `json:"path"`
}
