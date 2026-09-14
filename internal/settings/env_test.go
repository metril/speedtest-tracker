package settings

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestEnvKeyToSetting(t *testing.T) {
	for env, want := range map[string]string{
		"ST_AUTH_MODE":                   KeyAuthMode,
		"ST_GENERAL_RETENTION_DAYS_RUNS": KeyRetentionDaysRuns,
		"ST_INTEGRATIONS_VM_URL":         KeyVMURL,
	} {
		got, ok := EnvKeyToSetting(env)
		if !ok || got != want {
			t.Errorf("EnvKeyToSetting(%q) = %q, %v; want %q", env, got, ok, want)
		}
	}
	for _, env := range []string{"ST_DB_PATH", "ST_LISTEN", "ST_LOCK_ENV", "PATH", "ST_", "ST_AUTH"} {
		if got, ok := EnvKeyToSetting(env); ok {
			t.Errorf("EnvKeyToSetting(%q) = %q, want not a setting", env, got)
		}
	}
}

func TestSeedFromEnvSeedsOnlyMissingKeys(t *testing.T) {
	st, _ := newTestSettings(t)
	ctx := context.Background()
	if err := st.Set(ctx, KeyAuthAdminGroup, "stored"); err != nil {
		t.Fatal(err)
	}
	locked, err := st.SeedFromEnv(ctx, []string{
		"ST_AUTH_MODE=token",
		"ST_AUTH_ADMIN_GROUP=from-env",
		"ST_AUTH_TRUSTED_PROXIES=[\"10.0.0.0/8\"]",
		"ST_NOT_A_SECTION_KEY=x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(locked) != 0 {
		t.Fatalf("locked = %v, want none without ST_LOCK_ENV", locked)
	}
	got, _ := st.Auth(ctx)
	if got.Mode != "token" {
		t.Errorf("mode = %q, want the env seed to fill a missing key", got.Mode)
	}
	if got.AdminGroup != "stored" {
		t.Errorf("admin group = %q, want the stored value to win when unlocked", got.AdminGroup)
	}
	if len(got.TrustedProxies) != 1 || got.TrustedProxies[0] != "10.0.0.0/8" {
		t.Errorf("trusted proxies = %v, want the JSON env value decoded", got.TrustedProxies)
	}
}

func TestSeedFromEnvLockedOverwritesAndLocks(t *testing.T) {
	st, _ := newTestSettings(t)
	ctx := context.Background()
	st.Set(ctx, KeyAuthAdminGroup, "stored")
	locked, err := st.SeedFromEnv(ctx, []string{
		"ST_LOCK_ENV=true",
		"ST_AUTH_ADMIN_GROUP=from-env",
		"ST_AUTH_MODE=forward_auth",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(locked, []string{"auth.admin_group", "auth.mode"}) {
		t.Fatalf("locked = %v, want both keys sorted", locked)
	}
	got, _ := st.Auth(ctx)
	if got.AdminGroup != "from-env" {
		t.Errorf("admin group = %q, want the env value to win when locked", got.AdminGroup)
	}
	if !st.IsLocked(KeyAuthMode) || st.IsLocked(KeyAuthUserHeader) {
		t.Errorf("IsLocked = %v / %v", st.IsLocked(KeyAuthMode), st.IsLocked(KeyAuthUserHeader))
	}
	if !slices.Equal(st.LockedKeys(), locked) {
		t.Errorf("LockedKeys() = %v, want %v", st.LockedKeys(), locked)
	}
}

func TestSeedFromEnvValueDecoding(t *testing.T) {
	st, _ := newTestSettings(t)
	ctx := context.Background()
	if _, err := st.SeedFromEnv(ctx, []string{
		"ST_GENERAL_RETENTION_DAYS_RUNS=45",
		"ST_INTEGRATIONS_VM_ENABLED=true",
		"ST_GENERAL_TIMEZONE=Europe/Berlin",
	}); err != nil {
		t.Fatal(err)
	}
	g, _ := st.General(ctx)
	if g.RetentionDaysRuns != 45 || g.Timezone != "Europe/Berlin" {
		t.Fatalf("general = %+v, want a number decoded as a number and a bare string as a string", g)
	}
	i, _ := st.Integrations(ctx)
	if !i.VMEnabled {
		t.Fatal("vm_enabled = false, want true decoded from the bare env value")
	}
}

func TestSeedFromEnvRejectsUndecodableValue(t *testing.T) {
	st, _ := newTestSettings(t)
	_, err := st.SeedFromEnv(context.Background(), []string{"ST_GENERAL_RETENTION_DAYS_RUNS=soon"})
	if err == nil || !strings.Contains(err.Error(), "ST_GENERAL_RETENTION_DAYS_RUNS") {
		t.Fatalf("err = %v, want one naming the offending variable", err)
	}
}
