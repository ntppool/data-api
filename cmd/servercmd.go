package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/version"
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
	log := logger.Setup()

	// cfg := cli.Config
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	log.Info("starting", "version", version.Version())

	srv, err := server.NewServer(ctx, cfgFile)
	if err != nil {
		return fmt.Errorf("srv setup: %s", err)
	}

	g.Go(func() error {
		return srv.Run()
	})

	g.Go(func() error {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})

	err = g.Wait()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server error", "err", err)
	}

	// don't tell cobra something went wrong as it'll just
	// print usage information
	return nil
}
