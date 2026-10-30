package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/metril/speedtest-tracker/internal/store"
)

// maxQueueNameLen bounds a queue's name.
const maxQueueNameLen = 64

// queueBody is the request payload for queue create/rename.
type queueBody struct {
	Name string `json:"name"`
}

// validateQueueName trims and checks a queue name, answering 400 itself.
// It reports the normalised name and whether it is usable.
func validateQueueName(w http.ResponseWriter, raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" {
		errBadRequest(w, "name is required")
		return "", false
	}
	if len(name) > maxQueueNameLen {
		errBadRequest(w, "name must be at most 64 characters")
		return "", false
	}
	return name, true
}

// queueStoreError maps store errors for queues onto status codes. It
// reports whether it handled err.
func (d Deps) queueStoreError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrNameConflict):
		writeError(w, http.StatusConflict, "name_conflict", "a queue with that name already exists")
		return true
	case errors.Is(err, store.ErrQueueInUse):
		writeError(w, http.StatusConflict, "queue_in_use", "targets still reference this queue")
		return true
	case errors.Is(err, store.ErrLastQueue):
		writeError(w, http.StatusConflict, "last_queue", "cannot delete the last queue")
		return true
	}
	return storeError(w, d.Logger, "queue", err)
}

type queueIn struct {
	ID   int64 `path:"id" minimum:"1" doc:"Resource ID."`
	Body queueBody
}

type queueCreateIn struct {
	Body queueBody
}

func (d Deps) listQueues(ctx context.Context, _ *struct{}) (*jsonOut[[]store.Queue], error) {
	queues, err := d.Store.ListQueues(ctx)
	if err != nil {
		return nil, internalErr(d.Logger, "list queues failed", err)
	}
	return &jsonOut[[]store.Queue]{queues}, nil
}

func (d Deps) createQueue(ctx context.Context, in *queueCreateIn) (*jsonOut[store.Queue], error) {
	c := &errCapture{}
	name, ok := validateQueueName(c, in.Body.Name)
	if !ok {
		return nil, c.err()
	}
	q, err := d.Store.CreateQueue(ctx, name)
	if e := d.queueStoreErr(err); e != nil {
		return nil, e
	}
	return &jsonOut[store.Queue]{q}, nil
}

func (d Deps) updateQueue(ctx context.Context, in *queueIn) (*jsonOut[store.Queue], error) {
	if err := (idIn{in.ID}).check(); err != nil {
		return nil, err
	}
	c := &errCapture{}
	name, ok := validateQueueName(c, in.Body.Name)
	if !ok {
		return nil, c.err()
	}
	q, err := d.Store.RenameQueue(ctx, in.ID, name)
	if e := d.queueStoreErr(err); e != nil {
		return nil, e
	}
	return &jsonOut[store.Queue]{q}, nil
}

func (d Deps) deleteQueue(ctx context.Context, in *idIn) (*struct{}, error) {
	if err := in.check(); err != nil {
		return nil, err
	}
	if e := d.queueStoreErr(d.Store.DeleteQueue(ctx, in.ID)); e != nil {
		return nil, e
	}
	return nil, nil
}

// queueStoreErr is queueStoreError as a returned error (nil for nil).
func (d Deps) queueStoreErr(err error) error {
	if err == nil {
		return nil
	}
	c := &errCapture{}
	d.queueStoreError(c, err)
	return c.err()
}
