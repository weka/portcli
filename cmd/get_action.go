package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/weka/portcli/internal/portfmt"
)

var (
	actionGetJSON bool
	actionGetEnum string
)

var actionGetCmd = &cobra.Command{
	Use:   "get <action-identifier>",
	Short: "Get a self-service action's input schema",
	Long: `Fetch a Port self-service action and display its inputs: type, whether required,
default, and allowed enum values.

Examples:
  portcli action get selfservice-deployment-test_run_test
  portcli action get selfservice-deployment-test_run_test --json
  portcli action get selfservice-deployment-test_run_test --enum test_name`,
	Args: cobra.ExactArgs(1),
	RunE: getAction,
}

func init() {
	actionGetCmd.Flags().BoolVar(&actionGetJSON, "json", false, "Output the input schema as JSON")
	actionGetCmd.Flags().StringVar(&actionGetEnum, "enum", "", "Print only the enum values of the named input, one per line")
}

func getAction(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	action, err := c.GetAction(args[0])
	if err != nil {
		return fmt.Errorf("failed to get action: %w", err)
	}
	inputs := action.Trigger.UserInputs

	// --enum: print just one input's allowed values, one per line (for scripting).
	if actionGetEnum != "" {
		prop, ok := inputs.Properties[actionGetEnum]
		if !ok {
			return fmt.Errorf("input %q not found on action %q", actionGetEnum, action.Identifier)
		}
		values, dynamic := prop.EnumValues()
		if dynamic {
			// Printing nothing here would read as "this input accepts nothing",
			// when in fact Port computes the choices at run time.
			return fmt.Errorf("input %q of action %q has no fixed enum: its values come from a server-side query, so they cannot be listed",
				actionGetEnum, action.Identifier)
		}
		for _, v := range values {
			fmt.Println(portfmt.AnyToString(v))
		}
		return nil
	}

	// --json: machine-readable schema (properties + required + blueprint).
	if actionGetJSON {
		out, _ := json.MarshalIndent(map[string]any{
			"identifier": action.Identifier,
			"title":      action.Title,
			"blueprint":  action.Trigger.BlueprintIdentifier,
			"required":   inputs.Required,
			"properties": inputs.Properties,
		}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	// Default: human-readable table.
	required := make(map[string]bool, len(inputs.Required))
	for _, r := range inputs.Required {
		required[r] = true
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "INPUT\tTYPE\tREQUIRED\tDEFAULT\tENUM")
	for _, name := range portfmt.InputOrder(inputs.Order, inputs.Properties) {
		p := inputs.Properties[name]
		req := ""
		if required[name] {
			req = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, p.Type, req, portfmt.AnyToString(p.Default), enumColumn(p))
	}
	w.Flush()
	return nil
}

// enumColumn renders the ENUM cell for one input. A jq-driven enum has no
// values to show, so it is named as such instead of rendering blank alongside
// inputs that genuinely accept anything.
func enumColumn(p client.ActionInput) string {
	values, dynamic := p.EnumValues()
	if dynamic {
		return "<dynamic>"
	}
	return portfmt.EnumPreview(values)
}
