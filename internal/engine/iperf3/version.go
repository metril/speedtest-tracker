package iperf3

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/metril/speedtest-tracker/internal/engine/execx"
)

var versionRe = regexp.MustCompile(`iperf\s+(\d+)\.(\d+)`)

// parseVersion extracts the major and minor version from `iperf3 --version`.
func parseVersion(s string) (int, int, error) {
	m := versionRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, fmt.Errorf("iperf3: cannot parse version from %q", s)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return major, minor, nil
}

// supportsJSONStream reports whether the binary at bin is iperf3 3.17 or
// newer, which is when --json-stream was introduced.
func supportsJSONStream(ctx context.Context, bin string) (bool, error) {
	out, err := execx.Command(ctx, bin, "--version").CombinedOutput()
	if err != nil && len(out) == 0 {
		return false, fmt.Errorf("%s --version: %w", bin, err)
	}
	major, minor, err := parseVersion(string(out))
	if err != nil {
		return false, err
	}
	return major > 3 || (major == 3 && minor >= 17), nil
}
