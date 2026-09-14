package stream_parser

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

type portPlanBranch struct {
	priority   int
	candidates []string
	legacy     bool
}

type portDispatchPlan struct {
	transport           string
	source, destination map[uint16]*portPlanBranch
	otherwise           *portPlanBranch
}

// Port maps retain source-order priority. Looking up src/dst independently and
// taking the earliest branch is equivalent to the original if/else chain,
// including conflicting well-known ports and direction-specific UDP rules.
func compilePortDispatchPlan(tokens string) *operatorPlan {
	p := &portDispatchPlan{source: make(map[uint16]*portPlanBranch), destination: make(map[uint16]*portPlanBranch)}
	var rest string
	for _, transport := range []string{"TCP", "UDP"} {
		prefix := planTokens(strings.ReplaceAll(portDispatchPrefix, "$TRANSPORT", transport)) + " "
		if strings.HasPrefix(tokens, prefix) {
			p.transport, rest = transport, strings.TrimPrefix(tokens, prefix)
			break
		}
	}
	if p.transport == "" {
		return nil
	}
	defaultBody, rest, ok := strings.Cut(rest, " if ")
	if !ok {
		return nil
	}
	candidates, ok := portCandidates(defaultBody)
	if !ok {
		return nil
	}
	p.otherwise = &portPlanBranch{candidates: candidates}
	parts := planLex(rest)
	for priority := 0; ; priority++ {
		brace := -1
		for i, token := range parts {
			if token == "{" {
				brace = i
				break
			}
		}
		if brace < 0 {
			return nil
		}
		condition := strings.Join(parts[:brace], " ")
		parts = parts[brace+1:]
		// Conditions have a closed grammar. Opaque bodies are never executed by
		// a plan; the selected branch falls back before parsing any candidate.
		conditions := strings.Split(condition, " || ")
		branch := &portPlanBranch{priority: priority}
		for _, condition := range conditions {
			name, value, ok := strings.Cut(condition, " == ")
			port, err := strconv.ParseUint(value, 0, 16)
			if !ok || err != nil || name != "src" && name != "dst" {
				return nil
			}
			index := p.source
			if name == "dst" {
				index = p.destination
			}
			if _, exists := index[uint16(port)]; !exists {
				index[uint16(port)] = branch
			}
		}
		// The lexer keeps quoted strings whole, including braces inside them.
		depth, end := 1, -1
		for i, token := range parts {
			if token == "{" {
				depth++
			}
			if token == "}" {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
		if end < 0 {
			return nil
		}
		branch.candidates, ok = portCandidates(strings.Join(parts[:end], " "))
		branch.legacy = !ok
		parts = parts[end+1:]
		if len(parts) >= 2 && parts[0] == "else" && parts[1] == "if" {
			parts = parts[2:]
			continue
		}
		break
	}
	suffix := portDispatchSuffix
	if p.transport == "TCP" {
		suffix = strings.Replace(suffix, "debug(op.Message)\n", "", 1)
		suffix = strings.Replace(suffix, "// AFTER_RECOVERY", `debug("parse node %s failed: %v", typeName, op.Message)`, 1)
	}
	if strings.Join(parts, " ") != planTokens(suffix) {
		return nil
	}
	return &operatorPlan{kind: "port-dispatch", run: p.execute, ports: p}
}

func portCandidates(body string) ([]string, bool) {
	body = strings.TrimSuffix(body, " ;")
	if body == "typeNameList = [ ] string { }" {
		return []string{}, true
	}
	if !strings.HasPrefix(body, "typeNameList = [ ") || !strings.HasSuffix(body, " ]") {
		return nil, false
	}
	body = strings.TrimSuffix(strings.TrimPrefix(body, "typeNameList = [ "), " ]")
	var candidates []string
	for _, quoted := range strings.Split(body, " , ") {
		name, err := strconv.Unquote(quoted)
		if err != nil {
			return nil, false
		}
		candidates = append(candidates, name)
	}
	return candidates, true
}

func (p *portDispatchPlan) choose(src, dst uint16) *portPlanBranch {
	a, b := p.source[src], p.destination[dst]
	if a == nil {
		a = b
	} else if b != nil && b.priority < a.priority {
		a = b
	}
	if a == nil {
		a = p.otherwise
	}
	return a
}

func (p *portDispatchPlan) execute(e *planExecution) bool {
	// Admission checks perform no parser operations, writes or user callbacks.
	// Nonstandard/mutable field formatters keep their original VM semantics.
	srcNode := getNodeByPath(e.this.origin, "@"+p.transport+"/Source Port")
	dstNode := getNodeByPath(e.this.origin, "@"+p.transport+"/Destination Port")
	if srcNode == nil || dstNode == nil || srcNode.Cfg.Has("out") || dstNode.Cfg.Has("out") || !NodeHasResult(srcNode) || !NodeHasResult(dstNode) {
		return false
	}
	src, srcOK := GetNodeResult(srcNode).(uint16)
	dst, dstOK := GetNodeResult(dstNode).(uint16)
	if !srcOK || !dstOK {
		return false
	}
	branch := p.choose(src, dst)
	if branch.legacy {
		return false
	}
	e.at("getCurrentPosition()")
	buffer := e.this.origin.Ctx.GetItem("buffer").(*bytes.Buffer)
	start := buffer.Len()
	e.at("this.GetMaxLength()")
	maximum := e.this.GetMaxLength()
	bounded := maximum > 0 && maximum < 1048576
	// Keep Node.Result calls and their observable validation even though the
	// default numeric fields were already read to select a branch above.
	for _, field := range []string{"Source Port", "Destination Port"} {
		path := "@" + p.transport + "/" + field
		e.at("getNodeResult(" + strconv.Quote(path) + ")")
		if _, err := getNodeByPath(e.this.origin, path).Result(); err != nil {
			panic(err)
		}
	}
	for _, name := range branch.candidates {
		e.at("this.TryProcessByType(typeName)")
		op := e.this.tryProcessByTypeDiscard(name)
		if op.ok {
			e.at("op.Save()")
			op.save()
			p.tail(e, buffer, start, maximum, bounded, 0)
			return true
		}
		if p.transport == "UDP" {
			log.Debugf(op.message)
		}
		e.at("op.Recovery()")
		op.recovery()
		if p.transport == "TCP" {
			log.Debugf("parse node %s failed: %v", name, op.message)
		}
	}
	p.tail(e, buffer, start, maximum, bounded, 1)
	return true
}

func (p *portDispatchPlan) tail(e *planExecution, buffer *bytes.Buffer, start int, maximum uint64, bounded bool, occurrence int) {
	if !bounded {
		return
	}
	consumed := buffer.Len() - start
	if uint64(consumed) >= maximum {
		return
	}
	e.atNth(`this.NewUnknownNode("Remaining Payload")`, occurrence)
	tail := e.this.NewUnknownNode("Remaining Payload")
	e.atNth("tail.SetMaxLength(remaining)", occurrence)
	tail.SetMaxLength(maximum - uint64(consumed))
	e.atNth("tail.Process()", occurrence)
	tail.processDiscard()
}

const portDispatchPrefix = `payloadStart = getCurrentPosition()
maximum = this.GetMaxLength()
bounded = maximum > 0 && maximum < 1048576
src = getNodeResult("@$TRANSPORT/Source Port").Value
dst = getNodeResult("@$TRANSPORT/Destination Port").Value`

const portDispatchSuffix = `for typeName in typeNameList{
 res,op = this.TryProcessByType(typeName)
 if op.OK {
  err = op.Save()
  if err != nil{ panic(err) }
  if bounded {
   consumed = getCurrentPosition() - payloadStart
   if consumed < maximum {
    remaining = maximum - consumed
    tail = this.NewUnknownNode("Remaining Payload")
    tail.SetMaxLength(remaining)
    tail.Process()
   }
  }
  return
 }else{
  debug(op.Message)
  err = op.Recovery()
  if err != nil{ panic(err) }
  // AFTER_RECOVERY
 }
}
if bounded {
 consumed = getCurrentPosition() - payloadStart
 if consumed < maximum {
  remaining = maximum - consumed
  tail = this.NewUnknownNode("Remaining Payload")
  tail.SetMaxLength(remaining)
  tail.Process()
 }
}`
