package apf

import (
	"strings"

	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
)

// Probe is a request to classify against the cluster's flow schemas.
type Probe struct {
	User           string
	Groups         []string
	Verb           string
	Resource       string
	Subresource    string
	APIGroup       string
	Namespace      string
	NonResourceURL string
}

// Match returns the flow schemas that match the probe, lowest precedence first.
// Dangling schemas are skipped because the API server does not select them.
func (s *Snapshot) Match(probe Probe) []Flow {
	groups := groupsFor(probe.User, probe.Groups)
	var matched []Flow
	for _, flow := range s.Flows() {
		if flow.Dangling {
			continue
		}
		if flowMatches(flow, probe, groups) {
			matched = append(matched, flow)
		}
	}
	sortFlows(matched)
	return matched
}

func flowMatches(flow Flow, probe Probe, groups []string) bool {
	for _, rule := range flow.Rules {
		if ruleMatches(rule, probe, groups) {
			return true
		}
	}
	return false
}

func ruleMatches(rule Rule, probe Probe, groups []string) bool {
	if !subjectMatches(rule.Subjects, probe.User, groups) {
		return false
	}
	if probe.NonResourceURL != "" {
		return nonResourceMatches(rule.NonResources, probe.Verb, probe.NonResourceURL)
	}
	return resourceMatches(rule.Resources, probe)
}

func subjectMatches(subjects []Subject, username string, groups []string) bool {
	for _, subject := range subjects {
		switch subject.Kind {
		case string(flowcontrolv1.SubjectKindUser):
			if subject.Name == flowcontrolv1.NameAll || subject.Name == username {
				return true
			}
		case string(flowcontrolv1.SubjectKindGroup):
			if subject.Name == "*" {
				return true
			}
			for _, group := range groups {
				if group == subject.Name {
					return true
				}
			}
		case string(flowcontrolv1.SubjectKindServiceAccount):
			if subject.Name == flowcontrolv1.NameAll {
				if serviceAccountInNamespace(subject.Namespace, username) {
					return true
				}
				continue
			}
			if username == "system:serviceaccount:"+subject.Namespace+":"+subject.Name {
				return true
			}
		}
	}
	return false
}

func resourceMatches(rules []ResourceRule, probe Probe) bool {
	for _, rule := range rules {
		if !contains(probe.Verb, rule.Verbs, flowcontrolv1.VerbAll) {
			continue
		}
		resource := probe.Resource
		if probe.Subresource != "" {
			resource += "/" + probe.Subresource
		}
		if !contains(resource, rule.Resources, flowcontrolv1.ResourceAll) {
			continue
		}
		if !contains(probe.APIGroup, rule.APIGroups, flowcontrolv1.APIGroupAll) {
			continue
		}
		if probe.Namespace == "" {
			if rule.ClusterScope {
				return true
			}
			continue
		}
		if contains(probe.Namespace, rule.Namespaces, flowcontrolv1.NamespaceEvery) {
			return true
		}
	}
	return false
}

func nonResourceMatches(rules []NonResourceRule, verb, path string) bool {
	for _, rule := range rules {
		if !contains(verb, rule.Verbs, flowcontrolv1.VerbAll) {
			continue
		}
		for _, rulePath := range rule.URLs {
			if rulePath == flowcontrolv1.NonResourceAll || rulePath == path {
				return true
			}
			prefix := strings.TrimSuffix(rulePath, "*")
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}

func contains(value string, list []string, wildcard string) bool {
	if len(list) == 1 && list[0] == wildcard {
		return true
	}
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func groupsFor(username string, extra []string) []string {
	seen := map[string]struct{}{}
	var groups []string
	add := func(group string) {
		if group == "" {
			return
		}
		if _, ok := seen[group]; ok {
			return
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	for _, group := range extra {
		add(group)
	}
	if namespace, _, ok := splitServiceAccount(username); ok {
		add("system:serviceaccounts")
		add("system:serviceaccounts:" + namespace)
	}
	switch username {
	case "system:anonymous", "system:unauthenticated":
		add("system:unauthenticated")
	default:
		if username != "" {
			add("system:authenticated")
		}
	}
	return groups
}

func splitServiceAccount(username string) (namespace, name string, ok bool) {
	const prefix = "system:serviceaccount:"
	if !strings.HasPrefix(username, prefix) {
		return "", "", false
	}
	rest := username[len(prefix):]
	namespace, name, ok = strings.Cut(rest, ":")
	if !ok || namespace == "" || name == "" {
		return "", "", false
	}
	return namespace, name, true
}

func serviceAccountInNamespace(namespace, username string) bool {
	const prefix = "system:serviceaccount:"
	if !strings.HasPrefix(username, prefix) {
		return false
	}
	rest := username[len(prefix):]
	if !strings.HasPrefix(rest, namespace) {
		return false
	}
	rest = rest[len(namespace):]
	return strings.HasPrefix(rest, ":")
}
