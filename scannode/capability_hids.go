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
	once        sync.Once
	applyMu     sync.Mutex
	mu          sync.RWMutex
	manager     *hidsruntime.Manager
	alerts      chan CapabilityRuntimeAlert
	config      hidsAlertConfig
	appliedSpec []byte
}

type hidsAlertConfig struct {
	capabilityKey        string
	specVersion          string
	emitCapabilityStatus bool
	emitCapabilityAlert  bool
}

func newCapabilityHIDSHooks() capabilityHIDSHooks {
	return &hidsCapabilityHooks{
		alerts: make(chan CapabilityRuntimeAlert, 64),
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
		capabilityKey:        input.CapabilityKey,
		specVersion:          input.SpecVersion,
		emitCapabilityStatus: spec.Reporting.EmitCapabilityStatus,
		emitCapabilityAlert:  spec.Reporting.EmitCapabilityAlert,
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
