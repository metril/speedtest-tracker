package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/metril/speedtest-tracker/internal/runner"
	"github.com/metril/speedtest-tracker/internal/settings"
	"github.com/metril/speedtest-tracker/internal/store"
)

// isJSONNull reports whether m is exactly the JSON literal null: decoding a
// request body's "options": null (or "thresholds": null) into a
// json.RawMessage field yields the 4-byte literal "null", not an empty/nil
// slice, so callers must check for it explicitly to fall back to the
// default document instead of forwarding the literal bytes to the engine.
func isJSONNull(m json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(m), []byte("null"))
}

// Runner is the subset of *runner.Runner the API needs.
type Runner interface {
	Enqueue(ctx context.Context, req runner.RunRequest) (int64, error)
	Cancel(runID int64) bool
}

// targetBody is the request payload for target create/update.
type targetBody struct {
	Name    string `json:"name"`
	Engine  string `json:"engine"`
	Enabled bool   `json:"enabled"`
	QueueID int64  `json:"queue_id"`
	// Lane is a deprecated alias for QueueID from v0.6.0 clients, still
	// accepted (resolved by queue name) so an old client sending
	// {"lane":"lan"} isn't rejected by decodeJSON's DisallowUnknownFields.
	// queue_id, when present, always wins.
	Lane       string          `json:"lane,omitempty"`
	Options    json.RawMessage `json:"options"`
	Thresholds json.RawMessage `json:"thresholds"`
}

// pathID parses the {id} URL parameter, answering 400 when it is not an
// integer. It reports whether parsing succeeded.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		errBadRequest(w, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// validateTarget checks required fields, resolves queue_id (defaulting to
// the wan queue when absent, and rejecting an id that names no queue) and
// asks the engine to validate the option document. It reports whether the
// target is usable.
func (d Deps) validateTarget(ctx context.Context, w http.ResponseWriter, b *targetBody) bool {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		errBadRequest(w, "name is required")
		return false
	}
	if b.QueueID == 0 && b.Lane != "" {
		q, err := d.Store.GetQueueByName(ctx, b.Lane)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				errBadRequest(w, "unknown lane")
				return false
			}
			internalError(w, d.Logger, "resolve lane failed", err)
			return false
		}
		b.QueueID = q.ID
	}
	if b.QueueID == 0 {
		id, err := d.Store.DefaultQueueID(ctx)
		if err != nil {
			internalError(w, d.Logger, "resolve default queue failed", err)
			return false
		}
		b.QueueID = id
	} else if _, err := d.Store.GetQueue(ctx, b.QueueID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			errBadRequest(w, "unknown queue_id")
			return false
		}
		internalError(w, d.Logger, "resolve queue failed", err)
		return false
	}
	if len(b.Options) == 0 || isJSONNull(b.Options) {
		b.Options = json.RawMessage(`{}`)
	}
	if len(b.Thresholds) == 0 || isJSONNull(b.Thresholds) {
		b.Thresholds = json.RawMessage(`{}`)
	}
	var th settings.Thresholds
	if err := json.Unmarshal(b.Thresholds, &th); err != nil {
		errBadRequest(w, "invalid thresholds: "+err.Error())
		return false
	}
	if err := validateThresholds(th); err != nil {
		errBadRequest(w, "invalid thresholds: "+err.Error())
		return false
	}
	eng, ok := d.Registry.Get(b.Engine)
	if !ok {
		errBadRequest(w, "unknown engine "+strconv.Quote(b.Engine)+"; have "+strings.Join(d.Registry.Names(), ", "))
		return false
	}
	if err := eng.Validate(b.Options); err != nil {
		errBadRequest(w, "invalid options for engine "+b.Engine+": "+err.Error())
		return false
	}
	return true
}

func (d Deps) listTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := d.Store.ListTargets(r.Context())
	if err != nil {
		internalError(w, d.Logger, "list targets failed", err)
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (d Deps) createTarget(w http.ResponseWriter, r *http.Request) {
	var b targetBody
	if !decodeJSON(w, r, &b) || !d.validateTarget(r.Context(), w, &b) {
		return
	}
	t := &store.Target{Name: b.Name, Engine: b.Engine, Enabled: b.Enabled,
		QueueID: b.QueueID, Options: b.Options, Thresholds: b.Thresholds}
	id, err := d.Store.CreateTarget(r.Context(), t)
	if err != nil {
		internalError(w, d.Logger, "create target failed", err)
		return
	}
	created, err := d.Store.GetTarget(r.Context(), id)
	if storeError(w, d.Logger, "target", err) {
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (d Deps) getTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := d.Store.GetTarget(r.Context(), id)
	if storeError(w, d.Logger, "target", err) {
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (d Deps) updateTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var b targetBody
	if !decodeJSON(w, r, &b) || !d.validateTarget(r.Context(), w, &b) {
		return
	}
	t := &store.Target{ID: id, Name: b.Name, Engine: b.Engine, Enabled: b.Enabled,
		QueueID: b.QueueID, Options: b.Options, Thresholds: b.Thresholds}
	if err := d.Store.UpdateTarget(r.Context(), t); err != nil {
		storeError(w, d.Logger, "target", err)
		return
	}
	updated, err := d.Store.GetTarget(r.Context(), id)
	if storeError(w, d.Logger, "target", err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (d Deps) deleteTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := d.Store.DeleteTarget(r.Context(), id); err != nil {
		storeError(w, d.Logger, "target", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) runTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := d.Store.GetTarget(r.Context(), id); storeError(w, d.Logger, "target", err) {
		return
	}
	runID, err := d.Runner.Enqueue(r.Context(), runner.RunRequest{
		Trigger: "manual", TargetIDs: []int64{id}})
	if err != nil {
		enqueueError(w, d.Logger, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int64{"run_id": runID})
}

// enqueueError maps runner errors onto status codes.
func enqueueError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, runner.ErrNoTargets):
		errBadRequest(w, "no runnable targets")
	case errors.Is(err, runner.ErrQueueFull):
		writeError(w, http.StatusServiceUnavailable, "queue_full", "queue is full, try again shortly")
	case errors.Is(err, runner.ErrShuttingDown):
		writeError(w, http.StatusServiceUnavailable, "shutting_down", "server is shutting down")
	default:
		internalError(w, logger, "enqueue run failed", err)
	}
}

func (d Deps) targetLatest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := d.Store.LatestResultForTarget(r.Context(), id)
	if storeError(w, d.Logger, "result", err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// testTarget validates an engine/options pair without storing anything.
func (d Deps) testTarget(w http.ResponseWriter, r *http.Request) {
	var b targetBody
	if !decodeJSON(w, r, &b) {
		return
	}
	b.Name = "validation" // name is irrelevant for a validation-only call
	if !d.validateTarget(r.Context(), w, &b) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "engine": b.Engine})
}
