package talk

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHistoryKeepsSixContentFreeIndependentSnapshots(t *testing.T) {
	var h History
	for i := 0; i < 10; i++ {
		h.Add(ActivationReport{EndedAtMS: int64(i), State: Ended, Transport: Diagnostics{Sent: uint64(i)}})
	}
	got := h.Snapshot()
	if len(got) != 6 || got[0].EndedAtMS != 4 || got[5].Transport.Sent != 9 {
		t.Fatal("history is unbounded or out of order")
	}
	got[0].State = Failed
	if h.Snapshot()[0].State != Ended {
		t.Fatal("snapshot aliases live state")
	}
	raw, _ := json.Marshal(h.Snapshot())
	for _, field := range []string{"sessionKey", "voiceSessionId", "transcript", "ClientSecret", "OfferURL"} {
		if strings.Contains(string(raw), field) {
			t.Fatal("content/signaling field in diagnostic history")
		}
	}
}
