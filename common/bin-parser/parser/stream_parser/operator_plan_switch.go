package stream_parser

import (
	"strconv"
	"strings"
)

// A field discriminator is data: a field path, integer-to-node table, and
// default action. The optional IEEE 802.3 length branch is checked explicitly.
// No speculative protocol prefix checks or stronger validation are introduced.
func compileFieldDispatchPlan(tokens string) *operatorPlan {
	variable, rest, ok := strings.Cut(tokens, " = getNodeResult ( ")
	if !ok || variable == "" || strings.Contains(variable, " ") {
		return nil
	}
	quoted, rest, ok := strings.Cut(rest, " ) . Value ")
	path, err := strconv.Unquote(quoted)
	if !ok || err != nil {
		return nil
	}
	llc := planTokens(strings.ReplaceAll(`if VALUE < 0x0600 { this.SetMaxLength(VALUE); this.ProcessByType("LLC"); return }`, "VALUE", variable))
	hasLLC := strings.HasPrefix(rest, llc+" ")
	if hasLLC {
		rest = strings.TrimPrefix(rest, llc+" ")
	}
	prefix := "switch " + variable + " { "
	if !strings.HasPrefix(rest, prefix) || !strings.HasSuffix(rest, " }") {
		return nil
	}
	rest = strings.TrimSuffix(strings.TrimPrefix(rest, prefix), " }")
	choices := make(map[uint64]sequenceStep)
	for strings.HasPrefix(rest, "case ") {
		key, body, ok := strings.Cut(strings.TrimPrefix(rest, "case "), " : ")
		value, err := strconv.ParseUint(key, 0, 64)
		if !ok || err != nil {
			return nil
		}
		end := strings.Index(body, " case ")
		if d := strings.Index(body, " default : "); d >= 0 && (end < 0 || d < end) {
			end = d
		}
		if end < 0 {
			return nil
		}
		step, ok := singleNodeCall(body[:end])
		if !ok {
			return nil
		}
		if _, exists := choices[value]; exists {
			return nil
		}
		choices[value], rest = step, body[end+1:]
	}
	if !strings.HasPrefix(rest, "default : ") {
		return nil
	}
	defaultBody := strings.TrimPrefix(rest, "default : ")
	peek := defaultBody == planTokens(fieldDispatchPeekDefault)
	fallback, directDefault := singleNodeCall(defaultBody)
	if !peek && !directDefault {
		return nil
	}
	fieldCall := "getNodeResult(" + quoted + ")"
	return &operatorPlan{kind: "field-dispatch", run: func(e *planExecution) bool {
		e.at(fieldCall)
		n := getNodeByPath(e.this.origin, path)
		if n == nil {
			panic("node not found")
		}
		if n.Cfg.Has("out") {
			return false
		}
		result, err := n.Result()
		if err != nil {
			panic(err)
		}
		var value uint64
		switch v := result.Value.(type) {
		case uint8:
			value = uint64(v)
		case uint16:
			value = uint64(v)
		case uint32:
			value = uint64(v)
		case uint64:
			value = v
		default:
			return false
		}
		if hasLLC && value < 0x600 {
			e.at("this.SetMaxLength(" + variable + ")")
			e.this.SetMaxLength(value)
			e.at(`this.ProcessByType("LLC")`)
			e.this.processByTypeDiscard("LLC")
			return true
		}
		step, found := choices[value]
		if !found {
			if peek {
				e.at(`this.TryProcessByType("PeekByte")`)
				op := e.this.tryProcessByTypeDiscard("PeekByte")
				e.at("op.Recovery()")
				op.recovery()
				if op.ok {
					e.at(`this.ProcessByType("Next Protocol Data")`)
					e.this.processByTypeDiscard("Next Protocol Data")
				}
				return true
			}
			step = fallback
		}
		e.at(step.expression)
		if step.method == "ProcessByType" {
			e.this.processByTypeDiscard(step.name)
		} else {
			e.this.processSubNodeDiscard(step.name)
		}
		return true
	}}
}

func singleNodeCall(tokens string) (sequenceStep, bool) {
	for _, method := range []string{"ProcessByType", "ProcessSubNode"} {
		prefix := "this . " + method + " ( "
		if !strings.HasPrefix(tokens, prefix) || !strings.HasSuffix(tokens, " )") {
			continue
		}
		quoted := strings.TrimSuffix(strings.TrimPrefix(tokens, prefix), " )")
		name, err := strconv.Unquote(quoted)
		if err == nil {
			return sequenceStep{method: method, name: name, expression: "this." + method + "(" + quoted + ")"}, true
		}
	}
	return sequenceStep{}, false
}

const fieldDispatchPeekDefault = `res, op = this.TryProcessByType("PeekByte")
op.Recovery()
if op.OK { this.ProcessByType("Next Protocol Data") }
return`
