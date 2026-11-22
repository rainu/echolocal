package avatar

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ygelfand/echolocal/internal/devicefs"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"
)

func ValidatePath(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > 255 || !utf8.ValidString(value) || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\x00\\") || !strings.EqualFold(path.Ext(value), ".vrm") {
		return errors.New("invalid_path")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "." || part == ".." {
			return errors.New("invalid_path")
		}
		for _, c := range part {
			if c < 32 || c == 127 {
				return errors.New("invalid_path")
			}
		}
	}
	return nil
}

// ResolveCustom takes a validated snapshot, never passes the user's path to the
// native parser, and retains at most one temporary model per renderer lifetime.
// The same root/pinned-open policy is used by browser downloads and WebDAV.
func ResolveCustom(root, dir, name string) (AssetSet, string, string, func(), error) {
	fail := func(code string) (AssetSet, string, string, func(), error) {
		return AssetSet{}, "", "", func() {}, errors.New(code)
	}
	if ValidatePath(name) != nil {
		return fail("custom_unavailable")
	}
	f, err := devicefs.OpenRegular(root, name)
	if err != nil {
		return fail("custom_unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fail("custom_unavailable")
	}
	if info.Size() <= 0 || info.Size() >= MaxModelBytes {
		return fail("custom_invalid")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxModelBytes))
	if err != nil {
		return fail("custom_unavailable")
	}
	if len(data) >= MaxModelBytes {
		return fail("custom_invalid")
	}
	doc, err := glbDocument(data, MaxModelBytes)
	if err != nil {
		return fail("custom_invalid")
	}
	var ext map[string]json.RawMessage
	if json.Unmarshal(doc["extensions"], &ext) != nil {
		return fail("custom_invalid")
	}
	version := struct {
		SpecVersion string `json:"specVersion"`
	}{}
	if value, ok := ext["VRMC_vrm"]; ok {
		if json.Unmarshal(value, &version) != nil || version.SpecVersion != "1.0" {
			return fail("custom_invalid")
		}
	} else if value, ok := ext["VRM"]; ok {
		if json.Unmarshal(value, &version) != nil || version.SpecVersion != "0.0" {
			return fail("custom_invalid")
		}
	} else {
		return fail("custom_invalid")
	}
	assets, err := PrepareAssets(data, nil)
	if err != nil {
		return fail("custom_invalid")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return fail("custom_storage")
	}
	tmp, err := os.CreateTemp(dir, ".custom-*.vrm")
	if err != nil {
		return fail("custom_storage")
	}
	cleanup := func() { _ = tmp.Close() }
	// Unlink immediately: even a daemon crash cannot leave model snapshots behind.
	if err = os.Remove(tmp.Name()); err != nil {
		cleanup()
		return fail("custom_storage")
	}
	if _, err = tmp.Write(data); err != nil {
		cleanup()
		return fail("custom_storage")
	}
	snapshot := fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), tmp.Fd())
	// Reuse only the already validated installed animation, never another path
	// embedded in the custom model. A missing default catalog does not prevent
	// loading a custom model without animation.
	var motion string
	if selected, e := ReadSelection(dir); e == nil && selected.Motion != "" {
		if file, bytes, e := assetFile(dir, selected.Motion, ".vrma", MaxMotionBytes); e == nil {
			if doc, e := glbDocument(bytes, MaxMotionBytes); e == nil {
				var ext map[string]json.RawMessage
				if json.Unmarshal(doc["extensions"], &ext) == nil && ext["VRMC_vrm_animation"] != nil {
					motion = file
				}
			}
		}
	}
	return assets, snapshot, motion, cleanup, nil
}
