package scannode

import (
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
	capabilityStoredMessage      = "desired spec persisted locally; runtime hook is not wired yet"
)

var (
	ErrInvalidCapabilityKey  = errors.New("capability_key is required")
	ErrInvalidCapabilitySpec = errors.New("desired_spec_json must be valid JSON")
)

type CapabilityManagerConfig struct {
	NodeID  string
	BaseDir string
}

type CapabilityApplyInput struct {
	CapabilityKey   string
	SpecVersion     string
	DesiredSpecJSON []byte
}

type CapabilityApplyResult struct {
	CapabilityKey string
	SpecVersion   string
	Status        string
	Message       string
	ObservedAt    time.Time
}

type CapabilityManager struct {
	nodeID   string
	storeDir string
}

type capabilityDocument struct {
	NodeID        string          `json:"node_id"`
	CapabilityKey string          `json:"capability_key"`
	SpecVersion   string          `json:"spec_version"`
	DesiredSpec   json.RawMessage `json:"desired_spec"`
	StoredAt      time.Time       `json:"stored_at"`
}

func newCapabilityManager(cfg CapabilityManagerConfig) *CapabilityManager {
	baseDir := strings.TrimSpace(cfg.BaseDir)
	if baseDir == "" {
		baseDir = consts.GetDefaultYakitBaseDir()
	}
	return &CapabilityManager{
		nodeID:   strings.TrimSpace(cfg.NodeID),
		storeDir: filepath.Join(baseDir, "legion", "capabilities"),
	}
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
