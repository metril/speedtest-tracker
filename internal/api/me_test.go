package api

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/metril/speedtest-tracker/internal/auth"
	"github.com/metril/speedtest-tracker/internal/settings"
)

// TestDisplayNameResolution exercises the fallback chain used by
// GET /auth/me to pick a display name: the configured
// auth.oidc_display_claim wins when its claim is present, otherwise
// name -> username -> email -> user. Outside oidc mode, DisplayName is
// always the resolved user.
func TestDisplayNameResolution(t *testing.T) {
	_, _, st := newTestAPIWithSettings(t)

	for _, tc := range []struct {
		name  string
		mode  string
		claim string
		id    auth.Identity
		want  string
	}{
		{
			name: "non-oidc mode always uses user",
			mode: "token",
			id:   auth.Identity{User: "alice", Name: "Alice", Username: "al", Email: "a@example.com"},
			want: "alice",
		},
		{
			name:  "claim name present",
			mode:  settings.AuthModeOIDC,
			claim: settings.OIDCDisplayClaimName,
			id:    auth.Identity{User: "u1", Name: "Alice", Username: "al", Email: "a@example.com"},
			want:  "Alice",
		},
		{
			name:  "claim preferred_username present",
			mode:  settings.AuthModeOIDC,
			claim: settings.OIDCDisplayClaimUsername,
			id:    auth.Identity{User: "u1", Name: "Alice", Username: "al", Email: "a@example.com"},
			want:  "al",
		},
		{
			name:  "claim email present",
			mode:  settings.AuthModeOIDC,
			claim: settings.OIDCDisplayClaimEmail,
			id:    auth.Identity{User: "u1", Name: "Alice", Username: "al", Email: "a@example.com"},
			want:  "a@example.com",
		},
		{
			name:  "claim username missing falls back to name",
			mode:  settings.AuthModeOIDC,
			claim: settings.OIDCDisplayClaimUsername,
			id:    auth.Identity{User: "u1", Name: "Alice", Email: "a@example.com"},
			want:  "Alice",
		},
		{
			name:  "claim name missing falls back to username",
			mode:  settings.AuthModeOIDC,
			claim: settings.OIDCDisplayClaimName,
			id:    auth.Identity{User: "u1", Username: "al", Email: "a@example.com"},
			want:  "al",
		},
		{
			name:  "nothing but email falls back to email",
			mode:  settings.AuthModeOIDC,
			claim: settings.OIDCDisplayClaimName,
			id:    auth.Identity{User: "u1", Email: "a@example.com"},
			want:  "a@example.com",
		},
		{
			name:  "nothing at all falls back to user",
			mode:  settings.AuthModeOIDC,
			claim: settings.OIDCDisplayClaimName,
			id:    auth.Identity{User: "u1"},
			want:  "u1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.claim != "" {
				if err := st.Set(context.Background(), settings.KeyAuthOIDCDisplayClaim, tc.claim); err != nil {
					t.Fatal(err)
				}
			}
			d := Deps{Settings: st}
			r := httptest.NewRequest("GET", "/auth/me", nil)
			if got := d.displayName(r, tc.id, tc.mode); got != tc.want {
				t.Errorf("displayName = %q, want %q", got, tc.want)
			}
		})
	}
}
