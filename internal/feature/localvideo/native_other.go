//go:build !board_athena

package localvideo

import "github.com/ygelfand/libcountertop/pkg/media/urlplayer"

func nativeDecoder() *urlplayer.Native { return nil }
