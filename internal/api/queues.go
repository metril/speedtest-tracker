package api

import (
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

func (d Deps) listQueues(w http.ResponseWriter, r *http.Request) {
	queues, err := d.Store.ListQueues(r.Context())
	if err != nil {
		internalError(w, d.Logger, "list queues failed", err)
		return
	}
	writeJSON(w, http.StatusOK, queues)
}

func (d Deps) createQueue(w http.ResponseWriter, r *http.Request) {
	var b queueBody
	if !decodeJSON(w, r, &b) {
		return
	}
	name, ok := validateQueueName(w, b.Name)
	if !ok {
		return
	}
	q, err := d.Store.CreateQueue(r.Context(), name)
	if d.queueStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, q)
}

func (d Deps) updateQueue(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var b queueBody
	if !decodeJSON(w, r, &b) {
		return
	}
	name, ok := validateQueueName(w, b.Name)
	if !ok {
		return
	}
	q, err := d.Store.RenameQueue(r.Context(), id, name)
	if d.queueStoreError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (d Deps) deleteQueue(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := d.Store.DeleteQueue(r.Context(), id); d.queueStoreError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
