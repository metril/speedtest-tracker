package notify

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// KindResult marks a per-result summary message, delivered for every
// completed result when a target's (or the global default) NotifyAlways
// threshold is on — unlike "alert"/"recovery", it is not tied to any
// threshold breach, cooldown or firing state.
const KindResult = "result"

// Message is one rendered notification, shared by every channel type. It
// is also the exact JSON body a webhook channel receives, so its tags are
// part of the public contract documented in the README.
type Message struct {
	Kind     string    `json:"kind"` // "alert" or "recovery"
	Title    string    `json:"title"`
	Body     string    `json:"body"`
	Target   string    `json:"target"`
	TargetID int64     `json:"target_id"`
	Metric   Metric    `json:"metric"`
	Value    float64   `json:"value"`
	Limit    float64   `json:"limit"`
	Unit     string    `json:"unit"`
	ResultID int64     `json:"result_id"`
	At       time.Time `json:"at"`
}

// minStyle metrics breach when the value falls below the limit; the rest
// (max-style) breach when the value rises above the limit.
func isMinStyle(m Metric) bool {
	return m == MetricDownload || m == MetricUpload
}

func fmtNum(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// BuildMessage renders an Eval into a Message for the given target and
// result. errText is the engine's error string, used only for a
// MetricFailure alert.
func BuildMessage(kind string, target string, targetID, resultID int64, e Eval, at time.Time, errText string) Message {
	m := Message{
		Kind: kind, Target: target, TargetID: targetID,
		Metric: e.Metric, Value: e.Value, Limit: e.Limit, Unit: e.Unit,
		ResultID: resultID, At: at,
	}

	if e.Metric == MetricFailure {
		if kind == "recovery" {
			m.Title = fmt.Sprintf("%s: %s recovered", target, e.Metric)
			m.Body = "the test completed successfully"
			return m
		}
		m.Title = fmt.Sprintf("%s: test failed", target)
		if errText == "" {
			errText = "the test did not complete"
		}
		m.Body = errText
		return m
	}

	if kind == "recovery" {
		m.Title = fmt.Sprintf("%s: %s recovered", target, e.Metric)
		m.Body = fmt.Sprintf("%s %s", fmtNum(e.Value), e.Unit)
		return m
	}

	dir := "above"
	if isMinStyle(e.Metric) {
		dir = "below"
	}
	m.Title = fmt.Sprintf("%s: %s %s %s %s", target, e.Metric, dir, fmtNum(e.Limit), e.Unit)
	m.Body = fmt.Sprintf("%s %s (limit %s %s)", fmtNum(e.Value), e.Unit, fmtNum(e.Limit), e.Unit)
	return m
}

// BuildResultMessage renders a per-result summary for res, delivered
// regardless of thresholds when NotifyAlways is on. A failed result (Status
// != "ok") carries no usable metrics, so its body is just the engine's
// error text, mirroring BuildMessage's MetricFailure case; m.Metric is set
// to MetricFailure so mapNotifyType can pick NotifyFailure for it. An "ok"
// result's body lists download/upload/loss always and ping/jitter only when
// non-zero, since some engines (e.g. iperf3 TCP) never measure them.
func BuildResultMessage(target string, targetID, resultID int64, res *store.Result, at time.Time) Message {
	m := Message{Kind: KindResult, Target: target, TargetID: targetID, ResultID: resultID, At: at}

	if res.Status != "ok" {
		m.Metric = MetricFailure
		m.Title = fmt.Sprintf("%s: test failed", target)
		errText := res.Error
		if errText == "" {
			errText = "the test did not complete"
		}
		m.Body = errText
		return m
	}

	m.Title = fmt.Sprintf("%s: test complete", target)
	parts := []string{
		fmt.Sprintf("↓ %s Mbps", fmtNum(res.DownloadBps/1e6)),
		fmt.Sprintf("↑ %s Mbps", fmtNum(res.UploadBps/1e6)),
	}
	if res.PingMs != 0 {
		parts = append(parts, fmt.Sprintf("ping %s ms", fmtNum(res.PingMs)))
	}
	if res.JitterMs != 0 {
		parts = append(parts, fmt.Sprintf("jitter %s ms", fmtNum(res.JitterMs)))
	}
	parts = append(parts, fmt.Sprintf("loss %s%%", fmtNum(res.PacketLossPct)))
	m.Body = strings.Join(parts, " · ")
	return m
}
