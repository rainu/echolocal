//go:build !board_biscuit

package clawtalk

import "time"

const activityDotStep = 350 * time.Millisecond

// ActivityDotCount keeps at least one dot visible: 1, 2, 3, 2, then repeat.
func ActivityDotCount(elapsed time.Duration) int {
	if elapsed < 0 {
		elapsed = 0
	}
	return [...]int{1, 2, 3, 2}[int((elapsed/activityDotStep)%4)]
}

func (p *AvatarPage) ActivityDots() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.activityPaused {
		return 3
	}
	if p.activityDots < 1 {
		return 1
	}
	return p.activityDots
}
func (p *AvatarPage) ActivityPaused() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.activityPaused
}

// Pause is presentation-only and lasts for this page/call, across layout switches.
func (p *AvatarPage) ToggleActivityMotion() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activityPaused = !p.activityPaused
	p.activityStarted = time.Time{}
	p.activityDots = 1
}
func (p *AvatarPage) SetActivityRepaint(repaint func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activityRepaint = repaint
}

// Reuse the existing Talk status tick, never a new timer or goroutine. Only
// changed dot counts request a paint-only frame; long captions are not rebuilt.
// Covered/finished pages, paused motion and idle sessions request no frames.
func (p *AvatarPage) TickActivity(now time.Time, working bool) {
	p.mu.Lock()
	if !working {
		p.activityStarted = time.Time{}
		p.activityDots = 1
		p.mu.Unlock()
		return
	}
	if !p.started || p.closed || p.activityPaused {
		p.mu.Unlock()
		return
	}
	if p.activityStarted.IsZero() {
		p.activityStarted = now
	}
	dots := ActivityDotCount(now.Sub(p.activityStarted))
	changed := dots != p.activityDots
	p.activityDots = dots
	repaint := p.activityRepaint
	p.mu.Unlock()
	if changed && repaint != nil {
		repaint()
	}
}
