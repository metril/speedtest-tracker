package api

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/metril/speedtest-tracker/internal/store"
)

// csvHeader is the export's column order; keep it stable, people build
// spreadsheets on it.
var csvHeader = []string{
	"id", "started_at", "target_id", "target_name", "engine", "status", "error",
	"duration_ms", "download_bps", "upload_bps", "ping_ms", "jitter_ms",
	"packet_loss_pct", "bytes_down", "bytes_up", "server_id", "server_name",
	"isp", "external_ip", "result_url", "tags",
}

// csvSafe prefixes s with a single quote when it starts with a character
// spreadsheet software (Excel, Sheets, LibreOffice) treats as a formula
// trigger, guarding against CSV formula injection from user- or
// provider-supplied strings (target names, tags, ISP names, ...).
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

func csvRow(r *store.Result) []string {
	num := func(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
	targetID := ""
	if r.TargetID != nil {
		targetID = strconv.FormatInt(*r.TargetID, 10)
	}
	return []string{
		strconv.FormatInt(r.ID, 10), r.StartedAt, targetID, csvSafe(r.TargetName), r.Engine,
		r.Status, csvSafe(r.Error), strconv.FormatInt(r.DurationMs, 10),
		num(r.DownloadBps), num(r.UploadBps), num(r.PingMs), num(r.JitterMs),
		num(r.PacketLossPct), strconv.FormatInt(r.BytesDown, 10), strconv.FormatInt(r.BytesUp, 10),
		r.ServerID, csvSafe(r.ServerName), csvSafe(r.ISP), csvSafe(r.ExternalIP), csvSafe(r.ResultURL),
		csvSafe(strings.Join(r.Tags, " ")),
	}
}

// resultsCSV answers GET /results.csv with the same filters /results
// accepts. Rows are written and flushed as they are read, so memory stays
// flat and the download starts immediately. Because the status line is
// sent before the first row, a mid-stream failure can only be logged — it
// truncates the file rather than turning into a 500.
func (d Deps) resultsCSV(w http.ResponseWriter, r *http.Request) {
	f, ok := d.resultFilterFromQuery(w, r)
	if !ok {
		return
	}
	filename := "speedtest-results-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		d.Logger.Error("csv header", "error", err)
		return
	}
	rc := http.NewResponseController(w)
	n := 0
	err := d.Store.EachResult(r.Context(), f, func(res *store.Result) error {
		if err := cw.Write(csvRow(res)); err != nil {
			return err
		}
		n++
		if n%500 == 0 {
			cw.Flush()
			rc.Flush() // best-effort: an unwrapped writer just returns an error
		}
		return nil
	})
	cw.Flush()
	if err == nil {
		err = cw.Error()
	}
	if err != nil {
		d.Logger.Error("csv export truncated", "error", err, "rows", n)
	}
}
