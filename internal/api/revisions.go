package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/metril/speedtest-tracker/internal/store"
)

// revisionFields lists the top-level Target fields compared by
// diffChangedFields, in the order "changed" reports them.
var revisionFields = []string{"name", "engine", "enabled", "queue_id", "options", "thresholds"}

// revisionOut is one entry in GET /targets/{id}/revisions.
type revisionOut struct {
	Version   int             `json:"version"`
	Action    string          `json:"action"`
	CreatedAt string          `json:"created_at"`
	Snapshot  json.RawMessage `json:"snapshot"`
	Changed   []string        `json:"changed"`
}

// diffChangedFields reports which of revisionFields differ, semantically,
// between the two Target snapshots. Comparing decoded values (rather than
// raw bytes) avoids false positives from incidental whitespace/key-order
// differences in the stored JSON.
func diffChangedFields(cur, prev json.RawMessage) []string {
	var curFields, prevFields map[string]json.RawMessage
	if err := json.Unmarshal(cur, &curFields); err != nil {
		return nil
	}
	if err := json.Unmarshal(prev, &prevFields); err != nil {
		return nil
	}
	changed := []string{}
	for _, f := range revisionFields {
		var a, b any
		json.Unmarshal(curFields[f], &a)
		json.Unmarshal(prevFields[f], &b)
		if !reflect.DeepEqual(a, b) {
			changed = append(changed, f)
		}
	}
	return changed
}

// listTargetRevisions answers GET /targets/{id}/revisions: every revision
// for the target, newest first, each annotated with the top-level fields
// that changed versus the previous version. The first (oldest) version
// always reports an empty "changed" list.
func (d Deps) listTargetRevisions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	revs, err := d.Store.ListTargetRevisions(r.Context(), id)
	if err != nil {
		internalError(w, d.Logger, "list target revisions failed", err)
		return
	}
	out := make([]revisionOut, len(revs))
	for i, rev := range revs {
		ro := revisionOut{
			Version: rev.Version, Action: rev.Action, CreatedAt: rev.CreatedAt,
			Snapshot: rev.Snapshot, Changed: []string{},
		}
		if i+1 < len(revs) {
			ro.Changed = diffChangedFields(rev.Snapshot, revs[i+1].Snapshot)
		}
		out[i] = ro
	}
	writeJSON(w, http.StatusOK, out)
}

// pathVersion parses the {version} URL parameter, answering 400 when it
// is not a positive integer.
func pathVersion(w http.ResponseWriter, r *http.Request) (int, bool) {
	version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || version <= 0 {
		errBadRequest(w, "version must be a positive integer")
		return 0, false
	}
	return version, true
}

// revertTargetRevision answers POST /targets/{id}/revisions/{version}/revert:
// it loads the snapshot at that version, validates it exactly like a
// regular update, applies it to the live target and records a "revert"
// revision. It answers 404 if the target or the version does not exist.
func (d Deps) revertTargetRevision(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	version, ok := pathVersion(w, r)
	if !ok {
		return
	}
	rev, err := d.Store.GetTargetRevision(r.Context(), id, version)
	if storeError(w, d.Logger, "target revision", err) {
		return
	}
	var snap store.Target
	if err := json.Unmarshal(rev.Snapshot, &snap); err != nil {
		internalError(w, d.Logger, "unmarshal revision snapshot", err)
		return
	}
	// Older revisions (written before queues existed) carry only a "lane"
	// name in their snapshot, not "queue_id": resolve it the same way a
	// restore does, falling back to the default queue.
	if snap.QueueID == 0 {
		qid, err := d.Store.ResolveSnapshotQueueID(r.Context(), rev.Snapshot)
		if err != nil {
			internalError(w, d.Logger, "resolve revision queue", err)
			return
		}
		snap.QueueID = qid
	}

	b := targetBody{
		Name: snap.Name, Engine: snap.Engine, Enabled: snap.Enabled,
		QueueID: snap.QueueID, Options: snap.Options, Thresholds: snap.Thresholds,
	}
	if !d.validateTarget(r.Context(), w, &b) {
		return
	}

	t := &store.Target{ID: id, Name: b.Name, Engine: b.Engine, Enabled: b.Enabled,
		QueueID: b.QueueID, Options: b.Options, Thresholds: b.Thresholds}
	if err := d.Store.RevertTarget(r.Context(), t); err != nil {
		storeError(w, d.Logger, "target", err)
		return
	}
	updated, err := d.Store.GetTarget(r.Context(), id)
	if storeError(w, d.Logger, "target", err) {
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// listDeletedTargets answers GET /targets/deleted.
func (d Deps) listDeletedTargets(w http.ResponseWriter, r *http.Request) {
	deleted, err := d.Store.ListDeletedTargets(r.Context())
	if err != nil {
		internalError(w, d.Logger, "list deleted targets failed", err)
		return
	}
	writeJSON(w, http.StatusOK, deleted)
}

// restoreDeletedTarget answers POST /targets/deleted/{id}/restore: it
// re-inserts the target with its original id from the latest "delete"
// snapshot and records a "restore" revision. It answers 404 if id is not
// currently deleted, and 409 if a live row already occupies that id.
func (d Deps) restoreDeletedTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	snap, err := d.Store.LatestDeletedSnapshot(r.Context(), id)
	if storeError(w, d.Logger, "deleted target", err) {
		return
	}
	restored, err := d.Store.RestoreTarget(r.Context(), snap)
	if err != nil {
		if errors.Is(err, store.ErrIDConflict) {
			writeError(w, http.StatusConflict, "conflict", "a live target with that id already exists")
			return
		}
		internalError(w, d.Logger, "restore target failed", err)
		return
	}
	writeJSON(w, http.StatusOK, restored)
}
