package node

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/spec"
)

const (
	// DefaultHeartbeatInterval controls how often the node renews its session.
	DefaultHeartbeatInterval = 30 * time.Second
	// DefaultTickerInterval controls local ticker callback cadence.
	DefaultTickerInterval = time.Second
	// DefaultRequestTimeout bounds node-to-platform HTTP requests.
	DefaultRequestTimeout = 10 * time.Second
	// DefaultLifecycleState is reported before scanner-specific runtime wiring exists.
	DefaultLifecycleState = "ready"
)

type ActiveAttemptHeartbeat struct {
	AttemptID      string    `json:"attempt_id"`
	JobID          string    `json:"job_id"`
	SubtaskID      string    `json:"subtask_id"`
	Status         string    `json:"status"`
	CompletedUnits uint32    `json:"completed_units"`
	TotalUnits     uint32    `json:"total_units"`
	LastActivityAt time.Time `json:"last_activity_at"`
}

type HostInfo struct {
	Hostname        string   `json:"hostname"`
	PrimaryIP       string   `json:"primary_ip"`
	IPAddresses     []string `json:"ip_addresses"`
	OperatingSystem string   `json:"operating_system"`
	Architecture    string   `json:"architecture"`
}

type HostInfoProvider interface {
	Snapshot() HostInfo
}

// RuntimeStatus is the execution snapshot mixed into heartbeat payloads.
type RuntimeStatus struct {
	LifecycleState string
	RunningJobs    uint32
	MaxRunningJobs uint32
	ActiveAttempts []ActiveAttemptHeartbeat
}

// RuntimeStatusProvider lets higher-level runtimes report execution state.
type RuntimeStatusProvider interface {
	Snapshot() RuntimeStatus
}

// BaseConfig defines how NodeBase connects to the platform.
type BaseConfig struct {
	NodeType           spec.NodeType
	NodeID             string
	EnrollmentToken    string
	PlatformAPIBaseURL string
	Version            string
	Labels             map[string]string
	CapabilityKeys     []string
	HeartbeatInterval  time.Duration
	TickerInterval     time.Duration
	RequestTimeout     time.Duration
	LifecycleState     string
	MaxRunningJobs     uint32
	TransportClient    SessionTransport
	StatusProvider     RuntimeStatusProvider
	HostInfoProvider   HostInfoProvider
	HTTPClient         *http.Client
}

func normalizeBaseConfig(cfg BaseConfig) (BaseConfig, error) {
	if err := validateBaseConfig(cfg); err != nil {
		return BaseConfig{}, err
	}

	normalized := cfg
	if normalized.NodeType == "" {
		normalized.NodeType = spec.NodeType_Scanner
	}
	if normalized.HeartbeatInterval <= 0 {
		normalized.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if normalized.TickerInterval <= 0 {
		normalized.TickerInterval = DefaultTickerInterval
	}
	if normalized.RequestTimeout <= 0 {
		normalized.RequestTimeout = DefaultRequestTimeout
	}
	if normalized.LifecycleState == "" {
		normalized.LifecycleState = DefaultLifecycleState
	}
	if normalized.HostInfoProvider == nil {
		normalized.HostInfoProvider = systemHostInfoProvider{}
	}

	normalized.NodeID = strings.TrimSpace(normalized.NodeID)
	normalized.EnrollmentToken = strings.TrimSpace(normalized.EnrollmentToken)
	normalized.PlatformAPIBaseURL = strings.TrimRight(
		strings.TrimSpace(normalized.PlatformAPIBaseURL),
		"/",
	)
	normalized.Version = strings.TrimSpace(normalized.Version)
	normalized.Labels = cloneStringMap(normalized.Labels)
	normalized.CapabilityKeys = cloneStringSlice(normalized.CapabilityKeys)
	return normalized, nil
}

func validateBaseConfig(cfg BaseConfig) error {
	switch {
	case strings.TrimSpace(cfg.NodeID) == "":
		return fmt.Errorf("node_id is required")
	case strings.TrimSpace(cfg.EnrollmentToken) == "":
		return fmt.Errorf("enrollment_token is required")
	case strings.TrimSpace(cfg.PlatformAPIBaseURL) == "":
		return fmt.Errorf("platform_api_base_url is required")
	default:
		return nil
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		return []string{}
	}

	result := make([]string, len(input))
	copy(result, input)
	return result
}

func cloneActiveAttemptHeartbeats(input []ActiveAttemptHeartbeat) []ActiveAttemptHeartbeat {
	if len(input) == 0 {
		return []ActiveAttemptHeartbeat{}
	}

	result := make([]ActiveAttemptHeartbeat, len(input))
	copy(result, input)
	return result
}

func cloneHostInfo(input HostInfo) HostInfo {
	return HostInfo{
		Hostname:        input.Hostname,
		PrimaryIP:       input.PrimaryIP,
		IPAddresses:     cloneStringSlice(input.IPAddresses),
		OperatingSystem: input.OperatingSystem,
		Architecture:    input.Architecture,
	}
}
