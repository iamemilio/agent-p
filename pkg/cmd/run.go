package cmd

import (
	"github.com/spf13/cobra"
)

const (
	defaultJobsDir        = "jobs"
	defaultConfigFileName = "config.yaml"
)

// runCmd represents the run command
var run = &cobra.Command{
	Use:   "run [config.yaml]",
	Short: "Run a batch of performance profiling jobs. Optionally pass a specific config file.",
	Long: `Run generates, then runs the jobs outlined in the config file. It will collect metrics
on those jobs and output it the created directory for a given job as a file called data.csv.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputs.Run = &Run{}
		inputs.ShouldExit = false
		if len(args) == 0 {
			inputs.Run.Config = defaultConfigFileName
		} else {
			inputs.Run.Config = args[0]
		}
	},
}

func init() {
	rootCmd.AddCommand(run)
	run.Flags().BoolVarP(&inputs.CleanRun, "no-clean", "c", true, "do not clean up docker resources when run completes")
}
