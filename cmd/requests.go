package cmd

import (
	"github.com/spf13/cobra"

	"github.com/marinoborges/kubectl-apf/internal/apf"
	"github.com/marinoborges/kubectl-apf/internal/print"
)

func newRequests(opts *options) *cobra.Command {
	var omitObserver bool
	cmd := &cobra.Command{
		Use:   "requests [priority-level...]",
		Short: "List requests waiting or executing in API Priority and Fairness",
		Long: `List requests that are waiting in a queue or executing right now.

STATE queued means the request is still waiting for a seat.
STATE executing means it has been dispatched and is running.
WAIT for a queued request is how long it has been waiting. For an executing
request it is how long that request waited before it started.
SEATS is the number of seats the request holds during normal execution.
QUEUE is the shuffle-shard queue index inside the priority level. "-" means
the level has no queues. The same number is INDEX in kubectl apf queues.

This is a snapshot from /debug/api_priority_and_fairness/dump_requests.
This plugin can only poll that endpoint and keep the rows it sees. Requests
that start and finish between polls never appear. A record of requests over a
long interval is the API server audit log, where each request is logged with
the flow schema and priority level that handled it.`,
		Example: `  # Requests for one or more priority levels
  kubectl apf requests workload-low
  kubectl apf requests workload-low workload-high`,
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
			return print.Requests(cmd.OutOrStdout(), requests)
		},
	}
	cmd.Flags().BoolVar(&omitObserver, "omit-observer", false, "Omit the request this command sent to the debug endpoint")
	return cmd
}
