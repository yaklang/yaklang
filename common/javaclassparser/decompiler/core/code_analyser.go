package core

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
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
	FunctionType                  *types.JavaFuncType
	opcodeToSimulateStack         map[*OpCode]*StackSimulationImpl
	FunctionContext               *class_context.ClassContext
	varTable                      map[int]*values.JavaRef
	opcodeIdToRef                 map[*OpCode][][2]any
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
	DumpClassLambdaMethod         func(name, desc string, id *utils2.VariableId) (string, error)
	CurrentId                     int
	BodyStartId                   int
	BaseVarId                     *utils2.VariableId
	Params                        []values.JavaValue
	ifNodeConditionCallback       map[*OpCode]func(value values.JavaValue)

	varUserMap     *omap.OrderedMap[*values.JavaRef, []*VarFoldRule]
	disFoldRef     []*values.JavaRef
	delRefUserAttr map[string][3]int // [0] = del times,[1] = assign times, [2] = self assign
}
type VarFoldRule struct {
	Replace          func(v values.JavaValue)
	CurrentOpcode    *OpCode
	UserIsNextOpcode bool
}

func NewDecompiler(bytecodes []byte, constantPoolGetter func(id int) values.JavaValue) *Decompiler {
	return &Decompiler{
		FunctionContext:     &class_context.ClassContext{},
		bytecodes:           bytecodes,
		constantPoolGetter:  constantPoolGetter,
		offsetToOpcodeIndex: map[uint16]int{},
		opcodeIndexToOffset: map[int]uint16{},
		varTable:            map[int]*values.JavaRef{},
		opcodeIdToRef:       map[*OpCode][][2]any{},
		varUserMap:          omap.NewEmptyOrderedMap[*values.JavaRef, []*VarFoldRule](),
		delRefUserAttr:      map[string][3]int{},
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
			err = errors.New(fmt.Sprintf("%v", e))
		}
	}()
	defer func() {
		if len(d.opCodes) > 0 {
			d.RootOpCode = d.opCodes[0]
		}
	}()
	opcodes := []*OpCode{}
	opcodes = append(opcodes, &OpCode{Instr: &Instruction{OpCode: OP_START}})
	offsetToIndex := map[uint16]int{}
	indexToOffset := map[int]uint16{}
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
	visitNodeRecord := utils.NewSet[*OpCode]()
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

			if visitNodeRecord.Has(opcode) {
				break
			}
			visitNodeRecord.Add(opcode)
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
					if slices.Contains(opcode.Target, d.opCodes[gotoOp]) {
						opcode.SwitchJmpCase1.Set(v, len(opcode.Target)-1)
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
	visitNodeRecord := utils.NewSet[*OpCode]()
	err := WalkGraph[*OpCode](d.opCodes[0], func(code *OpCode) ([]*OpCode, error) {
		visitNodeRecord.Add(code)
		target := []*OpCode{}
		for _, opCode := range code.Target {
			target = append(target, opCode)
		}
		return target, nil
	})
	if err != nil {
		return err
	}
	var newOpcodes []*OpCode
	for _, code := range d.opCodes {
		if !visitNodeRecord.Has(code) {
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

func (d *Decompiler) calcOpcodeStackInfo(runtimeStackSimulation StackSimulation, opcode *OpCode) error {
	funcCtx := d.FunctionContext
	checkAndConvertRef := func(value values.JavaValue) func(int) {
		if _, ok := UnpackSoltValue(runtimeStackSimulation.Peek()).(*values.JavaRef); !ok {
			val := runtimeStackSimulation.Pop().(values.JavaValue)
			ref := runtimeStackSimulation.NewVar(val)
			d.opcodeIdToRef[opcode] = append(d.opcodeIdToRef[opcode], [2]any{ref, true})
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
		runtimeStackSimulation.Push(values.NewJavaLiteral(opcode.Data[0], types.NewJavaPrimer(types.JavaInteger)))
	case OP_SIPUSH:
		runtimeStackSimulation.Push(values.NewJavaLiteral(Convert2bytesToInt(opcode.Data), types.NewJavaPrimer(types.JavaInteger)))
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
		typ := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data[:2]))).(*values.JavaClassValue).Type()
		var lens []values.JavaValue
		for _, d := range runtimeStackSimulation.PopN(typ.ArrayDim()) {
			lens = append(lens, d.(values.JavaValue))
			typ = types.NewJavaArrayType(typ)
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
			panic("not support")
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
			return fmt.Sprintf("(%s)%s", fname, arg.String(funcCtx))
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
			return fmt.Sprintf("(%s)(%s)", classInfo.String(funcCtx), arg.String(funcCtx))
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
		var callResult values.JavaValue
		if f := buildinBootstrapMethods[fmt.Sprintf("%s.%s", memberInfo.Name, memberInfo.Member)]; f != nil {
			callResult, err = f(refMethod.Arguments...)(d, runtimeStackSimulation, callSiteReturnType.FunctionType().ReturnType, args...)
			if err != nil {
				return fmt.Errorf("call bootstrap method error: %v", err)
			}
		} else {
			callResult, err = buildinBootstrapMethods["defaultBootstrapMethod"]()(d, runtimeStackSimulation, callSiteReturnType.FunctionType().ReturnType, args...)
			return fmt.Errorf("call bootstrap method error: %v", err)
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
		return errors.New("not support opcode: jsr")
	case OP_RET:
		return errors.New("not support opcode: ret")
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
		statements.NewAssignStatement(staticVal, runtimeStackSimulation.Pop().(values.JavaValue), false)
	case OP_PUTFIELD:
		index := Convert2bytesToInt(opcode.Data)
		staticVal := d.constantPoolGetter(int(index))
		value := runtimeStackSimulation.Pop().(values.JavaValue)
		field := values.NewRefMember(runtimeStackSimulation.Pop().(values.JavaValue), staticVal.(*values.JavaClassMember).Member, staticVal.(*values.JavaClassMember).JavaType)
		statements.NewAssignStatement(field, value, false)
	case OP_SWAP:
		v1 := runtimeStackSimulation.Pop()
		v2 := runtimeStackSimulation.Pop()
		runtimeStackSimulation.Push(v1)
		runtimeStackSimulation.Push(v2)
	case OP_DUP:
		checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))(1)
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
		datas := runtimeStackSimulationPopN(2)
		runtimeStackSimulationPushReverse(datas)
		runtimeStackSimulationPushReverse(datas)
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
func (d *Decompiler) CalcOpcodeStackInfo() error {
	opcodeToSim := map[*OpCode]*StackSimulationImpl{}
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
			runtimeSim.AssignVar(slotIndex, values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return ""
			}, func() types.JavaType {
				return paramType
			}))
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
	nodeToVarScope := map[*OpCode]*Scope{}
	getVarScope := func(code *OpCode) *Scope {
		if vt, ok := nodeToVarScope[code]; ok {
			return vt
		}
		panic("not found var table")
	}
	setVarScope := func(code *OpCode, scope *Scope) {
		nodeToVarScope[code] = scope
	}
	ifNodeToConditionCallback := map[*OpCode]func(values.JavaValue){}
	var preRuntimeStackSimulation *StackSimulationImpl
	varTable := map[int]*values.JavaRef{}
	err := WalkGraph[*OpCode](d.RootOpCode, func(code *OpCode) ([]*OpCode, error) {
		if IsSwitchOpcode(code.Instr.OpCode) {
			sort.Slice(code.Target, func(i, j int) bool {
				return true
			})
		}
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
				runtimeStackSimulation = preRuntimeStackSimulation
			} else {
				entry := code.Source[0].StackEntry
				if entry == nil {
					return nil, fmt.Errorf("not found simuation stack for opcode %d", code.Source[0].Id)
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
					return nil, fmt.Errorf("not found simuation stack for opcode %d", source.Id)
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

			if len(validSources) == 0 {
				return nil, errors.New("invalid if merge node")
			}
			size := -1
			for _, validSource := range validSources {
				stackEntry := validSource.StackEntry
				scope := getVarScope(validSource)
				stackSize := NewStackSimulation(stackEntry, scope.VarTable, scope.VarId).Size()
				//if stackSize > 1 {
				//	return nil, fmt.Errorf("invalid stack size %d for opcode %d", stackSize, code.Id)
				//}
				if size == -1 {
					size = stackSize
				} else {
					if size != stackSize {
						return nil, fmt.Errorf("invalid stack size %d for opcode %d", stackSize, code.Id)
					}
				}
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
			if code.ExceptionTypeIndex != 0 {
				typ = d.GetValueFromPool(int(code.ExceptionTypeIndex)).Type()
			} else {
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
		preRuntimeStackSimulation = runtimeStackSimulation
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
	for _, code := range ternaryExpMergeNode {
		mergeNode := code
		ifNodes := mergeToIfNode[code]
		sort.Slice(ifNodes, func(i, j int) bool {
			return ifNodes[i].Id > ifNodes[j].Id
		})
		//var preVal values.JavaValue
		var trueFalseValuePair []values.JavaValue
		if len(ifNodes) > 0 {
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
							ifNodeToConditionCallback[opCode] = func(value values.JavaValue) {
								newValue.Condition = value
							}
							ternaryExpMergeNodeSlot[code].ResetValue(newValue)
							code.conditionOpId = 0
						}
					}
				}
			}
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
					if v, ok := val.(*values.JavaRef); ok {
						attr := d.delRefUserAttr[v.VarUid]
						attr[0]++
						d.delRefUserAttr[v.VarUid] = attr
					}
					if val, ok := val.(*values.JavaRef); ok {
						assignNode := refToNewExpressionAssignNode[val.Id]
						if assignNode != nil {
							assignSt := assignNode.Statement
							assignNode.IsDel = true

							users := d.varUserMap.GetMust(val)
							for _, user := range users {
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
				panic("not support")
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
			return errors.New("not support opcode: jsr")
		case OP_RET:
			return errors.New("not support opcode: ret")
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
			index := Convert2bytesToInt(opcode.Data)
			staticVal := d.constantPoolGetter(int(index))
			appendNode(statements.NewAssignStatement(staticVal, opcode.stackConsumed[0], false))
		case OP_PUTFIELD:
			index := Convert2bytesToInt(opcode.Data)
			staticVal := d.constantPoolGetter(int(index))
			value := opcode.stackConsumed[0]
			field := values.NewRefMember(opcode.stackConsumed[1], staticVal.(*values.JavaClassMember).Member, staticVal.(*values.JavaClassMember).JavaType)
			assignSt := statements.NewAssignStatement(field, value, false)
			appendNode(assignSt)
		case OP_DUP, OP_DUP_X1, OP_DUP_X2, OP_DUP2, OP_DUP2_X1, OP_DUP2_X2:
			refInfos := d.opcodeIdToRef[opcode]
			for i, refInfo := range refInfos {
				value := opcode.stackConsumed[i]
				ref := refInfo[0].(*values.JavaRef)
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
			var inc int
			if opcode.IsWide {
				inc = int(Convert2bytesToInt(opcode.Data[2:]))
			} else {
				inc = int(opcode.Data[1])
			}
			ref := opcode.Ref
			appendNode(values.NewBinaryExpression(ref, values.NewJavaLiteral(inc, types.NewJavaPrimer(types.JavaInteger)), INC, ref.Type()))
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
				catchNodeMap[int(catchInfo.EndIndex)] = NodeFilter(node.Next, func(n *Node) bool {
					return n.Id == getStatementNextIdByOpcodeId(catchInfo.OpCode.Id)
				})
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
