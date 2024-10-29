package core

import (
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/utils"
	"maps"
	"sort"
	"strings"
)

type ExceptionTableEntry struct {
	StartPc   uint16
	EndPc     uint16
	HandlerPc uint16
	CatchType uint16
}
type Decompiler struct {
	FunctionType                  *types.JavaFuncType
	FunctionContext               *class_context.ClassContext
	varTable                      map[int]*values.JavaRef
	idToValue                     map[int]values.JavaValue
	currentVarId                  int
	bytecodes                     []byte
	opCodes                       []*OpCode
	OpCodeRoot                    *OpCode
	RootNode                      *Node
	constantPoolGetter            func(id int) values.JavaValue
	ConstantPoolLiteralGetter     func(constantPoolGetterid int) values.JavaValue
	ConstantPoolInvokeDynamicInfo func(id int) (string, string)
	offsetToOpcodeIndex           map[uint16]int
	opcodeIndexToOffset           map[int]uint16
	ExceptionTable                []*ExceptionTableEntry
	CurrentId                     int
}

func NewDecompiler(bytecodes []byte, constantPoolGetter func(id int) values.JavaValue) *Decompiler {
	return &Decompiler{
		FunctionContext:     &class_context.ClassContext{},
		bytecodes:           bytecodes,
		constantPoolGetter:  constantPoolGetter,
		offsetToOpcodeIndex: map[uint16]int{},
		opcodeIndexToOffset: map[int]uint16{},
		varTable:            map[int]*values.JavaRef{},
		currentVarId:        -1,
		idToValue:           map[int]values.JavaValue{},
	}
}

func (d *Decompiler) GetValueFromPool(index int) values.JavaValue {
	return d.constantPoolGetter(index)
}
func (d *Decompiler) GetTypeFromPool(index int) types.JavaType {
	val := d.constantPoolGetter(index).(*values.JavaClassValue)
	typ, err := types.ParseDescriptor(val.Type().(*types.JavaClass).Name)
	if err != nil {
		panic("parse type failed")
	}
	return typ
}
func (d *Decompiler) GetMethodFromPool(index int) *values.JavaClassMember {
	return d.constantPoolGetter(index).(*values.JavaClassMember)
}
func (d *Decompiler) ParseOpcode() error {
	defer func() {
		if len(d.opCodes) > 0 {
			d.OpCodeRoot = d.opCodes[0]
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
				SetOpcode(pre, opcode)
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
					//if pre != nil {
					//	SetOpcode(pre, d.opCodes[gotoOp])
					//}
				}
			}

			if visitNodeRecord.Has(opcode) {
				break
			}
			visitNodeRecord.Add(opcode)
			pre = opcode
			switch opcode.Instr.OpCode {
			case OP_RETURN, OP_IRETURN, OP_ARETURN, OP_LRETURN, OP_DRETURN, OP_FRETURN, OP_ATHROW:
				opcode.Target = []*OpCode{endOp}
				pre = nil
			case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
				gotoRaw := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+gotoRaw]
				SetOpcode(opcode, d.opCodes[gotoOp])
				walkNode(gotoOp)
			case OP_GOTO:
				target := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+target]
				SetOpcode(opcode, d.opCodes[gotoOp])
				walkNode(gotoOp)
				return
			case OP_GOTO_W:
				target := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+target]
				SetOpcode(opcode, d.opCodes[gotoOp])
				walkNode(gotoOp)
				return
			case OP_LOOKUPSWITCH, OP_TABLESWITCH:
				targets := []uint32{}
				for _, u := range opcode.SwitchJmpCase {
					targets = append(targets, u)
				}
				for _, target := range targets {
					gotoOp := d.offsetToOpcodeIndex[uint16(target)]
					SetOpcode(opcode, d.opCodes[gotoOp])
					walkNode(gotoOp)
				}
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
func (d *Decompiler) GetRealValue(value values.JavaValue) values.JavaValue {
	if ref, ok := value.(*values.JavaRef); ok {
		val := d.idToValue[ref.Id]
		return d.GetRealValue(val)
	}
	return value
}
func (d *Decompiler) NewVar(val values.JavaValue) *values.JavaRef {
	d.currentVarId++
	newRef := values.NewJavaRef(d.currentVarId, val.Type())
	d.idToValue[d.currentVarId] = val
	return newRef
}
func (d *Decompiler) AssignVar(slot int, typ values.JavaValue) (*values.JavaRef, bool) {
	if slot == 7 {
		print()
	}
	ref, ok := d.varTable[slot]
	if !ok || ref.String(d.FunctionContext) != typ.String(d.FunctionContext) {
		newRef := d.NewVar(typ)
		d.varTable[slot] = newRef
		return newRef, true
	}
	if ref.Id == 2 {
		print()
	}
	return ref, false
}
func (d *Decompiler) GetVar(slot int) *values.JavaRef {
	if d.varTable[slot] == nil {
		panic(fmt.Sprintf("slot %d is nil", slot))
	}
	if d.varTable[slot].Id == 2 {
		print()
	}
	return d.varTable[slot]
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

	// convert opcode to statement
	var nodes []*Node
	statementsIndex := 0
	appendNode := func(statement statements.Statement) *Node {
		node := NewNode(statement)
		node.Id = statementsIndex
		nodes = append(nodes, node)
		return node
	}

	// add statement handle for complete toStatement
	//var statementHandle []func(getter func(id int) (toStatement int))
	//addStatementHandle := func(handle func(getter func(id int) (toStatement int))) {
	//	statementHandle = append(statementHandle, handle)
	//}

	getConstantPoolValue := func(opcode *OpCode) values.JavaValue {
		return d.getPoolValue(int(Convert2bytesToInt(opcode.Data)))
	}

	runtimeStackSimulation := utils.NewStack[any]()
	stackVarIndex := 0
	mapCodeToStackVarIndex := map[*OpCode]int{}
	assignStackVar := func(value values.JavaValue) {
		ref := values.NewJavaRef(stackVarIndex, value.Type())
		appendNode(statements.NewStackAssignStatement(stackVarIndex, ref))
		ref.StackVar = value
		runtimeStackSimulation.Push(ref)
		stackVarIndex++
	}
	lambdaIndex := 0
	getLambdaIndex := func() int {
		defer func() { lambdaIndex++ }()
		return lambdaIndex
	}
	if !d.FunctionContext.IsStatic {
		d.FunctionType.ParamTypes = append([]types.JavaType{types.NewJavaClass(d.FunctionContext.ClassName)}, d.FunctionType.ParamTypes...)
	}

	i := 0
	for _, paramType := range d.FunctionType.ParamTypes {
		//assignStackVar(values.NewJavaRef(stackVarIndex, paramType))
		d.AssignVar(i, values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
			return ""
		}, func() types.JavaType {
			return paramType
		}))
		i += GetTypeSize(paramType)
	}
	if !d.FunctionContext.IsStatic {
		d.GetVar(0).IsThis = true
	}
	//d.AssignVar(0, values.NewJavaRef(0,""))
	checkAndConvertRef := func(value values.JavaValue) {
		if _, ok := runtimeStackSimulation.Peek().(*values.JavaRef); !ok {
			val := runtimeStackSimulation.Pop().(values.JavaValue)
			ref := d.NewVar(val)
			runtimeStackSimulation.Push(ref)
			appendNode(statements.NewAssignStatement(ref, val, true))
		}
	}
	parseOpcode := func(opcode *OpCode) {
		if opcode.Id == 307 {
			print()
		}
		//opcodeIndex := opcode.Id
		statementsIndex = opcode.Id
		stackVarIndex = mapCodeToStackVarIndex[opcode]
		switch opcode.Instr.OpCode {
		case OP_ALOAD, OP_ILOAD, OP_LLOAD, OP_DLOAD, OP_FLOAD, OP_ALOAD_0, OP_ILOAD_0, OP_LLOAD_0, OP_DLOAD_0, OP_FLOAD_0, OP_ALOAD_1, OP_ILOAD_1, OP_LLOAD_1, OP_DLOAD_1, OP_FLOAD_1, OP_ALOAD_2, OP_ILOAD_2, OP_LLOAD_2, OP_DLOAD_2, OP_FLOAD_2, OP_ALOAD_3, OP_ILOAD_3, OP_LLOAD_3, OP_DLOAD_3, OP_FLOAD_3:
			if opcode.Id == 46 {
				print()
			}
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

			if slot == -1 {
				print()
			}
			runtimeStackSimulation.Push(d.GetVar(slot))
			////return mkRetrieve(variableFactory);
		case OP_ACONST_NULL:
			assignStackVar(values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object")))
		case OP_ICONST_M1:
			assignStackVar(values.NewJavaLiteral(-1, types.NewJavaPrimer(types.JavaInteger)))
		case OP_ICONST_0:
			assignStackVar(values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger)))
		case OP_ICONST_1:
			assignStackVar(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))
		case OP_ICONST_2:
			assignStackVar(values.NewJavaLiteral(2, types.NewJavaPrimer(types.JavaInteger)))
		case OP_ICONST_3:
			assignStackVar(values.NewJavaLiteral(3, types.NewJavaPrimer(types.JavaInteger)))
		case OP_ICONST_4:
			assignStackVar(values.NewJavaLiteral(4, types.NewJavaPrimer(types.JavaInteger)))
		case OP_ICONST_5:
			assignStackVar(values.NewJavaLiteral(5, types.NewJavaPrimer(types.JavaInteger)))
		case OP_LCONST_0:
			assignStackVar(values.NewJavaLiteral(int64(0), types.NewJavaPrimer(types.JavaLong)))
		case OP_LCONST_1:
			assignStackVar(values.NewJavaLiteral(int64(1), types.NewJavaPrimer(types.JavaLong)))
		case OP_FCONST_0:
			assignStackVar(values.NewJavaLiteral(float32(0), types.NewJavaPrimer(types.JavaFloat)))
		case OP_FCONST_1:
			assignStackVar(values.NewJavaLiteral(float32(1), types.NewJavaPrimer(types.JavaFloat)))
		case OP_FCONST_2:
			assignStackVar(values.NewJavaLiteral(float32(2), types.NewJavaPrimer(types.JavaFloat)))
		case OP_DCONST_0:
			assignStackVar(values.NewJavaLiteral(float64(0), types.NewJavaPrimer(types.JavaDouble)))
		case OP_DCONST_1:
			assignStackVar(values.NewJavaLiteral(float64(1), types.NewJavaPrimer(types.JavaDouble)))
		case OP_BIPUSH:
			assignStackVar(values.NewJavaLiteral(opcode.Data[0], types.NewJavaPrimer(types.JavaInteger)))
		case OP_SIPUSH:
			assignStackVar(values.NewJavaLiteral(Convert2bytesToInt(opcode.Data), types.NewJavaPrimer(types.JavaInteger)))
		case OP_ISTORE, OP_ASTORE, OP_LSTORE, OP_DSTORE, OP_FSTORE, OP_ISTORE_0, OP_ASTORE_0, OP_LSTORE_0, OP_DSTORE_0, OP_FSTORE_0, OP_ISTORE_1, OP_ASTORE_1, OP_LSTORE_1, OP_DSTORE_1, OP_FSTORE_1, OP_ISTORE_2, OP_ASTORE_2, OP_LSTORE_2, OP_DSTORE_2, OP_FSTORE_2, OP_ISTORE_3, OP_ASTORE_3, OP_LSTORE_3, OP_DSTORE_3, OP_FSTORE_3:
			slot := GetStoreIdx(opcode)
			value := runtimeStackSimulation.Pop().(values.JavaValue)
			ref, isFirst := d.AssignVar(slot, value)
			appendNode(statements.NewAssignStatement(ref, value, isFirst))
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
			value := getConstantPoolValue(opcode)
			length := runtimeStackSimulation.Pop().(values.JavaValue)
			arrayType := types.NewJavaArrayType(value.(*values.JavaClassValue).Type())
			exp := values.NewNewArrayExpression(arrayType, length)
			runtimeStackSimulation.Push(exp)
		case OP_MULTIANEWARRAY:
			typ := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data[:2]))).(*values.JavaClassValue).Type()
			//dimensions := int(opcode.Data[2])
			//typ, err := types.ParseDescriptor(desc)
			//if err != nil {
			//	log.Errorf("parse type `%s` error: %s", desc, err)
			//}
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
			if opcode.Id == 178 {
				print()
			}
			index := runtimeStackSimulation.Pop().(values.JavaValue)
			ref := runtimeStackSimulation.Pop().(values.JavaValue)
			runtimeStackSimulation.Push(values.NewJavaArrayMember(ref, index))
		case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
			value := runtimeStackSimulation.Pop().(values.JavaValue)
			index := runtimeStackSimulation.Pop().(values.JavaValue)
			ref := runtimeStackSimulation.Pop().(values.JavaValue)
			appendNode(statements.NewArrayMemberAssignStatement(values.NewJavaArrayMember(ref, index), value))
		case OP_LCMP, OP_DCMPG, OP_DCMPL, OP_FCMPG, OP_FCMPL:
			var1 := runtimeStackSimulation.Pop().(values.JavaValue)
			var2 := runtimeStackSimulation.Pop().(values.JavaValue)
			runtimeStackSimulation.Push(values.NewBinaryExpression(var1, var2, "compare", types.NewJavaPrimer(types.JavaBoolean)))
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
			ref := d.NewVar(value)
			appendNode(statements.NewAssignStatement(ref, value, true))
			runtimeStackSimulation.Push(ref)
		case OP_INVOKESTATIC:
			classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
			methodName := classInfo.Member
			funcCallValue := values.NewFunctionCallExpression(nil, methodName, classInfo.JavaType.FunctionType()) // 不push到栈中
			//funcCallValue.JavaType = classInfo.JavaType
			funcCallValue.Object = values.NewJavaClassValue(types.NewJavaClass(classInfo.Name))
			funcCallValue.IsStatic = true
			for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(values.JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
				runtimeStackSimulation.Push(funcCallValue)
			} else {
				appendNode(statements.NewExpressionStatement(funcCallValue))
			}
		case OP_INVOKEDYNAMIC:
			_, desc := d.ConstantPoolInvokeDynamicInfo(int(Convert2bytesToInt(opcode.Data)))
			typ, err := types.ParseMethodDescriptor(desc)
			if err != nil {
				panic(err)
				return
			}
			runtimeStackSimulation.Push(values.NewLambdaFuncRef(getLambdaIndex(), typ.FunctionType().ReturnType))
		case OP_INVOKESPECIAL:
			classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
			methodName := classInfo.Member
			funcCallValue := values.NewFunctionCallExpression(nil, methodName, classInfo.JavaType.FunctionType()) // 不push到栈中
			for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(values.JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)

			funcCallValue.Object = runtimeStackSimulation.Pop().(values.JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
				runtimeStackSimulation.Push(funcCallValue)
			} else {
				var skip bool
				func() {
					if methodName != "<init>" {
						return
					}
					if len(funcCallValue.Arguments) != 0 {
						value := d.GetRealValue(funcCallValue.Object)
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
				if !skip {
					appendNode(statements.NewExpressionStatement(funcCallValue))
				}
			}
		case OP_INVOKEINTERFACE:
			classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
			methodName := classInfo.Member
			funcCallValue := values.NewFunctionCallExpression(nil, methodName, classInfo.JavaType.FunctionType()) // 不push到栈中
			for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(values.JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
			funcCallValue.Object = runtimeStackSimulation.Pop().(values.JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
				runtimeStackSimulation.Push(funcCallValue)
			} else {
				appendNode(statements.NewExpressionStatement(funcCallValue))
			}
		case OP_INVOKEVIRTUAL:
			classInfo := d.GetMethodFromPool(int(Convert2bytesToInt(opcode.Data)))
			methodName := classInfo.Member
			funcCallValue := values.NewFunctionCallExpression(nil, methodName, classInfo.JavaType.FunctionType()) // 不push到栈中
			for i := 0; i < len(funcCallValue.FuncType.ParamTypes); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(values.JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]values.JavaValue)
			funcCallValue.Object = runtimeStackSimulation.Pop().(values.JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != types.NewJavaPrimer(types.JavaVoid).String(funcCtx) {
				runtimeStackSimulation.Push(funcCallValue)
			} else {
				appendNode(statements.NewExpressionStatement(funcCallValue))
			}
		case OP_RETURN:
			appendNode(statements.NewReturnStatement(nil))
		case OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE:
			op := GetNotOp(opcode)
			rv := runtimeStackSimulation.Pop().(values.JavaValue)
			lv := runtimeStackSimulation.Pop().(values.JavaValue)
			st := statements.NewConditionStatement(values.NewJavaCompare(lv, rv), op)
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Id)
			//})
		case OP_IFNONNULL:
			st := statements.NewConditionStatement(values.NewJavaCompare(runtimeStackSimulation.Pop().(values.JavaValue), values.JavaNull), EQ)
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Id)
			//})
		case OP_IFNULL:
			st := statements.NewConditionStatement(values.NewJavaCompare(runtimeStackSimulation.Pop().(values.JavaValue), values.JavaNull), NEQ)
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Id)
			//})
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
			//newIfScope(opcodeIndex, int(jmpTo))
			v := runtimeStackSimulation.Pop()
			if v == nil {
				panic("not support")
			}
			cmp, ok := v.(values.JavaValue)
			if !ok {
				panic("not support")
			}
			st := statements.NewConditionStatement(values.NewJavaCompare(cmp, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))), op)
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Id)
			//})
		case OP_JSR, OP_JSR_W:
			return
			panic("not support")
		case OP_RET:
			return
			panic("not support")
		case OP_GOTO, OP_GOTO_W:
			st := statements.NewGOTOStatement()
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Target[0].Id)
			//})
		case OP_ATHROW:
			val := runtimeStackSimulation.Pop().(values.JavaValue)
			appendNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("throw %v", val.String(funcCtx))
			}))
		case OP_IRETURN:
			v := runtimeStackSimulation.Pop().(values.JavaValue)
			v.Type().ResetType(funcCtx.FunctionType.(*types.JavaFuncType).ReturnType)
			appendNode(statements.NewReturnStatement(v))
		case OP_ARETURN, OP_LRETURN, OP_DRETURN, OP_FRETURN:
			v := runtimeStackSimulation.Pop().(values.JavaValue)
			v.Type().ResetType(funcCtx.FunctionType.(*types.JavaFuncType).ReturnType)
			appendNode(statements.NewReturnStatement(v))
		case OP_GETFIELD:
			index := Convert2bytesToInt(opcode.Data)
			member := d.constantPoolGetter(int(index)).(*values.JavaClassMember)
			v := runtimeStackSimulation.Pop().(values.JavaValue)
			//if v1, ok := v.(*values.JavaRef); !ok {
			//	print(v1)
			//	println(v.String(d.FunctionContext))
			//}
			runtimeStackSimulation.Push(values.NewRefMember(v, member.Member, member.JavaType))
		case OP_GETSTATIC:
			index := Convert2bytesToInt(opcode.Data)
			runtimeStackSimulation.Push(d.constantPoolGetter(int(index)))
		case OP_PUTSTATIC:
			index := Convert2bytesToInt(opcode.Data)
			staticVal := d.constantPoolGetter(int(index))
			appendNode(statements.NewAssignStatement(staticVal, runtimeStackSimulation.Pop().(values.JavaValue), false))
		case OP_PUTFIELD:
			index := Convert2bytesToInt(opcode.Data)
			staticVal := d.constantPoolGetter(int(index))
			value := runtimeStackSimulation.Pop().(values.JavaValue)
			field := values.NewRefMember(runtimeStackSimulation.Pop().(values.JavaValue), staticVal.(*values.JavaClassMember).Member, staticVal.(*values.JavaClassMember).JavaType)
			appendNode(statements.NewAssignStatement(field, value, false))
		case OP_SWAP:
			v1 := runtimeStackSimulation.Pop()
			v2 := runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(v1)
			runtimeStackSimulation.Push(v2)
		case OP_DUP:
			checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
			runtimeStackSimulation.Push(runtimeStackSimulation.Peek())
		case OP_DUP_X1:
			checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
			v1 := runtimeStackSimulation.Pop()
			v2 := runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(v1)
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
		case OP_DUP_X2:
			checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
			v1 := runtimeStackSimulation.Pop()
			runtimeStackSimulationPopN := func(n int) []any {
				datas := []any{}
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
			}
			runtimeStackSimulation.Push(v1)
		case OP_DUP2:
			runtimeStackSimulationPopN := func(n int) []any {
				datas := []any{}
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
			runtimeStackSimulationPushReverse := func(datas []any) {
				for i := len(datas) - 1; i >= 0; i-- {
					runtimeStackSimulation.Push(datas[i])
				}
			}
			datas := runtimeStackSimulationPopN(2)
			runtimeStackSimulationPushReverse(datas)
			runtimeStackSimulationPushReverse(datas)
		case OP_DUP2_X1:
			runtimeStackSimulationPopN := func(n int) []any {
				datas := []any{}
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
			runtimeStackSimulationPushReverse := func(datas []any) {
				for i := len(datas) - 1; i >= 0; i-- {
					runtimeStackSimulation.Push(datas[i])
				}
			}
			datas := runtimeStackSimulationPopN(2)
			v1 := runtimeStackSimulation.Pop()
			runtimeStackSimulationPushReverse(datas)
			runtimeStackSimulation.Push(v1)
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
			runtimeStackSimulationPopN := func(n int) []any {
				datas := []any{}
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
			runtimeStackSimulationPushReverse := func(datas []any) {
				for i := len(datas) - 1; i >= 0; i-- {
					runtimeStackSimulation.Push(datas[i])
				}
			}
			datas1 := runtimeStackSimulationPopN(2)
			datas2 := runtimeStackSimulationPopN(2)
			runtimeStackSimulationPushReverse(datas1)
			runtimeStackSimulationPushReverse(datas2)
			runtimeStackSimulationPushReverse(datas1)
			//checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
			//v1 := runtimeStackSimulation.Pop()
			//checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
			//v2 := runtimeStackSimulation.Pop()
			//checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
			//v3 := runtimeStackSimulation.Pop()
			//checkAndConvertRef(runtimeStackSimulation.Peek().(values.JavaValue))
			//v4 := runtimeStackSimulation.Pop()
			//runtimeStackSimulation.Push(v2)
			//runtimeStackSimulation.Push(v1)
			//runtimeStackSimulation.Push(v4)
			//runtimeStackSimulation.Push(v3)
			//runtimeStackSimulation.Push(v2)
			//runtimeStackSimulation.Push(v1)
		case OP_LDC:
			runtimeStackSimulation.Push(d.ConstantPoolLiteralGetter(int(opcode.Data[0])))
		case OP_LDC_W:
			runtimeStackSimulation.Push(d.ConstantPoolLiteralGetter(int(Convert2bytesToInt(opcode.Data))))
		case OP_LDC2_W:
			v := d.ConstantPoolLiteralGetter(int(Convert2bytesToInt(opcode.Data)))
			runtimeStackSimulation.Push(v)
		case OP_MONITORENTER:
			v := runtimeStackSimulation.Pop().(values.JavaValue)
			st := statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("synchronized (%s)", v.String(funcCtx))
			})
			st.Name = "monitor_enter"
			st.Info = v
			appendNode(st)
		case OP_MONITOREXIT:
			st := statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
				return ""
			})
			st.Name = "monitor_exit"
			appendNode(st)
		case OP_NOP:
			return
		case OP_POP:
			appendNode(statements.NewExpressionStatement(runtimeStackSimulation.Pop().(values.JavaValue)))
		case OP_POP2:
			val := runtimeStackSimulation.Peek().(values.JavaValue)
			if GetTypeSize(val.Type()) == 1 {
				appendNode(statements.NewExpressionStatement(runtimeStackSimulation.Pop().(values.JavaValue)))
				appendNode(statements.NewExpressionStatement(runtimeStackSimulation.Pop().(values.JavaValue)))
			} else {
				appendNode(statements.NewExpressionStatement(runtimeStackSimulation.Pop().(values.JavaValue)))
			}
		case OP_TABLESWITCH, OP_LOOKUPSWITCH:
			switchMap := map[int]int{}
			for k, v := range opcode.SwitchJmpCase {
				switchMap[k] = d.offsetToOpcodeIndex[uint16(v)]
			}
			switchStatement := statements.NewMiddleStatement(statements.MiddleSwitch, []any{switchMap, runtimeStackSimulation.Pop().(values.JavaValue)})
			appendNode(switchStatement)

			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	for k, v := range switchMap {
			//		switchMap[k] = getter(v)
			//	}
			//})
		case OP_IINC:
			var index, inc int
			if opcode.IsWide {
				index = int(Convert2bytesToInt(opcode.Data))
				inc = int(Convert2bytesToInt(opcode.Data[2:]))
			} else {
				index = int(opcode.Data[0])
				inc = int(opcode.Data[1])
			}
			ref := d.GetVar(int(index))
			appendNode(values.NewBinaryExpression(ref, values.NewJavaLiteral(inc, types.NewJavaPrimer(types.JavaInteger)), INC, ref.Type()))
		case OP_DNEG, OP_FNEG, OP_LNEG, OP_INEG:
			v := runtimeStackSimulation.Pop().(values.JavaValue)
			runtimeStackSimulation.Push(values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return fmt.Sprintf("-%s", v.String(funcCtx))
			}, func() types.JavaType {
				return v.Type()
			}))
		case OP_END:
			endNode := statements.NewMiddleStatement("end", nil)
			appendNode(endNode)
		case OP_START:
			endNode := statements.NewMiddleStatement("start", nil)
			appendNode(endNode)
		default:
			panic("not support")
		}
	}
	opcodeToVarTable := map[*OpCode][]any{}
	opcodeToVarTable[d.opCodes[0]] = []any{d.varTable, d.currentVarId}
	err = WalkGraph[*OpCode](d.opCodes[0], func(code *OpCode) ([]*OpCode, error) {
		var initN int
		if len(code.Source) == 0 {
			mapCodeToStackVarIndex[code] = 0
		} else {
			source := code.Source[0]
			initN = mapCodeToStackVarIndex[source]
			//popL := len(source.Instr.StackPopped)
			pushL := len(source.Instr.StackPushed)
			initN = initN + pushL
			mapCodeToStackVarIndex[code] = initN
		}
		return code.Target, nil
	})
	if err != nil {
		return err
	}
	err = WalkGraph[*OpCode](d.opCodes[0], func(node *OpCode) ([]*OpCode, error) {
		var validSource *OpCode
		if len(node.Source) != 0 {
			for _, s := range node.Source {
				if _, ok := opcodeToVarTable[s]; ok {
					validSource = s
					break
				}
			}
			if validSource != nil {
				d.varTable = opcodeToVarTable[validSource][0].(map[int]*values.JavaRef)
				d.currentVarId = opcodeToVarTable[validSource][1].(int)
				if len(validSource.Target) > 1 {
					newMap := map[int]*values.JavaRef{}
					maps.Copy(newMap, d.varTable)
					d.varTable = newMap
				}
			}
		}
		if node.Id == 291 {
			print()
		}
		if node.IsCatch {
			typ := d.GetValueFromPool(int(node.ExceptionTypeIndex))
			runtimeStackSimulation.Push(values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
				return "Exception"
			}, func() types.JavaType {
				return typ.Type()
			}))
		}
		parseOpcode(node)
		opcodeToVarTable[node] = []any{d.varTable, d.currentVarId}
		return node.Target, nil
	})
	if err != nil {
		return err
	}
	//println(DumpOpcodesToDotExp(d.opCodes[0]))
	//println(DumpOpcodesToDotExp(d.opCodes[0]))
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
		if v, ok := node.Statement.(*statements.MiddleStatement); ok && v.Flag == statements.MiddleSwitch {
			data := v.Data.([]any)
			m := data[0].(map[int]int)
			newM := map[int]*Node{}
			for k, v := range m {
				newM[k] = idToNode[getStatementNextIdByOpcodeId(v)]
			}
			data[0] = newM
			v.Data = data
		}

		opcode := idToOpcode[node.Id]
		for _, code := range opcode.Target {
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
	//for _, f := range statementHandle {
	//	f(func(id int) (toStatement int) {
	//		toStatement = getStatementNextIdByOpcodeId(id)
	//		return
	//	})
	//}
	d.RootNode = nodes[0]
	err = d.StandardStatement()
	if err != nil {
		return err
	}
	WalkGraph[*Node](d.RootNode, func(node *Node) ([]*Node, error) {
		if len(node.Next) > 1 {
			if _, ok := node.Statement.(*statements.ConditionStatement); ok {
				return node.Next, nil
			}
			if v, ok := node.Statement.(*statements.MiddleStatement); ok {
				if v.Flag == statements.MiddleSwitch || v.Flag == "tryStart" {
					return node.Next, nil
				}
			}
			tryStartNode := node.Next[0]
			catchStartNode := node.Next[1]
			tryNode := NewNode(statements.NewMiddleStatement("tryStart", nil))
			node.RemoveAllNext()
			node.AddNext(tryNode)
			tryNode.AddNext(tryStartNode)
			tryNode.AddNext(catchStartNode)
			source := funk.Filter(tryStartNode.Source, func(item *Node) bool {
				return item != tryNode
			}).([]*Node)
			for _, n := range source {
				tryStartNode.RemoveSource(n)
			}
			for _, n := range source {
				tryNode.AddSource(n)
			}
		}
		return node.Next, nil
	})
	err = d.ReGenerateNodeId()
	if err != nil {
		return err
	}
	//err = d.ScanIfStatementInfo()
	//if err != nil {
	//	return err
	//}
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
func (d *Decompiler) StandardStatement() error {
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
				return node.Next[trueIndex]
			}
			node.FalseNode = func() *Node {
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
	return sb.String()
}
