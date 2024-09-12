package decompiler

type OpCode struct {
	Id     int
	Instr  *Instruction
	Data   []byte
	Jmp    int
	Source []*OpCode
	Target []*OpCode
}
