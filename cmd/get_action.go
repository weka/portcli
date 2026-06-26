package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
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
		for _, v := range prop.Enum {
			fmt.Println(anyToString(v))
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
	for _, name := range inputOrder(inputs.Order, inputs.Properties) {
		p := inputs.Properties[name]
		req := ""
		if required[name] {
			req = "yes"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, p.Type, req, anyToString(p.Default), enumPreview(p.Enum))
	}
	w.Flush()
	return nil
}

// inputOrder returns input names in the action's declared order, appending any
// not listed in order (sorted) so nothing is dropped.
func inputOrder(order []string, props map[string]client.ActionInput) []string {
	seen := make(map[string]bool, len(props))
	names := make([]string, 0, len(props))
	for _, name := range order {
		if _, ok := props[name]; ok && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	rest := make([]string, 0, len(props))
	for name := range props {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(names, rest...)
}

// anyToString renders a JSON value as a plain string (strings as-is, others as JSON).
func anyToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	out, _ := json.Marshal(v)
	return string(out)
}

// enumPreview joins enum values for the table, truncating long lists.
func enumPreview(enum []any) string {
	if len(enum) == 0 {
		return ""
	}
	vals := make([]string, len(enum))
	for i, v := range enum {
		vals[i] = anyToString(v)
	}
	const max = 6
	if len(vals) > max {
		return fmt.Sprintf("%s,… (+%d more)", strings.Join(vals[:max], ","), len(vals)-max)
	}
	return strings.Join(vals, ",")
}
