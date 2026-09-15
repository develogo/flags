package cmd

import (
	"flags/internal/relayconfig"
	"flags/internal/services"
	"os"

	"github.com/spf13/cobra"
)

var (
	relayConfigAppsDir string
	relayConfigOutput  string
)

var relayConfigCmd = &cobra.Command{
	Use:   "relay-config",
	Short: "Generate the GOFF relay config with one flag set per app",
	Long: `Discover the apps in --apps-dir (one <app>.yaml per app) and write the GOFF relay
config with one flag set per app, named and keyed by the app name. Retriever paths are
built from --apps-dir, so pass the directory as the relay will see it.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true, // Execute já imprime o erro
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := relayconfig.Generate(relayConfigAppsDir)
		if err != nil {
			return err
		}
		if relayConfigOutput == "" || relayConfigOutput == "-" {
			_, err = cmd.OutOrStdout().Write(out)
			return err
		}
		return os.WriteFile(relayConfigOutput, out, 0o644)
	},
}

func init() {
	relayConfigCmd.Flags().StringVar(&relayConfigAppsDir, "apps-dir", services.DefaultFlagsDir, "directory with one <app>.yaml per app")
	relayConfigCmd.Flags().StringVarP(&relayConfigOutput, "output", "o", "", "output file (stdout if empty or -)")
	rootCmd.AddCommand(relayConfigCmd)
}
