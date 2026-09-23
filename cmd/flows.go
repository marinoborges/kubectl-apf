package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marinoborges/kubectl-apf/internal/print"
)

func newFlows(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:     "flows [name]",
		Aliases: []string{"flowschema", "flowschemas"},
		Short:   "List flow schemas in matching order",
		Long: `List FlowSchema objects in matching order. A lower precedence is chosen first.

flows NAME prints that schema's subjects and the resource and non-resource
rules that select requests.

Other fields are documented at
https://kubernetes.io/docs/reference/kubernetes-api/flowcontrol/flow-schema-v1/
and by kubectl explain flowschema.spec`,
		Example: `  # Every flow schema
  kubectl apf flows

  # One flow schema, including subjects and rules
  kubectl apf flows service-accounts`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				if err := cmd.Help(); err != nil {
					return err
				}
				return fmt.Errorf("accepts at most 1 arg(s), received %d", len(args))
			}
			loader, err := opts.loader()
			if err != nil {
				return err
			}
			snapshot, err := loader.Snapshot(cmd.Context(), false)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				return print.Flows(cmd.OutOrStdout(), snapshot.Flows())
			}
			flow, ok := snapshot.FindFlow(args[0])
			if !ok {
				return fmt.Errorf("flow schema %q not found", args[0])
			}
			return print.FlowDetail(cmd.OutOrStdout(), flow)
		},
	}
}
