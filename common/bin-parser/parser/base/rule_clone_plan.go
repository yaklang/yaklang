package base

import yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"

// The decoded document is immutable. Copy its scalar associations together,
// then replace just the mutable maps/lists with invocation-owned copies.
// Keys can themselves be YAML collections, and must also remain independent.
type ruleClonePlan struct {
	mapping  yaml.MapSlice
	sequence []any
	isMap    bool
	edits    []ruleCloneEdit
}
type ruleCloneEdit struct {
	index int
	key   bool
	plan  *ruleClonePlan
}

func compileRuleClone(value any) *ruleClonePlan {
	p := &ruleClonePlan{}
	switch v := value.(type) {
	case yaml.MapSlice:
		p.mapping, p.isMap = v, true
		for i, item := range v {
			if child := compileRuleClone(item.Key); child != nil {
				p.edits = append(p.edits, ruleCloneEdit{i, true, child})
			}
			if child := compileRuleClone(item.Value); child != nil {
				p.edits = append(p.edits, ruleCloneEdit{i, false, child})
			}
		}
	case []any:
		p.sequence = v
		for i, item := range v {
			if child := compileRuleClone(item); child != nil {
				p.edits = append(p.edits, ruleCloneEdit{i, false, child})
			}
		}
	default:
		return nil
	}
	return p
}

func (p *ruleClonePlan) clone() any {
	if p.isMap {
		if p.mapping == nil {
			return yaml.MapSlice(nil)
		}
		v := make(yaml.MapSlice, len(p.mapping))
		copy(v, p.mapping)
		for _, edit := range p.edits {
			if edit.key {
				v[edit.index].Key = edit.plan.clone()
			} else {
				v[edit.index].Value = edit.plan.clone()
			}
		}
		return v
	}
	if p.sequence == nil {
		return []any(nil)
	}
	v := make([]any, len(p.sequence))
	copy(v, p.sequence)
	for _, edit := range p.edits {
		v[edit.index] = edit.plan.clone()
	}
	return v
}
