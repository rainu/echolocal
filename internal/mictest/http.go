package mictest

import (
	"encoding/json"
	"net/http"
)

// Handler must be mounted behind the existing admin authentication middleware.
// It exposes status and commands only: no audio download/upload or persistence.
func (m *Monitor) Handler() http.Handler {
	return http.NewCrossOriginProtection().Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "" && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		owner := r.Header.Get("X-Microphone-Test")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(m.State(owner))
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", 405)
			return
		}
		var body struct {
			Action string `json:"action"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256))
		d.DisallowUnknownFields()
		if err := d.Decode(&body); err != nil {
			http.Error(w, "invalid command", 400)
			return
		}
		if err := m.Command(owner, body.Action); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		_ = json.NewEncoder(w).Encode(m.State(owner))
	}))
}
