package decompiler

type OpCode struct {
	Instr *Instruction
	Data  []byte
	Jmp   int
}

