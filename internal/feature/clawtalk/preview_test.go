//go:build !board_biscuit

package clawtalk

import (
	"bytes"
	"encoding/binary"
	"github.com/ygelfand/echolocal/internal/hardware/speaker"
	"testing"
)

func TestBrowserPreviewProducesAudioWithoutPlayback(t *testing.T) {
	for _, cue := range []string{"ready", "progress", "end"} {
		for _, choice := range []string{"", "none", "chirp"} {
			wav := PreviewAudio(cue, choice)
			if len(wav) <= 44 || string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " {
				t.Fatal("invalid WAV")
			}
			if binary.LittleEndian.Uint32(wav[4:]) != uint32(len(wav)-8) || binary.LittleEndian.Uint32(wav[40:]) != uint32(len(wav)-44) {
				t.Fatal("wrong length")
			}
			if binary.LittleEndian.Uint32(wav[24:]) != speaker.Rate || binary.LittleEndian.Uint16(wav[22:]) != speaker.Channels {
				t.Fatal("wrong format")
			}
			if choice == "none" && !bytes.Equal(wav[44:], make([]byte, len(wav)-44)) {
				t.Fatal("none is audible")
			}
			if choice == "" && bytes.Equal(wav[44:], make([]byte, len(wav)-44)) {
				t.Fatal("default is silent")
			}
		}
	}
	if bytes.Equal(PreviewAudio("ready", ""), PreviewAudio("end", "")) {
		t.Fatal("start/end collapsed")
	}
}
