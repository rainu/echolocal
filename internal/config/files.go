package config

// Files is presentation state only. It grants no filesystem or remote authority.
type Files struct {
	View       string `json:"view,omitempty"`
	Tile       int    `json:"tile,omitempty"`
	Sort       string `json:"sort,omitempty"`
	Descending bool   `json:"descending,omitempty"`
	Hidden     bool   `json:"hidden,omitempty"`
}

func (f Files) Presentation() Files {
	if f.View != "grid" {
		f.View = "list"
	}
	switch f.Tile {
	case 128, 192, 256:
	default:
		f.Tile = 192
	}
	switch f.Sort {
	case "modified", "size", "type":
	default:
		f.Sort = "name"
	}
	return f
}
