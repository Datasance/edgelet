package main

import (
	"os"

	"github.com/datasance/edgelet/internal/cli/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
