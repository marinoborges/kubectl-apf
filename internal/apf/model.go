// Package apf loads API Priority and Fairness objects and live debug counters.
package apf

import (
	"fmt"
	"time"
)

// Snapshot is the configuration of every priority level, with the flow schemas
// that select into it.
type Snapshot struct {
	Levels     []Level
	Unresolved []Flow
	Warning    string
}

// Level is one PriorityLevelConfiguration plus the flow schemas that reference it.
type Level struct {
	Name      string
	Type      string
	Shares    string
	Lend      string
	Borrow    string
	Response  string
	Waiting   string
	Executing string
	Rejected  string
	TimedOut  string
	Cancelled string
	Queues    string
	Flows     []Flow
}

// Flow is one FlowSchema.
type Flow struct {
	Name          string
	PriorityLevel string
	Precedence    int32
	Distinguisher string
	Subjects      string
	Dangling      bool
	Rules         []Rule
}

// Rule is one subject-and-policy rule inside a flow schema.
type Rule struct {
	Subjects     []Subject
	Resources    []ResourceRule
	NonResources []NonResourceRule
}

// Subject is one user, group, or ServiceAccount match.
type Subject struct {
	Kind      string
	Name      string
	Namespace string
}

// ResourceRule matches resource requests.
type ResourceRule struct {
	Verbs        []string
	APIGroups    []string
	Resources    []string
	Namespaces   []string
	ClusterScope bool
}

// NonResourceRule matches non-resource URLs.
type NonResourceRule struct {
	Verbs []string
	URLs  []string
}

// Queue is one shuffle-shard queue inside a priority level.
type Queue struct {
	PriorityLevel string
	Index         string
	Pending       string
	Executing     string
	Seats         string
	NextDispatch  string
	InitialSeats  string
	MaxSeats      string
	Work          string
}

// Idle reports a queue with nothing waiting, executing, or holding seats.
func (q Queue) Idle() bool {
	return zero(q.Pending) && zero(q.Executing) && zero(q.Seats)
}

func zero(value string) bool {
	return value == "" || value == "0"
}

// Request is one in-flight API request from the debug endpoint.
type Request struct {
	State         string
	PriorityLevel string
	FlowSchema    string
	Distinguisher string
	User          string
	Verb          string
	Target        string
	APIPath       string
	Seats         string
	Queue         string
	Arrive        time.Time
	Start         time.Time
}

// UserLoad is one user's waiting and executing requests.
type UserLoad struct {
	User      string
	Waiting   int
	Executing int
	Seats     int
}

// RequireLevels reports the first name that is not a priority level in the snapshot.
func (s *Snapshot) RequireLevels(names []string) error {
	have := make(map[string]struct{}, len(s.Levels))
	for _, level := range s.Levels {
		have[level.Name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := have[name]; !ok {
			return fmt.Errorf("priority level %q not found", name)
		}
	}
	return nil
}

// Detail returns one priority level, or every level when name is "all".
func (s *Snapshot) Detail(name string) (*Snapshot, error) {
	if name == "all" {
		copy := *s
		return &copy, nil
	}
	for _, level := range s.Levels {
		if level.Name == name {
			return &Snapshot{Levels: []Level{level}, Warning: s.Warning}, nil
		}
	}
	return nil, fmt.Errorf("priority level %q not found", name)
}

// FindFlow returns the named flow schema.
func (s *Snapshot) FindFlow(name string) (Flow, bool) {
	for _, flow := range s.Flows() {
		if flow.Name == name {
			return flow, true
		}
	}
	return Flow{}, false
}

// Flows returns every flow schema in matching order. Lower precedence wins.
func (s *Snapshot) Flows() []Flow {
	out := make([]Flow, 0, len(s.Unresolved))
	for _, level := range s.Levels {
		out = append(out, level.Flows...)
	}
	out = append(out, s.Unresolved...)
	sortFlows(out)
	return out
}
