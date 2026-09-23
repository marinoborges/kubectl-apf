package apf

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
)

const (
	requestQueued    = "queued"
	requestExecuting = "executing"
	unsetCounter     = "-"
	observerAPIPath  = "/debug/api_priority_and_fairness/dump_requests"
)

// BuildSnapshot joins priority levels and flow schemas. levelCSV may be nil
// when live counters were not requested.
func BuildSnapshot(levels *flowcontrolv1.PriorityLevelConfigurationList, flows *flowcontrolv1.FlowSchemaList, levelCSV []byte) (*Snapshot, error) {
	stats := map[string]levelStats{}
	if levelCSV != nil {
		parsed, err := parseLevelStats(levelCSV)
		if err != nil {
			return nil, err
		}
		stats = parsed
	}

	byName := map[string]int{}
	snapshot := &Snapshot{}
	if levels != nil {
		for _, item := range levels.Items {
			level := levelView(item, stats[item.Name], levelCSV != nil)
			byName[level.Name] = len(snapshot.Levels)
			snapshot.Levels = append(snapshot.Levels, level)
		}
	}

	for name, stat := range stats {
		if _, ok := byName[name]; ok {
			continue
		}
		byName[name] = len(snapshot.Levels)
		snapshot.Levels = append(snapshot.Levels, Level{
			Name:      name,
			Type:      "-",
			Shares:    "-",
			Lend:      "-",
			Borrow:    "-",
			Response:  "-",
			Waiting:   stat.waiting,
			Executing: stat.executing,
			Rejected:  stat.rejected,
			TimedOut:  stat.timedout,
			Cancelled: stat.cancelled,
			Queues:    unsetCounter,
		})
	}

	if flows != nil {
		for _, item := range flows.Items {
			flow := flowView(item)
			index, ok := byName[flow.PriorityLevel]
			if !ok {
				snapshot.Unresolved = append(snapshot.Unresolved, flow)
				continue
			}
			snapshot.Levels[index].Flows = append(snapshot.Levels[index].Flows, flow)
		}
	}

	sort.Slice(snapshot.Levels, func(i, j int) bool {
		return snapshot.Levels[i].Name < snapshot.Levels[j].Name
	})
	for i := range snapshot.Levels {
		sortFlows(snapshot.Levels[i].Flows)
	}
	sortFlows(snapshot.Unresolved)
	annotateShares(snapshot)
	return snapshot, nil
}

// ApplyQueueCounts sets each level's busy/total queue count from a dump_queues snapshot.
func (s *Snapshot) ApplyQueueCounts(queues []Queue) {
	type count struct{ total, busy int }
	counts := map[string]*count{}
	for _, queue := range queues {
		item := counts[queue.PriorityLevel]
		if item == nil {
			item = &count{}
			counts[queue.PriorityLevel] = item
		}
		item.total++
		if !queue.Idle() {
			item.busy++
		}
	}
	for i := range s.Levels {
		item := counts[s.Levels[i].Name]
		if item == nil {
			s.Levels[i].Queues = "0/0"
			continue
		}
		s.Levels[i].Queues = fmt.Sprintf("%d/%d", item.busy, item.total)
	}
}

// BuildQueues parses dump_queues CSV. Rows are ordered by priority level, then queue index.
func BuildQueues(data []byte) ([]Queue, error) {
	records, err := parseRecords(data)
	if err != nil {
		return nil, err
	}
	queues := make([]Queue, 0, len(records))
	for _, record := range records {
		queues = append(queues, Queue{
			PriorityLevel: field(record, "PriorityLevelName"),
			Index:         field(record, "Index"),
			Pending:       field(record, "PendingRequests"),
			Executing:     field(record, "ExecutingRequests"),
			Seats:         field(record, "SeatsInUse"),
			NextDispatch:  field(record, "NextDispatchR"),
			InitialSeats:  field(record, "InitialSeatsSum"),
			MaxSeats:      field(record, "MaxSeatsSum"),
			Work:          field(record, "TotalWorkSum"),
		})
	}
	sort.SliceStable(queues, func(i, j int) bool {
		if queues[i].PriorityLevel != queues[j].PriorityLevel {
			return queues[i].PriorityLevel < queues[j].PriorityLevel
		}
		return queueIndex(queues[i].Index) < queueIndex(queues[j].Index)
	})
	return queues, nil
}

// FilterQueues keeps queues whose priority level is one of names.
func FilterQueues(queues []Queue, names []string) []Queue {
	want := make(map[string]struct{}, len(names))
	for _, name := range names {
		want[name] = struct{}{}
	}
	kept := make([]Queue, 0, len(queues))
	for _, queue := range queues {
		if _, ok := want[queue.PriorityLevel]; ok {
			kept = append(kept, queue)
		}
	}
	return kept
}

// ActiveQueues drops queues with nothing waiting, executing, or holding seats.
func ActiveQueues(queues []Queue) []Queue {
	kept := make([]Queue, 0, len(queues))
	for _, queue := range queues {
		if queue.Idle() {
			continue
		}
		kept = append(kept, queue)
	}
	return kept
}

func queueIndex(value string) int {
	index, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return index
}

// BuildRequests parses dump_requests CSV, including the optional detail columns.
func BuildRequests(data []byte) ([]Request, error) {
	records, err := parseRecords(data)
	if err != nil {
		return nil, err
	}

	requests := make([]Request, 0, len(records))
	for _, record := range records {
		start := field(record, "StartTime")
		state := requestExecuting
		if start == "" || strings.HasPrefix(start, "0001-01-01") {
			state = requestQueued
		}
		queue := field(record, "QueueIndex")
		if queue == "" || queue == "-1" {
			queue = unsetCounter
		}
		apiPath := field(record, "APIPath")
		requests = append(requests, Request{
			State:         state,
			PriorityLevel: field(record, "PriorityLevelName"),
			FlowSchema:    field(record, "FlowSchemaName"),
			Distinguisher: field(record, "FlowDistingsher", "FlowDistinguisher"),
			User:          field(record, "UserName"),
			Verb:          field(record, "Verb"),
			Target:        requestTarget(record),
			APIPath:       apiPath,
			Seats:         field(record, "InitialSeats"),
			Queue:         queue,
			Arrive:        parseTimestamp(field(record, "ArriveTime")),
			Start:         parseTimestamp(start),
		})
	}

	sort.SliceStable(requests, func(i, j int) bool {
		if requests[i].State != requests[j].State {
			return requests[i].State == requestQueued
		}
		if requests[i].PriorityLevel != requests[j].PriorityLevel {
			return requests[i].PriorityLevel < requests[j].PriorityLevel
		}
		return requests[i].FlowSchema < requests[j].FlowSchema
	})
	return requests, nil
}

// FilterRequests keeps requests whose priority level is one of names.
func FilterRequests(requests []Request, names []string) []Request {
	want := make(map[string]struct{}, len(names))
	for _, name := range names {
		want[name] = struct{}{}
	}
	kept := make([]Request, 0, len(requests))
	for _, request := range requests {
		if _, ok := want[request.PriorityLevel]; ok {
			kept = append(kept, request)
		}
	}
	return kept
}

// UserLoads groups requests by user. Seats are the initial seats of executing requests.
func UserLoads(requests []Request) []UserLoad {
	index := map[string]int{}
	var loads []UserLoad
	for _, request := range requests {
		user := request.User
		if user == "" {
			user = unsetCounter
		}
		at, ok := index[user]
		if !ok {
			index[user] = len(loads)
			loads = append(loads, UserLoad{User: user})
			at = len(loads) - 1
		}
		if request.State == requestExecuting {
			loads[at].Executing++
			seats, _ := strconv.Atoi(request.Seats)
			loads[at].Seats += seats
			continue
		}
		loads[at].Waiting++
	}
	sort.Slice(loads, func(i, j int) bool {
		if loads[i].Seats != loads[j].Seats {
			return loads[i].Seats > loads[j].Seats
		}
		if loads[i].Executing != loads[j].Executing {
			return loads[i].Executing > loads[j].Executing
		}
		return loads[i].User < loads[j].User
	})
	return loads
}

// WaitLabel is how long a queued request has been waiting, or how long an
// executing request waited before it started.
func WaitLabel(request Request, now time.Time) string {
	if request.Arrive.IsZero() {
		return unsetCounter
	}
	end := now
	if request.State == requestExecuting && !request.Start.IsZero() {
		end = request.Start
	}
	if end.Before(request.Arrive) {
		return "0s"
	}
	wait := end.Sub(request.Arrive).Truncate(time.Second)
	if wait == 0 {
		return "0s"
	}
	return wait.String()
}

func parseTimestamp(value string) time.Time {
	if value == "" || strings.HasPrefix(value, "0001-01-01") {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// WithoutObserver drops the request this command sent to the dump_requests debug endpoint.
func WithoutObserver(requests []Request) []Request {
	kept := make([]Request, 0, len(requests))
	for _, request := range requests {
		if isObserverRequest(request) {
			continue
		}
		kept = append(kept, request)
	}
	return kept
}

func isObserverRequest(request Request) bool {
	path := request.APIPath
	if path == "" {
		path = request.Target
	}
	if before, _, ok := strings.Cut(path, "?"); ok {
		path = before
	}
	return path == observerAPIPath
}

type levelStats struct {
	waiting   string
	executing string
	rejected  string
	timedout  string
	cancelled string
}

func parseLevelStats(data []byte) (map[string]levelStats, error) {
	records, err := parseRecords(data)
	if err != nil {
		return nil, err
	}
	stats := make(map[string]levelStats, len(records))
	for _, record := range records {
		name := field(record, "PriorityLevelName")
		if name == "" {
			continue
		}
		stats[name] = levelStats{
			waiting:   zeroDash(field(record, "WaitingRequests")),
			executing: zeroDash(field(record, "ExecutingRequests")),
			rejected:  zeroDash(field(record, "RejectedRequests")),
			timedout:  zeroDash(field(record, "TimedoutRequests")),
			cancelled: zeroDash(field(record, "CancelledRequests")),
		}
	}
	return stats, nil
}

func levelView(item flowcontrolv1.PriorityLevelConfiguration, stat levelStats, live bool) Level {
	level := Level{
		Name:      item.Name,
		Type:      string(item.Spec.Type),
		Waiting:   unsetCounter,
		Executing: unsetCounter,
		Rejected:  unsetCounter,
		TimedOut:  unsetCounter,
		Cancelled: unsetCounter,
		Queues:    unsetCounter,
	}
	if live && stat.waiting != "" {
		level.Waiting = stat.waiting
		level.Executing = stat.executing
		level.Rejected = stat.rejected
		level.TimedOut = stat.timedout
		level.Cancelled = stat.cancelled
	}

	switch item.Spec.Type {
	case flowcontrolv1.PriorityLevelEnablementExempt:
		shares := int32(0)
		lend := int32(0)
		if item.Spec.Exempt != nil {
			shares = derefOr(item.Spec.Exempt.NominalConcurrencyShares, 0)
			lend = derefOr(item.Spec.Exempt.LendablePercent, 0)
		}
		level.Shares = fmt.Sprintf("%d", shares)
		level.Lend = fmt.Sprintf("%d%%", lend)
		level.Borrow = "unlimited"
		level.Response = unsetCounter
	case flowcontrolv1.PriorityLevelEnablementLimited:
		shares := int32(30)
		lend := int32(0)
		level.Borrow = "unlimited"
		level.Response = "Reject"
		if item.Spec.Limited != nil {
			shares = derefOr(item.Spec.Limited.NominalConcurrencyShares, 30)
			lend = derefOr(item.Spec.Limited.LendablePercent, 0)
			if item.Spec.Limited.BorrowingLimitPercent != nil {
				level.Borrow = fmt.Sprintf("%d%%", *item.Spec.Limited.BorrowingLimitPercent)
			}
			level.Response = responseView(item.Spec.Limited.LimitResponse)
		}
		level.Shares = fmt.Sprintf("%d", shares)
		level.Lend = fmt.Sprintf("%d%%", lend)
	default:
		level.Shares = unsetCounter
		level.Lend = unsetCounter
		level.Borrow = unsetCounter
		level.Response = unsetCounter
	}
	return level
}

func responseView(response flowcontrolv1.LimitResponse) string {
	switch response.Type {
	case flowcontrolv1.LimitResponseTypeReject:
		return "Reject"
	case flowcontrolv1.LimitResponseTypeQueue:
		queues, hand, length := int32(64), int32(8), int32(50)
		if response.Queuing != nil {
			if response.Queuing.Queues > 0 {
				queues = response.Queuing.Queues
			}
			if response.Queuing.HandSize > 0 {
				hand = response.Queuing.HandSize
			}
			if response.Queuing.QueueLengthLimit > 0 {
				length = response.Queuing.QueueLengthLimit
			}
		}
		return fmt.Sprintf("Queue %d/%d/%d", queues, hand, length)
	default:
		if response.Type == "" {
			return unsetCounter
		}
		return string(response.Type)
	}
}

func flowView(item flowcontrolv1.FlowSchema) Flow {
	distinguisher := "disabled"
	if item.Spec.DistinguisherMethod != nil && item.Spec.DistinguisherMethod.Type != "" {
		distinguisher = string(item.Spec.DistinguisherMethod.Type)
	}
	return Flow{
		Name:          item.Name,
		PriorityLevel: item.Spec.PriorityLevelConfiguration.Name,
		Precedence:    item.Spec.MatchingPrecedence,
		Distinguisher: distinguisher,
		Subjects:      subjectList(item),
		Dangling:      dangling(item),
		Rules:         ruleViews(item),
	}
}

func ruleViews(item flowcontrolv1.FlowSchema) []Rule {
	rules := make([]Rule, 0, len(item.Spec.Rules))
	for _, policy := range item.Spec.Rules {
		rule := Rule{}
		for _, subject := range policy.Subjects {
			rule.Subjects = append(rule.Subjects, subjectView(subject))
		}
		for _, resource := range policy.ResourceRules {
			rule.Resources = append(rule.Resources, ResourceRule{
				Verbs:        resource.Verbs,
				APIGroups:    resource.APIGroups,
				Resources:    resource.Resources,
				Namespaces:   resource.Namespaces,
				ClusterScope: resource.ClusterScope,
			})
		}
		for _, nonResource := range policy.NonResourceRules {
			rule.NonResources = append(rule.NonResources, NonResourceRule{
				Verbs: nonResource.Verbs,
				URLs:  nonResource.NonResourceURLs,
			})
		}
		rules = append(rules, rule)
	}
	return rules
}

func subjectView(subject flowcontrolv1.Subject) Subject {
	switch subject.Kind {
	case flowcontrolv1.SubjectKindServiceAccount:
		if subject.ServiceAccount == nil {
			return Subject{Kind: string(subject.Kind)}
		}
		return Subject{Kind: string(subject.Kind), Namespace: subject.ServiceAccount.Namespace, Name: subject.ServiceAccount.Name}
	case flowcontrolv1.SubjectKindUser:
		if subject.User == nil {
			return Subject{Kind: string(subject.Kind)}
		}
		return Subject{Kind: string(subject.Kind), Name: subject.User.Name}
	case flowcontrolv1.SubjectKindGroup:
		if subject.Group == nil {
			return Subject{Kind: string(subject.Kind)}
		}
		return Subject{Kind: string(subject.Kind), Name: subject.Group.Name}
	default:
		return Subject{Kind: string(subject.Kind)}
	}
}

func annotateShares(snapshot *Snapshot) {
	sum := 0
	values := make([]int, len(snapshot.Levels))
	known := make([]bool, len(snapshot.Levels))
	for i, level := range snapshot.Levels {
		value, err := strconv.Atoi(level.Shares)
		if err != nil {
			continue
		}
		values[i] = value
		known[i] = true
		sum += value
	}
	if sum == 0 {
		return
	}
	for i := range snapshot.Levels {
		if !known[i] {
			continue
		}
		percent := (values[i]*100 + sum/2) / sum
		snapshot.Levels[i].Shares = fmt.Sprintf("%d (%d%%)", values[i], percent)
	}
}

func subjectList(item flowcontrolv1.FlowSchema) string {
	seen := map[string]struct{}{}
	var parts []string
	for _, rule := range item.Spec.Rules {
		for _, subject := range rule.Subjects {
			label := formatSubject(subject)
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			parts = append(parts, label)
		}
	}
	if len(parts) == 0 {
		return unsetCounter
	}
	if len(parts) > 4 {
		return strings.Join(parts[:4], ", ") + fmt.Sprintf(" +%d", len(parts)-4)
	}
	return strings.Join(parts, ", ")
}

func formatSubject(subject flowcontrolv1.Subject) string {
	switch subject.Kind {
	case flowcontrolv1.SubjectKindServiceAccount:
		if subject.ServiceAccount == nil {
			return "sa:?"
		}
		return "sa:" + subject.ServiceAccount.Namespace + "/" + subject.ServiceAccount.Name
	case flowcontrolv1.SubjectKindUser:
		if subject.User == nil {
			return "user:?"
		}
		return "user:" + subject.User.Name
	case flowcontrolv1.SubjectKindGroup:
		if subject.Group == nil {
			return "group:?"
		}
		return "group:" + subject.Group.Name
	default:
		if subject.Kind == "" {
			return "unknown"
		}
		return string(subject.Kind)
	}
}

func dangling(item flowcontrolv1.FlowSchema) bool {
	for _, condition := range item.Status.Conditions {
		if condition.Type == flowcontrolv1.FlowSchemaConditionDangling && condition.Status == flowcontrolv1.ConditionTrue {
			return true
		}
	}
	return false
}

func requestTarget(record map[string]string) string {
	resource := field(record, "Resource")
	if resource == "" {
		return field(record, "APIPath")
	}
	if sub := field(record, "SubResource"); sub != "" {
		resource += "/" + sub
	}
	namespace := field(record, "Namespace")
	name := field(record, "Name")
	if namespace == "" && name == "" {
		return resource
	}
	return resource + " " + namespace + "/" + name
}

func sortFlows(flows []Flow) {
	sort.Slice(flows, func(i, j int) bool {
		if flows[i].Precedence != flows[j].Precedence {
			return flows[i].Precedence < flows[j].Precedence
		}
		return flows[i].Name < flows[j].Name
	})
}

func derefOr(value *int32, fallback int32) int32 {
	if value == nil {
		return fallback
	}
	return *value
}

func zeroDash(value string) string {
	if value == "" {
		return unsetCounter
	}
	return value
}
