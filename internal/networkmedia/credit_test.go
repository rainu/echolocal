package networkmedia

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Opt in only against a strict isolated server granting fewer than 16 credits,
// advertising at least 1 MiB MaxTransactSize, and closing undercredited queries.
func TestLiveSMBLowCreditMetadataAndLargeRanges(t *testing.T) {
	if os.Getenv("ECHOLOCAL_SMB_STRICT_CREDITS") != "1" {
		t.Skip("strict isolated fixture required")
	}
	raw, e := os.ReadFile(os.Getenv("ECHOLOCAL_SMB_TEST_CONFIG"))
	if e != nil {
		t.Fatal("fixture configuration unavailable")
	}
	var cfg saved
	if json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("fixture configuration malformed")
	}
	s := fixture(t)
	if e = s.Save(Update{Settings: cfg.Settings, Password: &cfg.Password}); e != nil {
		t.Fatal(e)
	}
	tr := NewTransport(s, nil)
	client := &http.Client{Transport: tr}
	// The 4 GiB boundary matters on the 32-bit ARM playback target.
	const size int64 = 1<<32 + 128
	req, _ := http.NewRequest("GET", "https://smb.media.invalid/large.bin", nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", size-32))
	r, e := client.Do(req)
	if e != nil {
		t.Fatal("low-credit stat/read regression", e)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if e != nil || r.StatusCode != 206 || r.ContentLength != 32 || string(b) != "0123456789ABCDEF0123456789ABCDEF" {
		t.Fatal("64-bit range body mismatch", e)
	}
	want := fmt.Sprintf("bytes %d-%d/%d", size-32, size-1, size)
	if r.Header.Get("Content-Range") != want {
		t.Fatal("64-bit Content-Range mismatch")
	}
	// Directory rejection must remain a metadata check, not a successful open.
	req, _ = http.NewRequest("GET", "https://smb.media.invalid/folder", nil)
	if _, e = client.Do(req); e == nil || !strings.Contains(tr.Error(), "NOT_A_REGULAR_FILE") {
		t.Fatal("non-regular file check was weakened", tr.Error())
	}
}
