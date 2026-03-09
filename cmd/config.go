package cmd

import (
	"fmt"

	"github.com/alphaleonis/cctote/internal/config"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/spf13/cobra"
)

// resolveConfigPath returns the config file path, respecting the --config flag.
func (a *App) resolveConfigPath() (string, error) {
	if a.cfgPath != "" {
		return a.cfgPath, nil
	}
	return config.DefaultPath()
}

func (a *App) addConfigCommands() {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage cctote configuration",
	}

	configGetCmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print the current value of a config key",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runConfigGet,
	}

	configSetCmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config key",
		Args:  cobra.ExactArgs(2),
		RunE:  a.runConfigSet,
	}

	configResetCmd := &cobra.Command{
		Use:   "reset <key>",
		Short: "Reset a config key to its default value",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runConfigReset,
	}

	configListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all config keys with their current values",
		RunE:  a.runConfigList,
	}

	configCmd.AddCommand(configGetCmd, configSetCmd, configResetCmd, configListCmd)
	a.root.AddCommand(configCmd)
}

func (a *App) runConfigGet(cmd *cobra.Command, args []string) error {
	k, err := config.LookupKey(args[0])
	if err != nil {
		return err
	}

	path, err := a.resolveConfigPath()
	if err != nil {
		return err
	}

	c, err := config.Load(path)
	if err != nil {
		return err
	}

	value := k.Get(c)

	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"key":   k.Name,
			"value": value,
		})
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), value)
	return err
}

func (a *App) runConfigSet(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	k, err := config.LookupKey(args[0])
	if err != nil {
		return err
	}

	path, err := a.resolveConfigPath()
	if err != nil {
		return err
	}

	c, err := config.Load(path)
	if err != nil {
		return err
	}

	if err := k.Set(c, args[1]); err != nil {
		return err
	}

	if err := config.Save(path, c); err != nil {
		return err
	}

	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"key":   k.Name,
			"value": k.Get(c),
		})
	}

	w.Success("Set %s = %s", k.Name, k.Get(c))
	return nil
}

func (a *App) runConfigReset(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	k, err := config.LookupKey(args[0])
	if err != nil {
		return err
	}

	path, err := a.resolveConfigPath()
	if err != nil {
		return err
	}

	c, err := config.Load(path)
	if err != nil {
		return err
	}

	k.Reset(c)

	if err := config.Save(path, c); err != nil {
		return err
	}

	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"key":     k.Name,
			"default": k.Default(),
		})
	}

	w.Success("Reset %s to default (%s)", k.Name, k.Default())
	return nil
}

func (a *App) runConfigList(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	path, err := a.resolveConfigPath()
	if err != nil {
		return err
	}

	c, err := config.Load(path)
	if err != nil {
		return err
	}

	configKeys := config.Keys()
	if a.jsonOutput {
		items := make([]map[string]any, 0, len(configKeys))
		for _, k := range configKeys {
			items = append(items, map[string]any{
				"key":     k.Name,
				"value":   k.Get(c),
				"default": k.Default(),
				"isSet":   k.IsSet(c),
			})
		}
		return writeJSON(cmd, items)
	}

	rows := make([][]string, 0, len(configKeys))
	for _, k := range configKeys {
		source := "default"
		if k.IsSet(c) {
			source = "set"
		}
		rows = append(rows, []string{k.Name, k.Get(c), source})
	}
	w.Table(cmd.OutOrStdout(), []string{"KEY", "VALUE", "SOURCE"}, rows)
	return nil
}
