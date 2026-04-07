package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/invopop/jsonschema"
	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:    "schema",
	Short:  "Generate JSON schema for configuration",
	Long:   "Generate JSON schema for the Codeplane configuration file",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		bts, err := generateConfigSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(bts))
		return nil
	},
}

func generateConfigSchema() ([]byte, error) {
	reflector := new(jsonschema.Reflector)
	bts, err := json.MarshalIndent(reflector.Reflect(&config.Config{}), "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	schema := strings.NewReplacer(
		"https://github.com/charmbracelet/crush/internal/config/config", "https://charm.land/codeplane.json",
		"github.com/charmbracelet/crush/internal/config.", "codeplane.internal.config.",
		"https://charm.land/crush.json", "https://charm.land/codeplane.json",
	).Replace(string(bts))

	return []byte(schema), nil
}
