package yakast

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (s *YakCompiler) _pushOpcodeWithCurrentCodeContext(codes ...*yakvm.Code) {
	for _, c := range codes {
		if s.currentStartPosition != nil && s.currentEndPosition != nil {
			c.StartLineNumber = s.currentStartPosition.LineNumber
			c.StartColumnNumber = s.currentStartPosition.ColumnNumber
			c.EndLineNumber = s.currentEndPosition.LineNumber
			c.EndColumnNumber = s.currentEndPosition.ColumnNumber
			c.SourceCodePointer = s.sourceCodePointer
			c.SourceCodeFilePath = s.sourceCodeFilePathPointer
		}
	}
	s.codes = append(s.codes, codes...)
}

func (s *YakCompiler) pushInteger(i int, origin string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "int",
			Value:       i,
			Literal:     origin,
		},
	})
}

func (s *YakCompiler) pushInt64(i int64, origin string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "int64",
			Value:       i,
			Literal:     origin,
		},
	})
}
func (s *YakCompiler) pushChar(i rune, origin string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "char",
			Value:       i,
			Literal:     origin,
		},
	})
}
func (s *YakCompiler) pushByte(i byte, origin string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "byte", // uint8
			Value:       i,
			Literal:     origin,
		},
	})
}

func (s *YakCompiler) pushBytes(i []byte, lit string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "bytes", // []uint8
			Value:       i,
			Literal:     lit,
		},
	})
}

func (s *YakCompiler) pushFloat(i float64, origin string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "float64",
			Value:       i,
			Literal:     origin,
		},
	})
}

func (s *YakCompiler) pushBool(i bool) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "bool",
			Value:       i,
			Literal:     fmt.Sprint(i),
		},
	})
}

func (s *YakCompiler) pushType(i string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpType,
		Op1: &yakvm.Value{
			TypeVerbose: i,
		},
	})
}

func (s *YakCompiler) pushMake(i int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpMake,
		Unary:  i,
	})
}

func (s *YakCompiler) pushOperator(i yakvm.OpcodeFlag) *yakvm.Code {
	code := &yakvm.Code{
		Opcode: i,
	}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushEnterFR() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpEnterFR,
	}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushBreak() {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpBreak,
		Op1:    yakvm.NewIntValue(s.getNearliestBreakScopeCounter()),
	})
}

func (s *YakCompiler) pushEllipsis(n int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpEllipsis,
		Unary:  n,
	})
}

func (s *YakCompiler) pushScope(i int) *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpScope, Unary: i}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushInNext(i int) *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpInNext, Unary: i}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushRangeNext(i int) *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpRangeNext, Unary: i}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushExitFR(i int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{Opcode: yakvm.OpExitFR, Unary: i})
}

func (s *YakCompiler) pushDefer(codes []*yakvm.Code) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpDefer,
		Unary:  len(codes),
		Op1: yakvm.NewValue(
			"opcodes", codes, "",
		)})
}

func (s *YakCompiler) pushIterableCall(i int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpIterableCall,
		Unary:  i,
	})
}
func (s *YakCompiler) pushListWithLen(i int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpList,
		Unary:  i,
	})
}

func (s *YakCompiler) pushCall(argCount int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{Opcode: yakvm.OpCall, Unary: argCount})
}

func (s *YakCompiler) pushCallWithWavy(argCount int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{Opcode: yakvm.OpCall, Unary: argCount, Op1: yakvm.NewBoolValue(true)})
}

// func (s *YakCompiler) pushCallWithVariadic(argCount int) {
// 	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{Opcode: yakvm.OpCall, Unary: argCount})
// }

func (s *YakCompiler) pushRef(i int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPushRef,
		Unary:  i,
	})
}

func (s *YakCompiler) pushLeftRef(i int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPushLeftRef,
		Unary:  i,
	})
}

func (s *YakCompiler) pushValue(i *yakvm.Value) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1:    i,
	})
}

func (s *YakCompiler) pushValueWithCopy(i *yakvm.Value) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Unary:  1,
		Op1:    i,
	})
}

func (s *YakCompiler) pushNewMap(count int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewMap,
		Unary:  count,
	})
}

func (s *YakCompiler) pushTypedMap(count int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewMapWithType,
		Unary:  count,
	})
}

func (s *YakCompiler) pushNewSlice(count int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewSlice,
		Unary:  count,
	})
}

func (s *YakCompiler) pushTypedSlice(count int) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewSliceWithType,
		Unary:  count,
	})
}

func (s *YakCompiler) pushString(i string, lit string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "string",
			Value:       i,
			Literal:     lit,
		},
	})
}

func (s *YakCompiler) pushPrefixString(prefix byte, i string, lit string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "string",
			Value:       i,
			Literal:     lit,
		},
		Unary: int(prefix),
	})
}

func (s *YakCompiler) pushOpPop() {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPop,
	})
}

func (s *YakCompiler) pushIdentifierName(i string) {
	if i == `undefined` {
		s.pushUndefined()
		return
	}
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPushId,
		Op1:    yakvm.NewIdentifierValue(i),
	})
}

func (s *YakCompiler) pushUndefined() {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1:    yakvm.GetUndefined(),
	})
}

func (s *YakCompiler) pushJmpIfTrue() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPT,
	}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushJmpIfTrueOrPop() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPTOP,
	}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushJmpIfFalse() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPF,
	}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushJmpIfFalseOrPop() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPFOP,
	}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) GetCodeIndex() int {
	return len(s.codes) - 1
}

func (s *YakCompiler) GetNextCodeIndex() int {
	// 获取下一跳 OpCode 的索引
	return len(s.codes)
}

func (s *YakCompiler) pushJmp() *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpJMP}
	s._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (s *YakCompiler) pushAssert(i int, defaultDesc string) {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpAssert,
		Unary:  i,
		Op1: yakvm.NewStringValue(
			fmt.Sprintf("assert error! expression code: %v", defaultDesc)),
	})
}

func (s *YakCompiler) pushContinue() {
	s._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpContinue,
		Op1:    yakvm.NewIntValue(s.getContinueScopeCounter()),
	})
}
