package main

import (
	"os"

	"github.com/eclipse-iofog/agent/internal/cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
