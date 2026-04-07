package main

import "testing"

func TestProfileEnabledPrefersCodeplaneEnv(t *testing.T) {
	t.Setenv("CODEPLANE_PROFILE", "1")
	t.Setenv("SMITHERS_TUI_PROFILE", "")
	t.Setenv("CRUSH_PROFILE", "")

	if !profileEnabled() {
		t.Fatal("expected CODEPLANE_PROFILE to enable profiling")
	}
}

func TestProfileEnabledFallsBackToLegacyEnv(t *testing.T) {
	t.Setenv("CODEPLANE_PROFILE", "")
	t.Setenv("SMITHERS_TUI_PROFILE", "1")
	t.Setenv("CRUSH_PROFILE", "")

	if !profileEnabled() {
		t.Fatal("expected SMITHERS_TUI_PROFILE to enable profiling as a fallback")
	}
}
