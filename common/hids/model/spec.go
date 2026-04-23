//go:build hids

package model

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"path/filepath"
	"strings"
	"time"
)

const (
	ModeObserve = "observe"

	CollectorBackendEBPF      = "ebpf"
	CollectorBackendAuditd    = "auditd"
	CollectorBackendFileWatch = "filewatch"
)

type DesiredSpec struct {
	Mode            string          `json:"mode"`
	Collectors      Collectors      `json:"collectors"`
	BuiltinRuleSets []string        `json:"builtin_rule_sets,omitempty"`
	TemporaryRules  []TemporaryRule `json:"temporary_rules,omitempty"`
	EvidencePolicy  EvidencePolicy  `json:"evidence_policy,omitempty"`
	Reporting       ReportingPolicy `json:"reporting,omitempty"`
	ContextPolicy   ContextPolicy   `json:"context_policy,omitempty"`
	BaselinePolicy  BaselinePolicy  `json:"baseline_policy,omitempty"`
}

type Collectors struct {
	Process CollectorSpec     `json:"process"`
	Network CollectorSpec     `json:"network"`
	File    FileCollectorSpec `json:"file"`
	Audit   CollectorSpec     `json:"audit"`
}

type CollectorSpec struct {
	Enabled bool   `json:"enabled"`
	Backend string `json:"backend,omitempty"`
}

type FileCollectorSpec struct {
	Enabled    bool     `json:"enabled"`
	Backend    string   `json:"backend,omitempty"`
	WatchPaths []string `json:"watch_paths,omitempty"`
}

type EvidencePolicy struct {
	CaptureProcessTree   bool `json:"capture_process_tree,omitempty"`
	CaptureProcessMemory bool `json:"capture_process_memory,omitempty"`
	CaptureFileHash      bool `json:"capture_file_hash,omitempty"`
}

type ReportingPolicy struct {
	EmitCapabilityStatus     bool  `json:"emit_capability_status,omitempty"`
	EmitCapabilityAlert      bool  `json:"emit_capability_alert,omitempty"`
	EmitSnapshotObservations *bool `json:"emit_snapshot_observations,omitempty"`
}

type ContextPolicy struct {
	ShortTermWindowMinutes int `json:"short_term_window_minutes,omitempty"`
}

type BaselinePolicy struct {
	HostUsers   HostUserBaselinePolicy `json:"host_users,omitempty"`
	Network     NetworkBaselinePolicy  `json:"network,omitempty"`
	DriftAlerts DriftAlertPolicy       `json:"drift_alerts,omitempty"`
}

type HostUserBaselinePolicy struct {
	FrozenUsers []FrozenHostUser `json:"frozen_users,omitempty"`
}

type FrozenHostUser struct {
	Username   string   `json:"username,omitempty"`
	UID        string   `json:"uid,omitempty"`
	Groups     []string `json:"groups,omitempty"`
	Privileged bool     `json:"privileged,omitempty"`
}

type NetworkBaselinePolicy struct {
	FrozenAllowlist []FrozenNetworkAllowlistEntry `json:"frozen_allowlist,omitempty"`
}

type FrozenNetworkAllowlistEntry struct {
	Direction string `json:"direction"`
	Protocol  string `json:"protocol"`
	DestCIDR  string `json:"dest_cidr"`
	DestPort  int    `json:"dest_port"`
}

type DriftAlertPolicy struct {
	Severity                 string `json:"severity,omitempty"`
	AggregationWindowMinutes int    `json:"aggregation_window_minutes,omitempty"`
	MaxAggregationEntries    int    `json:"max_aggregation_entries,omitempty"`
}

type TemporaryRule struct {
	RuleID         string         `json:"rule_id"`
	Title          string         `json:"title,omitempty"`
	Description    string         `json:"description,omitempty"`
	Enabled        bool           `json:"enabled"`
	MatchEventType string         `json:"match_event_type"`
	Severity       string         `json:"severity,omitempty"`
	Condition      string         `json:"condition"`
	Action         string         `json:"action,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

func (r TemporaryRule) IsBlank() bool {
	return !r.Enabled &&
		r.RuleID == "" &&
		r.MatchEventType == "" &&
		r.Condition == "" &&
		r.Action == "" &&
		len(r.Tags) == 0
}

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "invalid hids desired spec"
	}
	if e.Field == "" {
		return e.Reason
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func ParseDesiredSpec(raw []byte) (DesiredSpec, error) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}

	var spec DesiredSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return DesiredSpec{}, &ValidationError{
			Field:  "desired_spec_json",
			Reason: err.Error(),
		}
	}

	spec = spec.Normalize()
	if err := spec.Validate(); err != nil {
		return DesiredSpec{}, err
	}
	return spec, nil
}

func (s DesiredSpec) Normalize() DesiredSpec {
	s.Mode = normalizeStringOrDefault(s.Mode, ModeObserve)
	s.Collectors.Process.Backend = strings.ToLower(strings.TrimSpace(s.Collectors.Process.Backend))
	s.Collectors.Network.Backend = strings.ToLower(strings.TrimSpace(s.Collectors.Network.Backend))
	s.Collectors.File.Backend = strings.ToLower(strings.TrimSpace(s.Collectors.File.Backend))
	s.Collectors.Audit.Backend = strings.ToLower(strings.TrimSpace(s.Collectors.Audit.Backend))
	s.Collectors.File.WatchPaths = normalizePaths(s.Collectors.File.WatchPaths)
	s.BuiltinRuleSets = normalizeStringList(s.BuiltinRuleSets)
	s.TemporaryRules = normalizeTemporaryRules(s.TemporaryRules)
	s.ContextPolicy = s.ContextPolicy.Normalize()
	s.BaselinePolicy = s.BaselinePolicy.Normalize()
	return s
}

func (s DesiredSpec) Validate() error {
	if s.Mode != ModeObserve {
		return invalidField("mode", "only observe mode is supported")
	}
	if s.Collectors.Process.Enabled && s.Collectors.Process.Backend != CollectorBackendEBPF {
		return invalidField("collectors.process.backend", "must be ebpf")
	}
	if s.Collectors.Network.Enabled && s.Collectors.Network.Backend != CollectorBackendEBPF {
		return invalidField("collectors.network.backend", "must be ebpf")
	}
	if s.Collectors.File.Enabled {
		if s.Collectors.File.Backend != CollectorBackendFileWatch {
			return invalidField("collectors.file.backend", "must be filewatch")
		}
		if len(s.Collectors.File.WatchPaths) == 0 {
			return invalidField("collectors.file.watch_paths", "must contain at least one absolute path")
		}
		for _, watchPath := range s.Collectors.File.WatchPaths {
			if !filepath.IsAbs(watchPath) {
				return invalidField("collectors.file.watch_paths", "all watch_paths must be absolute")
			}
		}
	}
	if s.Collectors.Audit.Enabled && s.Collectors.Audit.Backend != CollectorBackendAuditd {
		return invalidField("collectors.audit.backend", "must be auditd")
	}
	if !s.Collectors.Process.Enabled &&
		!s.Collectors.Network.Enabled &&
		!s.Collectors.File.Enabled &&
		!s.Collectors.Audit.Enabled {
		return invalidField("collectors", "at least one collector must be enabled")
	}
	contextPolicy := s.ContextPolicy.Normalize()
	if err := contextPolicy.Validate(); err != nil {
		return err
	}
	baselinePolicy := s.BaselinePolicy.Normalize()
	if err := baselinePolicy.Validate(); err != nil {
		return err
	}

	for i, rule := range s.TemporaryRules {
		if !rule.Enabled {
			continue
		}
		fieldPrefix := fmt.Sprintf("temporary_rules[%d]", i)
		if rule.RuleID == "" {
			return invalidField(fieldPrefix+".rule_id", "is required")
		}
		if rule.MatchEventType == "" {
			return invalidField(fieldPrefix+".match_event_type", "is required")
		}
		if !isSupportedEventType(rule.MatchEventType) {
			return invalidField(
				fieldPrefix+".match_event_type",
				fmt.Sprintf("unsupported event type %q", rule.MatchEventType),
			)
		}
		if !s.CanCollectorEmit(rule.MatchEventType) {
			return invalidField(
				fieldPrefix+".match_event_type",
				fmt.Sprintf("event type %q is not producible by the enabled collectors", rule.MatchEventType),
			)
		}
		if rule.Condition == "" {
			return invalidField(fieldPrefix+".condition", "is required")
		}
	}
	return nil
}

// ShouldEmitSnapshotObservations returns whether platform-facing current-state
// snapshot observations should be exported for this reporting policy.
func (p ReportingPolicy) ShouldEmitSnapshotObservations() bool {
	if p.EmitSnapshotObservations == nil {
		return false
	}
	return *p.EmitSnapshotObservations
}

const (
	DefaultShortTermWindowMinutes              = 5
	DefaultBaselineDriftSeverity               = "medium"
	DefaultBaselineDriftAggregationMinutes     = 15
	DefaultBaselineDriftMaxAggregationEntries  = 1024
	defaultMinimumBaselineDriftAggregationSize = 1
)

func (p ContextPolicy) Normalize() ContextPolicy {
	if p.ShortTermWindowMinutes <= 0 {
		p.ShortTermWindowMinutes = DefaultShortTermWindowMinutes
	}
	return p
}

func (p ContextPolicy) Validate() error {
	if p.ShortTermWindowMinutes <= 0 {
		return invalidField("context_policy.short_term_window_minutes", "must be a positive integer")
	}
	return nil
}

func (p ContextPolicy) ShortTermWindow() time.Duration {
	p = p.Normalize()
	return time.Duration(p.ShortTermWindowMinutes) * time.Minute
}

func (p BaselinePolicy) Normalize() BaselinePolicy {
	p.HostUsers = p.HostUsers.Normalize()
	p.Network = p.Network.Normalize()
	p.DriftAlerts = p.DriftAlerts.Normalize()
	return p
}

func (p BaselinePolicy) Validate() error {
	if err := p.HostUsers.Validate(); err != nil {
		return err
	}
	if err := p.Network.Validate(); err != nil {
		return err
	}
	return p.DriftAlerts.Validate()
}

func (p HostUserBaselinePolicy) Normalize() HostUserBaselinePolicy {
	users := make([]FrozenHostUser, 0, len(p.FrozenUsers))
	seen := make(map[string]struct{}, len(p.FrozenUsers))
	for _, user := range p.FrozenUsers {
		user.Username = strings.TrimSpace(user.Username)
		user.UID = strings.TrimSpace(user.UID)
		user.Groups = normalizeStringList(user.Groups)
		if user.Username == "" && user.UID == "" {
			continue
		}
		key := strings.ToLower(user.Username) + "\x00" + user.UID
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		users = append(users, user)
	}
	p.FrozenUsers = users
	return p
}

func (p HostUserBaselinePolicy) Validate() error {
	for index, user := range p.FrozenUsers {
		if strings.TrimSpace(user.Username) == "" && strings.TrimSpace(user.UID) == "" {
			return invalidField(
				fmt.Sprintf("baseline_policy.host_users.frozen_users[%d]", index),
				"username or uid is required",
			)
		}
	}
	return nil
}

func (p NetworkBaselinePolicy) Normalize() NetworkBaselinePolicy {
	entries := make([]FrozenNetworkAllowlistEntry, 0, len(p.FrozenAllowlist))
	seen := make(map[string]struct{}, len(p.FrozenAllowlist))
	for _, entry := range p.FrozenAllowlist {
		entry.Direction = strings.ToLower(strings.TrimSpace(entry.Direction))
		entry.Protocol = strings.ToLower(strings.TrimSpace(entry.Protocol))
		entry.DestCIDR = normalizeCIDRString(entry.DestCIDR)
		if entry.Direction == "" && entry.Protocol == "" && entry.DestCIDR == "" && entry.DestPort == 0 {
			continue
		}
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%d", entry.Direction, entry.Protocol, entry.DestCIDR, entry.DestPort)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, entry)
	}
	p.FrozenAllowlist = entries
	return p
}

func (p NetworkBaselinePolicy) Validate() error {
	for index, entry := range p.FrozenAllowlist {
		fieldPrefix := fmt.Sprintf("baseline_policy.network.frozen_allowlist[%d]", index)
		if strings.TrimSpace(entry.Direction) == "" {
			return invalidField(fieldPrefix+".direction", "is required")
		}
		if strings.TrimSpace(entry.Protocol) == "" {
			return invalidField(fieldPrefix+".protocol", "is required")
		}
		if strings.TrimSpace(entry.DestCIDR) == "" {
			return invalidField(fieldPrefix+".dest_cidr", "is required")
		}
		if _, err := netip.ParsePrefix(entry.DestCIDR); err != nil {
			return invalidField(fieldPrefix+".dest_cidr", "must be a valid CIDR prefix")
		}
		if entry.DestPort <= 0 {
			return invalidField(fieldPrefix+".dest_port", "must be a positive integer")
		}
	}
	return nil
}

func (p DriftAlertPolicy) Normalize() DriftAlertPolicy {
	p.Severity = strings.ToLower(strings.TrimSpace(p.Severity))
	if p.Severity == "" {
		p.Severity = DefaultBaselineDriftSeverity
	}
	if p.AggregationWindowMinutes <= 0 {
		p.AggregationWindowMinutes = DefaultBaselineDriftAggregationMinutes
	}
	if p.MaxAggregationEntries <= 0 {
		p.MaxAggregationEntries = DefaultBaselineDriftMaxAggregationEntries
	}
	return p
}

func (p DriftAlertPolicy) Validate() error {
	if p.AggregationWindowMinutes <= 0 {
		return invalidField("baseline_policy.drift_alerts.aggregation_window_minutes", "must be a positive integer")
	}
	if p.MaxAggregationEntries < defaultMinimumBaselineDriftAggregationSize {
		return invalidField("baseline_policy.drift_alerts.max_aggregation_entries", "must be a positive integer")
	}
	return nil
}

func (p DriftAlertPolicy) AggregationWindow() time.Duration {
	p = p.Normalize()
	return time.Duration(p.AggregationWindowMinutes) * time.Minute
}

func normalizeCIDRString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix.Masked().String()
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return value
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr, bits).Masked().String()
}

func normalizeTemporaryRules(rules []TemporaryRule) []TemporaryRule {
	normalized := make([]TemporaryRule, 0, len(rules))
	for _, rule := range rules {
		rule.RuleID = strings.TrimSpace(rule.RuleID)
		rule.Title = strings.TrimSpace(rule.Title)
		rule.Description = strings.TrimSpace(rule.Description)
		rule.MatchEventType = strings.ToLower(strings.TrimSpace(rule.MatchEventType))
		rule.Severity = normalizeStringOrDefault(rule.Severity, "medium")
		rule.Condition = strings.TrimSpace(rule.Condition)
		rule.Action = strings.TrimSpace(rule.Action)
		rule.Tags = normalizeStringList(rule.Tags)
		rule.Metadata = normalizeMetadata(rule.Metadata)
		normalized = append(normalized, rule)
	}
	return normalized
}

func normalizeMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func isSupportedEventType(value string) bool {
	switch value {
	case EventTypeProcessExec,
		EventTypeProcessExit,
		EventTypeNetworkAccept,
		EventTypeNetworkConnect,
		EventTypeNetworkClose,
		EventTypeNetworkState,
		EventTypeFileChange,
		EventTypeAudit,
		EventTypeAuditLoss:
		return true
	default:
		return false
	}
}

func (s DesiredSpec) CanCollectorEmit(eventType string) bool {
	switch eventType {
	case EventTypeProcessExec, EventTypeProcessExit:
		return s.Collectors.Process.Enabled
	case EventTypeNetworkAccept, EventTypeNetworkConnect, EventTypeNetworkClose, EventTypeNetworkState:
		return s.Collectors.Network.Enabled
	case EventTypeFileChange:
		return s.Collectors.File.Enabled
	case EventTypeAudit, EventTypeAuditLoss:
		return s.Collectors.Audit.Enabled
	default:
		return false
	}
}

func normalizePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		normalized = append(normalized, path)
	}
	return normalized
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeStringOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func invalidField(field string, reason string) error {
	return &ValidationError{
		Field:  field,
		Reason: reason,
	}
}
