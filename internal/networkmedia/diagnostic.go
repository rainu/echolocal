package networkmedia

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	smb2 "github.com/cloudsoda/go-smb2"
)

type smbStatus struct{ name, kind string }

var smbStatuses = map[uint32]smbStatus{
	0xc0000008: {"STATUS_INVALID_HANDLE", "session"},
	0xc000000d: {"STATUS_INVALID_PARAMETER", "unsupported"},
	0xc000000f: {"STATUS_NO_SUCH_FILE", "missing"},
	0xc0000011: {"STATUS_END_OF_FILE", "truncated"},
	0xc0000022: {"STATUS_ACCESS_DENIED", "denied"},
	0xc0000033: {"STATUS_OBJECT_NAME_INVALID", "path"},
	0xc0000034: {"STATUS_OBJECT_NAME_NOT_FOUND", "missing"},
	0xc000003a: {"STATUS_OBJECT_PATH_NOT_FOUND", "missing"},
	0xc000003b: {"STATUS_OBJECT_PATH_SYNTAX_BAD", "path"},
	0xc0000043: {"STATUS_SHARING_VIOLATION", "locked"},
	0xc0000054: {"STATUS_FILE_LOCK_CONFLICT", "locked"},
	0xc0000056: {"STATUS_DELETE_PENDING", "missing"},
	0xc0000064: {"STATUS_NO_SUCH_USER", "auth"},
	0xc000006a: {"STATUS_WRONG_PASSWORD", "auth"},
	0xc000006d: {"STATUS_LOGON_FAILURE", "auth"},
	0xc0000072: {"STATUS_ACCOUNT_DISABLED", "auth"},
	0xc00000b5: {"STATUS_IO_TIMEOUT", "timeout"},
	0xc00000ba: {"STATUS_FILE_IS_A_DIRECTORY", "folder"},
	0xc00000bb: {"STATUS_NOT_SUPPORTED", "unsupported"},
	0xc00000be: {"STATUS_BAD_NETWORK_PATH", "share"},
	0xc00000c9: {"STATUS_NETWORK_NAME_DELETED", "closed"},
	0xc00000cc: {"STATUS_BAD_NETWORK_NAME", "share"},
	0xc00000d0: {"STATUS_REQUEST_NOT_ACCEPTED", "busy"},
	0xc0000103: {"STATUS_NOT_A_DIRECTORY", "path"},
	0xc0000128: {"STATUS_FILE_CLOSED", "session"},
	0xc0000203: {"STATUS_USER_SESSION_DELETED", "session"},
	0xc000020c: {"STATUS_CONNECTION_DISCONNECTED", "closed"},
	0xc000020d: {"STATUS_CONNECTION_RESET", "closed"},
	0xc0000234: {"STATUS_ACCOUNT_LOCKED_OUT", "auth"},
	0xc000035c: {"STATUS_NETWORK_SESSION_EXPIRED", "session"},
}

// The dependency's TransportError does not implement Unwrap. Never inspect its
// Error() text: wrapped paths, endpoints and credentials must remain private.
func networkCause(e error) error {
	for i := 0; i < 8; i++ {
		var transport *smb2.TransportError
		if !errors.As(e, &transport) || transport.Err == nil {
			break
		}
		e = transport.Err
	}
	return e
}

func category(e error, stage string) string {
	e = networkCause(e)
	kind, code := "unknown", "UNCLASSIFIED_ERROR"
	var response *smb2.ResponseError
	var dns *net.DNSError
	var timeout net.Error
	var invalid *smb2.InvalidResponseError
	var internal *smb2.InternalError
	switch {
	case errors.As(e, &response):
		code = fmt.Sprintf("NTSTATUS 0x%08X", response.Code)
		if s, ok := smbStatuses[response.Code]; ok {
			kind = s.kind
			code = fmt.Sprintf("%s (0x%08X)", s.name, response.Code)
		}
		if kind == "missing" && stage == "mount" {
			kind = "share"
		}
	case errors.Is(e, context.DeadlineExceeded), errors.As(e, &timeout) && timeout.Timeout():
		kind, code = "timeout", "TIMEOUT"
	case errors.As(e, &dns):
		kind, code = "dns", "DNS_LOOKUP_FAILED"
	case errors.Is(e, syscall.ECONNREFUSED):
		kind, code = "refused", "ECONNREFUSED"
	case errors.Is(e, syscall.EHOSTUNREACH), errors.Is(e, syscall.ENETUNREACH):
		kind, code = "unreachable", "NETWORK_UNREACHABLE"
	case errors.Is(e, syscall.ECONNRESET), errors.Is(e, syscall.ECONNABORTED), errors.Is(e, syscall.EPIPE):
		kind, code = "closed", "CONNECTION_RESET"
	case errors.Is(e, net.ErrClosed), errors.Is(e, io.EOF), errors.Is(e, context.Canceled):
		kind, code = "closed", "CONNECTION_CLOSED"
	case errors.Is(e, io.ErrUnexpectedEOF):
		kind, code = "truncated", "UNEXPECTED_EOF"
	case errors.Is(e, os.ErrPermission):
		kind, code = "denied", "ACCESS_DENIED"
	case errors.Is(e, os.ErrNotExist):
		kind, code = "missing", "FILE_NOT_FOUND"
	case errors.Is(e, syscall.EISDIR):
		kind, code = "folder", "FILE_IS_A_DIRECTORY"
	case errors.Is(e, syscall.ENOTDIR), errors.Is(e, syscall.EINVAL):
		kind, code = "path", "INVALID_FILE_PATH"
	case errors.Is(e, smb2.ErrRequestNotAccepted):
		kind, code = "busy", "SERVER_BUSY"
	case errors.As(e, &invalid):
		kind, code = "protocol", "INVALID_SMB_RESPONSE"
	case errors.As(e, &internal):
		kind, code = "internal", "SMB_CLIENT_ERROR"
	}
	return playbackDiagnostic(kind, stage, code)
}

// Capture the deadline before cleanup cancels the child context. Socket deadlines
// can fire just before ctx.Err() becomes visible, or be hidden by TransportError.
func deadlineError(ctx context.Context, deadline time.Time, e error) error {
	if !time.Now().Before(deadline) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return e
}
