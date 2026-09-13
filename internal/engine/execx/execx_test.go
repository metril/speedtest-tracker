//go:build unix

package execx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/metril/speedtest-tracker/internal/engine/exectest"
)

// TestCommandCancelKillsProcessGroup verifies that cancelling the context
// kills not just the direct child but its whole process group, including a
// grandchild the child spawned and left running in the background.
func TestCommandCancelKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := fmt.Sprintf(`sleep 30 &
echo $! > %q
wait`, pidFile)
	bin := exectest.Build(t, "spawner", script)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := Command(ctx, bin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	var pid int
	deadline := time.Now().Add(2 * time.Second)
	for {
		if b, err := os.ReadFile(pidFile); err == nil {
			if p, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && p > 0 {
				pid = p
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("grandchild pid file never appeared")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	_ = cmd.Wait()

	deadline = time.Now().Add(3 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err != nil {
			return // ESRCH: grandchild is gone
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild pid %d still alive %v after cancel", pid, 3*time.Second)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
