//go:build hids && linux

package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	hidscollector "github.com/yaklang/yaklang/common/hids/collector"
	"github.com/yaklang/yaklang/common/hids/collector/auditd"
	hidsebpf "github.com/yaklang/yaklang/common/hids/collector/ebpf"
	"github.com/yaklang/yaklang/common/hids/collector/filewatch"
	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/rule"
	builtinrules "github.com/yaklang/yaklang/common/hids/rule/builtin"
)

type Instance struct {
	spec                     model.DesiredSpec
	collectors               []collectorBinding
	pipeline                 *pipeline
	events                   chan model.Event
	cancel                   context.CancelFunc
	baselineProvider         inventoryProvider
	inventoryRefreshInterval time.Duration
	ruleCoverage             []builtinrules.RuleSetCoverage
}

type collectorBinding struct {
	kind      string
	backend   string
	collector hidscollector.Instance
	started   bool
	startErr  error
	updatedAt time.Time
}

func newInstance(spec model.DesiredSpec) (*Instance, error) {
	return newInstanceWithOptions(spec, instanceOptions{
		baselineProvider: newSystemInventoryProvider(),
	})
}

type instanceOptions struct {
	baselineProvider         inventoryProvider
	inventoryRefreshInterval time.Duration
}

func newInstanceWithOptions(spec model.DesiredSpec, options instanceOptions) (*Instance, error) {
	collectors, err := buildCollectors(spec)
	if err != nil {
		return nil, err
	}
	engine, err := rule.NewEngine(spec)
	if err != nil {
		return nil, err
	}
	coverage, err := builtinrules.DescribeCoverage(spec.BuiltinRuleSets, spec.CanCollectorEmit)
	if err != nil {
		return nil, err
	}
	return &Instance{
		spec:       spec,
		collectors: collectors,
		pipeline: newPipelineFromSpec(engine, spec).
			withArtifactEnricher(
				newArtifactEnricher(spec.EvidencePolicy, shortTermContextConfigFromSpec(spec)),
			).
			withEvidencePolicy(spec.EvidencePolicy),
		events:                   make(chan model.Event, 256),
		baselineProvider:         options.baselineProvider,
		inventoryRefreshInterval: options.inventoryRefreshInterval,
		ruleCoverage:             coverage,
	}, nil
}

func (i *Instance) start(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	i.cancel = cancel

	if i.pipeline != nil {
		go i.pipeline.Run(ctx, i.events)
	}

	startedCount := 0
	var startErrs []string
	for idx := range i.collectors {
		binding := &i.collectors[idx]
		if err := binding.start(ctx, i.events); err != nil {
			startErrs = append(startErrs, fmt.Sprintf("%s: %v", binding.label(), err))
			continue
		}
		startedCount++
	}

	if startedCount == 0 {
		cancel()
		for idx := range i.collectors {
			_ = i.collectors[idx].close()
		}
		if len(startErrs) > 0 {
			return fmt.Errorf("no hids collector started successfully: %s", strings.Join(startErrs, "; "))
		}
		return fmt.Errorf("no hids collector started successfully")
	}

	if i.baselineProvider != nil {
		go emitInventoryObservations(ctx, i.spec, i.baselineProvider, i.events)
		if i.spec.Reporting.ShouldEmitSnapshotObservations() {
			go i.refreshInventoryLoop(ctx)
		}
	}

	return nil
}

func (i *Instance) close() error {
	if i == nil {
		return nil
	}
	if i.cancel != nil {
		i.cancel()
	}

	var errs []string
	for idx := range i.collectors {
		if err := i.collectors[idx].close(); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", i.collectors[idx].label(), err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close hids collectors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (i *Instance) replayInventory(ctx context.Context) error {
	if i == nil || i.baselineProvider == nil || i.events == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	emitInventoryObservations(ctx, i.spec, i.baselineProvider, i.events)
	return nil
}

func (i *Instance) refreshInventoryLoop(ctx context.Context) {
	if i == nil || i.baselineProvider == nil || i.events == nil {
		return
	}

	interval := i.inventoryRefreshInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			emitInventoryObservations(ctx, i.spec, i.baselineProvider, i.events)
		}
	}
}

func (i *Instance) activeCollectorNames() []string {
	names := make([]string, 0, len(i.collectors))
	for _, collector := range i.collectors {
		if !collector.started {
			continue
		}
		names = append(names, collector.label())
	}
	return names
}

func (i *Instance) collectorStatusDetail() map[string]any {
	if i == nil || len(i.collectors) == 0 {
		return nil
	}

	collectors := make(map[string]any, len(i.collectors))
	for _, collector := range i.collectors {
		snapshot := collector.healthSnapshot()
		collectors[snapshot.Name] = map[string]any{
			"name":       snapshot.Name,
			"backend":    snapshot.Backend,
			"status":     snapshot.Status,
			"message":    snapshot.Message,
			"updated_at": snapshot.UpdatedAt,
			"detail":     cloneCollectorDetailMap(snapshot.Detail),
		}
	}
	if len(collectors) == 0 {
		return nil
	}
	return collectors
}

func (i *Instance) runtimeStatusDetail() map[string]any {
	if i == nil {
		return nil
	}

	detail := map[string]any{}
	if collectors := i.collectorStatusDetail(); len(collectors) > 0 {
		detail["collectors"] = collectors
	}
	if rules := i.ruleStatusDetail(); len(rules) > 0 {
		detail["rules"] = rules
	}
	if len(detail) == 0 {
		return nil
	}
	return detail
}

func (i *Instance) runtimeState() model.RuntimeState {
	if i == nil {
		return model.RuntimeState{}
	}

	activeCollectors := i.activeCollectorNames()
	status := "running"
	message := fmt.Sprintf("hids runtime active collectors: %s", strings.Join(activeCollectors, ", "))
	if len(activeCollectors) == 0 {
		status = "degraded"
		message = "hids runtime has no active collectors"
	}

	degradedCollectors := i.degradedCollectorSummaries()
	if len(degradedCollectors) > 0 {
		status = "degraded"
		message = fmt.Sprintf("%s; degraded collectors: %s", message, strings.Join(degradedCollectors, "; "))
	}
	if inactiveRules := i.inactiveRuleCount(); inactiveRules > 0 {
		status = "degraded"
		message = fmt.Sprintf("%s; %d builtin rule(s) inactive because enabled collectors cannot produce their event types", message, inactiveRules)
	}

	return model.RuntimeState{
		Status:           status,
		Message:          message,
		Mode:             i.spec.Mode,
		ActiveCollectors: activeCollectors,
		Detail:           i.runtimeStatusDetail(),
		UpdatedAt:        timeNowUTC(),
	}
}

func (i *Instance) ruleStatusDetail() map[string]any {
	if i == nil {
		return nil
	}

	detail := map[string]any{}
	if len(i.ruleCoverage) > 0 {
		ruleSets := make([]map[string]any, 0, len(i.ruleCoverage))
		activeCount := 0
		inactiveCount := 0
		for _, coverage := range i.ruleCoverage {
			activeRules := ruleCoverageList(coverage.ActiveRules)
			inactiveRules := inactiveRuleCoverageList(coverage.InactiveRules)
			activeCount += len(activeRules)
			inactiveCount += len(inactiveRules)
			ruleSets = append(ruleSets, map[string]any{
				"rule_set":       coverage.RuleSet,
				"status":         coverage.Status,
				"active_rules":   activeRules,
				"inactive_rules": inactiveRules,
			})
		}
		detail["builtin_rule_sets"] = ruleSets
		detail["active_count"] = activeCount
		detail["inactive_count"] = inactiveCount
	}
	if temporaryRules := i.temporaryRuleStatusDetail(); len(temporaryRules) > 0 {
		detail["temporary_rules"] = temporaryRules
	}
	if len(detail) == 0 {
		return nil
	}
	return detail
}

func (i *Instance) inactiveRuleCount() int {
	if i == nil {
		return 0
	}
	count := 0
	for _, coverage := range i.ruleCoverage {
		count += len(coverage.InactiveRules)
	}
	return count
}

func ruleCoverageList(rules []builtinrules.RuleCoverage) []map[string]any {
	items := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		items = append(items, ruleCoverageMap(rule))
	}
	return items
}

func inactiveRuleCoverageList(rules []builtinrules.InactiveRuleCoverage) []map[string]any {
	items := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		item := ruleCoverageMap(rule.RuleCoverage)
		item["reason"] = rule.Reason
		items = append(items, item)
	}
	return items
}

func (i *Instance) temporaryRuleStatusDetail() map[string]any {
	if i == nil || len(i.spec.TemporaryRules) == 0 {
		return nil
	}

	activeRules := make([]map[string]any, 0, len(i.spec.TemporaryRules))
	inactiveRules := make([]map[string]any, 0, len(i.spec.TemporaryRules))
	configuredCount := 0
	for _, temporaryRule := range i.spec.TemporaryRules {
		if temporaryRule.IsBlank() {
			continue
		}
		configuredCount++
		item := temporaryRuleStatusMap(temporaryRule)
		if temporaryRule.Enabled {
			item["status"] = "active"
			activeRules = append(activeRules, item)
			continue
		}
		item["status"] = "inactive"
		item["reason"] = "disabled in desired spec"
		inactiveRules = append(inactiveRules, item)
	}
	if configuredCount == 0 {
		return nil
	}

	return map[string]any{
		"configured_count": configuredCount,
		"active_count":     len(activeRules),
		"inactive_count":   len(inactiveRules),
		"active_rules":     activeRules,
		"inactive_rules":   inactiveRules,
	}
}

func temporaryRuleStatusMap(rule model.TemporaryRule) map[string]any {
	item := map[string]any{
		"rule_id":          rule.RuleID,
		"title":            firstNonEmptyRuleText(rule.Title, rule.RuleID),
		"match_event_type": rule.MatchEventType,
		"severity":         rule.Severity,
	}
	if templateID := temporaryRuleMetadataString(rule.Metadata, "template_id"); templateID != "" {
		item["template_id"] = templateID
	}
	if packID := temporaryRuleMetadataString(rule.Metadata, "pack_id"); packID != "" {
		item["pack_id"] = packID
	}
	return item
}

func temporaryRuleMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func firstNonEmptyRuleText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func ruleCoverageMap(rule builtinrules.RuleCoverage) map[string]any {
	return map[string]any{
		"rule_id":          rule.RuleID,
		"rule_set":         rule.RuleSet,
		"match_event_type": rule.MatchEventType,
		"severity":         rule.Severity,
		"title":            rule.Title,
	}
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func cloneCollectorDetailMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func (i *Instance) alerts() <-chan model.Alert {
	if i == nil || i.pipeline == nil {
		return nil
	}
	return i.pipeline.Alerts()
}

func (i *Instance) observations() <-chan model.Event {
	if i == nil || i.pipeline == nil {
		return nil
	}
	return i.pipeline.Observations()
}

func (i *Instance) degradedCollectorSummaries() []string {
	if i == nil {
		return nil
	}

	summaries := make([]string, 0, len(i.collectors))
	for _, collector := range i.collectors {
		snapshot := collector.healthSnapshot()
		if strings.EqualFold(strings.TrimSpace(snapshot.Status), "running") {
			continue
		}
		summaries = append(summaries, fmt.Sprintf("%s (%s)", collector.label(), strings.TrimSpace(snapshot.Message)))
	}
	return summaries
}

func (c *collectorBinding) label() string {
	kind := strings.TrimSpace(c.kind)
	backend := strings.TrimSpace(c.backend)
	switch {
	case kind != "" && backend != "":
		return kind + ":" + backend
	case kind != "":
		return kind
	case backend != "":
		return backend
	case c.collector != nil:
		return c.collector.Name()
	default:
		return "collector"
	}
}

func (c *collectorBinding) start(ctx context.Context, sink chan<- model.Event) error {
	if c == nil || c.collector == nil {
		c.updatedAt = timeNowUTC()
		c.startErr = fmt.Errorf("collector is nil")
		return c.startErr
	}
	if err := c.collector.Start(ctx, sink); err != nil {
		c.started = false
		c.startErr = err
		c.updatedAt = timeNowUTC()
		return err
	}
	c.started = true
	c.startErr = nil
	c.updatedAt = timeNowUTC()
	return nil
}

func (c *collectorBinding) close() error {
	if c == nil || c.collector == nil {
		return nil
	}
	return c.collector.Close()
}

func (c *collectorBinding) healthSnapshot() hidscollector.HealthSnapshot {
	if c == nil {
		return hidscollector.HealthSnapshot{
			Name:      "collector",
			Status:    "degraded",
			Message:   "collector binding is nil",
			UpdatedAt: timeNowUTC(),
		}
	}

	if c.startErr != nil {
		return hidscollector.HealthSnapshot{
			Name:      c.kind,
			Backend:   c.backend,
			Status:    "degraded",
			Message:   fmt.Sprintf("%s failed to start: %v", c.label(), c.startErr),
			UpdatedAt: c.updatedAt,
			Detail: map[string]any{
				"stats": map[string]any{
					"received": 0,
					"emitted":  0,
					"errors":   1,
					"dropped":  0,
				},
				"startup_error": fmt.Sprintf("%v", c.startErr),
			},
		}
	}

	snapshot := hidscollector.HealthSnapshot{
		Name:      c.kind,
		Backend:   c.backend,
		Status:    "running",
		Message:   fmt.Sprintf("%s collector is active", c.label()),
		UpdatedAt: c.updatedAt,
	}
	if reporter, ok := c.collector.(hidscollector.HealthReporter); ok {
		snapshot = reporter.HealthSnapshot()
	}
	if strings.TrimSpace(snapshot.Name) == "" {
		snapshot.Name = c.kind
	}
	if strings.TrimSpace(snapshot.Backend) == "" {
		snapshot.Backend = c.backend
	}
	if strings.TrimSpace(snapshot.Status) == "" {
		snapshot.Status = "running"
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = timeNowUTC()
	}
	return snapshot
}

func buildCollectors(spec model.DesiredSpec) ([]collectorBinding, error) {
	collectors := make([]collectorBinding, 0, 4)

	if spec.Collectors.Process.Enabled {
		if spec.Collectors.Process.Backend != model.CollectorBackendEBPF {
			return nil, &model.ValidationError{
				Field:  "collectors.process.backend",
				Reason: "must be ebpf",
			}
		}
		collectors = append(collectors, collectorBinding{
			kind:      "process",
			backend:   model.CollectorBackendEBPF,
			collector: hidsebpf.NewProcess(),
		})
	}
	if spec.Collectors.Network.Enabled {
		if spec.Collectors.Network.Backend != model.CollectorBackendEBPF {
			return nil, &model.ValidationError{
				Field:  "collectors.network.backend",
				Reason: "must be ebpf",
			}
		}
		collectors = append(collectors, collectorBinding{
			kind:      "network",
			backend:   model.CollectorBackendEBPF,
			collector: hidsebpf.NewNetwork(),
		})
	}
	if spec.Collectors.File.Enabled {
		if spec.Collectors.File.Backend != model.CollectorBackendFileWatch {
			return nil, &model.ValidationError{
				Field:  "collectors.file.backend",
				Reason: "must be filewatch",
			}
		}
		collectors = append(collectors, collectorBinding{
			kind:      "file",
			backend:   model.CollectorBackendFileWatch,
			collector: filewatch.New(spec.Collectors.File),
		})
	}
	if spec.Collectors.Audit.Enabled {
		if spec.Collectors.Audit.Backend != model.CollectorBackendAuditd {
			return nil, &model.ValidationError{
				Field:  "collectors.audit.backend",
				Reason: "must be auditd",
			}
		}
		collectors = append(collectors, collectorBinding{
			kind:      "audit",
			backend:   model.CollectorBackendAuditd,
			collector: auditd.New(),
		})
	}
	if len(collectors) == 0 {
		return nil, &model.ValidationError{
			Field:  "collectors",
			Reason: "at least one collector must be enabled",
		}
	}
	return collectors, nil
}
