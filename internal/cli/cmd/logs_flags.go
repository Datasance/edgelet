package cmd

import (
	"github.com/datasance/edgelet/internal/cli/domain/logs"
	"github.com/spf13/cobra"
)

type logsFlagValues struct {
	follow     bool
	tail       string
	since      string
	until      string
	timestamps bool
}

func registerLogsFlags(cmd *cobra.Command, v *logsFlagValues) {
	cmd.Flags().BoolVarP(&v.follow, "follow", "f", false, "Follow log output")
	cmd.Flags().StringVar(&v.tail, "tail", logs.DefaultTail, "Number of lines to show from the end")
	cmd.Flags().StringVar(&v.since, "since", "", "Show logs since ISO8601 timestamp")
	cmd.Flags().StringVar(&v.until, "until", "", "Show logs until ISO8601 timestamp")
	cmd.Flags().BoolVar(&v.timestamps, "timestamps", false, "Show log timestamps")
}

func (v *logsFlagValues) options() logs.Options {
	return logs.Options{
		Follow:     v.follow,
		Tail:       v.tail,
		Since:      v.since,
		Until:      v.until,
		Timestamps: v.timestamps,
	}
}
