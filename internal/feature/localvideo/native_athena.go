//go:build board_athena

package localvideo

import (
	"github.com/ygelfand/echolocal/internal/layout"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
)

func nativeDecoder() *urlplayer.Native { return &urlplayer.Native{Helper: layout.LegacyDecoder} }
