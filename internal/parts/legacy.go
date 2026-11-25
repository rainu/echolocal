//go:build board_athena && payload

package parts

import (
	_ "embed"
	"github.com/ygelfand/echolocal/internal/layout"
)

//go:embed payload/echolocal-legacy
var legacyDecoder []byte

//go:embed payload/echolocal-legacy.LICENSE
var legacyLicense []byte

//go:embed payload/echolocal-legacy.FFmpeg.LICENSE
var legacyFFmpegLicense []byte

//go:embed payload/echolocal-legacy.NOTICE
var legacyNotice []byte

func init() {
	register(Part{Name: "legacy video decoder", Path: layout.LegacyDecoder, Data: legacyDecoder, Mode: 0o755})
	register(Part{Name: "legacy decoder license", Path: layout.LegacyDecoder + ".LICENSE", Data: legacyLicense, Mode: 0o644})
	register(Part{Name: "FFmpeg license", Path: layout.LegacyDecoder + ".FFmpeg.LICENSE", Data: legacyFFmpegLicense, Mode: 0o644})
	register(Part{Name: "legacy decoder notices", Path: layout.LegacyDecoder + ".NOTICE", Data: legacyNotice, Mode: 0o644})
}
