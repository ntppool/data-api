package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"go.ntppool.org/data-api/server"
	"golang.org/x/sync/errgroup"
)

func (cli *CLI) serverCmd() *cobra.Command {

	var serverCmd = &cobra.Command{
		Use:   "server",
		Short: "server starts the API server",
		Long:  `starts the API server on (default) port 8000`,
		// DisableFlagParsing: true,
		// Args:  cobra.ExactArgs(1),
		RunE: cli.serverCLI,
	}

	return serverCmd
}

func (cli *CLI) serverCLI(cmd *cobra.Command, args []string) error {

	// cfg := cli.Config
	ctx, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		srv, err := server.NewServer(ctx)
		if err != nil {
			log.Printf("NewServer() error: %s", err)
			return fmt.Errorf("srv setup: %s", err)
		}
		return srv.Run()
	})

	err := g.Wait()
	if err != nil {
		log.Printf("server error: %s", err)
	}

	cancel()
	return err

}
