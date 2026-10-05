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

// probeVersion runs `bin --version` and returns the parsed major and minor.
func probeVersion(ctx context.Context, bin string) (int, int, error) {
	out, err := execx.Command(ctx, bin, "--version").CombinedOutput()
	if err != nil && len(out) == 0 {
		return 0, 0, fmt.Errorf("%s --version: %w", bin, err)
	}
	return parseVersion(string(out))
}

// supportsJSONStream reports whether the binary at bin is iperf3 3.17 or
// newer, which is when --json-stream was introduced.
func supportsJSONStream(ctx context.Context, bin string) (bool, error) {
	major, minor, err := probeVersion(ctx, bin)
	if err != nil {
		return false, err
	}
	return versionAtLeast(major, minor, 3, 17), nil
}

// versionAtLeast reports whether major.minor >= wantMajor.wantMinor.
func versionAtLeast(major, minor, wantMajor, wantMinor int) bool {
	return major > wantMajor || (major == wantMajor && minor >= wantMinor)
}

// supportsConnectTimeout reports whether the client accepts
// --connect-timeout (iperf3 3.10 or newer).
func supportsConnectTimeout(major, minor int) bool { return versionAtLeast(major, minor, 3, 10) }
