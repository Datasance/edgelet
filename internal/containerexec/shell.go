package containerexec

import "fmt"

// ShellCommandInteractive returns argv for an interactive exec session when no
// command is supplied. Prefers bash, then sh, then busybox.
func ShellCommandInteractive() []string {
	return []string{
		"/bin/sh", "-lc",
		"if [ -x /bin/bash ]; then exec /bin/bash; elif [ -x /bin/sh ]; then exec /bin/sh; else exec /busybox/sh; fi",
	}
}

// ShellCommandForScript runs script inside the container using the best available
// shell (bash → sh → busybox). For interactive exec default, use ShellCommandInteractive.
func ShellCommandForScript(script string) []string {
	q := shellQuote(script)
	wrapper := fmt.Sprintf(
		"if [ -x /bin/bash ]; then exec /bin/bash -c %s; elif [ -x /bin/sh ]; then exec /bin/sh -c %s; else exec /busybox/sh -c %s; fi",
		q, q, q,
	)
	return []string{"/bin/sh", "-lc", wrapper}
}

// shellQuote wraps s in single quotes for safe embedding in a POSIX shell -c script.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			out = append(out, '\'', '\\', '\'', '\'')
			continue
		}
		out = append(out, s[i])
	}
	out = append(out, '\'')
	return string(out)
}
