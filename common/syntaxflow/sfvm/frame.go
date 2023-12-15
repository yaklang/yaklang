package sfvm

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type SFFrame[T comparable, V any] struct {
	symbolTable *omap.OrderedMap[string, *omap.OrderedMap[T, V]]
	stack       *utils.Stack[*Value[T, V]]
	Text        string
	Codes       []*SFI[T, V]
	toLeft      bool
	debug       bool
}

func NewSFFrame[T comparable, V any](vars *omap.OrderedMap[string, *omap.OrderedMap[T, V]], text string, codes []*SFI[T, V]) *SFFrame[T, V] {
	v := vars
	if v == nil {
		v = omap.NewEmptyOrderedMap[string, *omap.OrderedMap[T, V]]()
	}
	return &SFFrame[T, V]{
		symbolTable: v,
		stack:       utils.NewStack[*Value[T, V]](),
		Text:        text,
		Codes:       codes,
	}
}

func (s *SFFrame[T, V]) Debug(v ...bool) *SFFrame[T, V] {
	if len(v) > 0 {
		s.debug = v[0]
	}
	return s
}

func (s *SFFrame[T, V]) GetSymbolTable() *omap.OrderedMap[string, *omap.OrderedMap[T, V]] {
	return s.symbolTable
}

func (s *SFFrame[T, V]) ToLeft() bool {
	return s.toLeft
}

func (s *SFFrame[T, V]) ToRight() bool {
	return !s.ToLeft()
}

func (s *SFFrame[T, V]) Execute(input *omap.OrderedMap[T, V]) error {
	for _, i := range s.Codes {
		s.debugLog(i.String())
		switch i.OpCode {
		case OpPushNumber:
			s.stack.Push(NewValue[T, V](i.UnaryInt))
		case OpPushString:
			s.stack.Push(NewValue[T, V](i.UnaryStr))
		case OpPushBool:
			s.stack.Push(NewValue[T, V](i.UnaryInt))
		case OpPushMatch:
			s.debugLog(" |-- search: %v", i.UnaryStr)
			res, err := input.SearchGlobKey(i.UnaryStr)
			if err != nil {
				return utils.Wrapf(err, "search glob key failed")
			}
			if res.Len() == 0 {
				s.debugLog(" |-- result: %v, not found", i.UnaryStr)
				return nil
			}
			s.debugLog(" |-- result: (len: %v)", res.Len())
			s.stack.Push(NewValue[T, V](res))
			s.debugLog(" |<- push")
		case OpPushIndex:
			panic("PushIndex not implemented")
		case OpPushRef:
			result, ok := s.symbolTable.Get(i.UnaryStr)
			if !ok {
				result = omap.NewEmptyOrderedMap[T, V]()
			}
			s.stack.Push(NewValue[T, V](result))
		case OpNewRef:
			val := s.stack.Peek()
			s.symbolTable.Set(i.UnaryStr, val.AsMap())
		case OpUpdateRef:
			val := s.stack.Pop()
			s.symbolTable.Set(i.UnaryStr, val.AsMap())
		case OpFetchField:
			results := s.stack.Pop().AsMap().Map(func(T, V) (T, V, error) {
				panic("FetchField not implemented")
			})
			s.stack.Push(NewValue[T, V](results))
		case OpFetchIndex:
			results := s.stack.Pop().AsMap().Map(func(T, V) (T, V, error) {
				panic("FetchIndex not implemented")
			})
			s.stack.Push(NewValue[T, V](results))
		case OpSetDirection:
			s.toLeft = i.UnaryStr == "<<"
		case OpFlat:
			result := s.stack.PopN(i.UnaryInt)
			var mergedMap []*omap.OrderedMap[T, V]
			for _, v := range result {
				_ = v
				panic("Flat not implemented")
			}
			_ = mergedMap
			panic("Flat not implemented")
		case OpMap:
			panic("Map is not implemented")
		case OpTypeCast:
			panic("TypeCast is not implemented")
		case OpEq:
			op2 := s.stack.Pop()
			op1 := s.stack.Pop()
			if !op1.IsMap() {
				return utils.Errorf("opEq op1 is not filter/map")
			}
			result, err := op1.AsMap().SearchKey(op2.AsString())
			if err != nil {
				return utils.Errorf("opEq search key failed: %v", err)
			}
			result.Values()

		case OpNotEq, OpGt, OpGtEq, OpLt, OpLtEq, OpLogicAnd, OpLogicOr:
			vals := s.stack.PopN(2)
			op1 := vals[0]
			op2 := vals[1]
			_ = op1
			_ = op2
		case OpNot:
			s.stack.Push(NewValue[T, V](!s.stack.Pop().AsBool()))
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

func (s *SFFrame[T, V]) debugLog(i string, item ...any) {
	if !s.debug {
		return
	}
	if len(item) > 0 {
		fmt.Printf("sf | "+i+"\n", item...)
	} else {
		fmt.Printf("sf | " + i + "\n")
	}
}

func (s *SFFrame[T, V]) debugSubLog(i string, item ...any) {
	s.debugLog("  |-- "+i, item...)
}
