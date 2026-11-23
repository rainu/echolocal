package talk

import "sync"

// ActivationReport contains only bounded lifecycle/audio metadata. It never
// retains conversation, target/voice IDs, credentials or media payloads.
type ActivationReport struct {
	EndedAtMS                                                         int64
	State                                                             State
	Transport                                                         Diagnostics
	Captured, InputDropped, Decoded, PlayoutDropped, SpeakerUnderruns uint64
	MaxReceiveGapMS, MaxPlayoutGapMS                                  int64
}
type History struct {
	mu      sync.Mutex
	reports []ActivationReport
}

func (h *History) Add(r ActivationReport) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.reports) == 6 {
		copy(h.reports, h.reports[1:])
		h.reports = h.reports[:5]
	}
	h.reports = append(h.reports, r)
}
func (h *History) Snapshot() []ActivationReport {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]ActivationReport(nil), h.reports...)
}
