package javaclassparser

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler"
	"github.com/yaklang/yaklang/common/utils"
)

func showOpcodes(codes []*decompiler.OpCode) {
	for i, opCode := range codes {
		if opCode.Instr.Name == "if_icmpge" || opCode.Instr.Name == "goto" {
			fmt.Printf("%d %s jmpto:%d\n", i, opCode.Instr.Name, opCode.Jmp)
		} else {
			fmt.Printf("%d %s %v\n", i, opCode.Instr.Name, opCode.Data)
		}
	}
}

func GetValueFromCP(pool []ConstantInfo, index int) decompiler.JavaValue {
	indexFromPool := func(i int) ConstantInfo {
		return pool[i-1]
	}
	constant := pool[index-1]
	switch ret := constant.(type) {
	case *ConstantFieldrefInfo:
		classInfo := indexFromPool(int(ret.ClassIndex)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)

		nameAndType := indexFromPool(int(ret.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		refNameInfo := indexFromPool(int(nameAndType.NameIndex)).(*ConstantUtf8Info)
		descInfo := indexFromPool(int(nameAndType.DescriptorIndex)).(*ConstantUtf8Info)
		classIns := decompiler.NewJavaClassMember(nameInfo.Value, refNameInfo.Value, descInfo.Value, decompiler.RT_RETURNADDRESS)
		return classIns
	case *ConstantMethodrefInfo:
		classInfo := indexFromPool(int(ret.ClassIndex)).(*ConstantClassInfo)
		nameInfo := indexFromPool(int(classInfo.NameIndex)).(*ConstantUtf8Info)

		nameAndType := indexFromPool(int(ret.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		refNameInfo := indexFromPool(int(nameAndType.NameIndex)).(*ConstantUtf8Info)
		descInfo := indexFromPool(int(nameAndType.DescriptorIndex)).(*ConstantUtf8Info)
		classIns := decompiler.NewJavaClassMember(nameInfo.Value, refNameInfo.Value, descInfo.Value, decompiler.RT_RETURNADDRESS)
		return classIns
	case *ConstantClassInfo:
		nameInfo := indexFromPool(int(ret.NameIndex)).(*ConstantUtf8Info)
		return decompiler.NewJavaClass(nameInfo.Value, decompiler.RT_RETURNADDRESS)
	default:
		panic("failed")
	}
}
func GetLiteralFromCP(pool []ConstantInfo, index int) *decompiler.JavaLiteral {
	constant := pool[index-1]
	switch ret := constant.(type) {
	case *ConstantStringInfo:
		return decompiler.NewJavaLiteral(pool[ret.StringIndex-1].(*ConstantUtf8Info).Value, decompiler.RT_REFERENCE)
	default:
		panic("failed")
	}
}

type VarMap struct {
	id  int
	val decompiler.JavaValue
}

func ParseBytesCode(dumper *ClassObjectDumper, codeAttr *CodeAttribute) (string, error) {
	code := ""
	code += "\n"
	opcodes := []*decompiler.OpCode{}
	offsetToIndex := map[uint16]int{}
	indexToOffset := map[int]uint16{}
	reader := bytes.NewReader(codeAttr.Code)
	i := 0
	getConstantPoolValue := func(opcode *decompiler.OpCode) decompiler.JavaValue {
		return GetValueFromCP(dumper.ConstantPool, int(decompiler.Convert2bytesToInt(opcode.Data)))
	}
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		instr, ok := decompiler.InstrInfos[int(b)]
		if !ok {
			return "", fmt.Errorf("unknow op: %x", b)
		}
		opcode := &decompiler.OpCode{Instr: instr, Data: make([]byte, instr.Length)}
		reader.Read(opcode.Data)
		opcodes = append(opcodes, opcode)
		offsetToIndex[uint16(i)] = len(opcodes) - 1
		indexToOffset[len(opcodes)-1] = uint16(i)
		i += instr.Length + 1
	}
	//for i, opcode := range opcodes {
	//
	//}
	statements := []decompiler.Statement{}
	statementsIndex := 0
	appendStatement := func(statement decompiler.Statement) {
		statementsIndex++
		statements = append(statements, statement)
	}
	_ = statements
	currentVarId := 0
	runtimeStack := utils.NewStack[any]()
	//runtimeStack.Push(decompiler.NewJavaLiteral(decompiler.Class, nil))
	varSlotTypeTable := map[int]decompiler.JavaType{}
	varTable := map[int]int{}
	assignVar := func(index int, value decompiler.JavaValue) {
		_, ok := varTable[index]
		if !ok {
			currentVarId++
			varTable[index] = currentVarId
		}
		//varTable[index] = currentVarId
		//currentVarId++
	}
	opIndexToStatementIndex := map[int]int{}
	varTable[0] = 0
	assignVar = func(index int, value decompiler.JavaValue) {
		varTable[index] = currentVarId
		currentVarId++
	}
	liveSlotPosition := map[int]int{}
	for opcodeIndex, opcode := range opcodes {
		switch opcode.Instr.OpCode {
		case decompiler.OP_ISTORE, decompiler.OP_ASTORE, decompiler.OP_LSTORE, decompiler.OP_DSTORE, decompiler.OP_FSTORE, decompiler.OP_ISTORE_0, decompiler.OP_ASTORE_0, decompiler.OP_LSTORE_0, decompiler.OP_DSTORE_0, decompiler.OP_FSTORE_0, decompiler.OP_ISTORE_1, decompiler.OP_ASTORE_1, decompiler.OP_LSTORE_1, decompiler.OP_DSTORE_1, decompiler.OP_FSTORE_1, decompiler.OP_ISTORE_2, decompiler.OP_ASTORE_2, decompiler.OP_LSTORE_2, decompiler.OP_DSTORE_2, decompiler.OP_FSTORE_2, decompiler.OP_ISTORE_3, decompiler.OP_ASTORE_3, decompiler.OP_LSTORE_3, decompiler.OP_DSTORE_3, decompiler.OP_FSTORE_3:
			id := decompiler.GetStoreIdx(opcode)
			assignVar(id, nil)
		case decompiler.OP_ALOAD, decompiler.OP_ILOAD, decompiler.OP_LLOAD, decompiler.OP_DLOAD, decompiler.OP_FLOAD, decompiler.OP_ALOAD_0, decompiler.OP_ILOAD_0, decompiler.OP_LLOAD_0, decompiler.OP_DLOAD_0, decompiler.OP_FLOAD_0, decompiler.OP_ALOAD_1, decompiler.OP_ILOAD_1, decompiler.OP_LLOAD_1, decompiler.OP_DLOAD_1, decompiler.OP_FLOAD_1, decompiler.OP_ALOAD_2, decompiler.OP_ILOAD_2, decompiler.OP_LLOAD_2, decompiler.OP_DLOAD_2, decompiler.OP_FLOAD_2, decompiler.OP_ALOAD_3, decompiler.OP_ILOAD_3, decompiler.OP_LLOAD_3, decompiler.OP_DLOAD_3, decompiler.OP_FLOAD_3:
			id := decompiler.GetRetrieveIdx(opcode)
			liveSlotPosition[id] = opcodeIndex
		}
	}
	varTable = map[int]int{}
	currentVarId = 0
	for opcodeIndex, opcode := range opcodes {
		assignVar = func(index int, value decompiler.JavaValue) {
			id, ok := varTable[index]
			if !ok {
				currentVarId++
				varTable[index] = currentVarId
				varSlotTypeTable[currentVarId] = value.Type()
			} else {
				//lastAlivePosition, ok := liveSlotPosition[id]
				//if !(!ok || opcodeIndex > lastAlivePosition) { // always die or current position is die
				//	currentVarId++
				//	varTable[index] = currentVarId
				//}
				if varSlotTypeTable[id] != value.Type() {
					currentVarId++
					varTable[index] = currentVarId
					varSlotTypeTable[currentVarId] = value.Type()
				}
			}
		}

		opIndexToStatementIndex[opcodeIndex] = statementsIndex
		switch opcode.Instr.OpCode {
		case decompiler.OP_ALOAD, decompiler.OP_ILOAD, decompiler.OP_LLOAD, decompiler.OP_DLOAD, decompiler.OP_FLOAD, decompiler.OP_ALOAD_0, decompiler.OP_ILOAD_0, decompiler.OP_LLOAD_0, decompiler.OP_DLOAD_0, decompiler.OP_FLOAD_0, decompiler.OP_ALOAD_1, decompiler.OP_ILOAD_1, decompiler.OP_LLOAD_1, decompiler.OP_DLOAD_1, decompiler.OP_FLOAD_1, decompiler.OP_ALOAD_2, decompiler.OP_ILOAD_2, decompiler.OP_LLOAD_2, decompiler.OP_DLOAD_2, decompiler.OP_FLOAD_2, decompiler.OP_ALOAD_3, decompiler.OP_ILOAD_3, decompiler.OP_LLOAD_3, decompiler.OP_DLOAD_3, decompiler.OP_FLOAD_3:
			//varTable = append(varTable, runtimeStack.Pop())
			id := decompiler.GetRetrieveIdx(opcode)
			slot := varTable[id]
			runtimeStack.Push(decompiler.NewJavaRef(slot, varSlotTypeTable[slot]))
			////return mkRetrieve(variableFactory);
			//panic("not support")
		case decompiler.OP_ACONST_NULL:
			runtimeStack.Push(decompiler.NewJavaLiteral(nil, decompiler.RT_NULL))
		case decompiler.OP_ICONST_M1:
			runtimeStack.Push(decompiler.NewJavaLiteral(-1, decompiler.RT_INT))
		case decompiler.OP_ICONST_0:
			runtimeStack.Push(decompiler.NewJavaLiteral(0, decompiler.RT_INT))
		case decompiler.OP_ICONST_1:
			runtimeStack.Push(decompiler.NewJavaLiteral(1, decompiler.RT_INT))
		case decompiler.OP_ICONST_2:
			runtimeStack.Push(decompiler.NewJavaLiteral(2, decompiler.RT_INT))
		case decompiler.OP_ICONST_3:
			runtimeStack.Push(decompiler.NewJavaLiteral(3, decompiler.RT_INT))
		case decompiler.OP_ICONST_4:
			runtimeStack.Push(decompiler.NewJavaLiteral(4, decompiler.RT_INT))
		case decompiler.OP_ICONST_5:
			runtimeStack.Push(decompiler.NewJavaLiteral(5, decompiler.RT_INT))
		case decompiler.OP_LCONST_0:
			runtimeStack.Push(decompiler.NewJavaLiteral(0, decompiler.RT_LONG))
		case decompiler.OP_LCONST_1:
			runtimeStack.Push(decompiler.NewJavaLiteral(1, decompiler.RT_LONG))
		case decompiler.OP_FCONST_0:
			runtimeStack.Push(decompiler.NewJavaLiteral(0, decompiler.RT_FLOAT))
		case decompiler.OP_FCONST_1:
			runtimeStack.Push(decompiler.NewJavaLiteral(1, decompiler.RT_FLOAT))
		case decompiler.OP_FCONST_2:
			runtimeStack.Push(decompiler.NewJavaLiteral(2, decompiler.RT_FLOAT))
		case decompiler.OP_DCONST_0:
			runtimeStack.Push(decompiler.NewJavaLiteral(0, decompiler.RT_DOUBLE))
		case decompiler.OP_DCONST_1:
			runtimeStack.Push(decompiler.NewJavaLiteral(1, decompiler.RT_DOUBLE))
		case decompiler.OP_BIPUSH:
			runtimeStack.Push(decompiler.NewJavaLiteral(opcode.Data[0], decompiler.RT_INT))
		case decompiler.OP_SIPUSH:
			runtimeStack.Push(decompiler.NewJavaLiteral(decompiler.Convert2bytesToInt(opcode.Data), decompiler.RT_INT))
		case decompiler.OP_ISTORE, decompiler.OP_ASTORE, decompiler.OP_LSTORE, decompiler.OP_DSTORE, decompiler.OP_FSTORE, decompiler.OP_ISTORE_0, decompiler.OP_ASTORE_0, decompiler.OP_LSTORE_0, decompiler.OP_DSTORE_0, decompiler.OP_FSTORE_0, decompiler.OP_ISTORE_1, decompiler.OP_ASTORE_1, decompiler.OP_LSTORE_1, decompiler.OP_DSTORE_1, decompiler.OP_FSTORE_1, decompiler.OP_ISTORE_2, decompiler.OP_ASTORE_2, decompiler.OP_LSTORE_2, decompiler.OP_DSTORE_2, decompiler.OP_FSTORE_2, decompiler.OP_ISTORE_3, decompiler.OP_ASTORE_3, decompiler.OP_LSTORE_3, decompiler.OP_DSTORE_3, decompiler.OP_FSTORE_3:
			id := decompiler.GetStoreIdx(opcode)
			value := runtimeStack.Pop().(decompiler.JavaValue)
			assignVar(id, value)
			appendStatement(decompiler.NewAssignStatement(varTable[id], value))
		case decompiler.OP_NEW:
			n := decompiler.Convert2bytesToInt(opcode.Data)
			javaClass := GetValueFromCP(dumper.ConstantPool, int(n)).(*decompiler.JavaClass)
			runtimeStack.Push(javaClass)
			appendStatement(decompiler.NewNewStatement(javaClass))
		case decompiler.OP_NEWARRAY:
			panic("not support")
		case decompiler.OP_ANEWARRAY:
			value := getConstantPoolValue(opcode)
			length := runtimeStack.Pop().(*decompiler.JavaLiteral).Data.(int)
			runtimeStack.Push(decompiler.NewJavaArray(value.(*decompiler.JavaClass), length))
		case decompiler.OP_MULTIANEWARRAY:
			panic("not support")
		case decompiler.OP_ARRAYLENGTH:
			panic("not support")
		case decompiler.OP_AALOAD, decompiler.OP_IALOAD, decompiler.OP_BALOAD, decompiler.OP_CALOAD, decompiler.OP_FALOAD, decompiler.OP_LALOAD, decompiler.OP_DALOAD, decompiler.OP_SALOAD:
			panic("not support")
		case decompiler.OP_AASTORE, decompiler.OP_IASTORE, decompiler.OP_BASTORE, decompiler.OP_CASTORE, decompiler.OP_FASTORE, decompiler.OP_LASTORE, decompiler.OP_DASTORE, decompiler.OP_SASTORE:
			panic("not support")
		case decompiler.OP_LCMP, decompiler.OP_DCMPG, decompiler.OP_DCMPL, decompiler.OP_FCMPG, decompiler.OP_FCMPL, decompiler.OP_LSUB, decompiler.OP_LADD, decompiler.OP_IADD, decompiler.OP_FADD, decompiler.OP_DADD, decompiler.OP_ISUB, decompiler.OP_DSUB, decompiler.OP_FSUB, decompiler.OP_IREM, decompiler.OP_FREM, decompiler.OP_LREM, decompiler.OP_DREM, decompiler.OP_IDIV, decompiler.OP_FDIV, decompiler.OP_DDIV, decompiler.OP_IMUL, decompiler.OP_DMUL, decompiler.OP_FMUL, decompiler.OP_LMUL, decompiler.OP_LAND, decompiler.OP_LDIV, decompiler.OP_LOR, decompiler.OP_LXOR, decompiler.OP_ISHR, decompiler.OP_ISHL, decompiler.OP_LSHL, decompiler.OP_LSHR, decompiler.OP_IUSHR, decompiler.OP_LUSHR:
			panic("not support")
		case decompiler.OP_IOR, decompiler.OP_IAND, decompiler.OP_IXOR:
			panic("not support")
		case decompiler.OP_I2B, decompiler.OP_I2C, decompiler.OP_I2D, decompiler.OP_I2F, decompiler.OP_I2L, decompiler.OP_I2S, decompiler.OP_L2D, decompiler.OP_L2F, decompiler.OP_L2I, decompiler.OP_F2D, decompiler.OP_F2I, decompiler.OP_F2L, decompiler.OP_D2F, decompiler.OP_D2I, decompiler.OP_D2L:
			panic("not support")
		case decompiler.OP_INSTANCEOF:
			panic("not support")
		case decompiler.OP_CHECKCAST:
			panic("not support")
		case decompiler.OP_INVOKESTATIC:
			panic("not support")
		case decompiler.OP_INVOKEDYNAMIC:
			panic("not support")
		case decompiler.OP_INVOKESPECIAL, decompiler.OP_INVOKEVIRTUAL, decompiler.OP_INVOKEINTERFACE:
			classInfo := GetValueFromCP(dumper.ConstantPool, int(decompiler.Convert2bytesToInt(opcode.Data))).(*decompiler.JavaClassMember)
			methodName := classInfo.Member
			if methodName == "<init>" {
				appendStatement(decompiler.NewFunctionCallStatement(runtimeStack.Pop().(decompiler.JavaValue), methodName, nil))
				break
			}
			paramTypes, _, _ := decompiler.ParseMethodDescriptor(classInfo.Description)
			params := runtimeStack.PopN(len(paramTypes))
			valuesParams := []decompiler.JavaValue{}
			for _, param := range params {
				valuesParams = append(valuesParams, param.(decompiler.JavaValue))
			}
			ins := runtimeStack.Pop().(decompiler.JavaValue)
			appendStatement(decompiler.NewFunctionCallStatement(ins, methodName, valuesParams))
		case decompiler.OP_RETURN:
			statements = append(statements, decompiler.NewReturnStatement(nil))
		case decompiler.OP_IF_ACMPEQ, decompiler.OP_IF_ACMPNE, decompiler.OP_IF_ICMPLT, decompiler.OP_IF_ICMPGE, decompiler.OP_IF_ICMPGT, decompiler.OP_IF_ICMPNE, decompiler.OP_IF_ICMPEQ, decompiler.OP_IF_ICMPLE:
			op := decompiler.GetOp(opcode)
			rv := runtimeStack.Pop().(decompiler.JavaValue)
			lv := runtimeStack.Pop().(decompiler.JavaValue)
			gotoRaw := decompiler.Convert2bytesToInt(opcode.Data)
			gotoOp := offsetToIndex[indexToOffset[opcodeIndex]+gotoRaw]
			appendStatement(decompiler.NewConditionStatement(lv, rv, op, gotoOp))
		case decompiler.OP_IFNONNULL:
			panic("not support")
		case decompiler.OP_IFNULL:
			panic("not support")
		case decompiler.OP_IFEQ, decompiler.OP_IFNE:
			panic("not support")
		case decompiler.OP_IFLE, decompiler.OP_IFLT, decompiler.OP_IFGT, decompiler.OP_IFGE:
			panic("not support")
		case decompiler.OP_JSR, decompiler.OP_JSR_W:
			panic("not support")
		case decompiler.OP_RET:
			panic("not support")
		case decompiler.OP_GOTO:
			target := decompiler.Convert2bytesToInt(opcode.Data)
			gotoOp := offsetToIndex[indexToOffset[opcodeIndex]+target]
			appendStatement(decompiler.NewGOTOStatement(int(gotoOp)))
		case decompiler.OP_GOTO_W:
			panic("not support")
		case decompiler.OP_ATHROW:
			panic("not support")
		case decompiler.OP_IRETURN:
			panic("not support")
		case decompiler.OP_ARETURN:
			panic("not support")
		case decompiler.OP_LRETURN:
			panic("not support")
		case decompiler.OP_DRETURN:
			panic("not support")
		case decompiler.OP_FRETURN:
			panic("not support")
		case decompiler.OP_GETFIELD:
			panic("not support")
		case decompiler.OP_GETSTATIC:
			index := decompiler.Convert2bytesToInt(opcode.Data)
			runtimeStack.Push(GetValueFromCP(dumper.ConstantPool, int(index)))
		case decompiler.OP_PUTSTATIC:
			panic("not support")
		case decompiler.OP_PUTFIELD:
			panic("not support")
		case decompiler.OP_SWAP:
			panic("not support")
		case decompiler.OP_DUP:
			runtimeStack.Push(runtimeStack.Peek())
		case decompiler.OP_DUP_X1:
			panic("not support")
		case decompiler.OP_DUP_X2:
			panic("not support")
		case decompiler.OP_DUP2:
			panic("not support")
		case decompiler.OP_DUP2_X1:
			panic("not support")
		case decompiler.OP_DUP2_X2:
			panic("not support")
		case decompiler.OP_LDC:
			runtimeStack.Push(GetLiteralFromCP(dumper.ConstantPool, int(opcode.Data[0])))
		case decompiler.OP_LDC_W:
			panic("not support")
		case decompiler.OP_LDC2_W:
			panic("not support")
		case decompiler.OP_MONITORENTER:
			panic("not support")
		case decompiler.OP_MONITOREXIT:
			panic("not support")
		//case decompiler.OP_FAKE_TRY:
		//	panic("not support")
		//case decompiler.OP_FAKE_CATCH:
		//	panic("not support")
		case decompiler.OP_NOP:
			panic("not support")
		case decompiler.OP_POP:
			runtimeStack.Pop()
		case decompiler.OP_POP2:
			panic("not support")
		case decompiler.OP_TABLESWITCH:
			panic("not support")
		case decompiler.OP_LOOKUPSWITCH:
			panic("not support")
		case decompiler.OP_IINC:
			index := opcode.Data[0]
			inc := opcode.Data[1]
			slot := int(index)
			appendStatement(decompiler.NewBinaryExpression(decompiler.NewJavaRef(slot, varSlotTypeTable[slot]), decompiler.NewJavaLiteral(inc, decompiler.RT_INT), decompiler.ADD))
		//case decompiler.OP_IINC_WIDE:
		//	panic("not support")
		case decompiler.OP_DNEG:
			panic("not support")
		case decompiler.OP_FNEG:
			panic("not support")
		case decompiler.OP_LNEG:
			panic("not support")
		case decompiler.OP_INEG:
			panic("not support")
		default:
			panic("not support")
		}
	}
	for _, statement := range statements {
		switch ret := statement.(type) {
		case *decompiler.ConditionStatement:
			ret.ToStatement = opIndexToStatementIndex[ret.ToOpcode]
		case *decompiler.GOTOStatement:
			ret.ToStatement = opIndexToStatementIndex[ret.ToOpcode]
		}
	}
	//showOpcodes(opcodes)
	//code += strings.Join(instrNameList, "\n")
	code += "\n"
	for i, statement := range statements {
		fmt.Printf("%d: %s\n", i, statement.String())
	}
	return code, nil
}
