package main

import (
	"fmt"
	"os"

	"github.com/eclipse-iofog/agent/internal/cli"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	// Handle version command early
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("ioFog Agent %s (built %s, commit %s)\n", version, buildTime, gitCommit)
		os.Exit(0)
	}

	// Handle all other commands
	args := os.Args[1:]
	result := cli.HandleCommand(args)

	// Print result
	fmt.Print(result)

	// Exit with appropriate code
	if result != "" && len(args) > 0 {
		// Check if result indicates an error
		if contains(result, "Error") || contains(result, "error") {
			os.Exit(1)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			indexOf(s, substr) >= 0)))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
