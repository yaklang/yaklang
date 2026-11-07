package chaosmaker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/engine"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

// StatefulEngineOption configures a stateful Suricata engine created by
// NewStatefulEngine. Options are evaluated once, before rules are compiled.
type StatefulEngineOption func(*engine.Config)

// StatefulAlert is the Yak-facing alert projection. It keeps the stable
// Suricata identity fields and endpoint/packet evidence flat, while Rule still
// exposes the complete parsed rule for advanced callers.
type StatefulAlert struct {
	Timestamp       time.Time
	SID             int
	GID             int
	Rev             int
	Message         string
	ClassType       string
	Action          string
	Protocol        string
	Target          string
	Tagged          bool
	View            string
	Direction       int
	Source          string
	SourcePort      int
	Destination     string
	DestinationPort int
	Summary         string
	EventID         uint64
	FlowID          uint64
	TCPFlowID       uint64
	TransactionID   uint64
	ResponseTo      uint64
	FlowGeneration  uint64
	PacketNumbers   []uint64
	// References retain the full capture domain, so alerts from multi-section
	// PCAPNG files or tunnels can be joined back to the right packets.
	References []pcaputil.PacketReference
	Rule       *surirule.Rule
}

// StatefulDiagnostic describes a fail-closed inspection or resource-limit
// event. It is informational and never represents a rule match.
type StatefulDiagnostic struct {
	Timestamp   time.Time
	Kind        string
	Message     string
	SID         int
	GID         int
	RuleMessage string
}

// StatefulRejectedRule explains why one parsed rule could not enter the
// stateful engine. Skipping a rule never weakens rules that do compile.
type StatefulRejectedRule struct {
	SID     int
	GID     int
	Message string
	Reason  string
}

// StatefulRuleSet is the result of LoadStatefulRules. Rules are guaranteed to
// compile in M1; Rejected records every rule that was intentionally skipped.
type StatefulRuleSet struct {
	Rules    []*surirule.Rule
	Rejected []StatefulRejectedRule
}

// LoadStatefulRules parses one .rules file or a directory, then compiles every
// rule independently. Unsupported rules and duplicate identities are reported
// in Rejected instead of aborting the whole ruleset, which is what real ET
// corpora require. Rules that are returned are known to compile in M1.
func LoadStatefulRules(path string, envs ...string) (*StatefulRuleSet, error) {
	rules, err := LoadSuricataRules(path, envs...)
	if err != nil {
		return nil, err
	}
	result := &StatefulRuleSet{
		Rules:    make([]*surirule.Rule, 0, len(rules)),
		Rejected: make([]StatefulRejectedRule, 0),
	}
	identities := make(map[[2]int]struct{}, len(rules))
	for _, current := range rules {
		identity := [2]int{current.Gid, current.Sid}
		if current.Sid <= 0 {
			result.Rejected = append(result.Rejected, StatefulRejectedRule{
				GID: current.Gid, Message: current.Message,
				Reason: "rule requires a positive sid",
			})
			continue
		}
		if _, exists := identities[identity]; exists {
			result.Rejected = append(result.Rejected, StatefulRejectedRule{
				SID: current.Sid, GID: current.Gid, Message: current.Message,
				Reason: "duplicate rule identity",
			})
			continue
		}
		if _, err := engine.CompileStatefulRule(current); err != nil {
			result.Rejected = append(result.Rejected, StatefulRejectedRule{
				SID: current.Sid, GID: current.Gid, Message: current.Message,
				Reason: err.Error(),
			})
			continue
		}
		identities[identity] = struct{}{}
		result.Rules = append(result.Rules, current)
	}
	return result, nil
}

// NewStatefulEngine compiles rules for the stateful TCP inspection engine.
// In Yak, call it as suricata.NewStatefulEngine(rules, opts...).
func NewStatefulEngine(rules []*surirule.Rule, opts ...StatefulEngineOption) (*engine.StatefulEngine, error) {
	config := engine.Config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&config)
		}
	}
	return engine.NewStatefulEngine(rules, config)
}

// engineOnAlert installs the alert callback. The callback runs synchronously
// on a capture worker goroutine; keep it fast or hand work to a channel.
func engineOnAlert(callback func(StatefulAlert)) StatefulEngineOption {
	return func(config *engine.Config) {
		if callback == nil {
			config.OnAlert = nil
			return
		}
		config.OnAlert = func(alert engine.Alert) {
			callback(newStatefulAlert(alert))
		}
	}
}

// engineOnDiagnostic installs the diagnostic callback.
func engineOnDiagnostic(callback func(StatefulDiagnostic)) StatefulEngineOption {
	return func(config *engine.Config) {
		if callback == nil {
			config.OnDiagnostic = nil
			return
		}
		config.OnDiagnostic = func(diagnostic engine.Diagnostic) {
			callback(newStatefulDiagnostic(diagnostic))
		}
	}
}

// engineEpoch isolates flowbits, thresholds and xbits between ruleset
// reloads. Use a new monotonically increasing value when replacing rules.
func engineEpoch(epoch int) StatefulEngineOption {
	return func(config *engine.Config) {
		if epoch > 0 {
			config.Epoch = uint64(epoch)
		}
	}
}

// engineLimits bounds aggregate engine resources. Recognized keys:
// maxFlows, maxStreamBytes, maxTotalStreamBytes.
func engineLimits(values map[string]int64) StatefulEngineOption {
	return func(config *engine.Config) {
		for key, value := range values {
			if value <= 0 {
				continue
			}
			switch key {
			case "maxFlows":
				config.MaxFlows = int(value)
			case "maxStreamBytes":
				config.MaxStreamBytes = int(value)
			case "maxTotalStreamBytes":
				config.MaxTotalStreamBytes = value
			}
		}
	}
}

// engineStateLimits overrides state-store limits. Recognized keys:
// maxFlows, maxFlowBitsPerFlow, maxFlowIntsPerFlow, maxXBits,
// maxThresholds, maxNameLength, shards, flowIdleTimeoutSeconds.
func engineStateLimits(values map[string]int64) StatefulEngineOption {
	return func(config *engine.Config) {
		current := config.State
		if current.Shards == 0 {
			current = state.DefaultConfig()
		}
		for key, value := range values {
			if value <= 0 {
				continue
			}
			switch key {
			case "maxFlows":
				current.MaxFlows = int(value)
			case "maxFlowBitsPerFlow":
				current.MaxFlowBitsPerFlow = int(value)
			case "maxFlowIntsPerFlow":
				current.MaxFlowIntsPerFlow = int(value)
			case "maxXBits":
				current.MaxXBits = int(value)
			case "maxThresholds":
				current.MaxThresholds = int(value)
			case "maxNameLength":
				current.MaxNameLength = int(value)
			case "shards":
				current.Shards = int(value)
			case "flowIdleTimeoutSeconds":
				current.FlowIdleTimeout = time.Duration(value) * time.Second
			}
		}
		config.State = current
	}
}

func newStatefulAlert(alert engine.Alert) StatefulAlert {
	view := ""
	switch alert.View {
	case engine.ViewPhysicalPacket:
		view = "packet"
	case engine.ViewNormalizedPacket:
		view = "normalized-packet"
	case engine.ViewStream:
		view = "stream"
	case engine.ViewTransaction:
		view = "transaction"
	case engine.ViewFlowCreated:
		view = "flow-created"
	case engine.ViewFlowClosed:
		view = "flow-closed"
	}
	result := StatefulAlert{
		Timestamp:       alert.Timestamp,
		View:            view,
		Direction:       alert.Direction,
		Protocol:        alert.Protocol,
		Target:          alert.Target,
		Tagged:          alert.Tagged,
		Summary:         alert.Summary,
		EventID:         alert.EventID,
		FlowID:          alert.FlowID,
		TCPFlowID:       alert.TCPFlowID,
		TransactionID:   alert.TransactionID,
		ResponseTo:      alert.ResponseTo,
		FlowGeneration:  alert.Flow.Key.Generation,
		Source:          alert.Flow.First.Address.String(),
		SourcePort:      int(alert.Flow.First.Port),
		Destination:     alert.Flow.Second.Address.String(),
		DestinationPort: int(alert.Flow.Second.Port),
		References:      append([]pcaputil.PacketReference(nil), alert.References...),
		Rule:            alert.Rule,
	}
	if alert.Rule != nil {
		result.SID = alert.Rule.Sid
		result.GID = alert.Rule.Gid
		result.Rev = alert.Rule.Rev
		result.Message = alert.Rule.Message
		result.ClassType = alert.Rule.ClassType
		result.Action = alert.Rule.Action
	}
	for _, reference := range alert.References {
		result.PacketNumbers = append(result.PacketNumbers, reference.Number)
	}
	return result
}

// LoadSuricataRules reads one .rules file or every .rules file under a
// directory. Files are parsed in lexical order so rule order and flowbits
// writers stay deterministic. Tolerant parsing is available directly through
// ParseSuricata; this helper surfaces parse failures instead of silently
// dropping a file.
func LoadSuricataRules(path string, envs ...string) ([]*surirule.Rule, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	var files []string
	if info.IsDir() {
		err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if strings.HasSuffix(entry.Name(), ".rules") {
				files = append(files, current)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
	} else {
		files = []string{path}
	}
	var rules []*surirule.Rule
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		parsed, err := surirule.Parse(string(raw), envs...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		rules = append(rules, parsed...)
	}
	return rules, nil
}

func newStatefulDiagnostic(diagnostic engine.Diagnostic) StatefulDiagnostic {
	result := StatefulDiagnostic{
		Timestamp: diagnostic.Timestamp,
		Kind:      diagnostic.Kind,
		Message:   diagnostic.Message,
	}
	if diagnostic.Rule != nil {
		result.SID = diagnostic.Rule.Sid
		result.GID = diagnostic.Rule.Gid
		result.RuleMessage = diagnostic.Rule.Message
	}
	return result
}
