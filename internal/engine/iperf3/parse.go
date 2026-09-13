package iperf3

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/metril/speedtest-tracker/internal/engine"
)

type sum struct {
	Start         float64 `json:"start"`
	End           float64 `json:"end"`
	Seconds       float64 `json:"seconds"`
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	JitterMs      float64 `json:"jitter_ms"`
	LostPercent   float64 `json:"lost_percent"`
}

type summary struct {
	Error string `json:"error"`
	Start struct {
		ConnectingTo struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		} `json:"connecting_to"`
		Version string `json:"version"`
	} `json:"start"`
	End struct {
		Sum             sum `json:"sum"`
		SumSent         sum `json:"sum_sent"`
		SumReceived     sum `json:"sum_received"`
		SumBidirForward sum `json:"sum_bidir_forward"`
		SumBidirReverse sum `json:"sum_bidir_reverse"`
	} `json:"end"`
}

// parseSummary converts one `iperf3 -J` (or --json-stream "end" event) document
// into a Result. Direction is decided by the options that produced the run,
// never by field names.
func parseSummary(data []byte, o Options) (*engine.Result, error) {
	var s summary
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("iperf3: parse json: %w", err)
	}
	if s.Error != "" {
		return nil, fmt.Errorf("iperf3: %s", s.Error)
	}
	res := &engine.Result{
		ServerHost: fmt.Sprintf("%s:%d", o.Host, o.Port),
		ServerName: o.Host,
		ServerID:   o.Host,
		Raw:        append(json.RawMessage(nil), data...),
	}
	switch {
	case o.Bidir:
		res.UploadBps = s.End.SumBidirForward.BitsPerSecond
		res.BytesUp = s.End.SumBidirForward.Bytes
		res.DownloadBps = s.End.SumBidirReverse.BitsPerSecond
		res.BytesDown = s.End.SumBidirReverse.Bytes
	case o.Protocol == "udp":
		// UDP reports one sum with jitter and loss.
		if o.Reverse {
			res.DownloadBps = s.End.Sum.BitsPerSecond
			res.BytesDown = s.End.Sum.Bytes
		} else {
			res.UploadBps = s.End.Sum.BitsPerSecond
			res.BytesUp = s.End.Sum.Bytes
		}
		res.JitterMs = s.End.Sum.JitterMs
		res.PacketLossPct = s.End.Sum.LostPercent
	case o.Reverse:
		// -R: the server sends, so what arrived here is the download.
		res.DownloadBps = s.End.SumReceived.BitsPerSecond
		res.BytesDown = s.End.SumReceived.Bytes
	default:
		// Forward TCP: sum_received is what the server actually got.
		res.UploadBps = s.End.SumReceived.BitsPerSecond
		res.BytesUp = s.End.SumReceived.Bytes
	}
	if res.DownloadBps == 0 && res.UploadBps == 0 {
		return nil, errors.New("iperf3: no throughput in summary")
	}
	return res, nil
}

// streamLine is one --json-stream record.
type streamLine struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// parseStreamJSONL consumes --json-stream output, emitting Progress per
// interval and returning the Result built from the final end event.
func parseStreamJSONL(r io.Reader, o Options, prog func(engine.Progress)) (*engine.Result, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	phase := engine.PhaseUpload
	if o.Reverse {
		phase = engine.PhaseDownload
	}

	var (
		res    *engine.Result
		errMsg string
	)
	for sc.Scan() {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		var l streamLine
		if err := json.Unmarshal(raw, &l); err != nil {
			continue
		}
		switch l.Event {
		case "start":
			engine.Emit(prog, engine.Progress{Phase: engine.PhaseConnecting, ServerName: o.Host})
		case "interval":
			var d struct {
				Sum sum `json:"sum"`
			}
			if err := json.Unmarshal(l.Data, &d); err != nil {
				continue
			}
			secs := d.Sum.End
			if secs == 0 {
				secs = d.Sum.Seconds
			}
			elapsed := int64(secs * 1000)
			bps := d.Sum.BitsPerSecond
			engine.Emit(prog, engine.Progress{
				Phase:      phase,
				Progress:   progressFraction(elapsed, o.DurationS),
				Bps:        bps,
				JitterMs:   d.Sum.JitterMs,
				LossPct:    d.Sum.LostPercent,
				ElapsedMs:  elapsed,
				ServerName: o.Host,
			})
		case "end":
			// data holds the same shape as the -J document.
			r, err := parseSummary(l.Data, o)
			if err != nil {
				return nil, err
			}
			res = r
		case "error":
			var d struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(l.Data, &d); err == nil && d.Error != "" {
				errMsg = d.Error
			}
			engine.Emit(prog, engine.Progress{Phase: engine.PhaseError, ServerName: o.Host})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("iperf3: read stream: %w", err)
	}
	if errMsg != "" {
		return nil, fmt.Errorf("iperf3: %s", errMsg)
	}
	if res == nil {
		return nil, errors.New("iperf3: no end event in stream")
	}
	engine.Emit(prog, engine.Progress{
		Phase: engine.PhaseDone, Progress: 1,
		Bps: maxBps(res), JitterMs: res.JitterMs, LossPct: res.PacketLossPct,
		ElapsedMs: int64(o.DurationS) * 1000, ServerName: o.Host,
	})
	return res, nil
}

func progressFraction(elapsedMs int64, durationS int) float64 {
	if durationS <= 0 {
		return 0
	}
	f := float64(elapsedMs) / float64(durationS*1000)
	if f > 1 {
		f = 1
	}
	return f
}

func maxBps(r *engine.Result) float64 {
	if r.DownloadBps > r.UploadBps {
		return r.DownloadBps
	}
	return r.UploadBps
}
