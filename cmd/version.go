package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) addVersionCommands() {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "cctote version "+a.root.Version)
			return err
		},
	}
	a.root.AddCommand(versionCmd)
}
