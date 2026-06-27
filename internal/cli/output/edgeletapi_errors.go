package output

import "strings"

const execStartTimeoutMessage = "Interactive shell did not start within 15 seconds. Retry `edgelet ms exec`; if the problem persists, check microservice and engine logs."

// FormatEdgeletAPIErrorMessage maps stable EdgeletAPI error codes to operator-facing CLI text.
func FormatEdgeletAPIErrorMessage(code, serverMessage string) string {
	switch strings.TrimSpace(code) {
	case "EXEC_START_TIMEOUT":
		return execStartTimeoutMessage
	default:
		return strings.TrimSpace(serverMessage)
	}
}
