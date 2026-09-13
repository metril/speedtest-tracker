package config

import "testing"

func TestLoadDefaultsOutsideContainer(t *testing.T) {
	t.Setenv("ST_DB_PATH", "")
	t.Setenv("ST_LISTEN", "")
	os := stubOS{inContainer: false}
	got := load(os)
	if got.DBPath != "./data/speedtest.db" {
		t.Errorf("DBPath = %q, want ./data/speedtest.db", got.DBPath)
	}
	if got.Listen != ":8080" {
		t.Errorf("Listen = %q, want :8080", got.Listen)
	}
}

func TestLoadDefaultsInContainer(t *testing.T) {
	t.Setenv("ST_DB_PATH", "")
	got := load(stubOS{inContainer: true})
	if got.DBPath != "/data/speedtest.db" {
		t.Errorf("DBPath = %q, want /data/speedtest.db", got.DBPath)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("ST_DB_PATH", "/tmp/custom.db")
	t.Setenv("ST_LISTEN", "127.0.0.1:9999")
	got := load(stubOS{inContainer: true})
	if got.DBPath != "/tmp/custom.db" {
		t.Errorf("DBPath = %q", got.DBPath)
	}
	if got.Listen != "127.0.0.1:9999" {
		t.Errorf("Listen = %q", got.Listen)
	}
}

type stubOS struct{ inContainer bool }

func (s stubOS) InContainer() bool { return s.inContainer }
