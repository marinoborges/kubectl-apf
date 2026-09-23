package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marinoborges/kubectl-apf/internal/print"
)

func newDetail(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "detail <priority-level>|all",
		Short: "Show one priority level, or all of them, with its flow schemas",
		Example: `  # One priority level and the flow schemas that select into it
  kubectl apf detail workload-low

  # Every priority level, with its flow schemas
  kubectl apf detail all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if len(args) != 1 {
				if err := cmd.Help(); err != nil {
					return err
				}
				return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
			}
			snapshot, err := opts.prioritySnapshot(cmd.Context())
			if err != nil {
				return err
			}
			snapshot, err = snapshot.Detail(args[0])
			if err != nil {
				return err
			}
			return print.Detail(cmd.OutOrStdout(), snapshot, args[0] == "all")
		},
	}
}
