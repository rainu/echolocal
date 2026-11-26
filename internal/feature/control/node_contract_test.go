//go:build !board_biscuit

package control

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	connection "github.com/ygelfand/echolocal/internal/openclaw"
	libalarm "github.com/ygelfand/libcountertop/pkg/assistant/alarm"
	"github.com/ygelfand/libcountertop/pkg/display/surface"
)

func TestNodePublicCommandNamespaces(t *testing.T) {
	for _, name := range []string{"screen.snapshot", "screen.record", "device.info", "device.status", "device.health", "device.permissions", "system.notify", "camera.snap"} {
		if got := nodeCommandName(name); got != name {
			t.Fatal(got)
		}
	}
	for _, name := range []string{"input.tap", "settings.import", "audio.volume.set", "timer.create", "talk.start", "maintenance.restart", "device.sensors"} {
		if got := nodeCommandName(name); got != "echolocal."+name {
			t.Fatal(got)
		}
	}
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	_ = nodeRegister(a, connection.NodeOperation{Command: "audio.volume.set", Group: "audio", Run: func(context.Context, json.RawMessage) (any, error) { return true, nil }})
	if _, err := a.Execute(context.Background(), "audio.volume.set", nil); err == nil {
		t.Fatal("legacy unnamespaced command still executable")
	}
	if _, err := a.Execute(context.Background(), "echolocal.audio.volume.set", []byte(`{"volume":50}`)); err != nil {
		t.Fatal(err)
	}
	if err := a.SetGrants(map[string]bool{"audio": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Execute(context.Background(), "echolocal.audio.volume.set", []byte(`{"volume":50}`)); err == nil || err.Error() != "permission_denied" {
		t.Fatal("grant bypass", err)
	}
}
func TestNodeSnapshotContractEncoding(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	got, err := encodeNodeSnapshot(img, nodeSnapshotParams{Format: "png", MaxWidth: 10})
	if err != nil {
		t.Fatal(err)
	}
	obj := got.(map[string]any)
	data, err := base64.StdEncoding.DecodeString(obj["base64"].(string))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 10 || decoded.Bounds().Dy() != 5 || obj["format"] != "png" {
		t.Fatal(obj["width"], obj["height"])
	}
	for _, p := range []nodeSnapshotParams{{Format: "gif"}, {ScreenIndex: 1}, {MaxWidth: -1}, {Quality: ptrFloat(1.1)}} {
		if _, err := encodeNodeSnapshot(img, p); err == nil {
			t.Fatal("invalid snapshot accepted", p)
		}
	}
}
func ptrFloat(v float64) *float64 { return &v }
func TestNodeTypedParameterValidation(t *testing.T) {
	for _, v := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		if nodePercent(&v) {
			t.Fatal(v)
		}
	}
	if nodePercent(nil) {
		t.Fatal("missing percent accepted")
	}
	for _, v := range []float64{0, 50, 100} {
		if !nodePercent(&v) {
			t.Fatal(v)
		}
	}
	if _, ok := nodeStream("microphone"); ok {
		t.Fatal("speaker mute may not control microphone")
	}
	p := nodeAlarmParams{}
	if _, err := p.apply(libalarm.Alarm{}, true); err == nil {
		t.Fatal("new alarm needs time")
	}
	raw := json.RawMessage("{\"time\":\"25:99\"}")
	if decodeNode(raw, &p) != nil {
		t.Fatal("decode")
	}
	if _, err := p.apply(libalarm.Alarm{}, true); err == nil {
		t.Fatal("invalid clock accepted")
	}
	clock := "06:30"
	name := "synthetic"
	fresh, err := (nodeAlarmParams{Time: &clock, Name: &name}).apply(libalarm.Alarm{}, true)
	if err != nil || fresh.Hour != 6 || fresh.Minute != 30 || !fresh.On || fresh.Volume != libalarm.VolumeStatic {
		t.Fatal("valid new alarm rejected", err)
	}
	changed := "updated"
	updated, err := (nodeAlarmParams{Name: &changed}).apply(fresh, false)
	if err != nil || updated.Hour != fresh.Hour || updated.Volume != fresh.Volume || updated.Name != "updated" {
		t.Fatal("patch did not preserve fields", err)
	}
	if nodeSettingGroup("screen") != "display" {
		t.Fatal("settings could bypass display permission")
	}
}

type fakeNodeRecording struct {
	frames []surface.Encoded
	cancel context.CancelFunc
}

func (f *fakeNodeRecording) Read() (surface.Encoded, error) {
	if len(f.frames) == 0 {
		f.cancel()
		return surface.Encoded{}, io.EOF
	}
	v := f.frames[0]
	f.frames = f.frames[1:]
	return v, nil
}
func TestNodeRecordingProducesParseableMP4(t *testing.T) {
	sps := []byte{0x67, 0x42, 0xc0, 0x28, 0xd9, 0x00, 0x78, 0x02, 0x27, 0xe5, 0x84, 0x00, 0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xf0, 0x3c, 0x60, 0xc9, 0x20}
	annex := func(nals ...[]byte) []byte {
		b, err := h264.AnnexB(nals).Marshal()
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeNodeRecording{cancel: cancel, frames: []surface.Encoded{
		{Data: annex(sps, []byte{0x68, 0x01}), Flags: 2},
		{Data: annex([]byte{0x65, 0x01, 0x02}), Flags: 1, At: time.Second},
		{Data: annex([]byte{0x41, 0x02, 0x03}), At: time.Second + time.Second/30},
	}}
	got, err := nodeRecordMP4(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	obj := got.(map[string]any)
	data, err := base64.StdEncoding.DecodeString(obj["base64"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var init fmp4.Init
	if err := init.Unmarshal(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if len(init.Tracks) != 1 {
		t.Fatal("no video track")
	}
	at := bytes.Index(data, []byte("moof"))
	if at < 4 {
		t.Fatal("missing video fragment")
	}
	var parts fmp4.Parts
	if err := parts.Unmarshal(data[at-4:]); err != nil {
		t.Fatal(err)
	}
	if len(parts) != 1 || len(parts[0].Tracks[0].Samples) != 2 {
		t.Fatal("missing samples")
	}
	if obj["hasAudio"] != false || obj["format"] != "mp4" {
		t.Fatal("wrong wire metadata")
	}
}
func TestNodeRecordingRejectsBrokenInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeNodeRecording{cancel: cancel, frames: []surface.Encoded{{Data: []byte("not H264")}}}
	if _, err := nodeRecordMP4(ctx, f); err == nil {
		t.Fatal("malformed capture accepted")
	}
}

func TestNodeTransferContractAndGrant(t *testing.T) {
	a := connection.NewNodeAccess(filepath.Join(t.TempDir(), "permissions"))
	calls := 0
	err := nodeRegister(a, connection.NodeOperation{Command: "talk.transfer", Group: "talk", Run: func(context.Context, json.RawMessage) (any, error) { calls++; return true, nil }})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"{}", "{\"sessionId\":12}", "{\"sessionId\":\"agent:test:next\",\"extra\":true}",
	} {
		if _, err := a.Execute(context.Background(), "echolocal.talk.transfer", []byte(raw)); err == nil {
			t.Fatal("invalid transfer parameters accepted", raw)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request reached executor")
	}
	if _, err := a.Execute(context.Background(), "echolocal.talk.transfer", []byte("{\"sessionId\":\"agent:test:next\"}")); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
	if err := a.SetGrants(map[string]bool{"talk": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Execute(context.Background(), "echolocal.talk.transfer", []byte("{\"sessionId\":\"agent:test:next\"}")); err == nil {
		t.Fatal("talk grant bypass")
	}
	if calls != 1 {
		t.Fatal("revoked command reached executor")
	}
}
