package api

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

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
func (d Deps) listTargetRevisions(ctx context.Context, in *idIn) (*jsonOut[[]revisionOut], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	revs, err := d.Store.ListTargetRevisions(ctx, in.ID)
	if err != nil {
		return nil, internalErr(d.Logger, "list target revisions failed", err)
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
	return &jsonOut[[]revisionOut]{out}, nil
}

type revertIn struct {
	ID      int64 `path:"id" minimum:"1" doc:"Resource ID."`
	Version int   `path:"version" minimum:"1" doc:"Revision version."`
}

// revertTargetRevision answers POST /targets/{id}/revisions/{version}/revert:
// it loads the snapshot at that version, validates it exactly like a
// regular update, applies it to the live target and records a "revert"
// revision. It answers 404 if the target or the version does not exist.
func (d Deps) revertTargetRevision(ctx context.Context, in *revertIn) (*jsonOut[*store.Target], error) {
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	if in.Version <= 0 {
		return nil, badRequest("version must be a positive integer")
	}
	id := in.ID
	rev, err := d.Store.GetTargetRevision(ctx, id, in.Version)
	if e := storeErr(d.Logger, "target revision", err); e != nil {
		return nil, e
	}
	var snap store.Target
	if err := json.Unmarshal(rev.Snapshot, &snap); err != nil {
		return nil, internalErr(d.Logger, "unmarshal revision snapshot", err)
	}
	// Always re-resolve the queue rather than trusting snap.QueueID as-is:
	// older revisions (written before queues existed) carry only a "lane"
	// name, and even a revision that does carry "queue_id" may name a
	// queue since deleted. ResolveSnapshotQueueID handles all three cases
	// (present/valid, present/stale, legacy lane), falling back to the
	// default queue.
	qid, err := d.Store.ResolveSnapshotQueueID(ctx, rev.Snapshot)
	if err != nil {
		return nil, internalErr(d.Logger, "resolve revision queue", err)
	}
	snap.QueueID = qid

	b := targetBody{
		Name: snap.Name, Engine: snap.Engine, Enabled: snap.Enabled,
		QueueID: snap.QueueID, Options: snap.Options, Thresholds: snap.Thresholds,
	}
	if err := d.validateTargetErr(ctx, &b); err != nil {
		return nil, err
	}

	t := &store.Target{ID: id, Name: b.Name, Engine: b.Engine, Enabled: b.Enabled,
		QueueID: b.QueueID, Options: b.Options, Thresholds: b.Thresholds}
	if err := d.Store.RevertTarget(ctx, t); err != nil {
		return nil, storeErr(d.Logger, "target", err)
	}
	updated, err := d.Store.GetTarget(ctx, id)
	if e := storeErr(d.Logger, "target", err); e != nil {
		return nil, e
	}
	return &jsonOut[*store.Target]{updated}, nil
}

// listDeletedTargets answers GET /targets/deleted.
func (d Deps) listDeletedTargets(ctx context.Context, _ *struct{}) (*jsonOut[[]store.DeletedTarget], error) {
	deleted, err := d.Store.ListDeletedTargets(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "list deleted targets failed", err)
	}
	return &jsonOut[[]store.DeletedTarget]{deleted}, nil
}

// restoreDeletedTarget answers POST /targets/deleted/{id}/restore: it
// re-inserts the target with its original id from the latest "delete"
// snapshot and records a "restore" revision. It answers 404 if id is not
// currently deleted, and 409 if a live row already occupies that id.
func (d Deps) restoreDeletedTarget(ctx context.Context, in *idIn) (*jsonOut[*store.Target], error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	snap, err := d.Store.LatestDeletedSnapshot(ctx, in.ID)
	if e := storeErr(d.Logger, "deleted target", err); e != nil {
		return nil, e
	}
	restored, err := d.Store.RestoreTarget(ctx, snap)
	if err != nil {
		if errors.Is(err, store.ErrIDConflict) {
			return nil, conflict("conflict", "a live target with that id already exists")
		}
		return nil, internalErr(d.Logger, "restore target failed", err)
	}
	return &jsonOut[*store.Target]{restored}, nil
}
