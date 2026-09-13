package execx

import (
	"strconv"
	"sync"
	"testing"
)

func TestTailBufferCapsToLimit(t *testing.T) {
	b := NewTailBuffer(4)
	b.Write([]byte("abcdefgh"))
	if got := b.String(); got != "efgh" {
		t.Errorf("String() = %q, want %q", got, "efgh")
	}
}

func TestTailBufferConcurrentWrites(t *testing.T) {
	b := NewTailBuffer(64)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			b.Write([]byte(strconv.Itoa(i)))
			_ = b.String()
		}(i)
	}
	wg.Wait()
}
