package cmd

import (
	"github.com/spf13/cobra"

	"github.com/marinoborges/kubectl-apf/internal/print"
)

func newLevels(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:     "plcs",
		Aliases: []string{"plc"},
		Short:   "List priority level configurations",
		Long: `List PriorityLevelConfiguration objects and the live counters.

This is a snapshot. Waiting, executing, rejected, timed-out, and cancelled
counts come from /debug/api_priority_and_fairness/dump_priority_levels.
QUEUES comes from /debug/api_priority_and_fairness/dump_queues.

SHARES is nominal concurrency shares. The percentage is this level's shares
divided by the sum of nominal concurrency shares of every priority level.
WAITING is requests waiting in this level's queues right now.
EXECUTING is requests running right now.
REJECTED is requests rejected since this API server process started.
TIMEDOUT is requests whose wait ended because their deadline passed, since
this process started.
CANCELLED is requests the client cancelled while they were waiting, since
this process started.
QUEUES is busy/total shuffle-shard queues. A queue is busy when it has a
request waiting, executing, or holding seats.

Other fields are documented at
https://kubernetes.io/docs/reference/kubernetes-api/flowcontrol/priority-level-configuration-v1/
and by kubectl explain prioritylevelconfiguration.spec`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			snapshot, err := opts.prioritySnapshot(cmd.Context())
			if err != nil {
				return err
			}
			return print.Levels(cmd.OutOrStdout(), snapshot)
		},
	}
}
