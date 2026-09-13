package exectest

import (
	"os/exec"
	"strings"
	"testing"
)

func TestBuildRunsScript(t *testing.T) {
	bin := Build(t, "stub", "printf 'hello\\n'")
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run stub: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("out = %q", out)
	}
}
