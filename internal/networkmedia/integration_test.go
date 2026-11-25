package networkmedia

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLiveSMBReadRangesFailuresAndCancellation(t *testing.T) {
	configPath := os.Getenv("ECHOLOCAL_SMB_TEST_CONFIG")
	if configPath == "" {
		t.Skip("set ECHOLOCAL_SMB_TEST_CONFIG to an isolated fixture configuration")
	}
	b, e := os.ReadFile(configPath)
	if e != nil {
		t.Fatal("fixture config unavailable")
	}
	var cfg saved
	if json.Unmarshal(b, &cfg) != nil {
		t.Fatal("fixture config malformed")
	}
	s := fixture(t)
	password := cfg.Password
	if s.Save(Update{Settings: cfg.Settings, Password: &password}) != nil {
		t.Fatal("fixture configuration invalid")
	}
	tr := NewTransport(s, nil)
	client := &http.Client{Transport: tr}
	read := func(name, rangeValue string) (*http.Response, []byte, error) {
		req, _ := http.NewRequest("GET", "https://smb.media.invalid/"+name, nil)
		req.Header.Set("Range", rangeValue)
		r, e := client.Do(req)
		if e != nil {
			return nil, nil, e
		}
		defer r.Body.Close()
		data, e := io.ReadAll(r.Body)
		return r, data, e
	}
	r, full, e := read("sample.mp4", "bytes=0-")
	if e != nil || r.StatusCode != 206 || len(full) != 1179184 {
		t.Fatal("representative SMB file unreadable", e)
	}
	sum := sha256.Sum256(full)
	if hex.EncodeToString(sum[:]) != "71dbbf4f0256e4ccdde77419f5162d77f572572f975f45be76a7049f13e95fce" {
		t.Fatal("fixture bytes differ")
	}
	for _, v := range []struct {
		raw        string
		start, end int
	}{{"bytes=0-31", 0, 32}, {"bytes=1000-1031", 1000, 1032}, {"bytes=1179152-", 1179152, 1179184}} {
		r, b, e = read("sample.mp4", v.raw)
		if e != nil || r.StatusCode != 206 || !bytes.Equal(b, full[v.start:v.end]) {
			t.Fatal("range mismatch", v.raw, e)
		}
	}
	if _, _, e = read("folder", "bytes=0-"); e == nil || !strings.Contains(tr.Error(), "folder") || !strings.Contains(tr.Error(), "code:") {
		t.Fatal("directory error category", tr.Error())
	}
	if _, _, e = read("missing.mp4", "bytes=0-"); e == nil || !strings.Contains(tr.Error(), "not found") {
		t.Fatal("missing-file category")
	}
	if e = s.Save(Update{Settings: cfg.Settings, Password: func() *string { v := "wrong-synthetic-password"; return &v }()}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = read("sample.mp4", "bytes=0-31"); e == nil || !strings.Contains(tr.Error(), "sign-in") {
		t.Fatal("credential error category")
	}
	changed := cfg.Settings
	changed.Share = "missing-fixture"
	if e = s.Save(Update{Settings: changed, Password: &password}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = read("sample.mp4", "bytes=0-31"); e == nil || !strings.Contains(tr.Error(), "share") {
		t.Fatal("share error category")
	}
	changed = cfg.Settings
	changed.Port++
	if e = s.Save(Update{Settings: changed}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = read("sample.mp4", "bytes=0-31"); e == nil || !strings.Contains(tr.Error(), "ECONNREFUSED") {
		t.Fatal("offline error category")
	}
	if e = s.Save(Update{Settings: cfg.Settings}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = read("sample.mp4", "bytes=0-31"); e != nil {
		t.Fatal("reconnection after failed source", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://smb.media.invalid/sample.mp4", nil)
	if _, e = tr.RoundTrip(req); e == nil || len(tr.slots) != 0 {
		t.Fatal("cancel leaked request")
	}
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("unchanged HTTP")) }))
	defer fallback.Close()
	response, e := client.Get(fallback.URL)
	if e != nil {
		t.Fatal(e)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(data) != "unchanged HTTP" {
		t.Fatal("HTTP fallback changed")
	}
	if strings.Contains(tr.Error(), password) || strings.Contains(tr.Error(), cfg.Host) {
		t.Fatal("unsafe diagnostics")
	}
}
