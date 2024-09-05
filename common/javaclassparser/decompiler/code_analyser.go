package decompiler

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
)

type FunctionContext struct {
	ClassName    string
	FunctionName string
	PackageName  string
	BuildInLibs []string
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
	for {
		b, err := reader.ReadByte()
		if err != nil {
			break
		}
		instr, ok := InstrInfos[int(b)]
		if !ok {
			return fmt.Errorf("unknow op: %x", b)
		}
		opcode := &OpCode{Instr: instr, Data: make([]byte, instr.Length)}
		reader.Read(opcode.Data)
		opcodes = append(opcodes, opcode)
		offsetToIndex[uint16(i)] = len(opcodes) - 1
		indexToOffset[len(opcodes)-1] = uint16(i)
		i += instr.Length + 1
	}
	d.offsetToOpcodeIndex = offsetToIndex
	d.opcodeIndexToOffset = indexToOffset
	d.opCodes = opcodes
	return nil
}
func (d *Decompiler) ParseStatement1() error {
	funcCtx := d.FunctionContext
	err := d.ParseOpcode()
	if err != nil {
		return err
	}
	statements := []Statement{}
	statementsIndex := 0
	appendStatement := func(statement Statement) {
		statementsIndex++
		statements = append(statements, statement)
	}
	getConstantPoolValue := func(opcode *OpCode) JavaValue {
		return d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data)))
	}
	currentVarId := 0
	varTable := map[int]int{}
	varTable[0] = 0
	varSlotTypeTable := map[int]JavaType{}
	opIndexToStatementIndex := map[int]int{}
	runtimeStack := utils.NewStack[any]()
	for opcodeIndex, opcode := range d.opCodes {
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

		opIndexToStatementIndex[opcodeIndex] = statementsIndex
		switch opcode.Instr.OpCode {
		case OP_ALOAD, OP_ILOAD, OP_LLOAD, OP_DLOAD, OP_FLOAD, OP_ALOAD_0, OP_ILOAD_0, OP_LLOAD_0, OP_DLOAD_0, OP_FLOAD_0, OP_ALOAD_1, OP_ILOAD_1, OP_LLOAD_1, OP_DLOAD_1, OP_FLOAD_1, OP_ALOAD_2, OP_ILOAD_2, OP_LLOAD_2, OP_DLOAD_2, OP_FLOAD_2, OP_ALOAD_3, OP_ILOAD_3, OP_LLOAD_3, OP_DLOAD_3, OP_FLOAD_3:
			//varTable = append(varTable, runtimeStack.Pop())
			id := GetRetrieveIdx(opcode)
			slot := varTable[id]
			runtimeStack.Push(NewJavaRef(slot, varSlotTypeTable[slot]))
			////return mkRetrieve(variableFactory);
			//panic("not support")
		case OP_ACONST_NULL:
			runtimeStack.Push(NewJavaLiteral(nil, JavaNull{}))
		case OP_ICONST_M1:
			runtimeStack.Push(NewJavaLiteral(-1, JavaInteger))
		case OP_ICONST_0:
			runtimeStack.Push(NewJavaLiteral(0, JavaInteger))
		case OP_ICONST_1:
			runtimeStack.Push(NewJavaLiteral(1, JavaInteger))
		case OP_ICONST_2:
			runtimeStack.Push(NewJavaLiteral(2, JavaInteger))
		case OP_ICONST_3:
			runtimeStack.Push(NewJavaLiteral(3, JavaInteger))
		case OP_ICONST_4:
			runtimeStack.Push(NewJavaLiteral(4, JavaInteger))
		case OP_ICONST_5:
			runtimeStack.Push(NewJavaLiteral(5, JavaInteger))
		case OP_LCONST_0:
			runtimeStack.Push(NewJavaLiteral(0, JavaLong))
		case OP_LCONST_1:
			runtimeStack.Push(NewJavaLiteral(1, JavaLong))
		case OP_FCONST_0:
			runtimeStack.Push(NewJavaLiteral(0, JavaFloat))
		case OP_FCONST_1:
			runtimeStack.Push(NewJavaLiteral(1, JavaFloat))
		case OP_FCONST_2:
			runtimeStack.Push(NewJavaLiteral(2, JavaFloat))
		case OP_DCONST_0:
			runtimeStack.Push(NewJavaLiteral(0, JavaDouble))
		case OP_DCONST_1:
			runtimeStack.Push(NewJavaLiteral(1, JavaDouble))
		case OP_BIPUSH:
			runtimeStack.Push(NewJavaLiteral(opcode.Data[0], JavaInteger))
		case OP_SIPUSH:
			runtimeStack.Push(NewJavaLiteral(Convert2bytesToInt(opcode.Data), JavaInteger))
		case OP_ISTORE, OP_ASTORE, OP_LSTORE, OP_DSTORE, OP_FSTORE, OP_ISTORE_0, OP_ASTORE_0, OP_LSTORE_0, OP_DSTORE_0, OP_FSTORE_0, OP_ISTORE_1, OP_ASTORE_1, OP_LSTORE_1, OP_DSTORE_1, OP_FSTORE_1, OP_ISTORE_2, OP_ASTORE_2, OP_LSTORE_2, OP_DSTORE_2, OP_FSTORE_2, OP_ISTORE_3, OP_ASTORE_3, OP_LSTORE_3, OP_DSTORE_3, OP_FSTORE_3:
			id := GetStoreIdx(opcode)
			value := runtimeStack.Pop().(JavaValue)
			isFirst := assignVar(id, value)
			appendStatement(NewAssignStatement(varTable[id], value, isFirst))
		case OP_NEW:
			n := Convert2bytesToInt(opcode.Data)
			javaClass := d.constantPoolGetter(int(n)).(*JavaClass)
			//runtimeStack.Push(javaClass)
			runtimeStack.Push(NewNewExpression(javaClass))
			//appendStatement()
		case OP_NEWARRAY:
			panic("not support")
		case OP_ANEWARRAY:
			value := getConstantPoolValue(opcode)
			length := runtimeStack.Pop().(*JavaLiteral).Data.(int)
			arrayType := NewJavaArrayType(value.(*JavaClass), length)
			exp := NewNewArrayExpression(arrayType, length)
			runtimeStack.Push(exp)
		case OP_MULTIANEWARRAY:
			panic("not support")
		case OP_ARRAYLENGTH:
			panic("not support")
		case OP_AALOAD, OP_IALOAD, OP_BALOAD, OP_CALOAD, OP_FALOAD, OP_LALOAD, OP_DALOAD, OP_SALOAD:
			panic("not support")
		case OP_AASTORE, OP_IASTORE, OP_BASTORE, OP_CASTORE, OP_FASTORE, OP_LASTORE, OP_DASTORE, OP_SASTORE:
			panic("not support")
		case OP_LCMP, OP_DCMPG, OP_DCMPL, OP_FCMPG, OP_FCMPL, OP_LSUB, OP_LADD, OP_IADD, OP_FADD, OP_DADD, OP_ISUB, OP_DSUB, OP_FSUB, OP_IREM, OP_FREM, OP_LREM, OP_DREM, OP_IDIV, OP_FDIV, OP_DDIV, OP_IMUL, OP_DMUL, OP_FMUL, OP_LMUL, OP_LAND, OP_LDIV, OP_LOR, OP_LXOR, OP_ISHR, OP_ISHL, OP_LSHL, OP_LSHR, OP_IUSHR, OP_LUSHR:
			panic("not support")
		case OP_IOR, OP_IAND, OP_IXOR:
			panic("not support")
		case OP_I2B, OP_I2C, OP_I2D, OP_I2F, OP_I2L, OP_I2S, OP_L2D, OP_L2F, OP_L2I, OP_F2D, OP_F2I, OP_F2L, OP_D2F, OP_D2I, OP_D2L:
			panic("not support")
		case OP_INSTANCEOF:
			panic("not support")
		case OP_CHECKCAST:
			panic("not support")
		case OP_INVOKESTATIC:
			panic("not support")
		case OP_INVOKEDYNAMIC:
			panic("not support")
		case OP_INVOKESPECIAL, OP_INVOKEVIRTUAL, OP_INVOKEINTERFACE:
			classInfo := d.constantPoolGetter(int(Convert2bytesToInt(opcode.Data))).(*JavaClassMember)
			methodName := classInfo.Member
			if methodName == "<init>" {
				appendStatement(NewFunctionCallStatement(runtimeStack.Pop().(JavaValue), methodName, nil))
				break
			}
			paramTypes, _, _ := ParseMethodDescriptor(classInfo.Description)
			params := runtimeStack.PopN(len(paramTypes))
			valuesParams := []JavaValue{}
			for _, param := range params {
				valuesParams = append(valuesParams, param.(JavaValue))
			}
			ins := runtimeStack.Pop().(JavaValue)
			appendStatement(NewFunctionCallStatement(ins, methodName, valuesParams))
		case OP_RETURN:
			statements = append(statements, NewReturnStatement(nil))
		case OP_IF_ACMPEQ, OP_IF_ACMPNE, OP_IF_ICMPLT, OP_IF_ICMPGE, OP_IF_ICMPGT, OP_IF_ICMPNE, OP_IF_ICMPEQ, OP_IF_ICMPLE:
			op := GetNotOp(opcode)
			rv := runtimeStack.Pop().(JavaValue)
			lv := runtimeStack.Pop().(JavaValue)
			gotoRaw := Convert2bytesToInt(opcode.Data)
			gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[opcodeIndex]+gotoRaw]
			appendStatement(NewConditionStatement(lv, rv, op, gotoOp))
		case OP_IFNONNULL:
			panic("not support")
		case OP_IFNULL:
			panic("not support")
		case OP_IFEQ, OP_IFNE:
			panic("not support")
		case OP_IFLE, OP_IFLT, OP_IFGT, OP_IFGE:
			panic("not support")
		case OP_JSR, OP_JSR_W:
			panic("not support")
		case OP_RET:
			panic("not support")
		case OP_GOTO:
			target := Convert2bytesToInt(opcode.Data)
			gotoOp := d.offsetToOpcodeIndex[d.opcodeIndexToOffset[opcodeIndex]+target]
			appendStatement(NewGOTOStatement(int(gotoOp)))
		case OP_GOTO_W:
			panic("not support")
		case OP_ATHROW:
			panic("not support")
		case OP_IRETURN:
			panic("not support")
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
			panic("not support")
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
			runtimeStack.Pop()
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
			appendStatement(NewBinaryExpression(NewJavaRef(slot, varSlotTypeTable[slot]), NewJavaLiteral(inc, JavaInteger), INC))
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
	var statements2 []Statement
	i := 0
	for {
		if i >= len(statements) {
			break
		}
		statement := statements[i]
		i++
		var skip bool
		switch ret := statement.(type) {
		case *ConditionStatement:
			ret.ToStatement = opIndexToStatementIndex[ret.ToOpcode]
		case *GOTOStatement:
			ret.ToStatement = opIndexToStatementIndex[ret.ToOpcode]
			if _, ok := statements[ret.ToStatement].(*ConditionStatement); ok {
				statements2 = statements2[:ret.ToStatement-1]
				firstStatement := statements[ret.ToStatement-1].(*AssignStatement)
				declareStatement := NewDeclareStatement(firstStatement.Id, firstStatement.JavaValue.Type())
				statements2 = append(statements2, declareStatement)
				firstStatement.IsFirst = false
				statements2 = append(statements2, NewForStatement(append([]Statement{firstStatement}, statements[ret.ToStatement:i]...)))
				skip = true
			}
		}
		if skip {
			continue
		}
		statements2 = append(statements2, statement)
	}
	d.Statements = statements2
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
