//go:build !board_biscuit

package voice

import "github.com/ygelfand/echolocal/internal/feature/clawtalk"

func routeTalk(v *Voice, slot int) bool {
	return clawtalk.Get().Route(slot, v.Turn().Busy() || v.Turn().Pending())
}
func stopTalk() bool { return clawtalk.Get().Stop() }
