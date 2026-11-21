package echolocal

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/layout"
	"os"
	"strings"
	"testing"
)

type avatarRemote struct {
	files                  map[string][]byte
	badHash, failSelection bool
	selected               bool
}

func (*avatarRemote) Getprop(string) (string, error) { return "athena", nil }
func (r *avatarRemote) WriteFile(p string, b []byte, mode os.FileMode) error {
	if mode != 0600 {
		return fmt.Errorf("wrong mode")
	}
	r.files[p] = append([]byte(nil), b...)
	return nil
}
func (r *avatarRemote) Shell(cmd string) (string, error) {
	if strings.HasPrefix(cmd, "sha256sum ") {
		if r.badHash {
			return "mismatch", nil
		}
		b := r.files[strings.TrimPrefix(cmd, "sha256sum ")]
		return fmt.Sprintf("%x", sha256.Sum256(b)), nil
	}
	if strings.HasPrefix(cmd, "cd ") {
		if r.failSelection {
			return "", fmt.Errorf("selection failed")
		}
		r.files[layout.AvatarDir+"/previous.json"] = r.files[layout.AvatarDir+"/current.json"]
		r.files[layout.AvatarDir+"/current.json"] = r.files[layout.AvatarDir+"/current.new.json"]
		r.selected = true
	}
	return "", nil
}
func importModel() []byte {
	b := []byte(`{"extensions":{"VRMC_vrm":{"meta":{"name":"Asset","authors":["Artist"]}}}}`)
	for len(b)%4 != 0 {
		b = append(b, ' ')
	}
	h := make([]byte, 20)
	copy(h, "glTF")
	binary.LittleEndian.PutUint32(h[4:], 2)
	binary.LittleEndian.PutUint32(h[8:], uint32(len(b)+20))
	binary.LittleEndian.PutUint32(h[12:], uint32(len(b)))
	copy(h[16:], "JSON")
	return append(h, b...)
}
func TestAvatarImportSelectsVerifiedAssetsLast(t *testing.T) {
	model := importModel()
	a, e := avatar.PrepareAssets(model, nil)
	if e != nil {
		t.Fatal(e)
	}
	r := &avatarRemote{files: map[string][]byte{layout.AvatarDir + "/current.json": []byte("old selection")}}
	if e = installAvatarAssets(r, a, model, nil); e != nil {
		t.Fatal(e)
	}
	var selected avatar.AssetSet
	if e = json.Unmarshal(r.files[layout.AvatarDir+"/current.json"], &selected); e != nil || selected != a || !r.selected {
		t.Fatal(selected, e)
	}
	if string(r.files[layout.AvatarDir+"/previous.json"]) != "old selection" {
		t.Fatal("lost previous selection")
	}
	for _, bad := range []bool{false, true} {
		r = &avatarRemote{files: map[string][]byte{layout.AvatarDir + "/current.json": []byte("keep")}, badHash: bad, failSelection: !bad}
		if e = installAvatarAssets(r, a, model, nil); e == nil || r.selected || string(r.files[layout.AvatarDir+"/current.json"]) != "keep" {
			t.Fatal("failed import replaced selection", e)
		}
	}
}
