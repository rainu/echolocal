package avatar

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ygelfand/echolocal/internal/devicefs"
	"golang.org/x/sys/unix"
)

// Motions are a flat, fixed Lab-only catalog, never a configurable search path.
const MotionDirectory = "animations"
const maxMotionEntries = 8192

type MotionEntry struct{ File, Name string }
type MotionState struct {
	Entries                  []MotionEntry
	Notice, Selected, Status string
	Pending, Playing         bool
	Tracks                   int
}

func validMotionName(s string) bool {
	if s == "" || len(s) > 255 || !utf8.ValidString(s) || strings.ContainsAny(s, "/\\") || !strings.EqualFold(filepath.Ext(s), ".vrma") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func openMotions(dir string) (*os.File, error) {
	// Each component is pinned without following symlinks, including ancestors.
	name, err := filepath.Abs(filepath.Join(dir, MotionDirectory))
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(name, "/"), "/") {
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), name), nil
}
func DiscoverMotions(dir string) ([]MotionEntry, string) {
	d, err := openMotions(dir)
	if os.IsNotExist(err) {
		return nil, "motion_empty"
	}
	if err != nil {
		return nil, "motion_directory_error"
	}
	defer d.Close()
	names, err := d.Readdirnames(maxMotionEntries + 1)
	if err != nil && err != io.EOF {
		return nil, "motion_directory_error"
	}
	if len(names) > maxMotionEntries {
		return nil, "motion_limit"
	}
	out := []MotionEntry{}
	for _, name := range names {
		if !validMotionName(name) {
			continue
		}
		// Use a pinned, no-follow regular open, shared with existing file access.
		// Unreadable regular files remain selectable and yield a useful read error.
		var st unix.Stat_t
		if unix.Fstatat(int(d.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
			continue
		}
		out = append(out, MotionEntry{File: name, Name: name})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a == b {
			return out[i].File < out[j].File
		}
		return a < b
	})
	if len(out) == 0 {
		return out, "motion_empty"
	}
	return out, ""
}
func snapshotMotion(dir, name string) (*os.File, error) {
	fail := func(code string) (*os.File, error) { return nil, errors.New(code) }
	if !validMotionName(name) {
		return fail("motion_unavailable")
	}
	d, err := openMotions(dir)
	if err != nil {
		return fail("motion_unavailable")
	}
	defer d.Close()
	f, err := devicefs.OpenRegularAt(d, name)
	if err != nil {
		return fail("motion_unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return fail("motion_unavailable")
	}
	if st.Size() <= 0 || st.Size() > MaxMotionBytes {
		return fail("motion_invalid")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxMotionBytes+1))
	if err != nil {
		return fail("motion_unavailable")
	}
	doc, err := glbDocument(data, MaxMotionBytes)
	if err != nil {
		return fail("motion_invalid")
	}
	var ext map[string]json.RawMessage
	var spec struct{ SpecVersion string }
	if json.Unmarshal(doc["extensions"], &ext) != nil || json.Unmarshal(ext["VRMC_vrm_animation"], &spec) != nil || spec.SpecVersion != "1.0" {
		return fail("motion_invalid")
	}
	tmp, err := os.CreateTemp(dir, ".lab-motion-*")
	if err != nil {
		return fail("motion_storage")
	}
	if err = os.Remove(tmp.Name()); err != nil {
		tmp.Close()
		return fail("motion_storage")
	}
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return fail("motion_storage")
	}
	return tmp, nil
}
func (c *Controller) Motions() MotionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.motions
	s.Entries = append([]MotionEntry(nil), s.Entries...)
	return s
}
func (c *Controller) RefreshMotions() {
	entries, notice := DiscoverMotions(c.dir)
	c.mu.Lock()
	c.motions.Entries, c.motions.Notice = entries, notice
	c.mu.Unlock()
	c.redraw()
}

// SelectMotion takes an immutable unlinked snapshot. Only its scalar descriptor
// crosses IPC; the native process never receives a user pathname. No Talk state,
// composition, pose, model or persistent settings are changed.
func (c *Controller) SelectMotion(name string) {
	c.mu.Lock()
	defer func() { c.mu.Unlock(); c.redraw() }()
	if c.talkLayout || (c.motions.Pending && c.state.Running) {
		return
	}
	var file *os.File
	if name != "" {
		var err error
		file, err = snapshotMotion(c.dir, name)
		if err != nil {
			c.motions.Status = err.Error()
			return
		}
	}
	if c.motionFile != nil {
		c.motionFile.Close()
	}
	c.motionFile = file
	if name != "" {
		c.pose.Active = false
		c.pose.Enabled = false
		c.pose.revision++
	}
	c.motionRevision++
	c.motions.Selected = name
	c.motions.Pending = true
	c.motions.Playing = name != ""
	c.motions.Status = "motion_loading"
	c.motions.Tracks = 0
}
func (c *Controller) PlayMotion(play bool) {
	c.mu.Lock()
	c.motions.Playing = play && c.motions.Selected != ""
	c.mu.Unlock()
	c.redraw()
}
func (c *Controller) motionLine(gen uint64, sent *uint64, stdin io.Writer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != gen || c.talkLayout || c.motionRevision == 0 {
		return nil
	}
	if *sent != c.motionRevision {
		fd := uintptr(0)
		if c.motionFile != nil {
			fd = c.motionFile.Fd()
		}
		if _, err := fmt.Fprintf(stdin, "A %d %d\n", c.motionRevision, fd); err != nil {
			return err
		}
		*sent = c.motionRevision
	}
	_, err := fmt.Fprintf(stdin, "N %d\n", bit(c.motions.Playing))
	return err
}
func (c *Controller) motionStats(gen uint64, line string) bool {
	if !strings.HasPrefix(line, "AVATAR_MOTION ") {
		return false
	}
	var revision uint64
	var status string
	var tracks int
	var extra string
	n, _ := fmt.Sscan(strings.TrimPrefix(line, "AVATAR_MOTION "), &revision, &status, &tracks, &extra)
	if n != 3 || tracks < 0 || tracks > 4096 {
		return true
	}
	switch status {
	case "motion_ready", "motion_invalid", "motion_incompatible", "motion_none":
	default:
		return true
	}
	c.mu.Lock()
	if c.generation == gen && revision == c.motionRevision {
		c.motions.Pending = false
		c.motions.Status = status
		if status == "motion_none" {
			c.motions.Status = "motion_none_status"
		}
		c.motions.Tracks = tracks
		if status != "motion_ready" {
			c.motions.Playing = false
		}
	}
	c.mu.Unlock()
	c.redraw()
	return true
}
