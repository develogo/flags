package cmd

import (
	"flags/internal/config"
	"flags/internal/handlers"
	"flags/internal/middleware"
	"flags/internal/services"

	fxserver "flags/internal/fx"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the feature flag proxy server",
	Long:  `Start the HTTP server that proxies feature flag requests with authentication..`,
	Run: func(cmd *cobra.Command, args []string) {
		fx.New(
			config.Module,
			services.Module,
			handlers.Module,
			middleware.Module,
			fxserver.Module,
		).Run()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
