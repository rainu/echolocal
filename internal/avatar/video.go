package avatar

import (
	"context"
	"errors"
	"fmt"
	"github.com/ygelfand/echolocal/internal/devicefs"
	"github.com/ygelfand/echolocal/internal/layout"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

const MaxBackgroundVideoBytes = 64 << 20

func isBackgroundVideo(name string) bool { return strings.EqualFold(path.Ext(name), ".mp4") }

// Probe uses the same bounded video-only decoder as playback, without a GPU,
// audio output, network access or shell. Keep errors independent of file paths.
var probeBackgroundVideo = func(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, layout.AvatarRenderer, "--probe-background", name)
	out, err := cmd.Output()
	if err != nil || !strings.HasPrefix(string(out), "AVATAR_VIDEO_OK ") {
		return errors.New("background_video_unsupported")
	}
	return nil
}

// Pin a private, unlinked encoded snapshot for the renderer lifetime, rather
// than retaining every decoded frame. Source edits cannot change the session.
func resolveBackgroundVideo(root, dir, name string) (string, func(), error) {
	sceneDecodeMu.Lock()
	defer sceneDecodeMu.Unlock()
	noop := func() {}
	if ValidateBackgroundPath(name) != nil {
		return "", noop, errors.New("background_invalid")
	}
	src, err := devicefs.OpenRegular(root, name)
	if err != nil {
		return "", noop, errors.New("background_unavailable")
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil || st.Size() < 16 || st.Size() > MaxBackgroundVideoBytes {
		return "", noop, errors.New("background_video_unsupported")
	}
	var magic [12]byte
	if _, err = io.ReadFull(src, magic[:]); err != nil || string(magic[4:8]) != "ftyp" {
		return "", noop, errors.New("background_video_unsupported")
	}
	if _, err = src.Seek(0, 0); err != nil {
		return "", noop, errors.New("background_unavailable")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", noop, errors.New("background_storage")
	}
	f, err := os.CreateTemp(dir, ".background-video-*")
	if err != nil {
		return "", noop, errors.New("background_storage")
	}
	release := func() { _ = f.Close() }
	if err = os.Remove(f.Name()); err != nil {
		release()
		_ = os.Remove(f.Name())
		return "", noop, errors.New("background_storage")
	}
	n, err := io.Copy(f, io.LimitReader(src, MaxBackgroundVideoBytes+1))
	if err != nil || n != st.Size() || n > MaxBackgroundVideoBytes {
		release()
		return "", noop, errors.New("background_storage")
	}
	name = fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), f.Fd())
	if err = probeBackgroundVideo(name); err != nil {
		release()
		return "", noop, err
	}
	return name, release, nil
}
