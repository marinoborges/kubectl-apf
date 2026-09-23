package cmd

import (
	"github.com/spf13/cobra"

	"github.com/marinoborges/kubectl-apf/internal/apf"
	"github.com/marinoborges/kubectl-apf/internal/print"
)

func newUsers(opts *options) *cobra.Command {
	var omitObserver bool
	cmd := &cobra.Command{
		Use:   "users [priority-level...]",
		Short: "Show which users are waiting or holding seats",
		Long: `Group the current dump_requests rows by user.

WAITING is how many of that user's requests are still queued.
EXECUTING is how many are running.
SEATS is the sum of initial seats of the executing requests, which is the
concurrency that user is holding right now.`,
		Example: `  kubectl apf users
  kubectl apf users workload-low`,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := opts.loader()
			if err != nil {
				return err
			}
			requests, err := loader.Requests(cmd.Context())
			if err != nil {
				return err
			}
			if len(args) > 0 {
				snapshot, err := loader.Snapshot(cmd.Context(), false)
				if err != nil {
					return err
				}
				if err := snapshot.RequireLevels(args); err != nil {
					return err
				}
				requests = apf.FilterRequests(requests, args)
			}
			if omitObserver {
				requests = apf.WithoutObserver(requests)
			}
			return print.Users(cmd.OutOrStdout(), apf.UserLoads(requests))
		},
	}
	cmd.Flags().BoolVar(&omitObserver, "omit-observer", false, "Omit the request this command sent to the debug endpoint")
	return cmd
}
