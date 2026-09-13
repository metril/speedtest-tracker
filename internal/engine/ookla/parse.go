// Package ookla runs the Ookla speedtest CLI and parses its JSONL output.
package ookla

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/metril/speedtest-tracker/internal/engine"
)

// line is the union of every JSONL record the CLI emits. Fields absent for
// a given type stay zero.
type line struct {
	Type string `json:"type"`

	Level   string `json:"level"`
	Message string `json:"message"`

	Ping struct {
		Jitter   float64 `json:"jitter"`
		Latency  float64 `json:"latency"`
		Progress float64 `json:"progress"`
	} `json:"ping"`

	Download transfer `json:"download"`
	Upload   transfer `json:"upload"`

	// packetLoss may be null or absent; *float64 distinguishes that from 0.
	PacketLoss *float64 `json:"packetLoss"`

	ISP       string `json:"isp"`
	Interface struct {
		ExternalIP string `json:"externalIp"`
	} `json:"interface"`
	Server struct {
		ID       int64  `json:"id"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Name     string `json:"name"`
		Location string `json:"location"`
		Country  string `json:"country"`
	} `json:"server"`
	Result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"result"`
}

type transfer struct {
	Bandwidth float64 `json:"bandwidth"` // bytes per second
	Bytes     int64   `json:"bytes"`
	Elapsed   int64   `json:"elapsed"` // milliseconds
	Progress  float64 `json:"progress"`
}

// logLine is a remembered error-level log record, returned as the parse
// error when no result record follows it.
type logLine struct {
	Level   string
	Message string
}

// bufSize caps a JSONL line at 1 MiB; result lines are a few KiB at most.
const bufSize = 1 << 20

// errNoResult is returned when the stream ended without a result record and
// without a log-level error record either. It lets callers (e.g. ookla.Run)
// distinguish "the CLI told us why it failed" from "we have nothing to go
// on but the exit code and stderr".
var errNoResult = errors.New("speedtest: no result record in output")

// parseStream consumes the CLI's JSONL output, emitting a Progress event for
// every ping/download/upload record and returning the Result built from the
// final result record. An error-level log record is returned as the error if
// no result record follows it.
func parseStream(r io.Reader, prog func(engine.Progress)) (*engine.Result, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), bufSize)

	var (
		serverName string
		logErr     *logLine
	)

	for sc.Scan() {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var l line
		if err := json.Unmarshal(raw, &l); err != nil {
			continue // non-JSON noise on stdout
		}
		switch l.Type {
		case "testStart":
			serverName = l.Server.Name
			engine.Emit(prog, engine.Progress{Phase: engine.PhaseConnecting, ServerName: serverName})
		case "ping":
			engine.Emit(prog, engine.Progress{
				Phase: engine.PhasePing, Progress: l.Ping.Progress,
				PingMs: l.Ping.Latency, JitterMs: l.Ping.Jitter, ServerName: serverName,
			})
		case "download":
			engine.Emit(prog, transferProgress(engine.PhaseDownload, l.Download, serverName))
		case "upload":
			engine.Emit(prog, transferProgress(engine.PhaseUpload, l.Upload, serverName))
		case "log":
			if l.Level == "error" {
				logErr = &logLine{Level: l.Level, Message: l.Message}
				engine.Emit(prog, engine.Progress{Phase: engine.PhaseError, ServerName: serverName})
			}
		case "result":
			res := &engine.Result{
				DownloadBps: l.Download.Bandwidth * 8,
				UploadBps:   l.Upload.Bandwidth * 8,
				PingMs:      l.Ping.Latency,
				JitterMs:    l.Ping.Jitter,
				BytesDown:   l.Download.Bytes,
				BytesUp:     l.Upload.Bytes,
				ServerName:  l.Server.Name,
				ServerHost:  l.Server.Host,
				ISP:         l.ISP,
				ExternalIP:  l.Interface.ExternalIP,
				ResultURL:   l.Result.URL,
				Raw:         append(json.RawMessage(nil), raw...),
			}
			if l.PacketLoss != nil {
				res.PacketLossPct = *l.PacketLoss
			}
			if l.Server.ID != 0 {
				res.ServerID = fmt.Sprintf("%d", l.Server.ID)
			}
			if res.ServerName == "" {
				res.ServerName = serverName
			}
			engine.Emit(prog, engine.Progress{
				Phase: engine.PhaseDone, Progress: 1, Bps: res.DownloadBps,
				PingMs: res.PingMs, JitterMs: res.JitterMs, LossPct: res.PacketLossPct,
				ElapsedMs: l.Download.Elapsed + l.Upload.Elapsed, ServerName: res.ServerName,
			})
			return res, nil
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read speedtest output: %w", err)
	}
	if logErr != nil {
		return nil, fmt.Errorf("speedtest: %s", logErr.Message)
	}
	return nil, errNoResult
}

func transferProgress(phase engine.Phase, t transfer, serverName string) engine.Progress {
	return engine.Progress{
		Phase: phase, Progress: t.Progress, Bps: t.Bandwidth * 8,
		ElapsedMs: t.Elapsed, ServerName: serverName,
	}
}
