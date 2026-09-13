// Package exectest builds tiny stub executables so engine tests can run
// without the real speedtest or iperf3 binaries installed.
package exectest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Build writes body into an executable /bin/sh script named name inside a
// temporary directory and returns its absolute path.
func Build(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", path, err)
	}
	return path
}

// ScriptEmitFile returns a script body that copies the file at the absolute
// path fixture to stdout and then exits with code.
func ScriptEmitFile(fixture string, code int) string {
	return fmt.Sprintf("cat %q\nexit %d", fixture, code)
}

// ScriptEmitFileOnFlag returns a script body that emits whenFlag's fixture
// when the flag appears in the arguments and otherwise emits elseFixture.
// Both paths must be absolute. The script always exits 0.
func ScriptEmitFileOnFlag(flag, whenFixture, elseFixture string) string {
	return fmt.Sprintf(`for a in "$@"; do
  if [ "$a" = %q ]; then cat %q; exit 0; fi
done
cat %q
exit 0`, flag, whenFixture, elseFixture)
}
