package sfvm

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type SFFrame[V any] struct {
	symbolTable *omap.OrderedMap[string, *omap.OrderedMap[string, V]]
	stack       *utils.Stack[*Value[V]]
	Text        string
	Codes       []*SFI[V]
	toLeft      bool
	debug       bool
}

func NewSFFrame[V any](vars *omap.OrderedMap[string, *omap.OrderedMap[string, V]], text string, codes []*SFI[V]) *SFFrame[V] {
	v := vars
	if v == nil {
		v = omap.NewEmptyOrderedMap[string, *omap.OrderedMap[string, V]]()
	}
	return &SFFrame[V]{
		symbolTable: v,
		stack:       utils.NewStack[*Value[V]](),
		Text:        text,
		Codes:       codes,
	}
}

func (s *SFFrame[V]) Debug(v ...bool) *SFFrame[V] {
	if len(v) > 0 {
		s.debug = v[0]
	}
	return s
}

func (s *SFFrame[V]) GetSymbolTable() *omap.OrderedMap[string, *omap.OrderedMap[string, V]] {
	return s.symbolTable
}

func (s *SFFrame[V]) ToLeft() bool {
	return s.toLeft
}

func (s *SFFrame[V]) ToRight() bool {
	return !s.ToLeft()
}

func (s *SFFrame[V]) exec(input *omap.OrderedMap[string, V]) (ret error) {
	defer func() {
		if err := recover(); err != nil {
			ret = utils.Errorf("sft panic: %v", err)
		}
	}()
	for _, i := range s.Codes {
		s.debugLog(i.String())
		switch i.OpCode {
		case OpPushNumber:
			s.stack.Push(NewValue[V](i.UnaryInt))
		case OpPushString:
			s.stack.Push(NewValue[V](i.UnaryStr))
		case OpPushBool:
			s.stack.Push(NewValue[V](i.UnaryInt))
		case OpPushMatch:
			s.debugSubLog("search: %v", i.UnaryStr)
			res, err := input.SearchGlobKey(i.UnaryStr)
			if err != nil {
				return utils.Wrapf(err, "search glob key failed")
			}
			if res.Len() == 0 {
				s.debugSubLog("result: %v, not found", i.UnaryStr)
				return nil
			}
			res = res.ValuesMap()
			s.debugSubLog("result: (len: %v)", res.Len())
			s.stack.Push(NewValue[V](res))
			s.debugSubLog("<< push")
		case OpPushIndex:
			panic("PushIndex not implemented")
		case OpPushRef:
			result, ok := s.symbolTable.Get(i.UnaryStr)
			if !ok {
				result = omap.NewEmptyOrderedMap[string, V]()
			}
			s.stack.Push(NewValue[V](result))
		case OpNewRef:
			val := s.stack.Peek()
			s.symbolTable.Set(i.UnaryStr, val.AsMap())
		case OpUpdateRef:
			val := s.stack.Pop()
			s.symbolTable.Set(i.UnaryStr, val.AsMap())
		case OpFetchField:
			results := s.stack.Pop().AsMap().Map(func(string, V) (string, V, error) {
				panic("FetchField not implemented")
			})
			s.stack.Push(NewValue[V](results))
		case OpFetchIndex:
			results := s.stack.Pop().AsMap().Map(func(string, V) (string, V, error) {
				panic("FetchIndex not implemented")
			})
			s.stack.Push(NewValue[V](results))
		case OpSetDirection:
			s.toLeft = i.UnaryStr == "<<"
		case OpFlat:
			s.debugSubLog(">> pop %v then merge", i.UnaryInt)
			result := s.stack.PopN(i.UnaryInt)
			var mergedMap []*omap.OrderedMap[string, V]
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
			s.stack.Push(NewValue[V](merged))
		case OpMap:
			panic("Map is not implemented")
		case OpTypeCast:
			panic("TypeCast is not implemented")
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
			var a = NewValue[V](result)
			s.stack.Push(a)
			s.debugSubLog("<< push map(len: %v)", result.Len())
		case OpNotEq, OpGt, OpGtEq, OpLt, OpLtEq, OpLogicAnd, OpLogicOr:
			vals := s.stack.PopN(2)
			op1 := vals[0]
			op2 := vals[1]
			_ = op1
			_ = op2
		case OpNot:
			s.stack.Push(NewValue[V](!s.stack.Pop().AsBool()))
		case OpReMatch, OpGlobMatch:
			op1 := s.stack.Pop()
			op2 := i.UnaryStr
			_ = op1
			_ = op2
		default:
			panic(fmt.Sprintf("unhandled default case， undefined opcode: %v", spew.Sdump(i)))
		}
	}
	return nil
}

func (s *SFFrame[V]) debugLog(i string, item ...any) {
	if !s.debug {
		return
	}
	if len(item) > 0 {
		fmt.Printf("sf | "+i+"\n", item...)
	} else {
		fmt.Printf("sf | " + i + "\n")
	}
}

func (s *SFFrame[V]) debugSubLog(i string, item ...any) {
	s.debugLog("  |-- "+i, item...)
}
