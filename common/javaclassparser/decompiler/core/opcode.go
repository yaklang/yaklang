package core

type OpCode struct {
	Id                             int
	Instr                          *Instruction
	CurrentOffset                  uint16
	Data                           []byte
	Jmp                            int
	IsWide                         bool
	IsCatch                        bool
	ExceptionTypeIndex             uint16
	SwitchJmpCase                  map[int]uint32
	SwitchJmpCase1                 map[int]int
	Source                         []*OpCode
	Target                         []*OpCode
	TrueNode, FalseNode, MergeNode *OpCode
	IsTernaryNode                  bool
}
