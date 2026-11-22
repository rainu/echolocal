package userfiles

import (
	"golang.org/x/net/webdav"
	"path"
	"strings"
	"sync"
	"time"
)

// x/net's MemLS does not implement Not/ETag conditions or shared locks. Keep
// the bounded lock table common to DAV and API, and evaluate every condition
// in a state list (AND); the HTTP handler evaluates alternative lists (OR).
type davLocks struct {
	mu     sync.Mutex
	active map[string]heldLock
}
type heldLock struct {
	details webdav.LockDetails
	expires time.Time
	shared  bool
}

func newDAVLocks() *davLocks { return &davLocks{active: map[string]heldLock{}} }
func (l *davLocks) prune(now time.Time) {
	for t, v := range l.active {
		if !now.Before(v.expires) {
			delete(l.active, t)
		}
	}
}
func lockCovers(d webdav.LockDetails, n string) bool {
	return d.Root == n || (!d.ZeroDepth && strings.HasPrefix(n, strings.TrimSuffix(d.Root, "/")+"/"))
}
func lockOverlap(a, b webdav.LockDetails) bool { return lockCovers(a, b.Root) || lockCovers(b, a.Root) }
func boundedDuration(d time.Duration) time.Duration {
	if d < 0 || d > time.Hour {
		return time.Hour
	}
	return d
}
func (l *davLocks) create(now time.Time, d webdav.LockDetails, shared bool) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	d.Root = path.Clean("/" + d.Root)
	if len(l.active) >= MaxFiles*2 {
		return "", webdav.ErrLocked
	}
	for _, v := range l.active {
		if lockOverlap(d, v.details) && !(shared && v.shared) {
			return "", webdav.ErrLocked
		}
	}
	d.Duration = boundedDuration(d.Duration)
	t := "opaquelocktoken:" + randomID()
	l.active[t] = heldLock{d, now.Add(d.Duration), shared}
	return t, nil
}
func (l *davLocks) Create(now time.Time, d webdav.LockDetails) (string, error) {
	return l.create(now, d, false)
}
func (l *davLocks) Refresh(now time.Time, t string, d time.Duration) (webdav.LockDetails, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	v, ok := l.active[t]
	if !ok {
		return webdav.LockDetails{}, webdav.ErrNoSuchLock
	}
	d = boundedDuration(d)
	v.details.Duration = d
	v.expires = now.Add(d)
	l.active[t] = v
	return v.details, nil
}
func (l *davLocks) Unlock(now time.Time, t string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	if _, ok := l.active[t]; !ok {
		return webdav.ErrNoSuchLock
	}
	delete(l.active, t)
	return nil
}

// requestLocks carries request-specific shared scope, collection semantics and
// an ETag lookup. It never stores request context or file data in the lock table.
type requestLocks struct {
	*davLocks
	s        *Store
	shared   bool
	method   string
	verified bool
}

func (l requestLocks) Create(now time.Time, d webdav.LockDetails) (string, error) {
	if l.method != "LOCK" && (l.method == "DELETE" || l.method == "MOVE" || l.method == "COPY") {
		l.s.mu.Lock()
		fi, e := l.s.statTree(strings.TrimPrefix(d.Root, "/"))
		l.s.mu.Unlock()
		if e == nil && fi.IsDir() {
			d.ZeroDepth = false
		}
	}
	return l.create(now, d, l.shared && l.method == "LOCK")
}
func (l requestLocks) Confirm(now time.Time, a, b string, cs ...webdav.Condition) (func(), error) {
	if !l.verified {
		return nil, webdav.ErrConfirmationFailed
	}
	return func() {}, nil
}
