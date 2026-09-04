package scannode

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const legionFocusExecutionContractSchemaV1 = "legion.focus-execution/v1"

var (
	legionFocusExecutionKeyPattern        = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	legionFocusExecutionCapabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9_.]{1,127}$`)
)

type legionFocusExecutionContract struct {
	SchemaVersion string                               `json:"schema_version"`
	Stages        []legionFocusExecutionStage          `json:"stages,omitempty"`
	Capabilities  []string                             `json:"capabilities,omitempty"`
	Results       []legionFocusExecutionResultContract `json:"results,omitempty"`

	stageSet      map[string]struct{}
	capabilitySet map[string]struct{}
	resultByCap   map[string]legionFocusExecutionResultContract
}

type legionFocusExecutionStage struct {
	Key string `json:"key"`
}

type legionFocusExecutionResultContract struct {
	Key        string `json:"key"`
	Capability string `json:"capability"`
	Kind       string `json:"kind"`
	Required   bool   `json:"required,omitempty"`
}

func parseLegionFocusExecutionContract(raw string) (*legionFocusExecutionContract, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var contract legionFocusExecutionContract
	if err := json.Unmarshal([]byte(raw), &contract); err != nil {
		return nil, fmt.Errorf("decode Focus execution contract: %w", err)
	}
	contract.SchemaVersion = strings.TrimSpace(contract.SchemaVersion)
	if contract.SchemaVersion != legionFocusExecutionContractSchemaV1 {
		return nil, fmt.Errorf("unsupported Focus execution contract schema_version %q", contract.SchemaVersion)
	}
	if len(contract.Stages) == 0 || len(contract.Stages) > 32 {
		return nil, fmt.Errorf("Focus execution contract must contain between 1 and 32 stages")
	}
	contract.stageSet = make(map[string]struct{}, len(contract.Stages))
	for index := range contract.Stages {
		key := strings.TrimSpace(contract.Stages[index].Key)
		if !legionFocusExecutionKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("Focus execution stage key %q is invalid", key)
		}
		if _, exists := contract.stageSet[key]; exists {
			return nil, fmt.Errorf("Focus execution stage key %q is duplicated", key)
		}
		contract.Stages[index].Key = key
		contract.stageSet[key] = struct{}{}
	}
	if len(contract.Capabilities) == 0 || len(contract.Capabilities) > 64 {
		return nil, fmt.Errorf("Focus execution contract must contain between 1 and 64 capabilities")
	}
	contract.capabilitySet = make(map[string]struct{}, len(contract.Capabilities))
	for index, candidate := range contract.Capabilities {
		capability := strings.TrimSpace(candidate)
		if !legionFocusExecutionCapabilityPattern.MatchString(capability) {
			return nil, fmt.Errorf("Focus execution capability %q is invalid", capability)
		}
		if _, exists := contract.capabilitySet[capability]; exists {
			return nil, fmt.Errorf("Focus execution capability %q is duplicated", capability)
		}
		contract.Capabilities[index] = capability
		contract.capabilitySet[capability] = struct{}{}
	}
	sort.Strings(contract.Capabilities)
	if len(contract.Results) > 16 {
		return nil, fmt.Errorf("Focus execution contract contains too many results")
	}
	contract.resultByCap = make(map[string]legionFocusExecutionResultContract, len(contract.Results))
	seenResults := make(map[string]struct{}, len(contract.Results))
	for index := range contract.Results {
		result := &contract.Results[index]
		result.Key = strings.TrimSpace(result.Key)
		result.Capability = strings.TrimSpace(result.Capability)
		result.Kind = strings.TrimSpace(result.Kind)
		if !legionFocusExecutionKeyPattern.MatchString(result.Key) || !legionFocusExecutionCapabilityPattern.MatchString(result.Kind) {
			return nil, fmt.Errorf("Focus execution result %q or kind %q is invalid", result.Key, result.Kind)
		}
		if _, exists := seenResults[result.Key]; exists {
			return nil, fmt.Errorf("Focus execution result key %q is duplicated", result.Key)
		}
		if _, exists := contract.capabilitySet[result.Capability]; !exists {
			return nil, fmt.Errorf("Focus execution result %q uses undeclared capability %q", result.Key, result.Capability)
		}
		if _, exists := contract.resultByCap[result.Capability]; exists {
			return nil, fmt.Errorf("Focus execution capability %q maps multiple result contracts", result.Capability)
		}
		if (result.Capability == serverFocusCapabilityRuleCandidate) != (result.Kind == legionSyntaxFlowRuleCandidateKind) {
			return nil, fmt.Errorf("SyntaxFlow rule candidates require the dedicated result.rule_candidate.v1 contract and ai_syntaxflow_rule_v1 kind")
		}
		seenResults[result.Key] = struct{}{}
		contract.resultByCap[result.Capability] = *result
	}
	if contract.allowsCapability(serverFocusCapabilityRuleCandidate) {
		if _, ok := contract.resultByCap[serverFocusCapabilityRuleCandidate]; !ok {
			return nil, fmt.Errorf("result.rule_candidate.v1 requires an immutable result contract")
		}
	}
	canonicalView := contract
	canonicalView.stageSet = nil
	canonicalView.capabilitySet = nil
	canonicalView.resultByCap = nil
	canonical, err := json.Marshal(canonicalView)
	if err != nil {
		return nil, fmt.Errorf("canonicalize Focus execution contract: %w", err)
	}
	if string(canonical) != raw {
		return nil, fmt.Errorf("Focus execution contract is not canonical JSON")
	}
	return &contract, nil
}

func (c *legionFocusExecutionContract) allowsCapability(capability string) bool {
	if c == nil {
		return false
	}
	_, ok := c.capabilitySet[strings.TrimSpace(capability)]
	return ok
}

func (c *legionFocusExecutionContract) allowsStage(stage string) bool {
	if c == nil {
		return false
	}
	_, ok := c.stageSet[strings.TrimSpace(stage)]
	return ok
}

func (c *legionFocusExecutionContract) resultForCapability(capability string) (legionFocusExecutionResultContract, bool) {
	if c == nil {
		return legionFocusExecutionResultContract{}, false
	}
	result, ok := c.resultByCap[strings.TrimSpace(capability)]
	return result, ok
}

func cloneLegionFocusExecutionContract(contract *legionFocusExecutionContract) *legionFocusExecutionContract {
	if contract == nil {
		return nil
	}
	cloned := *contract
	cloned.Stages = append([]legionFocusExecutionStage(nil), contract.Stages...)
	cloned.Capabilities = append([]string(nil), contract.Capabilities...)
	cloned.Results = append([]legionFocusExecutionResultContract(nil), contract.Results...)
	cloned.stageSet = make(map[string]struct{}, len(contract.stageSet))
	for key := range contract.stageSet {
		cloned.stageSet[key] = struct{}{}
	}
	cloned.capabilitySet = make(map[string]struct{}, len(contract.capabilitySet))
	for key := range contract.capabilitySet {
		cloned.capabilitySet[key] = struct{}{}
	}
	cloned.resultByCap = make(map[string]legionFocusExecutionResultContract, len(contract.resultByCap))
	for key, result := range contract.resultByCap {
		cloned.resultByCap[key] = result
	}
	return &cloned
}
