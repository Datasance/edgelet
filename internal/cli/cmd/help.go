package cmd

import "github.com/spf13/cobra"

// cliHelpTemplate is inherited by subcommands. Long and Examples render through
// UsageString, which uses Cobra's default usage template (flags, examples, subcommands).
const cliHelpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

var cobraDefaultHelpFunc = (&cobra.Command{}).HelpFunc()

func printCommandHelp(c *cobra.Command, args []string) {
	if shouldPrintBanner(c, args) {
		printBannerTo(c.ErrOrStderr())
	}
	cobraDefaultHelpFunc(c, args)
}
