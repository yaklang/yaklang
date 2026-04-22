//go:build hids && linux

package scannode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	hidsmodel "github.com/yaklang/yaklang/common/hids/model"
	hidsruntime "github.com/yaklang/yaklang/common/hids/runtime"
	"github.com/yaklang/yaklang/common/log"
)

type hidsCapabilityHooks struct {
	once         sync.Once
	applyMu      sync.Mutex
	mu           sync.RWMutex
	manager      *hidsruntime.Manager
	alerts       chan CapabilityRuntimeAlert
	observations chan CapabilityRuntimeObservation
	config       hidsAlertConfig
	appliedSpec  []byte
}

type hidsAlertConfig struct {
	capabilityKey            string
	specVersion              string
	emitCapabilityStatus     bool
	emitCapabilityAlert      bool
	emitSnapshotObservations bool
}

const (
	hidsSnapshotObservationFlushInterval = 2 * time.Second
	hidsSnapshotObservationMinInterval   = 2 * time.Minute
	hidsSnapshotObservationMaxPending    = 2048
)

func newCapabilityHIDSHooks() capabilityHIDSHooks {
	return &hidsCapabilityHooks{
		alerts:       make(chan CapabilityRuntimeAlert, 64),
		observations: make(chan CapabilityRuntimeObservation, 2048),
	}
}

func (h *hidsCapabilityHooks) Apply(
	m *CapabilityManager,
	input capabilityHIDSApplyInput,
) (CapabilityApplyResult, error) {
	h.applyMu.Lock()
	defer h.applyMu.Unlock()

	spec, err := hidsmodel.ParseDesiredSpec(input.DesiredSpec)
	if err != nil {
		return CapabilityApplyResult{}, fmt.Errorf("%w: %v", ErrInvalidHIDSCapabilitySpec, err)
	}
	normalizedSpec, err := json.Marshal(spec)
	if err != nil {
		return CapabilityApplyResult{}, fmt.Errorf("marshal normalized hids desired spec: %w", err)
	}

	previousConfig := h.alertConfig()
	h.setAlertConfig(hidsAlertConfig{
		capabilityKey:            input.CapabilityKey,
		specVersion:              input.SpecVersion,
		emitCapabilityStatus:     spec.Reporting.EmitCapabilityStatus,
		emitCapabilityAlert:      spec.Reporting.EmitCapabilityAlert,
		emitSnapshotObservations: spec.Reporting.ShouldEmitSnapshotObservations(),
	})
	result, reused := h.tryReuseRunningRuntime(normalizedSpec)
	if !reused {
		result, err = h.runtimeManager().Apply(m.rootCtx, spec)
		if err != nil {
			h.setAlertConfig(previousConfig)
			var validationErr *hidsmodel.ValidationError
			if errors.As(err, &validationErr) {
				return CapabilityApplyResult{}, fmt.Errorf("%w: %v", ErrInvalidHIDSCapabilitySpec, validationErr)
			}
			return CapabilityApplyResult{}, err
		}
	}

	now := result.State.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	document := capabilityDocument{
		NodeID:        m.nodeID,
		CapabilityKey: input.CapabilityKey,
		SpecVersion:   input.SpecVersion,
		DesiredSpec:   cloneBytes(normalizedSpec),
		StoredAt:      now,
	}
	if err := m.save(document); err != nil {
		return CapabilityApplyResult{}, fmt.Errorf("persist capability spec: %w", err)
	}
	h.setAppliedSpec(normalizedSpec)

	status, detailJSON := normalizeCapabilityEventStatus(
		result.State.Status,
		marshalCapabilityStatusDetail(result.State.Detail),
	)
	message := strings.TrimSpace(result.State.Message)
	if message == "" {
		message = "hids runtime applied"
	}
	return CapabilityApplyResult{
		CapabilityKey:    input.CapabilityKey,
		SpecVersion:      input.SpecVersion,
		Status:           status,
		Message:          message,
		StatusDetailJSON: detailJSON,
		ObservedAt:       now,
	}, nil
}

func (h *hidsCapabilityHooks) Close() error {
	if h == nil || h.manager == nil {
		return nil
	}
	h.setAppliedSpec(nil)
	return h.manager.Close()
}

func (h *hidsCapabilityHooks) Alerts() <-chan CapabilityRuntimeAlert {
	if h == nil {
		return nil
	}
	return h.alerts
}

func (h *hidsCapabilityHooks) Observations() <-chan CapabilityRuntimeObservation {
	if h == nil {
		return nil
	}
	return h.observations
}

func (h *hidsCapabilityHooks) CurrentStatus() (CapabilityRuntimeStatus, bool) {
	if h == nil || h.manager == nil {
		return CapabilityRuntimeStatus{}, false
	}

	config := h.alertConfig()
	if !config.emitCapabilityStatus || strings.TrimSpace(config.capabilityKey) == "" {
		return CapabilityRuntimeStatus{}, false
	}

	state := h.manager.State()
	return h.convertStatus(state, config)
}

func (h *hidsCapabilityHooks) OnSessionReady(ctx context.Context) error {
	if h == nil || h.manager == nil {
		return nil
	}

	replayCtx, cancel := context.WithTimeout(rootContextOrBackground(ctx), 20*time.Second)
	go func() {
		defer cancel()
		if err := h.manager.ReplayInventory(replayCtx); err != nil {
			log.Warnf("replay hids inventory after session restore failed: %v", err)
		}
	}()
	return nil
}

func (h *hidsCapabilityHooks) runtimeManager() *hidsruntime.Manager {
	h.once.Do(func() {
		h.manager = hidsruntime.NewManager()
		go h.forwardAlerts(h.manager.Alerts())
		go h.forwardObservations(h.manager.Observations())
	})
	return h.manager
}

func (h *hidsCapabilityHooks) forwardAlerts(alerts <-chan hidsmodel.Alert) {
	if h == nil || alerts == nil {
		return
	}
	for alert := range alerts {
		capabilityAlert, ok := h.convertAlert(alert)
		if !ok {
			continue
		}
		select {
		case h.alerts <- capabilityAlert:
		default:
		}
	}
}

func (h *hidsCapabilityHooks) forwardObservations(observations <-chan hidsmodel.Event) {
	if h == nil || observations == nil {
		return
	}

	state := newHIDSSnapshotObservationState()
	ticker := time.NewTicker(hidsSnapshotObservationFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-observations:
			if !ok {
				h.flushPendingObservations(state)
				return
			}
			config := h.alertConfig()
			observation, key, ok := h.convertObservation(event, config, state)
			if !ok {
				continue
			}
			state.pending[key] = observation
			if len(state.pending) >= hidsSnapshotObservationMaxPending {
				h.flushPendingObservations(state)
			}
		case <-ticker.C:
			h.flushPendingObservations(state)
		}
	}
}

func newHIDSSnapshotObservationState() *hidsSnapshotObservationState {
	return &hidsSnapshotObservationState{
		pending:       make(map[string]CapabilityRuntimeObservation),
		lastPublished: make(map[string]time.Time),
	}
}

type hidsSnapshotObservationState struct {
	pending       map[string]CapabilityRuntimeObservation
	lastPublished map[string]time.Time
}

func (h *hidsCapabilityHooks) convertObservation(
	event hidsmodel.Event,
	config hidsAlertConfig,
	state *hidsSnapshotObservationState,
) (CapabilityRuntimeObservation, string, bool) {
	if !config.emitSnapshotObservations ||
		strings.TrimSpace(config.capabilityKey) == "" ||
		state == nil {
		return CapabilityRuntimeObservation{}, "", false
	}

	key, ok := hidsSnapshotObservationKey(event)
	if !ok {
		return CapabilityRuntimeObservation{}, "", false
	}
	if hidsSnapshotObservationIsTerminal(event) {
		if _, exists := state.pending[key]; !exists {
			if _, exists := state.lastPublished[key]; !exists {
				return CapabilityRuntimeObservation{}, "", false
			}
		}
	} else if hidsSnapshotObservationIsInventory(event) {
		if lastPublished, exists := state.lastPublished[key]; exists &&
			time.Since(lastPublished) < hidsSnapshotObservationMinInterval {
			return CapabilityRuntimeObservation{}, "", false
		}
	}

	observedAt := event.Timestamp.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
		event.Timestamp = observedAt
	}
	raw, err := json.Marshal(event)
	if err != nil {
		log.Warnf("marshal hids platform snapshot observation failed: type=%s err=%v", event.Type, err)
		return CapabilityRuntimeObservation{}, "", false
	}

	return CapabilityRuntimeObservation{
		CapabilityKey: config.capabilityKey,
		SpecVersion:   config.specVersion,
		HIDSEventType: event.Type,
		EventJSON:     raw,
		ObservedAt:    observedAt,
	}, key, true
}

func (h *hidsCapabilityHooks) flushPendingObservations(state *hidsSnapshotObservationState) {
	if h == nil || state == nil || len(state.pending) == 0 {
		return
	}
	for key, observation := range state.pending {
		select {
		case h.observations <- observation:
			state.lastPublished[key] = time.Now().UTC()
			delete(state.pending, key)
		default:
			return
		}
	}
}

func hidsSnapshotObservationKey(event hidsmodel.Event) (string, bool) {
	switch event.Type {
	case hidsmodel.EventTypeProcessExec, hidsmodel.EventTypeProcessExit, hidsmodel.EventTypeProcessState:
		if event.Process == nil {
			return "", false
		}
		if event.Process.PID > 0 && event.Process.StartTimeUnixMillis > 0 {
			return nonEmptyObservationKey(
				"process",
				event.Process.BootID,
				fmt.Sprintf("%d", event.Process.PID),
				fmt.Sprintf("%d", event.Process.StartTimeUnixMillis),
			)
		}
		if event.Process.PID > 0 {
			return nonEmptyObservationKey("process", fmt.Sprintf("%d", event.Process.PID))
		}
		return nonEmptyObservationKey(
			"process",
			event.Process.Image,
			event.Process.Command,
			event.Process.ParentName,
		)
	case hidsmodel.EventTypeNetworkAccept,
		hidsmodel.EventTypeNetworkConnect,
		hidsmodel.EventTypeNetworkState,
		hidsmodel.EventTypeNetworkClose,
		hidsmodel.EventTypeNetworkSocket:
		if event.Network == nil {
			return "", false
		}
		return nonEmptyObservationKey(
			"network",
			processBootID(event.Process),
			processPIDKey(event.Process),
			processStartTimeKey(event.Process),
			fmt.Sprintf("%d", event.Network.FD),
			event.Network.SourceAddress,
			fmt.Sprintf("%d", event.Network.SourcePort),
			event.Network.DestAddress,
			fmt.Sprintf("%d", event.Network.DestPort),
			event.Network.Protocol,
			event.Network.Direction,
		)
	case hidsmodel.EventTypeFileChange:
		if event.File == nil {
			return "", false
		}
		return nonEmptyObservationKey("file", event.File.Path)
	case hidsmodel.EventTypeHostUsers:
		return nonEmptyObservationKey("host-users", "inventory")
	default:
		return "", false
	}
}

func processBootID(process *hidsmodel.Process) string {
	if process == nil {
		return ""
	}
	return process.BootID
}

func processPIDKey(process *hidsmodel.Process) string {
	if process == nil || process.PID <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", process.PID)
}

func processStartTimeKey(process *hidsmodel.Process) string {
	if process == nil || process.StartTimeUnixMillis <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", process.StartTimeUnixMillis)
}

func nonEmptyObservationKey(prefix string, values ...string) (string, bool) {
	parts := make([]string, 0, len(values)+1)
	parts = append(parts, strings.TrimSpace(prefix))
	hasValue := false
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" && trimmed != "0" {
			hasValue = true
		}
		parts = append(parts, strings.ToLower(trimmed))
	}
	if !hasValue {
		return "", false
	}
	return strings.Join(parts, "\x00"), true
}

func hidsSnapshotObservationIsTerminal(event hidsmodel.Event) bool {
	switch event.Type {
	case hidsmodel.EventTypeProcessExit, hidsmodel.EventTypeNetworkClose:
		return true
	case hidsmodel.EventTypeFileChange:
		if event.File == nil {
			return false
		}
		operation := strings.ToUpper(strings.TrimSpace(event.File.Operation))
		return strings.Contains(operation, "REMOVE") ||
			strings.Contains(operation, "DELETE") ||
			strings.Contains(operation, "UNLINK")
	default:
		return false
	}
}

func hidsSnapshotObservationIsInventory(event hidsmodel.Event) bool {
	if strings.HasPrefix(strings.TrimSpace(event.Source), "inventory.") {
		return true
	}
	for _, tag := range event.Tags {
		if strings.EqualFold(strings.TrimSpace(tag), "inventory") {
			return true
		}
	}
	return false
}

func (h *hidsCapabilityHooks) convertAlert(alert hidsmodel.Alert) (CapabilityRuntimeAlert, bool) {
	config := h.alertConfig()
	if !config.emitCapabilityAlert {
		return CapabilityRuntimeAlert{}, false
	}

	detail := map[string]any{
		"rule_id": alert.RuleID,
		"tags":    cloneStringSlice(alert.Tags),
	}
	if !alert.ObservedAt.IsZero() {
		detail["observed_at"] = alert.ObservedAt.UTC().Format(time.RFC3339Nano)
	}
	for key, value := range alert.Detail {
		detail[key] = value
	}

	raw, err := json.Marshal(detail)
	if err != nil {
		log.Warnf("marshal hids capability alert detail failed: rule_id=%s err=%v", alert.RuleID, err)
		return CapabilityRuntimeAlert{}, false
	}

	observedAt := alert.ObservedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	return CapabilityRuntimeAlert{
		CapabilityKey: config.capabilityKey,
		SpecVersion:   config.specVersion,
		RuleID:        alert.RuleID,
		Severity:      alert.Severity,
		Title:         strings.TrimSpace(alert.Title),
		DetailJSON:    raw,
		ObservedAt:    observedAt,
	}, true
}

func (h *hidsCapabilityHooks) alertConfig() hidsAlertConfig {
	if h == nil {
		return hidsAlertConfig{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config
}

func (h *hidsCapabilityHooks) setAlertConfig(config hidsAlertConfig) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.config = config
}

func (h *hidsCapabilityHooks) setAppliedSpec(spec []byte) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.appliedSpec = cloneBytes(spec)
}

func (h *hidsCapabilityHooks) tryReuseRunningRuntime(
	normalizedSpec []byte,
) (hidsruntime.ApplyResult, bool) {
	if h == nil || h.manager == nil {
		return hidsruntime.ApplyResult{}, false
	}

	h.mu.RLock()
	appliedSpec := cloneBytes(h.appliedSpec)
	h.mu.RUnlock()
	if !bytes.Equal(appliedSpec, normalizedSpec) {
		return hidsruntime.ApplyResult{}, false
	}

	state := h.manager.State()
	if !isReusableHIDSRuntimeStatus(state.Status) {
		return hidsruntime.ApplyResult{}, false
	}
	state.Message = "hids runtime already running; desired spec unchanged"
	state.UpdatedAt = time.Now().UTC()
	return hidsruntime.ApplyResult{State: state}, true
}

func isReusableHIDSRuntimeStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case capabilityStatusRunning, "degraded":
		return true
	default:
		return false
	}
}

func (h *hidsCapabilityHooks) convertStatus(
	state hidsmodel.RuntimeState,
	config hidsAlertConfig,
) (CapabilityRuntimeStatus, bool) {
	status, detailJSON := normalizeCapabilityEventStatus(
		state.Status,
		marshalCapabilityStatusDetail(state.Detail),
	)

	message := strings.TrimSpace(state.Message)
	observedAt := state.UpdatedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	return CapabilityRuntimeStatus{
		CapabilityKey: config.capabilityKey,
		SpecVersion:   config.specVersion,
		Status:        status,
		Message:       message,
		DetailJSON:    detailJSON,
		ObservedAt:    observedAt,
	}, true
}

func marshalCapabilityStatusDetail(detail map[string]any) []byte {
	if len(detail) == 0 {
		return nil
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		log.Warnf("marshal hids capability status detail failed: err=%v", err)
		return nil
	}
	return raw
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
