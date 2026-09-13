package ookla

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// Server is one entry of the Ookla server list.
type Server struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
	Country  string `json:"country"`
	Host     string `json:"host"`
}

// ServerList fetches and caches `speedtest -L -f json`.
type ServerList struct {
	bin string
	ttl time.Duration

	mu        sync.Mutex
	cache     []Server
	fetchedAt time.Time
	now       func() time.Time
}

// NewServerList returns a cache over the server list of the binary at bin.
func NewServerList(bin string, ttl time.Duration) *ServerList {
	return &ServerList{bin: bin, ttl: ttl, now: time.Now}
}

// SetNow overrides the clock; used by tests.
func (l *ServerList) SetNow(f func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = f
}

// Invalidate drops the cached list.
func (l *ServerList) Invalidate() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache, l.fetchedAt = nil, time.Time{}
}

// Servers returns the cached list, refreshing it when older than the TTL.
func (l *ServerList) Servers(ctx context.Context) ([]Server, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cache != nil && l.now().Sub(l.fetchedAt) < l.ttl {
		return l.cache, nil
	}
	servers, err := fetchServers(ctx, l.bin)
	if err != nil {
		return nil, err
	}
	l.cache, l.fetchedAt = servers, l.now()
	return servers, nil
}

func fetchServers(ctx context.Context, bin string) ([]Server, error) {
	cmd := exec.CommandContext(ctx, bin, "-L", "-f", "json")
	cmd.SysProcAttr = sysProcAttr()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s -L: %w", bin, err)
	}
	var doc struct {
		Servers []struct {
			ID       int64  `json:"id"`
			Host     string `json:"host"`
			Name     string `json:"name"`
			Location string `json:"location"`
			Country  string `json:"country"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("parse server list: %w", err)
	}
	servers := make([]Server, 0, len(doc.Servers))
	for _, s := range doc.Servers {
		servers = append(servers, Server{
			ID:       strconv.FormatInt(s.ID, 10),
			Name:     s.Name,
			Location: s.Location,
			Country:  s.Country,
			Host:     s.Host,
		})
	}
	return servers, nil
}
