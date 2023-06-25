package yakvm

var buildinBinaryOperatorHandler = make(map[YVMMode]map[OpcodeFlag]func(*Value, *Value) *Value)
var buildinUnaryOperatorOperatorHandler = make(map[YVMMode]map[OpcodeFlag]func(*Value) *Value)

func getBuildinBinaryOperatorHandler(v YVMMode) map[OpcodeFlag]func(*Value, *Value) *Value {
	if v1, ok := buildinBinaryOperatorHandler[v]; ok {
		return v1
	} else {
		buildinBinaryOperatorHandler[v] = make(map[OpcodeFlag]func(*Value, *Value) *Value)
		return buildinBinaryOperatorHandler[v]
	}
}

func getBuildinUnaryOperatorOperatorHandler(v YVMMode) map[OpcodeFlag]func(*Value) *Value {
	if v1, ok := buildinUnaryOperatorOperatorHandler[v]; ok {
		return v1
	} else {
		buildinUnaryOperatorOperatorHandler[v] = make(map[OpcodeFlag]func(*Value) *Value)
		return buildinUnaryOperatorOperatorHandler[v]
	}
}

func ImportBinaryOperator(vmType YVMMode, flag OpcodeFlag, handler func(*Value, *Value) *Value) {
	getBuildinBinaryOperatorHandler(vmType)[flag] = handler
}
func ImportUnaryOperator(vmType YVMMode, flag OpcodeFlag, handler func(*Value) *Value) {
	getBuildinUnaryOperatorOperatorHandler(vmType)[flag] = handler
}

// nasl
func ImportNaslBinaryOperator(flag OpcodeFlag, handler func(*Value, *Value) *Value) {
	ImportBinaryOperator(NASL, flag, handler)
}
func ImportNaslUnaryOperator(flag OpcodeFlag, handler func(*Value) *Value) {
	ImportUnaryOperator(NASL, flag, handler)
}

// yak
func ImportYakBinaryOperator(flag OpcodeFlag, handler func(*Value, *Value) *Value) {
	ImportBinaryOperator(YAK, flag, handler)
}
func ImportYakUnaryOperator(flag OpcodeFlag, handler func(*Value) *Value) {
	ImportUnaryOperator(YAK, flag, handler)
}

// lua
func ImportLuaBinaryOperator(flag OpcodeFlag, handler func(*Value, *Value) *Value) {
	ImportBinaryOperator(LUA, flag, handler)
}
func ImportLuaUnaryOperator(flag OpcodeFlag, handler func(*Value) *Value) {
	ImportUnaryOperator(LUA, flag, handler)
}
