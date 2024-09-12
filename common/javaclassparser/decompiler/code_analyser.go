package decompiler

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
)

type FunctionContext struct {
	ClassName    string
	FunctionName string
	PackageName  string
	BuildInLibs  []string
}
type Decompiler struct {
	FunctionContext           *FunctionContext
	bytecodes                 []byte
	opCodes                   []*OpCode
	Statements                []Statement
	constantPoolGetter        func(id int) JavaValue
	ConstantPoolLiteralGetter func(id int) *JavaLiteral
	offsetToOpcodeIndex       map[uint16]int
	opcodeIndexToOffset       map[int]uint16
}

func NewDecompiler(bytecodes []byte, constantPoolGetter func(id int) JavaValue) *Decompiler {
	return &Decompiler{
		FunctionContext:     &FunctionContext{},
		bytecodes:           bytecodes,
		constantPoolGetter:  constantPoolGetter,
		offsetToOpcodeIndex: map[uint16]int{},
		opcodeIndexToOffset: map[int]uint16{},
	}
}

func (d *Decompiler) ParseOpcode() error {
	opcodes := []*OpCode{}
	offsetToIndex := map[uint16]int{}
	indexToOffset := map[int]uint16{}
	reader := bytes.NewReader(d.bytecodes)
	i := 0
	id := 0
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		instr, ok := InstrInfos[int(b)]
		if !ok {
			return fmt.Errorf("unknow op: %x", b)
		}
		opcode := &OpCode{Instr: instr, Data: make([]byte, instr.Length), Id: id}
		reader.Read(opcode.Data)
		opcodes = append(opcodes, opcode)
		offsetToIndex[uint16(i)] = len(opcodes) - 1
		indexToOffset[len(opcodes)-1] = uint16(i)
		i += instr.Length + 1
		id++
	}
	d.offsetToOpcodeIndex = offsetToIndex
	d.opcodeIndexToOffset = indexToOffset
	d.opCodes = opcodes
	return d.ScanJmp()
}

type Node struct {
	Statement Statement
	Id        int
	Source    []*Node
	Next      []*Node
}

func (n *Node) AddSource(node *Node) {
	n.Source = append(n.Source, node)
}
func (n *Node) AddNext(node *Node) {
	n.Next = append(n.Next, node)
}
func NewNode(statement Statement) *Node {
	return &Node{Statement: statement}
}
func (d *Decompiler) ScanJmp() error {
	opcodes := d.opCodes
	visitNodeRecord := utils.NewSet[*OpCode]()
	var walkNode func(start int)
	walkNode = func(start int) {
		var pre *OpCode
		deferStartOp := []int{}
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
			switch opcode.Instr.OpCode {
			case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE, OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE, OP_IFNONNULL, OP_IFNULL:
				gotoRaw := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+gotoRaw]
				SetOpcode(opcode, d.opCodes[gotoOp])
				deferStartOp = append(deferStartOp, gotoOp)
			case OP_GOTO:
				target := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+target]
				SetOpcode(opcode, d.opCodes[gotoOp])
				i = gotoOp - 1
			case OP_GOTO_W:
				target := Convert2bytesToInt(opcode.Data)
				gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[i]+target]
				SetOpcode(opcode, d.opCodes[gotoOp])
				i = gotoOp - 1
			}
			i++
		}
		for _, code := range deferStartOp {
			walkNode(code)
		}
	}
	walkNode(0)
	return nil
}
func (d *Decompiler) DropUnreachableOpcode() error {
	// DropUnreachableOpcode and nop
	visitNodeRecord := utils.NewSet[*OpCode]()
	WalkGraph[*OpCode](d.opCodes[0], func(code *OpCode) []*OpCode {
		visitNodeRecord.Add(code)
		target := []*OpCode{}
		for _, opCode := range code.Target {
			target = append(target, opCode)
		}
		return target
	})
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
func (d *Decompiler) ParseStatement1() error {
	funcCtx := d.FunctionContext
	err := d.ParseOpcode()
	if err != nil {
		return err
	}
	err = d.DropUnreachableOpcode()
	if err != nil {
		return err
	}
	nodes := []*Node{}
	statementsIndex := 0

	appendNode := func(statement Statement) *Node {
		node := NewNode(statement)
		node.Id = statementsIndex
		nodes = append(nodes, node)
		return node
	}
	getConstantPoolValue := func(opcode *OpCode) JavaValue {
		return d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data)))
	}
	currentVarId := 0
	varTable := map[int]int{}
	varTable[0] = 0
	varSlotTypeTable := map[int]JavaType{}
	runtimeStack := utils.NewStack[any]()
	runtimeStackStack := utils.NewStack[any]()
	runtimeStackStack.Push(runtimeStack)
	assignVar := func(index int, value JavaValue) bool {
		id, ok := varTable[index]
		if !ok {
			currentVarId++
			varTable[index] = currentVarId
			varSlotTypeTable[currentVarId] = value.Type()
			return true
		} else {
			if varSlotTypeTable[id].String(funcCtx) != value.Type().String(funcCtx) {
				currentVarId++
				varTable[index] = currentVarId
				varSlotTypeTable[currentVarId] = value.Type()
			}
		}
		return false
	}
	stackVarIndex := 0
	mapCodeToStackVarIndex := map[*OpCode]int{}
	assignStackVar := func(value JavaValue) {
		appendNode(NewStackAssignStatement(stackVarIndex, value))
		ref := NewJavaRef(stackVarIndex, value.Type())
		ref.StackVar = value
		runtimeStack.Push(ref)
		stackVarIndex++
	}

	parseOpcode := func(opcode *OpCode) {
		//opcodeIndex := opcode.Id
		statementsIndex = opcode.Id
		stackVarIndex = mapCodeToStackVarIndex[opcode]
		switch opcode.Instr.OpCode {
		case OP_ALOAD, OP_ILOAD, OP_LLOAD, OP_DLOAD, OP_FLOAD, OP_ALOAD_0, OP_ILOAD_0, OP_LLOAD_0, OP_DLOAD_0, OP_FLOAD_0, OP_ALOAD_1, OP_ILOAD_1, OP_LLOAD_1, OP_DLOAD_1, OP_FLOAD_1, OP_ALOAD_2, OP_ILOAD_2, OP_LLOAD_2, OP_DLOAD_2, OP_FLOAD_2, OP_ALOAD_3, OP_ILOAD_3, OP_LLOAD_3, OP_DLOAD_3, OP_FLOAD_3:
			//varTable = append(varTable, runtimeStack.Pop())
			id := GetRetrieveIdx(opcode)
			slot := varTable[id]
			runtimeStack.Push(NewJavaRef(slot, varSlotTypeTable[slot]))
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
			id := GetStoreIdx(opcode)
			value := runtimeStack.Pop().(JavaValue)
			isFirst := assignVar(id, value)
			appendNode(NewAssignStatement(varTable[id], value, isFirst))
		case OP_NEW:
			n := Convert2bytesToInt(opcode.Data)
			javaClass := d.constantPoolGetter(int(n)).(*JavaClass)
			//runtimeStack.Push(javaClass)
			runtimeStack.Push(NewNewExpression(javaClass))
			//appendNode()
		case OP_NEWARRAY:
			length := runtimeStack.Pop().(JavaValue)
			primerTypeName := GetPrimerArrayType(int(opcode.Data[0]))
			runtimeStack.Push(NewNewArrayExpression(NewJavaArrayType(primerTypeName, length)))
		case OP_ANEWARRAY:
			value := getConstantPoolValue(opcode)
			length := runtimeStack.Pop().(JavaValue)
			arrayType := NewJavaArrayType(value.(*JavaClass), length)
			exp := NewNewArrayExpression(arrayType)
			runtimeStack.Push(exp)
		case OP_MULTIANEWARRAY:
			desc := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data[:2]))).(*JavaClass).Name
			dimensions := int(opcode.Data[2])
			var lens []JavaValue
			for _, d := range runtimeStack.PopN(dimensions) {
				lens = append(lens, d.(JavaValue))
			}
			lens = funk.Reverse(lens).([]JavaValue)
			typ, rest, err := parseType(desc)
			if err != nil || rest != "" {
				log.Errorf("parse type `%s` error: %s", desc, err)
			}
			typ.(*JavaArrayType).Length = lens
			exp := NewNewArrayExpression(typ)
			runtimeStack.Push(exp)
		case OP_ARRAYLENGTH:
			ref := runtimeStack.Pop().(*JavaRef)
			runtimeStack.Push(NewVirtualRefMember(ref.Id, "length", JavaInteger))
		case OP_AALOAD, OP_IALOAD, OP_BALOAD, OP_CALOAD, OP_FALOAD, OP_LALOAD, OP_DALOAD, OP_SALOAD:
			index := runtimeStack.Pop().(JavaValue)
			ref := runtimeStack.Pop().(*JavaRef)
			runtimeStack.Push(NewJavaArrayMember(ref, index))
		case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
			value := runtimeStack.Pop().(JavaValue)
			index := runtimeStack.Pop().(JavaValue)
			ref := runtimeStack.Pop().(*JavaRef)
			appendNode(NewArrayMemberAssignStatement(NewJavaArrayMember(ref, index), value))
		case OP_LCMP, OP_DCMPG, OP_DCMPL, OP_FCMPG, OP_FCMPL:
			var1 := runtimeStack.Pop().(JavaValue)
			var2 := runtimeStack.Pop().(JavaValue)
			runtimeStack.Push(NewBinaryExpression(var1, var2, "compare"))
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
			var2 := runtimeStack.Pop().(JavaValue)
			var1 := runtimeStack.Pop().(JavaValue)
			runtimeStack.Push(NewBinaryExpression(var1, var2, op))
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
			runtimeStack.Push(NewVirtualFunctionCall(fname, []JavaValue{runtimeStack.Pop().(JavaValue)}, typ))
		case OP_INSTANCEOF:
			panic("not support")
		case OP_CHECKCAST:
			panic("not support")
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
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStack.Pop().(JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != JavaVoid.String(funcCtx) {
				runtimeStack.Push(funcCallValue)
			}
		case OP_INVOKEDYNAMIC:
			panic("not support")
		case OP_INVOKESPECIAL:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClassMember)
			methodName := classInfo.Member
			funcType, _, err := parseFuncType(classInfo.Description)
			if err != nil {
				panic(utils.Errorf("parseFuncType %s error:%v", classInfo.Description, err))
			}
			funcCallValue := NewFunctionCallExpression(nil, methodName, funcType) // 不push到栈中
			for i := 0; i < len(funcCallValue.FuncType.Params); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStack.Pop().(JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]JavaValue)

			funcCallValue.Object = runtimeStack.Pop().(JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != JavaVoid.String(funcCtx) {
				runtimeStack.Push(funcCallValue)
			}
		case OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClassMember)
			methodName := classInfo.Member
			funcType, _, err := parseFuncType(classInfo.Description)
			if err != nil {
				panic(utils.Errorf("parseFuncType %s error:%v", classInfo.Description, err))
			}
			funcCallValue := NewFunctionCallExpression(nil, methodName, funcType) // 不push到栈中
			for i := 0; i < len(funcCallValue.FuncType.Params); i++ {
				funcCallValue.Arguments = append(funcCallValue.Arguments, runtimeStack.Pop().(JavaValue))
			}
			funcCallValue.Arguments = funk.Reverse(funcCallValue.Arguments).([]JavaValue)
			funcCallValue.Object = runtimeStack.Pop().(JavaValue)
			if funcCallValue.FuncType.ReturnType.String(funcCtx) != JavaVoid.String(funcCtx) {
				runtimeStack.Push(funcCallValue)
			} else {
				appendNode(NewExpressionStatement(funcCallValue))
			}

			//classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClassMember)
			//methodName := classInfo.Member
			////if methodName == "<init>" {
			////	runtimeStack.Push(NewFunctionCallExpression(runtimeStack.Pop().(JavaValue), methodName, nil))
			////	break
			////}
			//paramTypes, _, _ := ParseMethodDescriptor(classInfo.Description)
			//params := runtimeStack.PopN(len(paramTypes))
			//valuesParams := []JavaValue{}
			//for _, param := range params {
			//	valuesParams = append(valuesParams, param.(JavaValue))
			//}
			//ins := runtimeStack.Pop().(JavaValue)
			//runtimeStack.Push(NewFunctionCallExpression(ins, methodName, classInfo.Description, valuesParams))
		case OP_RETURN:
			appendNode(NewReturnStatement(nil))
		case OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE:
			op := GetNotOp(opcode)
			rv := runtimeStack.Pop().(JavaValue)
			lv := runtimeStack.Pop().(JavaValue)
			appendNode(NewConditionStatement(NewJavaCompare(lv, rv), op))
		case OP_IFNONNULL:
			panic("not support")
		case OP_IFNULL:
			panic("not support")
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
			v := runtimeStack.Pop()
			if v == nil {
				panic("not support")
			}
			cmp, ok := v.(JavaValue)
			if !ok {
				panic("not support")
			}
			appendNode(NewConditionStatement(cmp, op))
		case OP_JSR, OP_JSR_W:
			panic("not support")
		case OP_RET:
			panic("not support")
		case OP_GOTO:
			appendNode(NewGOTOStatement())
		case OP_GOTO_W:
			panic("not support")
		case OP_ATHROW:
			panic("not support")
		case OP_IRETURN:
			v := runtimeStack.Pop().(JavaValue)
			appendNode(NewReturnStatement(v))
		case OP_ARETURN:
			panic("not support")
		case OP_LRETURN:
			panic("not support")
		case OP_DRETURN:
			panic("not support")
		case OP_FRETURN:
			panic("not support")
		case OP_GETFIELD:
			panic("not support")
		case OP_GETSTATIC:
			index := Convert2bytesToInt(opcode.Data)
			runtimeStack.Push(d.constantPoolGetter(int(index)))
		case OP_PUTSTATIC:
			panic("not support")
		case OP_PUTFIELD:
			panic("not support")
		case OP_SWAP:
			panic("not support")
		case OP_DUP:
			runtimeStack.Push(runtimeStack.Peek())
		case OP_DUP_X1:
			panic("not support")
		case OP_DUP_X2:
			panic("not support")
		case OP_DUP2:
			panic("not support")
		case OP_DUP2_X1:
			panic("not support")
		case OP_DUP2_X2:
			panic("not support")
		case OP_LDC:
			runtimeStack.Push(d.ConstantPoolLiteralGetter(int(opcode.Data[0])))
		case OP_LDC_W:
			panic("not support")
		case OP_LDC2_W:
			v := d.ConstantPoolLiteralGetter(int(Convert2bytesToInt(opcode.Data)))
			runtimeStack.Push(v)
		case OP_MONITORENTER:
			panic("not support")
		case OP_MONITOREXIT:
			panic("not support")
		//case OP_FAKE_TRY:
		//	panic("not support")
		//case OP_FAKE_CATCH:
		//	panic("not support")
		case OP_NOP:
			panic("not support")
		case OP_POP:
			appendNode(NewExpressionStatement(runtimeStack.Pop().(JavaValue)))
		case OP_POP2:
			panic("not support")
		case OP_TABLESWITCH:
			panic("not support")
		case OP_LOOKUPSWITCH:
			panic("not support")
		case OP_IINC:
			index := opcode.Data[0]
			inc := opcode.Data[1]
			slot := int(index)
			appendNode(NewBinaryExpression(NewJavaRef(slot, varSlotTypeTable[slot]), NewJavaLiteral(inc, JavaInteger), INC))
		//case OP_IINC_WIDE:
		//	panic("not support")
		case OP_DNEG:
			panic("not support")
		case OP_FNEG:
			panic("not support")
		case OP_LNEG:
			panic("not support")
		case OP_INEG:
			panic("not support")
		default:
			panic("not support")
		}
	}
	WalkGraph[*OpCode](d.opCodes[0], func(code *OpCode) []*OpCode {
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
		return code.Target
	})
	WalkGraph[*OpCode](d.opCodes[0], func(node *OpCode) []*OpCode {
		parseOpcode(node)
		return node.Target
	})
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Id < nodes[j].Id
	})
	idToNode := map[int]*Node{}
	for _, node := range nodes {
		idToNode[node.Id] = node
	}
	getStatementIdByOpcodeId := func(id int) int {
		if v, ok := idToNode[id]; ok {
			return v.Id
		}
		idx := sort.Search(len(nodes), func(i int) bool {
			return nodes[i].Id > id
		})
		return nodes[idx].Id
	}
	idToOpcode := map[int]*OpCode{}
	for _, opcode := range d.opCodes {
		idToOpcode[opcode.Id] = opcode
	}
	for _, node := range nodes {
		switch ret := node.Statement.(type) {
		case *ConditionStatement:
			ret.ToStatement = getStatementIdByOpcodeId(idToOpcode[node.Id].Target[0].Id)
		case *GOTOStatement:
			ret.ToStatement = getStatementIdByOpcodeId(idToOpcode[node.Id].Target[0].Id)
		}
	}
	nodes, err = RewriteIf(nodes)
	if err != nil {
		return err
	}
	statements := []Statement{}
	for _, node := range nodes {
		statements = append(statements, node.Statement)
	}
	var filterNodes func(nodes []Statement) []Statement
	filterNodes = func(nodes []Statement) []Statement {
		return funk.Filter(nodes, func(item Statement) bool {
			switch ret := item.(type) {
			case *IfStatement:
				ret.IfBody = filterNodes(ret.IfBody)
				ret.ElseBody = filterNodes(ret.ElseBody)
			case *StackAssignStatement:
				_, ok := item.(*StackAssignStatement)
				return !ok
			}
			return true
		}).([]Statement)
	}
	statements = filterNodes(statements)
	//ShowStatementNodes(nodes)
	d.Statements = statements
	return nil
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
func (d *Decompiler) SplitCodeBlocks() error {
	ifStack := utils.NewStack[*IfScope]()
	newIfScope := func(ifStart, elseStart int) {
		ifStack.Push(&IfScope{
			IfStart:   ifStart,
			ElseStart: elseStart,
		})
	}
	rootBlock := &CodeBlock{}
	currentBlock := rootBlock
	_ = currentBlock
	for codeIndex, code := range d.opCodes {
		if ifStack.Len() > 0 {
			if codeIndex >= ifStack.Peek().ElseEnd {
				currentBlock.opcodes = append(currentBlock.opcodes, code)
				currentBlock = utils.GetLastElement(currentBlock.source)
			}
		}
		switch code.Instr.OpCode {
		case OP_IFEQ, OP_IFNE, OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE:
			newIfScope(codeIndex, int(Convert2bytesToInt(code.Data)))
			currentBlock.opcodes = append(currentBlock.opcodes, code)
			currentBlock = NewCodeBlock(currentBlock)
			continue
		case OP_IFNULL:
			newIfScope(codeIndex, int(Convert2bytesToInt(code.Data)))
		case OP_IFNONNULL:
			newIfScope(codeIndex, int(Convert2bytesToInt(code.Data)))
		case OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE:
			newIfScope(codeIndex, int(Convert2bytesToInt(code.Data)))
		case OP_GOTO_W:
			if ifStack.Len() > 0 {
				ifScope := ifStack.Peek()
				ifScope.IfEnd = codeIndex
				ifScope.ElseEnd = int(Convert4bytesToInt(code.Data))
			}
		case OP_GOTO:
			currentBlock.opcodes = append(currentBlock.opcodes, code)
			currentBlock = utils.GetLastElement(currentBlock.source)
			if ifStack.Len() > 0 {
				ifScope := ifStack.Peek()
				ifScope.IfEnd = codeIndex
				ifScope.ElseEnd = int(Convert2bytesToInt(code.Data))
			}
		default:
			currentBlock.opcodes = append(currentBlock.opcodes, code)
		}
	}
	return nil
}
func (d *Decompiler) ParseSourceCode() error {
	err := d.ParseStatement1()
	if err != nil {
		return err
	}
	return nil
}
func ParseBytesCode(constantPoolGetter func(id int) JavaValue, constantPoolLiteralGetter func(id int) *JavaLiteral, code []byte) ([]Statement, error) {
	decompiler := NewDecompiler(code, constantPoolGetter)
	decompiler.ConstantPoolLiteralGetter = constantPoolLiteralGetter
	err := decompiler.ParseSourceCode()
	if err != nil {
		return nil, err
	}
	return decompiler.Statements, nil
}
