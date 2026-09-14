package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/settings"
)

var validChannelTypes = map[string]bool{"webhook": true, "ntfy": true, "apprise": true}
var validPriorities = map[string]bool{"min": true, "low": true, "default": true, "high": true, "max": true}

// ValidateChannel checks a Channel for well-formedness before it is stored
// or used for delivery.
func ValidateChannel(ch settings.Channel) error {
	name := ch.ID
	if name == "" {
		name = "(new)"
	}
	if ch.ID == "" {
		return fmt.Errorf("channel %s: id must be set", name)
	}
	if !validChannelTypes[ch.Type] {
		return fmt.Errorf("channel %s: type must be one of webhook, ntfy, apprise", name)
	}
	u, err := url.Parse(ch.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("channel %s: url must be an absolute http(s) URL", name)
	}
	if ch.Priority != "" && !validPriorities[ch.Priority] {
		return fmt.Errorf("channel %s: priority must be one of min, low, default, high, max", name)
	}
	for k := range ch.Headers {
		if !isValidHTTPToken(k) {
			return fmt.Errorf("channel %s: header %q is not a valid HTTP header name", name, k)
		}
	}
	return nil
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

	case "ntfy":
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, strings.NewReader(m.Body))
		if err != nil {
			return err
		}
		req.Header.Set("Title", m.Title)
		if ch.Priority != "" {
			req.Header.Set("Priority", ch.Priority)
		}
		if len(ch.Tags) > 0 {
			req.Header.Set("Tags", strings.Join(ch.Tags, ","))
		}
		if ch.Token != "" {
			req.Header.Set("Authorization", "Bearer "+ch.Token)
		}

	case "apprise":
		payload := map[string]any{
			"title": m.Title,
			"body":  m.Body,
			"type":  appriseType(m.Kind),
		}
		if len(ch.Tags) > 0 {
			payload["tag"] = strings.Join(ch.Tags, ",")
		}
		if len(ch.URLs) > 0 {
			payload["urls"] = ch.URLs
		}
		body, encErr := json.Marshal(payload)
		if encErr != nil {
			return fmt.Errorf("encode apprise message: %w", encErr)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if ch.Token != "" {
			req.Header.Set("Authorization", "Bearer "+ch.Token)
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

func appriseType(kind string) string {
	if kind == "recovery" {
		return "success"
	}
	return "warning"
}
