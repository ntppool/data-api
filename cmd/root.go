/*
Copyright © 2023 Ask Bjørn Hansen

See LICENSE
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.ntppool.org/common/version"

	"github.com/spf13/viper"
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
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
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
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.data-api.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	cmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".data-api" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".data-api")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
