package yakvm

import (
	"bytes"
	"fmt"
)

type OpcodeFlag int

const (
	OpNop OpcodeFlag = iota

	// OpTypeCast 从栈中取出两个值，第一个值为类型，第二个值为具体需要转换的值，进行类型转换，并把结果推入栈中
	OpTypeCast // type convert
	// Unary
	OpNot  // !
	OpNeg  // -
	OpPlus // +
	OpChan // <-

	OpPlusPlus   // ++
	OpMinusMinus // --
	/*
		^ & * <-
	*/

	// Binary
	OpShl    // <<
	OpShr    // >>
	OpAnd    // &
	OpAndNot // &^
	OpOr     // |
	OpXor    // ^

	OpAdd  // +
	OpSub  // -
	OpMul  // *
	OpDiv  // /
	OpMod  // %
	OpGt   // >
	OpLt   // <
	OpGtEq // >=
	OpLtEq // <=

	OpEq       // ==
	OpNotEq    // != <>
	OpPlusEq   // +=
	OpMinusEq  // -=
	OpMulEq    // *=
	OpDivEq    // /=
	OpModEq    // %=
	OpAndEq    // &=
	OpOrEq     // |=
	OpXorEq    // ^=
	OpShlEq    // <<=
	OpShrEq    // >>=
	OpAndNotEq // &^=

	OpIn       // in
	OpSendChan // <-

	OpType // type
	OpMake // make

	OpPush     // 把一个 Op1 压入栈
	OpPushfuzz // 把一个 Op1，执行 Fuzz String 操作，并且压入栈

	/*
		for range 和 for in 需要对右值表达式结果进行迭代，需要配合 Op 实现
	*/
	OpRangeNext // 从栈中取出一个元素，然后迭代, 并且压入栈
	OpInNext    // 从栈中取出一个元素，然后迭代, 并且压入栈, 与range稍微有点区别，比如可以解包slice以及迭代slice时第一个值时value而不是index
	OpEnterFR   // 进入for range, 从栈中peek值创建迭代器，并往IteratorStack栈中压入迭代器
	OpExitFR    // 退出for range, 判断IteratorStack是否已经结束，如果未结束则跳到for range开头(Unary),否则将pop IteratorStack并继续执行后续代码

	OpList // 这个操作有一个参数，从栈中取多少个元素组成 list，用 unary: int 标记

	// OpAssign 这个操作有两个参数
	// 左右值一般都会是 ValueList，所以 TypeVerbose 为 list, 将会设定类型断言 []*Value
	// popArgN 后，0 为右值，1 为左值
	OpAssign

	// OpFastAssign 快速赋值，存在于特殊的赋值中
	// 从栈中取两个值，arg1 为符号左，arg2 为具体值
	// 进行快速赋值，直接把值赋值给符号左，并且把 arg2 继续压入栈
	OpFastAssign

	// push 一个符号对应的值，这个值没有操作数 op1 op2 只操作 unary
	OpPushRef
	// 左值引用，是一个专属 push 一般用于赋值需要替换 **Value 的时候
	// 这个只操作了 Unary，Unary 传递具体的符号
	OpPushLeftRef

	// 除了跳转指令之外，其他指令都不应该直接操作 index！
	// JMP 无条件跳转到第几条指令，unary 记录指令数
	OpJMP
	// 从栈中取出一个值，如果值为 true 跳转到 unary 的指令中
	OpJMPT
	// 从栈中取出一个值，值为 false 跳转到 unary 的指令中
	OpJMPF
	// 从栈中查看最近的一个值，值为 true 则跳转到 unary 位置，否则 pop 出栈数据
	OpJMPTOP
	// 从栈中查看最近的一个值，值为 false 则跳转到 unary 位置，否则 pop 出栈数据
	OpJMPFOP

	// OpBreak 这个语句是为 break 设置的语句，一般是用来记录跳转到的位置，基本等同于 JMP
	// 不一样的是 Break 的位置，其实是无法事先预知的，
	// 所以需要 for 循环结束的最后一步，去寻找当前 for 循环中没有被设置过的 break 语句
	// 没有被设置之前，Unary 应该是小于等于零的
	// 因为 Break 也操作了指针，所以 OpBreak 结束后也不应该操作指针
	//
	// 和 JMP 不一样的地方是，Break/Continue 会破坏 scope 栈的平衡
	// 所以，这两个执行的时候，需要附带栈推出
	OpBreak
	OpContinue

	// OpCall / OpVariadicCall 从 unary 中取应该 pop 多少个数
	// 并且再重栈中 pop 出应该调用的内容
	OpCall
	OpVariadicCall /*这个是针对可变参数的调用*/

	// OpPushId push 一个引用名字，这个名字可能是无法获取到符号表中的符号，但是被使用了
	// 所以这个不能使用 unary，使用 op1 类型为 Identifier 作为操作数，找不到就是 nil 或者 undefined
	OpPushId

	// OpPop 一般用于维持栈平衡，比如说 push 一个表达式语句，一般不会 pop，需要用 OpPop 来弹出
	OpPop

	// OpNewMap 这个指令用于创建一个 map，从栈中取出 unary * 2 个数据，然后俩俩组合，左边为 key 右边为 value
	OpNewMap

	// OpNewMapWithType 这个指令用于创建一个 map，从栈中取出 unary * 2 个数据，然后俩俩组合，左边为 key 右边为 value, 再取出 Type，组合成 Map
	OpNewMapWithType

	// OpNewSlice 用于从栈中创建 Slice，取出 unary 个数据，推断类型，然后组合成 slice
	OpNewSlice

	// OpNewSliceWithType 从栈中创建 Slice，取出 unary 个操作数，再取出 Type，组合成 Slice
	OpNewSliceWithType

	// OpSliceCall 索引Silice
	OpIterableCall

	// OpReturn 从栈取一个数据出来，复制给返回值缓存数据，一般来说，可以用 lastStackValue 来取数
	OpReturn

	// OpDefer 执行 op1，一般 op1 的值必须是 codes 也就是 []*Opcode
	// 作为虚拟机退出的时候需要执行的值
	OpDefer
	// OpAssert 从栈中取出几一个或两个参数，然后断言类型，如果是false，就 panic参数第二个参数
	OpAssert

	// OpMemberCall 获取map或结构体的成员变量或方法
	OpMemberCall

	// OpAsyncCall 执行 goroutine
	// unary 为
	OpAsyncCall

	// OpScope 会新创建一个定义域，通过 OpScopeEnd 来停止定义域
	// 定义域是一个树形结构，保存了父定义域的引用，因为需要看到父定义域的内容
	OpScope
	OpScopeEnd

	// include 会直接从栈中pop文件路径然后执行
	OpInclude

	// OpPanic 主动 panic 掉，然后把错误交给 Defer 的 Recover 实现
	OpPanic
	OpRecover

	// OpEllipsis 函数不定参数调用拆包
	OpEllipsis
	// OpBitwiseNot 按位取反
	OpBitwiseNot

	OpCatchError
	OpStopCatchError
	OpExit
)

func (f OpcodeFlag) IsJmp() bool {
	return f == OpJMP || f == OpJMPT || f == OpJMPF || f == OpJMPTOP || f == OpJMPFOP || f == OpRangeNext || f == OpInNext || f == OpBreak || f == OpContinue || f == OpEnterFR || f == OpExitFR
}

func OpcodeToName(op OpcodeFlag) string {
	i, ok := OpcodeVerboseName[op]
	if ok {
		return i
	}
	return fmt.Sprintf("unknown[%v]", op)
}

var OpcodeVerboseName = map[OpcodeFlag]string{
	OpBitwiseNot: `not`,
	OpAnd:        `and`,
	OpAndNot:     `and-not`,
	OpOr:         `or`,
	OpXor:        `xor`,
	OpShl:        `shl`,
	OpShr:        `shr`,
	OpTypeCast:   `type-cast`,
	OpPlusPlus:   `self-add-one`,
	OpMinusMinus: `self-minus-one`,
	OpNot:        `bang`,
	OpNeg:        `neg`,
	OpPlus:       `plus`,
	OpChan:       `chan-recv`,
	OpAdd:        `add`,
	OpSub:        `sub`,
	OpMod:        `mod`,
	OpMul:        `mul`,
	OpDiv:        `div`,
	OpGt:         `gt`,
	OpLt:         `lt`,
	OpLtEq:       `lt-eq`,
	OpGtEq:       `gt-eq`,
	OpNotEq:      `neq`,
	OpEq:         `eq`,
	OpPlusEq:     `self-plus-eq`,
	OpMinusEq:    `self-minus-eq`,
	OpMulEq:      `self-mul-eq`,
	OpDivEq:      `self-div-eq`,
	OpModEq:      `self-mod-eq`,
	OpAndEq:      `self-and-eq`,
	OpOrEq:       `self-or-eq`,
	OpXorEq:      `self-xor-eq`,
	OpShlEq:      `self-shl-eq`,
	OpShrEq:      `self-shr-eq`,
	OpAndNotEq:   `self-and-not-eq`,

	OpIn:       `in`,
	OpSendChan: `chan-send`,

	OpRangeNext: `range-next`,
	OpInNext:    `in-next`,
	OpEnterFR:   `enter-for-range`,
	OpExitFR:    `exit-for-range`,

	OpType:        `type`,
	OpMake:        `make`,
	OpPush:        `push`,
	OpList:        `list`,
	OpAssign:      `assign`,
	OpFastAssign:  `fast-assign`,
	OpPushRef:     `pushr`,
	OpPushLeftRef: `pushleftr`,
	OpJMP:         `jmp`,
	OpJMPT:        `jmpt`,
	OpJMPF:        `jmpf`,
	OpJMPTOP:      `jmpt-or-pop`,
	OpJMPFOP:      `jmpf-or-pop`,

	OpCall:             `call`,
	OpVariadicCall:     `callvar`,
	OpPushId:           `pushid`,
	OpPop:              `pop`,
	OpPushfuzz:         `pushf`,
	OpNewMap:           `newmap`,
	OpNewMapWithType:   `typedmap`,
	OpNewSlice:         `newslice`,
	OpNewSliceWithType: `typedslice`,
	OpIterableCall:     `iterablecall`,
	OpReturn:           `return`,
	OpAssert:           `assert`,
	OpDefer:            `defer`,
	OpMemberCall:       `membercall`,
	OpBreak:            `break`,
	OpContinue:         `continue`,
	OpAsyncCall:        "async-call",

	OpScope:    `new-scope`,
	OpScopeEnd: `end-scope`,

	OpInclude: `include`,

	OpRecover:  `recover`,
	OpPanic:    `panic`,
	OpEllipsis: `ellipsis`,

	OpCatchError:     `catch-error`,
	OpStopCatchError: `stop-catch-error`,
	OpExit:           `exit`,
}

type Code struct {
	Opcode OpcodeFlag

	Unary int
	Op1   *Value
	Op2   *Value

	// 记录 Opcode 的位置
	SourceCodeFilePath *string
	SourceCodePointer  *string
	StartLineNumber    int
	StartColumnNumber  int
	EndLineNumber      int
	EndColumnNumber    int
}

func (c *Code) IsJmp() bool {
	return c.Opcode.IsJmp()
}

func (c *Code) GetJmpIndex() int {
	flag := c.Opcode
	if !flag.IsJmp() {
		return -1
	}
	if flag == OpInNext || flag == OpRangeNext {
		return c.Op1.Int()
	}
	return c.Unary
}

func (c *Code) RangeVerbose() string {
	return fmt.Sprintf(
		"%v:%v->%v:%v",
		c.StartLineNumber, c.StartColumnNumber,
		c.EndLineNumber, c.EndColumnNumber,
	)
}

func (c *Code) String() string {
	var buf bytes.Buffer
	op, ok := OpcodeVerboseName[c.Opcode]
	if !ok {
		op = "unknown[" + fmt.Sprint(c.Opcode) + "]"
	}
	buf.WriteString(fmt.Sprintf("OP:%-20s", op) + " ")
	switch c.Opcode {
	case OpBitwiseNot, OpAnd, OpAndNot, OpOr, OpXor, OpShl, OpShr:
	case OpTypeCast:
	case OpPlusPlus, OpMinusMinus:
	case OpNot, OpNeg, OpPlus, OpChan:
	case OpScope:
		buf.WriteString(fmt.Sprint(c.Unary))
	case OpScopeEnd:
	case OpMake:
	case OpType:
		buf.WriteString(c.Op1.TypeVerbose)
	case OpPush:
		buf.WriteString(c.Op1.String())
		if c.Unary == 1 {
			buf.WriteString(" (copy)")
		}
	case OpPushId, OpPushfuzz:
		buf.WriteString(c.Op1.String())
		// 特殊的 push 用来处理 f 作为 prefix 的前缀 push string
	case OpAdd, OpSub, OpMul, OpDiv, OpMod, OpIn, OpSendChan:
	case OpGt, OpLt, OpGtEq, OpLtEq, OpEq, OpNotEq, OpPlusEq, OpMinusEq, OpMulEq, OpDivEq, OpModEq, OpAndEq, OpOrEq, OpXorEq, OpShlEq, OpShrEq:
	case OpPop, OpReturn, OpRecover, OpPanic:
	case OpCall, OpVariadicCall, OpDefer, OpAsyncCall:
		buf.WriteString(fmt.Sprintf("vlen:%d", c.Unary))
	case OpRangeNext, OpInNext, OpFastAssign:
	case OpJMP, OpJMPT, OpJMPF, OpJMPTOP, OpJMPFOP, OpEnterFR, OpExitFR:
		buf.WriteString(fmt.Sprintf("-> %d", c.Unary))
	case OpAssign:
		switch c.Op1.String() {
		case "nasl_global_declare", "nasl_declare":
			buf.WriteString("-> with pop")
		}
	case OpContinue:
		buf.WriteString(fmt.Sprintf("-> %d (-%d scope)", c.Unary, c.Op1.Int()))
	case OpBreak:
		if c.Op2.Int() != 2 {
			buf.WriteString(fmt.Sprintf("-> %d (-%d scope) mode: %v", c.Unary, c.Op1.Int(), c.Op2.String()))
		} else {
			buf.Reset()
			buf.WriteString(fmt.Sprintf("OP:%-20s", "fallthrough") + " ")
			buf.WriteString(fmt.Sprintf("-> %d (-%d scope)", c.Unary, c.Op1.Int()))
		}
	case OpList, OpPushRef, OpNewMap, OpNewMapWithType, OpNewSlice, OpNewSliceWithType, OpPushLeftRef:
		buf.WriteString(fmt.Sprint(c.Unary))
	case OpCatchError:
		buf.WriteString(fmt.Sprintf("err -> %d", c.Op1.Int()+1))
	case OpStopCatchError, OpExit:
	default:
		if c.Unary > 0 {
			buf.WriteString("off:" + fmt.Sprint(c.Unary) + " ")
		}
		buf.WriteString("op1: " + c.Op1.String())
		buf.WriteString("\t\t")
		buf.WriteString("op2: " + c.Op2.String())
	}
	return buf.String()
}
func (c *Code) Dump() {
	println(fmt.Sprintf("%-13s %v", c.RangeVerbose(), c.String()))
}
