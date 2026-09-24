package stream_parser

import (
	"bytes"
	"fmt"
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
	tlsGuard            bool
	opcuaAdmission      bool
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
	// TCP's OPC UA admission probe is deliberately evaluated before the
	// ordinary port table: a complete UACP header may identify the protocol on
	// a custom port, while port 4840 adds a strict client/server direction
	// constraint. Keep this exact source shape in the native plan instead of
	// falling back to the Yak VM for every TCP payload.
	var defaultBody string
	if strings.HasPrefix(rest, planTokens(opcuaPortDispatchPrelude)+" ") {
		rest = strings.TrimPrefix(rest, planTokens(opcuaPortDispatchPrelude)+" ")
		if !strings.HasPrefix(rest, planTokens(opcuaPortDispatchSelection)+" ") {
			return nil
		}
		rest = strings.TrimPrefix(rest, planTokens(opcuaPortDispatchSelection)+" ")
		rest = strings.TrimPrefix(rest, "if ")
		p.opcuaAdmission = true
		defaultBody = planTokens(`typeNameList = ["TLS", "HTTP", "SSH"]`)
	} else {
		var ok bool
		defaultBody, rest, ok = strings.Cut(rest, " if ")
		if !ok {
			return nil
		}
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
	actualSuffix := strings.Join(parts, " ")
	if actualSuffix != planTokens(suffix) {
		guarded := strings.Replace(suffix, "for typeName in typeNameList{", "for typeName in typeNameList{\n"+portDispatchTLSGuard, 1)
		if p.transport != "TCP" || actualSuffix != planTokens(guarded) {
			return nil
		}
		p.tlsGuard = true
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
	// An opaque selected branch is an intentional VM fallback. Let the VM run
	// the complete operator once, including the optional OPC UA probe; probing
	// here and then falling back would repeat parser callbacks and transactions.
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
	if p.opcuaAdmission {
		candidate, standardPort := p.opcuaCandidate(e, src, dst, maximum)
		if candidate {
			branch = &portPlanBranch{candidates: []string{"OPCUATCPMessage"}}
		} else if standardPort {
			branch = &portPlanBranch{candidates: []string{}}
		}
	}
	for _, name := range branch.candidates {
		if name == "TLS" && p.tlsGuard {
			e.at("this.TryProcessByType(\"XMPPTLSRecordHeader\")")
			probe := e.this.tryProcessByTypeDiscard("XMPPTLSRecordHeader")
			valid := false
			if probe.ok {
				value, ok := GetNodeResult(probe.parent.Children[len(probe.parent.Children)-1]).(uint64)
				if ok {
					content, version, length := value>>32, (value>>16)&65535, value&65535
					valid = content >= 20 && content <= 24 && version >= 0x0300 && version <= 0x0304 && length > 0 && length <= 18432 && (!bounded || length+5 <= maximum)
				}
			}
			e.at("probe.Recovery()")
			probe.recovery()
			if !valid {
				continue
			}
		}
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

// opcuaCandidate preserves the TCP operator's bounded OPCUAHeaderProbe and
// its Recovery transaction. The admission criteria intentionally mirror the
// rule's little-endian UACP header checks; the full OPCUATCPMessage parser
// still validates every body field before a frame is accepted.
func (p *portDispatchPlan) opcuaCandidate(e *planExecution, src, dst uint16, maximum uint64) (candidate, standardPort bool) {
	e.at(`this.TryProcessByType("OPCUAHeaderProbe")`)
	probe := e.this.tryProcessByTypeDiscard("OPCUAHeaderProbe")
	if probe.ok {
		e.at(`prefix.Child("OPCUA Header").Value`)
		prefix, err := probe.parent.Children[len(probe.parent.Children)-1].Result()
		if err != nil {
			panic(err)
		}
		headerNode := prefix.Child("OPCUA Header")
		if headerNode == nil {
			panic("OPCUAHeaderProbe did not return OPCUA Header")
		}
		header, ok := headerNode.Value.(uint64)
		if !ok {
			panic(fmt.Sprintf("OPCUA Header has unexpected type %T", headerNode.Value))
		}
		messageType := header & 0xffffff
		chunkType := (header >> 24) & 0xff
		messageSize := header >> 32
		var minimumSize uint64
		directionOK := true
		switch messageType {
		case 0x4c4548: // HEL
			minimumSize = 32
			if src == 4840 || dst == 4840 {
				directionOK = dst == 4840 && src != 4840
			}
		case 0x4b4341: // ACK
			minimumSize = 28
			if src == 4840 || dst == 4840 {
				directionOK = src == 4840 && dst != 4840
			}
		case 0x525245: // ERR
			minimumSize = 16
			if src == 4840 || dst == 4840 {
				directionOK = src == 4840 && dst != 4840
			}
		case 0x454852: // RHE
			minimumSize = 16
			if src == 4840 || dst == 4840 {
				directionOK = src == 4840 && dst != 4840
			}
		case 0x4e504f: // OPN
			minimumSize = 33
			directionOK = src != dst
		}
		candidate = chunkType == 0x46 && minimumSize != 0 && messageSize >= minimumSize && messageSize <= maximum && messageSize <= 16777216 && directionOK
	}
	e.at("probe.Recovery()")
	probe.recovery()
	return candidate, src == 4840 || dst == 4840
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

// Keep this closed form in sync with the TCP rule. The native dispatch plan
// must preserve TLS admission instead of falling back to the VM on all ports.
const portDispatchTLSGuard = `if typeName == "TLS" {
 prefix,probe = this.TryProcessByType("XMPPTLSRecordHeader")
 tlsRecord = false
 if probe.OK {
  content = prefix.Value >> 32
  version = (prefix.Value >> 16) & 65535
  length = prefix.Value & 65535
  tlsRecord = content >= 20 && content <= 24 && version >= 0x0300 && version <= 0x0304 && length > 0 && length <= 18432 && (!bounded || length + 5 <= maximum)
 }
 err = probe.Recovery()
 if err != nil { panic(err) }
 if !tlsRecord { continue }
}`

// This exact prelude is present only in the TCP transport operator. Keeping
// the complete normalized token sequence here makes any future rule change
// drop back to the VM until its semantics are deliberately added to the plan.
const opcuaPortDispatchPrelude = `typeNameList = ["TLS", "HTTP", "SSH"]
prefix,probe = this.TryProcessByType("OPCUAHeaderProbe")
opcuaCandidate = false
if probe.OK {
  header = prefix.Child("OPCUA Header").Value
  messageType = header & 16777215
  chunkType = (header >> 24) & 255
  messageSize = header >> 32
  minimumSize = 0
  directionOK = true
  if messageType == 0x4c4548 {
    minimumSize = 32
    if src == 4840 || dst == 4840 { directionOK = dst == 4840 && src != 4840 }
  } else if messageType == 0x4b4341 {
    minimumSize = 28
    if src == 4840 || dst == 4840 { directionOK = src == 4840 && dst != 4840 }
  } else if messageType == 0x525245 {
    minimumSize = 16
    if src == 4840 || dst == 4840 { directionOK = src == 4840 && dst != 4840 }
  } else if messageType == 0x454852 {
    minimumSize = 16
    if src == 4840 || dst == 4840 { directionOK = src == 4840 && dst != 4840 }
  } else if messageType == 0x4e504f {
    minimumSize = 33
    directionOK = src != dst
  }
  opcuaCandidate = chunkType == 0x46 && minimumSize != 0 && messageSize >= minimumSize && messageSize <= maximum && messageSize <= 16777216 && directionOK
}
err = probe.Recovery()
if err != nil { panic(err) }`

const opcuaPortDispatchSelection = `if opcuaCandidate {
  typeNameList = ["OPCUATCPMessage"]
} else if src == 4840 || dst == 4840 {
  typeNameList = []string{}
} else`
