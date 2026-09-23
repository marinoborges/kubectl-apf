package apf

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	priorityLevelsPath = "/debug/api_priority_and_fairness/dump_priority_levels"
	queuesPath         = "/debug/api_priority_and_fairness/dump_queues"
	requestsPath       = "/debug/api_priority_and_fairness/dump_requests"
)

// RawFunc fetches a raw API server path, query string included.
type RawFunc func(ctx context.Context, path string) ([]byte, error)

// Loader reads flow-control objects and the API Priority and Fairness debug endpoints.
type Loader struct {
	Client kubernetes.Interface
	Raw    RawFunc
}

// Snapshot lists priority levels and flow schemas. When live is set, waiting,
// executing, and rejected counters are joined from the debug endpoint.
func (l *Loader) Snapshot(ctx context.Context, live bool) (*Snapshot, error) {
	levels, err := l.Client.FlowcontrolV1().PriorityLevelConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list priority level configurations: %w", err)
	}
	flows, err := l.Client.FlowcontrolV1().FlowSchemas().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list flow schemas: %w", err)
	}

	var raw []byte
	var warning string
	if live {
		raw, err = l.Raw(ctx, priorityLevelsPath)
		if err != nil {
			warning = "live counters unavailable: " + explainLive(err)
			raw = nil
		}
	}

	snapshot, err := BuildSnapshot(levels, flows, raw)
	if err != nil {
		return nil, fmt.Errorf("parse priority level debug data: %w", err)
	}
	snapshot.Warning = warning
	return snapshot, nil
}

// Queues lists the shuffle-shard queues from the debug endpoint.
func (l *Loader) Queues(ctx context.Context) ([]Queue, error) {
	raw, err := l.Raw(ctx, queuesPath)
	if err != nil {
		return nil, fmt.Errorf("read queue debug data: %s", explainLive(err))
	}
	queues, err := BuildQueues(raw)
	if err != nil {
		return nil, fmt.Errorf("parse queue debug data: %w", err)
	}
	return queues, nil
}

// Requests lists queued and executing requests. It asks for user, verb, and
// resource columns, then retries without them on servers that reject the query.
func (l *Loader) Requests(ctx context.Context) ([]Request, error) {
	raw, err := l.Raw(ctx, requestsPath+"?includeRequestDetails=1")
	if apierrors.IsBadRequest(err) || apierrors.IsNotAcceptable(err) {
		raw, err = l.Raw(ctx, requestsPath)
	}
	if err != nil {
		return nil, fmt.Errorf("read request debug data: %s", explainLive(err))
	}
	requests, err := BuildRequests(raw)
	if err != nil {
		return nil, fmt.Errorf("parse request debug data: %w", err)
	}
	return requests, nil
}

func explainLive(err error) string {
	if apierrors.IsForbidden(err) {
		return "forbidden; grant get on non-resource URL /debug/api_priority_and_fairness/*"
	}
	return err.Error()
}
