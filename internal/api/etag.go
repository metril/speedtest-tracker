package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// etagBuffer captures a handler's response so its body can be hashed
// before anything reaches the client.
type etagBuffer struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (b *etagBuffer) Header() http.Header { return b.header }
func (b *etagBuffer) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}
func (b *etagBuffer) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.body.Write(p)
}

// etagJSON gives a JSON list handler a strong ETag over its body and
// answers 304 when the client already has that exact body. List responses
// are small and cheap to buffer; a 304 skips both the JSON transfer and
// the compression pass.
func etagJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := &etagBuffer{header: http.Header{}}
		next(buf, r)

		if buf.status != http.StatusOK {
			for k, v := range buf.header {
				w.Header()[k] = v
			}
			w.WriteHeader(buf.status)
			w.Write(buf.body.Bytes())
			return
		}
		sum := sha256.Sum256(buf.body.Bytes())
		tag := `"` + hex.EncodeToString(sum[:]) + `"`
		if matchesETag(r.Header.Get("If-None-Match"), tag) {
			// A 304 carries no body, so its Content-Type/Content-Length
			// would describe content that isn't being sent; RFC 7232
			// §4.1 only allows headers that would apply to a 200, and
			// those two specifically describe the (absent) body.
			for k, v := range buf.header {
				if k == "Content-Type" || k == "Content-Length" {
					continue
				}
				w.Header()[k] = v
			}
			w.Header().Set("ETag", tag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		for k, v := range buf.header {
			w.Header()[k] = v
		}
		w.Header().Set("ETag", tag)
		w.WriteHeader(http.StatusOK)
		w.Write(buf.body.Bytes())
	}
}

// matchesETag implements the If-None-Match comparison: a list of tags, or
// "*", with weak prefixes ignored.
func matchesETag(header, tag string) bool {
	if header == "" {
		return false
	}
	if strings.TrimSpace(header) == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == tag {
			return true
		}
	}
	return false
}
