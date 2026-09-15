package runner

import "encoding/json"

// rotate rewrites the ordered host/server-id list an engine's options
// carry — ookla's "server_ids", iperf3's "hosts" — into the singular
// field the engine's Run expects ("server_id" / "host"), replacing it
// with the entry pick(n) selects out of the n-element list. The listing
// key itself is dropped from the result so options_snapshot records only
// the single host actually used.
//
// It leaves opts untouched (returning ok=false) when the engine has no
// such list (cloudflare and anything else), the list has at most one
// entry (nothing to rotate through), or opts don't parse as a JSON
// object.
func rotate(opts json.RawMessage, engineName string, pick func(n int) int) (json.RawMessage, bool) {
	switch engineName {
	case "ookla":
		return rotateField(opts, "server_ids", "server_id", pick)
	case "iperf3":
		return rotateField(opts, "hosts", "host", pick)
	default:
		return opts, false
	}
}

// rotateField implements rotate for one engine's (listKey, singleKey)
// pair.
func rotateField(opts json.RawMessage, listKey, singleKey string, pick func(n int) int) (json.RawMessage, bool) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(opts, &m); err != nil {
		return opts, false
	}
	raw, ok := m[listKey]
	if !ok {
		return opts, false
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil || len(list) <= 1 {
		return opts, false
	}
	idx := pick(len(list))
	if idx < 0 || idx >= len(list) {
		idx = 0
	}
	delete(m, listKey)
	m[singleKey] = list[idx]
	out, err := json.Marshal(m)
	if err != nil {
		return opts, false
	}
	return out, true
}
