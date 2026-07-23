package containerexec

import "testing"

func TestExecEnvForTTY_PreservesExistingTerm(t *testing.T) {
	in := []string{"PATH=/bin", "TERM=linux", "HOME=/root"}
	got := ExecEnvForTTY(in, "xterm")
	if len(got) != len(in) {
		t.Fatalf("len(got)=%d want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("got[%d]=%q want %q", i, got[i], in[i])
		}
	}
}

func TestExecEnvForTTY_UsesPreferredTerm(t *testing.T) {
	got := ExecEnvForTTY([]string{"PATH=/bin"}, "vt100")
	if got[len(got)-1] != "TERM=vt100" {
		t.Fatalf("last env = %q, want TERM=vt100", got[len(got)-1])
	}
}

func TestExecEnvForTTY_DefaultTerm(t *testing.T) {
	got := ExecEnvForTTY(nil, "")
	if len(got) != 1 || got[0] != "TERM="+DefaultExecTerm {
		t.Fatalf("got=%v want [%q]", got, "TERM="+DefaultExecTerm)
	}
}

func TestExecEnvForTTY_NilContainerEnv(t *testing.T) {
	got := ExecEnvForTTY(nil, "xterm")
	if len(got) != 1 || got[0] != "TERM=xterm" {
		t.Fatalf("got=%v", got)
	}
}
