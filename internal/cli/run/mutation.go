package run

// WithSpinner runs fn with a stderr spinner in human mode.
// Structured output skips the spinner; the spinner is always stopped before return.
func WithSpinner(ctx *CLIContext, msg string, fn func() error) error {
	if ctx == nil {
		return NewCLIError(CodeInternal, "cli context is nil", nil)
	}
	if ctx.Format.IsStructured() {
		return fn()
	}
	if ctx.UI == nil {
		return NewCLIError(CodeInternal, "cli ui is nil", nil)
	}
	spin := ctx.UI.StartSpinner(msg)
	err := fn()
	spin.Stop()
	return err
}

// WithSpinnerHumanSuccess runs fn with a stderr spinner in human mode and writes the
// returned message via WriteHumanSuccess. Structured output skips the spinner and
// does not write success text; callers handle structured stdout separately.
func WithSpinnerHumanSuccess(ctx *CLIContext, msg string, fn func() (string, error)) error {
	if ctx == nil {
		return NewCLIError(CodeInternal, "cli context is nil", nil)
	}
	if ctx.Format.IsStructured() {
		_, err := fn()
		return err
	}
	if ctx.UI == nil {
		return NewCLIError(CodeInternal, "cli ui is nil", nil)
	}
	spin := ctx.UI.StartSpinner(msg)
	human, err := fn()
	spin.Stop()
	if err != nil {
		return err
	}
	return WriteHumanSuccess(ctx, human)
}
