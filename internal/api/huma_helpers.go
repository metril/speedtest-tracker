package api

import (
	"log/slog"
	"net/url"
)

// jsonOut is the output of a handler that answers with a JSON body.
type jsonOut[T any] struct {
	Body T
}

// idIn is the input of a handler addressed by a numeric {id}.
type idIn struct {
	ID int64 `path:"id" minimum:"1" doc:"Resource ID."`
}

func (i idIn) check() error {
	if i.ID <= 0 {
		return badRequest("id must be a positive integer")
	}
	return nil
}

// Adapters from the legacy w-writing helpers to returned errors.

func internalErr(logger *slog.Logger, msg string, err error) error {
	c := &errCapture{}
	internalError(c, logger, msg, err)
	return c.err()
}

// storeErr maps a store error like storeError; nil in, nil out.
func storeErr(logger *slog.Logger, what string, err error) error {
	if err == nil {
		return nil
	}
	c := &errCapture{}
	storeError(c, logger, what, err)
	return c.err()
}

func enqueueErr(logger *slog.Logger, err error) error {
	c := &errCapture{}
	enqueueError(c, logger, err)
	return c.err()
}

// queryOf builds the url.Values the legacy query helpers read from
// name/value pairs, leaving empty values out.
func queryOf(kv ...string) url.Values {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			q.Set(kv[i], kv[i+1])
		}
	}
	return q
}
