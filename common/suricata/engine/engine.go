package engine

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

type Config struct {
	Epoch               uint64
	MaxStreamBytes      int
	MaxTotalStreamBytes int64
	MaxFlows            int
	State               state.Config
	OnAlert             func(Alert)
	OnDiagnostic        func(Diagnostic)
}

type Stats struct {
	Events       uint64
	PacketEvents uint64
	StreamEvents uint64
	Transactions uint64
	Alerts       uint64
	Suppressed   uint64
	Diagnostics  uint64
	ActiveFlows  int
	State        state.Stats
}

type StatefulEngine struct {
	epoch          uint64
	maxStreamBytes int
	plans          []*CompiledRulePlan
	store          *state.Store
	onAlert        func(Alert)
	onDiagnostic   func(Diagnostic)

	flowsMu             sync.Mutex
	flows               map[FlowKey]*flowRuntime
	tombstones          map[FlowKey]struct{}
	tombstoneOrder      []FlowKey
	closed              atomic.Bool
	streamBytes         int64
	maxFlows            int
	maxTotalStreamBytes int64

	events       atomic.Uint64
	packetEvents atomic.Uint64
	streamEvents atomic.Uint64
	transactions atomic.Uint64
	alerts       atomic.Uint64
	suppressed   atomic.Uint64
	diagnostics  atomic.Uint64
}

func NewStatefulEngine(rules []*rule.Rule, config Config) (*StatefulEngine, error) {
	if config.Epoch == 0 {
		config.Epoch = 1
	}
	if config.MaxStreamBytes <= 0 {
		config.MaxStreamBytes = 8 << 20
	}
	if config.MaxTotalStreamBytes <= 0 {
		config.MaxTotalStreamBytes = 256 << 20
	}
	if config.MaxFlows <= 0 {
		config.MaxFlows = 65536
	}
	stateConfig := config.State
	if stateConfig.Shards == 0 {
		stateConfig = state.DefaultConfig()
	}
	store, err := state.NewStore(stateConfig, nil)
	if err != nil {
		return nil, err
	}
	engine := &StatefulEngine{
		epoch:               config.Epoch,
		maxStreamBytes:      config.MaxStreamBytes,
		maxTotalStreamBytes: config.MaxTotalStreamBytes,
		maxFlows:            config.MaxFlows,
		store:               store,
		onAlert:             config.OnAlert,
		onDiagnostic:        config.OnDiagnostic,
		flows:               make(map[FlowKey]*flowRuntime),
		tombstones:          make(map[FlowKey]struct{}),
	}
	identities := make(map[[2]int]struct{}, len(rules))
	for index, current := range rules {
		identity := [2]int{current.Gid, current.Sid}
		if current.Sid <= 0 {
			_ = store.Close()
			return nil, fmt.Errorf("stateful rule requires a positive sid")
		}
		if _, exists := identities[identity]; exists {
			_ = store.Close()
			return nil, fmt.Errorf("duplicate stateful rule identity gid=%d sid=%d", current.Gid, current.Sid)
		}
		identities[identity] = struct{}{}
		plan, err := CompileStatefulRule(current)
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("compile stateful rule sid=%d: %w", current.Sid, err)
		}
		plan.Index = uint64(index + 1)
		engine.plans = append(engine.plans, plan)
	}
	return engine, nil
}

func (e *StatefulEngine) Handle(event InspectionEvent) Result {
	if e == nil || e.closed.Load() {
		return Result{}
	}
	e.events.Add(1)
	if e.events.Load()%1024 == 0 {
		_, _ = e.store.Cleanup(eventTime(event))
	}
	switch event.View {
	case ViewFlowCreated:
		e.flowsMu.Lock()
		delete(e.tombstones, event.Flow.Key)
		e.flowsMu.Unlock()
		e.flow(event.Flow.Key, true)
		return Result{}
	case ViewFlowClosed:
		return e.closeFlow(event)
	case ViewNormalizedPacket:
		e.packetEvents.Add(1)
	case ViewStream:
		e.streamEvents.Add(1)
	case ViewTransaction:
		e.transactions.Add(1)
		if event.Protocol == nil || event.Protocol.Status != "decoded" || event.Protocol.Completeness == "incomplete" || event.Protocol.Error != "" {
			var result Result
			message := "protocol transaction is not complete"
			if event.Protocol != nil && event.Protocol.Error != "" {
				message = event.Protocol.Error
			}
			e.diagnostic(&result, event, nil, "protocol-event", message)
			return result
		}
	}
	return e.inspect(event)
}

func (e *StatefulEngine) Close() error {
	if e == nil || !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	e.flowsMu.Lock()
	e.flows = make(map[FlowKey]*flowRuntime)
	e.flowsMu.Unlock()
	return e.store.Close()
}

func (e *StatefulEngine) Stats() Stats {
	stateStats := e.store.Stats()
	e.flowsMu.Lock()
	active := len(e.flows)
	e.flowsMu.Unlock()
	return Stats{
		Events:       e.events.Load(),
		PacketEvents: e.packetEvents.Load(),
		StreamEvents: e.streamEvents.Load(),
		Transactions: e.transactions.Load(),
		Alerts:       e.alerts.Load(),
		Suppressed:   e.suppressed.Load(),
		Diagnostics:  e.diagnostics.Load(),
		ActiveFlows:  active,
		State:        stateStats,
	}
}

func (e *StatefulEngine) flow(key FlowKey, create bool) *flowRuntime {
	if key.Generation == 0 {
		return nil
	}
	e.flowsMu.Lock()
	defer e.flowsMu.Unlock()
	if _, closed := e.tombstones[key]; closed {
		return nil
	}
	flow := e.flows[key]
	if flow == nil && create && len(e.flows) < e.maxFlows {
		flow = &flowRuntime{streamCursor: make(map[ruleDirection]int)}
		e.flows[key] = flow
	}
	return flow
}

func (e *StatefulEngine) closeFlow(event InspectionEvent) Result {
	e.flowsMu.Lock()
	flow := e.flows[event.Flow.Key]
	e.flowsMu.Unlock()
	if flow != nil {
		flow.mu.Lock()
		defer flow.mu.Unlock()
		flow.closed = true
		flow.incomplete = event.Flow.PendingGaps
	}
	e.flowsMu.Lock()
	if e.flows[event.Flow.Key] == flow {
		delete(e.flows, event.Flow.Key)
	}
	e.tombstones[event.Flow.Key] = struct{}{}
	e.tombstoneOrder = append(e.tombstoneOrder, event.Flow.Key)
	for len(e.tombstoneOrder) > e.maxFlows {
		oldest := e.tombstoneOrder[0]
		e.tombstoneOrder = e.tombstoneOrder[1:]
		delete(e.tombstones, oldest)
	}
	if flow != nil {
		e.streamBytes -= int64(len(flow.stream[0]) + len(flow.stream[1]))
	}
	e.flowsMu.Unlock()
	if key, err := stateFlowKey(e.epoch, event.Flow); err == nil {
		_, _ = e.store.DeleteFlow(key)
	}
	return Result{}
}

func (e *StatefulEngine) releaseFlowStreams(flow *flowRuntime) {
	if flow == nil {
		return
	}
	released := len(flow.stream[0]) + len(flow.stream[1])
	flow.stream[0], flow.stream[1] = nil, nil
	e.flowsMu.Lock()
	e.streamBytes -= int64(released)
	if e.streamBytes < 0 {
		e.streamBytes = 0
	}
	e.flowsMu.Unlock()
}

func (e *StatefulEngine) inspect(event InspectionEvent) Result {
	var result Result
	flow := e.flow(event.Flow.Key, event.Flow.Key.Generation != 0)
	if event.Flow.Key.Generation != 0 && flow == nil {
		e.diagnostic(&result, event, nil, "flow-unavailable", "flow is closed or engine flow capacity was exceeded")
		return result
	}
	if flow != nil {
		flow.mu.Lock()
		defer flow.mu.Unlock()
		if flow.closed {
			return result
		}
		if event.View == ViewNormalizedPacket && event.Packet != nil {
			if tcp, _ := event.Packet.Layer(layers.LayerTypeTCP).(*layers.TCP); tcp != nil {
				flow.observeTCP(event.Direction, tcp)
			}
		}
		if event.View == ViewStream {
			if event.Flow.PendingGaps || flow.incomplete {
				e.diagnostic(&result, event, nil, "incomplete-flow", "stream event rejected because the flow has pending gaps")
				return result
			}
			if event.Direction < 0 || event.Direction > 1 {
				e.diagnostic(&result, event, nil, "invalid-direction", "stream event has an invalid direction")
				return result
			}
			if len(flow.stream[event.Direction])+len(event.Payload) > e.maxStreamBytes {
				flow.streamOverflow[event.Direction] = true
				e.releaseFlowStreams(flow)
				e.diagnostic(&result, event, nil, "stream-limit", "stream buffer capacity exceeded")
				return result
			}
			e.flowsMu.Lock()
			if e.streamBytes+int64(len(event.Payload)) > e.maxTotalStreamBytes {
				e.flowsMu.Unlock()
				flow.streamOverflow[event.Direction] = true
				e.releaseFlowStreams(flow)
				e.diagnostic(&result, event, nil, "stream-global-limit", "global stream buffer capacity exceeded")
				return result
			}
			e.streamBytes += int64(len(event.Payload))
			e.flowsMu.Unlock()
			flow.stream[event.Direction] = append(flow.stream[event.Direction], event.Payload...)
		}
	}

	for index, plan := range e.plans {
		if !plan.acceptsView(event.View) {
			continue
		}
		if flow != nil && !matchFlow(plan.Flow, flow, event.Direction) {
			continue
		}
		streamKey := ruleDirection{ruleIndex: index, direction: event.Direction}
		var streamPayload []byte
		cursor := 0
		if event.View == ViewStream && flow != nil {
			cursor = flow.streamCursor[streamKey]
			streamPayload = flow.stream[event.Direction]
			flow.streamCursor[streamKey] = len(streamPayload)
		}
		matched, evidenceEnd, err := plan.match(event, flow, streamPayload)
		if err != nil {
			e.diagnostic(&result, event, plan.Rule, "match-error", err.Error())
			continue
		}
		if !matched || event.View == ViewStream && evidenceEnd <= cursor {
			continue
		}
		stateResult, err := e.applyState(plan, event)
		if err != nil {
			e.diagnostic(&result, event, plan.Rule, "state-error", err.Error())
			continue
		}
		if !stateResult.qualified {
			continue
		}
		if !stateResult.emit || plan.NoAlert {
			e.suppressed.Add(1)
			continue
		}
		alert := buildAlert(plan.Rule, event)
		result.Alerts = append(result.Alerts, alert)
		e.alerts.Add(1)
		if e.onAlert != nil {
			e.onAlert(alert)
		}
	}
	return result
}

func (e *StatefulEngine) diagnostic(result *Result, event InspectionEvent, currentRule *rule.Rule, kind, message string) {
	diagnostic := Diagnostic{Timestamp: event.Timestamp, Flow: event.Flow, Kind: kind, Message: message, Rule: currentRule}
	result.Diagnostics = append(result.Diagnostics, diagnostic)
	e.diagnostics.Add(1)
	if e.onDiagnostic != nil {
		e.onDiagnostic(diagnostic)
	}
}

func buildAlert(currentRule *rule.Rule, event InspectionEvent) Alert {
	alert := Alert{Timestamp: event.Timestamp, Rule: currentRule, Flow: event.Flow, Direction: event.Direction, View: event.View, References: append([]pcaputil.PacketReference(nil), event.References...)}
	if event.Protocol != nil {
		alert.Protocol = event.Protocol.Protocol
		alert.Profile = event.Protocol.Profile
		alert.EventID = event.Protocol.ID
		alert.FlowID = event.Protocol.FlowID
		alert.TCPFlowID = event.Protocol.TCPFlowID
		alert.TransactionID = event.Protocol.TransactionID
		alert.ResponseTo = event.Protocol.ResponseTo
		alert.Summary = event.Protocol.Summary
	}
	return alert
}

func eventTime(event InspectionEvent) time.Time {
	if event.Timestamp.IsZero() {
		return time.Now()
	}
	return event.Timestamp
}
