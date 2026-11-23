package avatar

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func glb(s string) []byte {
	b := []byte(s)
	for len(b)%4 != 0 {
		b = append(b, ' ')
	}
	v := make([]byte, 20)
	copy(v, "glTF")
	binary.LittleEndian.PutUint32(v[4:], 2)
	binary.LittleEndian.PutUint32(v[8:], uint32(20+len(b)))
	binary.LittleEndian.PutUint32(v[12:], uint32(len(b)))
	copy(v[16:], "JSON")
	return append(v, b...)
}
func TestAssetSelectionAndIntegrity(t *testing.T) {
	model := glb(`{"extensions":{"VRMC_vrm":{"meta":{"name":"Example","authors":["Artist"]}}}}`)
	motion := glb(`{"extensions":{"VRMC_vrm_animation":{}}}`)
	a, e := PrepareAssets(model, motion)
	if e != nil {
		t.Fatal(e)
	}
	if a.Name != "Example" || a.Credit != "Artist" {
		t.Fatal(a)
	}
	dir := t.TempDir()
	raw, _ := json.Marshal(a)
	os.WriteFile(filepath.Join(dir, "current.json"), raw, 0600)
	os.WriteFile(filepath.Join(dir, a.Model+".vrm"), model, 0600)
	os.WriteFile(filepath.Join(dir, a.Motion+".vrma"), motion, 0600)
	got, mp, ap, e := ResolveAssets(dir)
	if e != nil || got != a || mp == "" || ap == "" {
		t.Fatal(got, mp, ap, e)
	}
	os.WriteFile(mp, []byte("tampered"), 0600)
	if _, _, _, e = ResolveAssets(dir); e == nil {
		t.Fatal("corrupt model accepted")
	}
	os.Remove(mp)
	os.Symlink(filepath.Join(dir, a.Motion+".vrma"), mp)
	if _, _, _, e = ResolveAssets(dir); e == nil {
		t.Fatal("symlink accepted")
	}
}
func TestAssetImportRejectsExternalAndInvalid(t *testing.T) {
	for _, b := range [][]byte{nil, []byte("bad"), glb(`{"extensions":{"VRMC_vrm":{}},"buffers":[{"uri":"https://example.invalid/a"}]}`), glb(`{"extensions":{"VRMC_vrm":{}},"images":[{"uri":"file:///private"}]}`), glb(`{"extensions":{"other":{}}}`)} {
		if _, e := PrepareAssets(b, nil); e == nil {
			t.Fatal("invalid model accepted")
		}
	}
	model := glb(`{"extensions":{"VRM":{"meta":{"title":"Legacy","author":"Author"}}}}`)
	a, e := PrepareAssets(model, nil)
	if e != nil || a.Motion != "" || a.Name != "Legacy" {
		t.Fatal(a, e)
	}
	if _, e = PrepareAssets(model, model); e == nil {
		t.Fatal("non-VRMA motion accepted")
	}
	for _, bad := range []string{"../private", strings.Repeat("a", 63), strings.Repeat("Z", 64)} {
		if validDigest(bad) {
			t.Fatal(bad)
		}
	}
}
func TestSelectionManifestIsStrict(t *testing.T) {
	dir := t.TempDir()
	for _, raw := range []string{`{}`, `{"version":1,"model":"../bad"}`, `{"version":1,"model":"` + strings.Repeat("a", 64) + `","token":"not-a-setting"}`, `{} {}`} {
		os.WriteFile(filepath.Join(dir, "current.json"), []byte(raw), 0600)
		if _, e := ReadSelection(dir); e == nil {
			t.Fatal("invalid manifest accepted")
		}
	}
}
