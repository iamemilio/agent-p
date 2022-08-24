package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	appName = "agent-p"
)

type Inputs struct {
	ShouldExit bool
	Debug      bool
	Silent     bool
	CleanRun   bool
	*Run
	*Create
	*Clean
}

type Clean struct {
	Config string
}

type Run struct {
	Config string
}

type Create struct {
	Config string
	Jobs   bool
}

var (
	inputs = Inputs{}
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   appName,
	Short: "the easiest way to observe performance over time in the tri-state area",
	Long: `Agent P seems like just a regular pet platypus, but when your language agent
is up to no good in the tri-state area, Agent P comes to the rescue!
	
Agent P is a tool that simplifies the process of measuring the performance of an app
that is being monitored by a language agent. This tool is completely idempotentent, and
can be run multiple times. Note that running it will overwrite any collected data.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
// Populates the struct inputs with data gathered from the user inputs.
func Execute() Inputs {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}

	return inputs
}

func init() {
	inputs.ShouldExit = true
	rootCmd.PersistentFlags().BoolVarP(&inputs.Debug, "debug", "d", false, "enable debug level logging")
	rootCmd.PersistentFlags().BoolVarP(&inputs.Silent, "silent", "s", false, "disable all logs except for fatal errors")
}
