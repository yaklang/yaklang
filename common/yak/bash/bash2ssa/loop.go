package bash2ssa

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"mvdan.cc/sh/v3/syntax"
	"strings"
)

// Keep loop stdout and the last body status on the backedge, not the failing
// condition at the exit. A loop that executes no body succeeds.
func (ab *astBuilder) loopBody(lb *ssa.LoopBuilder, body func() ssa.Value) ssa.Value {
	output := ab.slot("loop.stdout", ab.EmitConstInst(""))
	status := ab.slot("loop.status", ab.EmitConstInst(0))
	lb.SetBody(func() {
		ab.loopDepth++
		out := body()
		ab.loopDepth--
		if ab.IsBlockFinish() {
			return
		}
		if out != nil {
			ab.put(output, ab.concat(ab.ReadValue(output), out))
		}
		ab.put(status, ab.ReadValue(statusName))
	})
	lb.Finish()
	ab.foldTrivialPhis()
	ab.put(statusName, ab.ReadValue(status))
	return ab.ReadValue(output)
}

func (ab *astBuilder) buildWhileClause(c *syntax.WhileClause) ssa.Value {
	ab.declareMergeTargets(c)
	lb := ab.CreateLoopBuilder()
	lb.SetCondition(func() ssa.Value {
		ab.buildStmts(c.Cond)
		v := ab.success()
		if c.Until {
			return ab.EmitUnOp(ssa.OpNot, v)
		}
		return v
	})
	return ab.loopBody(lb, func() ssa.Value { return ab.buildStmts(c.Do) })
}

func (ab *astBuilder) buildForClause(c *syntax.ForClause) ssa.Value {
	ab.declareMergeTargets(c)
	switch it := c.Loop.(type) {
	case *syntax.CStyleLoop:
		lb := ab.CreateLoopBuilder()
		lb.SetFirst(func() []ssa.Value { ab.buildArithm(it.Init); return nil })
		lb.SetCondition(func() ssa.Value {
			if v := ab.buildArithm(it.Cond); v != nil {
				return ab.EmitBinOp(ssa.OpNotEq, v, ab.EmitConstInst(0))
			}
			return ab.EmitConstInst(true)
		})
		lb.SetThird(func() []ssa.Value { ab.buildArithm(it.Post); return nil })
		return ab.loopBody(lb, func() ssa.Value { return ab.buildStmts(c.Do) })
	case *syntax.WordIter:
		fields, spread := ab.words(it.Items)
		if !it.InPos.IsValid() {
			fields, spread = ab.spread(ab.positional("@"))
		}
		if len(fields) == 0 && !c.Select {
			ab.put(statusName, ab.EmitConstInst(0))
			return nil
		}
		vec := ab.packFields(fields, spread)
		name := it.Name.Value
		lb := ab.CreateLoopBuilder()
		var item, index ssa.Value
		if c.Select {
			ab.readOrCreate("REPLY")
			lb.SetCondition(func() ssa.Value {
				line := ab.intrinsic("select.read")
				ab.put("REPLY", line)
				item = ab.intrinsic("select.item", vec, line)
				return ab.intrinsic("select.has-input", line)
			})
		} else if spread {
			lb.SetCondition(func() ssa.Value { _, v, ok := ab.EmitNext(vec, false); item = v; return ok })
		} else {
			cursor := ab.slot("index", ab.EmitConstInst(0))
			lb.SetCondition(func() ssa.Value {
				index = ab.ReadValue(cursor)
				return ab.EmitBinOp(ssa.OpLt, index, ab.EmitConstInst(len(fields)))
			})
			lb.SetThird(func() []ssa.Value { ab.put(cursor, ab.EmitBinOp(ssa.OpAdd, index, ab.EmitConstInst(1))); return nil })
		}
		// Specialize literal arguments inside a real loop so concrete-argument
		// rules still see each command. The latch, not the last compiled body,
		// determines the value carried to the next iteration.
		specialize := !c.Select && !spread && len(fields) > 1 && len(fields) <= maxLiteralLoopUnroll && ab.loopUnrollDepth < maxLoopUnrollDepth && ab.loopUnrollBudget >= len(fields)
		for _, w := range it.Items {
			if _, ok := staticWord(w); !ok {
				specialize = false
			}
		}
		if specialize {
			ab.loopUnrollDepth++
			ab.loopUnrollBudget -= len(fields)
			defer func() { ab.loopUnrollDepth-- }()
		}
		straight := true
		bodyWrites := false
		for _, s := range c.Do {
			if _, ok := s.Cmd.(*syntax.CallExpr); !ok {
				straight = false
			}
			for _, n := range assignedNames(s) {
				if n == name {
					bodyWrites = true
				}
				straight = false
			}
			syntax.Walk(s, func(n syntax.Node) bool {
				if call, ok := n.(*syntax.CallExpr); ok && len(call.Args) > 0 {
					cmd := call.Args[0].Lit()
					if cmd == "break" || cmd == "continue" || cmd == "return" || cmd == "eval" {
						straight = false
						bodyWrites = true
					}
				}
				return true
			})
		}
		out := ab.loopBody(lb, func() ssa.Value {
			if specialize {
				result := ab.slot("item.stdout", ab.EmitConstInst(""))
				b := ab.CreateIfBuilder()
				for i, v := range fields {
					i, v := i, v
					arm := func() {
						ab.put(name, v)
						if straight {
							if o := ab.buildStmts(c.Do); o != nil && !ab.IsBlockFinish() {
								ab.put(result, o)
							}
						}
					}
					if i+1 == len(fields) {
						b.SetElse(arm)
					} else {
						b.AppendItem(func() ssa.Value { return ab.EmitBinOp(ssa.OpEq, index, ab.EmitConstInst(i)) }, arm)
					}
				}
				b.Build()
				if straight {
					return ab.ReadValue(result)
				}
			} else {
				if item == nil {
					if len(fields) == 1 {
						item = fields[0]
					} else {
						item = ab.ReadMemberCallValue(vec, index)
					}
				}
				ab.put(name, item)
			}
			return ab.buildStmts(c.Do)
		})
		if !spread && !c.Select && !bodyWrites && len(fields) > 0 {
			ab.put(name, fields[len(fields)-1])
		}
		return out
	}
	return nil
}

// read consumes option operands; only shell identifiers (or an array target)
// are writes. Without an explicit name Bash uses REPLY.
func readTargetNames(args []*syntax.Word) []string {
	if len(args) == 0 || args[0].Lit() != "read" {
		return nil
	}
	var out []string
	options := true
	for i := 1; i < len(args); i++ {
		s, ok := staticWord(args[i])
		if !ok {
			return out
		}
		if options && s == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(s, "-") && len(s) > 1 {
			for j := 1; j < len(s); j++ {
				if strings.ContainsRune("adinNptu", rune(s[j])) {
					val := s[j+1:]
					if val == "" && i+1 < len(args) {
						i++
						val, _ = staticWord(args[i])
					}
					if s[j] == 'a' && syntax.ValidName(val) {
						out = append(out, val)
					}
					break
				}
			}
			continue
		}
		options = false
		if syntax.ValidName(s) {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = append(out, "REPLY")
	}
	return out
}
