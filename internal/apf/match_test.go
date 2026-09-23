package apf

import (
	"testing"
	"time"

	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMatchSelectsTheLowerPrecedenceSchema(t *testing.T) {
	controller := flowcontrolv1.FlowSchema{
		ObjectMeta: metav1.ObjectMeta{Name: "replicaset-controller"},
		Spec: flowcontrolv1.FlowSchemaSpec{
			MatchingPrecedence:         1000,
			PriorityLevelConfiguration: flowcontrolv1.PriorityLevelConfigurationReference{Name: "workload-low"},
			Rules: []flowcontrolv1.PolicyRulesWithSubjects{{
				Subjects: []flowcontrolv1.Subject{serviceAccountSubject("kube-system", "replicaset-controller")},
				ResourceRules: []flowcontrolv1.ResourcePolicyRule{{
					Verbs:      []string{"list"},
					APIGroups:  []string{""},
					Resources:  []string{"pods"},
					Namespaces: []string{"kube-system"},
				}},
			}},
		},
	}
	catchAll := flowcontrolv1.FlowSchema{
		ObjectMeta: metav1.ObjectMeta{Name: "service-accounts"},
		Spec: flowcontrolv1.FlowSchemaSpec{
			MatchingPrecedence:         9000,
			PriorityLevelConfiguration: flowcontrolv1.PriorityLevelConfigurationReference{Name: "workload-low"},
			Rules: []flowcontrolv1.PolicyRulesWithSubjects{{
				Subjects: []flowcontrolv1.Subject{groupSubject("system:serviceaccounts")},
				ResourceRules: []flowcontrolv1.ResourcePolicyRule{{
					Verbs:      []string{"*"},
					APIGroups:  []string{"*"},
					Resources:  []string{"*"},
					Namespaces: []string{"*"},
				}},
			}},
		},
	}
	snapshot := &Snapshot{Levels: []Level{{Flows: []Flow{flowView(controller), flowView(catchAll)}}}}
	matched := snapshot.Match(Probe{
		User:      "system:serviceaccount:kube-system:replicaset-controller",
		Verb:      "list",
		Resource:  "pods",
		Namespace: "kube-system",
	})
	if len(matched) != 2 || matched[0].Name != "replicaset-controller" || matched[1].Name != "service-accounts" {
		t.Fatalf("matched = %+v", namesOf(matched))
	}

	otherNamespace := snapshot.Match(Probe{
		User:      "system:serviceaccount:kube-system:replicaset-controller",
		Verb:      "list",
		Resource:  "pods",
		Namespace: "default",
	})
	if len(otherNamespace) != 1 || otherNamespace[0].Name != "service-accounts" {
		t.Fatalf("other namespace = %+v", namesOf(otherNamespace))
	}
}

func TestWaitLabelAndUserLoadsAndQueueCounts(t *testing.T) {
	arrive := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start := arrive.Add(90 * time.Second)
	now := arrive.Add(3 * time.Minute)
	queued := Request{State: requestQueued, Arrive: arrive}
	if got := WaitLabel(queued, now); got != "3m0s" {
		t.Fatalf("queued wait = %s", got)
	}
	executing := Request{State: requestExecuting, Arrive: arrive, Start: start}
	if got := WaitLabel(executing, now); got != "1m30s" {
		t.Fatalf("executing wait = %s", got)
	}

	loads := UserLoads([]Request{
		{User: "one", State: requestExecuting, Seats: "4"},
		{User: "two", State: requestQueued},
		{User: "one", State: requestExecuting, Seats: "1"},
	})
	if len(loads) != 2 || loads[0].User != "one" || loads[0].Seats != 5 || loads[0].Executing != 2 || loads[1].Waiting != 1 {
		t.Fatalf("loads = %+v", loads)
	}

	snapshot := &Snapshot{Levels: []Level{{Name: "workload-low", Queues: "-"}}}
	snapshot.ApplyQueueCounts([]Queue{
		{PriorityLevel: "workload-low", Pending: "1"},
		{PriorityLevel: "workload-low", Pending: "0", Executing: "0", Seats: "0"},
	})
	if snapshot.Levels[0].Queues != "1/2" {
		t.Fatalf("queues = %s", snapshot.Levels[0].Queues)
	}
}

func namesOf(flows []Flow) []string {
	out := make([]string, len(flows))
	for i, flow := range flows {
		out[i] = flow.Name
	}
	return out
}
