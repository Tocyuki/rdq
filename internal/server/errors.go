package server

import (
	"encoding/json"
	"net/http"
)

// Error codes are a small, stable enum used across endpoints so the SPA can
// branch on them without parsing free-form messages.
const (
	errCodeBadRequest      = "bad_request"
	errCodeNotFound        = "not_found"
	errCodeOriginDenied    = "origin_denied"
	errCodeRequestTooLarge = "request_too_large"
	errCodeUnauthorized    = "unauthorized"
	errCodeTimeout         = "timeout"
	errCodeAWSError        = "aws_error"
	errCodeInternal        = "internal"
	errCodeReadOnly        = "read_only"
)

// writeJSONError serializes an ErrorDTO with the given status, code, and
// message. It is a no-op if the client has already disconnected (the
// underlying Encode would fail silently).
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorDTO{
		Error: ErrorDetail{Code: code, Message: message},
	})
}

// writeJSON serializes v as JSON with a 200 status. Callers that need a
// different status should set it before calling (or use writeJSONError).
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}
