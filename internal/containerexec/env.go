package containerexec

import "strings"

const (
	// DefaultExecTerm is used for interactive TTY exec when the container has no TERM.
	DefaultExecTerm = "xterm-256color"
)

// ExecEnvForTTY returns container env with TERM set for interactive exec sessions.
// An existing container TERM is preserved. When TERM is missing, preferredTerm is
// used when non-empty; otherwise DefaultExecTerm is injected.
func ExecEnvForTTY(containerEnv []string, preferredTerm string) []string {
	if hasEnvKey(containerEnv, "TERM") {
		out := make([]string, len(containerEnv))
		copy(out, containerEnv)
		return out
	}
	term := strings.TrimSpace(preferredTerm)
	if term == "" {
		term = DefaultExecTerm
	}
	out := make([]string, len(containerEnv), len(containerEnv)+1)
	copy(out, containerEnv)
	return append(out, "TERM="+term)
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}
