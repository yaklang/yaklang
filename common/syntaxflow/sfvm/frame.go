package sfvm

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type mapCtx struct {
	OriginDepth int
	Current     int
	Index       int
	Value       []*omap.OrderedMap[string, any]
}

type SFFrame struct {
	symbolTable *omap.OrderedMap[string, any]
	stack       *utils.Stack[*Value]
	mapStack    *utils.Stack[*mapCtx]
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
		stack:       utils.NewStack[*Value](),
		mapStack:    utils.NewStack[*mapCtx](),
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

func (s *SFFrame) exec(input *omap.OrderedMap[string, any]) (ret error) {
	s.stack.Push(NewValue(input))
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
		case OpPushNumber:
			s.stack.Push(NewValue(i.UnaryInt))
		case OpPushString:
			s.stack.Push(NewValue(i.UnaryStr))
		case OpPushBool:
			s.stack.Push(NewValue(i.UnaryInt))
		case OpPushMatch:
			s.debugSubLog("search: %v", i.UnaryStr)
			res, err := s.stack.Pop().AsMap().SearchGlobKey(i.UnaryStr)
			if err != nil {
				return utils.Wrapf(err, "search glob key failed")
			}
			if res.Len() == 0 {
				s.debugSubLog("result: %v, not found", i.UnaryStr)
				return nil
			}
			res = res.ValuesMap()
			s.debugSubLog("result: (len: %v)", res.Len())
			s.stack.Push(NewValue(res))
			s.debugSubLog("<< push")
		case OpPushIndex:
			s.debugSubLog("peek stack top index: [%v]", i.UnaryInt)
			parent := s.stack.Peek().AsMap()
			results, err := parent.SearchIndexKey(i.UnaryInt)
			if err != nil {
				return utils.Wrap(err, "search index key failed")
			}
			s.stack.Push(NewValue(results))
			s.debugSubLog("<< push")
		case OpPushRef:
			result, ok := s.symbolTable.Get(i.UnaryStr)
			if !ok {
				result = omap.NewEmptyOrderedMap[string, any]()
			}
			s.stack.Push(NewValue(result))
		case OpNewRef:
			s.debugSubLog("new$ref: %v", i.UnaryStr)
			s.symbolTable.Set(i.UnaryStr, omap.NewEmptyOrderedMap[string, any]())
		case OpUpdateRef:
			s.debugSubLog("fetch$ref: %v", i.UnaryStr)
			result, ok := s.symbolTable.Get(i.UnaryStr)
			if !ok {
				result = omap.NewEmptyOrderedMap[string, any]()
				s.debugSubLog("auto new$ref: %v", i.UnaryStr)
				s.symbolTable.Set(i.UnaryStr, result)
			}
			val := s.stack.Pop()
			if val.IsMap() {
				s.symbolTable.Set(i.UnaryStr, val.AsMap())
			} else {
				s.symbolTable.Set(i.UnaryStr, val.Value())
			}
			s.debugSubLog("update$ref: %v := %v", i.UnaryStr, val.VerboseString())
		case OpFetchField:
			results := s.stack.Pop().AsMap()
			s.debugSubLog(">> (pop)")
			r := results.Field(i.UnaryStr)
			s.debugSubLog(".%v (len: %v)", i.UnaryStr, r.Len())
			s.stack.Push(NewValue(r))
			s.debugSubLog("<< push")
		case OpFetchIndex:
			results := s.stack.Pop().AsMap()
			s.debugSubLog(">> pop")
			ret := results.Index(i.UnaryInt)
			s.debugSubLog("[%v] (len: %v)", i.UnaryInt, ret.Len())
			s.stack.Push(NewValue(ret))
			s.debugSubLog("<< push")
		case OpSetDirection:
			s.toLeft = i.UnaryStr == "<<"
		case OpFlat:
			s.debugSubLog(">> pop %v then merge", i.UnaryInt)
			result := s.stack.PopN(i.UnaryInt)
			var mergedMap []*omap.OrderedMap[string, any]
			for index, v := range result {
				if v == nil {
					s.debugSubLog("%2d: empty value", index)
					continue
				}
				s.debugSubLog("%2d: merge-map %v", index, v.AsMap().Len())
				mergedMap = append(mergedMap, v.AsMap())
			}
			merged := omap.Merge(mergedMap...).ValuesMap()
			s.debugSubLog("<< push map(len: %v)", merged.Len())
			s.stack.Push(NewValue(merged))
		case OpMapStart:
			v := s.stack.Peek()
			if !v.IsMap() {
				return utils.Errorf("map start failed: stack top is not map/dict/array")
			}
			m := v.AsMap()
			var l int
			if m.CanAsList() {
				panic("NOT IMPL ARRAY LIST")
			} else {
				m.UnsetParent()
				l = 1
			}
			s.mapStack.Push(&mapCtx{
				Current: l, Index: idx, OriginDepth: l,
			})
			s.debugSubLog("check top stack is omap/array: len(%v)", m.Len())
		case OpMapDone:
			val := s.mapStack.Peek()
			val.Current--

			resultNow := omap.NewGeneralOrderedMap()
			for _, k := range i.Values {
				v, ok := s.symbolTable.Get(k)
				if ok {
					switch ret := v.(type) {
					case *omap.OrderedMap[string, any]:
						if ret.HaveLiteralValue() {
							resultNow.Set(k, ret.LiteralValue())
						} else {
							resultNow.Set(k, v)
						}
					default:
						resultNow.Set(k, v)
					}
				} else {
					var i any = omap.NewEmptyOrderedMap[string, any]()
					resultNow.Set(k, i)
				}
			}
			val.Value = append(val.Value, resultNow)
			if val.Current <= 0 {
				s.mapStack.Pop()
				nxt := omap.NewEmptyOrderedMap[string, any]()
				for _, result := range val.Value {
					var v any = result
					nxt.Add(v)
				}
				s.debugSubLog("<< push (len: %v)", nxt.Len())
				s.stack.Push(NewValue(nxt))
			}
		case OpTypeCast:
			s.debugSubLog(">> pop -> (%v)", i.UnaryStr)
			op1 := s.stack.Pop()
			switch i.UnaryStr {
			case "string", "str", "s":
				s.stack.Push(NewValue(op1.AsString()))
			case "int", "number", "float", "i":
				s.stack.Push(NewValue(op1.AsInt()))
			case "bool", "boolean", "b":
				s.stack.Push(NewValue(op1.AsBool()))
			case "dict":
				s.stack.Push(NewValue(op1.AsMap()))
			default:
				log.Warnf("unknown type cast: %v", i.UnaryStr)
				s.stack.Push(op1)
			}
			s.debugSubLog("<< push")
		case OpEq:
			op2 := s.stack.Pop()
			op1 := s.stack.Pop()
			ret := op1.IsMap()
			s.debugSubLog(">> pop 2 values, op1 must be map: %v", ret)
			if !ret {
				return utils.Errorf("opEq op1 is not filter/map")
			}
			search := op2.AsString()
			s.debugSubLog("search op2: %v", op2.AsString())
			result, err := op1.AsMap().SearchKey(search)
			if err != nil {
				return utils.Errorf("opEq search key failed: %v", err)
			}
			var a = NewValue(result)
			s.stack.Push(a)
			s.debugSubLog("<< push map(len: %v)", result.Len())
		case OpNotEq, OpGt, OpGtEq, OpLt, OpLtEq, OpLogicAnd, OpLogicOr:
			vals := s.stack.PopN(2)
			op1 := vals[0]
			op2 := vals[1]
			_ = op1
			_ = op2
		case OpNot:
			s.stack.Push(NewValue(!s.stack.Pop().AsBool()))
		case OpReMatch, OpGlobMatch:
			op1 := s.stack.Pop()
			op2 := i.UnaryStr
			_ = op1
			_ = op2
		case OpRestoreContext:
			s.debugSubLog("restore context")
			if !s.stack.HaveLastStackValue() {
				return utils.Errorf("restore context failed: stack is empty")
			}
			v, ok := s.stack.LastStackValue().AsMap().GetRoot()
			if !ok {
				return utils.Errorf("restore context failed: %v", "cannot find root node")
			}
			s.stack.Push(NewValue(v))
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
