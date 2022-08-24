package cmd

import (
	"github.com/spf13/cobra"
)

var createConfig = &cobra.Command{
	Use:   "config [config-file-name.yaml]",
	Short: "Create a default yaml config file",
	Long: `Generate a template config file to help speed up your development. This command
can optionally take a name for that file, but when unspecified, it will create a file named
config.yaml in your working directory.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputs.Create = &Create{}
		inputs.ShouldExit = false
		if len(args) == 0 {
			inputs.Create.Config = defaultConfigFileName
		} else {
			inputs.Create.Config = args[0]
		}
	},
}

var createJobs = &cobra.Command{
	Use:   "jobs",
	Short: "Consume a config file and generate jobs",
	Long: `When passed a valid config file, this command will generate
the docker compose jobs needed to fulfil it. It will always create those
jobs in the "jobs" directory in the working directory this command was run from.
Note that this will not check if jobs already exist, and will duplicate existing
jobs if run multiple times from the same config.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputs.ShouldExit = false
		inputs.Create = &Create{
			Jobs: true,
		}
		if len(args) == 0 {
			inputs.Create.Config = defaultConfigFileName
		} else {
			inputs.Create.Config = args[0]
		}
	},
}

// createCmd represents the create command
var create = &cobra.Command{
	Use:   "create",
	Short: "Create resources for a run",
	Long: `For each stage in a run there are specific resources that
need to exist or be created. The Create command lets you create those specific
resources and stage them without running them. This lets you see each step of
the process and can help with debugging.`,
}

func init() {
	rootCmd.AddCommand(create)
	create.AddCommand(createJobs)
	create.AddCommand(createConfig)
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// createCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
