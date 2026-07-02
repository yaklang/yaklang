package preprocess

import (
	"strings"
	"unicode"
)

// ConditionalStack tracks active #if branches.
type ConditionalStack struct {
	frames []condFrame
	env    *MacroEnvironment
	global *MacroEnvironment
	defs   map[string]string
}

type condFrame struct {
	parentActive bool
	active       bool
	elseSeen     bool
}

func NewConditionalStack(env *MacroEnvironment, defs map[string]string) *ConditionalStack {
	return NewConditionalStackWithGlobal(env, nil, defs)
}

func NewConditionalStackWithGlobal(env, global *MacroEnvironment, defs map[string]string) *ConditionalStack {
	cs := &ConditionalStack{
		env:    env,
		global: global,
		defs:   defs,
	}
	cs.frames = append(cs.frames, condFrame{parentActive: true, active: true})
	return cs
}

func (cs *ConditionalStack) Active() bool {
	if len(cs.frames) == 0 {
		return true
	}
	return cs.frames[len(cs.frames)-1].active
}

func (cs *ConditionalStack) HandleDirective(line string) (consumed bool, skipContent bool) {
	name := DirectiveName(line)
	switch name {
	case "if":
		expr := DirectiveRest(line)
		val := EvalPreprocessorCondition(expr, cs.env, cs.global, cs.defs)
		cs.push(val)
		return true, true
	case "ifdef":
		macro := ppFirstIdent(DirectiveRest(line))
		val := cs.isMacroDefined(macro)
		cs.push(val)
		return true, true
	case "ifndef":
		macro := ppFirstIdent(DirectiveRest(line))
		val := !cs.isMacroDefined(macro)
		cs.push(val)
		return true, true
	case "elif":
		if len(cs.frames) == 0 {
			return true, true
		}
		top := &cs.frames[len(cs.frames)-1]
		if !top.parentActive {
			top.active = false
			top.elseSeen = true
			return true, true
		}
		if top.active || top.elseSeen {
			top.active = false
			top.elseSeen = true
			return true, true
		}
		val := EvalPreprocessorCondition(DirectiveRest(line), cs.env, cs.global, cs.defs)
		top.active = val
		top.elseSeen = true
		return true, true
	case "else":
		if len(cs.frames) == 0 {
			return true, true
		}
		top := &cs.frames[len(cs.frames)-1]
		if !top.parentActive {
			top.active = false
			top.elseSeen = true
			return true, true
		}
		top.active = !top.elseSeen && !top.active
		top.elseSeen = true
		return true, true
	case "endif":
		if len(cs.frames) > 1 {
			cs.frames = cs.frames[:len(cs.frames)-1]
		}
		return true, true
	default:
		return false, false
	}
}

func (cs *ConditionalStack) push(active bool) {
	parentActive := cs.Active()
	cs.frames = append(cs.frames, condFrame{
		parentActive: parentActive,
		active:       parentActive && active,
	})
}

func (cs *ConditionalStack) isMacroDefined(name string) bool {
	if name == "" {
		return false
	}
	if _, ok := cs.defs[name]; ok {
		return true
	}
	if cs.env != nil && cs.env.IsDefined(name) {
		return true
	}
	if cs.global != nil && cs.global.IsDefined(name) {
		return true
	}
	return false
}

func ppFirstIdent(s string) string {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) {
		r := rune(s[i])
		if r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			i++
			continue
		}
		break
	}
	return s[:i]
}
