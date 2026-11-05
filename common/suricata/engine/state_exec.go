package engine

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/suricata/state"
)

type stateResult struct {
	qualified bool
	emit      bool
}

func (e *StatefulEngine) applyState(plan *CompiledRulePlan, event InspectionEvent) (stateResult, error) {
	var predicates []*rule.FlowBitsOption
	var effects []*rule.FlowBitsOption
	for index := range plan.Options {
		option := plan.Options[index].FlowBits
		if option == nil || option.Operation == rule.FlowBitsNoAlert {
			continue
		}
		switch option.Operation {
		case rule.FlowBitsIsSet, rule.FlowBitsIsNotSet:
			predicates = append(predicates, option)
		case rule.FlowBitsSet, rule.FlowBitsUnset, rule.FlowBitsToggle:
			effects = append(effects, option)
		default:
			return stateResult{}, fmt.Errorf("unsupported flowbits operation %q", option.Operation)
		}
	}
	if len(predicates) == 0 && len(effects) == 0 && plan.Threshold == nil {
		return stateResult{qualified: true, emit: true}, nil
	}
	key, err := stateFlowKey(e.epoch, event.Flow)
	if err != nil {
		return stateResult{}, err
	}
	stateEffects := make([]func(*state.FlowState) error, 0, len(effects))
	for _, option := range effects {
		current := option
		stateEffects = append(stateEffects, func(flow *state.FlowState) error {
			switch current.Operation {
			case rule.FlowBitsSet:
				return flow.SetBit(current.Name)
			case rule.FlowBitsUnset:
				return flow.UnsetBit(current.Name)
			case rule.FlowBitsToggle:
				_, err := flow.ToggleBit(current.Name)
				return err
			default:
				return fmt.Errorf("unsupported flowbits effect %q", current.Operation)
			}
		})
	}
	matched, err := e.store.EvaluateAt(key, eventTime(event), func(view state.FlowView) (bool, error) {
		for _, option := range predicates {
			set, err := view.TestBit(option.Name)
			if err != nil {
				return false, err
			}
			if option.Operation == rule.FlowBitsIsSet && !set || option.Operation == rule.FlowBitsIsNotSet && set {
				return false, nil
			}
		}
		return true, nil
	}, stateEffects...)
	if err != nil || !matched {
		return stateResult{}, err
	}
	if plan.Threshold == nil {
		return stateResult{qualified: true, emit: true}, nil
	}
	thresholdKey, err := e.thresholdKey(plan, key, event)
	if err != nil {
		return stateResult{}, err
	}
	thresholdResult, err := e.store.CheckThresholdAt(thresholdKey, eventTime(event), thresholdSpec(plan.Threshold))
	if err != nil {
		return stateResult{}, err
	}
	return stateResult{qualified: true, emit: thresholdResult.Alert}, nil
}

func thresholdSpec(config *rule.ThresholdingConfig) state.ThresholdSpec {
	mode := state.DetectionFilter
	if config.Keyword == "threshold" {
		switch config.Type {
		case "limit":
			mode = state.ThresholdLimit
		case "threshold":
			mode = state.ThresholdEvery
		case "both":
			mode = state.ThresholdBoth
		}
	}
	return state.ThresholdSpec{Mode: mode, Count: uint64(config.Count), Window: time.Duration(config.Seconds) * time.Second}
}

func (e *StatefulEngine) thresholdKey(plan *CompiledRulePlan, flow state.FlowKey, event InspectionEvent) (state.ThresholdKey, error) {
	ruleID := uint64(uint32(plan.Rule.Gid))<<32 | uint64(uint32(plan.Rule.Sid))
	ruleID ^= plan.Index * 0x9e3779b97f4a7c15
	source, destination := eventEndpoints(event)
	switch strings.ToLower(plan.Threshold.Track) {
	case "by_rule":
		return state.GlobalThresholdKey(e.epoch, flow.Domain, ruleID), nil
	case "by_src":
		if !source.IsValid() {
			return state.ThresholdKey{}, fmt.Errorf("threshold source is unavailable")
		}
		return state.HostThresholdKey(ruleID, state.NewHostKey(e.epoch, flow.Domain, source)), nil
	case "by_dst":
		if !destination.IsValid() {
			return state.ThresholdKey{}, fmt.Errorf("threshold destination is unavailable")
		}
		return state.HostThresholdKey(ruleID, state.NewHostKey(e.epoch, flow.Domain, destination)), nil
	case "by_both":
		if !source.IsValid() || !destination.IsValid() {
			return state.ThresholdKey{}, fmt.Errorf("threshold endpoint pair is unavailable")
		}
		return state.PairThresholdKey(ruleID, state.NewPairKey(e.epoch, flow.Domain, source, destination)), nil
	default:
		return state.ThresholdKey{}, fmt.Errorf("unsupported threshold track %q", plan.Threshold.Track)
	}
}

func eventEndpoints(event InspectionEvent) (netip.Addr, netip.Addr) {
	if event.Direction == 1 {
		return event.Flow.Second.Address, event.Flow.First.Address
	}
	return event.Flow.First.Address, event.Flow.Second.Address
}

func parseStateInt(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid state integer %q: %w", raw, err)
	}
	return value, nil
}
