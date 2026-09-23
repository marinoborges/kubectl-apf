package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/marinoborges/kubectl-apf/internal/apf"
	"github.com/marinoborges/kubectl-apf/internal/print"
)

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
