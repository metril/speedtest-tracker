// Package config parses the bootstrap environment. Only ST_DB_PATH and
// ST_LISTEN are env-only; every other knob lives in the settings table.
package config

import "os"

// Config holds the bootstrap configuration.
type Config struct {
	DBPath string
	Listen string
}

type environment interface {
	InContainer() bool
}

type realEnv struct{}

// InContainer reports whether the process looks like it runs inside a
// container. Docker creates /.dockerenv; Podman/Kubernetes expose
// /run/.containerenv.
func (realEnv) InContainer() bool {
	for _, p := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// Load reads the configuration from the process environment.
func Load() Config { return load(realEnv{}) }

func load(env environment) Config {
	dbDefault := "./data/speedtest.db"
	if env.InContainer() {
		dbDefault = "/data/speedtest.db"
	}
	return Config{
		DBPath: lookup("ST_DB_PATH", dbDefault),
		Listen: lookup("ST_LISTEN", ":8080"),
	}
}

func lookup(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
