package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/metril/speedtest-tracker/internal/engine/ookla"
)

// ServerLister supplies the Ookla server list (cached upstream).
type ServerLister interface {
	Servers(ctx context.Context) ([]ookla.Server, error)
}

// listOoklaServers answers GET /ookla/servers?q=, filtering name, location,
// country and host case-insensitively.
func (d Deps) listOoklaServers(w http.ResponseWriter, r *http.Request) {
	if d.ServerList == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "ookla server list not configured")
		return
	}
	servers, err := d.ServerList.Servers(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "server_list_failed", err.Error())
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	out := []ookla.Server{}
	for _, s := range servers {
		if q == "" || matchesServer(s, q) {
			out = append(out, s)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func matchesServer(s ookla.Server, q string) bool {
	for _, field := range []string{s.Name, s.Location, s.Country, s.Host} {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	return false
}
