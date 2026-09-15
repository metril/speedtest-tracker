package notify

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	apprise "github.com/unraid/apprise-go"

	"github.com/metril/speedtest-tracker/internal/settings"
)

// MigrateNtfyChannels rewrites every legacy "ntfy" channel into an
// "apprise" channel that points apprise-go's own ntfy:// target at the
// same server, so the dedicated ntfy delivery path — and the external
// Apprise API server ntfy channels never actually needed — can be
// retired. It is pure and safe to call on every startup: channels that
// are already not type "ntfy" pass through unchanged and changed is
// false when nothing needed rewriting. Callers persist the result.
//
// A channel whose stored URL cannot be turned into a valid apprise-go
// ntfy:// target (bad URL, no host, or a URL apprise-go itself rejects)
// is disabled rather than dropped or left as an invalid ntfy channel, and
// logged via logger (nil is fine — the check is skipped).
func MigrateNtfyChannels(channels []settings.Channel, logger *slog.Logger) ([]settings.Channel, bool) {
	changed := false
	out := make([]settings.Channel, len(channels))
	for i, ch := range channels {
		if ch.Type != "ntfy" {
			out[i] = ch
			continue
		}
		changed = true
		out[i] = migrateNtfyChannel(ch, logger)
	}
	return out, changed
}

func migrateNtfyChannel(ch settings.Channel, logger *slog.Logger) settings.Channel {
	next := ch
	next.Type = "apprise"
	next.URL = ""
	next.Token = ""
	next.Priority = ""

	built, err := buildNtfyURL(ch)
	if err == nil {
		err = apprise.New().Add(built)
	}
	if err != nil {
		next.Enabled = false
		next.URLs = nil
		if logger != nil {
			logger.Warn("notify: migrate ntfy channel to apprise: unparseable url, disabling channel",
				"channel", ch.ID, "url", ch.URL, "err", err)
		}
		return next
	}
	next.URLs = []string{built}
	return next
}

// buildNtfyURL turns a legacy ntfy channel's full topic URL (e.g.
// "https://ntfy.sh/mytopic" or "https://ntfy.example.com/mytopic") into
// the apprise-go equivalent: "ntfy://[token@]host/topic?priority=&tags=",
// carrying over ch.Token, ch.Priority and ch.Tags. ntfys:// is used when
// the original URL was https, ntfy:// (plain HTTP) when it was http.
func buildNtfyURL(ch settings.Channel) (string, error) {
	u, err := url.Parse(strings.TrimSpace(ch.URL))
	if err != nil {
		return "", fmt.Errorf("parse ntfy url: %w", err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("ntfy url %q has no host", ch.URL)
	}

	scheme := "ntfy"
	if u.Scheme == "https" {
		scheme = "ntfys"
	}

	topic := strings.Trim(u.Path, "/")
	built := url.URL{Scheme: scheme, Host: u.Host, Path: "/" + topic}
	if ch.Token != "" {
		built.User = url.User(ch.Token)
	}

	q := url.Values{}
	if ch.Priority != "" {
		q.Set("priority", ch.Priority)
	}
	if len(ch.Tags) > 0 {
		q.Set("tags", strings.Join(ch.Tags, ","))
	}
	built.RawQuery = q.Encode()

	return built.String(), nil
}
