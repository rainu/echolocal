package networkmedia

import (
	"encoding/json"
	"io"
	"net/http"
)

func (s *Store) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path != "/" && r.URL.Path != "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(s.View())
		case http.MethodPut:
			var u Update
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
			d.DisallowUnknownFields()
			if d.Decode(&u) != nil || d.Decode(new(any)) != io.EOF {
				http.Error(w, "Invalid SMB settings request", 400)
				return
			}
			if e := s.Save(u); e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, PUT")
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
