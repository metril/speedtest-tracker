package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/metril/speedtest-tracker/internal/store"
)

// ErrorResponse is the JSON error envelope every handler returns.
type ErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// writeJSON encodes v with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Default().Error("encode response", "error", err)
	}
}

// writeError writes the {error:{code,message}} envelope.
func writeError(w http.ResponseWriter, status int, code, message string) {
	var b ErrorResponse
	b.Error.Code = code
	b.Error.Message = message
	writeJSON(w, status, b)
}

// errBadRequest reports a validation failure.
func errBadRequest(w http.ResponseWriter, message string) {
	writeError(w, http.StatusBadRequest, "invalid_request", message)
}

// errNotFound reports a missing resource.
func errNotFound(w http.ResponseWriter, message string) {
	writeError(w, http.StatusNotFound, "not_found", message)
}

// errForbidden reports that the caller's identity is not permitted to
// perform the request.
func errForbidden(w http.ResponseWriter, message string) {
	writeError(w, http.StatusForbidden, "forbidden", message)
}

// internalError logs err and reports a generic failure, never leaking the
// underlying message to the client.
func internalError(w http.ResponseWriter, logger *slog.Logger, msg string, err error) {
	logger.Error(msg, "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", msg)
}

// storeError maps store.ErrNotFound to 404 and anything else to 500.
// It reports whether it handled err.
func storeError(w http.ResponseWriter, logger *slog.Logger, what string, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, store.ErrNotFound) {
		errNotFound(w, what+" not found")
		return true
	}
	internalError(w, logger, what+" lookup failed", err)
	return true
}

// decodeJSON reads a JSON request body, answering 415 on a non-JSON
// Content-Type and 400 on malformed input.
// It reports whether decoding succeeded.

// decodeOptionalJSON is decodeJSON for endpoints whose body may be absent.
// An empty body (Content-Length 0, or an unknown length - chunked - that
// turns out to be empty) leaves dst untouched and succeeds.
