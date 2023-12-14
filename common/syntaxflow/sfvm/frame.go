package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type SFFrame[T comparable, V any] struct {
	symbolTable *omap.OrderedMap[string, *omap.OrderedMap[T, V]]
	stack       *utils.Stack[*Value[T, V]]
	Text        string
	Codes       []*SFI[T, V]
	toLeft      bool
}

func NewSFFrame[T comparable, V any](text string, codes []*SFI[T, V]) *SFFrame[T, V] {
	return &SFFrame[T, V]{
		symbolTable: omap.NewEmptyOrderedMap[string, *omap.OrderedMap[T, V]](),
		stack:       utils.NewStack[*Value[T, V]](),
		Text:        text,
		Codes:       codes,
	}
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

func (s *SFFrame[T, V]) Execute(input *omap.OrderedMap[T, V]) {
	for _, i := range s.Codes {
		switch i.OpCode {
		case OpPushNumber:
			s.stack.Push(NewValue[T, V](i.UnaryInt))
		case OpPushString:
			s.stack.Push(NewValue[T, V](i.UnaryStr))
		case OpPushBool:
			s.stack.Push(NewValue[T, V](i.UnaryInt))
		case OpPushMatch:
			panic("PushRef not implemented")
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
			s.symbolTable.Set(i.UnaryStr, val.Filter())
		case OpUpdateRef:
			val := s.stack.Pop()
			s.symbolTable.Set(i.UnaryStr, val.Filter())
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
		case OpEq, OpNotEq, OpGt, OpGtEq, OpLt, OpLtEq, OpLogicAnd, OpLogicOr:
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
		}
	}
}
