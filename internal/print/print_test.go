package print

import (
	"bytes"
	"strings"
	"testing"

	"github.com/marinoborges/kubectl-apf/internal/apf"
)

func TestDetailOneOmitsRepeatedLevelName(t *testing.T) {
	var buf bytes.Buffer
	err := Detail(&buf, &apf.Snapshot{
		Warning: "live counters unavailable: forbidden",
		Levels: []apf.Level{{
			Name: "workload-low", Type: "Limited", Shares: "100", Lend: "50%", Borrow: "0%",
			Response: "Queue 128/6/50", Waiting: "-", Executing: "-", Rejected: "-",
			Flows: []apf.Flow{{
				Name: "replicaset-controller", PriorityLevel: "workload-low", Precedence: 1000,
				Distinguisher: "ByUser", Subjects: "sa:kube-system/replicaset-controller",
			}},
		}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"warning: live counters unavailable", "PriorityLevelConfigurations\n", "workload-low", "\nFlowSchemas\n", "Queue 128/6/50", "1000", "sa:kube-system/replicaset-controller"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\nworkload-low\n") {
		t.Fatalf("priority level name repeated above FlowSchemas:\n%s", out)
	}
}

func TestDetailAllUsesOneTitle(t *testing.T) {
	var buf bytes.Buffer
	err := Detail(&buf, &apf.Snapshot{
		Levels: []apf.Level{
			{Name: "exempt", Type: "Exempt", Shares: "0", Lend: "0%", Borrow: "unlimited", Response: "-", Waiting: "0", Executing: "0", Rejected: "0"},
			{
				Name: "workload-low", Type: "Limited", Shares: "100", Lend: "50%", Borrow: "0%",
				Response: "Queue 128/6/50", Waiting: "0", Executing: "0", Rejected: "0",
				Flows: []apf.Flow{{Name: "replicaset-controller", Precedence: 1000, Distinguisher: "ByUser", Subjects: "sa:kube-system/replicaset-controller"}},
			},
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "PriorityLevelConfigurations\n") {
		t.Fatalf("first line = %q", out)
	}
	title := strings.Index(out, "PriorityLevelConfigurations X FlowSchemas")
	flows := strings.Index(out, "\nexempt\nFlowSchemas\n")
	if strings.Count(out, "PriorityLevelConfigurations X FlowSchemas") != 1 || title < 0 || flows < title {
		t.Fatalf("title placement:\n%s", out)
	}
	if strings.Contains(out, "PriorityLevelConfigurations X FlowSchemas\n\n") {
		t.Fatalf("blank line after title:\n%s", out)
	}
	for _, want := range []string{"\nexempt\nFlowSchemas\n", "\nworkload-low\nFlowSchemas\n", "replicaset-controller"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestLevelsOmitsFlowSchemas(t *testing.T) {
	var buf bytes.Buffer
	err := Levels(&buf, &apf.Snapshot{
		Levels: []apf.Level{{
			Name: "exempt", Type: "Exempt", Shares: "0", Lend: "0%", Borrow: "unlimited",
			Response: "-", Waiting: "0", Executing: "1", Rejected: "0",
			Flows: []apf.Flow{{Name: "exempt", Precedence: 1, Subjects: "group:system:masters"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "PriorityLevelConfigurations") || !strings.Contains(out, "exempt") {
		t.Fatalf("output = %s", out)
	}
	if strings.Contains(out, "FlowSchemas") || strings.Contains(out, "group:system:masters") {
		t.Fatalf("flow schemas leaked into priority levels:\n%s", out)
	}
}

func TestQueuesPrintsOccupancy(t *testing.T) {
	var buf bytes.Buffer
	err := Queues(&buf, []apf.Queue{{
		PriorityLevel: "workload-low", Index: "14", Pending: "27", Executing: "0",
		Seats: "0", NextDispatch: "77.64342019ss", InitialSeats: "270", MaxSeats: "270", Work: "0.81000000ss",
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"LEVEL", "INDEX", "PENDING", "EXECUTING", "SEATS", "workload-low", "14", "27"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, buf.String())
		}
	}
	if strings.Contains(buf.String(), "NEXT") || strings.Contains(buf.String(), "77.64342019ss") {
		t.Fatalf("dropped columns still printed:\n%s", buf.String())
	}
}

func TestRequestsEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Requests(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No requests are waiting or executing.") {
		t.Fatalf("output = %q", buf.String())
	}
}
