package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marinoborges/kubectl-apf/internal/apf"
	"github.com/marinoborges/kubectl-apf/internal/print"
)

func newQueues(opts *options) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "queues [priority-level...]",
		Aliases: []string{"queue"},
		Short:   "List API Priority and Fairness queues",
		Long: `List shuffle-shard queues from /debug/api_priority_and_fairness/dump_queues.

Idle queues are omitted. --all includes them. Names limit the list to those
priority levels.

LEVEL is the priority level that owns the queue.
INDEX is that queue's number inside the level, starting at 0. A level whose
response is "Queue 128/6/50" has indexes 0 through 127. Shuffle sharding hashes
the flow and places each request in one of those queues. The same index on two
levels is a different queue. kubectl apf requests shows this number in QUEUE
while a request is still waiting.
PENDING is how many requests are waiting in the queue.
EXECUTING is how many requests from this queue are running.
SEATS is how many execution seats those running requests hold.`,
		Example: `  # Queues that have requests waiting or executing
  kubectl apf queues

  # Every queue of one priority level, including idle ones
  kubectl apf queues workload-low --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			loader, err := opts.loader()
			if err != nil {
				return err
			}
			queues, err := loader.Queues(cmd.Context())
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
				queues = apf.FilterQueues(queues, args)
			}
			if !all {
				queues = apf.ActiveQueues(queues)
			}
			if all && len(queues) == 0 {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "No queues found.")
				return err
			}
			return print.Queues(cmd.OutOrStdout(), queues)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Include idle queues")
	return cmd
}
