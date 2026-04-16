//go:build hids && linux

package runtime

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

type networkTracker struct {
	byProcessFD map[string]*trackedNetwork
	byTuple     map[string]*trackedNetwork
}

type trackedNetwork struct {
	process      model.Process
	network      model.Network
	data         map[string]any
	openedAt     time.Time
	stateChanged time.Time
	processFDKey string
	tupleKey     string
}

func newNetworkTracker() *networkTracker {
	return &networkTracker{
		byProcessFD: make(map[string]*trackedNetwork),
		byTuple:     make(map[string]*trackedNetwork),
	}
}

func (t *networkTracker) Apply(event model.Event) model.Event {
	if t == nil {
		return event
	}

	switch event.Type {
	case model.EventTypeNetworkAccept:
		event = enrichNetworkEvent(event)
		event = initializeNetworkLifecycle(event)
		t.remember(event)
	case model.EventTypeNetworkConnect:
		event = enrichNetworkEvent(event)
		event = initializeNetworkLifecycle(event)
		t.remember(event)
	case model.EventTypeNetworkState:
		event = t.enrichState(event)
		event = enrichNetworkEvent(event)
	case model.EventTypeNetworkClose:
		event = t.enrichClose(event)
		event = enrichNetworkEvent(event)
		t.forget(event)
	case model.EventTypeProcessExit:
		t.forgetProcess(event.Process)
	}
	return event
}

func (t *networkTracker) remember(event model.Event) {
	if event.Process == nil || event.Network == nil {
		return
	}

	processFDKey, hasProcessFDKey := networkTrackerKey(event.Process, event.Data)
	tupleKey, hasTupleKey := networkTupleKey(event.Network)
	if !hasProcessFDKey && !hasTupleKey {
		return
	}

	entry := &trackedNetwork{
		process:  *event.Process,
		network:  *event.Network,
		data:     cloneAnyMap(event.Data),
		openedAt: normalizedEventTimestamp(event.Timestamp),
	}
	if strings.TrimSpace(entry.network.ConnectionState) == "" {
		entry.network.ConnectionState = "connected"
	}
	entry.stateChanged = entry.openedAt
	if hasProcessFDKey {
		entry.processFDKey = processFDKey
		t.byProcessFD[processFDKey] = entry
	}
	if hasTupleKey {
		entry.tupleKey = tupleKey
		t.byTuple[tupleKey] = entry
	}
}

func (t *networkTracker) enrichState(event model.Event) model.Event {
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
	event.Data = mergeEventData(cloneAnyMap(cached.data), event.Data)
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	if previousState := strings.TrimSpace(cached.network.ConnectionState); previousState != "" {
		event.Data["previous_connection_state"] = previousState
	}
	applyNetworkLifecycleProgress(event.Data, cached, event.Network, event.Timestamp)
	cached.network = *event.Network
	cached.data = cloneAnyMap(event.Data)
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
	event.Data = mergeEventData(cloneAnyMap(cached.data), event.Data)
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
	prefix := fmt.Sprintf("%d:", process.PID)
	for key, entry := range t.byProcessFD {
		if strings.HasPrefix(key, prefix) {
			t.deleteEntry(entry)
		}
	}
}

func networkTrackerKey(process *model.Process, data map[string]any) (string, bool) {
	if process == nil || process.PID <= 0 {
		return "", false
	}
	fd, ok := networkFDFromData(data)
	if !ok || fd < 0 {
		return "", false
	}
	return fmt.Sprintf("%d:%d", process.PID, fd), true
}

func networkTupleKey(network *model.Network) (string, bool) {
	if network == nil {
		return "", false
	}

	protocol := strings.TrimSpace(strings.ToLower(network.Protocol))
	sourceAddress := strings.TrimSpace(network.SourceAddress)
	destAddress := strings.TrimSpace(network.DestAddress)
	if protocol == "" || sourceAddress == "" || destAddress == "" || network.SourcePort <= 0 || network.DestPort <= 0 {
		return "", false
	}
	return fmt.Sprintf(
		"%s|%s|%d|%s|%d",
		protocol,
		sourceAddress,
		network.SourcePort,
		destAddress,
		network.DestPort,
	), true
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

	cloned := *current
	if cloned.ParentPID == 0 {
		cloned.ParentPID = cached.ParentPID
	}
	if strings.TrimSpace(cloned.Name) == "" {
		cloned.Name = cached.Name
	}
	if strings.TrimSpace(cloned.Username) == "" {
		cloned.Username = cached.Username
	}
	if strings.TrimSpace(cloned.Image) == "" {
		cloned.Image = cached.Image
	}
	if strings.TrimSpace(cloned.Command) == "" {
		cloned.Command = cached.Command
	}
	if strings.TrimSpace(cloned.ParentName) == "" {
		cloned.ParentName = cached.ParentName
	}
	return &cloned
}

func enrichCloseNetwork(current *model.Network, cached model.Network) *model.Network {
	if current == nil {
		cloned := cached
		cloned.ConnectionState = "closed"
		return &cloned
	}

	cloned := *current
	if strings.TrimSpace(cloned.Protocol) == "" {
		cloned.Protocol = cached.Protocol
	}
	if strings.TrimSpace(cloned.SourceAddress) == "" {
		cloned.SourceAddress = cached.SourceAddress
	}
	if strings.TrimSpace(cloned.DestAddress) == "" {
		cloned.DestAddress = cached.DestAddress
	}
	if cloned.SourcePort == 0 {
		cloned.SourcePort = cached.SourcePort
	}
	if cloned.DestPort == 0 {
		cloned.DestPort = cached.DestPort
	}
	cloned.ConnectionState = "closed"
	return &cloned
}

func enrichStateNetwork(current *model.Network, cached model.Network) *model.Network {
	if current == nil {
		cloned := cached
		return &cloned
	}

	cloned := *current
	if strings.TrimSpace(cloned.Protocol) == "" {
		cloned.Protocol = cached.Protocol
	}
	if strings.TrimSpace(cloned.SourceAddress) == "" {
		cloned.SourceAddress = cached.SourceAddress
	}
	if strings.TrimSpace(cloned.DestAddress) == "" {
		cloned.DestAddress = cached.DestAddress
	}
	if cloned.SourcePort == 0 {
		cloned.SourcePort = cached.SourcePort
	}
	if cloned.DestPort == 0 {
		cloned.DestPort = cached.DestPort
	}
	if strings.TrimSpace(cloned.ConnectionState) == "" {
		cloned.ConnectionState = cached.ConnectionState
	}
	return &cloned
}

func mergeEventData(parts ...map[string]any) map[string]any {
	var merged map[string]any
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		if merged == nil {
			merged = make(map[string]any, len(part))
		}
		for key, value := range part {
			merged[key] = value
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
	if entry.processFDKey != "" {
		delete(t.byProcessFD, entry.processFDKey)
	}
	if entry.tupleKey != "" {
		delete(t.byTuple, entry.tupleKey)
	}
}
