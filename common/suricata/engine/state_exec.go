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

type xbitPredicateSpec struct {
	key  state.XBitKey
	want bool
}

type xbitEffectSpec struct {
	key       state.XBitKey
	operation string
	ttl       time.Duration
}

type flowIntPredicateSpec struct {
	name    string
	compare state.IntCompare
	rhs     int64
	isset   bool
	negate  bool
}

type flowIntEffectSpec struct {
	name      string
	operation string
	rhs       int64
}

func (e *StatefulEngine) xbitKey(flow state.FlowKey, option *rule.StateValueOption, event InspectionEvent) (state.XBitKey, error) {
	source, destination := eventEndpoints(event)
	switch strings.ToLower(strings.TrimSpace(option.Track)) {
	case "ip_src":
		if !source.IsValid() {
			return state.XBitKey{}, fmt.Errorf("xbits ip_src source is unavailable")
		}
		return state.HostXBitKey(state.NewHostKey(flow.Epoch, flow.Domain, source), option.Name), nil
	case "ip_dst":
		if !destination.IsValid() {
			return state.XBitKey{}, fmt.Errorf("xbits ip_dst destination is unavailable")
		}
		return state.HostXBitKey(state.NewHostKey(flow.Epoch, flow.Domain, destination), option.Name), nil
	case "ip_pair":
		if !source.IsValid() || !destination.IsValid() {
			return state.XBitKey{}, fmt.Errorf("xbits ip_pair endpoints are unavailable")
		}
		return state.PairXBitKey(state.NewPairKey(flow.Epoch, flow.Domain, source, destination), option.Name), nil
	default:
		return state.XBitKey{}, fmt.Errorf("unsupported xbits track %q", option.Track)
	}
}

func xbitTTL(option *rule.StateValueOption) time.Duration {
	if option == nil || option.Expire == nil {
		return 0
	}
	return time.Duration(*option.Expire) * time.Second
}

func (e *StatefulEngine) applyState(plan *CompiledRulePlan, event InspectionEvent) (stateResult, error) {
	var predicates []*rule.FlowBitsOption
	var effects []*rule.FlowBitsOption
	var xbitOptions []*rule.StateValueOption
	var flowIntOptions []*rule.StateValueOption
	for index := range plan.Options {
		if plan.Options[index].XBits != nil {
			xbitOptions = append(xbitOptions, plan.Options[index].XBits)
		}
		if plan.Options[index].FlowInt != nil {
			flowIntOptions = append(flowIntOptions, plan.Options[index].FlowInt)
		}
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
	flowDependent := len(predicates) > 0 || len(effects) > 0 || len(xbitOptions) > 0 || len(flowIntOptions) > 0
	if !flowDependent && plan.Threshold != nil {
		// Threshold-only rules do not need a flow state entry. This also lets
		// IP/ICMP rules use by_src/by_dst/by_both counters without a transport
		// flow generation.
		flowKey := state.FlowKey{Epoch: e.epoch, Domain: stateCaptureDomain(event.Flow.Key.Domain)}
		thresholdKey, err := e.thresholdKey(plan, flowKey, event)
		if err != nil {
			return stateResult{}, err
		}
		thresholdResult, err := e.store.CheckThresholdAt(thresholdKey, eventTime(event), thresholdSpec(plan.Threshold))
		if err != nil {
			return stateResult{}, err
		}
		return stateResult{qualified: true, emit: thresholdResult.Alert}, nil
	}
	if !flowDependent {
		return stateResult{qualified: true, emit: true}, nil
	}
	key, err := stateFlowKey(e.epoch, event.Flow)
	if err != nil {
		return stateResult{}, err
	}
	var xbitPredicates []xbitPredicateSpec
	var xbitEffects []xbitEffectSpec
	for _, option := range xbitOptions {
		xbitKey, err := e.xbitKey(key, option, event)
		if err != nil {
			return stateResult{}, err
		}
		switch option.Operation {
		case "isset":
			xbitPredicates = append(xbitPredicates, xbitPredicateSpec{key: xbitKey, want: true})
		case "isnotset":
			xbitPredicates = append(xbitPredicates, xbitPredicateSpec{key: xbitKey, want: false})
		case "set", "unset", "toggle":
			xbitEffects = append(xbitEffects, xbitEffectSpec{key: xbitKey, operation: option.Operation, ttl: xbitTTL(option)})
		default:
			return stateResult{}, fmt.Errorf("unsupported xbits operation %q", option.Operation)
		}
	}
	var flowIntPredicates []flowIntPredicateSpec
	var flowIntEffects []flowIntEffectSpec
	for _, option := range flowIntOptions {
		operation := strings.ToLower(strings.TrimSpace(option.Operation))
		switch operation {
		case "isset", "isnotset":
			flowIntPredicates = append(flowIntPredicates, flowIntPredicateSpec{
				name:   option.Name,
				isset:  true,
				negate: operation == "isnotset",
			})
		case "+", "-", "=":
			rhs, err := strconv.ParseInt(strings.TrimSpace(option.Arguments[0]), 10, 64)
			if err != nil {
				return stateResult{}, err
			}
			flowIntEffects = append(flowIntEffects, flowIntEffectSpec{name: option.Name, operation: operation, rhs: rhs})
		case "==", "!=", "<", ">", "<=", ">=":
			rhs, err := strconv.ParseInt(strings.TrimSpace(option.Arguments[0]), 10, 64)
			if err != nil {
				return stateResult{}, err
			}
			compare := state.IntEqual
			switch operation {
			case "!=":
				compare = state.IntNotEqual
			case "<":
				compare = state.IntLess
			case ">":
				compare = state.IntGreater
			case "<=":
				compare = state.IntLessOrEqual
			case ">=":
				compare = state.IntGreaterOrEqual
			}
			flowIntPredicates = append(flowIntPredicates, flowIntPredicateSpec{name: option.Name, compare: compare, rhs: rhs})
		default:
			return stateResult{}, fmt.Errorf("unsupported flowint operation %q", option.Operation)
		}
	}
	stateEffects := make([]func(*state.FlowState) error, 0, len(effects)+len(flowIntEffects))
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
	for _, effect := range flowIntEffects {
		current := effect
		stateEffects = append(stateEffects, func(flow *state.FlowState) error {
			switch current.operation {
			case "=":
				return flow.SetInt(current.name, current.rhs)
			case "+":
				_, err := flow.AddInt(current.name, current.rhs)
				return err
			case "-":
				_, err := flow.AddInt(current.name, -current.rhs)
				return err
			default:
				return fmt.Errorf("unsupported flowint effect %q", current.operation)
			}
		})
	}
	now := eventTime(event)
	for _, predicate := range xbitPredicates {
		set, err := e.store.TestXBitAt(predicate.key, now)
		if err != nil {
			return stateResult{}, err
		}
		if set != predicate.want {
			return stateResult{}, nil
		}
	}
	matched, err := e.store.EvaluateAt(key, now, func(view state.FlowView) (bool, error) {
		for _, option := range predicates {
			set, err := view.TestBit(option.Name)
			if err != nil {
				return false, err
			}
			if option.Operation == rule.FlowBitsIsSet && !set || option.Operation == rule.FlowBitsIsNotSet && set {
				return false, nil
			}
		}
		for _, predicate := range flowIntPredicates {
			if predicate.isset {
				_, exists, err := view.GetInt(predicate.name)
				if err != nil {
					return false, err
				}
				if exists == predicate.negate {
					return false, nil
				}
				continue
			}
			matched, err := view.CompareInt(predicate.name, predicate.compare, predicate.rhs)
			if err != nil {
				return false, err
			}
			if !matched {
				return false, nil
			}
		}
		return true, nil
	}, stateEffects...)
	if err != nil || !matched {
		return stateResult{}, err
	}
	for _, effect := range xbitEffects {
		switch effect.operation {
		case "set":
			if err := e.store.SetXBitAt(effect.key, effect.ttl, now); err != nil {
				return stateResult{}, err
			}
		case "unset":
			if _, err := e.store.UnsetXBit(effect.key); err != nil {
				return stateResult{}, err
			}
		case "toggle":
			if _, err := e.store.ToggleXBitAt(effect.key, effect.ttl, now); err != nil {
				return stateResult{}, err
			}
		default:
			return stateResult{}, fmt.Errorf("unsupported xbits effect %q", effect.operation)
		}
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
