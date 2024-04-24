package sfvm

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"regexp"
)

type SFFrame struct {
	symbolTable *omap.OrderedMap[string, any]
	stack       *utils.Stack[ValueOperator]
	Text        string
	Codes       []*SFI
	toLeft      bool
	debug       bool
}

func NewSFFrame(vars *omap.OrderedMap[string, any], text string, codes []*SFI) *SFFrame {
	v := vars
	if v == nil {
		v = omap.NewEmptyOrderedMap[string, any]()
	}
	return &SFFrame{
		symbolTable: v,
		stack:       utils.NewStack[ValueOperator](),
		Text:        text,
		Codes:       codes,
	}
}

func (s *SFFrame) Debug(v ...bool) *SFFrame {
	if len(v) > 0 {
		s.debug = v[0]
	}
	return s
}

func (s *SFFrame) GetSymbolTable() *omap.OrderedMap[string, any] {
	return s.symbolTable
}

func (s *SFFrame) ToLeft() bool {
	return s.toLeft
}

func (s *SFFrame) ToRight() bool {
	return !s.ToLeft()
}

func (s *SFFrame) exec(input ValueOperator) (ret error) {
	s.stack.Push(input)
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Errorf("sft panic: %v", err)
		}
	}()

	idx := 0
	for {
		if idx >= len(s.Codes) {
			break
		}
		i := s.Codes[idx]

		s.debugLog(i.String())
		switch i.OpCode {
		case OpCheckStackTop:
			if s.stack.Len() == 0 {
				return utils.Errorf("stack top is empty")
			}
		case OpPushSearchExact:
			s.debugSubLog("<< pop search: %v", i.UnaryStr)
			value := s.stack.Pop()
			if value == nil {
				return utils.Errorf("search exact failed: stack top is empty")
			}
			result, next, err := value.ExactMatch(i.UnaryStr)
			if err != nil {
				return utils.Wrapf(err, "search exact failed")
			}
			if !result {
				s.debugSubLog("result: %v, not found", i.UnaryStr)
				return utils.Errorf("search exact failed: not found: %v", i.UnaryStr)
			}
			if next != nil {
				s.debugSubLog("result: %v", next.GetName())
				s.stack.Push(next)
				s.debugSubLog("<< push")
			} else {
				s.debugSubLog("result: %v", value.GetName())
				s.stack.Push(value)
				s.debugSubLog("<< push")
			}
		case OpPushSearchGlob:
			s.debugSubLog("<< pop search glob: %v", i.UnaryStr)
			value := s.stack.Pop()
			if value == nil {
				return utils.Errorf("search glob failed: stack top is empty")
			}
			globIns, err := glob.Compile(i.UnaryStr)
			if err != nil {
				return utils.Wrap(err, "compile glob failed")
			}
			result, next, err := value.GlobMatch(globIns)
			if err != nil {
				return utils.Wrap(err, "search glob failed")
			}
			if !result {
				s.debugSubLog("result: %v, not found", i.UnaryStr)
				return utils.Errorf("search glob failed: not found: %v", i.UnaryStr)
			}
			if next != nil {
				s.debugSubLog("result: %v", next.GetName())
				s.stack.Push(next)
				s.debugSubLog("<< push")
				return nil
			}
			s.debugSubLog("result: %v", value.GetName())
			s.stack.Push(value)
			s.debugSubLog("<< push")
			return nil
		case OpPushSearchRegexp:
			s.debugSubLog("<< pop search regexp: %v", i.UnaryStr)
			value := s.stack.Pop()
			if value == nil {
				return utils.Errorf("search regexp failed: stack top is empty")
			}
			regexpIns, err := regexp.Compile(i.UnaryStr)
			if err != nil {
				return utils.Wrap(err, "compile regexp failed")
			}
			result, next, err := value.RegexpMatch(regexpIns)
			if err != nil {
				return utils.Wrap(err, "search regexp failed")
			}
			if !result {
				s.debugSubLog("result: %v, not found", i.UnaryStr)
				return utils.Errorf("search regexp failed: not found: %v", i.UnaryStr)
			}
			if next != nil {
				s.debugSubLog("result: %v", next.GetName())
				s.stack.Push(next)
				s.debugSubLog("<< push")
				return nil
			}
			s.debugSubLog("result: %v", value.GetName())
			s.stack.Push(value)
			s.debugSubLog("<< push")
		//case OpPushIndex:
		//	s.debugSubLog("peek stack top index: [%v]", i.UnaryInt)
		//	parent := s.stack.Peek().AsMap()
		//	results, err := parent.SearchIndexKey(i.UnaryInt)
		//	if err != nil {
		//		return utils.Wrap(err, "search index key failed")
		//	}
		//	s.stack.Push(NewValue(results))
		//	s.debugSubLog("<< push")
		//case OpPushRef:
		//	result, ok := s.symbolTable.Get(i.UnaryStr)
		//	if !ok {
		//		result = omap.NewEmptyOrderedMap[string, any]()
		//	}
		//	s.stack.Push(NewValue(result))
		//case OpNewRef:
		//	s.debugSubLog("new$ref: %v", i.UnaryStr)
		//	s.symbolTable.Set(i.UnaryStr, omap.NewEmptyOrderedMap[string, any]())
		//case OpUpdateRef:
		//	s.debugSubLog("fetch$ref: %v", i.UnaryStr)
		//	_, ok := s.symbolTable.Get(i.UnaryStr)
		//	if !ok {
		//		return utils.Errorf("update$ref failed: ref: %v not found", i.UnaryStr)
		//	}
		//	val := s.stack.Pop()
		//	if val.IsMap() {
		//		if ret := val.AsMap(); ret.CanAsList() {
		//			s.symbolTable.Set(i.UnaryStr, ret.Values())
		//		} else {
		//			s.symbolTable.Set(i.UnaryStr, ret)
		//		}
		//	} else {
		//		s.symbolTable.Set(i.UnaryStr, val.Value())
		//	}
		//	s.debugSubLog("update$ref: %v := %v", i.UnaryStr, val.VerboseString())
		//case OpFetchField:
		//	results := s.stack.Pop().AsMap()
		//	s.debugSubLog(">> (pop)")
		//	r, ok := results.Get(i.UnaryStr)
		//	if !ok {
		//		s.debugSubLog(".%v empty", i.UnaryStr)
		//		s.stack.Push(NewValue(omap.NewEmptyOrderedMap[string, any]()))
		//	} else {
		//		s.debugSubLog(".%v := %v", i.UnaryStr, r)
		//		s.stack.Push(NewValue(r))
		//		s.debugSubLog("<< push")
		//	}
		//case OpFetchIndex:
		//	top := s.stack.Pop()
		//	s.debugSubLog(">> pop %v", top.VerboseString())
		//	var results *omap.OrderedMap[string, any]
		//	if !top.IsMap() {
		//		results = omap.BuildGeneralMap[any](top.Value())
		//	} else {
		//		results = top.AsMap()
		//	}
		//	ret, ok := results.GetByIndex(i.UnaryInt)
		//	if ok {
		//		s.debugSubLog("[%v]: (%T)", i.UnaryInt, ret)
		//		s.stack.Push(NewValue(ret))
		//		s.debugSubLog("<< push")
		//	} else {
		//		s.debugSubLog("[%v] any", i.UnaryInt)
		//		s.stack.Push(NewValue(omap.NewGeneralOrderedMap()))
		//		s.debugSubLog("<< push")
		//	}
		//case OpSetDirection:
		//	s.toLeft = i.UnaryStr == "<<"
		//case OpFlatStart:
		//	// flat will create empty array
		//	i := s.stack.Peek()
		//	if !i.IsMap() {
		//		return utils.Errorf("flat start failed: stack top is not map, (%v)", i.Value())
		//	}
		//	om := i.AsMap()
		//	if om.CanAsList() {
		//		om.UnsetParent()
		//		l := om.Len()
		//		s.flatStack.Push(&flatCtx{
		//			OriginDepth: l, Current: l,
		//			Index: idx,
		//			Root:  i,
		//			Value: omap.NewGeneralOrderedMap(),
		//		})
		//		s.stack.Push(NewValue(om.Index(0)))
		//	} else {
		//		om.UnsetParent()
		//		s.flatStack.Push(&flatCtx{
		//			OriginDepth: 1,
		//			Current:     1,
		//			Index:       idx,
		//			Value:       omap.NewGeneralOrderedMap(),
		//			Root:        NewValue(om),
		//		})
		//	}
		//case OpRestoreFlatContext:
		//	s.debugSubLog(">> restore flat ctx")
		//	ctx := s.flatStack.Peek()
		//	if ctx == nil {
		//		return utils.Errorf("restore flat ctx failed: stack is empty")
		//	}
		//	if !s.stack.HaveLastStackValue() {
		//		return utils.Errorf("restore flat ctx failed: stack is empty(last stack value empty)")
		//	}
		//	val := s.stack.LastStackValue()
		//	ctx.Value = ctx.Value.Merge(val.AsMap())
		//
		//	root := ctx.Root.AsMap()
		//	if ret := ctx.OriginDepth - ctx.Current + 1; ret >= ctx.OriginDepth {
		//		s.stack.Push(NewValue(omap.NewGeneralOrderedMap()))
		//	} else {
		//		s.stack.Push(NewValue(root.Index(ret)))
		//	}
		//case OpFlatDone:
		//	s.debugSubLog(">> flat done")
		//	ctx := s.flatStack.Peek()
		//	if ctx == nil {
		//		return utils.Errorf("flat done failed: stack is empty")
		//	}
		//	ctx.Current--
		//	if ctx.Current <= 0 {
		//		// finished
		//		s.flatStack.Pop()
		//		s.debugSubLog(">> pop origin value")
		//		s.stack.Push(NewValue(ctx.Value))
		//	} else {
		//		idx = ctx.Index
		//		s.debugSubLog("<< restore index: %v", idx)
		//	}
		//case OpMapStart:
		//	v := s.stack.Peek()
		//	if !v.IsMap() {
		//		return utils.Errorf("map start failed: stack top is not map/dict/array")
		//	}
		//	m := v.AsMap()
		//	if m.CanAsList() {
		//		m.UnsetParent()
		//		l := m.Len()
		//		s.mapStack.Push(&mapCtx{
		//			Current: l, Index: idx, OriginDepth: l,
		//			Root: v,
		//		})
		//		s.stack.Push(NewValue(m.Index(0)))
		//	} else {
		//		m.UnsetParent()
		//		buildMaterial := omap.NewGeneralOrderedMap()
		//		ret := buildMaterial.Merge(m)
		//		rootValue := NewValue(ret)
		//		s.mapStack.Push(&mapCtx{
		//			Current: 1, Index: idx, OriginDepth: 1,
		//			Root: rootValue,
		//		})
		//		s.stack.Push(rootValue)
		//	}
		//	s.debugSubLog("check top stack is omap/array: len(%v)", m.Len())
		//case OpRestoreMapContext:
		//	s.debugSubLog(">> restore map ctx")
		//	ctx := s.mapStack.Peek()
		//	if ctx == nil {
		//		return utils.Errorf("restore map ctx failed: stack is empty")
		//	}
		//	if !s.stack.HaveLastStackValue() {
		//		return utils.Errorf("restore map ctx failed: stack is empty(last stack value empty)")
		//	}
		//	root := ctx.Root.AsMap()
		//	if ret := ctx.OriginDepth - ctx.Current + 1; ret >= ctx.OriginDepth {
		//		s.stack.Push(NewValue(root))
		//	} else {
		//		s.stack.Push(NewValue(root.Index(ret)))
		//	}
		//case OpMapDone:
		//	val := s.mapStack.Peek()
		//	val.Current--
		//
		//	resultNow := omap.NewGeneralOrderedMap()
		//	for _, k := range i.Values {
		//		v, ok := s.symbolTable.Get(k)
		//		if ok {
		//			switch ret := v.(type) {
		//			case *omap.OrderedMap[string, any]:
		//				if ret.HaveLiteralValue() {
		//					resultNow.Set(k, ret.LiteralValue())
		//				} else {
		//					resultNow.Set(k, v)
		//				}
		//			default:
		//				resultNow.Set(k, v)
		//			}
		//		} else {
		//			var i any = omap.NewEmptyOrderedMap[string, any]()
		//			resultNow.Set(k, i)
		//		}
		//	}
		//	val.Value = append(val.Value, resultNow)
		//	if val.Current <= 0 {
		//		s.mapStack.Pop()
		//		nxt := omap.NewEmptyOrderedMap[string, any]()
		//		for _, result := range val.Value {
		//			var v any = result
		//			nxt.Add(v)
		//		}
		//		s.debugSubLog(">> pop origin value")
		//		s.stack.Pop()
		//		s.debugSubLog("<< push (len: %v)", nxt.Len())
		//		s.stack.Push(NewValue(nxt))
		//	} else {
		//		idx = val.Index
		//		s.debugSubLog("<< restore index: %v", idx)
		//	}
		//case OpTypeCast:
		//	s.debugSubLog(">> pop -> (%v)", i.UnaryStr)
		//	op1 := s.stack.Pop()
		//	switch i.UnaryStr {
		//	case "string", "str", "s":
		//		s.stack.Push(NewValue(op1.AsString()))
		//	case "int", "number", "float", "i":
		//		s.stack.Push(NewValue(op1.AsInt()))
		//	case "bool", "boolean", "b":
		//		s.stack.Push(NewValue(op1.AsBool()))
		//	case "dict":
		//		s.stack.Push(NewValue(op1.AsMap()))
		//	default:
		//		log.Warnf("unknown type cast: %v", i.UnaryStr)
		//		s.stack.Push(op1)
		//	}
		//	s.debugSubLog("<< push")
		//case OpEq:
		//	op2 := s.stack.Pop()
		//	op1 := s.stack.Pop()
		//	ret := op1.IsMap()
		//	s.debugSubLog(">> pop 2 values, op1 must be map: %v", ret)
		//	if !ret {
		//		return utils.Errorf("opEq op1 is not filter/map")
		//	}
		//	search := op2.AsString()
		//	s.debugSubLog("search op2: %v", op2.AsString())
		//	result, err := op1.AsMap().SearchKey(search)
		//	if err != nil {
		//		return utils.Errorf("opEq search key failed: %v", err)
		//	}
		//	var a = NewValue(result)
		//	s.stack.Push(a)
		//	s.debugSubLog("<< push map(len: %v)", result.Len())
		//case OpNotEq, OpGt, OpGtEq, OpLt, OpLtEq, OpLogicAnd, OpLogicOr, OpNot:
		//	op2 := s.stack.Pop()
		//	op1 := s.stack.Pop()
		//	result, err := op1.Exec(i.OpCode, op2)
		//	if err != nil {
		//		return utils.Wrap(err, "exec failed")
		//	}
		//	s.stack.Push(result)
		//case OpReMatch, OpGlobMatch:
		//	op1 := s.stack.Pop()
		//	op2 := i.UnaryStr
		//	_ = op1
		//	_ = op2
		//case OpPop:
		//	s.stack.Pop()
		//case OpWithdraw:
		//	s.stack.Push(s.stack.LastStackValue())
		default:
			panic(fmt.Sprintf("unhandled default case， undefined opcode: %v", spew.Sdump(i)))
		}

		idx++
	}

	return nil
}

func (s *SFFrame) debugLog(i string, item ...any) {
	if !s.debug {
		return
	}
	if len(item) > 0 {
		fmt.Printf("sf | "+i+"\n", item...)
	} else {
		fmt.Printf("sf | " + i + "\n")
	}
}

func (s *SFFrame) debugSubLog(i string, item ...any) {
	s.debugLog("  |-- "+i, item...)
}
