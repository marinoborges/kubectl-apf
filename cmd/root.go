// Package cmd is the kubectl-apf command line.
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/marinoborges/kubectl-apf/internal/apf"
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
