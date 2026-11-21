//go:build board_athena && payload

package parts

import (
	_ "embed"
	"github.com/ygelfand/echolocal/internal/layout"
)

// The helper remains a separate AGPL-3.0 executable; retain its full notices.
//
//go:embed payload/echolocal-avatar
var avatarRenderer []byte

//go:embed payload/echolocal-avatar.LICENSE
var avatarLicense []byte

//go:embed payload/echolocal-avatar.THIRD_PARTY.md
var avatarNotices []byte

func init() {
	register(Part{Name: "native avatar renderer", Path: layout.AvatarRenderer, Data: avatarRenderer, Mode: 0o755})
	register(Part{Name: "avatar renderer license", Path: layout.AvatarRenderer + ".LICENSE", Data: avatarLicense, Mode: 0o644})
	register(Part{Name: "avatar renderer third-party notices", Path: layout.AvatarRenderer + ".THIRD_PARTY.md", Data: avatarNotices, Mode: 0o644})
}
