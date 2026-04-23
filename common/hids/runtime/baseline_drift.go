//go:build hids && linux

package runtime

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

const (
	hostUserBaselineDriftRuleID  = "linux.process.host_user_baseline_drift"
	networkBaselineDriftRuleID   = "linux.network.allowlist_baseline_drift"
	hostUserBaselineDriftRuleSet = "linux.process.baseline"
	networkBaselineDriftRuleSet  = "linux.network.baseline"

	baselineDriftAggregationUpdateInterval = 30 * time.Second
)

type baselineDriftDetector struct {
	hostUsers  frozenHostUserSet
	network    frozenNetworkAllowlist
	drift      model.DriftAlertPolicy
	aggregator *driftAggregator
}

type frozenHostUserSet struct {
	usernames map[string]struct{}
	uids      map[string]struct{}
}

type frozenNetworkAllowlist struct {
	entries []frozenNetworkEntry
}

type frozenNetworkEntry struct {
	direction string
	protocol  string
	destCIDR  string
	prefix    netip.Prefix
	destPort  int
}

func newBaselineDriftDetector(policy model.BaselinePolicy) *baselineDriftDetector {
	policy = policy.Normalize()
	hostUsers := newFrozenHostUserSet(policy.HostUsers.FrozenUsers)
	network := newFrozenNetworkAllowlist(policy.Network.FrozenAllowlist)
	if hostUsers.empty() && network.empty() {
		return nil
	}
	return &baselineDriftDetector{
		hostUsers: hostUsers,
		network:   network,
		drift:     policy.DriftAlerts,
		aggregator: newDriftAggregator(
			policy.DriftAlerts.AggregationWindow(),
			policy.DriftAlerts.MaxAggregationEntries,
		),
	}
}

func (d *baselineDriftDetector) Evaluate(event model.Event) []model.Alert {
	if d == nil {
		return nil
	}
	switch event.Type {
	case model.EventTypeProcessExec:
		if alert, ok := d.evaluateHostUser(event); ok {
			return []model.Alert{alert}
		}
	case model.EventTypeNetworkAccept, model.EventTypeNetworkConnect, model.EventTypeNetworkState:
		if alert, ok := d.evaluateNetwork(event); ok {
			return []model.Alert{alert}
		}
	}
	return nil
}

func (d *baselineDriftDetector) evaluateHostUser(event model.Event) (model.Alert, bool) {
	if d == nil || d.hostUsers.empty() || event.Process == nil {
		return model.Alert{}, false
	}
	username := strings.TrimSpace(event.Process.Username)
	uid := strings.TrimSpace(event.Process.UID)
	if username == "" && uid == "" {
		return model.Alert{}, false
	}
	if d.hostUsers.contains(username, uid) {
		return model.Alert{}, false
	}

	observedAt := normalizedEventTimestamp(event.Timestamp)
	aggregationKey := strings.Join([]string{"host_user", username, uid}, "\x00")
	aggregate := d.aggregator.observe(aggregationKey, observedAt)
	if !aggregate.emit {
		return model.Alert{}, false
	}
	return model.Alert{
		RuleID:   hostUserBaselineDriftRuleID,
		Severity: d.drift.Severity,
		Title:    "process executed by unfrozen host user",
		Tags:     []string{"builtin", "baseline", "process", "host-user", "drift"},
		Detail: d.baselineDriftDetail(
			hostUserBaselineDriftRuleID,
			hostUserBaselineDriftRuleSet,
			event,
			aggregate,
			map[string]any{
				"baseline_kind": "host_user",
				"username":      username,
				"uid":           uid,
			},
			"process username or uid is outside the frozen host-user baseline",
		),
		ObservedAt: observedAt,
	}, true
}

func (d *baselineDriftDetector) evaluateNetwork(event model.Event) (model.Alert, bool) {
	if d == nil || d.network.empty() || event.Network == nil {
		return model.Alert{}, false
	}
	identity, ok := networkBaselineIdentityFromEvent(event)
	if !ok {
		return model.Alert{}, false
	}
	if d.network.contains(identity) {
		return model.Alert{}, false
	}

	observedAt := normalizedEventTimestamp(event.Timestamp)
	aggregationKey := strings.Join(
		[]string{
			"network",
			identity.direction,
			identity.protocol,
			identity.destCIDR,
			fmt.Sprintf("%d", identity.destPort),
		},
		"\x00",
	)
	aggregate := d.aggregator.observe(aggregationKey, observedAt)
	if !aggregate.emit {
		return model.Alert{}, false
	}
	return model.Alert{
		RuleID:   networkBaselineDriftRuleID,
		Severity: d.drift.Severity,
		Title:    "network connection outside frozen baseline",
		Tags:     []string{"builtin", "baseline", "network", "allowlist", "drift"},
		Detail: d.baselineDriftDetail(
			networkBaselineDriftRuleID,
			networkBaselineDriftRuleSet,
			event,
			aggregate,
			map[string]any{
				"baseline_kind": "network",
				"direction":     identity.direction,
				"protocol":      identity.protocol,
				"dest_cidr":     identity.destCIDR,
				"dest_port":     identity.destPort,
			},
			"network destination is outside the frozen network baseline",
		),
		ObservedAt: observedAt,
	}, true
}

func (d *baselineDriftDetector) baselineDriftDetail(
	ruleID string,
	ruleSet string,
	event model.Event,
	aggregate driftAggregationResult,
	baseline map[string]any,
	summary string,
) map[string]any {
	detail := map[string]any{
		"rule_id":                       ruleID,
		"builtin_rule_set":              ruleSet,
		"match_event_type":              event.Type,
		"rule_description":              summary,
		"summary":                       summary,
		"baseline":                      baseline,
		"aggregation_key":               aggregate.key,
		"aggregation_window_minutes":    d.drift.AggregationWindowMinutes,
		"aggregation_window_started_at": aggregate.windowStartedAt.UTC().Format(time.RFC3339Nano),
		"aggregation_last_observed_at":  aggregate.lastSeenAt.UTC().Format(time.RFC3339Nano),
		"hit_count":                     aggregate.count,
		"event":                         baselineEventDetail(event),
	}
	return detail
}

func newFrozenHostUserSet(users []model.FrozenHostUser) frozenHostUserSet {
	set := frozenHostUserSet{
		usernames: make(map[string]struct{}, len(users)),
		uids:      make(map[string]struct{}, len(users)),
	}
	for _, user := range users {
		if username := strings.TrimSpace(user.Username); username != "" {
			set.usernames[username] = struct{}{}
		}
		if uid := strings.TrimSpace(user.UID); uid != "" {
			set.uids[uid] = struct{}{}
		}
	}
	return set
}

func (s frozenHostUserSet) empty() bool {
	return len(s.usernames) == 0 && len(s.uids) == 0
}

func (s frozenHostUserSet) contains(username string, uid string) bool {
	if username = strings.TrimSpace(username); username != "" {
		if _, ok := s.usernames[username]; ok {
			return true
		}
	}
	if uid = strings.TrimSpace(uid); uid != "" {
		if _, ok := s.uids[uid]; ok {
			return true
		}
	}
	return false
}

func newFrozenNetworkAllowlist(entries []model.FrozenNetworkAllowlistEntry) frozenNetworkAllowlist {
	allowlist := frozenNetworkAllowlist{
		entries: make([]frozenNetworkEntry, 0, len(entries)),
	}
	for _, entry := range entries {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(entry.DestCIDR))
		if err != nil {
			continue
		}
		allowlist.entries = append(allowlist.entries, frozenNetworkEntry{
			direction: strings.ToLower(strings.TrimSpace(entry.Direction)),
			protocol:  strings.ToLower(strings.TrimSpace(entry.Protocol)),
			destCIDR:  prefix.Masked().String(),
			prefix:    prefix.Masked(),
			destPort:  entry.DestPort,
		})
	}
	return allowlist
}

func (a frozenNetworkAllowlist) empty() bool {
	return len(a.entries) == 0
}

func (a frozenNetworkAllowlist) contains(identity networkBaselineIdentity) bool {
	for _, entry := range a.entries {
		if entry.direction != identity.direction ||
			entry.protocol != identity.protocol ||
			entry.destPort != identity.destPort {
			continue
		}
		if identity.destAddr.IsValid() && entry.prefix.Contains(identity.destAddr) {
			return true
		}
		if entry.destCIDR == identity.destCIDR {
			return true
		}
	}
	return false
}

type networkBaselineIdentity struct {
	direction string
	protocol  string
	destCIDR  string
	destAddr  netip.Addr
	destPort  int
}

func networkBaselineIdentityFromEvent(event model.Event) (networkBaselineIdentity, bool) {
	if event.Network == nil {
		return networkBaselineIdentity{}, false
	}
	direction := strings.ToLower(strings.TrimSpace(event.Network.Direction))
	if direction == "" {
		direction = strings.ToLower(readStringMapValue(event.Data, "direction"))
	}
	protocol := strings.ToLower(strings.TrimSpace(event.Network.Protocol))
	destAddress := strings.TrimSpace(event.Network.DestAddress)
	destPort := event.Network.DestPort
	if direction == "" || protocol == "" || destAddress == "" || destPort <= 0 {
		return networkBaselineIdentity{}, false
	}
	destCIDR, destAddr := normalizeNetworkDestCIDR(destAddress)
	if destCIDR == "" {
		return networkBaselineIdentity{}, false
	}
	return networkBaselineIdentity{
		direction: direction,
		protocol:  protocol,
		destCIDR:  destCIDR,
		destAddr:  destAddr,
		destPort:  destPort,
	}, true
}

func normalizeNetworkDestCIDR(value string) (string, netip.Addr) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", netip.Addr{}
	}
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix.Masked().String(), netip.Addr{}
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return value, netip.Addr{}
	}
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr, bits).Masked().String(), addr
}

func baselineEventDetail(event model.Event) map[string]any {
	detail := map[string]any{
		"type":   event.Type,
		"source": event.Source,
	}
	if !event.Timestamp.IsZero() {
		detail["timestamp"] = event.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	if event.Process != nil {
		detail["process"] = baselineProcessDetail(event.Process)
	}
	if event.Network != nil {
		detail["network"] = baselineNetworkDetail(event.Network)
	}
	return detail
}

func baselineProcessDetail(process *model.Process) map[string]any {
	if process == nil {
		return nil
	}
	return map[string]any{
		"pid":                       process.PID,
		"parent_pid":                process.ParentPID,
		"name":                      process.Name,
		"username":                  process.Username,
		"uid":                       process.UID,
		"gid":                       process.GID,
		"image":                     process.Image,
		"command":                   process.Command,
		"parent_name":               process.ParentName,
		"parent_image":              process.ParentImage,
		"parent_command":            process.ParentCommand,
		"boot_id":                   process.BootID,
		"start_time_unix_ms":        process.StartTimeUnixMillis,
		"parent_start_time_unix_ms": process.ParentStartTimeUnixMillis,
	}
}

func baselineNetworkDetail(network *model.Network) map[string]any {
	if network == nil {
		return nil
	}
	return map[string]any{
		"protocol":         network.Protocol,
		"source_address":   network.SourceAddress,
		"dest_address":     network.DestAddress,
		"source_port":      network.SourcePort,
		"dest_port":        network.DestPort,
		"connection_state": network.ConnectionState,
		"direction":        network.Direction,
		"fd":               network.FD,
		"family":           network.Family,
		"socket_type":      network.SocketType,
		"inode":            network.Inode,
	}
}

type driftAggregator struct {
	window         time.Duration
	maxEntries     int
	updateInterval time.Duration
	entries        map[string]driftAggregateEntry
}

type driftAggregateEntry struct {
	windowStartedAt time.Time
	lastSeenAt      time.Time
	lastEmitAt      time.Time
	count           int
}

type driftAggregationResult struct {
	key             string
	windowStartedAt time.Time
	lastSeenAt      time.Time
	count           int
	emit            bool
}

func newDriftAggregator(window time.Duration, maxEntries int) *driftAggregator {
	if window <= 0 {
		window = time.Duration(model.DefaultBaselineDriftAggregationMinutes) * time.Minute
	}
	if maxEntries <= 0 {
		maxEntries = model.DefaultBaselineDriftMaxAggregationEntries
	}
	return &driftAggregator{
		window:         window,
		maxEntries:     maxEntries,
		updateInterval: baselineDriftAggregationUpdateInterval,
		entries:        make(map[string]driftAggregateEntry),
	}
}

func (a *driftAggregator) observe(key string, observedAt time.Time) driftAggregationResult {
	if a == nil {
		return driftAggregationResult{
			key:             strings.TrimSpace(key),
			windowStartedAt: observedAt,
			lastSeenAt:      observedAt,
			count:           1,
			emit:            true,
		}
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return driftAggregationResult{
			windowStartedAt: observedAt,
			lastSeenAt:      observedAt,
			count:           1,
			emit:            true,
		}
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	a.prune(observedAt)
	entry, exists := a.entries[key]
	if exists && observedAt.Sub(entry.windowStartedAt) < a.window {
		entry.count++
		entry.lastSeenAt = observedAt
		emit := entry.count == 2 ||
			a.updateInterval <= 0 ||
			observedAt.Sub(entry.lastEmitAt) >= a.updateInterval
		if emit {
			entry.lastEmitAt = observedAt
		}
		a.entries[key] = entry
		return driftAggregationResult{
			key:             key,
			windowStartedAt: entry.windowStartedAt,
			lastSeenAt:      entry.lastSeenAt,
			count:           entry.count,
			emit:            emit,
		}
	}
	a.entries[key] = driftAggregateEntry{
		windowStartedAt: observedAt,
		lastSeenAt:      observedAt,
		lastEmitAt:      observedAt,
		count:           1,
	}
	a.enforceCapacity()
	return driftAggregationResult{
		key:             key,
		windowStartedAt: observedAt,
		lastSeenAt:      observedAt,
		count:           1,
		emit:            true,
	}
}

func (a *driftAggregator) prune(observedAt time.Time) {
	if a == nil || a.window <= 0 {
		return
	}
	for key, entry := range a.entries {
		if observedAt.Sub(entry.windowStartedAt) >= a.window {
			delete(a.entries, key)
		}
	}
}

func (a *driftAggregator) enforceCapacity() {
	if a == nil || a.maxEntries <= 0 {
		return
	}
	for len(a.entries) > a.maxEntries {
		oldestKey := ""
		var oldestSeen time.Time
		for key, entry := range a.entries {
			if oldestKey == "" || entry.lastSeenAt.Before(oldestSeen) {
				oldestKey = key
				oldestSeen = entry.lastSeenAt
			}
		}
		if oldestKey == "" {
			return
		}
		delete(a.entries, oldestKey)
	}
}
