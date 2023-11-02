package ssaapi

/*
This Package only one Function: Parse

Parse(code string, opt ... Option) *Program
	parse code to SSA form.

Program struct method:
	Ref(name string) Values

Values:
	Show()
	Ref(name string) Values
		just a filter
	Get(index int) *Value
	ForEach(func(*Value))

*Value:
	// display
	Show()
	ShowUseDefChain()

	GetUsers() Values
	GetUser(index int) *Value

	GetOperands() Values
	GetOperand(index int) *Value


	// instruction
	GetOpcode() Opcode

	IsUpdate() bool
	IsCall()   bool
	IsConst()  bool
	...


*/
