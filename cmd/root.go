/*
Copyright © 2023 Ask Bjørn Hansen

See LICENSE
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
)

var cfgFile string

type CLI struct {
	root *cobra.Command
}

func NewCLI() *CLI {
	cli := &CLI{}
	cli.root = cli.rootCmd()

	cli.init(cli.root)
	return cli
}

// RootCmd represents the base command when called without any subcommands
func (cli *CLI) rootCmd() *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "data-api",
		Short: "A brief description of your application",
		// Uncomment the following line if your bare application
		// has an action associated with it:
		//	Run: func(cmd *cobra.Command, args []string) { },
	}

	cmd.AddCommand(cli.serverCmd())
	cmd.AddCommand(version.VersionCmd("data-api"))

	return cmd
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {

	cli := NewCLI()

	if err := cli.root.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func (cli *CLI) init(cmd *cobra.Command) {

	logger.Setup()

	cmd.PersistentFlags().StringVar(&cfgFile, "database-config", "database.yaml", "config file (default is $HOME/.data-api.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	cmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
