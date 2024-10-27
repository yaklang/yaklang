package core

type OpCode struct {
	Id            int
	Instr         *Instruction
	CurrentOffset uint16
	Data          []byte
	Jmp           int
	IsWide        bool
	SwitchJmpCase map[int]uint32
	Source        []*OpCode
	Target        []*OpCode
}
