// Package engine defines the speed-test engine abstraction: the Engine
// interface every engine implements, the Result it produces and the
// Progress events it streams while running.
package engine

import (
	"context"
	"encoding/json"
)

// Phase is the stage a running test is in. The values match the phase field
// of the SSE progress event.
type Phase string

// Test phases, in the order an engine normally reports them.
const (
	PhaseConnecting Phase = "connecting"
	PhasePing       Phase = "ping"
	PhaseDownload   Phase = "download"
	PhaseUpload     Phase = "upload"
	PhaseDone       Phase = "done"
	PhaseError      Phase = "error"
)

// Progress is one incremental update from a running test. It is the payload
// of the SSE progress event; the runner adds run/target identifiers.
type Progress struct {
	Phase      Phase   `json:"phase"`
	Progress   float64 `json:"progress"` // 0..1 within the phase
	Bps        float64 `json:"bps"`      // bits per second, instantaneous
	PingMs     float64 `json:"ping_ms"`
	JitterMs   float64 `json:"jitter_ms"`
	LossPct    float64 `json:"loss_pct"`
	ElapsedMs  int64   `json:"elapsed_ms"`
	ServerName string  `json:"server_name"`
}

// Result is the final outcome of a test, stored as one results row.
type Result struct {
	DownloadBps   float64 `json:"download_bps"`
	UploadBps     float64 `json:"upload_bps"`
	PingMs        float64 `json:"ping_ms"`
	JitterMs      float64 `json:"jitter_ms"`
	PacketLossPct float64 `json:"packet_loss_pct"`
	BytesDown     int64   `json:"bytes_down"`
	BytesUp       int64   `json:"bytes_up"`

	ServerID   string `json:"server_id"`
	ServerName string `json:"server_name"`
	ServerHost string `json:"server_host"`
	ISP        string `json:"isp"`
	ExternalIP string `json:"external_ip"`
	ResultURL  string `json:"result_url"`

	Raw json.RawMessage `json:"raw,omitempty"`
}

// ProgressFunc receives Progress events while a test runs. It may be nil.
type ProgressFunc func(Progress)

// Engine runs one kind of speed test.
type Engine interface {
	// Name is the stable identifier stored in targets.engine.
	Name() string
	// Validate reports whether opts is a usable option document.
	Validate(opts json.RawMessage) error
	// Run executes one test. prog may be nil.
	Run(ctx context.Context, opts json.RawMessage, prog func(Progress)) (*Result, error)
}

// Emit sends p to prog when prog is non-nil.
func Emit(prog func(Progress), p Progress) {
	if prog != nil {
		prog(p)
	}
}
