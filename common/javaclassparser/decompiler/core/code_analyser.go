package core

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	utils2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"golang.org/x/exp/slices"
)

type BootstrapMethod struct {
	Ref       values.JavaValue
	Arguments []values.JavaValue
}
type ExceptionTableEntry struct {
	StartPc   uint16
	EndPc     uint16
	HandlerPc uint16
	CatchType uint16
}

type Decompiler struct {
	FunctionType          *types.JavaFuncType
	opcodeToSimulateStack map[*OpCode]*StackSimulationImpl
	FunctionContext       *class_context.ClassContext
	varTable              map[int]*values.JavaRef
	opcodeIdToRef         map[*OpCode][][2]any
	// dupConvertedRefValue records, per dup-family opcode and in the SAME order as the
	// opcodeIdToRef entries appended by checkAndConvertRef, the actual value each synthesized
	// temp was created from. The dup statement-parse handler must use this instead of
	// stackConsumed[i]: when checkAndConvertRef converts a value that is NOT on top of the
	// consume order (e.g. the array reference under the index in `this.f[i] op= v`, whose
	// dup2 pops index first), stackConsumed[i] points at the wrong operand and would emit a
	// bogus assignment like `int t = i; t[i] = ...`.
	dupConvertedRefValue          map[*OpCode][]values.JavaValue
	bytecodes                     []byte
	opCodes                       []*OpCode
	RootOpCode                    *OpCode
	RootNode                      *Node
	constantPoolGetter            func(id int) values.JavaValue
	ConstantPoolLiteralGetter     func(constantPoolGetterid int) values.JavaValue
	ConstantPoolInvokeDynamicInfo func(id int) (uint16, string, string)
	offsetToOpcodeIndex           map[uint16]int
	opcodeIndexToOffset           map[int]uint16
	ExceptionTable                []*ExceptionTableEntry
	BootstrapMethods              []*BootstrapMethod
	DumpClassLambdaMethod         func(name, desc string, id *utils2.VariableId, capturedCount int) (string, error)
	InvokeDynamicName             string
	CurrentId                     int
	BodyStartId                   int
	BaseVarId                     *utils2.VariableId
	Params                        []values.JavaValue
	ifNodeConditionCallback       map[*OpCode]func(value values.JavaValue)

	varUserMap     *omap.OrderedMap[*values.JavaRef, []*VarFoldRule]
	disFoldRef     []*values.JavaRef
	delRefUserAttr map[string][3]int // [0] = del times,[1] = assign times, [2] = self assign

	// selfOpFoldedRefs holds the VarUid of dup/dup_x1-created temporaries whose only purpose was
	// to carry the old value of a field/static post-increment/decrement (the `x++` / `x--`
	// idiom). When the putfield/putstatic is folded into the post-op expression, the temporary's
	// assignment statement must not be emitted, otherwise it would leave a side-effect statement
	// in a ternary/expression branch and break structuring (multiple next).
	selfOpFoldedRefs map[string]bool
}
type VarFoldRule struct {
	Replace          func(v values.JavaValue)
	CurrentOpcode    *OpCode
	UserIsNextOpcode bool
}

func NewDecompiler(bytecodes []byte, constantPoolGetter func(id int) values.JavaValue) *Decompiler {
	return &Decompiler{
		FunctionContext:      &class_context.ClassContext{},
		bytecodes:            bytecodes,
		constantPoolGetter:   constantPoolGetter,
		offsetToOpcodeIndex:  map[uint16]int{},
		opcodeIndexToOffset:  map[int]uint16{},
		varTable:             map[int]*values.JavaRef{},
		opcodeIdToRef:        map[*OpCode][][2]any{},
		dupConvertedRefValue: map[*OpCode][]values.JavaValue{},
		varUserMap:           omap.NewEmptyOrderedMap[*values.JavaRef, []*VarFoldRule](),
		delRefUserAttr:       map[string][3]int{},
		selfOpFoldedRefs:     map[string]bool{},
	}
}

func (d *Decompiler) GetValueFromPool(index int) values.JavaValue {
	return d.constantPoolGetter(index)
}

func (d *Decompiler) GetMethodFromPool(index int) *values.JavaClassMember {
	return d.constantPoolGetter(index).(*values.JavaClassMember)
}
func (d *Decompiler) ParseOpcode() (err error) {
	defer func() {
		if e := recover(); e != nil {
			if os.Getenv("DEC_PANIC_STACK") != "" {
				err = fmt.Errorf("%v\n%s", e, debug.Stack())
			} else {
				err = fmt.Errorf("%v", e)
			}
		}
	}()
	defer func() {
		if len(d.opCodes) > 0 {
			d.RootOpCode = d.opCodes[0]
		}
	}()
	// Capacity hint: every opcode consumes at least one bytecode byte, and the
	// shortest instructions are 1-2 bytes, so len(bytecodes)/2 closely tracks the
	// real opcode count for typical code while bounding the worst case. Pre-sizing
	// the slice and offset maps avoids the repeated grow/rehash garbage that made
	// ParseOpcode the single largest core allocator.
	sizeHint := len(d.bytecodes)/2 + 8
	opcodes := make([]*OpCode, 0, sizeHint)
	opcodes = append(opcodes, &OpCode{Instr: &Instruction{OpCode: OP_START}})
	offsetToIndex := make(map[uint16]int, sizeHint)
	indexToOffset := make(map[int]uint16, sizeHint)
	reader := NewJavaByteCodeReader(d.bytecodes)
	id := 1
	isWide := false
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		current := reader.CurrentPos - 1
		instr, ok := InstrInfos[int(b)]
		if !ok {
			return fmt.Errorf("unknow op: %x", b)
		}
		if instr.OpCode == OP_WIDE {
			isWide = true
			continue
		}
		opcode := &OpCode{Instr: instr, Id: id, CurrentOffset: uint16(current)}
		if isWide {
			opcode.IsWide = true
			isWide = false
		}
		var factory = DefaultFactory
		if v, ok := OpFactories[instr.HandleName]; ok {
			factory = v
		}
		err = factory(reader, opcode)
		if err != nil {
			return err
		}
		opcodes = append(opcodes, opcode)
		offsetToIndex[uint16(current)] = len(opcodes) - 1
		indexToOffset[len(opcodes)-1] = uint16(current)
		id++
	}
	d.offsetToOpcodeIndex = offsetToIndex
	d.opcodeIndexToOffset = indexToOffset
	d.CurrentId = id
	d.opCodes = opcodes
	return nil
}

func (d *Decompiler) ScanJmp() error {
	opcodes := d.opCodes
	// Plain map (single-goroutine walk) avoids the mutex + interface boxing of utils.Set.
	visitNodeRecord := make(map[*OpCode]struct{})
	endOp := &OpCode{Instr: InstrInfos[OP_END], Id: d.CurrentId}
	var walkNode func(start int)
	walkNode = func(start int) {
		deferWalkId := []int{}
		defer func() {
			for _, id := range deferWalkId {
				walkNode(id)
			}
		}()
		var pre *OpCode
		i := start
		for {
			if i >= len(opcodes) {
				break
			}
			opcode := opcodes[i]
			if opcode.Instr.OpCode == OP_START {
				i++
				pre = opcode
				continue
			}
			if pre != nil {
				LinkOpcode(pre, opcode)
			}
			for _, entry := range d.ExceptionTable {
				if opcode.CurrentOffset == entry.StartPc {
					if entry.StartPc == entry.HandlerPc {
						continue
					}
					gotoOp := d.offsetToOpcodeIndex[entry.HandlerPc]
					d.opCodes[gotoOp].IsCatch = true
					d.opCodes[gotoOp].ExceptionTypeIndex = entry.CatchType
					// Accumulate every catch type that shares this handler PC so multi-catch
					// (`catch (A | B)`) can be reconstructed. Dedupe because the exception-table
					// scan may revisit the same start opcode while walking the graph.
					if !slices.Contains(d.opCodes[gotoOp].ExceptionTypeIndexes, entry.CatchType) {
						d.opCodes[gotoOp].ExceptionTypeIndexes = append(d.opCodes[gotoOp].ExceptionTypeIndexes, entry.CatchType)
					}
					deferWalkId = append(deferWalkId, gotoOp)
					walkNode(gotoOp)
					if pre != nil {
						LinkOpcode(pre, d.opCodes[gotoOp])
						pre.IsTryCatchParent = true
						pre.TryNode = opcode
						pre.CatchNode = append(pre.CatchNode, &CatchNode{
							ExceptionTypeIndex: entry.CatchType,
							StartIndex:         entry.StartPc,
							EndIndex:           entry.EndPc,
							OpCode:             d.opCodes[gotoOp],
						})
					}
				}
			}

			if _, ok := visitNodeRecord[opcode]; ok {
				break
			}
			visitNodeRecord[opcode] = struct{}{}
			pre = opcode
			switch opcode.Instr.OpCode {
			case OP_RETURN, OP_IRETURN, OP_ARETURN, OP_LRETURN, OP_DRETURN, OP_FRETURN, OP_ATHROW:
				LinkOpcode(opcode, endOp)
				pre = nil
			case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
				gotoRaw := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+gotoRaw]
				LinkOpcode(opcode, d.opCodes[gotoOp])
				walkNode(gotoOp)
			case OP_GOTO:
				target := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+target]
				LinkOpcode(opcode, d.opCodes[gotoOp])
				walkNode(gotoOp)
				return
			case OP_GOTO_W:
				target := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+target]
				LinkOpcode(opcode, d.opCodes[gotoOp])
				walkNode(gotoOp)
				return
			case OP_LOOKUPSWITCH, OP_TABLESWITCH:
				opcode.SwitchJmpCase.ForEach(func(v int, target int32) bool {
					gotoOp := d.offsetToOpcodeIndex[uint16(target)]
					// Several case values can share one handler (`case 1: case 2: ...`). When the
					// target is already linked, map this case to the EXISTING target's index, not
					// len-1: len-1 is whatever was appended last, so it pointed at an unrelated body
					// (a correctness bug) and, once later passes shrink node.Next, could exceed it
					// and panic the switch rewriter with index-out-of-range.
					if idx := slices.Index(opcode.Target, d.opCodes[gotoOp]); idx >= 0 {
						opcode.SwitchJmpCase1.Set(v, idx)
						return true
					}
					LinkOpcode(opcode, d.opCodes[gotoOp])
					opcode.SwitchJmpCase1.Set(v, len(opcode.Target)-1)
					walkNode(gotoOp)
					return true
				})

				return
			}
			i++
		}
	}
	walkNode(0)
	d.opCodes = append(d.opCodes, endOp)
	return nil
}
func (d *Decompiler) DropUnreachableOpcode() error {
	// DropUnreachableOpcode and nop
	// Plain map (single-goroutine walk); WalkGraph copies the returned slice into its own
	// stack and never mutates/retains it, so code.Target can be returned directly instead of
	// allocating a per-node copy.
	visitNodeRecord := make(map[*OpCode]struct{})
	err := WalkGraph[*OpCode](d.opCodes[0], func(code *OpCode) ([]*OpCode, error) {
		visitNodeRecord[code] = struct{}{}
		return code.Target, nil
	})
	if err != nil {
		return err
	}
	var newOpcodes []*OpCode
	for _, code := range d.opCodes {
		if _, ok := visitNodeRecord[code]; !ok {
			continue
		}
		if code.Instr.OpCode == OP_NOP {
			for _, source := range code.Source {
				source.Target = funk.Filter(source.Target, func(opCode *OpCode) bool {
					return opCode != code
				}).([]*OpCode)
				source.Target = append(source.Target, code.Target...)
			}
		} else {
			newOpcodes = append(newOpcodes, code)
		}
	}
	d.opCodes = newOpcodes
	return nil
}
func (d *Decompiler) getPoolValue(index int) values.JavaValue {
	return d.constantPoolGetter(index)
}

// isLiteralIntOne reports whether a java literal is the integer constant 1, regardless of the
// concrete numeric Go type produced by the various *const/push opcodes.
func isLiteralIntOne(v values.JavaValue) bool {
	lit, ok := UnpackSoltValue(v).(*values.JavaLiteral)
	if !ok {
		return false
	}
	switch n := lit.Data.(type) {
	case int:
		return n == 1
	case int8:
		return n == 1
	case int16:
		return n == 1
	case int32:
		return n == 1
	case int64:
		return n == 1
	case uint8:
		return n == 1
	case uint16:
		return n == 1
	case uint32:
		return n == 1
	case uint64:
		return n == 1
	default:
		return fmt.Sprint(lit.Data) == "1"
	}
}

// tryFoldPostIncDec recognizes the post-increment / post-decrement idiom for a field or static
// whose stored value is `old (+|-) 1`, where `old` (the loaded field value) was duplicated on the
// stack via dup/dup_x1 and is reused as the expression result. When the value currently on top of
// the stack is exactly that same `old` object, the putfield/putstatic is the post-inc/dec of the
// field, so we return the folded `old++` / `old--` expression. The caller replaces the bare old
// value on the stack with this expression and drops the standalone assignment, keeping
// ternary/expression branches side-effect-free so the structuring pass can fold them.
func (d *Decompiler) tryFoldPostIncDec(stack StackSimulation, storedValue values.JavaValue) (values.JavaValue, bool) {
	expr, ok := UnpackSoltValue(storedValue).(*values.JavaExpression)
	if !ok || len(expr.Values) != 2 {
		return nil, false
	}
	if expr.Op != ADD && expr.Op != SUB {
		return nil, false
	}
	if !isLiteralIntOne(expr.Values[1]) {
		return nil, false
	}
	if stack.Size() == 0 {
		return nil, false
	}
	// The old field value must be exactly the value currently on top of the stack (placed there
	// by the dup/dup_x1 that captured it for reuse), proving this is the post-op idiom.
	top := stack.Peek()
	if UnpackSoltValue(top) != UnpackSoltValue(expr.Values[0]) {
		return nil, false
	}
	// Suppress the temporary that dup/dup_x1 created to carry the old value, if any, so its
	// assignment statement is not emitted into the (ternary/expression) branch.
	if ref, ok := UnpackSoltValue(top).(*values.JavaRef); ok {
		d.selfOpFoldedRefs[ref.VarUid] = true
	}
	op := INC
	if expr.Op == SUB {
		op = DEC
	}
	// Render against the real field reference (this.f / Class.f), not the synthetic temporary.
	target := GetRealValue(UnpackSoltValue(top))
	return values.NewBinaryExpression(target, expr.Values[1], op, target.Type()), true
}

func (d *Decompiler) calcOpcodeStackInfo(runtimeStackSimulation StackSimulation, opcode *OpCode) error {
	funcCtx := d.FunctionContext
	checkAndConvertRef := func(value values.JavaValue) func(int) {
		if _, ok := UnpackSoltValue(runtimeStackSimulation.Peek()).(*values.JavaRef); !ok {
			val := runtimeStackSimulation.Pop().(values.JavaValue)
			ref := runtimeStackSimulation.NewVar(val)
			d.opcodeIdToRef[opcode] = append(d.opcodeIdToRef[opcode], [2]any{ref, true})
			// Record the real source value so the dup statement-parse handler does not rely on
			// stackConsumed[i] (which is mis-indexed when this is not the top-of-consume operand).
			d.dupConvertedRefValue[opcode] = append(d.dupConvertedRefValue[opcode], val)
			attr := d.delRefUserAttr[ref.VarUid]
			attr[2] = 1
			d.delRefUserAttr[ref.VarUid] = attr
			slotVal := values.NewSlotValue(ref, ref.Type())
			addUser := func(n int) {
				for i := 0; i < n; i++ {
					d.varUserMap.Set(ref, append(d.varUserMap.GetMust(ref), &VarFoldRule{
						Replace: func(v values.JavaValue) {
							slotVal.ResetValue(v)
						},
						CurrentOpcode: opcode,
					}))
				}
			}
			runtimeStackSimulation.Push(slotVal)
			addUser(1)
			return addUser
			//appendNode(statements.NewAssignStatement(ref, val, true))
		}
		return func(n int) {
		}
	}
	loadVarBySlot := func(slot int) values.JavaValue {
		varRef := runtimeStackSimulation.GetVar(slot)
		slotvalue := values.NewSlotValue(varRef, varRef.Type())
		users := d.varUserMap.GetMust(varRef)
		d.varUserMap.Set(varRef, append(users, &VarFoldRule{
			Replace: func(v values.JavaValue) {
				slotvalue.ResetValue(v)
			},
			CurrentOpcode: opcode,
		}))
		return slotvalue
	}
	switch opcode.Instr.OpCode {
	case OP_ALOAD, OP_ILOAD, OP_LLOAD, OP_DLOAD, OP_FLOAD, OP_ALOAD_0, OP_ILOAD_0, OP_LLOAD_0, OP_DLOAD_0, OP_FLOAD_0, OP_ALOAD_1, OP_ILOAD_1, OP_LLOAD_1, OP_DLOAD_1, OP_FLOAD_1, OP_ALOAD_2, OP_ILOAD_2, OP_LLOAD_2, OP_DLOAD_2, OP_FLOAD_2, OP_ALOAD_3, OP_ILOAD_3, OP_LLOAD_3, OP_DLOAD_3, OP_FLOAD_3:
		var slot int = -1
		switch opcode.Instr.OpCode {
		case OP_ALOAD_0, OP_ILOAD_0, OP_LLOAD_0, OP_DLOAD_0, OP_FLOAD_0:
			slot = 0
		case OP_ALOAD_1, OP_ILOAD_1, OP_LLOAD_1, OP_DLOAD_1, OP_FLOAD_1:
			slot = 1
		case OP_ALOAD_2, OP_ILOAD_2, OP_LLOAD_2, OP_DLOAD_2, OP_FLOAD_2:
			slot = 2
		case OP_ALOAD_3, OP_ILOAD_3, OP_LLOAD_3, OP_DLOAD_3, OP_FLOAD_3:
			slot = 3
		default:
			slot = GetRetrieveIdx(opcode)
		}

		runtimeStackSimulation.Push(loadVarBySlot(slot))
		////return mkRetrieve(variableFactory);
	case OP_ACONST_NULL:
		runtimeStackSimulation.Push(values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")))
	case OP_ICONST_M1:
		runtimeStackSimulation.Push(values.NewJavaLiteral(-1, types.NewJavaPrimer(types.JavaInteger)))
	case OP_ICONST_0:
		runtimeStackSimulation.Push(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
	case OP_ICONST_1:
		runtimeStackSimulation.Push(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
	case OP_ICONST_2:
		runtimeStackSimulation.Push(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger)))
	case OP_ICONST_3:
		runtimeStackSimulation.Push(values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger)))
	case OP_ICONST_4:
		runtimeStackSimulation.Push(values.NewJavaLiteral(4, types.NewJavaPrimer(types.JavaInteger)))
	case OP_ICONST_5:
		runtimeStackSimulation.Push(values.NewJavaLiteral(5, types.NewJavaPrimer(types.JavaInteger)))
	case OP_LCONST_0:
		runtimeStackSimulation.Push(values.NewJavaLiteral(int64(0), types.NewJavaPrimer(types.JavaLong)))
	case OP_LCONST_1:
		runtimeStackSimulation.Push(values.NewJavaLiteral(int64(1), types.NewJavaPrimer(types.JavaLong)))
	case OP_FCONST_0:
		runtimeStackSimulation.Push(values.NewJavaLiteral(float32(0), types.NewJavaPrimer(types.JavaFloat)))
	case OP_FCONST_1:
		runtimeStackSimulation.Push(values.NewJavaLiteral(float32(1), types.NewJavaPrimer(types.JavaFloat)))
	case OP_FCONST_2:
		runtimeStackSimulation.Push(values.NewJavaLiteral(float32(2), types.NewJavaPrimer(types.JavaFloat)))
	case OP_DCONST_0:
		runtimeStackSimulation.Push(values.NewJavaLiteral(float64(0), types.NewJavaPrimer(types.JavaDouble)))
	case OP_DCONST_1:
		runtimeStackSimulation.Push(values.NewJavaLiteral(float64(1), types.NewJavaPrimer(types.JavaDouble)))
	case OP_BIPUSH:
		// The bipush operand is a SIGNED byte. Reading it unsigned turns -5 (0xFB) into 251 and
		// silently corrupts every negative byte literal; sign-extend via int8. Using a plain int
		// (not byte) also lets JavaLiteral.String render boolean 0/1 as false/true.
		runtimeStackSimulation.Push(values.NewJavaLiteral(int(int8(opcode.Data[0])), types.NewJavaPrimer(types.JavaInteger)))
	case OP_SIPUSH:
		// The sipush operand is a SIGNED short. Convert2bytesToInt returns uint16, so -10 (0xFFF6)
		// would become 65526; sign-extend through int16 before widening to int.
		runtimeStackSimulation.Push(values.NewJavaLiteral(int(int16(Convert2bytesToInt(opcode.Data))), types.NewJavaPrimer(types.JavaInteger)))
	case OP_ISTORE, OP_ASTORE, OP_LSTORE, OP_DSTORE, OP_FSTORE, OP_ISTORE_0, OP_ASTORE_0, OP_LSTORE_0, OP_DSTORE_0, OP_FSTORE_0, OP_ISTORE_1, OP_ASTORE_1, OP_LSTORE_1, OP_DSTORE_1, OP_FSTORE_1, OP_ISTORE_2, OP_ASTORE_2, OP_LSTORE_2, OP_DSTORE_2, OP_FSTORE_2, OP_ISTORE_3, OP_ASTORE_3, OP_LSTORE_3, OP_DSTORE_3, OP_FSTORE_3:
		slot := GetStoreIdx(opcode)
		value := runtimeStackSimulation.Pop().(values.JavaValue)
		ref, isFirst := runtimeStackSimulation.AssignVar(slot, value)
		statements.NewAssignStatement(ref, value, isFirst)
		d.opcodeIdToRef[opcode] = append(d.opcodeIdToRef[opcode], [2]any{ref, isFirst})
		attr := d.delRefUserAttr[ref.VarUid]
		attr[1]++
		d.delRefUserAttr[ref.VarUid] = attr

		if v, ok := value.(*values.CustomValue); ok {
			if v.Flag == "exception" {
				loadVarBySlot(slot)
			}
		}
		if !isFirst {
			d.disFoldRef = append(d.disFoldRef, ref)
		}
	case OP_NEW:
		n := Convert2bytesToInt(opcode.Data)
		javaClass := d.constantPoolGetter(int(n)).(*values.JavaClassValue)
		//runtimeStackSimulation.Push(javaClass)
		runtimeStackSimulation.Push(values.NewNewExpression(javaClass.Type()))
		//appendNode()
	case OP_NEWARRAY:
		length := runtimeStackSimulation.Pop().(values.JavaValue)
		primerTypeName := types.GetPrimerArrayType(int(opcode.Data[0]))
		runtimeStackSimulation.Push(values.NewNewArrayExpression(types.NewJavaArrayType(primerTypeName), length))
	case OP_ANEWARRAY:
		value := d.getPoolValue(int(Convert2bytesToInt(opcode.Data)))
		length := runtimeStackSimulation.Pop().(values.JavaValue)
		arrayType := types.NewJavaArrayType(value.(*values.JavaClassValue).Type())
		exp := values.NewNewArrayExpression(arrayType, length)
		runtimeStackSimulation.Push(exp)
	case OP_MULTIANEWARRAY:
		// The constant-pool entry is ALREADY the full array class type (e.g. "[[I" is
		// int[][]); the third operand byte is the count of explicitly-sized leading
		// dimensions whose lengths are on the stack (always <= the array rank). The
		// type must be used as-is: re-wrapping it once per popped dimension doubled the
		// rank, turning `new int[3][4]` into a 7-dimensional `new int[3][4][][]`.
		typ := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data[:2]))).(*values.JavaClassValue).Type()
		dims := int(opcode.Data[2])
		lens := make([]values.JavaValue, 0, dims)
		for _, v := range runtimeStackSimulation.PopN(dims) {
			lens = append(lens, v.(values.JavaValue))
		}
		lens = funk.Reverse(lens).([]values.JavaValue)
		exp := values.NewNewArrayExpression(typ, lens...)
		runtimeStackSimulation.Push(exp)
	case OP_ARRAYLENGTH:
		ref := runtimeStackSimulation.Pop().(values.JavaValue)
		runtimeStackSimulation.Push(values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			return fmt.Sprintf("%s.length", ref.String(funcCtx))
		}, func() types.JavaType {
			return types.NewJavaPrimer(types.JavaInteger)
		}))
	case OP_AALOAD, OP_IALOAD, OP_BALOAD, OP_CALOAD, OP_FALOAD, OP_LALOAD, OP_DALOAD, OP_SALOAD:
		index := runtimeStackSimulation.Pop().(values.JavaValue)
		ref := runtimeStackSimulation.Pop().(values.JavaValue)
		runtimeStackSimulation.Push(values.NewJavaArrayMember(ref, index))
	case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
		value := runtimeStackSimulation.Pop().(values.JavaValue)
		index := runtimeStackSimulation.Pop().(values.JavaValue)
		ref := runtimeStackSimulation.Pop().(values.JavaValue)
		statements.NewArrayMemberAssignStatement(values.NewJavaArrayMember(ref, index), value)
	case OP_LCMP, OP_DCMPG, OP_DCMPL, OP_FCMPG, OP_FCMPL:
		var1 := runtimeStackSimulation.Pop().(values.JavaValue)
		var2 := runtimeStackSimulation.Pop().(values.JavaValue)
		runtimeStackSimulation.Push(values.NewJavaCompare(var2, var1))
	case OP_LSUB, OP_ISUB, OP_DSUB, OP_FSUB, OP_LADD, OP_IADD, OP_FADD, OP_DADD, OP_IREM, OP_FREM, OP_LREM, OP_DREM, OP_IDIV, OP_FDIV, OP_DDIV, OP_LDIV, OP_IMUL, OP_DMUL, OP_FMUL, OP_LMUL, OP_LAND, OP_LOR, OP_LXOR, OP_ISHR, OP_ISHL, OP_LSHL, OP_LSHR, OP_IUSHR, OP_LUSHR, OP_IOR, OP_IAND, OP_IXOR:
		var op string
		switch opcode.Instr.OpCode {
		case OP_LSUB, OP_ISUB, OP_DSUB, OP_FSUB:
			op = SUB
		case OP_LADD, OP_IADD, OP_FADD, OP_DADD:
			op = ADD
		case OP_IREM, OP_FREM, OP_LREM, OP_DREM:
			op = REM
		case OP_IDIV, OP_FDIV, OP_DDIV, OP_LDIV:
			op = DIV
		case OP_IMUL, OP_DMUL, OP_FMUL, OP_LMUL:
			op = MUL
		case OP_LAND, OP_IAND:
			op = AND
		case OP_LOR, OP_IOR:
			op = OR
		case OP_LXOR, OP_IXOR:
			op = XOR
		case OP_ISHR, OP_LSHR:
			op = SHR
		case OP_ISHL, OP_LSHL:
			op = SHL
		case OP_IUSHR, OP_LUSHR:
			op = USHR
		default:
			// Unknown shift opcode: default to shift-left as a safe fallback.
			op = SHL
		}
		var2 := runtimeStackSimulation.Pop().(values.JavaValue)
		var1 := runtimeStackSimulation.Pop().(values.JavaValue)
		runtimeStackSimulation.Push(values.NewBinaryExpression(var1, var2, op, var1.Type()))
	case OP_I2B, OP_I2C, OP_I2D, OP_I2F, OP_I2L, OP_I2S, OP_L2D, OP_L2F, OP_L2I, OP_F2D, OP_F2I, OP_F2L, OP_D2F, OP_D2I, OP_D2L:
		var fname string
		var typ types.JavaType
		switch opcode.Instr.OpCode {
		case OP_I2B:
			fname = TypeCaseByte
			typ = types.NewJavaPrimer(types.JavaByte)
		case OP_I2C:
			fname = TypeCaseChar
			typ = types.NewJavaPrimer(types.JavaChar)
		case OP_I2D:
			fname = TypeCaseDouble
			typ = types.NewJavaPrimer(types.JavaDouble)
		case OP_I2F:
			fname = TypeCaseFloat
			typ = types.NewJavaPrimer(types.JavaFloat)
		case OP_I2L:
			fname = TypeCaseLong
			typ = types.NewJavaPrimer(types.JavaLong)
		case OP_I2S:
			fname = TypeCaseShort
			typ = types.NewJavaPrimer(types.JavaShort)
		case OP_L2D:
			fname = TypeCaseDouble
			typ = types.NewJavaPrimer(types.JavaDouble)
		case OP_L2F:
			fname = TypeCaseFloat
			typ = types.NewJavaPrimer(types.JavaFloat)
		case OP_L2I:
			fname = TypeCaseInt
			typ = types.NewJavaPrimer(types.JavaInteger)
		case OP_F2D:
			fname = TypeCaseDouble
			typ = types.NewJavaPrimer(types.JavaDouble)
		case OP_F2I:
			fname = TypeCaseInt
			typ = types.NewJavaPrimer(types.JavaInteger)
		case OP_F2L:
			fname = TypeCaseLong
			typ = types.NewJavaPrimer(types.JavaLong)
		case OP_D2F:
			fname = TypeCaseFloat
			typ = types.NewJavaPrimer(types.JavaFloat)
		case OP_D2I:
			fname = TypeCaseInt
			typ = types.NewJavaPrimer(types.JavaInteger)
		case OP_D2L:
			fname = TypeCaseLong
			typ = types.NewJavaPrimer(types.JavaLong)
		}
		arg := runtimeStackSimulation.Pop().(values.JavaValue)
		runtimeStackSimulation.Push(values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			// Parenthesize the operand so the (unary) cast keeps the right precedence
			// when the operand is a lower-precedence expression: without it,
			// `(long)a * b` parses as `((long)a) * b` instead of `(long)(a * b)`,
			// causing "possible lossy conversion" recompile failures. The extra parens
			// are always valid Java.
			return fmt.Sprintf("(%s)(%s)", fname, arg.String(funcCtx))
		}, func() types.JavaType {
			return typ
		}))
	case OP_INSTANCEOF:
		classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*values.JavaClassValue).Type()
		value := runtimeStackSimulation.Pop().(values.JavaValue)
		runtimeStackSimulation.Push(values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			return fmt.Sprintf("%s instanceof %s", value.String(funcCtx), classInfo.String(funcCtx))
		}, func() types.JavaType {
			return types.NewJavaPrimer(types.JavaBoolean)
		}))
	case OP_CHECKCAST:
		classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*values.JavaClassValue).Type()
		arg := runtimeStackSimulation.Pop().(values.JavaValue)
		value := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			// Wrap the whole cast in parentheses so it keeps the correct precedence
			// when it becomes the receiver of a member access or method call: without
			// the outer parens, `(T)(x).m()` parses as `(T)(x.m())` instead of
			// `((T)x).m()`. The extra parens are always valid Java in every context.
			return fmt.Sprintf("((%s)(%s))", classInfo.String(funcCtx), arg.String(funcCtx))
		}, func() types.JavaType {
			return classInfo
		})
		ref := runtimeStackSimulation.NewVar(value)
		slotvalue := values.NewSlotValue(ref, ref.Type())
		users := d.varUserMap.GetMust(ref)
		d.varUserMap.Set(ref, append(users, &VarFoldRule{
			Replace: func(v values.JavaValue) {
				slotvalue.ResetValue(v)
			},
			CurrentOpcode:    opcode,
			UserIsNextOpcode: true,
		}))
		runtimeStackSimulation.Push(slotvalue)
	case OP_INVOKESTATIC:
		classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
		funcCallValue := values.NewFunctionCallExpression(nil, classInfo, classInfo.JavaType.FunctionType()) // 不push到栈中
		//funcCallValue.JavaType = classInfo.JavaType
		funcCallValue.Object = values.NewJavaClassValue(types.NewJavaClass(classInfo.Name))
		funcCallValue.IsStatic = true
		for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
			funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(values.JavaValue))
		}
		funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
		if funcCallValue.FuncType.ReturnType.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
			runtimeStackSimulation.Push(funcCallValue)
		}
	case OP_INVOKEDYNAMIC:
		index, name, desc := d.ConstantPoolInvokeDynamicInfo(int(Convert2bytesToInt(opcode.Data)))
		_ = name
		_ = desc
		callSiteReturnType, err := types.ParseMethodDescriptor(desc)
		if err != nil {
			return err
		}
		//callerClassName := callSiteReturnType.String(d.FunctionContext)
		//values.NewJavaClassMember(callerClassName, name, callSiteReturnType)
		//var typ types.JavaType
		args := []values.JavaValue{}
		paramLen := len(callSiteReturnType.FunctionType().ParamTypes)
		for i := 0; i < paramLen; i++ {
			args = append(args, runtimeStackSimulation.Pop())
		}
		refMethod := d.BootstrapMethods[index]
		memberInfo := refMethod.Ref.(*values.JavaClassMember)
		d.InvokeDynamicName = name
		var callResult values.JavaValue
		if f := buildinBootstrapMethods[fmt.Sprintf("%s.%s", memberInfo.Name, memberInfo.Member)]; f != nil {
			callResult, err = f(refMethod.Arguments...)(d, runtimeStackSimulation, callSiteReturnType.FunctionType().ReturnType, args...)
			if err != nil {
				return fmt.Errorf("call bootstrap method error: %v", err)
			}
		} else {
			callResult, err = buildinBootstrapMethods["defaultBootstrapMethod"]()(d, runtimeStackSimulation, callSiteReturnType.FunctionType().ReturnType, args...)
			if err != nil {
				return fmt.Errorf("call bootstrap method error: %v", err)
			}
		}
		if callResult.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
			runtimeStackSimulation.Push(callResult)
		}
	case OP_INVOKESPECIAL:
		classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
		funcCallValue := values.NewFunctionCallExpression(nil, classInfo, classInfo.JavaType.FunctionType()) // 不push到栈中
		for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
			funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(values.JavaValue))
		}
		funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)

		funcCallValue.Object = runtimeStackSimulation.Pop().(values.JavaValue)
		if funcCallValue.FuncType.ReturnType.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
			runtimeStackSimulation.Push(funcCallValue)
		}
	case OP_INVOKEINTERFACE:
		classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
		funcCallValue := values.NewFunctionCallExpression(nil, classInfo, classInfo.JavaType.FunctionType()) // 不push到栈中
		for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
			funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(values.JavaValue))
		}
		funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
		funcCallValue.Object = runtimeStackSimulation.Pop().(values.JavaValue)
		if funcCallValue.FuncType.ReturnType.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
			runtimeStackSimulation.Push(funcCallValue)
		}
	case OP_INVOKEVIRTUAL:
		classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
		funcCallValue := values.NewFunctionCallExpression(nil, classInfo, classInfo.JavaType.FunctionType()) // 不push到栈中
		for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
			funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(values.JavaValue))
		}
		funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
		funcCallValue.Object = runtimeStackSimulation.Pop().(values.JavaValue)
		if funcCallValue.FuncType.ReturnType.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
			runtimeStackSimulation.Push(funcCallValue)
		}
	case OP_RETURN:
	case OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE:
		op := GetNotOp(opcode)
		rv := runtimeStackSimulation.Pop().(values.JavaValue)
		lv := runtimeStackSimulation.Pop().(values.JavaValue)

		statements.NewConditionStatement(values.NewJavaCompare(lv, rv), op)
	case OP_IFNONNULL:
		statements.NewConditionStatement(values.NewJavaCompare(runtimeStackSimulation.Pop().(values.JavaValue), values.JavaNull), EQ)
	case OP_IFNULL:
		statements.NewConditionStatement(values.NewJavaCompare(runtimeStackSimulation.Pop().(values.JavaValue), values.JavaNull), NEQ)
	case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE:
		op := ""
		switch opcode.Instr.OpCode {
		case OP_IFEQ:
			op = "=="
		case OP_IFNE:
			op = "!="
		case OP_IFLE:
			op = "<="
		case OP_IFLT:
			op = "<"
		case OP_IFGT:
			op = ">"
		case OP_IFGE:
			op = ">="
		}
		op = GetReverseOp(op)
		runtimeStackSimulation.Pop()
	case OP_JSR, OP_JSR_W:
		// The JSR inliner should have handled these. If it bailed (e.g. switch + jsr
		// in the same method), treat as no-op rather than failing the entire method.
		// This produces partial output (missing finally body) instead of a full stub.
	case OP_RET:
		// Same as above — no-op if inliner bailed.
	case OP_GOTO, OP_GOTO_W:
	case OP_ATHROW:
		runtimeStackSimulation.Pop()
	case OP_IRETURN:
		v := runtimeStackSimulation.Pop().(values.JavaValue)
		v.Type().ResetType(funcCtx.FunctionType.(*types.JavaFuncType).ReturnType)
		statements.NewReturnStatement(v)
	case OP_ARETURN, OP_LRETURN, OP_DRETURN, OP_FRETURN:
		v := runtimeStackSimulation.Pop().(values.JavaValue)
		v.Type().ResetType(funcCtx.FunctionType.(*types.JavaFuncType).ReturnType)
		statements.NewReturnStatement(v)
	case OP_GETFIELD:
		index := Convert2bytesToInt(opcode.Data)
		member := d.constantPoolGetter(int(index)).(*values.JavaClassMember)
		v := runtimeStackSimulation.Pop().(values.JavaValue)
		runtimeStackSimulation.Push(values.NewRefMember(v, member.Member, member.JavaType))
	case OP_GETSTATIC:
		index := Convert2bytesToInt(opcode.Data)
		runtimeStackSimulation.Push(d.constantPoolGetter(int(index)))
	case OP_PUTSTATIC:
		index := Convert2bytesToInt(opcode.Data)
		staticVal := d.constantPoolGetter(int(index))
		value := runtimeStackSimulation.Pop().(values.JavaValue)
		if selfOp, ok := d.tryFoldPostIncDec(runtimeStackSimulation, value); ok {
			// static field post-increment/decrement reused as a value: drop the bare old
			// value left by dup and push `field++` / `field--` instead.
			runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(selfOp)
			opcode.SelfOpFolded = true
		} else {
			statements.NewAssignStatement(staticVal, value, false)
		}
	case OP_PUTFIELD:
		index := Convert2bytesToInt(opcode.Data)
		staticVal := d.constantPoolGetter(int(index))
		value := runtimeStackSimulation.Pop().(values.JavaValue)
		field := values.NewRefMember(runtimeStackSimulation.Pop().(values.JavaValue), staticVal.(*values.JavaClassMember).Member, staticVal.(*values.JavaClassMember).JavaType)
		if selfOp, ok := d.tryFoldPostIncDec(runtimeStackSimulation, value); ok {
			// instance field post-increment/decrement reused as a value (dup_x1 idiom): drop
			// the bare old value and push `field++` / `field--` so the surrounding ternary or
			// expression branch stays side-effect-free and can be structured.
			runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(selfOp)
			opcode.SelfOpFolded = true
		} else {
			statements.NewAssignStatement(field, value, false)
		}
	case OP_SWAP:
		v1 := runtimeStackSimulation.Pop()
		v2 := runtimeStackSimulation.Pop()
		runtimeStackSimulation.Push(v1)
		runtimeStackSimulation.Push(v2)
	case OP_DUP:
		// Do not ref-fold NewExpression values from 'new; dup; invokespecial' patterns:
		// the invokespecial modifies the NewExpression in-place (ArgumentsGetter), and
		// ref-folding it into a shared temp variable causes both branches of an if/else
		// to share the same variable, corrupting the output. Array creation NewExpressions
		// (which have Length set) DO need ref-folding for array-store patterns.
		peekVal := UnpackSoltValue(runtimeStackSimulation.Peek().(values.JavaValue))
		if newExpr, ok := peekVal.(*values.NewExpression); !ok || len(newExpr.Length) > 0 {
			checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))(1)
		}
		runtimeStackSimulation.Push(runtimeStackSimulation.Peek())
	case OP_DUP_X1:
		checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))(3)
		v1 := runtimeStackSimulation.Pop()
		v2 := runtimeStackSimulation.Pop()
		runtimeStackSimulation.Push(v1)
		runtimeStackSimulation.Push(v2)
		runtimeStackSimulation.Push(v1)
	case OP_DUP_X2:
		adduser := checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
		v1 := runtimeStackSimulation.Pop()
		runtimeStackSimulationPopN := func(n int) []values.JavaValue {
			datas := []values.JavaValue{}
			current := 0
			for {
				if current >= n {
					break
				}
				checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
				v := runtimeStackSimulation.Pop()
				current += GetTypeSize(v.(values.JavaValue).Type())
				datas = append(datas, v)
			}
			return datas
		}
		datas := runtimeStackSimulationPopN(2)
		runtimeStackSimulation.Push(v1)
		for i := len(datas) - 1; i >= 0; i-- {
			runtimeStackSimulation.Push(datas[i])
			adduser(1)
		}
		runtimeStackSimulation.Push(v1)
		adduser(1)
	case OP_DUP2:
		// dup2 duplicates the top two category-1 slots (or one category-2 value). Each duplicated
		// value must carry its OWN ref-fold callback. The previous code kept a single shared addUser
		// (overwritten to the LAST converted value), so when both pushes reused it, the deeper value's
		// fold rule fired on the shallower value too. For a compound array store on a field array
		// (`this.f[i] op= v`, bytecode getfield;iload;dup2;iaload;...;iastore) that folded the index
		// into the arrayref temp, emitting `int t = i; t[i] = t[i] op v` (an int indexed as an array).
		// Tracking addUser per element keeps each duplicated value's fold rule bound to that value.
		type dup2Item struct {
			val     values.JavaValue
			addUser func(int)
		}
		popItems := func(n int) []dup2Item {
			items := []dup2Item{}
			current := 0
			for current < n {
				au := checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
				v := runtimeStackSimulation.Pop()
				current += GetTypeSize(v.(values.JavaValue).Type())
				items = append(items, dup2Item{val: v, addUser: au})
			}
			return items
		}
		pushReverse := func(items []dup2Item) {
			for i := len(items) - 1; i >= 0; i-- {
				runtimeStackSimulation.Push(items[i].val)
				items[i].addUser(1)
			}
		}
		items := popItems(2)
		pushReverse(items)
		pushReverse(items)
	case OP_DUP2_X1:
		var addUser func(int)
		runtimeStackSimulationPopN := func(n int) []values.JavaValue {
			datas := []values.JavaValue{}
			current := 0
			for {
				if current >= n {
					break
				}
				addUser = checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
				v := runtimeStackSimulation.Pop()
				current += GetTypeSize(v.(values.JavaValue).Type())
				datas = append(datas, v)
			}
			return datas
		}
		runtimeStackSimulationPushReverse := func(datas []values.JavaValue) {
			for i := len(datas) - 1; i >= 0; i-- {
				runtimeStackSimulation.Push(datas[i])
				addUser(1)
			}
		}
		datas := runtimeStackSimulationPopN(2)
		v1 := runtimeStackSimulation.Pop()
		runtimeStackSimulationPushReverse(datas)
		runtimeStackSimulation.Push(v1)
		addUser(1)
		runtimeStackSimulationPushReverse(datas)
		//checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
		//v1 := runtimeStackSimulation.Pop()
		//checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
		//v2 := runtimeStackSimulation.Pop()
		//checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
		//v3 := runtimeStackSimulation.Pop()
		//runtimeStackSimulation.Push(v2)
		//runtimeStackSimulation.Push(v1)
		//runtimeStackSimulation.Push(v3)
		//runtimeStackSimulation.Push(v2)
		//runtimeStackSimulation.Push(v1)
	case OP_DUP2_X2:
		var addUser func(int2 int)
		runtimeStackSimulationPopN := func(n int) []values.JavaValue {
			datas := []values.JavaValue{}
			current := 0
			for {
				if current >= n {
					break
				}
				addUser = checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
				v := runtimeStackSimulation.Pop()
				current += GetTypeSize(v.(values.JavaValue).Type())
				datas = append(datas, v)
			}
			return datas
		}
		runtimeStackSimulationPushReverse := func(datas []values.JavaValue) {
			for i := len(datas) - 1; i >= 0; i-- {
				runtimeStackSimulation.Push(datas[i])
				addUser(1)
			}
		}
		datas1 := runtimeStackSimulationPopN(2)
		datas2 := runtimeStackSimulationPopN(2)
		runtimeStackSimulationPushReverse(datas1)
		runtimeStackSimulationPushReverse(datas2)
		runtimeStackSimulationPushReverse(datas1)
	case OP_LDC:
		runtimeStackSimulation.Push(d.ConstantPoolLiteralGetter(int(opcode.Data[0])))
	case OP_LDC_W:
		runtimeStackSimulation.Push(d.ConstantPoolLiteralGetter(int(Convert2bytesToInt(opcode.Data))))
	case OP_LDC2_W:
		v := d.ConstantPoolLiteralGetter(int(Convert2bytesToInt(opcode.Data)))
		runtimeStackSimulation.Push(v)
	case OP_MONITORENTER:
		runtimeStackSimulation.Pop()
	case OP_MONITOREXIT:
		runtimeStackSimulation.Pop()
	case OP_NOP:
		return nil
	case OP_POP:
		statements.NewExpressionStatement(runtimeStackSimulation.Pop().(values.JavaValue))
	case OP_POP2:
		val := runtimeStackSimulation.Peek()
		if GetTypeSize(val.Type()) == 1 {
			runtimeStackSimulation.PopN(2)
		} else {
			runtimeStackSimulation.Pop()
		}
	case OP_TABLESWITCH, OP_LOOKUPSWITCH:
		statements.NewMiddleStatement(statements.MiddleSwitch, []any{opcode.SwitchJmpCase1, runtimeStackSimulation.Pop().(values.JavaValue)})
	case OP_IINC:
		var index int
		if opcode.IsWide {
			index = int(Convert2bytesToInt(opcode.Data))
		} else {
			index = int(opcode.Data[0])
		}
		ref := runtimeStackSimulation.GetVar(index)
		opcode.Ref = ref
	case OP_DNEG, OP_FNEG, OP_LNEG, OP_INEG:
		v := runtimeStackSimulation.Pop().(values.JavaValue)
		runtimeStackSimulation.Push(values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			return fmt.Sprintf("-%s", v.String(funcCtx))
		}, func() types.JavaType {
			return v.Type()
		}))
	case OP_END:
	case OP_START:
	default:
		return fmt.Errorf("not support opcode: %x", opcode.Instr.OpCode)
	}
	return nil
}
// EnableLegacyMergeReconstruction selects the original heuristic if-merge / ternary value
// reconstruction (the dual structural-probe + chain combiner). When false (default), value merges
// whose conditions converge on shared leaf values (short-circuit &&/|| predicates) are rebuilt by a
// principled recursive ternary-tree builder that allows shared leaves, so the operand-stack value
// at the merge is always reconstructed instead of leaking an "empty slot value" placeholder. The
// flag exists so the old path can be restored for A/B comparison if a regression surfaces.
var EnableLegacyMergeReconstruction = false

func (d *Decompiler) CalcOpcodeStackInfo() error {
	// Pre-sized to the opcode count: this map gets one entry per opcode, so sizing it up
	// front avoids the incremental rehash-growth garbage (it was a top per-opcode allocator).
	opcodeToSim := make(map[*OpCode]*StackSimulationImpl, len(d.opCodes))
	//dominatorMap := GenerateDominatorTree(d.RootOpCode)
	//codes := GraphToList(d.RootOpCode)
	//codes = lo.Filter(codes, func(code *OpCode, index int) bool {
	//	switch code.Instr.OpCode {
	//	case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
	//		return true
	//	default:
	//		return false
	//	}
	//})
	mergeToIfNode := map[*OpCode][]*OpCode{}
	//for _, ifNode := range codes {
	//	if len(dominatorMap[ifNode]) == 3 {
	//		mergeNode := funk.Filter(dominatorMap[ifNode], func(code *OpCode) bool {
	//			return code != ifNode.Target[0] && code != ifNode.Target[1]
	//		}).([]*OpCode)[0]
	//		mergeToIfNode[mergeNode] = append(mergeToIfNode[mergeNode], ifNode)
	//	}
	//}
	//
	//isIfMergeNode := func(code *OpCode) bool {
	//	_, ok := mergeToIfNode[code]
	//	return ok
	//}
	//getIfMergeNodeCondition := func(code *OpCode) values.JavaValue {
	//	ifs := mergeToIfNode[code]
	//	if len(ifs) > 0 {
	//		ifNode := ifs[0]
	//		ifs = ifs[1:]
	//		return ifNode.stackConsumed[0]
	//	}
	//	return nil
	//}
	ternaryExpMergeNode := []*OpCode{}
	ternaryExpMergeNodeSlot := map[*OpCode]*values.SlotValue{}
	if !d.FunctionContext.IsStatic {
		d.FunctionType.ParamTypes = append([]types.JavaType{types.NewJavaClass(d.FunctionContext.ClassName)}, d.FunctionType.ParamTypes...)
	}

	initMethodVar := func(runtimeSim StackSimulation) {
		params := []values.JavaValue{}
		slotIndex := 0
		for _, paramType := range d.FunctionType.ParamTypes {
			//assignStackVar(values.NewJavaRef(stackVarIndex, paramType))
			var isDouble bool
			if v, ok := paramType.RawType().(*types.JavaPrimer); ok {
				isDouble = v.Name == types.JavaDouble || v.Name == types.JavaLong
			}
			paramPlaceholder := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return ""
			}, func() types.JavaType {
				return paramType
			})
			// Tag the seed value so GetRealValue stops at the parameter ref (rendered by name)
			// instead of unwrapping into this empty placeholder. Without the tag, folding a temp
			// that copies a parameter (`var2 = param; this.f = var2;`) inlined the empty string
			// and produced `this.f = ;` (invalid Java -> stub).
			paramPlaceholder.Flag = "param_placeholder"
			runtimeSim.AssignVar(slotIndex, paramPlaceholder)
			val := runtimeSim.GetVar(slotIndex)
			params = append(params, val)
			if isDouble {
				slotIndex += 2
			} else {
				slotIndex += 1
			}
		}
		d.Params = params
		if !d.FunctionContext.IsStatic {
			runtimeSim.GetVar(0).IsThis = true
		}
		d.BodyStartId = len(params)
	}
	isIfNode := func(code *OpCode) bool {
		switch code.Instr.OpCode {
		case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
			return true
		default:
			return false
		}
	}
	opcodes := GraphToList(d.RootOpCode)
	ifOpcodes := lo.Filter(opcodes, func(code *OpCode, index int) bool {
		return isIfNode(code)
	})

	ifNodeToMergeNode := map[*OpCode]*OpCode{}
	mergeNodeToIfNode := map[*OpCode][]*OpCode{}
	//DumpOpcodesToDotExp(d.RootOpCode)
	for _, opcode := range ifOpcodes {
		mergeNode := CalcMergeOpcode(opcode)
		if mergeNode != nil {
			ifNodeToMergeNode[opcode] = mergeNode
			mergeNodeToIfNode[mergeNode] = append(mergeNodeToIfNode[mergeNode], opcode)
		}
	}
	// One scope entry per opcode; pre-size to avoid rehash-growth garbage (see opcodeToSim).
	nodeToVarScope := make(map[*OpCode]*Scope, len(d.opCodes))
	getVarScope := func(code *OpCode) *Scope {
		if vt, ok := nodeToVarScope[code]; ok {
			return vt
		}
		// Fallback: create a new scope with an empty var table and root var id.
		// This avoids panicking on complex CFG paths where the scope wasn't set.
		vt := &Scope{VarTable: map[int]*values.JavaRef{}, VarId: utils2.NewRootVariableId()}
		nodeToVarScope[code] = vt
		return vt
	}
	setVarScope := func(code *OpCode, scope *Scope) {
		nodeToVarScope[code] = scope
	}
	ifNodeToConditionCallback := map[*OpCode]func(values.JavaValue){}
	varTable := map[int]*values.JavaRef{}
	err := WalkGraph[*OpCode](d.RootOpCode, func(code *OpCode) ([]*OpCode, error) {
		// NOTE: do not sort code.Target for switch opcodes. The previous sort.Slice used an
		// invalid comparator (always returning true), which scrambled the case successor order
		// (which must stay aligned with SwitchJmpCase1's case-value -> index mapping). That made
		// every case map to the wrong body. Target is already in the correct case-index order.
		var runtimeStackSimulation *StackSimulationImpl
		if len(code.Source) == 0 {
			if code.Instr.OpCode == OP_START {
				emptySim := NewEmptyStackEntry()
				varId := d.BaseVarId
				if varId == nil {
					varId = utils2.NewRootVariableId()
				}
				runtimeStackSimulation = NewStackSimulation(emptySim, varTable, varId)
				initMethodVar(runtimeStackSimulation)
			} else {
				return nil, fmt.Errorf("opcode %d has no source", code.Id)
			}
		} else if len(code.Source) == 1 {
			if IsSwitchOpcode(code.Source[0].Instr.OpCode) {
				// A switch's case/default targets must inherit the operand-stack state that held
				// AFTER the switch instruction consumed its selector. The selector Pop happens in
				// calcOpcodeStackInfo, so code.Source[0].StackEntry is exactly that post-switch
				// stack. Rebuilding from it (instead of a shared preRuntimeStackSimulation) is
				// correct for EVERY branch independently: previously a single shared variable was
				// clobbered as soon as an earlier branch ended (e.g. an athrow/return that left an
				// empty stack), so later case bodies started with a stale/empty operand stack and
				// underflowed (the Groovy selectConstructorAndTransformArguments switch, whose
				// first case builds args via dup_x1/dup2_x1 on top of [objarr,newexpr], leaked
				// empty-slot placeholders). Falling back to an empty entry keeps the panic-free
				// contract if the switch's own stack entry was never set.
				entry := code.Source[0].StackEntry
				if entry == nil {
					entry = NewEmptyStackEntry()
				}
				scope := getVarScope(code.Source[0])
				runtimeStackSimulation = NewStackSimulation(entry, scope.VarTable, scope.VarId)
			} else {
				entry := code.Source[0].StackEntry
				if entry == nil {
					// The source opcode's stack entry is nil (e.g. from an unvisited
					// predecessor in a complex CFG). Use an empty stack to continue
					// rather than failing the entire method.
					entry = NewEmptyStackEntry()
				}
				scope := getVarScope(code.Source[0])
				runtimeStackSimulation = NewStackSimulation(entry, scope.VarTable, scope.VarId)
			}
		} else if len(code.Source) > 0 {
			//ifNodes := mergeNodeToIfNode[code]
			//for _, opCode := range code.Source {
			//	WalkGraph[*OpCode](opCode, func(code *OpCode) ([]*OpCode, error) {
			//		switch code.Instr.OpCode {
			//		case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
			//			if !slices.Contains(ifNodes, code) {
			//				ifNodes = append(ifNodes, code)
			//			}
			//			return nil, nil
			//		default:
			//		}
			//		return code.Source, nil
			//	})
			//}
			//ifNodes = lo.Filter(ifNodes, func(item *OpCode, index int) bool {
			//	return item.StackEntry != nil
			//})
			sources := lo.Filter(code.Source, func(item *OpCode, index int) bool {
				return item.StackEntry != nil
			})
			validSources := []*OpCode{}
			for _, source := range sources {
					entry := source.StackEntry
					if entry == nil {
						entry = NewEmptyStackEntry()
					}
				validSources = append(validSources, source)
			}
			//sourceIfCodes := []*OpCode{}
			//for _, opCode := range code.Source {
			//	WalkGraph[*OpCode](opCode, func(code *OpCode) ([]*OpCode, error) {
			//		switch code.Instr.OpCode {
			//		case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
			//			if !slices.Contains(sourceIfCodes, code) {
			//				sourceIfCodes = append(sourceIfCodes, code)
			//			}
			//			return nil, nil
			//		default:
			//		}
			//		return code.Source, nil
			//	})
			//}
			//sourceIfCodes = lo.Filter(sourceIfCodes, func(item *OpCode, index int) bool {
			//	return slices.Contains(ifNodes, item)
			//})
			//ifStackEntries := []*StackItem{}
			//for _, ifNode := range sourceIfCodes {
			//	entry := ifNode.StackEntry
			//	if entry == nil {
			//		return nil, fmt.Errorf("not found simuation stack for opcode %d", ifNode.Id)
			//	}
			//	ifStackEntries = append(stackEntries, entry)
			//}

			size := -1
			for _, vs := range validSources {
				vsScope := getVarScope(vs)
				vsSize := NewStackSimulation(vs.StackEntry, vsScope.VarTable, vsScope.VarId).Size()
				if size == -1 {
					size = vsSize
				}
			}
			if len(validSources) == 0 {
				runtimeStackSimulation = NewStackSimulation(NewEmptyStackEntry(), varTable, utils2.NewRootVariableId())
			} else {
				validSource := validSources[0]
				vscope := getVarScope(validSource)
				runtimeStackSimulation = NewStackSimulation(validSource.StackEntry, vscope.VarTable, vscope.VarId)
			}
			ifNodes := []*OpCode{}
			for _, opCode := range code.Source {
				WalkGraph[*OpCode](opCode, func(code *OpCode) ([]*OpCode, error) {
					switch code.Instr.OpCode {
					case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
						if !slices.Contains(ifNodes, code) {
							ifNodes = append(ifNodes, code)
						}
						return nil, nil
					default:
					}
					return code.Source, nil
				})
			}
			ifNodes = lo.Filter(ifNodes, func(item *OpCode, index int) bool {
				return slices.Contains(mergeNodeToIfNode[code], item)
			})
			var isIfMergeNode bool
			if len(ifNodes) > 0 {
				ifSize := -1
				for _, ifNode := range ifNodes {
					if ifNode.StackEntry == nil {
						continue
					}
					scope := getVarScope(ifNode)
					stackSize := NewStackSimulation(ifNode.StackEntry, scope.VarTable, scope.VarId).Size()
					if ifSize == -1 {
						ifSize = stackSize
					} else {
						if ifSize != stackSize {
							return nil, fmt.Errorf("invalid stack size %d for opcode %d", stackSize, code.Id)
						}
					}
				}
				if ifSize != -1 {
					isIfMergeNode = ifSize < size
				}
			}
			if isIfMergeNode {
				validSource := validSources[0]
				scope := getVarScope(validSource)
				preSim := NewStackSimulation(validSource.StackEntry, scope.VarTable, scope.VarId)
				preSim.Pop()
				runtimeStackSimulation = NewStackSimulation(preSim.stackEntry, scope.VarTable, scope.VarId)
				slotVal := values.NewSlotValue(nil, validSource.StackEntry.value.Type())
				runtimeStackSimulation.Push(slotVal)
				ternaryExpMergeNodeSlot[code] = slotVal
				ternaryExpMergeNode = append(ternaryExpMergeNode, code)
				mergeToIfNode[code] = append(mergeToIfNode[code], ifNodes...)
			} else {
				validSource := validSources[0]
				scope := getVarScope(validSource)
				runtimeStackSimulation = NewStackSimulation(validSource.StackEntry, scope.VarTable, scope.VarId)
			}
		} else {
			for _, opCode := range code.Source {
				if opCode.StackEntry != nil {
					scope := getVarScope(opCode)
					runtimeStackSimulation = NewStackSimulation(opCode.StackEntry, scope.VarTable, scope.VarId)
					break
				}
			}
		}
		opcodeToSim[code] = runtimeStackSimulation

		sim := NewStackSimulationProxy(runtimeStackSimulation, func(value values.JavaValue) {
			runtimeStackSimulation.Push(value)
			code.stackProduced = append(code.stackProduced, value)
		}, func() values.JavaValue {
			val := runtimeStackSimulation.Pop()
			code.stackConsumed = append(code.stackConsumed, val)
			return val
		})
		if code.IsCatch {
			var typ types.JavaType
			// Reconstruct a multi-catch clause when several catch types share this handler.
			multiTypes := make([]types.JavaType, 0, len(code.ExceptionTypeIndexes))
			for _, idx := range code.ExceptionTypeIndexes {
				if idx != 0 {
					multiTypes = append(multiTypes, d.GetValueFromPool(int(idx)).Type())
				}
			}
			switch {
			case len(multiTypes) > 1:
				typ = types.NewMultiCatchType(multiTypes)
			case len(multiTypes) == 1:
				typ = multiTypes[0]
			case code.ExceptionTypeIndex != 0:
				typ = d.GetValueFromPool(int(code.ExceptionTypeIndex)).Type()
			default:
				typ = types.NewJavaClass("Throwable")
			}
			exceptionValue := values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return "Exception"
			}, func() types.JavaType {
				return typ
			})
			exceptionValue.Flag = "exception"
			runtimeStackSimulation.Push(exceptionValue)
		}
		runtimeStackSimulation.varTable = varTable
		err := d.calcOpcodeStackInfo(sim, code)
		if err != nil {
			return nil, err
		}
		code.StackEntry = runtimeStackSimulation.stackEntry
		scope := NewScope()
		scope.VarId = runtimeStackSimulation.currentVarId
		scope.VarTable = varTable
		setVarScope(code, scope)
		return code.Target, nil
	})
	if err != nil {
		return err
	}
	d.opCodes = GraphToList(d.RootOpCode)
	d.opcodeToSimulateStack = opcodeToSim
	ternaryExpMergeNode = utils.NewSet(ternaryExpMergeNode).List()
	sort.Slice(ternaryExpMergeNode, func(i, j int) bool {
		return ternaryExpMergeNode[i].Id > ternaryExpMergeNode[j].Id
	})
	d.ifNodeConditionCallback = ifNodeToConditionCallback
	// isTernaryArmStore reports whether an opcode writes a local/field/array element. A store on an
	// arm path means the "merge" is statement dispatch (e.g. a lexer assigning a token in each branch
	// then converging), not a value ternary, so the principled builder declines it.
	isTernaryArmStore := func(op int) bool {
		switch op {
		case OP_ISTORE, OP_ASTORE, OP_LSTORE, OP_DSTORE, OP_FSTORE,
			OP_ISTORE_0, OP_ASTORE_0, OP_LSTORE_0, OP_DSTORE_0, OP_FSTORE_0,
			OP_ISTORE_1, OP_ASTORE_1, OP_LSTORE_1, OP_DSTORE_1, OP_FSTORE_1,
			OP_ISTORE_2, OP_ASTORE_2, OP_LSTORE_2, OP_DSTORE_2, OP_FSTORE_2,
			OP_ISTORE_3, OP_ASTORE_3, OP_LSTORE_3, OP_DSTORE_3, OP_FSTORE_3,
			OP_IINC, OP_PUTFIELD, OP_PUTSTATIC,
			OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
			return true
		default:
			return false
		}
	}
	// buildSharedLeafTernary rebuilds the value left on the operand stack at mergeNode as a nested
	// ternary tree. It is the principled replacement for the legacy chain combiner on short-circuit
	// shapes: each conditional arm is walked straight-line; an if-node whose BOTH branches converge on
	// mergeNode becomes a nested ternary condition (discovered dynamically, so chains the bottom-up
	// merge detection under-reports are still picked up), and a node flowing into mergeNode is a leaf
	// whose produced stack value is the arm value. Unlike the legacy structural probe it ALLOWS shared
	// leaves (the canonical short-circuit shape where many conditions converge on the same iconst_0 /
	// iconst_1) - a ternary only evaluates its chosen arm, so textually reusing the shared leaf object
	// is semantically exact.
	//
	// detectedIfNodes seeds the root search. Returns (root, builtConditions, sharedLeaf, ok). ok=false
	// means the shape is irreducible (a store on an arm, a non-conditional fork, a cycle, or an
	// unresolved leaf) and the caller falls back to the legacy path unchanged. sharedLeaf=false means
	// it is a plain tree the legacy probe already handles, so the caller also defers to avoid churn.
	buildSharedLeafTernary := func(mergeNode *OpCode, detectedIfNodes []*OpCode) (root *values.TernaryExpression, built map[*OpCode]*values.TernaryExpression, sharedLeaf bool, hasMiddleCond bool, ok bool) {
		// valueMergeSet is every node the merge detection registered as carrying a value across control
		// flow (a ternary / short-circuit result on the operand stack). It is the principled signal for
		// an INNER value computation: if two branches of a condition reconverge on such a node (other
		// than our own mergeNode), that condition produces a sub-value consumed downstream and is opaque
		// to this merge. A plain control reconvergence (e.g. the next range check in an ||-chain) is NOT
		// in this set, so the condition is correctly recognised as a real arm of this ternary.
		valueMergeSet := map[*OpCode]bool{}
		for _, m := range ternaryExpMergeNode {
			valueMergeSet[m] = true
		}
		// canReachMerge reports whether EVERY forward path from node (under the straight-line / nested-
		// condition discipline, no stores) reaches mergeNode. It identifies value-ternary conditions:
		// a fork qualifies only if both its arms post-converge on mergeNode. Memoized; a node currently
		// on the DFS stack (a cycle / loop back-edge) is treated as not-reaching, so loops are declined.
		const (
			reachUnknown = iota
			reachTrue
			reachFalse
			reachVisiting
		)
		reachMemo := map[*OpCode]int{}
		var canReachMerge func(n *OpCode) bool
		canReachMerge = func(n *OpCode) bool {
			cur := n
			for step := 0; cur != nil && step < (1 << 16); step++ {
				if cur == mergeNode || slices.Contains(cur.Target, mergeNode) {
					return true
				}
				switch reachMemo[cur] {
				case reachTrue:
					return true
				case reachFalse, reachVisiting:
					return false
				}
				if isTernaryArmStore(cur.Instr.OpCode) {
					reachMemo[cur] = reachFalse
					return false
				}
				if isIfNode(cur) && len(cur.Target) >= 2 {
					reachMemo[cur] = reachVisiting
					res := canReachMerge(cur.Target[0]) && canReachMerge(cur.Target[1])
					if res {
						reachMemo[cur] = reachTrue
					} else {
						reachMemo[cur] = reachFalse
					}
					return res
				}
				if len(cur.Target) != 1 {
					reachMemo[cur] = reachFalse
					return false
				}
				cur = cur.Target[0]
			}
			return false
		}
		// bfsDist returns BFS hop distances from start to every forward-reachable node (start excluded
		// from the result unless it is on a cycle). Used to find the NEAREST common reconvergence of a
		// condition's two branches by minimising the summed distance, which is robust to target ordering
		// (a plain reachability probe can return a farther shared node first when a branch forks).
		bfsDist := func(start *OpCode) map[*OpCode]int {
			dist := map[*OpCode]int{}
			queue := []*OpCode{start}
			d := map[*OpCode]int{start: 0}
			for i := 0; i < len(queue) && i < (1<<16); i++ {
				n := queue[i]
				cd := d[n]
				if _, ok := dist[n]; !ok {
					dist[n] = cd
				}
				for _, t := range n.Target {
					if _, seen := d[t]; !seen {
						d[t] = cd + 1
						queue = append(queue, t)
					}
				}
			}
			return dist
		}
		// firstReconverge finds the NEAREST node reachable from BOTH of c's branches (the common node
		// minimising branch0-distance + branch1-distance). It distinguishes two shapes:
		//   - a real condition of THIS value-merge: its branches stay disjoint until mergeNode, or one
		//     branch flows into the other (short-circuit &&/|| chain), so the reconvergence is mergeNode,
		//     one of c's targets, or a plain control node (the next chained condition / range check);
		//   - an inner value ternary (a diamond): both branches meet at a value-merge node whose result
		//     is then consumed (e.g. `!x` feeding an ixor). isInnerValueTernary keys off that.
		firstReconvMemo := map[*OpCode]*OpCode{}
		firstReconverge := func(c *OpCode) *OpCode {
			if len(c.Target) < 2 {
				return nil
			}
			if m, found := firstReconvMemo[c]; found {
				return m
			}
			d0 := bfsDist(c.Target[0])
			d1 := bfsDist(c.Target[1])
			var res *OpCode
			best := 1 << 30
			for n, a := range d0 {
				if n == c {
					continue
				}
				if b, ok := d1[n]; ok {
					if a+b < best {
						best = a + b
						res = n
					}
				}
			}
			firstReconvMemo[c] = res
			return res
		}
		// isInnerValueTernary reports a diamond whose merged VALUE is consumed before mergeNode: the two
		// branches reconverge on a registered value-merge node other than our own mergeNode. A control
		// reconvergence (next ||-chain condition / range check, not a value merge) is NOT inner, so the
		// short-circuit condition is kept as a genuine arm of this ternary.
		isInnerValueTernary := func(n *OpCode) bool {
			if !isIfNode(n) || len(n.Target) < 2 {
				return false
			}
			fr := firstReconverge(n)
			if fr == nil || fr == mergeNode {
				return false
			}
			return valueMergeSet[fr]
		}
		isTernaryCondition := func(n *OpCode) bool {
			return isIfNode(n) && len(n.Target) >= 2 && !isInnerValueTernary(n) &&
				canReachMerge(n.Target[0]) && canReachMerge(n.Target[1])
		}

		built = map[*OpCode]*values.TernaryExpression{}
		usedLeaf := map[*OpCode]bool{}
		failed := false
		var arm func(entry *OpCode) values.JavaValue
		var probe func(ifNode *OpCode) *values.TernaryExpression
		arm = func(entry *OpCode) values.JavaValue {
			cur := entry
			for step := 0; cur != nil && step < (1 << 16); step++ {
				if failed {
					return nil
				}
				// A node flowing straight into mergeNode is a leaf; its produced stack value is the
				// arm value. Checked first so a leaf that also happens to be a store target is still
				// taken as the (already side-effect-accounted) stack value.
				if slices.Contains(cur.Target, mergeNode) {
					if cur.StackEntry == nil {
						failed = true
						return nil
					}
					if usedLeaf[cur] {
						// A ternary only evaluates its chosen arm, so textually reusing a shared leaf is
						// semantically exact - BUT a ternary tree cannot share nodes, so a shared leaf is
						// duplicated once per arm that reaches it at render time. That is harmless for the
						// canonical short-circuit shape (the shared leaf is a single iconst_0 / iconst_1
						// literal), but a shared leaf that is a large value subtree (e.g. a method-call
						// fall-through in a giant instanceof type-dispatch) would expand combinatorially
						// into megabytes of duplicated source. Only adopt literal shared leaves; decline
						// any non-literal shared leaf so the legacy path (which keeps it as control flow)
						// handles it unchanged.
						if _, isLit := UnpackSoltValue(cur.StackEntry.value).(*values.JavaLiteral); !isLit {
							failed = true
							return nil
						}
						sharedLeaf = true
					}
					usedLeaf[cur] = true
					return cur.StackEntry.value
				}
				if isTernaryCondition(cur) {
					return probe(cur)
				}
				// An inner value ternary (diamond) computes a value that is consumed downstream (it is
				// reconstructed by its OWN merge pass); skip the whole sub-region to its reconvergence
				// point and keep walking toward this merge's conditions/leaves.
				if isInnerValueTernary(cur) {
					cur = firstReconverge(cur)
					continue
				}
				if isTernaryArmStore(cur.Instr.OpCode) {
					failed = true
					return nil
				}
				if len(cur.Target) != 1 {
					failed = true
					return nil
				}
				cur = cur.Target[0]
			}
			failed = true
			return nil
		}
		probe = func(ifNode *OpCode) *values.TernaryExpression {
			if t, found := built[ifNode]; found {
				return t // condition reached twice: reuse the same sub-expression (still a correct value)
			}
			if len(ifNode.Target) < 2 {
				failed = true
				return nil
			}
			t := values.NewTernaryExpression(values.NewSlotValue(nil, types.NewJavaPrimer(types.JavaBoolean)), nil, nil)
			built[ifNode] = t
			t.TrueValue = arm(ifNode.Target[1])
			t.FalseValue = arm(ifNode.Target[0])
			// A "middle" condition has BOTH arms leading to further conditions (no direct leaf arm).
			// The legacy chain combiner only attaches a callback to conditions with a direct leaf route,
			// so a middle condition that becomes the MergeIf survivor leaks an empty slot. Its presence
			// is exactly the signal that the legacy path fails and the principled rebuild is needed.
			if _, t1 := t.TrueValue.(*values.TernaryExpression); t1 {
				if _, t0 := t.FalseValue.(*values.TernaryExpression); t0 {
					hasMiddleCond = true
				}
			}
			return t
		}
		// nearestIfAncestor walks back through single-source straight-line predecessors to the closest
		// dominating condition (nil if the chain forks / merges before reaching one).
		nearestIfAncestor := func(n *OpCode) *OpCode {
			cur := n
			for step := 0; cur != nil && step < (1 << 16); step++ {
				if len(cur.Source) != 1 {
					return nil
				}
				p := cur.Source[0]
				if isIfNode(p) {
					return p
				}
				cur = p
			}
			return nil
		}
		// Seed the root with the lowest-id detected condition, then climb to the outermost enclosing
		// ternary condition (both arms still converge on mergeNode). probe(root) then discovers the
		// entire condition set top-down, including chain links the bottom-up detection missed.
		var rootNode *OpCode
		for _, n := range detectedIfNodes {
			if rootNode == nil || n.Id < rootNode.Id {
				rootNode = n
			}
		}
		if rootNode == nil || !isTernaryCondition(rootNode) {
			return nil, nil, false, false, false
		}
		for {
			anc := nearestIfAncestor(rootNode)
			if anc == nil || !isTernaryCondition(anc) {
				break
			}
			rootNode = anc
		}
		root = probe(rootNode)
		if failed || root == nil {
			return nil, nil, false, false, false
		}
		return root, built, sharedLeaf, hasMiddleCond, true
	}
	for _, code := range ternaryExpMergeNode {
		mergeNode := code
		ifNodes := mergeToIfNode[code]
		if len(ifNodes) == 0 {
			continue
		}
		if !EnableLegacyMergeReconstruction {
			rootTern, built, sharedLeaf, hasMiddleCond, ok := buildSharedLeafTernary(mergeNode, ifNodes)
			if os.Getenv("DEBUG_TERNARY") != "" {
				log.Errorf("TERNARY merge=%d ifNodes=%d ok=%v sharedLeaf=%v middle=%v built=%d", mergeNode.Id, len(ifNodes), ok, sharedLeaf, hasMiddleCond, len(built))
			}
			// Intercept every shared-leaf short-circuit/ternary the principled builder can fully rebuild.
			// The legacy chain combiner only attaches a condition callback to if-nodes with a direct leaf
			// arm; any chain whose detection under-reports an interior condition leaks an empty slot
			// (CronPattern.match etc.). The rebuilt nested tree wires a callback to EVERY adopted
			// condition, and TernaryExpression.String folds the shared-leaf shape back into idiomatic
			// &&/|| at render time, so this is both more complete and equally readable.
			if ok && sharedLeaf {
				// Wire every condition: its statement's Callback fills its own nested ternary's
				// Condition (post-MergeIf). Marking TernaryChainArm keeps MergeIf from folding the
				// condition NODES (which would unfire some callbacks and leak), so each condition is
				// dissolved individually into its ternary arm. The &&/|| rendering is recovered purely
				// at the value level by TernaryExpression.String's short-circuit fold.
				for ifNode, t := range built {
					tt := t
					t.ConditionFromOp = ifNode.Id
					ifNode.TernaryChainArm = true
					ifNodeToConditionCallback[ifNode] = func(value values.JavaValue) {
						tt.Condition = value
					}
				}
				ternaryExpMergeNodeSlot[code].ResetValue(rootTern)
				code.conditionOpId = 0
				continue
			}
		}
		// A conditional (?:) converges all of its arms at a single mergeNode; the if-nodes feeding it
		// form a binary tree (outer condition = root, a ternary nested in an arm = subtree). The legacy
		// combiner below walks them as a right-leaning chain and also reconstructs short-circuit &&/||
		// (which compile to a DAG that shares a boolean value node), and it handles those correctly.
		// It only breaks when BOTH arms are nested ternaries (a balanced tree c?(a?:):(b?:)): the
		// bottom-up merge detection records only the if-nodes nearest the leaves, so the outer
		// condition - which has no direct leaf arm - is missing, and an arm is silently dropped. We
		// detect exactly that case by trying to adopt missing dominating conditions (the expansion
		// below); if it grows the if-set we rebuild the tree structurally, otherwise we keep the proven
		// legacy path so short-circuit and simple/chain ternaries are unaffected.
		ifSet := map[*OpCode]bool{}
		for _, n := range ifNodes {
			ifSet[n] = true
		}
		// branchReachesNestedIf reports whether the straight-line path from entry reaches an existing
		// condition node of THIS merge (a nested ternary) before anything else. We deliberately do NOT
		// accept a branch that flows directly into mergeNode (a plain leaf value): adopting an ancestor
		// is only correct for the balanced BOTH-arms shape c?(a?:):(b?:), where each arm is itself a
		// nested ternary. Accepting a leaf arm would also match ordinary if/else dispatch (e.g. a lexer
		// nextToken) whose branches merely converge, and committing that as a ternary corrupts the CFG.
		branchReachesNestedIf := func(entry *OpCode) bool {
			cur := entry
			for step := 0; cur != nil && step < 1<<16; step++ {
				if ifSet[cur] {
					return true
				}
				if slices.Contains(cur.Target, mergeNode) {
					return false
				}
				if len(cur.Target) != 1 {
					return false
				}
				cur = cur.Target[0]
			}
			return false
		}
		// nearestIfAncestor walks straight back through the single-predecessor chain to the condition
		// node that branches into n (n's dominating if), or nil if the chain forks first.
		nearestIfAncestor := func(n *OpCode) *OpCode {
			cur := n
			for step := 0; cur != nil && step < 1<<16; step++ {
				if len(cur.Source) != 1 {
					return nil
				}
				p := cur.Source[0]
				if isIfNode(p) {
					return p
				}
				cur = p
			}
			return nil
		}
		// The bottom-up merge detection records only the if-nodes nearest the leaves. An outer
		// condition whose arms are BOTH nested ternaries (a balanced tree, c?(a?:):(b?:)) has no
		// direct leaf arm and is therefore absent from ifNodes, leaving two disconnected sub-ternaries
		// with no common root. Walk up from each known condition to its nearest condition ancestor and
		// adopt it when both of its branches feed this same merge, repeating until a fixpoint. The
		// both-branches guard prevents adopting an enclosing if-statement (whose other branch escapes).
		adopted := []*OpCode{}
		for changed := true; changed; {
			changed = false
			for _, n := range append(append([]*OpCode{}, ifNodes...), adopted...) {
				p := nearestIfAncestor(n)
				if p == nil || ifSet[p] || len(p.Target) < 2 {
					continue
				}
				if branchReachesNestedIf(p.Target[0]) && branchReachesNestedIf(p.Target[1]) {
					ifSet[p] = true
					adopted = append(adopted, p)
					changed = true
				}
			}
		}
		if len(adopted) > 0 {
			// A dominating condition was missing (its arms are BOTH nested ternaries), so the legacy
			// combiner would drop an arm. Probe a structural rebuild WITHOUT global side effects and
			// commit only if it forms a clean ternary TREE. Short-circuit &&/|| compile to a DAG that
			// shares a boolean value node; the probe detects the shared leaf (dag) and declines, so we
			// fall through to the proven legacy path rather than regressing those shapes to a stub.
			treeNodes := append(append([]*OpCode{}, ifNodes...), adopted...)
			built := map[*OpCode]*values.TernaryExpression{}
			visited := map[*OpCode]bool{}
			usedLeaf := map[*OpCode]bool{}
			var failed, dag bool
			var probe func(ifNode *OpCode) *values.TernaryExpression
			// A genuine ?: arm leaves its value on the operand stack and never writes a local/field. A
			// store on an arm path means the "merge" is really statement dispatch (e.g. a lexer's
			// if/else chain assigning a token, all branches converging via astore;goto), not a value
			// ternary. Committing that as a tree steals the branch conditions and orphans the stores
			// ("multiple next"), so the probe declines and we keep the legacy path.
			isStoreOp := func(op int) bool {
				switch op {
				case OP_ISTORE, OP_ASTORE, OP_LSTORE, OP_DSTORE, OP_FSTORE,
					OP_ISTORE_0, OP_ASTORE_0, OP_LSTORE_0, OP_DSTORE_0, OP_FSTORE_0,
					OP_ISTORE_1, OP_ASTORE_1, OP_LSTORE_1, OP_DSTORE_1, OP_FSTORE_1,
					OP_ISTORE_2, OP_ASTORE_2, OP_LSTORE_2, OP_DSTORE_2, OP_FSTORE_2,
					OP_ISTORE_3, OP_ASTORE_3, OP_LSTORE_3, OP_DSTORE_3, OP_FSTORE_3,
					OP_IINC, OP_PUTFIELD, OP_PUTSTATIC,
					OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
					return true
				default:
					return false
				}
			}
			arm := func(entry *OpCode) values.JavaValue {
				cur := entry
				for step := 0; cur != nil && step < 1<<16; step++ {
					if failed {
						return nil
					}
					if ifSet[cur] {
						return probe(cur)
					}
					if isStoreOp(cur.Instr.OpCode) {
						failed = true
						return nil
					}
					if slices.Contains(cur.Target, mergeNode) {
						if cur.StackEntry == nil {
							failed = true
							return nil
						}
						if usedLeaf[cur] {
							dag = true // a leaf shared by two branches => not a tree (short-circuit)
						}
						usedLeaf[cur] = true
						return cur.StackEntry.value
					}
					if len(cur.Target) != 1 {
						failed = true
						return nil
					}
					cur = cur.Target[0]
				}
				failed = true
				return nil
			}
			probe = func(ifNode *OpCode) *values.TernaryExpression {
				if t, ok := built[ifNode]; ok {
					dag = true // a condition reached from two parents => not a tree
					return t
				}
				if len(ifNode.Target) < 2 {
					failed = true
					return nil
				}
				visited[ifNode] = true
				tern := values.NewTernaryExpression(values.NewSlotValue(nil, types.NewJavaPrimer(types.JavaBoolean)), nil, nil)
				built[ifNode] = tern
				tern.TrueValue = arm(ifNode.Target[1])
				tern.FalseValue = arm(ifNode.Target[0])
				return tern
			}
			root := treeNodes[0]
			for _, n := range treeNodes {
				if n.Id < root.Id {
					root = n
				}
			}
			rootTern := probe(root)
			allVisited := rootTern != nil
			for _, n := range treeNodes {
				if !visited[n] {
					allVisited = false
				}
			}
			if !failed && !dag && allVisited {
				// Clean tree: wire each condition callback and publish the tree as the merge value.
				// Iterate treeNodes (a stable slice), not the built map, so callback wiring is
				// deterministic; built's keys equal treeNodes for a committed tree.
				for _, ifNode := range treeNodes {
					t := built[ifNode]
					t.ConditionFromOp = ifNode.Id
					ifNode.TernaryChainArm = true
					ifNodeToConditionCallback[ifNode] = func(value values.JavaValue) {
						t.Condition = value
					}
				}
				ternaryExpMergeNodeSlot[code].ResetValue(rootTern)
				code.conditionOpId = 0
				continue
			}
			// Probe declined (DAG / unresolved arm); fall through to the legacy reconstruction.
		}
		// Legacy chain/short-circuit reconstruction. Handles simple, chained, and short-circuit &&/||
		// shapes, and is the safe fallback when the structural probe above declines.
		{
			sort.Slice(ifNodes, func(i, j int) bool {
				return ifNodes[i].Id > ifNodes[j].Id
			})
			var trueFalseValuePair []values.JavaValue
			ifNode := ifNodes[0]
			var source []*OpCode
			falseSource := []*OpCode{}
			trueSource := []*OpCode{}
			WalkGraph[*OpCode](ifNode.Target[0], func(code *OpCode) ([]*OpCode, error) {
				if slices.Contains(code.Target, mergeNode) {
					falseSource = append(falseSource, code)
					return nil, nil
				}
				return code.Target, nil
			})
			WalkGraph[*OpCode](ifNode.Target[1], func(code *OpCode) ([]*OpCode, error) {
				if slices.Contains(code.Target, mergeNode) {
					trueSource = append(trueSource, code)
					return nil, nil
				}
				return code.Target, nil
			})
			source = utils.NewSet(append(trueSource, falseSource...)).List()
			if len(source) != 2 {
				isTernaryExp := false
				if len(source) == 1 {
					if _, ok := source[0].StackEntry.value.(*values.TernaryExpression); ok {
						isTernaryExp = true
					}
				}
				if !isTernaryExp {
					return errors.New("found invalid ternary expression")
				}
			}
			var defaultTarnaryValue *values.TernaryExpression
			for i, opCode := range ifNodes {
				if i == 0 {
					var falseRouteEnd, trueRouteEnd *OpCode
					if slices.Contains(falseSource, source[0]) {
						falseRouteEnd = source[0]
						trueRouteEnd = source[1]
					} else {
						falseRouteEnd = source[1]
						trueRouteEnd = source[0]
					}
					trueFalseValuePair = []values.JavaValue{falseRouteEnd.StackEntry.value, trueRouteEnd.StackEntry.value}
					ternaryValue := values.NewTernaryExpression(values.NewSlotValue(nil, types.NewJavaPrimer(types.JavaBoolean)), trueRouteEnd.StackEntry.value, falseRouteEnd.StackEntry.value)
					code.conditionOpId = opCode.Id
					ternaryExpMergeNodeSlot[code].ResetValue(ternaryValue)
					ifNodeToConditionCallback[opCode] = func(value values.JavaValue) {
						ternaryValue.Condition = value
					}
					defaultTarnaryValue = ternaryValue
				}
				if i != 0 {
					var routeToCode bool
					var target *OpCode
					WalkGraph[*OpCode](opCode.Target[0], func(code *OpCode) ([]*OpCode, error) {
						if isIfNode(code) {
							return nil, nil
						}
						for _, t := range code.Target {
							if t == mergeNode {
								routeToCode = true
								target = code
								return nil, nil
							}
						}
						return code.Target, nil
					})
					if routeToCode {
						if GetRealValue(target.StackEntry.value) == GetRealValue(trueFalseValuePair[0]) {
							ifNodeToConditionCallback[opCode] = func(value values.JavaValue) {
								defaultTarnaryValue.Condition = value
							}
						} else if GetRealValue(target.StackEntry.value) == GetRealValue(trueFalseValuePair[1]) {
							opCode.Negative = !opCode.Negative
							ifNodeToConditionCallback[opCode] = func(value values.JavaValue) {
								defaultTarnaryValue.Condition = value
							}
						} else {
							newValue := values.NewTernaryExpression(values.NewSlotValue(nil, types.NewJavaPrimer(types.JavaBoolean)), ternaryExpMergeNodeSlot[code].GetValue(), target.StackEntry.value)
							newValue.Type().ResetType(ternaryExpMergeNodeSlot[code].TmpType)
							newValue.ConditionFromOp = opCode.Id
							opCode.TernaryChainArm = true
							ifNodeToConditionCallback[opCode] = func(value values.JavaValue) {
								newValue.Condition = value
							}
							ternaryExpMergeNodeSlot[code].ResetValue(newValue)
							code.conditionOpId = 0
						}
					}
					routeToCode = false
					WalkGraph[*OpCode](opCode.Target[1], func(code *OpCode) ([]*OpCode, error) {
						if isIfNode(code) {
							return nil, nil
						}
						for _, t := range code.Target {
							if t == mergeNode {
								routeToCode = true
								target = code
								return nil, nil
							}
						}
						return code.Target, nil
					})
					if routeToCode {
						if GetRealValue(target.StackEntry.value) == GetRealValue(trueFalseValuePair[1]) {
							ifNodeToConditionCallback[opCode] = func(value values.JavaValue) {
								defaultTarnaryValue.Condition = value
							}
						} else if GetRealValue(target.StackEntry.value) == GetRealValue(trueFalseValuePair[0]) {
							opCode.Negative = !opCode.Negative
							ifNodeToConditionCallback[opCode] = func(value values.JavaValue) {
								defaultTarnaryValue.Condition = value
							}
						} else {
							newValue := values.NewTernaryExpression(values.NewSlotValue(nil, types.NewJavaPrimer(types.JavaBoolean)), target.StackEntry.value, ternaryExpMergeNodeSlot[code].GetValue())
							newValue.Type().ResetType(ternaryExpMergeNodeSlot[code].TmpType)
							newValue.ConditionFromOp = opCode.Id
							opCode.TernaryChainArm = true
							ifNodeToConditionCallback[opCode] = func(value values.JavaValue) {
								newValue.Condition = value
							}
							ternaryExpMergeNodeSlot[code].ResetValue(newValue)
							code.conditionOpId = 0
						}
					}
				}
			}
			continue
		}
	}
	for _, slotVal := range ternaryExpMergeNodeSlot {
		ternaryExp := slotVal.GetValue().(*values.TernaryExpression)
		types.MergeTypes(ternaryExp.TrueValue.Type(), ternaryExp.FalseValue.Type())
	}
	return nil
}
func (d *Decompiler) ParseStatement() error {
	funcCtx := d.FunctionContext
	err := d.ParseOpcode()
	if err != nil {
		return err
	}
	// Rewrite pre-Java-6 jsr/ret finally subroutines into the modern inlined-duplicate form so the
	// CFG/structuring below never sees jsr/ret. No-op when the method has none; conservatively
	// leaves the bytecode (and thus the existing stub path) untouched for non-canonical shapes.
	d.inlineJSRSubroutines()
	err = d.ScanJmp()
	if err != nil {
		return err
	}
	err = d.DropUnreachableOpcode()
	if err != nil {
		return err
	}
	err = d.CalcOpcodeStackInfo()
	if err != nil {
		return err
	}
	// convert opcode to statement
	var nodes []*Node
	statementsIndex := 0
	var tryCatchOpcode *OpCode
	refToNewExpressionAssignNode := map[*utils2.VariableId]*Node{}
	conditionOpToAssignNode := map[int]int{}
	appendNodeWithOpcode := func(statement statements.Statement, opcode *OpCode) *Node {
		if opcode.conditionOpId != 0 {
			conditionOpToAssignNode[opcode.conditionOpId] = opcode.Id
		}
		if conditionSt, ok := statement.(*statements.ConditionStatement); ok && opcode.Negative {
			conditionSt.Condition = values.NewUnaryExpression(conditionSt.Condition, values.Not, conditionSt.Condition.Type())
			conditionSt.Neg = !conditionSt.Neg
		}
		if d.ifNodeConditionCallback != nil {
			if cb, ok := d.ifNodeConditionCallback[opcode]; ok {
				if conditionSt, ok := statement.(*statements.ConditionStatement); ok {
					conditionSt.Callback = func(value values.JavaValue) {
						cb(value)
					}
					conditionSt.TernaryChainArm = opcode.TernaryChainArm
				}
			}
		}
		node := NewNode(statement)
		if v, ok := statement.(*statements.AssignStatement); ok {
			if v1, ok := v.LeftValue.(*values.JavaRef); ok {
				refToNewExpressionAssignNode[v1.Id] = node
			}
		}
		node.Id = statementsIndex
		nodes = append(nodes, node)
		if tryCatchOpcode != nil {
			node.IsTryCatch = true
			node.TryNodeId = tryCatchOpcode.TryNode.Id
			for _, code := range tryCatchOpcode.CatchNode {
				node.CatchNodeInfo = append(node.CatchNodeInfo, code)
			}
			tryCatchOpcode = nil
		}
		return node
	}
	mapCodeToStackVarIndex := map[*OpCode]int{}
	//DumpOpcodesToDotExp(d.RootOpCode)
	var runCode func(startNode *OpCode) error
	var parseOpcode func(opcode *OpCode) error
	parseOpcode = func(opcode *OpCode) error {
		appendNode := func(statement statements.Statement) *Node {
			return appendNodeWithOpcode(statement, opcode)
		}
		if opcode.IsTryCatchParent {
			tryCatchOpcode = opcode
		}
		//opcodeIndex := opcode.Id
		statementsIndex = opcode.Id
		switch opcode.Instr.OpCode {
		case OP_ISTORE, OP_ASTORE, OP_LSTORE, OP_DSTORE, OP_FSTORE, OP_ISTORE_0, OP_ASTORE_0, OP_LSTORE_0, OP_DSTORE_0, OP_FSTORE_0, OP_ISTORE_1, OP_ASTORE_1, OP_LSTORE_1, OP_DSTORE_1, OP_FSTORE_1, OP_ISTORE_2, OP_ASTORE_2, OP_LSTORE_2, OP_DSTORE_2, OP_FSTORE_2, OP_ISTORE_3, OP_ASTORE_3, OP_LSTORE_3, OP_DSTORE_3, OP_FSTORE_3:
			refInfos := d.opcodeIdToRef[opcode]
			for i, refInfo := range refInfos {
				value := opcode.stackConsumed[i]
				ref := refInfo[0].(*values.JavaRef)
				isFirst := refInfo[1].(bool)
				assignSt := statements.NewAssignStatement(ref, value, isFirst)
				appendNode(assignSt)
			}
		case OP_CHECKCAST:
			slotVal := opcode.stackProduced[0]
			leftRef := UnpackSoltValue(slotVal).(*values.JavaRef)
			val := GetRealValue(leftRef.Val)
			appendNode(statements.NewAssignStatement(slotVal, val, true))
		case OP_INVOKESTATIC:
			if len(opcode.stackProduced) == 0 {
				classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
				funcCallValue := values.NewFunctionCallExpression(nil, classInfo, classInfo.JavaType.FunctionType()) // 不push到栈中
				//funcCallValue.JavaType = classInfo.JavaType
				funcCallValue.Object = values.NewJavaClassValue(types.NewJavaClass(classInfo.Name))
				funcCallValue.IsStatic = true
				n := 0
				for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
					funcCallValue.Arguments = append(funcCallValue.Arguments, opcode.stackConsumed[n])
					n++
				}
				funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
				appendNode(statements.NewExpressionStatement(funcCallValue))
			}
		case OP_INVOKESPECIAL:
			if len(opcode.stackProduced) == 0 {
				classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
				methodName := classInfo.Member
				funcCallValue := values.NewFunctionCallExpression(nil, classInfo, classInfo.JavaType.FunctionType()) // 不push到栈中
				n := 0
				for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
					funcCallValue.Arguments = append(funcCallValue.Arguments, opcode.stackConsumed[n])
					n++
				}
				funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
				funcCallValue.Object = opcode.stackConsumed[n]
				var skip bool
				func() {
					if methodName != "<init>" {
						return
					}
					if len(funcCallValue.Arguments) != 0 {
						value := GetRealValue(funcCallValue.Object)
						if value == nil {
							return
						}
						if v, ok := value.(*values.NewExpression); ok {
							v.ArgumentsGetter = func() string {
								sts := funk.Map(funcCallValue.Arguments, func(arg values.JavaValue) string {
									return arg.String(funcCtx)
								}).([]string)
								return strings.Join(sts, ",")
							}
							skip = true
						}
					} else {
						skip = true
					}
				}()
				if skip {
					// for _, argument := range append(funcCallValue.Arguments, funcCallValue.Object) {
					// 	val := values.UnpackSoltValue(argument)
					// 	println(val.String(funcCtx))
					// 	println(funcCallValue.String(funcCtx))
					// 	if v, ok := val.(*values.JavaRef); ok {
					// 		attr := d.delRefUserAttr[v.VarUid]
					// 		attr[0]++
					// 		d.delRefUserAttr[v.VarUid] = attr
					// 	}
					// }
					val := UnpackSoltValue(funcCallValue.Object)
					// println(val.String(funcCtx))
					// println(funcCallValue.String(funcCtx))
					if v, ok := val.(*values.JavaRef); ok && v != nil {
						attr := d.delRefUserAttr[v.VarUid]
						attr[0]++
						d.delRefUserAttr[v.VarUid] = attr
					}
					if val, ok := val.(*values.JavaRef); ok && val != nil {
						assignNode := refToNewExpressionAssignNode[val.Id]
						if assignNode != nil {
							assignSt := assignNode.Statement
							assignNode.IsDel = true

							users := d.varUserMap.GetMust(val)
							for _, user := range users {
								if user == nil {
									continue
								}
								user.CurrentOpcode = opcode
							}
							appendNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
								return assignSt.String(funcCtx)
							}, func(oldId *utils2.VariableId, newId *utils2.VariableId) {
								assignSt.ReplaceVar(oldId, newId)
							}))
						}
					}
				} else {
					appendNode(statements.NewExpressionStatement(funcCallValue))
				}
			}
		case OP_INVOKEINTERFACE:
			if len(opcode.stackProduced) == 0 {
				classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
				funcCallValue := values.NewFunctionCallExpression(nil, classInfo, classInfo.JavaType.FunctionType()) // 不push到栈中
				n := 0
				for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
					funcCallValue.Arguments = append(funcCallValue.Arguments, opcode.stackConsumed[n])
					n++
				}
				funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
				funcCallValue.Object = opcode.stackConsumed[n]
				appendNode(statements.NewExpressionStatement(funcCallValue))
			}
		case OP_INVOKEVIRTUAL:
			if len(opcode.stackProduced) == 0 {
				classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
				funcCallValue := values.NewFunctionCallExpression(nil, classInfo, classInfo.JavaType.FunctionType()) // 不push到栈中
				n := 0
				for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
					funcCallValue.Arguments = append(funcCallValue.Arguments, opcode.stackConsumed[n])
					n++
				}
				funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
				funcCallValue.Object = opcode.stackConsumed[n]
				appendNode(statements.NewExpressionStatement(funcCallValue))
			}
		case OP_RETURN:
			appendNode(statements.NewReturnStatement(nil))
		case OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE:
			op := GetNotOp(opcode)
			rv := opcode.stackConsumed[0]
			lv := opcode.stackConsumed[1]
			st := statements.NewConditionStatement(values.NewJavaCompare(lv, rv), op)
			appendNode(st)
		case OP_IFNONNULL:
			st := statements.NewConditionStatement(values.NewJavaCompare(opcode.stackConsumed[0], values.JavaNull), EQ)
			appendNode(st)
		case OP_IFNULL:
			st := statements.NewConditionStatement(values.NewJavaCompare(opcode.stackConsumed[0], values.JavaNull), NEQ)
			appendNode(st)
		case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
			value := opcode.stackConsumed[0]
			index := opcode.stackConsumed[1]
			ref := opcode.stackConsumed[2]
			st := statements.NewArrayMemberAssignStatement(values.NewJavaArrayMember(ref, index), value)
			appendNode(st)
		case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE:
			op := ""
			switch opcode.Instr.OpCode {
			case OP_IFEQ:
				op = "=="
			case OP_IFNE:
				op = "!="
			case OP_IFLE:
				op = "<="
			case OP_IFLT:
				op = "<"
			case OP_IFGT:
				op = ">"
			case OP_IFGE:
				op = ">="
			}
			op = GetReverseOp(op)
			v := opcode.stackConsumed[0]
			if v == nil {
				// Stack consumed value is nil (incomplete simulation for this path).
				// Skip this opcode rather than panicking — the method may still produce
				// partial output for other opcodes.
				return nil
			}
			cmp, ok := v.(*values.JavaCompare)
			if ok {
				st := statements.NewConditionStatement(cmp, op)
				appendNode(st)
			} else {
				st := statements.NewConditionStatement(values.NewJavaCompare(v, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))), op)
				appendNode(st)
			}
		case OP_JSR, OP_JSR_W:
			// No-op if JSR inliner bailed (see CalcOpcodeStackInfo handler).
		case OP_RET:
			// No-op if JSR inliner bailed.
		case OP_GOTO, OP_GOTO_W:
			st := statements.NewGOTOStatement()
			appendNode(st)
		case OP_ATHROW:
			val := opcode.stackConsumed[0]
			appendNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("throw %v", val.String(funcCtx))
			}, func(oldId *utils2.VariableId, newId *utils2.VariableId) {
				val.ReplaceVar(oldId, newId)
			}))
		case OP_IRETURN:
			v := opcode.stackConsumed[0]
			v.Type().ResetType(funcCtx.FunctionType.(*types.JavaFuncType).ReturnType)
			appendNode(statements.NewReturnStatement(v))
		case OP_ARETURN, OP_LRETURN, OP_DRETURN, OP_FRETURN:
			v := opcode.stackConsumed[0]
			v.Type().ResetType(funcCtx.FunctionType.(*types.JavaFuncType).ReturnType)
			appendNode(statements.NewReturnStatement(v))
		case OP_GETFIELD:
		case OP_GETSTATIC:
		case OP_PUTSTATIC:
			if !opcode.SelfOpFolded {
				index := Convert2bytesToInt(opcode.Data)
				staticVal := d.constantPoolGetter(int(index))
				appendNode(statements.NewAssignStatement(staticVal, opcode.stackConsumed[0], false))
			}
		case OP_PUTFIELD:
			if !opcode.SelfOpFolded {
				index := Convert2bytesToInt(opcode.Data)
				staticVal := d.constantPoolGetter(int(index))
				value := opcode.stackConsumed[0]
				field := values.NewRefMember(opcode.stackConsumed[1], staticVal.(*values.JavaClassMember).Member, staticVal.(*values.JavaClassMember).JavaType)
				assignSt := statements.NewAssignStatement(field, value, false)
				appendNode(assignSt)
			}
		case OP_DUP, OP_DUP_X1, OP_DUP_X2, OP_DUP2, OP_DUP2_X1, OP_DUP2_X2:
			refInfos := d.opcodeIdToRef[opcode]
			convertedVals := d.dupConvertedRefValue[opcode]
			for i, refInfo := range refInfos {
				// Prefer the value checkAndConvertRef actually converted; stackConsumed[i] is only
				// aligned when the converted operand sat on top of the consume order. For a compound
				// store on a FIELD array (`this.f[i] op= v`) the dup2 pops the index before the
				// array reference, so stackConsumed[0] is the index and using it would emit
				// `int t = i; t[i] = ...`. The recorded value is the true array reference.
				var value values.JavaValue
				if i < len(convertedVals) {
					value = convertedVals[i]
				} else {
					value = opcode.stackConsumed[i]
				}
				ref := refInfo[0].(*values.JavaRef)
				// This temporary only carried the old value of a folded field/static
				// post-increment/decrement; its `x++` / `x--` expression already embeds the
				// field reference, so emitting the assignment would be both redundant and a
				// branch side-effect that breaks structuring.
				if d.selfOpFoldedRefs[ref.VarUid] {
					continue
				}
				isFirst := refInfo[1].(bool)
				assignSt := statements.NewAssignStatement(ref, value, isFirst)
				appendNode(assignSt)
			}
			//for i, value := range opcode.stackConsumed {
			//	appendNode(statements.NewAssignStatement(opcode.stackProduced[i], value, true))
			//}
		case OP_MONITORENTER:
			v := opcode.stackConsumed[0]
			st := statements.NewMiddleStatement("monitor_enter", v)
			appendNode(st)
		case OP_MONITOREXIT:
			st := statements.NewMiddleStatement("monitor_exit", nil)
			appendNode(st)
		case OP_NOP:
			return nil
		case OP_POP:
			appendNode(statements.NewExpressionStatement(opcode.stackConsumed[0]))
		case OP_POP2:
			for _, value := range opcode.stackConsumed {
				appendNode(statements.NewExpressionStatement(value))
			}
		case OP_TABLESWITCH, OP_LOOKUPSWITCH:
			switchStatement := statements.NewMiddleStatement(statements.MiddleSwitch, []any{opcode.SwitchJmpCase1, opcode.stackConsumed[0]})
			appendNode(switchStatement)
		case OP_IINC:
			// The iinc increment is a SIGNED constant. Reading it unsigned turns `i--`
			// (iinc i, -1 => byte 0xFF) into `i + 255`, and because the renderer prints any
			// INC op as `i++` it silently became `i++`, inverting every descending loop.
			// Sign-extend (int8 / int16) and pick the faithful form: ++ / -- / += k / -= k.
			var inc int
			if opcode.IsWide {
				inc = int(int16(Convert2bytesToInt(opcode.Data[2:])))
			} else {
				inc = int(int8(opcode.Data[1]))
			}
			ref := opcode.Ref
			intType := types.NewJavaPrimer(types.JavaInteger)
			switch {
			case inc == 1:
				appendNode(values.NewBinaryExpression(ref, values.NewJavaLiteral(1, intType), INC, ref.Type()))
			case inc == -1:
				appendNode(values.NewBinaryExpression(ref, values.NewJavaLiteral(1, intType), values.DEC, ref.Type()))
			case inc >= 0:
				appendNode(statements.NewAssignStatement(ref, values.NewBinaryExpression(ref, values.NewJavaLiteral(inc, intType), ADD, ref.Type()), false))
			default:
				appendNode(statements.NewAssignStatement(ref, values.NewBinaryExpression(ref, values.NewJavaLiteral(-inc, intType), SUB, ref.Type()), false))
			}
		case OP_END:
			endNode := statements.NewMiddleStatement("end", nil)
			appendNode(endNode)
		case OP_START:
			endNode := statements.NewMiddleStatement("start", nil)
			appendNode(endNode)
		default:
			return nil
		}
		return nil
	}

	err = WalkGraph[*OpCode](d.opCodes[0], func(code *OpCode) ([]*OpCode, error) {
		var initN int
		if len(code.Source) == 0 {
			mapCodeToStackVarIndex[code] = 0
		} else {
			source := code.Source[0]
			initN = mapCodeToStackVarIndex[source]
			pushL := len(source.Instr.StackPushed)
			initN = initN + pushL
			mapCodeToStackVarIndex[code] = initN
		}
		return code.Target, nil
	})
	if err != nil {
		return err
	}
	runCode = func(startNode *OpCode) error {
		return WalkGraph[*OpCode](startNode, func(node *OpCode) ([]*OpCode, error) {
			err := parseOpcode(node)
			if err != nil {
				return nil, err
			}
			return node.Target, nil
		})
	}
	err = runCode(d.opCodes[0])
	if err != nil {
		return err
	}
	// generate to statement
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Id < nodes[j].Id
	})

	idToNode := map[int]*Node{}
	for _, node := range nodes {
		idToNode[node.Id] = node
	}
	getStatementNextIdByOpcodeId := func(id int) int {
		if v, ok := idToNode[id]; ok {
			return v.Id
		}
		idx := sort.Search(len(nodes), func(i int) bool {
			return nodes[i].Id > id
		})
		if idx >= len(nodes) || idx < 0 {
			return -1
		}
		return nodes[idx].Id
	}
	idToOpcode := map[int]*OpCode{}
	for _, opcode := range d.opCodes {
		idToOpcode[opcode.Id] = opcode
	}
	for _, node := range nodes {
		node := node
		opcode := idToOpcode[node.Id]

		var getTarget func(code *OpCode) []*OpCode
		getTarget = func(code *OpCode) []*OpCode {
			targetList := []*OpCode{}
			if code == nil {
				return nil
			}
			for _, target := range code.Target {
				if target.IsCustom {
					targetList = append(targetList, getTarget(target)...)
				} else {
					targetList = append(targetList, target)
				}
			}
			return targetList
		}
		for _, code := range getTarget(opcode) {
			id := getStatementNextIdByOpcodeId(code.Id)
			if id == -1 {
				continue
			}
			node.Next = append(node.Next, idToNode[id])
			if opcode.Jmp == code.Id {
				node.JmpNode = idToNode[id]
			}
			idToNode[id].Source = append(idToNode[id].Source, node)
		}
	}

	for conditionId, toNodeId := range conditionOpToAssignNode {
		conditionId = getStatementNextIdByOpcodeId(conditionId)
		toNodeId = getStatementNextIdByOpcodeId(toNodeId)
		idToNode[toNodeId].SourceConditionNode = idToNode[conditionId]
	}
	d.RootNode = nodes[0]
	MiscRewriter(d.RootNode, d.delRefUserAttr)
	uidToPairs := omap.NewEmptyOrderedMap[string, []*VarFoldRule]()
	uidToRef := map[string]*values.JavaRef{}
	WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		if v, ok := node.Statement.(*statements.ConditionStatement); ok && v.Neg {
			node.Next[0], node.Next[1] = node.Next[1], node.Next[0]
		}
		if node.IsDel {
			sources := slices.Clone(node.Source)
			next := slices.Clone(node.Next)
			node.RemoveAllSource()
			node.RemoveAllNext()
			for _, source := range sources {
				for _, n := range next {
					n.AddSource(source)
				}
			}
			return next, nil
		}
		return node.Next, nil
	})
	idToNode = map[int]*Node{}
	nodes = []*Node{}
	WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		nodes = append(nodes, node)
		idToNode[node.Id] = node
		return node.Next, nil
	})
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Id < nodes[j].Id
	})
	// A typed-nil *JavaRef can end up as a varUserMap key when loadVarBySlot loads an
	// uninitialized local slot (GetVar returns nil). The variable-fold walker below
	// dereferences ref.VarUid / ref.Val, which would panic and crash the whole decompile.
	// This indicates an upstream stack-simulation gap on a malformed/complex CFG; surface it as an
	// ordinary error so the method degrades to a tagged stub (the same end state as the old panic,
	// but reached cleanly without a Go panic that the recover net has to catch).
	if _, hasNil := d.varUserMap.Get(nil); hasNil {
		return errors.New("variable-fold: nil ref key in varUserMap (uninitialized local slot)")
	}
	d.varUserMap.ForEach(func(ref *values.JavaRef, pairs []*VarFoldRule) bool {
		uidToPairs.Set(ref.VarUid, append(uidToPairs.GetMust(ref.VarUid), pairs...))
		uidToRef[ref.VarUid] = ref
		return true
	})
	disRefUid := lo.Map(d.disFoldRef, func(item *values.JavaRef, index int) string {
		return item.VarUid
	})
	uidToPairs.ForEach(func(uid string, pairs []*VarFoldRule) bool {
		ref := uidToRef[uid]
		val := GetRealValue(ref.Val)
		attr := d.delRefUserAttr[ref.VarUid]
		if slices.Contains(disRefUid, ref.VarUid) {
			return true
		}

		func() {
			if len(pairs) != 2 {
				return
			}
			// A newly created array (new T[...]) assigned through javac's dup idiom must not go through
			// the chained-assignment dup-collapse: that path assumes the duplicated value feeds two
			// plain assignment statements (a = b = expr) and, for an array whose element stores are
			// later folded into an initializer, it drops the array's own declaration, leaving the
			// local undefined. Let it fall through to the normal single-use fold instead.
			if ne, ok := val.(*values.NewExpression); ok && ne.IsArray() {
				return
			}
			isDup := pairs[0].CurrentOpcode.Instr.OpCode == OP_DUP && pairs[1].CurrentOpcode.Instr.OpCode == OP_DUP
			isSameCode := pairs[0].CurrentOpcode.Id == pairs[1].CurrentOpcode.Id
			if !isDup || !isSameCode {
				return
			}
			nodeId := getStatementNextIdByOpcodeId(pairs[0].CurrentOpcode.Id)
			currentNode := idToNode[nodeId]
			v, ok := currentNode.Statement.(*statements.AssignStatement)
			if !ok {
				return
			}
			dupRef, ok := v.LeftValue.(*values.JavaRef)
			if !ok {
				return
			}

			if len(currentNode.Next) != 1 {
				return
			}
			nextNode := currentNode.Next[0]
			nextAssign, ok := nextNode.Statement.(*statements.AssignStatement)
			if !ok {
				return
			}
			rightRef, ok := UnpackSoltValue(nextAssign.JavaValue).(*values.JavaRef)
			if !ok {
				return
			}
			if dupRef.VarUid != ref.VarUid || rightRef.VarUid != ref.VarUid {
				return
			}
			if len(nextNode.Next) != 1 {
				return
			}
			if len(currentNode.Source) != 1 {
				return
			}
			currentNodeSource := currentNode.Source[0]
			nnext := nextNode.Next[0]
			currentNode.RemoveNext(nextNode)
			nextNode.RemoveNext(nnext)
			currentNode.AddNext(nnext)
			currentNodeSource.RemoveNext(currentNode)
			currentNodeSource.AddNext(nnext)

			pairs[1].Replace(values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return statements.NewAssignStatement(nextAssign.LeftValue, val, false).String(funcCtx)
			}, func() types.JavaType {
				return val.Type()
			}))

		}()
		if len(pairs)-attr[0] == 1 {
			pair := pairs[0]
			var sourceNode, node *Node
			if pair.UserIsNextOpcode {
				if len(pair.CurrentOpcode.Target) != 1 {
					return true
				}
				opCodeId := pair.CurrentOpcode.Target[0].Id
				nodeId := getStatementNextIdByOpcodeId(opCodeId)
				node = idToNode[nodeId]
				sourceNode = idToNode[getStatementNextIdByOpcodeId(pair.CurrentOpcode.Id)]
			} else {
				opCodeId := pair.CurrentOpcode.Id
				node = idToNode[getStatementNextIdByOpcodeId(opCodeId)]
				if len(node.Source) == 1 && len(node.Source[0].Next) == 1 {
					if v, ok := node.Source[0].Statement.(*statements.AssignStatement); ok && UnpackSoltValue(v.LeftValue) == ref {
						sourceNode = node.Source[0]
					}
				}
			}
			rewriteIsOk := false
			if sourceNode != nil {
				assignNode := sourceNode
				beforeNodes := slices.Clone(assignNode.Source)
				assignNode.RemoveAllNext()
				for _, beforeNode := range beforeNodes {
					for i, n := range beforeNode.Next {
						if n == assignNode {
							beforeNode.Next[i] = node
							node.Source = append(node.Source, beforeNode)
							assignNode.RemoveSource(beforeNode)
						}
					}
				}
				ref.Id.Delete()
				pair.Replace(val)
				node.SourceConditionNode = assignNode.SourceConditionNode
				rewriteIsOk = true
			}
			if !rewriteIsOk && attr[2] == 1 {
				source := slices.Clone(node.Source)
				node.RemoveAllSource()
				next := slices.Clone(node.Next)
				node.RemoveAllNext()
				for _, source := range source {
					for _, n := range next {
						source.AddNext(n)
					}
				}
				ref.Id.Delete()
				pair.Replace(val)
				rewriteIsOk = true
			}
			// if !rewriteIsOk {
			// 	DumpNodesToDotExp(d.RootNode)
			// 	(pair[0]).(func(value values.JavaValue))(val)
			// 	sources := slices.Clone(node.Source)
			// 	nexts := slices.Clone(node.Next)
			// 	for _, s := range sources {
			// 		s.RemoveNext(node)
			// 		for _, n := range nexts {
			// 			s.AddNext(n)
			// 			node.RemoveNext(n)
			// 		}
			// 	}
			// 	DumpNodesToDotExp(d.RootNode)
			// 	// for _, source := range slices.Clone(node.Source) {
			// 	// 	for i, n := range source.Next {
			// 	// 		if n == node {
			// 	// 			source.Next[i] = next
			// 	// 			next.Source = append(next.Source, source)
			// 	// 			next.RemoveSource(node)
			// 	// 			node.RemoveSource(source)
			// 	// 		}
			// 	// 	}
			// 	// }
			// }
		} else if len(pairs)-attr[0] == 0 {

		}
		return true
	})

	idToNode = map[int]*Node{}
	nodes = []*Node{}
	WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		nodes = append(nodes, node)
		idToNode[node.Id] = node
		return node.Next, nil
	})
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Id < nodes[j].Id
	})
	err = WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		if node.IsTryCatch {
			tryNodeId := getStatementNextIdByOpcodeId(node.TryNodeId)
			tryNodes := NodeFilter(node.Next, func(n *Node) bool {
				return n.Id == tryNodeId
			})
			if len(tryNodes) == 0 {
				return nil, errors.New("not found try body")
			}
			tryStartNode := tryNodes[0]
			catchInfos := slices.Clone(node.CatchNodeInfo)
			// group by endIndex
			catchNodeMap := map[int][]*Node{}
			for _, catchInfo := range catchInfos {
				// Multiple catch handlers can protect the same try region (same end index): a real
				// catch (e.g. ArrayIndexOutOfBoundsException) plus the synthetic catch-all `any`
				// handler that javac emits for a `finally`. These must be APPENDED into one group so
				// they all become successors of a single tryStart node; the previous code overwrote
				// the slot, dropping the earlier handler and leaving it dangling on the pre-try
				// statement node with two successors ("multiple next"). The raw NodeFilter result is
				// appended without deduping on purpose: a multi-catch (`A | B`) shares one handler PC
				// and therefore appears as two identical edges in node.Next, and the construction
				// below removes one edge per group entry, so preserving the multiplicity is required
				// to clear both edges. AddNext on the try node dedupes the successor side, and any
				// surplus RemoveNext calls are safe no-ops.
				found := NodeFilter(node.Next, func(n *Node) bool {
					return n.Id == getStatementNextIdByOpcodeId(catchInfo.OpCode.Id)
				})
				endIndex := int(catchInfo.EndIndex)
				catchNodeMap[endIndex] = append(catchNodeMap[endIndex], found...)
			}
			endIndexes := []int{}
			for endIndex := range catchNodeMap {
				endIndexes = append(endIndexes, endIndex)
			}
			sort.Slice(endIndexes, func(i, j int) bool {
				return endIndexes[i] < endIndexes[j]
			})
			// build try node
			currentTryNode := tryStartNode
			for _, endIndex := range endIndexes {
				catchNodes := catchNodeMap[endIndex]
				tryNode := NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
				tryNode.Id = statementsIndex
				for _, n := range currentTryNode.Source {
					currentTryNode.RemoveSource(n)
					tryNode.AddSource(n)
				}
				tryNode.AddNext(currentTryNode)
				statementsIndex++
				for _, catchNode := range catchNodes {
					// Mark the handler entry so TryRewriter can identify it structurally even when the
					// handler body has no leading exception-store (e.g. an empty `catch` that discards
					// the unused exception with `pop`).
					catchNode.IsCatchStart = true
					tryNode.AddNext(catchNode)
					node.RemoveNext(catchNode)
				}
				node.AddNext(tryNode)
				currentTryNode = tryNode
			}
			// tryNode := NewNode(statements.NewMiddleStatement(statements.MiddleTryStart, nil))
			// tryNode.Id = statementsIndex
			// statementsIndex++
			// node.RemoveNext(tryNode)
			// node.AddNext(tryNode)
			// tryNode.AddNext(tryStartNode)
			// for _, catchNode := range catchNodes {
			// 	tryNode.AddNext(catchNode)
			// 	node.RemoveNext(catchNode)
			// }
			// source := funk.Filter(tryStartNode.Source, func(item *Node) bool {
			// 	return item != tryNode
			// }).([]*Node)
			// for _, n := range source {
			// 	tryStartNode.RemoveSource(n)
			// }
			// for _, n := range source {
			// 	tryNode.AddSource(n)
			// }
		}
		return node.Next, nil
	})
	if err != nil {
		return err
	}
	err = d.RemoveGotoStatement()
	if err != nil {
		return err
	}
	err = d.ReGenerateNodeId()
	if err != nil {
		return err
	}
	return nil
}

func (d *Decompiler) ReGenerateNodeId() error {
	id := 0
	return WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		node.Id = id
		id++
		return node.Next, nil
	})
}
func (d *Decompiler) RemoveGotoStatement() error {
	return WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		if _, ok := node.Statement.(*statements.GOTOStatement); ok {
			for _, source := range node.Source {
				source.ReplaceNext(node, node.Next[0])
			}
			source := node.Next[0].Source
			for i, n := range node.Next[0].Source {
				if n == node {
					source = append(source[:i], source[i+1:]...)
					break
				}
			}
			node.Next[0].Source = source
			for _, n := range node.Source {
				node.Next[0].Source = append(node.Next[0].Source, n)
			}
		}
		if _, ok := node.Statement.(*statements.ConditionStatement); ok {
			var trueIndex, falseIndex int
			if node.Next[0] == node.JmpNode {
				trueIndex = 0
				falseIndex = 1
			} else {
				trueIndex = 1
				falseIndex = 0
			}
			node.TrueNode = func() *Node {
				if trueIndex >= len(node.Next) {
					return nil
				}
				return node.Next[trueIndex]
			}
			node.FalseNode = func() *Node {
				if falseIndex >= len(node.Next) {
					return nil
				}
				return node.Next[falseIndex]
			}
		}
		return node.Next, nil
	})
}

func (d *Decompiler) ParseSourceCode() error {
	err := d.ParseStatement()
	if err != nil {
		return err
	}
	return nil
}
func DumpOpcodesToDotExp(code *OpCode) string {
	var visitor func(node *OpCode, visited map[*OpCode]bool, sb *strings.Builder)
	visitor = func(node *OpCode, visited map[*OpCode]bool, sb *strings.Builder) {
		if node == nil {
			return
		}
		if visited[node] {
			return
		}
		visited[node] = true
		for _, nextNode := range node.Target {
			sb.WriteString(fmt.Sprintf("  \"%d%s\" -> \"%d%s\";\n", node.Id, node.Instr.Name, nextNode.Id, nextNode.Instr.Name))
			visitor(nextNode, visited, sb)
		}
	}
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	visited := make(map[*OpCode]bool)
	visitor(code, visited, &sb)
	sb.WriteString("}\n")
	println(sb.String())
	return sb.String()
}
