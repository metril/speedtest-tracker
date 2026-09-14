package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Result is one completed test, the row the whole product is about.
type Result struct {
	ID              int64           `json:"id"`
	RunID           *int64          `json:"run_id"`
	TargetID        *int64          `json:"target_id"`
	TargetName      string          `json:"target_name"`
	Engine          string          `json:"engine"`
	OptionsSnapshot json.RawMessage `json:"options_snapshot"`
	Status          string          `json:"status"`
	Error           string          `json:"error,omitempty"`
	StartedAt       string          `json:"started_at"`
	DurationMs      int64           `json:"duration_ms"`
	DownloadBps     float64         `json:"download_bps"`
	UploadBps       float64         `json:"upload_bps"`
	PingMs          float64         `json:"ping_ms"`
	JitterMs        float64         `json:"jitter_ms"`
	PacketLossPct   float64         `json:"packet_loss_pct"`
	BytesDown       int64           `json:"bytes_down"`
	BytesUp         int64           `json:"bytes_up"`
	ServerID        string          `json:"server_id"`
	ServerName      string          `json:"server_name"`
	ServerHost      string          `json:"server_host"`
	ISP             string          `json:"isp"`
	ExternalIP      string          `json:"external_ip"`
	ResultURL       string          `json:"result_url"`
	Raw             json.RawMessage `json:"raw,omitempty"`
	Tags            []string        `json:"tags"`
}

const resultColumns = `id,run_id,target_id,target_name,engine,options_snapshot,status,error,
	started_at,duration_ms,download_bps,upload_bps,ping_ms,jitter_ms,packet_loss_pct,
	bytes_down,bytes_up,server_id,server_name,server_host,isp,external_ip,result_url,raw`

func scanResult(sc interface{ Scan(...any) error }) (*Result, error) {
	var (
		r                                  Result
		snapshot                           string
		errMsg                             sql.NullString
		raw                                sql.NullString
		dur                                sql.NullInt64
		down, up, ping, jitter, loss       sql.NullFloat64
		bdown, bup                         sql.NullInt64
		sid, sname, shost, isp, extIP, url sql.NullString
	)
	if err := sc.Scan(&r.ID, &r.RunID, &r.TargetID, &r.TargetName, &r.Engine, &snapshot,
		&r.Status, &errMsg, &r.StartedAt, &dur, &down, &up, &ping, &jitter, &loss,
		&bdown, &bup, &sid, &sname, &shost, &isp, &extIP, &url, &raw); err != nil {
		return nil, err
	}
	r.OptionsSnapshot = json.RawMessage(snapshot)
	r.Error = errMsg.String
	r.DurationMs = dur.Int64
	r.DownloadBps, r.UploadBps = down.Float64, up.Float64
	r.PingMs, r.JitterMs, r.PacketLossPct = ping.Float64, jitter.Float64, loss.Float64
	r.BytesDown, r.BytesUp = bdown.Int64, bup.Int64
	r.ServerID, r.ServerName, r.ServerHost = sid.String, sname.String, shost.String
	r.ISP, r.ExternalIP, r.ResultURL = isp.String, extIP.String, url.String
	if raw.Valid && raw.String != "" {
		r.Raw = json.RawMessage(raw.String)
	}
	r.Tags = []string{}
	return &r, nil
}

// nullString stores "" as NULL so empty text columns stay empty.
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// InsertResult writes one result row inside a single write transaction and
// returns its id.
func (s *Store) InsertResult(ctx context.Context, r *Result) (int64, error) {
	tx, err := s.Write.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin result tx: %w", err)
	}
	defer tx.Rollback()

	var rawVal any
	if len(r.Raw) > 0 {
		rawVal = string(r.Raw)
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO results(run_id,target_id,target_name,engine,options_snapshot,status,error,
			started_at,duration_ms,download_bps,upload_bps,ping_ms,jitter_ms,packet_loss_pct,
			bytes_down,bytes_up,server_id,server_name,server_host,isp,external_ip,result_url,raw)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.RunID, r.TargetID, r.TargetName, r.Engine, rawOrEmpty(r.OptionsSnapshot), r.Status,
		nullString(r.Error), r.StartedAt, r.DurationMs, r.DownloadBps, r.UploadBps, r.PingMs,
		r.JitterMs, r.PacketLossPct, r.BytesDown, r.BytesUp, nullString(r.ServerID),
		nullString(r.ServerName), nullString(r.ServerHost), nullString(r.ISP),
		nullString(r.ExternalIP), nullString(r.ResultURL), rawVal)
	if err != nil {
		return 0, fmt.Errorf("insert result: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit result: %w", err)
	}
	r.ID = id
	return id, nil
}

// ResultFilter narrows a results listing. Cursor is the id returned by the
// previous page; From/To bound started_at inclusively.
type ResultFilter struct {
	TargetID *int64
	Engine   string
	Status   string
	From     string
	To       string
	Tag      string
	Limit    int
	Cursor   int64
}

// ListResults returns up to Limit results newest-first plus the cursor for
// the next page (0 when exhausted). Keyset pagination on id; never OFFSET.
func (s *Store) ListResults(ctx context.Context, f ResultFilter) ([]Result, int64, error) {
	limit := clampLimit(f.Limit)
	var where []string
	var args []any
	if f.TargetID != nil {
		where = append(where, `target_id=?`)
		args = append(args, *f.TargetID)
	}
	if f.Engine != "" {
		where = append(where, `engine=?`)
		args = append(args, f.Engine)
	}
	if f.Status != "" {
		where = append(where, `status=?`)
		args = append(args, f.Status)
	}
	if f.From != "" {
		where = append(where, `started_at>=?`)
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, `started_at<=?`)
		args = append(args, f.To)
	}
	if tag := strings.ToLower(strings.TrimSpace(f.Tag)); tag != "" {
		where = append(where, `EXISTS (SELECT 1 FROM result_tags rt
			JOIN tags t ON t.id = rt.tag_id
			WHERE rt.result_id = results.id AND t.name = ?)`)
		args = append(args, tag)
	}
	if f.Cursor > 0 {
		where = append(where, `id<?`)
		args = append(args, f.Cursor)
	}
	q := `SELECT ` + resultColumns + ` FROM results`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.Read.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list results: %w", err)
	}
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		r, err := scanResult(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan result: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var next int64
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].ID
	}
	if err := s.attachTags(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, next, nil
}

// GetResult returns one result, or ErrNotFound.
func (s *Store) GetResult(ctx context.Context, id int64) (*Result, error) {
	row := s.Read.QueryRowContext(ctx, `SELECT `+resultColumns+` FROM results WHERE id=?`, id)
	r, err := scanResult(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get result %d: %w", id, err)
	}
	one := []Result{*r}
	if err := s.attachTags(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

// DeleteResult removes one result (result_tags cascade), or ErrNotFound.
func (s *Store) DeleteResult(ctx context.Context, id int64) error {
	res, err := s.Write.ExecContext(ctx, `DELETE FROM results WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete result %d: %w", id, err)
	}
	return requireAffected(res)
}

// LatestResultForTarget returns the newest result for a target, or
// ErrNotFound when the target has never run.
func (s *Store) LatestResultForTarget(ctx context.Context, targetID int64) (*Result, error) {
	row := s.Read.QueryRowContext(ctx,
		`SELECT `+resultColumns+` FROM results WHERE target_id=? ORDER BY started_at DESC, id DESC LIMIT 1`,
		targetID)
	r, err := scanResult(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("latest result for target %d: %w", targetID, err)
	}
	one := []Result{*r}
	if err := s.attachTags(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

// LatestResults returns the newest result per target.
func (s *Store) LatestResults(ctx context.Context) ([]Result, error) {
	rows, err := s.Read.QueryContext(ctx, `
		SELECT `+resultColumns+` FROM results
		WHERE id IN (
			SELECT id FROM results r2
			WHERE r2.target_id IS NOT NULL
			  AND r2.id = (SELECT id FROM results r3
			               WHERE r3.target_id = r2.target_id
			               ORDER BY r3.started_at DESC, r3.id DESC LIMIT 1)
		)
		ORDER BY target_name, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("latest results: %w", err)
	}
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		r, err := scanResult(rows)
		if err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachTags(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}
