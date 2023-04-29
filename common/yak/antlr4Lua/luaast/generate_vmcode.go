package luaast

import (
	"fmt"
	"yaklang/common/yak/antlr4yak/yakvm"
)

const GLOBAL_ASSIGN_UNARY = 0
const LOCAL_ASSIGN_UNARY = 1

const OBJECT_METHOD = 0 // include self
const STATIC_METHOD = 1

func (l *LuaTranslator) _pushOpcodeWithCurrentCodeContext(codes ...*yakvm.Code) {
	for _, c := range codes {
		if l.currentStartPosition != nil && l.currentEndPosition != nil {
			c.StartLineNumber = l.currentStartPosition.LineNumber
			c.StartColumnNumber = l.currentStartPosition.ColumnNumber
			c.EndLineNumber = l.currentEndPosition.LineNumber
			c.EndColumnNumber = l.currentEndPosition.ColumnNumber
		}
	}
	l.codes = append(l.codes, codes...)
}

func (l *LuaTranslator) _popOpcode() {
	l.codes = l.codes[:len(l.codes)-1]
}

func (l *LuaTranslator) pushInteger(i int, origin string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "int",
			Value:       i,
			Literal:     origin,
		},
	})
}

func (l *LuaTranslator) pushInt64(i int64, origin string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "int64",
			Value:       i,
			Literal:     origin,
		},
	})
}
func (l *LuaTranslator) pushChar(i rune, origin string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "char",
			Value:       i,
			Literal:     origin,
		},
	})
}
func (l *LuaTranslator) pushByte(i byte, origin string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "byte", // uint8
			Value:       i,
			Literal:     origin,
		},
	})
}

func (l *LuaTranslator) pushBytes(i []byte, lit string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "bytes", // []uint8
			Value:       i,
			Literal:     lit,
		},
	})
}

func (l *LuaTranslator) pushFloat(i float64, origin string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "float64",
			Value:       i,
			Literal:     origin,
		},
	})
}

func (l *LuaTranslator) pushBool(i bool) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "bool",
			Value:       i,
			Literal:     fmt.Sprint(i),
		},
	})
}

func (l *LuaTranslator) pushType(i string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpType,
		Op1: &yakvm.Value{
			TypeVerbose: i,
		},
	})
}

func (l *LuaTranslator) pushMake(i int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpMake,
		Unary:  i,
	})
}

func (l *LuaTranslator) pushOperator(i yakvm.OpcodeFlag) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: i,
	})
}

func (l *LuaTranslator) pushGlobalAssign() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpAssign,
		Unary:  GLOBAL_ASSIGN_UNARY,
	})
}

func (l *LuaTranslator) pushLocalAssign() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpAssign,
		Unary:  LOCAL_ASSIGN_UNARY,
	})
}

func (l *LuaTranslator) pushLuaObjectMemberCall() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpMemberCall,
		Unary:  OBJECT_METHOD,
	})
}

func (l *LuaTranslator) pushLuaStaticMemberCall() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpMemberCall,
		Unary:  STATIC_METHOD,
	})
}

func (l *LuaTranslator) pushGlobalFastAssign() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpFastAssign,
		Unary:  GLOBAL_ASSIGN_UNARY,
	})
}

func (l *LuaTranslator) pushLocalFastAssign() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpFastAssign,
		Unary:  LOCAL_ASSIGN_UNARY,
	})
}

func (l *LuaTranslator) pushNOP() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNop,
	})
}

func (l *LuaTranslator) pushBreak() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpBreak,
		Op1:    yakvm.NewIntValue(l.getNearliestBreakScopeCounter()),
	})
}

func (l *LuaTranslator) pushEllipsis(n int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpEllipsis,
		Unary:  n,
	})
}

func (l *LuaTranslator) pushScope(i int) *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpScope, Unary: i}
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) pushInNext(i int) *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpInNext, Unary: i}
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) pushRangeNext(i int) *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpRangeNext, Unary: i}
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) pushExitFR(i int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{Opcode: yakvm.OpExitFR, Unary: i})
}

func (l *LuaTranslator) pushDefer(codes []*yakvm.Code) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpDefer,
		Unary:  len(codes),
		Op1: yakvm.NewValue(
			"opcodes", codes, "",
		)})
}

func (l *LuaTranslator) pushIterableCall(i int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpIterableCall,
		Unary:  i,
	})
}
func (l *LuaTranslator) pushListWithLen(i int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpList,
		Unary:  i,
	})
}

func (l *LuaTranslator) pushCall(argCount int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{Opcode: yakvm.OpCall, Unary: argCount})
}

func (l *LuaTranslator) pushCallWithVariadic(argCount int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{Opcode: yakvm.OpCall, Unary: argCount})
}

func (l *LuaTranslator) pushRef(i int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPushRef,
		Unary:  i,
	})
}

func (l *LuaTranslator) pushLeftRef(i int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPushLeftRef,
		Unary:  i,
	})
}

func (l *LuaTranslator) pushValue(i *yakvm.Value) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1:    i,
	})
}

func (l *LuaTranslator) pushNewMap(count int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewMap,
		Unary:  count,
	})
}

func (l *LuaTranslator) pushNewMapWithVariadicPos(count int, pos int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewMap,
		Unary:  count,
		Op1:    yakvm.NewIntValue(pos),
		Op2:    yakvm.GetUndefined(),
	})
}

func (l *LuaTranslator) pushNewVariadicMap(count int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewMap,
		Unary:  count,
		Op1:    yakvm.GetUndefined(), // 这个作为不定长map的标志位
	})
}

func (l *LuaTranslator) pushTypedMap(count int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewMapWithType,
		Unary:  count,
	})
}

func (l *LuaTranslator) pushNewSlice(count int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewSlice,
		Unary:  count,
	})
}

func (l *LuaTranslator) pushTypedSlice(count int) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpNewSliceWithType,
		Unary:  count,
	})
}

func (l *LuaTranslator) pushString(i string, lit string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "string",
			Value:       i,
			Literal:     lit,
		},
	})
}

func (l *LuaTranslator) pushPrefixString(prefix byte, i string, lit string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1: &yakvm.Value{
			TypeVerbose: "string",
			Value:       i,
			Literal:     lit,
		},
		Unary: int(prefix),
	})
}

func (l *LuaTranslator) pushOpPop() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPop,
	})
}

func (l *LuaTranslator) pushIdentifierName(i string) {
	if i == `undefined` {
		l.pushUndefined()
		return
	}
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPushId,
		Op1:    yakvm.NewIdentifierValue(i),
	})
}

func (l *LuaTranslator) pushUndefined() {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpPush,
		Op1:    yakvm.GetUndefined(),
	})
}

func (l *LuaTranslator) pushJmpIfTrue() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPT,
	}
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) pushJmpIfTrueOrPop() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPTOP,
	}
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) pushJmpIfFalse() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPF,
	}
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) pushJmpIfFalseOrPop() *yakvm.Code {
	code := &yakvm.Code{
		Opcode: yakvm.OpJMPFOP,
	}
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) GetCodeIndex() int {
	return len(l.codes) - 1
}

func (l *LuaTranslator) GetNextCodeIndex() int {
	// 获取下一跳 OpCode 的索引
	return len(l.codes)
}

func (l *LuaTranslator) pushJmp() *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpJMP}
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) pushJmpWithIndex(codeIndex int) *yakvm.Code {
	code := &yakvm.Code{Opcode: yakvm.OpJMP}
	code.Unary = codeIndex
	l._pushOpcodeWithCurrentCodeContext(code)
	return code
}

func (l *LuaTranslator) pushAssert(i int, defaultDesc string) {
	l._pushOpcodeWithCurrentCodeContext(&yakvm.Code{
		Opcode: yakvm.OpAssert,
		Unary:  i,
		Op1: yakvm.NewStringValue(
			fmt.Sprintf("assert error! expression code: %v", defaultDesc)),
	})
}
