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

	*BinOp: X Op Y
		IsBinOp() bool
		Op(0) X
		Op(1) Y

	*Update: update(address, value)
		IsUpdate() bool
		Op(0) Address
		Op(1) Value

	*Call: function(arg..., binding(..))
		IsCall()   bool
		Op(0) function
		Op(1--len(arg)) Arg
		op(len(arg) -- end) binding

	IsConst()  bool

	*Field: Object.Key
		IsField()  bool
		Op(0) Object
		Op(1) Key
	...


*/
