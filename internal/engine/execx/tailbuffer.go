package execx

import (
	"strings"
	"sync"
)

// TailBuffer is an io.Writer that keeps only the last max bytes written to
// it. Its Write and String are both mutex-guarded so String can be called
// safely while exec's own stderr copier goroutine is still writing to it
// during the WaitDelay grace period after Cancel.
type TailBuffer struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

// NewTailBuffer returns a TailBuffer that retains at most max bytes.
func NewTailBuffer(max int) *TailBuffer {
	return &TailBuffer{limit: max}
}

// Write implements io.Writer.
func (b *TailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.limit {
		b.buf = b.buf[len(b.buf)-b.limit:]
	}
	return len(p), nil
}

// String returns the retained tail, trimmed of surrounding whitespace.
func (b *TailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.buf))
}
