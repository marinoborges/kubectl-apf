// Package print renders API Priority and Fairness snapshots as aligned text.
package print

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/marinoborges/kubectl-apf/internal/apf"
)

// Detail prints priority levels and the flow schemas under each one.
// all uses one combined title. A single level omits a repeated name above FlowSchemas.
// RESPONSE Queue values are queues/handSize/queueLengthLimit.
func Detail(w io.Writer, snapshot *apf.Snapshot, all bool) error {
	if snapshot.Warning != "" {
		if _, err := fmt.Fprintf(w, "warning: %s\n\n", snapshot.Warning); err != nil {
			return err
		}
	}
	if len(snapshot.Levels) == 0 && len(snapshot.Unresolved) == 0 {
		_, err := fmt.Fprintln(w, "No priority levels or flow schemas found.")
		return err
	}
	if _, err := fmt.Fprintln(w, "PriorityLevelConfigurations"); err != nil {
		return err
	}
	if len(snapshot.Levels) > 0 {
		if err := writeLevelTable(w, snapshot.Levels); err != nil {
			return err
		}
	}
	if all {
		if _, err := fmt.Fprintln(w, "\nPriorityLevelConfigurations X FlowSchemas"); err != nil {
			return err
		}
	}

	for i, level := range snapshot.Levels {
		header := "\nFlowSchemas\n"
		if all {
			header = fmt.Sprintf("\n%s\nFlowSchemas\n", level.Name)
			if i == 0 {
				header = fmt.Sprintf("%s\nFlowSchemas\n", level.Name)
			}
		}
		if _, err := fmt.Fprint(w, header); err != nil {
			return err
		}
		if len(level.Flows) == 0 {
			if _, err := fmt.Fprintln(w, "  (no flow schemas)"); err != nil {
				return err
			}
			continue
		}
		if err := writeFlows(w, level.Flows, false); err != nil {
			return err
		}
	}
	if len(snapshot.Unresolved) > 0 {
		if _, err := fmt.Fprintln(w, "\nunresolved\nFlowSchemas"); err != nil {
			return err
		}
		return writeFlows(w, snapshot.Unresolved, true)
	}
	return nil
}

// Levels prints priority level configurations without their flow schemas.
func Levels(w io.Writer, snapshot *apf.Snapshot) error {
	if snapshot.Warning != "" {
		if _, err := fmt.Fprintf(w, "warning: %s\n\n", snapshot.Warning); err != nil {
			return err
		}
	}
	if len(snapshot.Levels) == 0 {
		_, err := fmt.Fprintln(w, "No priority levels found.")
		return err
	}
	return writeLevels(w, snapshot.Levels)
}

func writeLevels(w io.Writer, levels []apf.Level) error {
	if _, err := fmt.Fprintln(w, "PriorityLevelConfigurations"); err != nil {
		return err
	}
	return writeLevelTable(w, levels)
}

func writeLevelTable(w io.Writer, levels []apf.Level) error {
	rows := [][]string{{"NAME", "TYPE", "SHARES", "LEND", "BORROW", "RESPONSE", "WAITING", "EXECUTING", "REJECTED", "TIMEDOUT", "CANCELLED", "QUEUES", "FLOWS"}}
	for _, level := range levels {
		rows = append(rows, []string{
			level.Name,
			level.Type,
			level.Shares,
			level.Lend,
			level.Borrow,
			level.Response,
			level.Waiting,
			level.Executing,
			level.Rejected,
			level.TimedOut,
			level.Cancelled,
			dash(level.Queues),
			strconv.Itoa(len(level.Flows)),
		})
	}
	return writeTable(w, rows)
}

// Flows prints flow schemas in matching order. Lower precedence is chosen first.
func Flows(w io.Writer, flows []apf.Flow) error {
	if len(flows) == 0 {
		_, err := fmt.Fprintln(w, "No flow schemas found.")
		return err
	}
	return writeFlows(w, flows, true)
}

// FlowDetail prints one flow schema, including subjects and rules.
func FlowDetail(w io.Writer, flow apf.Flow) error {
	rows := [][]string{
		{"FLOW SCHEMA", flow.Name},
		{"PRIORITY LEVEL", dash(flow.PriorityLevel)},
		{"PRECEDENCE", strconv.FormatInt(int64(flow.Precedence), 10)},
		{"DISTINGUISHER", flow.Distinguisher},
		{"DANGLING", yesNo(flow.Dangling)},
	}
	if err := writeTable(w, rows); err != nil {
		return err
	}
	if len(flow.Rules) == 0 {
		_, err := fmt.Fprintln(w, "\n(no rules)")
		return err
	}
	for i, rule := range flow.Rules {
		if _, err := fmt.Fprintf(w, "\nRULE %d\n", i+1); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "SUBJECTS"); err != nil {
			return err
		}
		if len(rule.Subjects) == 0 {
			if _, err := fmt.Fprintln(w, "  -"); err != nil {
				return err
			}
		}
		for _, subject := range rule.Subjects {
			if _, err := fmt.Fprintf(w, "  %s\n", subjectLabel(subject)); err != nil {
				return err
			}
		}
		if err := writeResourceRules(w, rule.Resources); err != nil {
			return err
		}
		if err := writeNonResourceRules(w, rule.NonResources); err != nil {
			return err
		}
	}
	return nil
}

// Match prints the flow schema the API server would select, then the other matches.
func Match(w io.Writer, flows []apf.Flow) error {
	if len(flows) == 0 {
		_, err := fmt.Fprintln(w, "No flow schema matches.")
		return err
	}
	rows := [][]string{{"SELECTED", "PREC", "FLOW SCHEMA", "PRIORITY LEVEL"}}
	for i, flow := range flows {
		selected := ""
		if i == 0 {
			selected = "yes"
		}
		rows = append(rows, []string{
			selected,
			strconv.FormatInt(int64(flow.Precedence), 10),
			flow.Name,
			dash(flow.PriorityLevel),
		})
	}
	if err := writeTable(w, rows); err != nil {
		return err
	}
	if len(flows) > 1 && flows[1].Precedence == flows[0].Precedence {
		_, err := fmt.Fprintf(w, "\nprecedence %d is shared; the API server picks one schema at that precedence\n", flows[0].Precedence)
		return err
	}
	return nil
}

// Users prints seat holders grouped by user.
func Users(w io.Writer, loads []apf.UserLoad) error {
	if len(loads) == 0 {
		_, err := fmt.Fprintln(w, "No requests are waiting or executing.")
		return err
	}
	rows := [][]string{{"USER", "WAITING", "EXECUTING", "SEATS"}}
	for _, load := range loads {
		rows = append(rows, []string{
			load.User,
			strconv.Itoa(load.Waiting),
			strconv.Itoa(load.Executing),
			strconv.Itoa(load.Seats),
		})
	}
	return writeTable(w, rows)
}

// Queues prints one row per shuffle-shard queue.
func Queues(w io.Writer, queues []apf.Queue) error {
	if len(queues) == 0 {
		_, err := fmt.Fprintln(w, "No queues have waiting or executing requests.")
		return err
	}
	rows := [][]string{{"LEVEL", "INDEX", "PENDING", "EXECUTING", "SEATS"}}
	for _, queue := range queues {
		rows = append(rows, []string{
			queue.PriorityLevel,
			dash(queue.Index),
			dash(queue.Pending),
			dash(queue.Executing),
			dash(queue.Seats),
		})
	}
	return writeTable(w, rows)
}

// Requests prints queued requests ahead of executing ones.
func Requests(w io.Writer, requests []apf.Request) error {
	if len(requests) == 0 {
		_, err := fmt.Fprintln(w, "No requests are waiting or executing.")
		return err
	}
	now := time.Now()
	rows := [][]string{{"STATE", "WAIT", "LEVEL", "FLOW SCHEMA", "DISTINGUISHER", "USER", "VERB", "TARGET", "SEATS", "QUEUE"}}
	for _, request := range requests {
		rows = append(rows, []string{
			request.State,
			apf.WaitLabel(request, now),
			request.PriorityLevel,
			request.FlowSchema,
			dash(request.Distinguisher),
			dash(request.User),
			dash(request.Verb),
			dash(request.Target),
			dash(request.Seats),
			request.Queue,
		})
	}
	return writeTable(w, rows)
}

func writeFlows(w io.Writer, flows []apf.Flow, withLevel bool) error {
	header := []string{"PREC", "FLOW SCHEMA", "DISTINGUISHER", "SUBJECTS"}
	if withLevel {
		header = []string{"PREC", "FLOW SCHEMA", "PRIORITY LEVEL", "DISTINGUISHER", "SUBJECTS", "DANGLING"}
	}
	rows := [][]string{header}
	for _, flow := range flows {
		name := flow.Name
		if !withLevel && flow.Dangling {
			name += " (dangling)"
		}
		row := []string{strconv.FormatInt(int64(flow.Precedence), 10), name, flow.Distinguisher, flow.Subjects}
		if withLevel {
			dangling := ""
			if flow.Dangling {
				dangling = "yes"
			}
			row = []string{
				strconv.FormatInt(int64(flow.Precedence), 10),
				flow.Name,
				dash(flow.PriorityLevel),
				flow.Distinguisher,
				flow.Subjects,
				dangling,
			}
		}
		rows = append(rows, row)
	}
	return writeTable(w, rows)
}

func writeTable(w io.Writer, rows [][]string) error {
	writer := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintln(writer, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeResourceRules(w io.Writer, rules []apf.ResourceRule) error {
	if len(rules) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "RESOURCES"); err != nil {
		return err
	}
	for _, rule := range rules {
		line := fmt.Sprintf("  verbs=%s groups=%s resources=%s namespaces=%s",
			joinOrDash(rule.Verbs), joinOrDash(rule.APIGroups), joinOrDash(rule.Resources), joinOrDash(rule.Namespaces))
		if rule.ClusterScope {
			line += " clusterScope=true"
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func writeNonResourceRules(w io.Writer, rules []apf.NonResourceRule) error {
	if len(rules) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "NON-RESOURCES"); err != nil {
		return err
	}
	for _, rule := range rules {
		if _, err := fmt.Fprintf(w, "  verbs=%s urls=%s\n", joinOrDash(rule.Verbs), joinOrDash(rule.URLs)); err != nil {
			return err
		}
	}
	return nil
}

func subjectLabel(subject apf.Subject) string {
	switch subject.Kind {
	case "ServiceAccount":
		if subject.Namespace == "" && subject.Name == "" {
			return "sa:?"
		}
		return "sa:" + subject.Namespace + "/" + subject.Name
	case "User":
		if subject.Name == "" {
			return "user:?"
		}
		return "user:" + subject.Name
	case "Group":
		if subject.Name == "" {
			return "group:?"
		}
		return "group:" + subject.Name
	default:
		if subject.Kind == "" {
			return "unknown"
		}
		return subject.Kind
	}
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ",")
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
