package syntaxflowdoc

// seedOpcodeCatalog documents ?{opcode: ...} filter tokens (SSA IR kinds).
func seedOpcodeCatalog(h *DocumentHelper) {
	if h == nil {
		return
	}
	type item struct {
		name    string
		aliases []string
		desc    string
	}
	items := []item{
		{"undefined", []string{"und"}, "未定义值"},
		{"freevalue", []string{"free"}, "自由变量 / FreeValue"},
		{"extern_lib", []string{"externlib", "lib"}, "外部库符号"},
		{"call", nil, "函数/方法调用"},
		{"phi", nil, "SSA Phi 节点"},
		{"const", []string{"constant", "constinst"}, "常量"},
		{"parameter", []string{"param", "formal_param"}, "形式参数"},
		{"parametermember", []string{"param_member", "parammember"}, "参数成员"},
		{"return", nil, "返回指令"},
		{"function", []string{"func", "def"}, "函数定义"},
		{"basicblock", []string{"basic_block", "block"}, "基本块"},
		{"if", nil, "条件分支"},
		{"errorhandler", []string{"try"}, "错误处理 / try"},
		{"errorcatch", []string{"catch"}, "catch"},
		{"panic", []string{"throw"}, "panic/throw"},
		{"switch", nil, "switch"},
		{"loop", nil, "循环"},
		{"typecast", nil, "类型转换"},
		{"make", nil, "make/构造"},
		{"binop", nil, "二元运算"},
		{"unop", nil, "一元运算"},
		{"assert", nil, "断言"},
		{"jump", nil, "跳转"},
		{"next", nil, "迭代 next"},
		{"recover", nil, "recover"},
		{"sideeffect", nil, "副作用"},
		{"typevalue", nil, "类型值"},
		{"unknow", nil, "未知指令"},
	}
	for _, it := range items {
		h.Opcodes[it.name] = &DocEntry{
			Category:    CategoryOpcode,
			Name:        it.name,
			Aliases:     it.aliases,
			Title:       "opcode:" + it.name,
			Description: it.desc + "。用于 ?{opcode: " + it.name + "} 过滤。",
			Example:     "$v?{opcode: " + it.name + "}",
		}
	}
}
