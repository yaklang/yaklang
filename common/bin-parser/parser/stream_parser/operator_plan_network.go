package stream_parser

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

// Inspect only the default scalar projection during admission. Failed reads,
// custom output and custom formatters go to the VM before any callback or write.
func planScalar(node *base.Node) (value any, ok bool) {
	defer func() {
		if recover() != nil {
			value, ok = nil, false
		}
	}()
	if node == nil || node.Cfg.Has("out") || !NodeHasResult(node) ||
		node.Ctx.Has("formatter") && node.Ctx.GetString("formatter") != "default" {
		return nil, false
	}
	return GetNodeResult(node), true
}

const boundedDispatchPrefix = `protocol = getNodeResult("@Internet Protocol/Protocol").Value
total = getNodeResult("@Internet Protocol/Total Length").Value`
const boundedDispatchMiddle = `l = total - getNode("@Internet Protocol").Length()
var node
switch protocol {`

// Bounded protocol-number routing. Guarded branches retain their complete Yak
// implementation; their protocol numbers are excluded before executing a plan.
// An admitted route executes the same NewSubNode/SetMaxLength/Process calls.
// This recognizes a whole program, including all guards, cases and the suffix.
func compileBoundedDispatchPlan(tokens string) *operatorPlan {
	prefix := planTokens(boundedDispatchPrefix) + " "
	if !strings.HasPrefix(tokens, prefix) {
		return nil
	}
	parts := planLex(strings.TrimPrefix(tokens, prefix))
	legacy := make(map[uint64]bool)
	consumeGuards := func() bool {
		for len(parts) > 0 && parts[0] == "if" {
			brace := 1
			for brace < len(parts) && parts[brace] != "{" {
				brace++
			}
			if brace == len(parts) {
				return false
			}
			for _, condition := range strings.Split(strings.Join(parts[1:brace], " "), " || ") {
				name, literal, ok := strings.Cut(condition, " == ")
				key, err := strconv.ParseUint(literal, 0, 64)
				if !ok || name != "protocol" || err != nil {
					return false
				}
				legacy[key] = true
			}
			end := planClosingBrace(parts, brace)
			if end < 0 {
				return false
			}
			parts = parts[end+1:]
		}
		return true
	}
	if !consumeGuards() {
		return nil
	}
	// This local is used only in excluded guarded branches. No arbitrary
	// assignment, call or else block can be silently skipped here.
	if len(parts) >= 3 && strings.Join(parts[:3], " ") == "dccpDestination = nil" {
		parts = parts[3:]
	}
	if !consumeGuards() {
		return nil
	}
	middle := planLex(boundedDispatchMiddle)
	if len(parts) < len(middle) || strings.Join(parts[:len(middle)], " ") != strings.Join(middle, " ") {
		return nil
	}
	parts = parts[len(middle):]
	choices := make(map[uint64]sequenceStep)
	seen := make(map[uint64]bool)
	for len(parts) >= 3 && parts[0] == "case" && parts[2] == ":" {
		key, err := strconv.ParseUint(parts[1], 0, 64)
		if err != nil || seen[key] {
			return nil
		}
		seen[key] = true
		parts = parts[3:]
		end, depth := 0, 0
		for end < len(parts) {
			token := parts[end]
			if depth == 0 && (token == "case" || token == "default" || token == "}") {
				break
			}
			if token == "{" {
				depth++
			} else if token == "}" {
				depth--
			}
			end++
		}
		if end == len(parts) {
			return nil
		}
		step, ok := boundedDispatchStep(parts[:end])
		if !ok {
			legacy[key] = true
		} else {
			choices[key] = step
		}
		parts = parts[end:]
	}
	if len(parts) < 11 || parts[0] != "default" || parts[1] != ":" {
		return nil
	}
	fallback, ok := boundedDispatchStep(parts[2:10])
	if !ok || parts[10] != "}" {
		return nil
	}
	parts = parts[11:]
	lengthCall := planLex("node.SetMaxLength(l)")
	if len(parts) < len(lengthCall) || strings.Join(parts[:len(lengthCall)], " ") != strings.Join(lengthCall, " ") {
		return nil
	}
	parts = parts[len(lengthCall):]
	if !consumeGuards() || strings.Join(parts, " ") != planTokens("node.Process()") {
		return nil
	}
	for _, step := range choices {
		if strings.Count(tokens, planTokens(step.expression)) != 1 {
			return nil
		}
	}
	if strings.Count(tokens, planTokens(fallback.expression)) != 1 {
		return nil
	}
	return &operatorPlan{kind: "bounded-dispatch", run: func(e *planExecution) bool {
		protocolNode := getNodeByPath(e.this.origin, "@Internet Protocol/Protocol")
		totalNode := getNodeByPath(e.this.origin, "@Internet Protocol/Total Length")
		pv, _ := planScalar(protocolNode)
		tv, _ := planScalar(totalNode)
		protocol, pok := pv.(uint8)
		total, tok := tv.(uint16)
		if !pok || !tok || legacy[uint64(protocol)] {
			return false
		}
		for _, node := range []*base.Node{protocolNode, totalNode} {
			e.at("getNodeResult(" + strconv.Quote("@Internet Protocol/"+node.Name) + ")")
			if _, err := node.Result(); err != nil {
				panic(err)
			}
		}
		e.at(`getNode("@Internet Protocol")`)
		parent := getNodeByPath(e.this.origin, "@Internet Protocol")
		if parent == nil {
			panic("node not found")
		}
		e.at(`getNode("@Internet Protocol").Length()`)
		length := uint64(total) - convertOperatorNode(parent, e.this.operator).Length()
		step, found := choices[uint64(protocol)]
		if !found {
			step = fallback
		}
		e.at(step.expression)
		node := e.this.NewSubNode(step.name)
		e.at("node.SetMaxLength(l)")
		node.SetMaxLength(length)
		e.at("node.Process()")
		node.processDiscard()
		return true
	}}
}

func boundedDispatchStep(parts []string) (sequenceStep, bool) {
	if len(parts) != 8 || strings.Join(parts[:6], " ") != "node = this . NewSubNode (" || parts[7] != ")" {
		return sequenceStep{}, false
	}
	name, err := strconv.Unquote(parts[6])
	return sequenceStep{name: name, expression: "this.NewSubNode(" + parts[6] + ")"}, err == nil
}

func planClosingBrace(parts []string, start int) int {
	depth := 0
	for i := start; i < len(parts); i++ {
		switch parts[i] {
		case "{":
			depth++
		case "}":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

const optionsLoopSource = `this.SetMaxLength(getNodeResult("../Header Length").Value*4 - 20)
for {
 ele = this.NewElement()
 if ele.GetMaxLength() == 0 { break }
 ele.Process()
 if ele.GetSubNode("Kind").Result().Value == 0 { break }
}
if this.Length() < this.GetMaxLength() { this.NewEmptyNode().Process() }`

func compileOptionsLoopPlan(tokens string) *operatorPlan {
	if tokens != planTokens(optionsLoopSource) {
		return nil
	}
	return &operatorPlan{kind: "options-loop", run: func(e *planExecution) bool {
		header := getNodeByPath(e.this.origin, "../Header Length")
		value, _ := planScalar(header)
		length, ok := value.(uint8)
		if !ok {
			return false
		}
		e.at(`getNodeResult("../Header Length")`)
		if _, err := header.Result(); err != nil {
			panic(err)
		}
		e.at(`this.SetMaxLength(getNodeResult("../Header Length").Value*4 - 20)`)
		e.this.SetMaxLength(uint64(length)*4 - 20)
		for {
			e.at("this.NewElement()")
			ele := e.this.NewElement()
			e.at("ele.GetMaxLength()")
			if ele.GetMaxLength() == 0 {
				break
			}
			e.at("ele.Process()")
			ele.processDiscard()
			e.at(`ele.GetSubNode("Kind")`)
			kind := ele.GetSubNode("Kind")
			e.at(`ele.GetSubNode("Kind").Result()`)
			result := kind.Result().(*base.NodeValue)
			if yakvm.NewAutoValue(result.Value).Equal(yakvm.NewIntValue(0)) {
				break
			}
		}
		e.at("this.Length()")
		consumed := e.this.Length()
		e.at("this.GetMaxLength()")
		if consumed < e.this.GetMaxLength() {
			e.at("this.NewEmptyNode()")
			node := e.this.NewEmptyNode()
			e.at("this.NewEmptyNode().Process()")
			node.processDiscard()
		}
		return true
	}}
}
