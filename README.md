# kubectl-apf

`kubectl apf` shows help for this API Priority and Fairness (APF) plugin. `kubectl apf detail` shows priority levels and the flow schemas that select into them.

```text
kubectl apf detail workload-low
PriorityLevelConfigurations
NAME           TYPE     SHARES  LEND  BORROW  RESPONSE         WAITING  EXECUTING  REJECTED  FLOWS
workload-low   Limited  100     50%   0%      Queue 128/6/50   12       4          30        2

FlowSchemas
PREC  FLOW SCHEMA       DISTINGUISHER  SUBJECTS
1000  replicaset-controller  ByUser  sa:kube-system/replicaset-controller
9000  service-accounts  ByUser         group:system:serviceaccounts
```

`kubectl apf detail all` lists every priority level under `PriorityLevelConfigurations`, then prints `PriorityLevelConfigurations X FlowSchemas` once before the flow schemas under each level.

`Queue 128/6/50` is the number of queues, the hand size, and the queue length limit. Flow schemas are listed in matching order: a lower precedence is chosen first.

| Command | What it shows |
|---|---|
| `kubectl apf` | Help |
| `kubectl apf plcs` | Priority levels, including waiting, executing, rejected, timed-out, and cancelled counts, plus busy/total queues. `plc` is an alias |
| `kubectl apf detail <name>` | One priority level and the flow schemas under it. `all` shows every level. No argument prints help |
| `kubectl apf flows [name]` | Flow schemas in matching order. A name prints that schema's subjects and rules. `flowschema` and `flowschemas` are aliases |
| `kubectl apf match` | Which flow schema a user, verb, and resource would hit |
| `kubectl apf users [name...]` | Users waiting or holding seats, from the current request snapshot |
| `kubectl apf queues [name...]` | Queues with requests waiting or executing. `queue` is an alias. `--all` includes idle queues. Names limit the list to those priority levels |
| `kubectl apf requests [name...]` | Requests waiting or executing right now. Names limit the list to those priority levels. `--omit-observer` hides this command's own debug request |

`kubectl apf queues` reads `/debug/api_priority_and_fairness/dump_queues`, one row per shuffle-shard queue. Idle queues are left out unless `--all` is set.

`kubectl apf requests` reads `/debug/api_priority_and_fairness/dump_requests`. That endpoint is a snapshot of requests waiting in a queue or executing when the API server answers. This plugin can only poll that endpoint and keep the rows it sees. Requests that start and finish between polls never appear. A record of requests over a long interval is the API server audit log, where each request is logged with the flow schema and priority level that handled it.

`--context` and `--kubeconfig` work the same way as kubectl.

## Install

```bash
go build -o kubectl-apf .
install kubectl-apf ~/.local/bin/
kubectl apf
```

The binary name has to be `kubectl-apf`. kubectl turns `kubectl apf` into that executable.

## Permissions

Listing the objects needs `get` and `list` on `flowschemas` and `prioritylevelconfigurations` in the `flowcontrol.apiserver.k8s.io` API group.

Live counters come from `/debug/api_priority_and_fairness/dump_priority_levels`, `/debug/api_priority_and_fairness/dump_queues`, and `/debug/api_priority_and_fairness/dump_requests`. A cluster admin can call those. Anyone else needs `get` on that non-resource URL. When the debug call is forbidden, `kubectl apf` still prints the objects and says the counters are unavailable. `kubectl apf requests` needs the debug endpoint.

## Develop

```bash
go test ./...
go run . version
```

Releases are built with GoReleaser from `.goreleaser.yaml`. Tag `v0.1.0` and the archives are ready to publish on [krew](https://krew.sigs.k8s.io/docs/developer-guide/distributing-with-krew/).
