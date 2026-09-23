package apf

import (
	"testing"

	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildSnapshotJoinsFlowsAndLiveCounters(t *testing.T) {
	shares := int32(100)
	lend := int32(50)
	borrow := int32(0)
	levels := &flowcontrolv1.PriorityLevelConfigurationList{
		Items: []flowcontrolv1.PriorityLevelConfiguration{{
			ObjectMeta: metav1.ObjectMeta{Name: "workload-low"},
			Spec: flowcontrolv1.PriorityLevelConfigurationSpec{
				Type: flowcontrolv1.PriorityLevelEnablementLimited,
				Limited: &flowcontrolv1.LimitedPriorityLevelConfiguration{
					NominalConcurrencyShares: &shares,
					LendablePercent:          &lend,
					BorrowingLimitPercent:    &borrow,
					LimitResponse: flowcontrolv1.LimitResponse{
						Type:    flowcontrolv1.LimitResponseTypeQueue,
						Queuing: &flowcontrolv1.QueuingConfiguration{Queues: 128, HandSize: 6, QueueLengthLimit: 50},
					},
				},
			},
		}},
	}
	flows := &flowcontrolv1.FlowSchemaList{
		Items: []flowcontrolv1.FlowSchema{
			flowSchema("service-accounts", "workload-low", 9000, flowcontrolv1.FlowDistinguisherMethodByUserType, groupSubject("system:serviceaccounts")),
			flowSchema("replicaset-controller", "workload-low", 1000, flowcontrolv1.FlowDistinguisherMethodByUserType, serviceAccountSubject("kube-system", "replicaset-controller")),
			flowSchema("missing-pl", "gone", 50, "", groupSubject("system:authenticated")),
		},
	}
	csv := []byte(`PriorityLevelName, ActiveQueues, WaitingRequests, ExecutingRequests, RejectedRequests
workload-low, 1, 12, 4, 30
orphan, 0, 1, 0, 2
`)

	snapshot, err := BuildSnapshot(levels, flows, csv)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Levels[0].Name != "orphan" || snapshot.Levels[1].Name != "workload-low" {
		t.Fatalf("levels = %+v", names(snapshot))
	}

	level := snapshot.Levels[1]
	if level.Shares != "100 (100%)" || level.Lend != "50%" || level.Borrow != "0%" || level.Response != "Queue 128/6/50" {
		t.Fatalf("level config = %+v", level)
	}
	if level.Waiting != "12" || level.Executing != "4" || level.Rejected != "30" || level.TimedOut != "-" || level.Cancelled != "-" {
		t.Fatalf("live counters = %+v", level)
	}
	if len(level.Flows) != 2 || level.Flows[0].Name != "replicaset-controller" || level.Flows[1].Name != "service-accounts" {
		t.Fatalf("flows = %+v", level.Flows)
	}
	if level.Flows[0].Subjects != "sa:kube-system/replicaset-controller" {
		t.Fatalf("subjects = %s", level.Flows[0].Subjects)
	}
	if len(snapshot.Unresolved) != 1 || snapshot.Unresolved[0].Name != "missing-pl" || !snapshot.Unresolved[0].Dangling {
		t.Fatalf("unresolved = %+v", snapshot.Unresolved)
	}
	if got := snapshot.Flows()[0].Name; got != "missing-pl" {
		t.Fatalf("first matching flow = %s", got)
	}
}

func TestDetailSelectsOneLevelOrAll(t *testing.T) {
	snapshot := &Snapshot{
		Warning:    "live counters unavailable",
		Unresolved: []Flow{{Name: "missing-pl"}},
		Levels: []Level{
			{Name: "exempt", Flows: []Flow{{Name: "exempt"}}},
			{Name: "workload-low", Flows: []Flow{{Name: "replicaset-controller"}}},
		},
	}

	one, err := snapshot.Detail("workload-low")
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Levels) != 1 || one.Levels[0].Name != "workload-low" || len(one.Unresolved) != 0 {
		t.Fatalf("detail = %+v", one)
	}
	if one.Warning != snapshot.Warning {
		t.Fatalf("warning = %q", one.Warning)
	}

	all, err := snapshot.Detail("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Levels) != 2 || len(all.Unresolved) != 1 {
		t.Fatalf("all = %+v", all)
	}
	if _, err := snapshot.Detail("missing"); err == nil {
		t.Fatal("expected missing priority level to fail")
	}
}

func TestBuildSnapshotUsesDocumentedDefaults(t *testing.T) {
	levels := &flowcontrolv1.PriorityLevelConfigurationList{
		Items: []flowcontrolv1.PriorityLevelConfiguration{{
			ObjectMeta: metav1.ObjectMeta{Name: "exempt"},
			Spec:       flowcontrolv1.PriorityLevelConfigurationSpec{Type: flowcontrolv1.PriorityLevelEnablementExempt},
		}, {
			ObjectMeta: metav1.ObjectMeta{Name: "limited"},
			Spec: flowcontrolv1.PriorityLevelConfigurationSpec{
				Type:    flowcontrolv1.PriorityLevelEnablementLimited,
				Limited: &flowcontrolv1.LimitedPriorityLevelConfiguration{},
			},
		}},
	}

	snapshot, err := BuildSnapshot(levels, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	exempt, limited := snapshot.Levels[0], snapshot.Levels[1]
	if exempt.Shares != "0 (0%)" || exempt.Borrow != "unlimited" || exempt.Waiting != "-" {
		t.Fatalf("exempt = %+v", exempt)
	}
	if limited.Shares != "30 (100%)" || limited.Lend != "0%" || limited.Borrow != "unlimited" || limited.Response != "-" {
		t.Fatalf("limited = %+v", limited)
	}
}

func TestBuildRequestsClassifiesQueueAndDetailColumns(t *testing.T) {
	data := []byte(`PriorityLevelName, FlowSchemaName, QueueIndex, FlowDistingsher, ArriveTime, InitialSeats, StartTime, UserName, Verb, APIPath, Namespace, Name, Resource
exempt, exempt, 3, , 2023-07-15T04:51:25.596404345Z, 1, 2023-07-15T04:51:25.596404345Z, system:apiserver, list, /api/v1/namespaces/kube-system/configmaps, kube-system, , configmaps
workload-low, service-accounts, 14, system:serviceaccount:default:loadtest, 2023-07-18T00:12:51.386556253Z, 10, 0001-01-01T00:00:00Z, system:serviceaccount:default:loadtest, list, /api/v1/namespaces/default/configmaps, default, , configmaps
`)

	requests, err := BuildRequests(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("len = %d", len(requests))
	}
	queued := requests[0]
	if queued.State != requestQueued || queued.Queue != "14" || queued.Distinguisher != "system:serviceaccount:default:loadtest" {
		t.Fatalf("queued = %+v", queued)
	}
	if queued.Target != "configmaps default/" || queued.Seats != "10" || queued.User != "system:serviceaccount:default:loadtest" {
		t.Fatalf("queued target = %+v", queued)
	}
	if requests[1].State != requestExecuting || requests[1].Queue != "3" || requests[1].Target != "configmaps kube-system/" {
		t.Fatalf("executing = %+v", requests[1])
	}
}

func TestBuildQueuesKeepsBusyQueuesFirstByLevel(t *testing.T) {
	data := []byte(`PriorityLevelName, Index, PendingRequests, ExecutingRequests, SeatsInUse, NextDispatchR, InitialSeatsSum, MaxSeatsSum, TotalWorkSum
workload-low, 14, 27, 0, 0, 77.64342019ss, 270, 270, 0.81000000ss
leader-election, 1, 0, 0, 0, 0.00000000ss, 0, 0, 0.00000000ss
leader-election, 0, 0, 2, 4, 5088.87053833ss, 0, 0, 0.00000000ss
`)
	queues, err := BuildQueues(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(queues) != 3 || queues[0].PriorityLevel != "leader-election" || queues[0].Index != "0" || queues[0].Executing != "2" {
		t.Fatalf("queues = %+v", queues)
	}
	active := ActiveQueues(queues)
	if len(active) != 2 || active[1].Index != "14" || active[1].Pending != "27" {
		t.Fatalf("active = %+v", active)
	}
	filtered := FilterQueues(active, []string{"workload-low"})
	if len(filtered) != 1 || filtered[0].Work != "0.81000000ss" {
		t.Fatalf("filtered = %+v", filtered)
	}
}

func TestFilterRequestsKeepsNamedLevels(t *testing.T) {
	requests := []Request{
		{PriorityLevel: "workload-low", FlowSchema: "service-accounts"},
		{PriorityLevel: "exempt", FlowSchema: "exempt"},
		{PriorityLevel: "workload-high", FlowSchema: "kube-scheduler"},
	}
	kept := FilterRequests(requests, []string{"workload-high", "workload-low"})
	if len(kept) != 2 || kept[0].PriorityLevel != "workload-low" || kept[1].PriorityLevel != "workload-high" {
		t.Fatalf("kept = %+v", kept)
	}

	snapshot := &Snapshot{Levels: []Level{{Name: "workload-low"}, {Name: "exempt"}}}
	if err := snapshot.RequireLevels([]string{"workload-low"}); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.RequireLevels([]string{"missing"}); err == nil {
		t.Fatal("expected missing priority level to fail")
	}
}

func TestWithoutObserverDropsDebugDump(t *testing.T) {
	data := []byte(`PriorityLevelName, FlowSchemaName, QueueIndex, StartTime, UserName, Verb, APIPath, Resource
global-default, global-default, 58, 2023-07-15T04:51:25.596404345Z, kubernetes-admin, get, /debug/api_priority_and_fairness/dump_requests?includeRequestDetails=1,
workload-low, service-accounts, 14, 0001-01-01T00:00:00Z, system:serviceaccount:default:loadtest, list, /api/v1/namespaces/default/configmaps, configmaps
global-default, global-default, 1, 2023-07-15T04:51:25.596404345Z, other, get, /debug/api_priority_and_fairness/dump_priority_levels,
`)
	requests, err := BuildRequests(data)
	if err != nil {
		t.Fatal(err)
	}
	kept := WithoutObserver(requests)
	if len(kept) != 2 {
		t.Fatalf("kept %d requests: %+v", len(kept), kept)
	}
	for _, request := range kept {
		if isObserverRequest(request) {
			t.Fatalf("observer request kept: %+v", request)
		}
	}
	if kept[0].APIPath != "/api/v1/namespaces/default/configmaps" {
		t.Fatalf("queued request = %+v", kept[0])
	}
}

func flowSchema(name, level string, precedence int32, distinguisher flowcontrolv1.FlowDistinguisherMethodType, subject flowcontrolv1.Subject) flowcontrolv1.FlowSchema {
	schema := flowcontrolv1.FlowSchema{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: flowcontrolv1.FlowSchemaSpec{
			PriorityLevelConfiguration: flowcontrolv1.PriorityLevelConfigurationReference{Name: level},
			MatchingPrecedence:         precedence,
			Rules:                      []flowcontrolv1.PolicyRulesWithSubjects{{Subjects: []flowcontrolv1.Subject{subject}}},
		},
		Status: flowcontrolv1.FlowSchemaStatus{
			Conditions: []flowcontrolv1.FlowSchemaCondition{{
				Type:   flowcontrolv1.FlowSchemaConditionDangling,
				Status: flowcontrolv1.ConditionFalse,
			}},
		},
	}
	if distinguisher != "" {
		schema.Spec.DistinguisherMethod = &flowcontrolv1.FlowDistinguisherMethod{Type: distinguisher}
	}
	if level == "gone" {
		schema.Status.Conditions[0].Status = flowcontrolv1.ConditionTrue
	}
	return schema
}

func groupSubject(name string) flowcontrolv1.Subject {
	return flowcontrolv1.Subject{Kind: flowcontrolv1.SubjectKindGroup, Group: &flowcontrolv1.GroupSubject{Name: name}}
}

func serviceAccountSubject(namespace, name string) flowcontrolv1.Subject {
	return flowcontrolv1.Subject{
		Kind:           flowcontrolv1.SubjectKindServiceAccount,
		ServiceAccount: &flowcontrolv1.ServiceAccountSubject{Namespace: namespace, Name: name},
	}
}

func names(snapshot *Snapshot) []string {
	out := make([]string, len(snapshot.Levels))
	for i, level := range snapshot.Levels {
		out[i] = level.Name
	}
	return out
}
