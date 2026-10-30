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

type targetIn struct {
	ID   int64 `path:"id" minimum:"1" doc:"Resource ID."`
	Body targetBody
}

type targetCreateIn struct {
	Body targetBody
}

func (d Deps) validateTargetErr(ctx context.Context, b *targetBody) error {
	c := &errCapture{}
	if d.validateTarget(ctx, c, b) {
		return nil
	}
	return c.err()
}

func (d Deps) listTargets(ctx context.Context, _ *struct{}) (*jsonOut[[]store.Target], error) {
	targets, err := d.Store.ListTargets(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "list targets failed", err)
	}
	return &jsonOut[[]store.Target]{targets}, nil
}

func (d Deps) createTarget(ctx context.Context, in *targetCreateIn) (*jsonOut[*store.Target], error) {
	b := in.Body
	if err := d.validateTargetErr(ctx, &b); err != nil {
		return nil, err
	}
	t := &store.Target{Name: b.Name, Engine: b.Engine, Enabled: b.Enabled,
		QueueID: b.QueueID, Options: b.Options, Thresholds: b.Thresholds}
	id, err := d.Store.CreateTarget(ctx, t)
	if err != nil {
		return nil, internalErr(d.Logger, "create target failed", err)
	}
	created, err := d.Store.GetTarget(ctx, id)
	if e := storeErr(d.Logger, "target", err); e != nil {
		return nil, e
	}
	return &jsonOut[*store.Target]{created}, nil
}

func (d Deps) getTarget(ctx context.Context, in *idIn) (*jsonOut[*store.Target], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	t, err := d.Store.GetTarget(ctx, in.ID)
	if e := storeErr(d.Logger, "target", err); e != nil {
		return nil, e
	}
	return &jsonOut[*store.Target]{t}, nil
}

func (d Deps) updateTarget(ctx context.Context, in *targetIn) (*jsonOut[*store.Target], error) {
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	b := in.Body
	if err := d.validateTargetErr(ctx, &b); err != nil {
		return nil, err
	}
	t := &store.Target{ID: in.ID, Name: b.Name, Engine: b.Engine, Enabled: b.Enabled,
		QueueID: b.QueueID, Options: b.Options, Thresholds: b.Thresholds}
	if err := d.Store.UpdateTarget(ctx, t); err != nil {
		return nil, storeErr(d.Logger, "target", err)
	}
	updated, err := d.Store.GetTarget(ctx, in.ID)
	if e := storeErr(d.Logger, "target", err); e != nil {
		return nil, e
	}
	return &jsonOut[*store.Target]{updated}, nil
}

func (d Deps) deleteTarget(ctx context.Context, in *idIn) (*struct{}, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if err := d.Store.DeleteTarget(ctx, in.ID); err != nil {
		return nil, storeErr(d.Logger, "target", err)
	}
	return nil, nil
}

func (d Deps) runTarget(ctx context.Context, in *idIn) (*jsonOut[RunQueued], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if _, err := d.Store.GetTarget(ctx, in.ID); err != nil {
		return nil, storeErr(d.Logger, "target", err)
	}
	runID, err := d.Runner.Enqueue(ctx, runner.RunRequest{
		Trigger: "manual", TargetIDs: []int64{in.ID}})
	if err != nil {
		return nil, enqueueErr(d.Logger, err)
	}
	return &jsonOut[RunQueued]{RunQueued{RunID: runID}}, nil
}

// RunQueued is the 202 body returned when a run was enqueued.
type RunQueued struct {
	RunID int64 `json:"run_id" doc:"ID of the run that was enqueued."`
}

// TargetTestResponse is returned by POST /targets/test.
type TargetTestResponse struct {
	Engine string `json:"engine"`
	OK     bool   `json:"ok"`
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

func (d Deps) targetLatest(ctx context.Context, in *idIn) (*jsonOut[*store.Result], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	res, err := d.Store.LatestResultForTarget(ctx, in.ID)
	if e := storeErr(d.Logger, "result", err); e != nil {
		return nil, e
	}
	return &jsonOut[*store.Result]{res}, nil
}

// testTarget validates an engine/options pair without storing anything.
func (d Deps) testTarget(ctx context.Context, in *targetCreateIn) (*jsonOut[TargetTestResponse], error) {
	b := in.Body
	b.Name = "validation" // name is irrelevant for a validation-only call
	if err := d.validateTargetErr(ctx, &b); err != nil {
		return nil, err
	}
	return &jsonOut[TargetTestResponse]{TargetTestResponse{Engine: b.Engine, OK: true}}, nil
}
