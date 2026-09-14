package notify

import (
	"fmt"
	"strconv"
	"time"
)

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
