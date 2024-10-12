package core

import (
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"maps"
	"sort"
)

type FunctionContext struct {
	ClassName    string
	FunctionName string
	PackageName  string
	BuildInLibs  []string
}
type Decompiler struct {
	FunctionContext               *FunctionContext
	varTable                      map[int]*JavaRef
	currentVarId                  int
	bytecodes                     []byte
	opCodes                       []*OpCode
	OpCodeRoot                    *OpCode
	RootNode                      *Node
	constantPoolGetter            func(id int) JavaValue
	ConstantPoolLiteralGetter     func(constantPoolGetterid int) *JavaLiteral
	ConstantPoolInvokeDynamicInfo func(id int) (string, string)
	offsetToOpcodeIndex           map[uint16]int
	opcodeIndexToOffset           map[int]uint16
	CurrentId                     int
}

func NewDecompiler(bytecodes []byte, constantPoolGetter func(id int) JavaValue) *Decompiler {
	return &Decompiler{
		FunctionContext:     &FunctionContext{},
		bytecodes:           bytecodes,
		constantPoolGetter:  constantPoolGetter,
		offsetToOpcodeIndex: map[uint16]int{},
		opcodeIndexToOffset: map[int]uint16{},
		varTable:            map[int]*JavaRef{},
	}
}

func (d *Decompiler) ParseOpcode() error {
	defer func() {
		if len(d.opCodes) > 0 {
			d.OpCodeRoot = d.opCodes[0]
		}
	}()
	opcodes := []*OpCode{}
	offsetToIndex := map[uint16]int{}
	indexToOffset := map[int]uint16{}
	reader := NewJavaByteCodeReader(d.bytecodes)
	id := 0
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
		opcode := &OpCode{Instr: instr, Id: id, CurrentOffset: uint16(current)}
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
	d.opCodes = opcodes
	d.CurrentId = id
	return d.ScanJmp()
}

func (d *Decompiler) ScanJmp() error {
	opcodes := d.opCodes
	visitNodeRecord := utils.NewSet[*OpCode]()
	endOp := &OpCode{Instr: InstrInfos[OP_END], Id: d.CurrentId}
	var walkNode func(start int)
	walkNode = func(start int) {
		var pre *OpCode
		i := start
		for {
			if i >= len(opcodes) {
				break
			}
			opcode := opcodes[i]
			if pre != nil {
				SetOpcode(pre, opcode)
			}
			if visitNodeRecord.Has(opcode) {
				break
			}
			visitNodeRecord.Add(opcode)
			pre = opcode
			if opcode.Id == 39 {
				print()
			}
			switch opcode.Instr.OpCode {
			case OP_RETURN:
				opcode.Target = []*OpCode{endOp}
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
func (d *Decompiler) getPoolValue(index int) JavaValue {
	return d.constantPoolGetter(index)
}
func (d *Decompiler) AssignVar(slot int, value JavaValue) (*JavaRef, bool) {
	typ := value.Type()
	ref, ok := d.varTable[slot]
	if !ok || ref.String(d.FunctionContext) != typ.String(d.FunctionContext) {
		d.currentVarId++
		newRef := NewJavaRef(d.currentVarId, typ)
		d.varTable[slot] = newRef
		return newRef, true
	}
	return ref, false
}
func (d *Decompiler) GetVar(slot int) *JavaRef {
	return d.varTable[slot]
}
func (d *Decompiler) ParseStatement() error {
	funcCtx := d.FunctionContext
	err := d.ParseOpcode()
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
	appendNode := func(statement Statement) *Node {
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

	getConstantPoolValue := func(opcode *OpCode) JavaValue {
		return d.getPoolValue(int(Convert2bytesToInt(opcode.Data)))
	}

	runtimeStackSimulation := utils.NewStack[any]()
	stackVarIndex := 0
	mapCodeToStackVarIndex := map[*OpCode]int{}
	assignStackVar := func(value JavaValue) {
		//appendNode(NewStackAssignStatement(stackVarIndex, value))
		ref := NewJavaRef(stackVarIndex, value.Type())
		ref.StackVar = value
		runtimeStackSimulation.Push(ref)
		stackVarIndex++
	}
	lambdaIndex := 0
	getLambdaIndex := func() int {
		defer func() { lambdaIndex++ }()
		return lambdaIndex
	}
	parseOpcode := func(opcode *OpCode) {
		//opcodeIndex := opcode.Id
		statementsIndex = opcode.Id
		stackVarIndex = mapCodeToStackVarIndex[opcode]
		switch opcode.Instr.OpCode {
		case OP_ALOAD, OP_ILOAD, OP_LLOAD, OP_DLOAD, OP_FLOAD, OP_ALOAD_0, OP_ILOAD_0, OP_LLOAD_0, OP_DLOAD_0, OP_FLOAD_0, OP_ALOAD_1, OP_ILOAD_1, OP_LLOAD_1, OP_DLOAD_1, OP_FLOAD_1, OP_ALOAD_2, OP_ILOAD_2, OP_LLOAD_2, OP_DLOAD_2, OP_FLOAD_2, OP_ALOAD_3, OP_ILOAD_3, OP_LLOAD_3, OP_DLOAD_3, OP_FLOAD_3:
			//varTable = append(varTable, runtimeStackSimulation.Pop())
			slot := GetRetrieveIdx(opcode)
			runtimeStackSimulation.Push(d.GetVar(slot))
			////return mkRetrieve(variableFactory);
		case OP_ACONST_NULL:
			assignStackVar(NewJavaLiteral(nil, JavaNull))
		case OP_ICONST_M1:
			assignStackVar(NewJavaLiteral(-1, JavaInteger))
		case OP_ICONST_0:
			assignStackVar(NewJavaLiteral(0, JavaInteger))
		case OP_ICONST_1:
			assignStackVar(NewJavaLiteral(1, JavaInteger))
		case OP_ICONST_2:
			assignStackVar(NewJavaLiteral(2, JavaInteger))
		case OP_ICONST_3:
			assignStackVar(NewJavaLiteral(3, JavaInteger))
		case OP_ICONST_4:
			assignStackVar(NewJavaLiteral(4, JavaInteger))
		case OP_ICONST_5:
			assignStackVar(NewJavaLiteral(5, JavaInteger))
		case OP_LCONST_0:
			assignStackVar(NewJavaLiteral(int64(0), JavaLong))
		case OP_LCONST_1:
			assignStackVar(NewJavaLiteral(int64(1), JavaLong))
		case OP_FCONST_0:
			assignStackVar(NewJavaLiteral(float32(0), JavaFloat))
		case OP_FCONST_1:
			assignStackVar(NewJavaLiteral(float32(1), JavaFloat))
		case OP_FCONST_2:
			assignStackVar(NewJavaLiteral(float32(2), JavaFloat))
		case OP_DCONST_0:
			assignStackVar(NewJavaLiteral(float64(0), JavaDouble))
		case OP_DCONST_1:
			assignStackVar(NewJavaLiteral(float64(1), JavaDouble))
		case OP_BIPUSH:
			assignStackVar(NewJavaLiteral(opcode.Data[0], JavaInteger))
		case OP_SIPUSH:
			assignStackVar(NewJavaLiteral(Convert2bytesToInt(opcode.Data), JavaInteger))
		case OP_ISTORE, OP_ASTORE, OP_LSTORE, OP_DSTORE, OP_FSTORE, OP_ISTORE_0, OP_ASTORE_0, OP_LSTORE_0, OP_DSTORE_0, OP_FSTORE_0, OP_ISTORE_1, OP_ASTORE_1, OP_LSTORE_1, OP_DSTORE_1, OP_FSTORE_1, OP_ISTORE_2, OP_ASTORE_2, OP_LSTORE_2, OP_DSTORE_2, OP_FSTORE_2, OP_ISTORE_3, OP_ASTORE_3, OP_LSTORE_3, OP_DSTORE_3, OP_FSTORE_3:
			slot := GetStoreIdx(opcode)
			value := runtimeStackSimulation.Pop().(JavaValue)
			ref, isFirst := d.AssignVar(slot, value)
			appendNode(NewAssignStatement(ref, value, isFirst))
		case OP_NEW:
			n := Convert2bytesToInt(opcode.Data)
			javaClass := d.constantPoolGetter(int(n)).(*JavaClass)
			//runtimeStackSimulation.Push(javaClass)
			runtimeStackSimulation.Push(NewNewExpression(javaClass))
			//appendNode()
		case OP_NEWARRAY:
			length := runtimeStackSimulation.Pop().(JavaValue)
			primerTypeName := GetPrimerArrayType(int(opcode.Data[0]))
			runtimeStackSimulation.Push(NewNewArrayExpression(NewJavaArrayType(primerTypeName, length)))
		case OP_ANEWARRAY:
			value := getConstantPoolValue(opcode)
			length := runtimeStackSimulation.Pop().(JavaValue)
			arrayType := NewJavaArrayType(value.(*JavaClass), length)
			exp := NewNewArrayExpression(arrayType)
			runtimeStackSimulation.Push(exp)
		case OP_MULTIANEWARRAY:
			desc := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data[:2]))).(*JavaClass).Name
			dimensions := int(opcode.Data[2])
			var lens []JavaValue
			for _, d := range runtimeStackSimulation.PopN(dimensions) {
				lens = append(lens, d.(JavaValue))
			}
			lens = funk.Reverse(lens).([]JavaValue)
			typ, rest, err := parseType(desc)
			if err != nil || rest != "" {
				log.Errorf("parse type `%s` error: %s", desc, err)
			}
			typ.(*JavaArrayType).Length = lens
			exp := NewNewArrayExpression(typ)
			runtimeStackSimulation.Push(exp)
		case OP_ARRAYLENGTH:
			ref := runtimeStackSimulation.Pop().(*JavaRef)
			runtimeStackSimulation.Push(NewRefMember(ref.Id, "length", JavaInteger))
		case OP_AALOAD, OP_IALOAD, OP_BALOAD, OP_CALOAD, OP_FALOAD, OP_LALOAD, OP_DALOAD, OP_SALOAD:
			index := runtimeStackSimulation.Pop().(JavaValue)
			ref := runtimeStackSimulation.Pop().(*JavaRef)
			runtimeStackSimulation.Push(NewJavaArrayMember(ref, index))
		case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
			value := runtimeStackSimulation.Pop().(JavaValue)
			index := runtimeStackSimulation.Pop().(JavaValue)
			ref := runtimeStackSimulation.Pop().(*JavaRef)
			appendNode(NewArrayMemberAssignStatement(NewJavaArrayMember(ref, index), value))
		case OP_LCMP, OP_DCMPG, OP_DCMPL, OP_FCMPG, OP_FCMPL:
			var1 := runtimeStackSimulation.Pop().(JavaValue)
			var2 := runtimeStackSimulation.Pop().(JavaValue)
			runtimeStackSimulation.Push(NewBinaryExpression(var1, var2, "compare"))
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
			var2 := runtimeStackSimulation.Pop().(JavaValue)
			var1 := runtimeStackSimulation.Pop().(JavaValue)
			runtimeStackSimulation.Push(NewBinaryExpression(var1, var2, op))
		case OP_I2B, OP_I2C, OP_I2D, OP_I2F, OP_I2L, OP_I2S, OP_L2D, OP_L2F, OP_L2I, OP_F2D, OP_F2I, OP_F2L, OP_D2F, OP_D2I, OP_D2L:
			var fname string
			var typ JavaType
			switch opcode.Instr.OpCode {
			case OP_I2B:
				fname = TypeCaseByte
				typ = JavaByte
			case OP_I2C:
				fname = TypeCaseChar
				typ = JavaChar
			case OP_I2D:
				fname = TypeCaseDouble
				typ = JavaDouble
			case OP_I2F:
				fname = TypeCaseFloat
				typ = JavaFloat
			case OP_I2L:
				fname = TypeCaseLong
				typ = JavaLong
			case OP_I2S:
				fname = TypeCaseShort
				typ = JavaShort
			case OP_L2D:
				fname = TypeCaseDouble
				typ = JavaDouble
			case OP_L2F:
				fname = TypeCaseFloat
				typ = JavaFloat
			case OP_L2I:
				fname = TypeCaseInt
				typ = JavaInteger
			case OP_F2D:
				fname = TypeCaseDouble
				typ = JavaDouble
			case OP_F2I:
				fname = TypeCaseInt
				typ = JavaInteger
			case OP_F2L:
				fname = TypeCaseLong
				typ = JavaLong
			case OP_D2F:
				fname = TypeCaseFloat
				typ = JavaFloat
			case OP_D2I:
				fname = TypeCaseInt
				typ = JavaInteger
			case OP_D2L:
				fname = TypeCaseLong
				typ = JavaLong
			}
			arg := runtimeStackSimulation.Pop().(JavaValue)
			runtimeStackSimulation.Push(NewCustomValue(func(funcCtx *FunctionContext) string {
				return fmt.Sprintf("(%s)%s", fname, arg.String(funcCtx))
			}, func() JavaType {
				return typ
			}))
		case OP_INSTANCEOF:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClass)
			value := runtimeStackSimulation.Pop().(JavaValue)
			runtimeStackSimulation.Push(NewCustomValue(func(funcCtx *FunctionContext) string {
				return fmt.Sprintf("%s instanceof %s", value.String(funcCtx), classInfo.String(funcCtx))
			}, func() JavaType {
				return JavaBoolean
			}))
		case OP_CHECKCAST:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClass)
			arg := runtimeStackSimulation.Pop().(JavaValue)
			runtimeStackSimulation.Push(NewCustomValue(func(funcCtx *FunctionContext) string {
				return fmt.Sprintf("(%s)(%s)", classInfo.String(funcCtx), arg.String(funcCtx))
			}, func() JavaType {
				return classInfo
			}))
		case OP_INVOKESTATIC:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClassMember)
			methodName := classInfo.Member
			funcType, _, err := parseFuncType(classInfo.Description)
			if err != nil {
				panic(utils.Errorf("parseFuncType %s error:%v", classInfo.Description, err))
			}
			funcCallValue := NewFunctionCallExpression(nil, methodName, funcType) // 不push到栈中
			funcCallValue.JavaType = classInfo.JavaType
			funcCallValue.IsStatic = true
			for i := 0; i < len(funcCallValue.FuncType.Params); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != JavaVoid.String(funcCtx) {
				runtimeStackSimulation.Push(funcCallValue)
			}
		case OP_INVOKEDYNAMIC:
			_, desc := d.ConstantPoolInvokeDynamicInfo(int(Convert2bytesToInt(opcode.Data)))
			typ, _, err := parseFuncType(desc)
			if err != nil {
				panic(err)
				return
			}
			runtimeStackSimulation.Push(NewLambdaFuncRef(getLambdaIndex(), typ.ReturnType))
		case OP_INVOKESPECIAL:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClassMember)
			methodName := classInfo.Member
			funcType, _, err := parseFuncType(classInfo.Description)
			if err != nil {
				panic(utils.Errorf("parseFuncType %s error:%v", classInfo.Description, err))
			}
			funcCallValue := NewFunctionCallExpression(nil, methodName, funcType) // 不push到栈中
			for i := 0; i < len(funcCallValue.FuncType.Params); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]JavaValue)

			funcCallValue.Object = runtimeStackSimulation.Pop().(JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != JavaVoid.String(funcCtx) {
				runtimeStackSimulation.Push(funcCallValue)
			}
		case OP_INVOKEINTERFACE:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClassMember)
			methodName := classInfo.Member
			funcType, _, err := parseFuncType(classInfo.Description)
			if err != nil {
				panic(utils.Errorf("parseFuncType %s error:%v", classInfo.Description, err))
			}
			funcCallValue := NewFunctionCallExpression(nil, methodName, funcType) // 不push到栈中
			for i := 0; i < len(funcCallValue.FuncType.Params); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]JavaValue)
			funcCallValue.Object = runtimeStackSimulation.Pop().(JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != JavaVoid.String(funcCtx) {
				runtimeStackSimulation.Push(funcCallValue)
			} else {
				appendNode(NewExpressionStatement(funcCallValue))
			}
		case OP_INVOKEVIRTUAL:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClassMember)
			methodName := classInfo.Member
			funcType, _, err := parseFuncType(classInfo.Description)
			if err != nil {
				panic(utils.Errorf("parseFuncType %s error:%v", classInfo.Description, err))
			}
			funcCallValue := NewFunctionCallExpression(nil, methodName, funcType) // 不push到栈中
			for i := 0; i < len(funcCallValue.FuncType.Params); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStackSimulation.Pop().(JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]JavaValue)
			funcCallValue.Object = runtimeStackSimulation.Pop().(JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != JavaVoid.String(funcCtx) {
				runtimeStackSimulation.Push(funcCallValue)
			} else {
				appendNode(NewExpressionStatement(funcCallValue))
			}
		case OP_RETURN:
			appendNode(NewReturnStatement(nil))
		case OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE:
			op := GetNotOp(opcode)
			rv := runtimeStackSimulation.Pop().(JavaValue)
			lv := runtimeStackSimulation.Pop().(JavaValue)
			st := NewConditionStatement(NewJavaCompare(lv, rv), op)
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Id)
			//})
		case OP_IFNONNULL:
			st := NewConditionStatement(NewJavaCompare(runtimeStackSimulation.Pop().(JavaValue), JavaNull), EQ)
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Id)
			//})
		case OP_IFNULL:
			st := NewConditionStatement(NewJavaCompare(runtimeStackSimulation.Pop().(JavaValue), JavaNull), NEQ)
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
			//newIfScope(opcodeIndex, int(jmpTo))
			v := runtimeStackSimulation.Pop()
			if v == nil {
				panic("not support")
			}
			cmp, ok := v.(JavaValue)
			if !ok {
				panic("not support")
			}
			st := NewConditionStatement(cmp, op)
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Id)
			//})
		case OP_JSR, OP_JSR_W:
			panic("not support")
		case OP_RET:
			panic("not support")
		case OP_GOTO, OP_GOTO_W:
			st := NewGOTOStatement()
			appendNode(st)
			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	st.ToStatement = getter(opcode.Target[0].Id)
			//})
		case OP_ATHROW:
			val := runtimeStackSimulation.Pop().(JavaValue)
			appendNode(NewCustomStatement(func(funcCtx *FunctionContext) string {
				return fmt.Sprintf("throw %v", val.String(funcCtx))
			}))
		case OP_IRETURN:
			v := runtimeStackSimulation.Pop().(JavaValue)
			appendNode(NewReturnStatement(v))
		case OP_ARETURN, OP_LRETURN, OP_DRETURN, OP_FRETURN:
			v := runtimeStackSimulation.Pop().(JavaValue)
			appendNode(NewReturnStatement(v))
		case OP_GETFIELD:
			index := Convert2bytesToInt(opcode.Data)
			member := d.constantPoolGetter(int(index)).(*JavaClassMember)
			v := runtimeStackSimulation.Pop().(JavaValue)
			runtimeStackSimulation.Push(NewRefMember(v.(*JavaRef).Id, member.Member, member.JavaType))
		case OP_GETSTATIC:
			index := Convert2bytesToInt(opcode.Data)
			runtimeStackSimulation.Push(d.constantPoolGetter(int(index)))
		case OP_PUTSTATIC, OP_PUTFIELD:
			index := Convert2bytesToInt(opcode.Data)
			staticVal := d.constantPoolGetter(int(index))
			appendNode(NewAssignStatement(staticVal, runtimeStackSimulation.Pop().(JavaValue), false))
		case OP_SWAP:
			v1 := runtimeStackSimulation.Pop()
			v2 := runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(v1)
			runtimeStackSimulation.Push(v2)
		case OP_DUP:
			runtimeStackSimulation.Push(runtimeStackSimulation.Peek())
		case OP_DUP_X1:
			v1 := runtimeStackSimulation.Pop()
			v2 := runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(v1)
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
		case OP_DUP_X2:
			v1 := runtimeStackSimulation.Pop()
			v2 := runtimeStackSimulation.Pop()
			v3 := runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(v1)
			runtimeStackSimulation.Push(v3)
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
		case OP_DUP2:
			v1 := runtimeStackSimulation.Pop()
			v2 := runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
		case OP_DUP2_X1:
			v1 := runtimeStackSimulation.Pop()
			v2 := runtimeStackSimulation.Pop()
			v3 := runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
			runtimeStackSimulation.Push(v3)
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
		case OP_DUP2_X2:
			v1 := runtimeStackSimulation.Pop()
			v2 := runtimeStackSimulation.Pop()
			v3 := runtimeStackSimulation.Pop()
			v4 := runtimeStackSimulation.Pop()
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
			runtimeStackSimulation.Push(v4)
			runtimeStackSimulation.Push(v3)
			runtimeStackSimulation.Push(v2)
			runtimeStackSimulation.Push(v1)
		case OP_LDC:
			runtimeStackSimulation.Push(d.ConstantPoolLiteralGetter(int(opcode.Data[0])))
		case OP_LDC_W:
			runtimeStackSimulation.Push(d.ConstantPoolLiteralGetter(int(Convert2bytesToInt(opcode.Data))))
		case OP_LDC2_W:
			v := d.ConstantPoolLiteralGetter(int(Convert2bytesToInt(opcode.Data)))
			runtimeStackSimulation.Push(v)
		case OP_MONITORENTER:
			v := runtimeStackSimulation.Pop().(JavaValue)
			st := NewCustomStatement(func(funcCtx *FunctionContext) string {
				return fmt.Sprintf("synchronized (%s)", v.String(funcCtx))
			})
			st.Name = "monitor_enter"
			st.Info = v
			appendNode(st)
		case OP_MONITOREXIT:
			st := NewCustomStatement(func(funcCtx *FunctionContext) string {
				return ""
			})
			st.Name = "monitor_exit"
			appendNode(st)
		case OP_NOP:
			return
		case OP_POP:
			appendNode(NewExpressionStatement(runtimeStackSimulation.Pop().(JavaValue)))
		case OP_POP2:
			appendNode(NewExpressionStatement(runtimeStackSimulation.Pop().(JavaValue)))
			appendNode(NewExpressionStatement(runtimeStackSimulation.Pop().(JavaValue)))
		case OP_TABLESWITCH, OP_LOOKUPSWITCH:
			switchMap := map[int]int{}
			for k, v := range opcode.SwitchJmpCase {
				switchMap[k] = d.offsetToOpcodeIndex[uint16(v)]
			}
			switchStatement := NewMiddleStatement(MiddleSwitch, []any{switchMap, runtimeStackSimulation.Pop().(JavaValue)})
			appendNode(switchStatement)

			//addStatementHandle(func(getter func(id int) (toStatement int)) {
			//	for k, v := range switchMap {
			//		switchMap[k] = getter(v)
			//	}
			//})
		case OP_IINC:
			index := opcode.Data[0]
			inc := opcode.Data[1]
			ref := d.GetVar(int(index))
			appendNode(NewBinaryExpression(ref, NewJavaLiteral(inc, JavaInteger), INC))
		case OP_DNEG, OP_FNEG, OP_LNEG, OP_INEG:
			v := runtimeStackSimulation.Pop().(JavaValue)
			runtimeStackSimulation.Push(NewCustomValue(func(funcCtx *FunctionContext) string {
				return fmt.Sprintf("-%s", v.String(funcCtx))
			}, func() JavaType {
				return v.Type()
			}))
		case OP_END:
			return
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
			if node.Id == 4 {
				print()
			}
			for _, s := range node.Source {
				if _, ok := opcodeToVarTable[s]; ok {
					validSource = s
					break
				}
			}
			d.varTable = opcodeToVarTable[validSource][0].(map[int]*JavaRef)
			d.currentVarId = opcodeToVarTable[validSource][1].(int)
			if len(validSource.Target) > 1 {
				newMap := map[int]*JavaRef{}
				maps.Copy(newMap, d.varTable)
				d.varTable = newMap
			}
		}
		parseOpcode(node)
		opcodeToVarTable[node] = []any{d.varTable, d.currentVarId}
		return node.Target, nil
	})
	if err != nil {
		return err
	}
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
		if node.Id == 39 {
			print()
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
		for _, n := range node.Next {
			if _, ok := n.Statement.(*GOTOStatement); ok {
				gotoNext := n.Next[0]
				node.ReplaceNext(n, gotoNext)
				gotoNext.RemoveSource(n)
				gotoNext.AddSource(node)
				//n.RemoveAllNext()
				//n.RemoveAllSource()
			}
		}
		if _, ok := node.Statement.(*ConditionStatement); ok {
			if node.Id == 39 {
				print()
			}
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

type IfScope struct {
	stackDeep      int
	statementIndex int
	IfStart        int
	ElseStart      int
	IfEnd          int
	ElseEnd        int
}
type CodeBlock struct {
	source  []*CodeBlock
	next    []*CodeBlock
	opcodes []*OpCode
}

func NewCodeBlock(parent *CodeBlock) *CodeBlock {
	c := &CodeBlock{
		source: []*CodeBlock{parent},
	}
	parent.next = append(parent.next, c)
	return c
}
func (d *Decompiler) ParseSourceCode() error {
	err := d.ParseStatement()
	if err != nil {
		return err
	}
	return nil
}
