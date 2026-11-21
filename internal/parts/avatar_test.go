//go:build board_athena && payload

package parts

import (
	"bytes"
	"github.com/ygelfand/echolocal/internal/layout"
	"testing"
)

func TestAvatarPayloadIncludesExecutableAndLicenses(t *testing.T) {
	found := map[string]Part{}
	for _, p := range All() {
		found[p.Path] = p
	}
	helper, ok := found[layout.AvatarRenderer]
	if !ok || helper.Mode != 0755 || !bytes.HasPrefix(helper.Data, []byte{127, 69, 76, 70}) {
		t.Fatal("avatar executable missing from normal payload")
	}
	for _, suffix := range []string{".LICENSE", ".THIRD_PARTY.md"} {
		p, ok := found[layout.AvatarRenderer+suffix]
		if !ok || p.Mode != 0644 || len(p.Data) == 0 {
			t.Fatal("missing avatar notice", suffix)
		}
	}
}
