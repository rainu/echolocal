//go:build board_athena

package localvideo

import (
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"

	"github.com/ygelfand/echolocal/internal/layout"
)

func nativeDecoder() *urlplayer.Native { return &urlplayer.Native{Helper: layout.LegacyDecoder} }
