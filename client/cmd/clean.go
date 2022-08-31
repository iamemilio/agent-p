package cmd

import "github.com/spf13/cobra"

var clean = &cobra.Command{
	Use:   "clean [config.yaml]",
	Short: "Deletes all docker containers, and networks left over from a run.",
	Long: `Uses docker compose down to remove any resources created during a run. It will look for a file named config.yaml
or consume the config file if optinally passed. It will only clean up resoures for jobs that are in the given config file.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputs.Clean = &Clean{}
		inputs.ShouldExit = false
		if len(args) == 0 {
			inputs.Clean.Config = defaultConfigFileName
		} else {
			inputs.Clean.Config = args[0]
		}
	},
}

func init() {
	rootCmd.AddCommand(clean)
}
