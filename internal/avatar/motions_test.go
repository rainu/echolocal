package avatar

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/unix"
)

func motionFixture(t *testing.T) (string, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	folder := filepath.Join(dir, MotionDirectory)
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	data := glb(`{"extensions":{"VRMC_vrm_animation":{"specVersion":"1.0"}}}`)
	return dir, folder, data
}
func TestMotionDiscoveryRefreshAndBoundaries(t *testing.T) {
	dir := t.TempDir()
	if entries, notice := DiscoverMotions(dir); len(entries) != 0 || notice != "motion_empty" {
		t.Fatal(entries, notice)
	}
	dir, folder, data := motionFixture(t)
	for _, name := range []string{"z.vrma", "Alpha.vrma", "alpha.VRMA", "bad.vrma", "ignore.txt", "bad\nname.vrma"} {
		if err := os.WriteFile(filepath.Join(folder, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	os.Mkdir(filepath.Join(folder, "directory.vrma"), 0700)
	unix.Mkfifo(filepath.Join(folder, "pipe.vrma"), 0600)
	outside := filepath.Join(t.TempDir(), "outside.vrma")
	os.WriteFile(outside, data, 0600)
	os.Symlink(outside, filepath.Join(folder, "linked.vrma"))
	entries, notice := DiscoverMotions(dir)
	names := []string{}
	for _, e := range entries {
		names = append(names, e.File)
	}
	if notice != "" || !reflect.DeepEqual(names, []string{"Alpha.vrma", "alpha.VRMA", "bad.vrma", "z.vrma"}) {
		t.Fatal(names, notice)
	}
	for _, name := range []string{"linked.vrma", "pipe.vrma", "directory.vrma", "../outside.vrma", "/outside.vrma", "bad\nname.vrma"} {
		if file, err := snapshotMotion(dir, name); err == nil {
			file.Close()
			t.Fatal("unsafe file accepted", name)
		}
	}
	os.Remove(filepath.Join(folder, "Alpha.vrma"))
	os.WriteFile(filepath.Join(folder, "added.vrma"), data, 0600)
	entries, _ = DiscoverMotions(dir)
	if entries[0].File != "added.vrma" {
		t.Fatal(entries)
	}
	// Do not follow a symlink even when it replaces the entire catalog directory.
	os.Rename(folder, folder+"-old")
	os.Symlink(folder+"-old", folder)
	if _, notice = DiscoverMotions(dir); notice != "motion_directory_error" {
		t.Fatal(notice)
	}
	if f, e := snapshotMotion(dir, "z.vrma"); e == nil {
		f.Close()
		t.Fatal("linked directory accepted")
	}
}
func TestMotionSnapshotAndInvalidRecovery(t *testing.T) {
	dir, folder, data := motionFixture(t)
	good := filepath.Join(folder, "good.vrma")
	os.WriteFile(good, data, 0600)
	f, e := snapshotMotion(dir, "good.vrma")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	os.WriteFile(good, []byte("changed"), 0600)
	snapshot, e := os.ReadFile(fmt.Sprintf("/proc/self/fd/%d", f.Fd()))
	if e != nil || !bytes.Equal(snapshot, data) {
		t.Fatal("snapshot changed", e)
	}
	files, _ := filepath.Glob(filepath.Join(dir, ".lab-motion-*"))
	if len(files) != 0 {
		t.Fatal("snapshot was not unlinked")
	}
	for _, bad := range [][]byte{nil, []byte("bad"), glb(`{"extensions":{"VRMC_vrm_animation":{"specVersion":"2.0"}}}`), glb(`{"buffers":[{"uri":"file:///not-read"}],"extensions":{"VRMC_vrm_animation":{"specVersion":"1.0"}}}`), make([]byte, MaxMotionBytes+1)} {
		os.WriteFile(good, bad, 0600)
		if f, e := snapshotMotion(dir, "good.vrma"); e == nil {
			f.Close()
			t.Fatal("invalid accepted")
		}
	}
	os.WriteFile(good, data, 0600)
	c := New(dir, nil, func() bool { return false }, func() {})
	defer func() {
		if c.motionFile != nil {
			c.motionFile.Close()
		}
	}()
	c.Configure(Options{Zoom: 3, PanX: .4, Animation: false})
	before := c.Snapshot().Options
	c.pose.Rotations = map[string]Rotation{"head": {1, 2, 3}}
	c.pose.Active = true
	c.SelectMotion("good.vrma")
	if c.Snapshot().Options != before || c.pose.Rotations["head"] != (Rotation{1, 2, 3}) || c.pose.Active {
		t.Fatal("selection changed framing/pose edits")
	}
	var sent uint64
	var wire bytes.Buffer
	c.motionLine(c.generation, &sent, &wire)
	if !strings.HasPrefix(wire.String(), "A 1 ") {
		t.Fatal(wire.String())
	}
	c.motionStats(c.generation, "AVATAR_MOTION 1 motion_invalid 0")
	if c.Motions().Pending || c.Motions().Status != "motion_invalid" {
		t.Fatal(c.Motions())
	}
	c.SelectMotion("good.vrma")
	c.motionStats(c.generation, "AVATAR_MOTION 1 motion_ready 2")
	if !c.Motions().Pending {
		t.Fatal("stale acknowledgement")
	}
	c.motionStats(c.generation, "AVATAR_MOTION 2 motion_ready 2")
	if c.Motions().Pending || c.Motions().Status != "motion_ready" {
		t.Fatal(c.Motions())
	}
	c.PlayMotion(false)
	if c.Motions().Playing || c.Snapshot().Options != before {
		t.Fatal("pause changed unrelated settings")
	}
	c.SelectMotion("")
	c.motionStats(c.generation, "AVATAR_MOTION 3 motion_none 0")
	if c.motionFile != nil || c.Motions().Status != "motion_none_status" {
		t.Fatal(c.Motions())
	}
	c.EnableTalkLayout(nil)
	c.SelectMotion("good.vrma")
	wire.Reset()
	sent = 0
	c.motionLine(c.generation, &sent, &wire)
	if wire.Len() != 0 || c.motionRevision != 3 {
		t.Fatal("Lab animation leaked into Talk")
	}
}
func TestMotionStartupFailureCanRecover(t *testing.T) {
	dir, folder, data := motionFixture(t)
	os.WriteFile(filepath.Join(folder, "good.vrma"), data, 0600)
	c := New(dir, nil, func() bool { return false }, func() {})
	c.SelectMotion("good.vrma")
	defer func() {
		if c.motionFile != nil {
			c.motionFile.Close()
		}
	}()
	c.Start()
	if c.Motions().Pending {
		t.Fatal("asset failure left selection locked")
	}
	// Existing lifecycle helper validates the real controller feed and ACK path.
	var starts atomic.Int32
	helper(c, "motion", &starts)
	c.SelectMotion("good.vrma")
	c.Start()
	defer c.Stop()
	eventually(t, func() bool { return c.Motions().Status == "motion_ready" })
	if starts.Load() != 1 || !c.Snapshot().Ready {
		t.Fatal("motion restarted renderer")
	}
	c.SelectMotion("good.vrma")
	eventually(t, func() bool { return !c.Motions().Pending })
	if starts.Load() != 1 {
		t.Fatal("live replacement restarted renderer")
	}
	c.Stop()
	eventually(t, func() bool { return !c.Snapshot().Running })
}
func TestMotionCatalogLimitAndLanguages(t *testing.T) {
	dir, folder, _ := motionFixture(t)
	for i := 0; i <= maxMotionEntries; i++ {
		f, e := os.Create(filepath.Join(folder, fmt.Sprintf("%05d.txt", i)))
		if e != nil {
			t.Fatal(e)
		}
		f.Close()
	}
	if _, notice := DiscoverMotions(dir); notice != "motion_limit" {
		t.Fatal(notice)
	}
	for key := range motionWords["en"] {
		for _, lang := range []string{"en", "de", "it", "vi"} {
			if Text(lang, key) == "" {
				t.Fatal(lang, key)
			}
		}
	}
	if Text("unknown", "motion_label") != Text("en", "motion_label") {
		t.Fatal("fallback")
	}
}
