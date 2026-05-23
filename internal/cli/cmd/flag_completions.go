package cmd

import "github.com/spf13/cobra"

func registerOutputFlagCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("output", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"human", "json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func registerSourceFlagCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("source", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"managed", "local", "all"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func registerDeprovisionScopeCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("scope", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"all", "local"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func registerSystemPruneModeCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("mode", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"dangling", "containers", "volumes", "all"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func registerImagePruneModeCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("mode", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"dangling"}, cobra.ShellCompDirectiveNoFileComp
	})
}
