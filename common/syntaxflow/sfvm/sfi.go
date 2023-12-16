package sfvm

import (
	"fmt"
	"strconv"
)

type SFVMOpCode int

const (
	OpPass SFVMOpCode = iota

	// OpPushNumber and OpPushString and OpPushBool can push literal into stack
	OpPushNumber
	OpPushString
	OpPushBool

	// OpPushMatch can push data from origin
	OpPushMatch
	// OpPushIndex can push data from index
	OpPushIndex
	// OpPushRef can push reference into stack
	OpPushRef
	// OpNewRef can create new symbol reference
	OpNewRef
	OpUpdateRef

	//
	OpFetchField
	OpFetchIndex
	OpSetDirection
	OpFlat
	OpMapStart
	OpMapDone
	OpTypeCast

	/*
		Binary Operator
		Fetch TWO in STACK, calc result, push result into stack
	*/
	OpEq
	OpNotEq
	OpGt
	OpGtEq
	OpLt
	OpLtEq
	OpLogicAnd
	OpLogicOr

	/*
		Unary Operator: Fetch ONE in STACK, calc result, push result into stack
	*/
	OpReMatch
	OpGlobMatch
	OpNot
)

type SFI[V any] struct {
	OpCode   SFVMOpCode
	UnaryInt int
	UnaryStr string
	Desc     string
	Values   []string
}

const verboseLen = "%-12s"

func (s *SFI[V]) String() string {
	switch s.OpCode {
	case OpPass:
		return "- pass -"
	case OpPushBool:
		v := "false"
		if s.UnaryInt > 0 {
			v = "true"
		}
		return fmt.Sprintf(verboseLen+" %v", "push", v)
	case OpPushString:
		return fmt.Sprintf(verboseLen+" (len:%v) %v", "push", len(s.UnaryStr), strconv.Quote(s.UnaryStr))
	case OpPushNumber:
		return fmt.Sprintf(verboseLen+" %v", "push", s.UnaryInt)
	case OpPushRef:
		return fmt.Sprintf(verboseLen+" %v", "push$ref", s.UnaryStr)
	case OpPushMatch:
		return fmt.Sprintf(verboseLen+" like %v", "push$match", s.UnaryStr)
	case OpPushIndex:
		return fmt.Sprintf(verboseLen+" [%v]", "push$index", s.UnaryInt)
	case OpNewRef:
		return fmt.Sprintf(verboseLen+" %v", "new$ref", s.UnaryStr)
	case OpUpdateRef:
		return fmt.Sprintf(verboseLen+" %v", "update$ref", s.UnaryStr)
	case OpFetchField:
		return fmt.Sprintf(verboseLen+" .%v", "?field", s.UnaryStr)
	case OpFetchIndex:
		return fmt.Sprintf(verboseLen+" [%v]", "?=index", s.UnaryInt)
	case OpSetDirection:
		return fmt.Sprintf(verboseLen+" %v", "direction", s.UnaryStr)
	case OpFlat:
		return fmt.Sprintf(verboseLen+" (?len=%v) %v", "=>flat", s.UnaryInt, s.UnaryStr)
	case OpMapStart:
		return fmt.Sprintf(verboseLen+" %v", "=>start-map", s.UnaryStr)
	case OpMapDone:
		return fmt.Sprintf(verboseLen+" %v - %v", "=>build-map", s.UnaryInt, s.Values)
	case OpTypeCast:
		return fmt.Sprintf(verboseLen+" %v", "type-cast", s.UnaryStr)
	case OpEq:
		return fmt.Sprintf(verboseLen+" %v", "(operator) ==", s.UnaryStr)
	case OpNotEq:
		return fmt.Sprintf(verboseLen+" %v", "(operator) !=", s.UnaryStr)
	case OpGt:
		return fmt.Sprintf(verboseLen+" %v", "(operator) >", s.UnaryStr)
	case OpGtEq:
		return fmt.Sprintf(verboseLen+" %v", "(operator) >=", s.UnaryStr)
	case OpLt:
		return fmt.Sprintf(verboseLen+" %v", "(operator) <", s.UnaryStr)
	case OpLtEq:
		return fmt.Sprintf(verboseLen+" %v", "(operator) <=", s.UnaryStr)
	case OpReMatch:
		return fmt.Sprintf(verboseLen+" %v", "(operator) ~=", s.UnaryStr)
	case OpGlobMatch:
		return fmt.Sprintf(verboseLen+" %v", "(operator) *~", s.UnaryStr)
	case OpNot:
		return fmt.Sprintf(verboseLen+" %v", "(operator) !", s.UnaryStr)
	case OpLogicAnd:
		return fmt.Sprintf(verboseLen+" %v", "(operator) &&", s.UnaryStr)
	case OpLogicOr:
		return fmt.Sprintf(verboseLen+" %v", "(operator) ||", s.UnaryStr)
	}
	return ""
}
