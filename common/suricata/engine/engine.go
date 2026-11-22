package engine

import (
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

type Config struct {
	Epoch               uint64
	MaxStreamBytes      int
	MaxTotalStreamBytes int64
	MaxFlows            int
	UDPFlowTimeout      time.Duration
	State               state.Config
	OnAlert             func(Alert)
	OnDiagnostic        func(Diagnostic)
	// DisablePrefilter turns off minirehs/protocol prefiltering and evaluates
	// every rule linearly. Primarily intended for differential equivalence
	// verification and benchmarking.
	DisablePrefilter bool
}

type PrefilterStats struct {
	Enabled                bool
	ActiveRules            int
	CandidatesEvaluated    uint64
	RulesSkipped           uint64
	StreamFastFailHTTP     uint64
	StreamFastFailTLS      uint64
	DecompressedHTTPBodies uint64
	Reloads                uint64
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
	Capture      AdapterStats
	Prefilter    PrefilterStats
}

type ruleSnapshot struct {
	plans     []*CompiledRulePlan
	prefilter *enginePrefilter
}

type StatefulEngine struct {
	epoch            uint64
	maxStreamBytes   int
	disablePrefilter bool
	snapshotMu       sync.RWMutex
	snapshot         *ruleSnapshot
	store            *state.Store
	onAlert          func(Alert)
	onDiagnostic     func(Diagnostic)

	flowsMu             sync.Mutex
	flows               map[FlowKey]*flowRuntime
	tombstones          map[FlowKey]struct{}
	tombstoneOrder      []FlowKey
	closed              atomic.Bool
	streamBytes         int64
	maxFlows            int
	maxTotalStreamBytes int64

	captureMu      sync.Mutex
	captureAdapter *PcapAdapter
	captureOption  pcaputil.CaptureOption

	udpIndex      map[udpTupleKey]FlowKey
	udpGeneration atomic.Uint64
	udpTimeout    time.Duration

	events                 atomic.Uint64
	packetEvents           atomic.Uint64
	streamEvents           atomic.Uint64
	transactions           atomic.Uint64
	alerts                 atomic.Uint64
	suppressed             atomic.Uint64
	diagnostics            atomic.Uint64
	candidatesEvaluated    atomic.Uint64
	rulesSkipped           atomic.Uint64
	streamFastFailHTTP     atomic.Uint64
	streamFastFailTLS      atomic.Uint64
	decompressedHTTPBodies atomic.Uint64
	reloads                atomic.Uint64
}

func compileRuleSnapshot(rules []*rule.Rule, disablePrefilter bool) (*ruleSnapshot, error) {
	identities := make(map[[2]int]struct{}, len(rules))
	plans := make([]*CompiledRulePlan, 0, len(rules))
	for index, current := range rules {
		if current == nil || current.Sid <= 0 {
			return nil, fmt.Errorf("stateful rule requires a positive sid")
		}
		identity := [2]int{current.Gid, current.Sid}
		if _, exists := identities[identity]; exists {
			return nil, fmt.Errorf("duplicate stateful rule identity gid=%d sid=%d", current.Gid, current.Sid)
		}
		identities[identity] = struct{}{}
		plan, err := CompileStatefulRule(current)
		if err != nil {
			return nil, fmt.Errorf("compile stateful rule sid=%d: %w", current.Sid, err)
		}
		plan.Index = uint64(index + 1)
		plans = append(plans, plan)
	}
	var pf *enginePrefilter
	if !disablePrefilter && len(plans) > 0 {
		pf = buildEnginePrefilter(plans)
	}
	return &ruleSnapshot{
		plans:     plans,
		prefilter: pf,
	}, nil
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
	if config.UDPFlowTimeout <= 0 {
		config.UDPFlowTimeout = 30 * time.Second
	}
	stateConfig := config.State
	if stateConfig.Shards == 0 {
		stateConfig = state.DefaultConfig()
	}
	store, err := state.NewStore(stateConfig, nil)
	if err != nil {
		return nil, err
	}
	snapshot, err := compileRuleSnapshot(rules, config.DisablePrefilter)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	engine := &StatefulEngine{
		epoch:               config.Epoch,
		maxStreamBytes:      config.MaxStreamBytes,
		maxTotalStreamBytes: config.MaxTotalStreamBytes,
		maxFlows:            config.MaxFlows,
		disablePrefilter:    config.DisablePrefilter,
		snapshot:            snapshot,
		store:               store,
		onAlert:             config.OnAlert,
		onDiagnostic:        config.OnDiagnostic,
		flows:               make(map[FlowKey]*flowRuntime),
		tombstones:          make(map[FlowKey]struct{}),
		udpIndex:            make(map[udpTupleKey]FlowKey),
		udpTimeout:          config.UDPFlowTimeout,
	}
	// Keep UDP pseudo-flow generations disjoint from pcaputil TCP generation
	// numbers so the engine's FlowKey map cannot collide across transports.
	engine.udpGeneration.Store(uint64(1) << 63)
	return engine, nil
}

// Reload compiles a replacement rule set and prefilter off the hot path, then
// atomically swaps it into the running engine without dropping active flows or
// flow state. If compilation fails, the active rule set remains unchanged.
func (e *StatefulEngine) Reload(rules []*rule.Rule) error {
	if e == nil || e.closed.Load() {
		return fmt.Errorf("suricata engine is closed")
	}
	next, err := compileRuleSnapshot(rules, e.disablePrefilter)
	if err != nil {
		return err
	}
	e.snapshotMu.Lock()
	if e.closed.Load() {
		e.snapshotMu.Unlock()
		if next.prefilter != nil {
			next.prefilter.close()
		}
		return fmt.Errorf("suricata engine is closed")
	}
	prev := e.snapshot
	e.snapshot = next
	e.snapshotMu.Unlock()

	// All readers that held snapshotMu.RLock() for prev have exited before
	// Lock() was granted, so closing prev.prefilter here is race-free.
	if prev != nil && prev.prefilter != nil {
		prev.prefilter.close()
	}
	e.reloads.Add(1)
	return nil
}

func (e *StatefulEngine) Handle(event InspectionEvent) Result {
	if e == nil || e.closed.Load() {
		return Result{}
	}
	e.events.Add(1)
	if e.events.Load()%1024 == 0 {
		e.cleanupUDPFlows(eventTime(event))
		_, _ = e.store.Cleanup(eventTime(event))
	}
	switch event.View {
	case ViewPhysicalPacket:
		// No compiled rule targets raw physical frames directly; avoid
		// scanning every rule plan on each captured packet.
		return Result{}
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
		event = e.attachUDPFlow(event)
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
		// Complete decoded transactions do not currently match packet/stream
		// rule plans unless explicitly enabled; fast-fail when prefilter is on.
		if !e.disablePrefilter {
			return Result{}
		}
	}
	return e.inspect(event)
}

func (e *StatefulEngine) Close() error {
	if e == nil || !e.closed.CompareAndSwap(false, true) {
		return nil
	}
	e.snapshotMu.Lock()
	prev := e.snapshot
	e.snapshot = &ruleSnapshot{}
	e.snapshotMu.Unlock()
	if prev != nil && prev.prefilter != nil {
		prev.prefilter.close()
	}
	e.flowsMu.Lock()
	e.flows = make(map[FlowKey]*flowRuntime)
	e.udpIndex = make(map[udpTupleKey]FlowKey)
	e.flowsMu.Unlock()
	return e.store.Close()
}

func (e *StatefulEngine) Stats() Stats {
	stateStats := e.store.Stats()
	e.flowsMu.Lock()
	active := len(e.flows)
	e.flowsMu.Unlock()
	e.snapshotMu.RLock()
	activeRules := 0
	prefilterEnabled := false
	if e.snapshot != nil {
		activeRules = len(e.snapshot.plans)
		prefilterEnabled = e.snapshot.prefilter != nil
	}
	e.snapshotMu.RUnlock()
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
		Capture:      e.CaptureStats(),
		Prefilter: PrefilterStats{
			Enabled:                prefilterEnabled,
			ActiveRules:            activeRules,
			CandidatesEvaluated:    e.candidatesEvaluated.Load(),
			RulesSkipped:           e.rulesSkipped.Load(),
			StreamFastFailHTTP:     e.streamFastFailHTTP.Load(),
			StreamFastFailTLS:      e.streamFastFailTLS.Load(),
			DecompressedHTTPBodies: e.decompressedHTTPBodies.Load(),
			Reloads:                e.reloads.Load(),
		},
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
		flow = &flowRuntime{
			streamCursor: make(map[ruleDirection]int),
			tlsMatched:   make(map[ruleDirection]bool),
		}
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
	if event.View == ViewNormalizedPacket {
		populatePacketEndpoints(&event)
	}
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
		if event.View == ViewNormalizedPacket {
			e.emitTaggedAlerts(flow, event, &result)
		}
		if flow.transport == 0 {
			flow.transport = event.Flow.IPProtocol
			if flow.transport == 0 {
				flow.transport = uint8(layers.IPProtocolTCP)
			}
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

	httpOffset := 0
	var httpPayload []byte
	var fullStreamPayload []byte
	if event.View == ViewStream && flow != nil {
		fullStreamPayload = flow.stream[event.Direction]
		httpOffset = flow.httpNextOffset[event.Direction]
		if httpOffset < len(fullStreamPayload) {
			httpPayload = fullStreamPayload[httpOffset:]
		}
	}

	e.snapshotMu.RLock()
	defer e.snapshotMu.RUnlock()
	snap := e.snapshot
	if snap == nil || len(snap.plans) == 0 {
		return result
	}
	plans := snap.plans
	prefilter := snap.prefilter

	var sharedHTTP *match.HTTPBufferProvider
	var sharedTLS *match.TLSBufferProvider
	var headerMeta packetHeaderMeta

	evalPlan := func(index int) {
		plan := plans[index]
		switch event.View {
		case ViewNormalizedPacket:
			if !plan.acceptsPacket {
				return
			}
		case ViewStream:
			if !plan.acceptsStream {
				return
			}
		default:
			if !plan.acceptsView(event.View) {
				return
			}
		}
		if flow != nil && !matchFlow(plan.Flow, flow, event.Direction) {
			return
		}
		if headerMeta.hasAddr && !matchRuleHeaderFast(plan.Rule, headerMeta) {
			return
		}
		if len(plan.StreamSize) > 0 {
			packetView := plan.Rule != nil && plan.Rule.Protocol == "tcp-pkt"
			if !plan.matchStreamSize(flow, packetView) {
				return
			}
		}
		var streamKey ruleDirection
		if plan.Rule != nil {
			streamKey = ruleDirection{gid: plan.Rule.Gid, sid: plan.Rule.Sid, direction: event.Direction}
		} else {
			streamKey = ruleDirection{sid: -index - 1, direction: event.Direction}
		}
		var streamPayload []byte
		cursor := 0
		isHTTP := plan.isHTTP
		isTLS := plan.isTLS
		if event.View == ViewStream && flow != nil && isTLS && flow.tlsMatched[streamKey] {
			// TLS buffers are handshake-scoped. Once a rule matched a
			// direction, later stream frames repeat the same certificate or
			// SNI data and must not produce duplicate alerts or state effects.
			return
		}
		if event.View == ViewStream && flow != nil {
			if isHTTP {
				cursor = httpOffset
				streamPayload = httpPayload
				if len(streamPayload) == 0 {
					return
				}
			} else {
				cursor = flow.streamCursor[streamKey]
				streamPayload = fullStreamPayload
				flow.streamCursor[streamKey] = len(streamPayload)
			}
		}
		var planHTTP *match.HTTPBufferProvider
		if isHTTP {
			planHTTP = sharedHTTP
		}
		var planTLS *match.TLSBufferProvider
		if isTLS {
			planTLS = sharedTLS
		}
		matched, evidenceEnd, err := plan.matchWithShared(event, flow, streamPayload, planHTTP, planTLS)
		if err != nil {
			e.diagnostic(&result, event, plan.Rule, "match-error", err.Error())
			return
		}
		if !matched {
			return
		}
		if event.View == ViewStream && !isHTTP && !isTLS && evidenceEnd <= cursor {
			return
		}
		stateResult, err := e.applyState(plan, event)
		if err != nil {
			e.diagnostic(&result, event, plan.Rule, "state-error", err.Error())
			return
		}
		if event.View == ViewStream && flow != nil && isTLS {
			flow.tlsMatched[streamKey] = true
		}
		if !stateResult.qualified {
			return
		}
		if !stateResult.emit || plan.NoAlert {
			e.suppressed.Add(1)
			return
		}
		alert := buildAlert(plan.Rule, event)
		result.Alerts = append(result.Alerts, alert)
		e.alerts.Add(1)
		if e.onAlert != nil {
			e.onAlert(alert)
		}
		if plan.Tag != nil && flow != nil {
			tag := flowTag{plan: plan, unit: plan.Tag.Unit}
			switch plan.Tag.Unit {
			case "packets":
				tag.remaining = uint64(plan.Tag.Count)
			case "seconds":
				tag.expiresAt = eventTime(event).Add(time.Duration(plan.Tag.Count) * time.Second)
			}
			flow.tags = append(flow.tags, tag)
		}
	}

	if prefilter != nil && (event.View == ViewStream || event.View == ViewNormalizedPacket) {
		collector := prefilter.acquireCollector()
		if event.View == ViewStream {
			var httpFastFailed, tlsFastFailed bool
			sharedHTTP, sharedTLS, headerMeta, httpFastFailed, tlsFastFailed = prefilter.collectStreamCandidates(collector, event.Packet, flow, event.Direction, fullStreamPayload, httpPayload)
			if httpFastFailed {
				e.streamFastFailHTTP.Add(1)
			}
			if tlsFastFailed {
				e.streamFastFailTLS.Add(1)
			}
		} else {
			headerMeta = prefilter.collectPacketCandidates(collector, event.Packet)
		}
		candidates := collector.sorted()
		e.candidatesEvaluated.Add(uint64(len(candidates)))
		if len(plans) > len(candidates) {
			e.rulesSkipped.Add(uint64(len(plans) - len(candidates)))
		}
		for _, index := range candidates {
			evalPlan(index)
		}
		if sharedHTTP != nil && sharedHTTP.WasDecompressed() {
			e.decompressedHTTPBodies.Add(1)
		}
		prefilter.releaseCollector(collector)
	} else {
		e.candidatesEvaluated.Add(uint64(len(plans)))
		for index := range plans {
			evalPlan(index)
		}
	}
	// Advance the per-direction HTTP message cursor only after every HTTP rule
	// has seen this message. Incomplete messages stay at the same offset until
	// more stream bytes arrive.
	if event.View == ViewStream && flow != nil && len(httpPayload) > 0 {
		if length, complete := match.HTTPMessageLength(httpPayload); complete && length > 0 {
			flow.httpNextOffset[event.Direction] = httpOffset + length
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
	if currentRule != nil {
		alert.Target = currentRule.Target
	}
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

func populatePacketEndpoints(event *InspectionEvent) {
	if event == nil || event.Packet == nil || event.Flow.First.Address.IsValid() {
		return
	}
	network := event.Packet.NetworkLayer()
	if network == nil {
		return
	}
	flow := network.NetworkFlow()
	source, err := netip.ParseAddr(flow.Src().String())
	if err != nil {
		return
	}
	destination, err := netip.ParseAddr(flow.Dst().String())
	if err != nil {
		return
	}
	var sourcePort, destinationPort uint16
	if transport := event.Packet.TransportLayer(); transport != nil {
		transportFlow := transport.TransportFlow()
		if endpoint, err := netip.ParseAddrPort(transportFlow.Src().String()); err == nil {
			sourcePort = endpoint.Port()
		}
		if endpoint, err := netip.ParseAddrPort(transportFlow.Dst().String()); err == nil {
			destinationPort = endpoint.Port()
		}
	}
	event.Flow.First = Endpoint{Address: source.Unmap(), Port: sourcePort}
	event.Flow.Second = Endpoint{Address: destination.Unmap(), Port: destinationPort}
	switch network := network.(type) {
	case *layers.IPv4:
		event.Flow.IPv4 = true
		event.Flow.IPProtocol = uint8(network.Protocol)
	case *layers.IPv6:
		event.Flow.IPv6 = true
		event.Flow.IPProtocol = uint8(network.NextHeader)
	}
}

func (e *StatefulEngine) emitTaggedAlerts(flow *flowRuntime, event InspectionEvent, result *Result) {
	if flow == nil || len(flow.tags) == 0 {
		return
	}
	now := eventTime(event)
	kept := flow.tags[:0]
	for index := range flow.tags {
		tag := flow.tags[index]
		switch tag.unit {
		case "seconds":
			if !now.Before(tag.expiresAt) {
				continue
			}
			e.emitTaggedAlert(tag.plan, event, result)
			kept = append(kept, tag)
		case "packets":
			if tag.remaining == 0 {
				continue
			}
			e.emitTaggedAlert(tag.plan, event, result)
			tag.remaining--
			if tag.remaining > 0 {
				kept = append(kept, tag)
			}
		}
	}
	flow.tags = kept
}

func (e *StatefulEngine) emitTaggedAlert(plan *CompiledRulePlan, event InspectionEvent, result *Result) {
	if plan == nil || plan.Rule == nil {
		return
	}
	alert := buildAlert(plan.Rule, event)
	alert.Tagged = true
	result.Alerts = append(result.Alerts, alert)
	e.alerts.Add(1)
	if e.onAlert != nil {
		e.onAlert(alert)
	}
}

func eventTime(event InspectionEvent) time.Time {
	if event.Timestamp.IsZero() {
		return time.Now()
	}
	return event.Timestamp
}
