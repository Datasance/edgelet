package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/eclipse-iofog/agent/internal/cli"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

var errorCodePattern = regexp.MustCompile(`^Error\[([A-Z_]+)\]:`)

func main() {
	// Handle all other commands
	args := os.Args[1:]
	result := cli.HandleCommand(args)
	trimmedResult := strings.TrimSpace(result)
	if strings.HasPrefix(trimmedResult, "__EXIT_CODE__=") {
		codeRaw := strings.TrimSpace(strings.TrimPrefix(trimmedResult, "__EXIT_CODE__="))
		if code, err := strconv.Atoi(codeRaw); err == nil {
			os.Exit(code)
		}
		os.Exit(1)
	}
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	// Print result
	fmt.Print(result)

	// Exit with appropriate code
	if len(args) == 0 {
		return
	}
	if isErrorResult(result) {
		os.Exit(mapExitCode(result))
	}
}

func isErrorResult(result string) bool {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return false
	}
	return strings.HasPrefix(trimmed, "Error[")
}

func mapExitCode(result string) int {
	trimmed := strings.TrimSpace(result)
	matches := errorCodePattern.FindStringSubmatch(trimmed)
	if len(matches) < 2 {
		return 1
	}
	switch matches[1] {
	case "INVALID_ARGUMENT":
		return 2
	case "UNAUTHORIZED", "FORBIDDEN":
		return 3
	case "NOT_FOUND":
		return 4
	case "CONFLICT":
		return 5
	case "NOT_IMPLEMENTED":
		return 6
	default:
		return 1
	}
}
