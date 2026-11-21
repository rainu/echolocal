package userfiles

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DAVPrefix is fixed: clients cannot select a different root or filesystem.
// DAVHandler must be mounted ONLY behind owner authentication and opt-in HTTPS.
const DAVPrefix = "/api/v1/webdav"

const davAllow = "OPTIONS, PROPFIND, GET, HEAD, PUT, DELETE"

// DAVHandler implements a bounded flat WebDAV transfer subset. It deliberately
// does not advertise DAV compliance classes (properties, collections, COPY/MOVE
// and locking are not fully implemented). Data operations are the existing API.
func (s *Store) DAVHandler() http.Handler {
	files := s.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Allow", davAllow)
		if !sameOrigin(r) {
			fail(w, 403, "cross_origin")
			return
		}
		root := r.URL.Path == "" || r.URL.Path == "/"
		if !root && !ValidName(strings.TrimPrefix(r.URL.Path, "/")) {
			fail(w, 400, "invalid_name")
			return
		}
		// These preconditions require lock tokens or property-based semantics that
		// this subset does not implement. Never silently ignore a client's lock.
		if r.Header.Get("If") != "" || r.Header.Get("Lock-Token") != "" {
			fail(w, 412, "unsupported_precondition")
			return
		}
		switch r.Method {
		case "OPTIONS":
			w.WriteHeader(http.StatusNoContent)
		case "PROPFIND":
			s.propfind(w, r, root)
		case "GET", "HEAD":
			if root {
				fail(w, 405, "collection_not_downloadable")
				return
			}
			files.ServeHTTP(w, r)
		case "PUT", "DELETE":
			if root {
				fail(w, 405, "protected_collection")
				return
			}
			// Existing names may never be overwritten, regardless of Overwrite or
			// conditional headers. Reject other preconditions rather than misapply them.
			if r.Header.Get("If-Match") != "" || r.Header.Get("If-Unmodified-Since") != "" ||
				(r.Header.Get("If-None-Match") != "" && !(r.Method == "PUT" && r.Header.Get("If-None-Match") == "*")) {
				fail(w, 412, "unsupported_precondition")
				return
			}
			files.ServeHTTP(w, r)
		default:
			fail(w, 405, "method_not_allowed")
		}
	})
}

type davProperty struct{ XMLName xml.Name }
type davRequest struct {
	XMLName xml.Name
	All     *struct{} `xml:"DAV: allprop"`
	Names   *struct{} `xml:"DAV: propname"`
	Prop    *struct {
		Properties []davProperty `xml:",any"`
	} `xml:"DAV: prop"`
}
type davProperties struct {
	Values []davValue `xml:",any"`
}
type davValue struct {
	XMLName    xml.Name
	Text       string    `xml:",chardata"`
	Collection *struct{} `xml:"DAV: collection,omitempty"`
}
type davPropstat struct {
	Prop   davProperties `xml:"DAV: prop"`
	Status string        `xml:"DAV: status"`
}
type davResponse struct {
	Href     string        `xml:"DAV: href"`
	Propstat []davPropstat `xml:"DAV: propstat"`
}
type davMultistatus struct {
	XMLName   xml.Name      `xml:"DAV: multistatus"`
	Responses []davResponse `xml:"DAV: response"`
}

func (s *Store) propfind(w http.ResponseWriter, r *http.Request, root bool) {
	depth := r.Header.Get("Depth")
	if depth == "" || depth == "infinity" {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(403)
		io.WriteString(w, "<error xmlns=\"DAV:\"><propfind-finite-depth/></error>")
		return
	}
	if depth != "0" && depth != "1" {
		fail(w, 400, "invalid_depth")
		return
	}
	var query davRequest
	dec := xml.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	err := dec.Decode(&query)
	if err == io.EOF {
		query.All = &struct{}{}
	} else if err != nil {
		fail(w, 400, "invalid_propfind")
		return
	} else {
		var extra any
		if dec.Decode(&extra) != io.EOF || query.XMLName != (xml.Name{Space: "DAV:", Local: "propfind"}) {
			fail(w, 400, "invalid_propfind")
			return
		}
	}
	modes := 0
	if query.All != nil {
		modes++
	}
	if query.Names != nil {
		modes++
	}
	if query.Prop != nil {
		modes++
		if len(query.Prop.Properties) > 128 {
			fail(w, 400, "too_many_properties")
			return
		}
	}
	if modes != 1 {
		fail(w, 400, "invalid_propfind")
		return
	}
	out := davMultistatus{Responses: []davResponse{}}
	if root {
		view, err := s.List()
		if err != nil {
			httpFail(w, err)
			return
		}
		out.Responses = append(out.Responses, davEntry(Metadata{Name: "Managed files"}, true, query))
		if depth == "1" {
			for _, m := range view.Files {
				out.Responses = append(out.Responses, davEntry(m, false, query))
			}
		}
	} else {
		f, m, err := s.Open(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			httpFail(w, err)
			return
		}
		f.Close()
		out.Responses = append(out.Responses, davEntry(m, false, query))
	}
	// At most 257 responses with bounded names/property count: encode before
	// headers so an encoding error never becomes a partial successful listing.
	b, err := xml.Marshal(out)
	if err != nil {
		fail(w, 500, "listing_failed")
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(207)
	w.Write(append([]byte(xml.Header), b...))
}

func davEntry(m Metadata, root bool, q davRequest) davResponse {
	href := DAVPrefix + "/"
	if !root {
		href += url.PathEscape(m.Name)
	}
	values := []davValue{
		{XMLName: xml.Name{Space: "DAV:", Local: "displayname"}, Text: m.Name},
		{XMLName: xml.Name{Space: "DAV:", Local: "resourcetype"}},
	}
	if root {
		values[1].Collection = &struct{}{}
	} else {
		values = append(values,
			davValue{XMLName: xml.Name{Space: "DAV:", Local: "getcontentlength"}, Text: strconv.FormatInt(m.Size, 10)},
			davValue{XMLName: xml.Name{Space: "DAV:", Local: "getlastmodified"}, Text: m.Modified.UTC().Format(http.TimeFormat)},
			davValue{XMLName: xml.Name{Space: "DAV:", Local: "getcontenttype"}, Text: "application/octet-stream"},
		)
	}
	good, bad := davProperties{Values: []davValue{}}, davProperties{Values: []davValue{}}
	if q.Prop != nil {
		for _, p := range q.Prop.Properties {
			found := false
			for _, v := range values {
				if v.XMLName == p.XMLName {
					good.Values = append(good.Values, v)
					found = true
					break
				}
			}
			if !found {
				bad.Values = append(bad.Values, davValue{XMLName: p.XMLName})
			}
		}
	} else {
		good.Values = values
		if q.Names != nil {
			for i := range good.Values {
				good.Values[i].Text = ""
				good.Values[i].Collection = nil
			}
		}
	}
	response := davResponse{Href: href}
	if len(good.Values) > 0 || (q.Prop != nil && len(q.Prop.Properties) == 0) {
		response.Propstat = append(response.Propstat, davPropstat{good, "HTTP/1.1 200 OK"})
	}
	if len(bad.Values) > 0 {
		response.Propstat = append(response.Propstat, davPropstat{bad, "HTTP/1.1 404 Not Found"})
	}
	return response
}
