// Package cmd is the kubectl-apf command line.
package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/marinoborges/kubectl-apf/internal/apf"
	"github.com/marinoborges/kubectl-apf/internal/print"
)

// Version is overwritten by the release build.
var Version = "dev"

// Execute runs kubectl-apf.
func Execute(ctx context.Context) error {
	cmd := newRoot()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		fmt.Fprintln(c.ErrOrStderr(), c.UsageString())
		return err
	})
	return cmd.ExecuteContext(ctx)
}

type options struct {
	configFlags *genericclioptions.ConfigFlags
}

func newRoot() *cobra.Command {
	opts := &options{configFlags: genericclioptions.NewConfigFlags(false)}
	cmd := &cobra.Command{
		Use:   "apf",
		Short: "Visualize API Priority and Fairness priority levels and flow schemas",
		Long: `Show PriorityLevelConfiguration objects, the FlowSchemas that select into
them, and live waiting, executing, and rejected counters.

RESPONSE values of the form "Queue 128/6/50" are queues, hand size, and queue
length limit. A lower flow schema precedence is chosen first.`,
		Example: `  # Priority levels only
  kubectl apf plcs

  # One priority level and the flow schemas that select into it
  kubectl apf detail workload-low

  # Every priority level, with its flow schemas
  kubectl apf detail all

  # Flow schemas in the order the API server matches them
  kubectl apf flows

  # One flow schema, including subjects and rules
  kubectl apf flows service-accounts

  # Which flow schema a request would hit
  kubectl apf match --user system:serviceaccount:kube-system:replicaset-controller --verb list --resource pods --namespace kube-system

  # Who is holding seats right now
  kubectl apf users

  # Queues with requests waiting or executing
  kubectl apf queues

  # Requests waiting in a queue or executing
  kubectl apf requests

  # Requests for selected priority levels
  kubectl apf requests workload-low workload-high

  # Same list, without this command's own debug request
  kubectl apf requests --omit-observer`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	opts.configFlags.AddFlags(cmd.PersistentFlags())
	cmd.AddCommand(newLevels(opts), newDetail(opts), newFlows(opts), newMatch(opts), newUsers(opts), newQueues(opts), newRequests(opts), newVersion())
	return cmd
}

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

func newMatch(opts *options) *cobra.Command {
	var probe apf.Probe
	var groups []string
	cmd := &cobra.Command{
		Use:   "match",
		Short: "Show which flow schema a request would hit",
		Long: `Compare a user, verb, and resource with every FlowSchema and print the
one with the lowest precedence. Other matches are listed under it.

A system:serviceaccount:NAMESPACE:NAME user also matches ServiceAccount
subjects in that namespace, plus the groups system:serviceaccounts and
system:serviceaccounts:NAMESPACE. Every other user includes the group
system:authenticated. system:anonymous and system:unauthenticated include
system:unauthenticated. Extra groups are added with --group.`,
		Example: `  kubectl apf match --user system:serviceaccount:kube-system:replicaset-controller --verb list --resource pods --namespace kube-system
  kubectl apf match --user system:anonymous --verb get --non-resource-url /healthz`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if probe.User == "" || probe.Verb == "" {
				if err := cmd.Help(); err != nil {
					return err
				}
				return fmt.Errorf("--user and --verb are required")
			}
			if (probe.Resource == "") == (probe.NonResourceURL == "") {
				if err := cmd.Help(); err != nil {
					return err
				}
				return fmt.Errorf("set one of --resource or --non-resource-url")
			}
			if before, after, ok := strings.Cut(probe.Resource, "/"); ok {
				probe.Resource = before
				probe.Subresource = after
			}
			probe.Groups = groups
			loader, err := opts.loader()
			if err != nil {
				return err
			}
			snapshot, err := loader.Snapshot(cmd.Context(), false)
			if err != nil {
				return err
			}
			return print.Match(cmd.OutOrStdout(), snapshot.Match(probe))
		},
	}
	cmd.Flags().StringVar(&probe.User, "user", "", "Username, or system:serviceaccount:NAMESPACE:NAME")
	cmd.Flags().StringArrayVar(&groups, "group", nil, "Extra group. Repeat for more than one")
	cmd.Flags().StringVar(&probe.Verb, "verb", "", "API verb, for example get, list, or watch")
	cmd.Flags().StringVar(&probe.Resource, "resource", "", "Resource or resource/subresource, for example pods or pods/status")
	cmd.Flags().StringVar(&probe.Namespace, "namespace", "", "Namespace. Empty means a cluster-scoped request")
	cmd.Flags().StringVar(&probe.APIGroup, "api-group", "", "API group. Empty means the core group")
	cmd.Flags().StringVar(&probe.NonResourceURL, "non-resource-url", "", "Non-resource URL, for example /healthz")
	return cmd
}

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

func newVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the kubectl-apf version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), Version)
			return err
		},
	}
}

func (o *options) prioritySnapshot(ctx context.Context) (*apf.Snapshot, error) {
	loader, err := o.loader()
	if err != nil {
		return nil, err
	}
	snapshot, err := loader.Snapshot(ctx, true)
	if err != nil {
		return nil, err
	}
	queues, qerr := loader.Queues(ctx)
	if qerr != nil {
		note := "queue counts unavailable: " + qerr.Error()
		if snapshot.Warning != "" {
			snapshot.Warning += "; " + note
		} else {
			snapshot.Warning = note
		}
		return snapshot, nil
	}
	snapshot.ApplyQueueCounts(queues)
	return snapshot, nil
}

func (o *options) loader() (*apf.Loader, error) {
	cfg, err := o.configFlags.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	cfg.UserAgent = "kubectl-apf/" + Version
	return newLoader(cfg)
}

func newLoader(cfg *rest.Config) (*apf.Loader, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	raw, err := apf.NewRawGetter(cfg)
	if err != nil {
		return nil, err
	}
	return &apf.Loader{Client: client, Raw: raw}, nil
}
