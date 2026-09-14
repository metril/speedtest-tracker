package store

import (
	"context"
	"testing"
)

func sampleIperf3Servers() []Iperf3Server {
	return []Iperf3Server{
		{Host: "iperf.example.net", Port: 5201, PortEnd: 5210, Options: "-R,-u,-6", SupportsReverse: true, SupportsUDP: true, SupportsIPv6: true,
			GBs: "1", Continent: "EU", Country: "DE", Site: "Frankfurt", Provider: "Example Net"},
		{Host: "speed.other.net", Port: 5202, Options: "", SupportsReverse: false, SupportsUDP: false, SupportsIPv6: false,
			GBs: "10", Continent: "NA", Country: "US", Site: "Denver", Provider: "Other Net"},
	}
}

func TestReplaceIperf3ServersAndFetchedAt(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()

	if got, err := db.Iperf3ServersFetchedAt(ctx); err != nil || got != "" {
		t.Fatalf("Iperf3ServersFetchedAt on a fresh db = %q, %v, want empty", got, err)
	}
	if n, err := db.CountIperf3Servers(ctx); err != nil || n != 0 {
		t.Fatalf("CountIperf3Servers on a fresh db = %d, %v", n, err)
	}

	if err := db.ReplaceIperf3Servers(ctx, sampleIperf3Servers()); err != nil {
		t.Fatal(err)
	}

	n, err := db.CountIperf3Servers(ctx)
	if err != nil || n != 2 {
		t.Fatalf("CountIperf3Servers after replace = %d, %v, want 2", n, err)
	}
	fetchedAt, err := db.Iperf3ServersFetchedAt(ctx)
	if err != nil || fetchedAt == "" {
		t.Fatalf("Iperf3ServersFetchedAt after replace = %q, %v, want non-empty", fetchedAt, err)
	}

	// A second replace with a different set fully swaps the table rather
	// than merging into it.
	if err := db.ReplaceIperf3Servers(ctx, []Iperf3Server{
		{Host: "only.example.net", Port: 5201},
	}); err != nil {
		t.Fatal(err)
	}
	n, err = db.CountIperf3Servers(ctx)
	if err != nil || n != 1 {
		t.Fatalf("CountIperf3Servers after second replace = %d, %v, want 1", n, err)
	}
}

func TestReplaceIperf3ServersIgnoresDuplicateHostPort(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()

	// Two rows sharing the same host+port must not fail the whole batch:
	// the UNIQUE(host,port) constraint means only one survives.
	err := db.ReplaceIperf3Servers(ctx, []Iperf3Server{
		{Host: "dup.example.net", Port: 5201, Provider: "First"},
		{Host: "dup.example.net", Port: 5201, Provider: "Second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := db.CountIperf3Servers(ctx)
	if err != nil || n != 1 {
		t.Fatalf("CountIperf3Servers = %d, %v, want 1 (duplicate ignored)", n, err)
	}
}

// TestReplaceIperf3ServersRoundTripsPortEndAndIPv6 covers the columns added
// by migration 0006: PortEnd and SupportsIPv6 must survive a write/read
// round trip, and a server without a port range keeps PortEnd at 0.
func TestReplaceIperf3ServersRoundTripsPortEndAndIPv6(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()
	if err := db.ReplaceIperf3Servers(ctx, sampleIperf3Servers()); err != nil {
		t.Fatal(err)
	}
	got, err := db.SearchIperf3Servers(ctx, "", 50)
	if err != nil || len(got) != 2 {
		t.Fatalf("SearchIperf3Servers = %+v, %v, want 2 rows", got, err)
	}
	byHost := map[string]Iperf3Server{}
	for _, s := range got {
		byHost[s.Host] = s
	}
	ranged := byHost["iperf.example.net"]
	if ranged.PortEnd != 5210 || !ranged.SupportsIPv6 {
		t.Errorf("ranged server = %+v, want PortEnd=5210 SupportsIPv6=true", ranged)
	}
	single := byHost["speed.other.net"]
	if single.PortEnd != 0 || single.SupportsIPv6 {
		t.Errorf("single-port server = %+v, want PortEnd=0 SupportsIPv6=false", single)
	}
}

func TestSearchIperf3Servers(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()
	if err := db.ReplaceIperf3Servers(ctx, sampleIperf3Servers()); err != nil {
		t.Fatal(err)
	}

	all, err := db.SearchIperf3Servers(ctx, "", 50)
	if err != nil || len(all) != 2 {
		t.Fatalf("SearchIperf3Servers(\"\") = %d rows, %v, want 2", len(all), err)
	}

	byHost, err := db.SearchIperf3Servers(ctx, "iperf.example", 50)
	if err != nil || len(byHost) != 1 || byHost[0].Host != "iperf.example.net" {
		t.Fatalf("SearchIperf3Servers(host) = %+v, %v", byHost, err)
	}

	bySite, err := db.SearchIperf3Servers(ctx, "denver", 50)
	if err != nil || len(bySite) != 1 || bySite[0].Site != "Denver" {
		t.Fatalf("SearchIperf3Servers(site, case-insensitive) = %+v, %v", bySite, err)
	}

	byProvider, err := db.SearchIperf3Servers(ctx, "Other Net", 50)
	if err != nil || len(byProvider) != 1 || byProvider[0].Provider != "Other Net" {
		t.Fatalf("SearchIperf3Servers(provider) = %+v, %v", byProvider, err)
	}

	// "DE" also substring-matches "Denver" (the other row's site), so check
	// containment rather than an exact single-row match.
	byCountry, err := db.SearchIperf3Servers(ctx, "DE", 50)
	if err != nil {
		t.Fatalf("SearchIperf3Servers(country): %v", err)
	}
	foundDE := false
	for _, s := range byCountry {
		if s.Country == "DE" {
			foundDE = true
		}
	}
	if !foundDE {
		t.Fatalf("SearchIperf3Servers(country) = %+v, want the DE row included", byCountry)
	}

	none, err := db.SearchIperf3Servers(ctx, "nomatch", 50)
	if err != nil || len(none) != 0 {
		t.Fatalf("SearchIperf3Servers(no match) = %+v, %v, want empty", none, err)
	}

	limited, err := db.SearchIperf3Servers(ctx, "", 1)
	if err != nil || len(limited) != 1 {
		t.Fatalf("SearchIperf3Servers(limit=1) = %d rows, %v, want 1", len(limited), err)
	}
}

// TestSearchIperf3ServersEscapesLikeWildcards is a regression test: a query
// containing SQL LIKE metacharacters (%, _) must be matched literally, not
// as a wildcard that could match far more than intended.
func TestSearchIperf3ServersEscapesLikeWildcards(t *testing.T) {
	db := newTestStore(t)
	ctx := context.Background()
	servers := sampleIperf3Servers()
	servers = append(servers, Iperf3Server{Host: "literal%percent.example.net", Port: 5201, Provider: "100% Fiber"})
	if err := db.ReplaceIperf3Servers(ctx, servers); err != nil {
		t.Fatal(err)
	}

	// An unescaped "%" would match every row (it's already a wildcard), so
	// this must return only the row whose provider contains a literal "%".
	got, err := db.SearchIperf3Servers(ctx, "100%", 50)
	if err != nil {
		t.Fatalf("SearchIperf3Servers: %v", err)
	}
	if len(got) != 1 || got[0].Provider != "100% Fiber" {
		t.Fatalf("SearchIperf3Servers(%%q=100%%) = %+v, want only the literal-%% row", got)
	}
}

func TestReplaceIperf3ServersEmptyStillClearsTable(t *testing.T) {
	// ReplaceIperf3Servers itself always does what it's told: callers that
	// want to keep the existing table on an empty fetch (see
	// internal/iperf3list) must simply not call it with an empty slice.
	db := newTestStore(t)
	ctx := context.Background()
	if err := db.ReplaceIperf3Servers(ctx, sampleIperf3Servers()); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceIperf3Servers(ctx, nil); err != nil {
		t.Fatal(err)
	}
	n, err := db.CountIperf3Servers(ctx)
	if err != nil || n != 0 {
		t.Fatalf("CountIperf3Servers after empty replace = %d, %v, want 0", n, err)
	}
}
