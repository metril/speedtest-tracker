package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// envSections are the known settings section prefixes, tried in order when
// mapping an ST_<SECTION>_<KEY> environment variable onto a settings key.
// Both the section and the key can contain underscores (e.g.
// ST_GENERAL_RETENTION_DAYS_RUNS), so the split cannot simply be on the
// first underscore; instead each candidate is validated against defaults,
// the single source of truth for what is settable.
var envSections = []string{"general", "engines", "auth", "integrations", "notifications"}

// EnvKeyToSetting maps an environment variable name of the form
// ST_<SECTION>_<KEY> onto its settings key (e.g. ST_AUTH_MODE ->
// auth.mode). It returns false for anything that is not the ST_<SECTION>_
// shape or does not name a known settings key — including ST_DB_PATH,
// ST_LISTEN and ST_LOCK_ENV, which configure the process but are not
// settings rows.
func EnvKeyToSetting(env string) (string, bool) {
	const prefix = "ST_"
	if !strings.HasPrefix(env, prefix) {
		return "", false
	}
	rest := strings.ToLower(strings.TrimPrefix(env, prefix))
	if rest == "" {
		return "", false
	}
	for _, section := range envSections {
		sectionPrefix := section + "_"
		if !strings.HasPrefix(rest, sectionPrefix) {
			continue
		}
		key := section + "." + strings.TrimPrefix(rest, sectionPrefix)
		if _, ok := defaults[key]; ok {
			return key, true
		}
	}
	return "", false
}

// SeedFromEnv applies ST_<SECTION>_<KEY> variables to the settings table.
// An env value fills a key the database has never seen, so a fresh
// container comes up configured; it does not fight a value the operator
// later changed in the UI. When ST_LOCK_ENV is true the env value is
// authoritative instead: it is written on every boot and the key is
// reported by LockedKeys so the API refuses to write it and the UI shows
// it as "set by environment".
//
// SeedFromEnv returns the sorted set of keys locked by this call (empty
// unless ST_LOCK_ENV is true).
func (s *Store) SeedFromEnv(ctx context.Context, environ []string) ([]string, error) {
	raw := map[string]string{}
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		raw[k] = v
	}

	lockEnv := false
	if v, ok := raw["ST_LOCK_ENV"]; ok {
		if b, err := strconv.ParseBool(v); err == nil {
			lockEnv = b
		}
	}

	type entry struct {
		env string
		key string
	}
	var entries []entry
	for env := range raw {
		if key, ok := EnvKeyToSetting(env); ok {
			entries = append(entries, entry{env, key})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })

	var locked []string
	for _, e := range entries {
		value := raw[e.env]

		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			quoted, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", e.env, err)
			}
			if err := json.Unmarshal(quoted, &decoded); err != nil {
				return nil, fmt.Errorf("%s: %w", e.env, err)
			}
		}

		encoded, err := json.Marshal(decoded)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.env, err)
		}
		target := reflect.New(reflect.TypeOf(defaults[e.key])).Interface()
		if err := json.Unmarshal(encoded, target); err != nil {
			return nil, fmt.Errorf("%s: %w", e.env, err)
		}

		if !lockEnv {
			// New() seeds every default key with ON CONFLICT DO NOTHING,
			// so a row always exists; "never seen" means the stored value
			// still equals the seeded default, i.e. no one has set it
			// since. A value that differs from the default was set
			// explicitly (by the operator or a previous env seed) and
			// wins over an unlocked env value.
			storedRaw, ok, err := s.Get(ctx, e.key)
			if err != nil {
				return nil, err
			}
			if ok {
				defaultRaw, err := json.Marshal(defaults[e.key])
				if err != nil {
					return nil, fmt.Errorf("%s: %w", e.env, err)
				}
				if string(storedRaw) != string(defaultRaw) {
					continue
				}
			}
		}

		if err := s.Set(ctx, e.key, decoded); err != nil {
			return nil, fmt.Errorf("%s: %w", e.env, err)
		}
		if lockEnv {
			locked = append(locked, e.key)
		}
	}

	if len(locked) > 0 {
		s.mu.Lock()
		if s.locked == nil {
			s.locked = map[string]bool{}
		}
		for _, key := range locked {
			s.locked[key] = true
		}
		s.mu.Unlock()
	}

	sort.Strings(locked)
	return locked, nil
}

// LockedKeys returns the sorted set of settings keys currently locked by
// ST_LOCK_ENV, i.e. authoritative from the environment.
func (s *Store) LockedKeys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.locked))
	for key := range s.locked {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// IsLocked reports whether key is currently locked by ST_LOCK_ENV.
func (s *Store) IsLocked(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.locked[key]
}
