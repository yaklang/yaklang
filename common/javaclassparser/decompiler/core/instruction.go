package core

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/javaclassparser/classes"
	"github.com/yaklang/yaklang/common/log"
)

const (
	OP_DALOAD         = 0x31
	OP_ICONST_1       = 0x4
	OP_ILOAD          = 0x15
	OP_RET            = 0xa9
	OP_IFNONNULL      = 0xc7
	OP_FSTORE_0       = 0x43
	OP_DDIV           = 0x6f
	OP_ARRAYLENGTH    = 0xbe
	OP_LSUB           = 0x65
	OP_PUTFIELD       = 0xb5
	OP_IXOR           = 0x82
	OP_NOP            = 0x0
	OP_ALOAD_0        = 0x2a
	OP_INVOKESTATIC   = 0xb8
	OP_L2D            = 0x8a
	OP_FCONST_1       = 0xc
	OP_LDC_W          = 0x13
	OP_ISTORE_3       = 0x3e
	OP_ISTORE         = 0x36
	OP_LAND           = 0x7f
	OP_ASTORE_0       = 0x4b
	OP_FALOAD         = 0x30
	OP_I2D            = 0x87
	OP_ICONST_5       = 0x8
	OP_FDIV           = 0x6e
	OP_LMUL           = 0x69
	OP_ANEWARRAY      = 0xbd
	OP_SIPUSH         = 0x11
	OP_FREM           = 0x72
	OP_IADD           = 0x60
	OP_LLOAD          = 0x16
	OP_DLOAD_2        = 0x28
	OP_LOOKUPSWITCH   = 0xab
	OP_ISTORE_2       = 0x3d
	OP_LADD           = 0x61
	OP_SALOAD         = 0x35
	OP_ALOAD          = 0x19
	OP_GETFIELD       = 0xb4
	OP_LCMP           = 0x94
	OP_LDC2_W         = 0x14
	OP_IF_ICMPGE      = 0xa2
	OP_JSR_W          = 0xc9
	OP_ILOAD_2        = 0x1c
	OP_D2F            = 0x90
	OP_CHECKCAST      = 0xc0
	OP_FCONST_2       = 0xd
	OP_LALOAD         = 0x2f
	OP_DNEG           = 0x77
	OP_LLOAD_1        = 0x1f
	OP_DSUB           = 0x67
	OP_DREM           = 0x73
	OP_DSTORE_3       = 0x4a
	OP_ISTORE_1       = 0x3c
	OP_I2S            = 0x93
	OP_IMUL           = 0x68
	OP_GETSTATIC      = 0xb2
	OP_ISTORE_0       = 0x3b
	OP_DUP2_X1        = 0x5d
	OP_FSTORE_2       = 0x45
	OP_ICONST_0       = 0x3
	OP_GOTO           = 0xa7
	OP_IOR            = 0x80
	OP_DUP            = 0x59
	OP_ACONST_NULL    = 0x1
	OP_IFNE           = 0x9a
	OP_ALOAD_1        = 0x2b
	OP_NEW            = 0xbb
	OP_IDIV           = 0x6c
	OP_DADD           = 0x63
	OP_ARETURN        = 0xb0
	OP_LSHR           = 0x7b
	OP_DCMPL          = 0x97
	OP_L2F            = 0x89
	OP_AALOAD         = 0x32
	OP_IUSHR          = 0x7c
	OP_MULTIANEWARRAY = 0xc5
	OP_DSTORE_0       = 0x47
	OP_SWAP           = 0x5f
	OP_I2L            = 0x85
	OP_FLOAD_3        = 0x25
	OP_IFNULL         = 0xc6
	OP_I2F            = 0x86
	OP_LSTORE_3       = 0x42
	OP_IFEQ           = 0x99
	OP_WIDE           = 0xc4
	OP_IAND           = 0x7e
	OP_GOTO_W         = 0xc8
	OP_LASTORE        = 0x50
	OP_DMUL           = 0x6b
	OP_F2D            = 0x8d
	OP_FNEG           = 0x76
	OP_TABLESWITCH    = 0xaa
	OP_IFLE           = 0x9e
	OP_MONITOREXIT    = 0xc3
	OP_IREM           = 0x70
	OP_LSHL           = 0x79
	OP_FSTORE_1       = 0x44
	OP_LSTORE         = 0x37
	OP_LRETURN        = 0xad
	OP_FSTORE         = 0x38
	OP_BALOAD         = 0x33
	OP_ILOAD_0        = 0x1a
	OP_DUP2           = 0x5c
	OP_NEWARRAY       = 0xbc
	OP_DUP2_X2        = 0x5e
	OP_DSTORE_1       = 0x48
	OP_LSTORE_2       = 0x41
	OP_IFGE           = 0x9c
	OP_IF_ICMPEQ      = 0x9f
	OP_POP            = 0x57
	OP_DLOAD_0        = 0x26
	OP_FMUL           = 0x6a
	OP_LSTORE_0       = 0x3f
	OP_LLOAD_0        = 0x1e
	OP_DLOAD_1        = 0x27
	OP_LCONST_1       = 0xa
	OP_CASTORE        = 0x55
	OP_LNEG           = 0x75
	OP_LCONST_0       = 0x9
	OP_JSR            = 0xa8
	OP_CALOAD         = 0x34
	OP_IASTORE        = 0x4f
	OP_LDC            = 0x12
	OP_IINC           = 0x84
	OP_FLOAD_0        = 0x22
	OP_DLOAD_3        = 0x29
	OP_ASTORE_2       = 0x4d
	OP_F2I            = 0x8b
	OP_ICONST_M1      = 0x2
	OP_ISHR           = 0x7a
	OP_I2B            = 0x91
	OP_FSUB           = 0x66
	OP_ASTORE_1       = 0x4c
	OP_ICONST_3       = 0x6
	OP_IRETURN        = 0xac
	OP_DCONST_0       = 0xe
	OP_DASTORE        = 0x52
	OP_FASTORE        = 0x51
	OP_FCMPL          = 0x95
	OP_FCONST_0       = 0xb
	OP_F2L            = 0x8c
	OP_IFLT           = 0x9b
	OP_LSTORE_1       = 0x40
	OP_IF_ACMPEQ      = 0xa5
	OP_IF_ICMPGT      = 0xa3
	OP_FSTORE_3       = 0x46
	OP_RETURN         = 0xb1
	OP_BASTORE        = 0x54
	OP_SASTORE        = 0x56
	OP_DLOAD          = 0x18
	OP_LLOAD_2        = 0x20
	OP_LREM           = 0x71
	OP_DCONST_1       = 0xf
	//OP_LLOAD_WIDE = 0x-1
	OP_ICONST_2        = 0x5
	OP_IF_ICMPLE       = 0xa4
	OP_DSTORE          = 0x39
	OP_ICONST_4        = 0x7
	OP_LDIV            = 0x6d
	OP_LOR             = 0x81
	OP_DSTORE_2        = 0x49
	OP_INVOKEINTERFACE = 0xb9
	OP_LXOR            = 0x83
	OP_DUP_X1          = 0x5a
	OP_DRETURN         = 0xaf
	OP_FRETURN         = 0xae
	OP_FCMPG           = 0x96
	OP_DCMPG           = 0x98
	OP_INVOKESPECIAL   = 0xb7
	OP_L2I             = 0x88
	OP_PUTSTATIC       = 0xb3
	OP_ASTORE_3        = 0x4e
	OP_DUP_X2          = 0x5b
	OP_IF_ICMPNE       = 0xa0
	OP_ILOAD_3         = 0x1d
	OP_FLOAD           = 0x17
	OP_IF_ICMPLT       = 0xa1
	OP_LUSHR           = 0x7d
	OP_ILOAD_1         = 0x1b
	OP_AASTORE         = 0x53
	OP_MONITORENTER    = 0xc2
	OP_ALOAD_2         = 0x2c
	OP_ASTORE          = 0x3a
	OP_INEG            = 0x74
	OP_POP2            = 0x58
	OP_FLOAD_1         = 0x23
	OP_INVOKEDYNAMIC   = 0xba
	OP_I2C             = 0x92
	OP_INSTANCEOF      = 0xc1
	OP_FLOAD_2         = 0x24
	OP_IF_ACMPNE       = 0xa6
	OP_ALOAD_3         = 0x2d
	OP_BIPUSH          = 0x10
	OP_D2I             = 0x8e
	OP_D2L             = 0x8f
	OP_IFGT            = 0x9d
	OP_ISUB            = 0x64
	OP_LLOAD_3         = 0x21
	OP_ISHL            = 0x78
	OP_FADD            = 0x62
	OP_INVOKEVIRTUAL   = 0xb6
	OP_ATHROW          = 0xbf
	OP_IALOAD          = 0x2e
	OP_END             = 0xff
)

type Instruction struct {
	Name        string       `json:"name"`
	OpCode      int          `json:"opcode"`
	Length      int          `json:"data_length"`
	StackPopped []*StackType `json:"stack_types"`
	StackPushed []*StackType `json:"stack_pushed"`
	RawJavaType *RawJavaType `json:"raw_java_type"`
	HandleName  string       `json:"handler"`
	NoThrow     bool         `json:"no_throw"`
}
type RawJavaType struct {
	Name             string     `json:"name"`
	SuggestedVarName string     `json:"suggestedVarName"`
	StackType        *StackType `json:"stackType"`
	UsableType       bool       `json:"usableType"`
	BoxedName        string     `json:"boxedName"`
	IsNumber         bool       `json:"isNumber"`
	IsObject         bool       `json:"isObject"`
	IntMin           int        `json:"intMin"`
	IntMax           int        `json:"intMax"`
}

func (r *RawJavaType) GetStackType() *StackType {
	return r.StackType
}

type StackType struct {
	ComputationCategory int    `json:"computationCategory"`
	Closed              bool   `json:"closed"`
	Name                string `json:"name"`
}

var InstrInfos = map[int]*Instruction{}

func init() {
	content, err := classes.FS.ReadFile("instr_infos.json")
	if err != nil {
		log.Errorf("initialize instruction info failed")
		return
	}
	instrInfos := map[string]*Instruction{}
	err = json.Unmarshal(content, &instrInfos)
	if err != nil {
		log.Errorf("invalid json, parse instruction failed: %v", err)
		return
	}
	for k, instruction := range instrInfos {
		instruction.Name = k
		if instruction.Length < 0 {
			instruction.Length = 0
		}
		InstrInfos[instruction.OpCode] = instruction
	}
	InstrInfos[OP_END] = &Instruction{
		Name:   "end",
		OpCode: OP_END,
	}
}
