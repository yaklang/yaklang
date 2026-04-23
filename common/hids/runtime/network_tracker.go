//go:build hids && linux

package runtime

import (
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

type networkTracker struct {
	byProcessFD        map[networkProcessFDKey]*trackedNetwork
	byTuple            map[networkTupleCacheKey]*trackedNetwork
	window             time.Duration
	maxEntries         int
	nextPrune          time.Time
	sequence           uint64
	order              []trackedNetworkOrder
	detailedEnrichment bool
	freeEntries        []*trackedNetwork
}

type trackedNetwork struct {
	process         model.Process
	network         model.Network
	data            map[string]any
	openedAt        time.Time
	stateChanged    time.Time
	updatedAt       time.Time
	processFDKey    networkProcessFDKey
	tupleKey        networkTupleCacheKey
	hasProcessFDKey bool
	hasTupleKey     bool
	sequence        uint64
}

type networkProcessFDKey struct {
	pid int
	fd  int
}

type networkTupleCacheKey struct {
	protocol      string
	sourceAddress string
	sourcePort    int
	destAddress   string
	destPort      int
}

type trackedNetworkOrder struct {
	entry    *trackedNetwork
	sequence uint64
	lastSeen time.Time
}

func newNetworkTracker() *networkTracker {
	return newNetworkTrackerWithConfig(shortTermContextConfigFromSpec(model.DesiredSpec{}))
}

func newNetworkTrackerWithConfig(config shortTermContextConfig) *networkTracker {
	if config.window <= 0 {
		config.window = time.Duration(model.DefaultShortTermWindowMinutes) * time.Minute
	}
	if config.maxNetworks <= 0 {
		config.maxNetworks = defaultShortTermContextMaxNetworks
	}
	return &networkTracker{
		byProcessFD:        make(map[networkProcessFDKey]*trackedNetwork),
		byTuple:            make(map[networkTupleCacheKey]*trackedNetwork),
		window:             config.window,
		maxEntries:         config.maxNetworks,
		detailedEnrichment: true,
	}
}

func (t *networkTracker) Apply(event model.Event) model.Event {
	if t == nil {
		return event
	}

	observedAt := normalizedEventTimestamp(event.Timestamp)
	t.prune(observedAt)
	switch event.Type {
	case model.EventTypeNetworkAccept:
		event = t.enrichOpen(event)
		t.remember(event, observedAt)
	case model.EventTypeNetworkConnect:
		event = t.enrichOpen(event)
		t.remember(event, observedAt)
	case model.EventTypeNetworkState:
		event = t.enrichState(event, observedAt)
		event = t.enrichNetworkDetails(event)
	case model.EventTypeNetworkClose:
		event = t.enrichClose(event)
		event = t.enrichNetworkDetails(event)
		t.forget(event)
	case model.EventTypeProcessExit:
		t.forgetProcess(event.Process)
	}
	return event
}

func (t *networkTracker) enrichOpen(event model.Event) model.Event {
	if t == nil || !t.detailedEnrichment {
		return enrichNetworkDirection(event)
	}
	event = enrichNetworkEvent(event)
	return initializeNetworkLifecycle(event)
}

func (t *networkTracker) enrichNetworkDetails(event model.Event) model.Event {
	if t == nil || !t.detailedEnrichment {
		return enrichNetworkDirection(event)
	}
	return enrichNetworkEvent(event)
}

func (t *networkTracker) remember(event model.Event, observedAt time.Time) {
	if event.Process == nil || event.Network == nil {
		return
	}

	processFDKey, hasProcessFDKey := networkTrackerKey(event.Process, event.Data)
	tupleKey, hasTupleKey := networkTupleKey(event.Network)
	if !hasProcessFDKey && !hasTupleKey {
		return
	}

	t.sequence++
	entry := t.acquireEntry()
	*entry = trackedNetwork{
		process:   *event.Process,
		network:   *event.Network,
		openedAt:  observedAt,
		updatedAt: observedAt,
		sequence:  t.sequence,
	}
	if t.detailedEnrichment {
		entry.data = event.Data
	}
	if strings.TrimSpace(entry.network.ConnectionState) == "" {
		entry.network.ConnectionState = "connected"
	}
	entry.stateChanged = entry.openedAt
	if hasProcessFDKey {
		entry.processFDKey = processFDKey
		entry.hasProcessFDKey = true
		if old := t.byProcessFD[processFDKey]; old != nil {
			t.deleteEntry(old)
		}
		t.byProcessFD[processFDKey] = entry
	}
	if hasTupleKey {
		entry.tupleKey = tupleKey
		entry.hasTupleKey = true
		if old := t.byTuple[tupleKey]; old != nil {
			t.deleteEntry(old)
		}
		t.byTuple[tupleKey] = entry
	}
	t.order = append(t.order, trackedNetworkOrder{
		entry:    entry,
		sequence: t.sequence,
		lastSeen: observedAt,
	})
	t.prune(observedAt)
}

func (t *networkTracker) enrichState(event model.Event, observedAt time.Time) model.Event {
	if t == nil || event.Network == nil {
		return event
	}

	tupleKey, ok := networkTupleKey(event.Network)
	if !ok {
		return event
	}

	cached, exists := t.byTuple[tupleKey]
	if !exists || cached == nil {
		return event
	}

	event.Process = enrichNetworkProcess(event.Process, cached.process)
	event.Network = enrichStateNetwork(event.Network, cached.network)
	if !t.detailedEnrichment {
		cached.network = *event.Network
		cached.updatedAt = observedAt
		return event
	}
	event.Data = mergeEventData(cached.data, event.Data)
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	if previousState := strings.TrimSpace(cached.network.ConnectionState); previousState != "" {
		event.Data["previous_connection_state"] = previousState
	}
	applyNetworkLifecycleProgress(event.Data, cached, event.Network, event.Timestamp)
	cached.network = *event.Network
	cached.data = event.Data
	cached.updatedAt = observedAt
	return event
}

func (t *networkTracker) enrichClose(event model.Event) model.Event {
	key, ok := networkTrackerKey(event.Process, event.Data)
	if !ok {
		return event
	}

	cached, exists := t.byProcessFD[key]
	if !exists || cached == nil {
		if event.Network != nil && strings.TrimSpace(event.Network.ConnectionState) == "" {
			event.Network.ConnectionState = "closed"
		}
		return event
	}

	event.Process = enrichNetworkProcess(event.Process, cached.process)
	event.Network = enrichCloseNetwork(event.Network, cached.network)
	if !t.detailedEnrichment {
		return event
	}
	event.Data = mergeEventData(cached.data, event.Data)
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	if previousState := strings.TrimSpace(cached.network.ConnectionState); previousState != "" {
		event.Data["previous_connection_state"] = previousState
	}
	applyNetworkLifecycleProgress(event.Data, cached, event.Network, event.Timestamp)
	return event
}

func (t *networkTracker) forget(event model.Event) {
	key, ok := networkTrackerKey(event.Process, event.Data)
	if !ok {
		return
	}
	entry, exists := t.byProcessFD[key]
	if !exists || entry == nil {
		delete(t.byProcessFD, key)
		return
	}
	t.deleteEntry(entry)
}

func (t *networkTracker) forgetProcess(process *model.Process) {
	if process == nil || process.PID <= 0 {
		return
	}
	for key, entry := range t.byProcessFD {
		if key.pid == process.PID {
			t.deleteEntry(entry)
		}
	}
	t.compactOrderIfStale()
}

func networkTrackerKey(process *model.Process, data map[string]any) (networkProcessFDKey, bool) {
	if process == nil || process.PID <= 0 {
		return networkProcessFDKey{}, false
	}
	fd, ok := networkFDFromData(data)
	if !ok || fd < 0 {
		return networkProcessFDKey{}, false
	}
	return networkProcessFDKey{pid: process.PID, fd: fd}, true
}

func networkTupleKey(network *model.Network) (networkTupleCacheKey, bool) {
	if network == nil {
		return networkTupleCacheKey{}, false
	}

	protocol := strings.TrimSpace(strings.ToLower(network.Protocol))
	sourceAddress := strings.TrimSpace(network.SourceAddress)
	destAddress := strings.TrimSpace(network.DestAddress)
	if protocol == "" || sourceAddress == "" || destAddress == "" || network.SourcePort <= 0 || network.DestPort <= 0 {
		return networkTupleCacheKey{}, false
	}
	return networkTupleCacheKey{
		protocol:      protocol,
		sourceAddress: sourceAddress,
		sourcePort:    network.SourcePort,
		destAddress:   destAddress,
		destPort:      network.DestPort,
	}, true
}

func networkFDFromData(data map[string]any) (int, bool) {
	if len(data) == 0 {
		return 0, false
	}

	switch value := data["fd"].(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case uint32:
		return int(value), true
	case uint64:
		return int(value), true
	case float64:
		return int(value), true
	default:
		return 0, false
	}
}

func enrichNetworkProcess(current *model.Process, cached model.Process) *model.Process {
	if current == nil {
		cloned := cached
		return &cloned
	}

	if current.ParentPID == 0 {
		current.ParentPID = cached.ParentPID
	}
	if strings.TrimSpace(current.Name) == "" {
		current.Name = cached.Name
	}
	if strings.TrimSpace(current.Username) == "" {
		current.Username = cached.Username
	}
	if strings.TrimSpace(current.Image) == "" {
		current.Image = cached.Image
	}
	if strings.TrimSpace(current.Command) == "" {
		current.Command = cached.Command
	}
	if strings.TrimSpace(current.ParentName) == "" {
		current.ParentName = cached.ParentName
	}
	if strings.TrimSpace(current.ParentImage) == "" {
		current.ParentImage = cached.ParentImage
	}
	if strings.TrimSpace(current.ParentCommand) == "" {
		current.ParentCommand = cached.ParentCommand
	}
	return current
}

func enrichCloseNetwork(current *model.Network, cached model.Network) *model.Network {
	if current == nil {
		cloned := cached
		cloned.ConnectionState = "closed"
		return &cloned
	}

	if strings.TrimSpace(current.Protocol) == "" {
		current.Protocol = cached.Protocol
	}
	if strings.TrimSpace(current.SourceAddress) == "" {
		current.SourceAddress = cached.SourceAddress
	}
	if strings.TrimSpace(current.DestAddress) == "" {
		current.DestAddress = cached.DestAddress
	}
	if current.SourcePort == 0 {
		current.SourcePort = cached.SourcePort
	}
	if current.DestPort == 0 {
		current.DestPort = cached.DestPort
	}
	current.ConnectionState = "closed"
	return current
}

func enrichStateNetwork(current *model.Network, cached model.Network) *model.Network {
	if current == nil {
		cloned := cached
		return &cloned
	}

	if strings.TrimSpace(current.Protocol) == "" {
		current.Protocol = cached.Protocol
	}
	if strings.TrimSpace(current.SourceAddress) == "" {
		current.SourceAddress = cached.SourceAddress
	}
	if strings.TrimSpace(current.DestAddress) == "" {
		current.DestAddress = cached.DestAddress
	}
	if current.SourcePort == 0 {
		current.SourcePort = cached.SourcePort
	}
	if current.DestPort == 0 {
		current.DestPort = cached.DestPort
	}
	if strings.TrimSpace(current.ConnectionState) == "" {
		current.ConnectionState = cached.ConnectionState
	}
	if strings.TrimSpace(current.Direction) == "" {
		current.Direction = cached.Direction
	}
	return current
}

func mergeEventData(parts ...map[string]any) map[string]any {
	mergeIndex := -1
	for index := len(parts) - 1; index >= 0; index-- {
		if len(parts[index]) > 0 {
			mergeIndex = index
			break
		}
	}
	if mergeIndex < 0 {
		return nil
	}

	merged := parts[mergeIndex]
	for index := mergeIndex - 1; index >= 0; index-- {
		for key, value := range parts[index] {
			if _, exists := merged[key]; !exists {
				merged[key] = value
			}
		}
	}
	return merged
}

func initializeNetworkLifecycle(event model.Event) model.Event {
	if event.Network == nil {
		return event
	}
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	observedAt := normalizedEventTimestamp(event.Timestamp)
	event.Data["connection_opened_at_unix"] = observedAt.Unix()
	event.Data["state_changed_at_unix"] = observedAt.Unix()
	event.Data["connection_age_seconds"] = int64(0)
	event.Data["state_age_seconds"] = int64(0)
	event.Data["previous_state_age_seconds"] = int64(0)
	if _, exists := event.Data["previous_connection_state"]; !exists {
		event.Data["previous_connection_state"] = ""
	}
	return event
}

func applyNetworkLifecycleProgress(
	data map[string]any,
	cached *trackedNetwork,
	current *model.Network,
	timestamp time.Time,
) {
	if len(data) == 0 || cached == nil {
		return
	}

	observedAt := normalizedEventTimestamp(timestamp)
	if cached.openedAt.IsZero() {
		cached.openedAt = observedAt
	}
	if cached.stateChanged.IsZero() {
		cached.stateChanged = cached.openedAt
	}

	connectionAge := durationSeconds(cached.openedAt, observedAt)
	previousStateAge := durationSeconds(cached.stateChanged, observedAt)
	data["connection_opened_at_unix"] = cached.openedAt.Unix()
	data["connection_age_seconds"] = connectionAge
	data["previous_state_age_seconds"] = previousStateAge

	previousState := strings.TrimSpace(cached.network.ConnectionState)
	currentState := previousState
	if current != nil && strings.TrimSpace(current.ConnectionState) != "" {
		currentState = strings.TrimSpace(current.ConnectionState)
	}
	if currentState != previousState {
		cached.stateChanged = observedAt
		data["state_age_seconds"] = int64(0)
	} else {
		data["state_age_seconds"] = previousStateAge
	}
	data["state_changed_at_unix"] = cached.stateChanged.Unix()
}

func normalizedEventTimestamp(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func durationSeconds(start time.Time, end time.Time) int64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return int64(end.Sub(start).Seconds())
}

func (t *networkTracker) deleteEntry(entry *trackedNetwork) {
	if t == nil || entry == nil {
		return
	}
	if entry.hasProcessFDKey {
		delete(t.byProcessFD, entry.processFDKey)
	}
	if entry.hasTupleKey {
		delete(t.byTuple, entry.tupleKey)
	}
	t.releaseEntry(entry)
	t.compactOrderIfStale()
}

func (t *networkTracker) acquireEntry() *trackedNetwork {
	if t == nil || len(t.freeEntries) == 0 {
		return &trackedNetwork{}
	}
	lastIndex := len(t.freeEntries) - 1
	entry := t.freeEntries[lastIndex]
	t.freeEntries[lastIndex] = nil
	t.freeEntries = t.freeEntries[:lastIndex]
	return entry
}

func (t *networkTracker) releaseEntry(entry *trackedNetwork) {
	if t == nil || entry == nil || t.entryIsActive(entry) {
		return
	}
	if t.maxEntries > 0 && len(t.freeEntries) >= t.maxEntries {
		return
	}
	*entry = trackedNetwork{}
	t.freeEntries = append(t.freeEntries, entry)
}

func (t *networkTracker) prune(observedAt time.Time) {
	if t == nil {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	if t.maxEntries > 0 &&
		len(t.byProcessFD) <= t.maxEntries &&
		len(t.byTuple) <= t.maxEntries &&
		!t.nextPrune.IsZero() &&
		observedAt.Before(t.nextPrune) {
		return
	}
	if t.window > 0 {
		for _, entry := range t.byProcessFD {
			lastSeen := entry.updatedAt
			if lastSeen.IsZero() {
				lastSeen = entry.openedAt
			}
			if !lastSeen.IsZero() && observedAt.Sub(lastSeen) > t.window {
				t.deleteEntry(entry)
			}
		}
		for _, entry := range t.byTuple {
			if entry == nil || entry.hasProcessFDKey {
				continue
			}
			lastSeen := entry.updatedAt
			if lastSeen.IsZero() {
				lastSeen = entry.openedAt
			}
			if !lastSeen.IsZero() && observedAt.Sub(lastSeen) > t.window {
				t.deleteEntry(entry)
			}
		}
	}
	for t.maxEntries > 0 && t.uniqueEntryCount() > t.maxEntries {
		if !t.evictOverflow() {
			return
		}
	}
	t.nextPrune = observedAt.Add(shortTermContextPruneInterval)
	t.compactOrderIfStale()
}

func (t *networkTracker) uniqueEntryCount() int {
	if t == nil {
		return 0
	}
	count := len(t.byProcessFD)
	for _, entry := range t.byTuple {
		if entry != nil && !entry.hasProcessFDKey {
			count++
		}
	}
	return count
}

func (t *networkTracker) uniqueEntries() []*trackedNetwork {
	if t == nil {
		return nil
	}
	seen := make(map[*trackedNetwork]struct{}, len(t.byProcessFD)+len(t.byTuple))
	entries := make([]*trackedNetwork, 0, len(t.byProcessFD)+len(t.byTuple))
	for _, entry := range t.byProcessFD {
		if entry == nil {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		entries = append(entries, entry)
	}
	for _, entry := range t.byTuple {
		if entry == nil {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		entries = append(entries, entry)
	}
	return entries
}

func (t *networkTracker) evictOverflow() bool {
	if t == nil || t.maxEntries <= 0 {
		return false
	}
	for len(t.order) > 0 {
		candidate := t.order[0]
		t.order = t.order[1:]
		entry := candidate.entry
		if entry == nil || entry.sequence != candidate.sequence || !t.entryIsActive(entry) {
			continue
		}
		t.deleteEntry(entry)
		return true
	}
	t.compactOrder()
	if len(t.order) == 0 {
		return false
	}
	return t.evictOverflow()
}

func (t *networkTracker) entryIsActive(entry *trackedNetwork) bool {
	if t == nil || entry == nil {
		return false
	}
	if entry.hasProcessFDKey {
		return t.byProcessFD[entry.processFDKey] == entry
	}
	if entry.hasTupleKey {
		return t.byTuple[entry.tupleKey] == entry
	}
	return false
}

func (t *networkTracker) compactOrderIfStale() {
	if t == nil || len(t.order) == 0 {
		return
	}
	threshold := processTrackerOrderCompactThreshold(t.uniqueEntryCount(), t.maxEntries)
	if threshold <= 0 || len(t.order) <= threshold {
		return
	}
	t.compactOrder()
}

func (t *networkTracker) compactOrder() {
	if t == nil {
		return
	}
	count := t.uniqueEntryCount()
	if count == 0 {
		t.order = nil
		return
	}
	if cap(t.order) > processTrackerOrderCompactThreshold(count, t.maxEntries)*2 {
		t.order = make([]trackedNetworkOrder, 0, count)
	} else {
		t.order = t.order[:0]
	}
	for _, entry := range t.byProcessFD {
		if entry == nil {
			continue
		}
		lastSeen := entry.updatedAt
		if lastSeen.IsZero() {
			lastSeen = entry.openedAt
		}
		t.order = append(t.order, trackedNetworkOrder{
			entry:    entry,
			sequence: entry.sequence,
			lastSeen: lastSeen,
		})
	}
	for _, entry := range t.byTuple {
		if entry == nil || entry.hasProcessFDKey {
			continue
		}
		lastSeen := entry.updatedAt
		if lastSeen.IsZero() {
			lastSeen = entry.openedAt
		}
		t.order = append(t.order, trackedNetworkOrder{
			entry:    entry,
			sequence: entry.sequence,
			lastSeen: lastSeen,
		})
	}
	sortTrackedNetworkOrder(t.order)
}

func sortTrackedNetworkOrder(entries []trackedNetworkOrder) {
	sort.Slice(entries, func(left, right int) bool {
		leftSeen := entries[left].lastSeen
		rightSeen := entries[right].lastSeen
		if leftSeen.Equal(rightSeen) {
			return entries[left].sequence < entries[right].sequence
		}
		return leftSeen.Before(rightSeen)
	})
}
