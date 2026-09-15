package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	apprise "github.com/unraid/apprise-go"

	"github.com/metril/speedtest-tracker/internal/settings"
)

var validChannelTypes = map[string]bool{"webhook": true, "apprise": true}

// ValidateChannel checks a Channel for well-formedness before it is stored
// or used for delivery. apprise channels are validated by URLs (each must
// parse as a supported apprise-go target URL); the other types validate
// URL as an absolute http(s) endpoint.
func ValidateChannel(ch settings.Channel) error {
	name := ch.ID
	if name == "" {
		name = "(new)"
	}
	if ch.ID == "" {
		return fmt.Errorf("channel %s: id must be set", name)
	}
	if !validChannelTypes[ch.Type] {
		return fmt.Errorf("channel %s: type must be one of webhook, apprise", name)
	}
	if ch.Type == "apprise" {
		if len(ch.URLs) == 0 {
			return fmt.Errorf("channel %s: apprise urls must be set", name)
		}
		client := apprise.New()
		for i, u := range ch.URLs {
			if err := client.Add(u); err != nil {
				return fmt.Errorf("channel %s: apprise url #%d: %w", name, i+1, redactAppriseErr(err, ch.URLs))
			}
		}
	} else {
		u, err := url.Parse(ch.URL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("channel %s: url must be an absolute http(s) URL", name)
		}
	}
	for k := range ch.Headers {
		if !isValidHTTPToken(k) {
			return fmt.Errorf("channel %s: header %q is not a valid HTTP header name", name, k)
		}
	}
	return nil
}

// RedactURL returns u with userinfo stripped and every query parameter
// value replaced with "***", keeping scheme, host and path visible. It is
// the safe form of an apprise URL for errors, logs and the settings API:
// apprise targets embed credentials in userinfo, query params, or
// sometimes the path/host itself (e.g. discord://id/token), so this is a
// simplest-safe-rule redaction, not a guarantee every secret is stripped.
// An unparseable u redacts to "***" wholesale.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "***"
	}
	u.User = nil
	if q := u.Query(); len(q) > 0 {
		for k := range q {
			q[k] = []string{"***"}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// redactAppriseErr returns err with every URL in urls substituted by its
// RedactURL form, so a library error that echoes a failing target back
// verbatim (apprise-go's errors do) never leaks the credential it embeds.
func redactAppriseErr(err error, urls []string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, u := range urls {
		if u == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, u, RedactURL(u))
	}
	return errors.New(msg)
}

func isValidHTTPToken(s string) bool {
	if s == "" {
		return false
	}
	return textproto.TrimString(s) == s && http.CanonicalHeaderKey(s) != "" && !strings.ContainsAny(s, " \t\r\n:")
}

// Deliver sends m to ch over HTTP. client nil means a client with a 10s
// timeout. Any non-2xx response is returned as an error carrying the
// status code and up to 256 bytes of the response body.
func Deliver(ctx context.Context, client *http.Client, ch settings.Channel, m Message) error {
	if ch.Type == "apprise" {
		return deliverApprise(ctx, ch, m)
	}

	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	var req *http.Request
	var err error

	switch ch.Type {
	case "webhook":
		body, encErr := json.Marshal(m)
		if encErr != nil {
			return fmt.Errorf("encode webhook message: %w", encErr)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range ch.Headers {
			if strings.EqualFold(k, "Content-Type") {
				continue
			}
			req.Header.Set(k, v)
		}

	default:
		return fmt.Errorf("unknown channel type %q", ch.Type)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver to %s: %w", ch.Type, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited := io.LimitReader(resp.Body, 256)
		respBody, _ := io.ReadAll(limited)
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("deliver to %s: status %d: %s", ch.Type, resp.StatusCode, bytes.TrimSpace(respBody))
	}

	io.Copy(io.Discard, resp.Body)
	return nil
}

// deliverApprise sends m through the embedded apprise-go library to every
// URL configured on ch. The library has no context-aware Send, so it runs
// on its own goroutine and the call honours ctx's deadline/cancellation
// independently; a timeout here leaves the goroutine to finish on its own
// (the library owns its own HTTP timeouts internally). The returned error
// is the library's own — it already names the failing target URL — with
// every configured URL redacted via RedactURL so no credential leaks
// through it.
func deliverApprise(ctx context.Context, ch settings.Channel, m Message) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- apprise.Send(ch.URLs, m.Body, apprise.WithTitle(m.Title), apprise.WithNotifyType(mapNotifyType(m)))
	}()
	select {
	case err := <-errCh:
		return redactAppriseErr(err, ch.URLs)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// mapNotifyType maps a rendered Message onto apprise-go's semantic
// notification types: a recovery is a success, a test-failure alert is a
// failure, and every other alert is a warning.
func mapNotifyType(m Message) apprise.NotifyType {
	switch {
	case m.Kind == "recovery":
		return apprise.NotifySuccess
	case m.Metric == MetricFailure:
		return apprise.NotifyFailure
	default:
		return apprise.NotifyWarning
	}
}
