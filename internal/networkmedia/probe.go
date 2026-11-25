package networkmedia

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	smb2 "github.com/cloudsoda/go-smb2"
)

type TestResult struct {
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func testResult(code string) TestResult {
	return TestResult{OK: code == "test_ok", Code: code, Message: Labels()[code]}
}

// Test checks an unsaved draft. It never writes settings or invokes Changed.
func (s *Store) Test(parent context.Context, u Update) TestResult {
	u.Enabled = true // Testing is independent of the saved source's enabled switch.
	if !valid(u.Settings) || u.ClearPassword || u.Password != nil && (len(*u.Password) == 0 || len(*u.Password) > 1024) {
		return testResult("test_invalid")
	}
	cfg, loadErr := s.snapshot()
	if loadErr != nil && u.Password == nil {
		return testResult("test_invalid")
	}
	password := cfg.Password
	if u.Password != nil {
		password = *u.Password
	}
	if password == "" {
		return testResult("test_invalid")
	}
	select {
	case s.testSlots <- struct{}{}:
		defer func() { <-s.testSlots }()
	default:
		return testResult("test_busy")
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	failed := func(e error, code string) TestResult {
		e = networkCause(e)
		if errors.Is(ctx.Err(), context.Canceled) {
			return testResult("test_cancelled")
		}
		var timeout net.Error
		if !time.Now().Before(deadline) || errors.Is(e, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.As(e, &timeout) && timeout.Timeout() {
			r := testResult("test_timeout")
			r.Message = category(context.DeadlineExceeded, map[string]string{"test_connect": "connect", "test_auth": "auth", "test_share": "mount", "test_access": "metadata"}[code])
			return r
		}
		r := testResult(code)
		r.Message = category(e, map[string]string{"test_connect": "connect", "test_auth": "auth", "test_share": "mount", "test_access": "metadata"}[code])
		return r
	}
	conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(u.Host, strconv.Itoa(u.Port)))
	if e != nil {
		return failed(e, "test_connect")
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	closeConn := sync.OnceFunc(func() { stop(); _ = conn.Close() })
	defer closeConn()
	_ = conn.SetDeadline(deadline)
	d := smb2.Dialer{MaxCreditBalance: 16, Negotiator: smb2.Negotiator{RequireMessageSigning: true}, Initiator: &smb2.NTLMInitiator{User: u.Username, Password: password, Domain: u.Domain}}
	session, e := d.DialConn(ctx, conn, u.Host)
	if e != nil {
		return failed(e, "test_auth")
	}
	share, e := session.WithContext(ctx).Mount(u.Share)
	if e != nil {
		return failed(e, "test_share")
	}
	// Check root metadata only: no file enumeration, creation or media playback.
	if _, e = share.WithContext(ctx).Stat("."); e != nil {
		return failed(e, "test_access")
	}
	return testResult("test_ok")
}
