package run

import (
	"io"

	"github.com/datasance/edgelet/internal/cli/output"
	"github.com/datasance/edgelet/internal/cli/ui"
)

// CLIContext carries shared runtime state for Cobra commands.
type CLIContext struct {
	UI        *ui.UI
	Out       io.Writer
	ErrOut    io.Writer
	Format    output.Format
	Quiet     bool
	Verbose   bool
	Debug     bool
	Socket    string
	Timeout   string
	NoColor   bool
	Client    V3Client
	Version   string
	BuildTime string
	GitCommit string
}

// NewCLIContext builds a CLIContext with UI defaults.
func NewCLIContext(opts ui.Options, format output.Format) *CLIContext {
	u := ui.New(opts)
	return &CLIContext{
		UI:      u,
		Out:     u.Out,
		ErrOut:  u.ErrOut,
		Format:  format,
		Quiet:   opts.Quiet,
		NoColor: opts.NoColor,
	}
}

// NewCLIContextWithWriters builds a CLIContext for tests.
func NewCLIContextWithWriters(out, errOut io.Writer, opts ui.Options, format output.Format) *CLIContext {
	u := ui.NewWithWriters(out, errOut, opts)
	return &CLIContext{
		UI:      u,
		Out:     out,
		ErrOut:  errOut,
		Format:  format,
		NoColor: opts.NoColor,
		Quiet:   opts.Quiet,
	}
}
