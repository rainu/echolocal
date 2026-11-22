package userfiles

import (
	"fmt"
	"golang.org/x/net/webdav"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type ifGroup struct {
	resource string
	lists    [][]webdav.Condition
}

// RFC 4918 section 10.4: conditions inside a list are AND, alternative lists
// for one resource are OR, and tagged resource groups are AND. Evaluating the
// complete header here fixes the standard handler's per-list two-path shortcut.
func parseDAVIf(h string) ([]ifGroup, error) {
	var groups []ifGroup
	tagged := false
	untagged := false
	trim := func() { h = strings.TrimLeft(h, " \t\r\n") }
	for {
		trim()
		if h == "" {
			break
		}
		if h[0] == '<' {
			if untagged {
				return nil, ErrName
			}
			tagged = true
			i := strings.IndexByte(h, '>')
			if i < 1 {
				return nil, ErrName
			}
			groups = append(groups, ifGroup{resource: h[1:i]})
			h = h[i+1:]
			trim()
		} else if h[0] == '(' {
			if len(groups) == 0 {
				untagged = true
				groups = append(groups, ifGroup{})
			} else if !tagged {
				untagged = true
			}
		} else {
			return nil, ErrName
		}
		if len(groups) == 0 || h == "" || h[0] != '(' {
			return nil, ErrName
		}
		for h != "" && h[0] == '(' {
			h = h[1:]
			var list []webdav.Condition
			for {
				trim()
				if h == "" {
					return nil, ErrName
				}
				if h[0] == ')' {
					h = h[1:]
					break
				}
				c := webdav.Condition{}
				if strings.HasPrefix(h, "Not") {
					if len(h) < 4 || !strings.ContainsRune(" \t\r\n", rune(h[3])) {
						return nil, ErrName
					}
					c.Not = true
					h = h[3:]
					trim()
				}
				if h == "" {
					return nil, ErrName
				}
				switch h[0] {
				case '<':
					i := strings.IndexByte(h, '>')
					if i <= 1 {
						return nil, ErrName
					}
					c.Token = h[1:i]
					u, e := url.Parse(c.Token)
					if e != nil || u.Scheme == "" {
						return nil, ErrName
					}
					h = h[i+1:]
				case '[':
					i := strings.IndexByte(h, ']')
					if i <= 1 {
						return nil, ErrName
					}
					c.ETag = strings.TrimSpace(h[1:i])
					v := strings.TrimPrefix(c.ETag, "W/")
					if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
						return nil, ErrName
					}
					h = h[i+1:]
				default:
					return nil, ErrName
				}
				list = append(list, c)
			}
			if len(list) == 0 {
				return nil, ErrName
			}
			g := &groups[len(groups)-1]
			g.lists = append(g.lists, list)
			trim()
		}
	}
	if len(groups) == 0 {
		return nil, ErrName
	}
	return groups, nil
}

func (s *Store) checkDAVIf(r *http.Request, n string) (int, error) {
	h := r.Header.Get("If")
	if h == "" {
		return 0, nil
	}
	gs, e := parseDAVIf(h)
	if e != nil {
		return 400, e
	}
	type evaluation struct {
		name, etag string
		lists      [][]webdav.Condition
	}
	var evaluations []evaluation
	for _, g := range gs {
		name := "/" + n
		if g.resource != "" {
			u, e := url.Parse(g.resource)
			if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host != r.Host || u.Scheme != "https" || !strings.HasPrefix(u.Path, DAVPrefix+"/") {
				return 412, ErrName
			}
			name = strings.TrimPrefix(u.Path, DAVPrefix)
		}
		name = path.Clean(name)
		rel := strings.TrimPrefix(name, "/")
		if rel == "" {
			rel = "."
		} else if !ValidPath(rel) {
			return 412, ErrName
		}
		s.mu.Lock()
		fi, e := s.statTree(rel)
		s.mu.Unlock()
		etag := ""
		if e == nil {
			etag = fmt.Sprintf(`"%x%x"`, fi.ModTime().UnixNano(), fi.Size())
		}
		evaluations = append(evaluations, evaluation{name, etag, g.lists})
	}
	l := s.dav.locks
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(time.Now())
	positive := map[string]bool{}
	for _, g := range evaluations {
		anyTrue := false
		for _, list := range g.lists {
			all := true
			tokens := []string{}
			for _, c := range list {
				truth := false
				if c.Token != "" {
					v, ok := l.active[c.Token]
					truth = ok && lockCovers(v.details, g.name)
					if truth && !c.Not {
						tokens = append(tokens, c.Token)
					}
				}
				if c.ETag != "" {
					truth = g.etag != "" && strings.TrimPrefix(c.ETag, "W/") == strings.TrimPrefix(g.etag, "W/")
				}
				if c.Not {
					truth = !truth
				}
				all = all && truth
			}
			if all {
				anyTrue = true
				for _, t := range tokens {
					positive[t] = true
				}
			}
		}
		if !anyTrue {
			return 412, webdav.ErrConfirmationFailed
		}
	}
	var targets []string
	switch r.Method {
	case "PUT", "PROPPATCH", "DELETE", "MOVE":
		targets = append(targets, path.Clean("/"+n))
	}
	if r.Method == "COPY" || r.Method == "MOVE" {
		u, _ := url.Parse(r.Header.Get("Destination"))
		if u != nil {
			targets = append(targets, path.Clean(strings.TrimPrefix(u.Path, DAVPrefix)))
		}
	}
	recursive := r.Method == "DELETE" || r.Method == "COPY" || r.Method == "MOVE"
	for _, target := range targets {
		for token, v := range l.active {
			relevant := lockCovers(v.details, target) || (recursive && strings.HasPrefix(v.details.Root, strings.TrimSuffix(target, "/")+"/"))
			if !relevant {
				continue
			}
			if !v.shared {
				if !positive[token] {
					return 423, webdav.ErrLocked
				}
				continue
			}
			point := target
			if recursive && strings.HasPrefix(v.details.Root, strings.TrimSuffix(target, "/")+"/") {
				point = v.details.Root
			}
			match := false
			for t := range positive {
				other := l.active[t]
				if other.shared && lockCovers(other.details, point) {
					match = true
					break
				}
			}
			if !match {
				return 423, webdav.ErrLocked
			}
		}
	}
	return 0, nil
}
func (l *davLocks) deleteUnder(n string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n = path.Clean("/" + n)
	for t, v := range l.active {
		if v.details.Root == n || strings.HasPrefix(v.details.Root, strings.TrimSuffix(n, "/")+"/") {
			delete(l.active, t)
		}
	}
}
