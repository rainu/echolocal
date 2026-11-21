package avatar

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const ProtocolVersion = 4
const MaxModelBytes = 64 << 20
const MaxMotionBytes = 8 << 20

// AssetSet is the atomically selected, content-addressed local asset catalog.
// No caller-supplied filesystem paths or remote resources reach the renderer.
type AssetSet struct {
	Version int    `json:"version"`
	Model   string `json:"model"`
	Motion  string `json:"motion,omitempty"`
	Name    string `json:"name"`
	Credit  string `json:"credit"`
}

func cleanLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	r := []rune(s)
	if len(r) > 160 {
		r = r[:160]
	}
	return strings.TrimSpace(string(r))
}
func glbDocument(data []byte, limit int) (map[string]json.RawMessage, error) {
	if len(data) < 28 || len(data) > limit || string(data[:4]) != "glTF" || binary.LittleEndian.Uint32(data[4:]) != 2 || int(binary.LittleEndian.Uint32(data[8:])) != len(data) {
		return nil, errors.New("invalid GLB header or size")
	}
	n := int(binary.LittleEndian.Uint32(data[12:]))
	if n < 2 || n > 4<<20 || n > len(data)-20 || string(data[16:20]) != "JSON" {
		return nil, errors.New("invalid GLB JSON chunk")
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(data[20:20+n], &doc) != nil {
		return nil, errors.New("invalid GLB JSON")
	}
	for _, key := range []string{"buffers", "images"} {
		var items []map[string]json.RawMessage
		if v, ok := doc[key]; ok {
			if json.Unmarshal(v, &items) != nil {
				return nil, errors.New("invalid resources")
			}
			for _, item := range items {
				if _, ok := item["uri"]; ok {
					return nil, errors.New("external resources are not supported")
				}
			}
		}
	}
	return doc, nil
}
func PrepareAssets(model, motion []byte) (AssetSet, error) {
	doc, err := glbDocument(model, MaxModelBytes)
	if err != nil {
		return AssetSet{}, err
	}
	var ext map[string]json.RawMessage
	if json.Unmarshal(doc["extensions"], &ext) != nil {
		return AssetSet{}, errors.New("VRM extension required")
	}
	var name, credit string
	if v, ok := ext["VRMC_vrm"]; ok {
		var m struct {
			Meta struct {
				Name    string
				Authors []string
			}
		}
		if json.Unmarshal(v, &m) != nil {
			return AssetSet{}, errors.New("invalid VRM metadata")
		}
		name = m.Meta.Name
		credit = strings.Join(m.Meta.Authors, ", ")
	} else if v, ok := ext["VRM"]; ok {
		var m struct {
			Meta struct{ Title, Author string }
		}
		if json.Unmarshal(v, &m) != nil {
			return AssetSet{}, errors.New("invalid VRM metadata")
		}
		name = m.Meta.Title
		credit = m.Meta.Author
	} else {
		return AssetSet{}, errors.New("VRM extension required")
	}
	a := AssetSet{Version: 1, Model: fmt.Sprintf("%x", sha256.Sum256(model)), Name: cleanLabel(name), Credit: cleanLabel(credit)}
	if len(motion) > 0 {
		d, err := glbDocument(motion, MaxMotionBytes)
		if err != nil {
			return AssetSet{}, err
		}
		var e map[string]json.RawMessage
		if json.Unmarshal(d["extensions"], &e) != nil || e["VRMC_vrm_animation"] == nil {
			return AssetSet{}, errors.New("VRMA extension required")
		}
		a.Motion = fmt.Sprintf("%x", sha256.Sum256(motion))
	}
	if a.Name == "" {
		a.Name = "Avatar"
	}
	return a, nil
}
func validDigest(s string) bool {
	b, e := hex.DecodeString(s)
	return len(s) == 64 && s == strings.ToLower(s) && e == nil && len(b) == 32
}
func ReadSelection(dir string) (AssetSet, error) {
	p := filepath.Join(dir, "current.json")
	f, err := os.Open(p)
	if err != nil {
		return AssetSet{}, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(raw) > 4096 {
		return AssetSet{}, errors.New("invalid asset selection")
	}
	var a AssetSet
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil {
		return AssetSet{}, errors.New("invalid asset selection")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return AssetSet{}, errors.New("trailing asset selection")
	}
	if a.Version != 1 || !validDigest(a.Model) || (a.Motion != "" && !validDigest(a.Motion)) {
		return AssetSet{}, errors.New("invalid asset selection")
	}
	a.Name = cleanLabel(a.Name)
	a.Credit = cleanLabel(a.Credit)
	return a, nil
}
func assetFile(dir, hash, suffix string, limit int) (string, []byte, error) {
	p := filepath.Join(dir, hash+suffix)
	st, err := os.Lstat(p)
	if err != nil {
		return "", nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > int64(limit) {
		return "", nil, errors.New("invalid asset file")
	}
	f, err := os.Open(p)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if err != nil || len(b) > limit || fmt.Sprintf("%x", sha256.Sum256(b)) != hash {
		return "", nil, errors.New("asset integrity mismatch")
	}
	return p, b, nil
}
func ResolveAssets(dir string) (AssetSet, string, string, error) {
	a, err := ReadSelection(dir)
	if err != nil {
		return a, "", "", err
	}
	model, b, err := assetFile(dir, a.Model, ".vrm", MaxModelBytes)
	if err != nil {
		return a, "", "", err
	}
	var motion string
	var mb []byte
	if a.Motion != "" {
		motion, mb, err = assetFile(dir, a.Motion, ".vrma", MaxMotionBytes)
		if err != nil {
			return a, "", "", err
		}
	}
	checked, err := PrepareAssets(b, mb)
	if err != nil {
		return a, "", "", err
	}
	a.Name = checked.Name
	a.Credit = checked.Credit
	return a, model, motion, nil
}
