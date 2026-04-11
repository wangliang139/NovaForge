package restart

import (
	"testing"
)

func TestSigtermRestartEnabled(t *testing.T) {
	t.Setenv("ENABLE_SIGTERM_RESTART", "")
	if SigtermRestartEnabled() {
		t.Fatal("expected disabled when unset")
	}
	t.Setenv("ENABLE_SIGTERM_RESTART", "1")
	if !SigtermRestartEnabled() {
		t.Fatal("expected enabled for 1")
	}
	t.Setenv("ENABLE_SIGTERM_RESTART", "true")
	if !SigtermRestartEnabled() {
		t.Fatal("expected enabled for true")
	}
	t.Setenv("ENABLE_SIGTERM_RESTART", "0")
	if SigtermRestartEnabled() {
		t.Fatal("expected disabled for 0")
	}
}
