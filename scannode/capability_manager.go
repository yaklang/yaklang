package scannode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/yaklang/yaklang/common/consts"
)

const (
	capabilityDefaultSpecVersion = "v1"
	capabilityStatusStored       = "stored"
	capabilityStatusRunning      = "running"
	capabilityStoredMessage      = "desired spec persisted locally; runtime hook is not wired yet"
)

var (
	ErrInvalidCapabilityKey              = errors.New("capability_key is required")
	ErrInvalidCapabilitySpec             = errors.New("desired_spec_json must be valid JSON")
	ErrInvalidHIDSCapabilitySpec         = errors.New("hids desired spec is invalid")
	ErrHIDSCapabilityNotCompiled         = errors.New("hids capability is not compiled into this binary")
	ErrHIDSCapabilityUnsupportedPlatform = errors.New("hids capability is only supported on linux")
)

type CapabilityManagerConfig struct {
	NodeID      string
	BaseDir     string
	RootContext context.Context
}

type CapabilityApplyInput struct {
	CapabilityKey   string
	SpecVersion     string
	DesiredSpecJSON []byte
}

type CapabilityApplyResult struct {
	CapabilityKey    string
	SpecVersion      string
	Status           string
	Message          string
	StatusDetailJSON []byte
	ObservedAt       time.Time
}

type CapabilityRuntimeAlert struct {
	CapabilityKey string
	SpecVersion   string
	RuleID        string
	Severity      string
	Title         string
	DetailJSON    []byte
	ObservedAt    time.Time
}

type CapabilityRuntimeObservation struct {
	CapabilityKey string
	SpecVersion   string
	EventType     string
	EventJSON     []byte
	ObservedAt    time.Time
}

type CapabilityRuntimeStatus struct {
	CapabilityKey string
	SpecVersion   string
	Status        string
	Message       string
	DetailJSON    []byte
	ObservedAt    time.Time
}

type CapabilityManager struct {
	nodeID       string
	storeDir     string
	rootCtx      context.Context
	hidsHooks    capabilityHIDSHooks
	alerts       chan CapabilityRuntimeAlert
	observations chan CapabilityRuntimeObservation
}

type capabilityDocument struct {
	NodeID        string          `json:"node_id"`
	CapabilityKey string          `json:"capability_key"`
	SpecVersion   string          `json:"spec_version"`
	DesiredSpec   json.RawMessage `json:"desired_spec"`
	StoredAt      time.Time       `json:"stored_at"`
}

type capabilityHIDSApplyInput struct {
	CapabilityKey string
	SpecVersion   string
	DesiredSpec   json.RawMessage
}

type capabilityHIDSHooks interface {
	Apply(m *CapabilityManager, input capabilityHIDSApplyInput) (CapabilityApplyResult, error)
	Alerts() <-chan CapabilityRuntimeAlert
	Observations() <-chan CapabilityRuntimeObservation
	CurrentStatus() (CapabilityRuntimeStatus, bool)
	OnSessionReady(context.Context) error
	Close() error
}

func newCapabilityManager(cfg CapabilityManagerConfig) *CapabilityManager {
	baseDir := strings.TrimSpace(cfg.BaseDir)
	if baseDir == "" {
		baseDir = consts.GetDefaultYakitBaseDir()
	}
	manager := &CapabilityManager{
		nodeID:       strings.TrimSpace(cfg.NodeID),
		storeDir:     filepath.Join(baseDir, "legion", "capabilities"),
		rootCtx:      rootContextOrBackground(cfg.RootContext),
		alerts:       make(chan CapabilityRuntimeAlert, 64),
		observations: make(chan CapabilityRuntimeObservation, 128),
	}
	manager.hidsHooks = newCapabilityHIDSHooks()
	go manager.forwardRuntimeAlerts(manager.hidsHooks.Alerts())
	go manager.forwardRuntimeObservations(manager.hidsHooks.Observations())
	return manager
}

func (m *CapabilityManager) Apply(input CapabilityApplyInput) (CapabilityApplyResult, error) {
	key, err := normalizeCapabilityKey(input.CapabilityKey)
	if err != nil {
		return CapabilityApplyResult{}, err
	}
	specVersion := normalizeCapabilitySpecVersion(input.SpecVersion)
	desiredSpec, err := normalizeCapabilitySpec(input.DesiredSpecJSON)
	if err != nil {
		return CapabilityApplyResult{}, err
	}
	if isHIDSCapabilityKey(key) {
		if m.hidsHooks == nil {
			return CapabilityApplyResult{}, ErrHIDSCapabilityNotCompiled
		}
		return m.hidsHooks.Apply(m, capabilityHIDSApplyInput{
			CapabilityKey: key,
			SpecVersion:   specVersion,
			DesiredSpec:   desiredSpec,
		})
	}

	now := time.Now().UTC()
	document := capabilityDocument{
		NodeID:        m.nodeID,
		CapabilityKey: key,
		SpecVersion:   specVersion,
		DesiredSpec:   desiredSpec,
		StoredAt:      now,
	}
	if err := m.save(document); err != nil {
		return CapabilityApplyResult{}, fmt.Errorf("persist capability spec: %w", err)
	}
	return CapabilityApplyResult{
		CapabilityKey: key,
		SpecVersion:   specVersion,
		Status:        capabilityStatusStored,
		Message:       capabilityStoredMessage,
		ObservedAt:    now,
	}, nil
}

func (m *CapabilityManager) RestorePersisted() ([]CapabilityApplyResult, error) {
	if m == nil {
		return nil, nil
	}

	paths, err := filepath.Glob(filepath.Join(m.storeDir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("list persisted capabilities: %w", err)
	}
	if len(paths) == 0 {
		return []CapabilityApplyResult{}, nil
	}

	results := make([]CapabilityApplyResult, 0, len(paths))
	for _, path := range paths {
		document, err := loadCapabilityDocument(path)
		if err != nil {
			return results, fmt.Errorf("load persisted capability %s: %w", filepath.Base(path), err)
		}
		if document.NodeID != "" && document.NodeID != m.nodeID {
			continue
		}
		if !isHIDSCapabilityKey(document.CapabilityKey) {
			continue
		}
		if m.hidsHooks == nil {
			return results, ErrHIDSCapabilityNotCompiled
		}

		result, err := m.hidsHooks.Apply(m, capabilityHIDSApplyInput{
			CapabilityKey: document.CapabilityKey,
			SpecVersion:   document.SpecVersion,
			DesiredSpec:   document.DesiredSpec,
		})
		if err != nil {
			if errors.Is(err, ErrHIDSCapabilityNotCompiled) ||
				errors.Is(err, ErrHIDSCapabilityUnsupportedPlatform) {
				continue
			}
			return results, fmt.Errorf("restore capability %s: %w", document.CapabilityKey, err)
		}
		results = append(results, result)
	}

	return results, nil
}

func (m *CapabilityManager) Close() error {
	if m == nil || m.hidsHooks == nil {
		return nil
	}
	return m.hidsHooks.Close()
}

func (m *CapabilityManager) Alerts() <-chan CapabilityRuntimeAlert {
	if m == nil {
		return nil
	}
	return m.alerts
}

func (m *CapabilityManager) Observations() <-chan CapabilityRuntimeObservation {
	if m == nil {
		return nil
	}
	return m.observations
}

func (m *CapabilityManager) RuntimeStatuses() []CapabilityRuntimeStatus {
	if m == nil || m.hidsHooks == nil {
		return nil
	}
	status, ok := m.hidsHooks.CurrentStatus()
	if !ok {
		return nil
	}
	return []CapabilityRuntimeStatus{status}
}

func (m *CapabilityManager) OnSessionReady(ctx context.Context) error {
	if m == nil || m.hidsHooks == nil {
		return nil
	}
	return m.hidsHooks.OnSessionReady(ctx)
}

func normalizeCapabilityKey(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ErrInvalidCapabilityKey
	}
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '.', '-', '_':
			continue
		default:
			return "", ErrInvalidCapabilityKey
		}
	}
	return trimmed, nil
}

func isHIDSCapabilityKey(value string) bool {
	return value == "hids" || strings.HasPrefix(value, "hids.")
}

func normalizeCapabilitySpecVersion(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return capabilityDefaultSpecVersion
	}
	return trimmed
}

func normalizeCapabilitySpec(value []byte) (json.RawMessage, error) {
	if len(value) == 0 {
		return json.RawMessage("{}"), nil
	}
	if !json.Valid(value) {
		return nil, ErrInvalidCapabilitySpec
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return json.RawMessage(cloned), nil
}

func (m *CapabilityManager) save(document capabilityDocument) error {
	if err := os.MkdirAll(m.storeDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath(document.CapabilityKey), raw, 0o644)
}

func (m *CapabilityManager) filePath(capabilityKey string) string {
	return filepath.Join(m.storeDir, capabilityKey+".json")
}

func loadCapabilityDocument(path string) (capabilityDocument, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return capabilityDocument{}, err
	}

	var document capabilityDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return capabilityDocument{}, err
	}

	key, err := normalizeCapabilityKey(document.CapabilityKey)
	if err != nil {
		return capabilityDocument{}, err
	}
	desiredSpec, err := normalizeCapabilitySpec(document.DesiredSpec)
	if err != nil {
		return capabilityDocument{}, err
	}
	document.NodeID = strings.TrimSpace(document.NodeID)
	document.CapabilityKey = key
	document.SpecVersion = normalizeCapabilitySpecVersion(document.SpecVersion)
	document.DesiredSpec = desiredSpec
	document.StoredAt = document.StoredAt.UTC()
	return document, nil
}

func rootContextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (m *CapabilityManager) forwardRuntimeAlerts(alerts <-chan CapabilityRuntimeAlert) {
	if m == nil || alerts == nil {
		return
	}
	for alert := range alerts {
		select {
		case m.alerts <- alert:
		default:
		}
	}
}

func (m *CapabilityManager) forwardRuntimeObservations(observations <-chan CapabilityRuntimeObservation) {
	if m == nil || observations == nil {
		return
	}
	for observation := range observations {
		select {
		case m.observations <- observation:
		default:
		}
	}
}
