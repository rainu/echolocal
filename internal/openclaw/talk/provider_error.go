package talk

import (
	"encoding/json"
	"time"
)

// Give the native channel a bounded chance to supply structured details before
// accepting the Gateway's deliberately redacted generic provider error.
const providerErrorDetailWait = time.Second

type providerErrorEvent struct {
	Code   string
	Status json.RawMessage
	Error  struct {
		Code, Type, Param, Message string
		Status                     json.RawMessage
	}
}

// This is an allowlist, not a sanitizer for arbitrary provider-controlled text.
func safeProviderErrorToken(value string) string {
	switch value {
	case "":
		return ""
	case "server_error", "internal_error", "internal_server_error", "service_unavailable",
		"rate_limit_error", "rate_limit_exceeded", "session_expired", "session_timeout",
		"session_duration_exceeded", "max_session_duration_exceeded",
		"authentication_error", "permission_error", "invalid_api_key", "invalid_token",
		"token_expired", "insufficient_quota", "quota_exceeded", "permission_denied",
		"content_policy_violation", "invalid_request_error", "invalid_value",
		"unknown_parameter", "unknown_event", "unsupported_event",
		"type", "channel", "content", "delegation_id":
		return value
	}
	return "other"
}

func providerAuthStatus(status json.RawMessage) bool {
	switch string(status) {
	case "401", "403", `"401"`, `"403"`:
		return true
	}
	return false
}

// A native error is not automatically a rejected control request. Explicit
// transient classes may replace voice, never replay inputs. Authentication,
// quota/policy, invalid/unknown requests and contradictory errors fail closed.
func classifyNativeProviderError(e providerErrorEvent) (reason, code, kind string) {
	rawCode := e.Error.Code
	if rawCode == "" {
		rawCode = e.Code
	}
	code, kind = safeProviderErrorToken(rawCode), safeProviderErrorToken(e.Error.Type)
	reason = "talk_control_rejected"
	if providerAuthStatus(e.Status) || providerAuthStatus(e.Error.Status) ||
		kind == "authentication_error" || kind == "permission_error" {
		return
	}
	switch code {
	case "authentication_error", "invalid_api_key", "invalid_token", "token_expired",
		"insufficient_quota", "quota_exceeded", "permission_denied", "content_policy_violation",
		"invalid_request_error", "invalid_value", "unknown_parameter", "unknown_event", "unsupported_event":
		return
	}
	switch kind {
	case "", "server_error", "invalid_request_error", "rate_limit_error":
	default:
		return
	}
	switch code {
	case "session_expired", "session_timeout", "session_duration_exceeded", "max_session_duration_exceeded":
		reason = "provider_session_expired"
		return
	case "rate_limit_exceeded":
		reason = "provider_rate_limited"
		return
	}
	if kind == "rate_limit_error" {
		reason = "provider_rate_limited"
		return
	}
	if kind == "server_error" || (kind == "" &&
		(code == "server_error" || code == "internal_error" || code == "internal_server_error" || code == "service_unavailable")) {
		reason = "provider_transient_error"
	}
	return
}
