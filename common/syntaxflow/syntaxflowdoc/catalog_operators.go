package syntaxflowdoc

// seedOperatorAndSyntaxCatalog fills static SyntaxFlow language surface docs
// (operators, common patterns). This is the cookbook half of syntaxflowdoc;
// NativeCall / builtin libs come from live registries.
func seedOperatorAndSyntaxCatalog(h *DocumentHelper) {
	if h == nil {
		return
	}

	put := func(m map[string]*DocEntry, e *DocEntry) {
		if e == nil || e.Name == "" {
			return
		}
		m[e.Name] = e
	}

	ops := []*DocEntry{
		{Category: CategoryOperator, Name: "#->", Title: "数据流正向追踪", Description: "从当前值沿数据流向前追踪到使用点（use）。规则里最常用的 source→sink 连接符。", Example: ".Query(* #-> as $src); alert $src"},
		{Category: CategoryOperator, Name: "#>", Title: "数据流一步", Description: "数据流单步前进（比 #-> 更短距离）。", Example: "$x #> as $next"},
		{Category: CategoryOperator, Name: "-->", Title: "调用关系/依赖边", Description: "沿调用或依赖边前进；多数漏洞规则应优先用 #-> 做污点追踪，不要用 --> 替代数据流。", Example: "$fn --> as $callee"},
		{Category: CategoryOperator, Name: "->", Title: "单步边", Description: "图上的单步连接，语义弱于 #->。", Example: "$a -> as $b"},
		{Category: CategoryOperator, Name: "as", Title: "变量绑定", Description: "把当前匹配结果绑定到 $var，后续可引用。", Example: ".Query(* as $param)"},
		{Category: CategoryOperator, Name: "$", Title: "变量引用", Description: "引用已绑定的 SyntaxFlow 变量。", Example: "$param #-> as $sink"},
		{Category: CategoryOperator, Name: ".", Title: "成员/方法投影", Description: "从对象投影到成员或方法名；`.Method` 常用于匹配调用。", Example: ".ExecuteQuery(* #-> as $src)"},
		{Category: CategoryOperator, Name: "*", Title: "通配/实参捕获", Description: "在调用参数位置捕获实参；也可作 glob。", Example: ".Query(* as $p)"},
		{Category: CategoryOperator, Name: "?{}", Title: "条件过滤（花括号）", Description: "对当前值做属性/子表达式过滤，支持 opcode、native call、逻辑组合。", Example: "$x?{opcode: const}"},
		{Category: CategoryOperator, Name: "?()", Title: "条件过滤（圆括号）", Description: "与 ?{} 同类的条件过滤语法形式。", Example: "$x?(*<len> == 2)"},
		{Category: CategoryOperator, Name: "<nativeCall>", Title: "NativeCall 调用", Description: "尖括号调用内置 NativeCall，如 <typeName>、<include('lib')>、<dataflow...>。", Example: "$v<typeName> as $t"},
		{Category: CategoryOperator, Name: "include", Title: "引入内置/库规则", Description: "必须写成 <include('lib-name')> as $var；漏写 as 会导致变量未定义。", Example: "<include('golang-gin-context')> as $gin"},
		{Category: CategoryOperator, Name: "alert", Title: "告警输出", Description: "将变量标记为漏洞告警结果，可附 message/level。", Example: "alert $src for { message: \"sqli\", level: high }"},
		{Category: CategoryOperator, Name: "desc", Title: "规则元数据块", Description: "desc() 写 title/type/level 等元数据；检测逻辑必须在 desc 之外。测试用例常用第二个 desc 放 file:// UNSAFE/SAFE。", Example: "desc(\n  title: \"demo\"\n  type: audit\n  level: high\n)"},
		{Category: CategoryOperator, Name: "<<<HEREDOC", Title: "Heredoc 字符串", Description: "多行字符串；结束符必须单独占一行且行首无空格。", Example: "desc: <<<TEXT\n说明\nTEXT"},
	}
	for _, e := range ops {
		put(h.Operators, e)
	}

	syntaxes := []*DocEntry{
		{Category: CategorySyntax, Name: "rule-structure", Title: "规则结构", Description: "有效规则 = desc(元数据) + 至少一个匹配/数据流模式 + 至少一个 alert。仅含 desc() 的规则无效。", Example: "desc(title: \"x\", type: audit, level: high)\n.sink(* #-> as $s);\nalert $s"},
		{Category: CategorySyntax, Name: "source-sink", Title: "Source→Sink 模式", Description: "先定位危险 sink，再用 #-> 回追用户输入 source；复杂链路可拆成多步分别验证。", Example: "<include('golang-user-input')> as $in;\n$in.GetQuery(* #-> as $src);\n.Exec(* as $sink);\n$sink #{ until: `* & $src` }->;\nalert $sink"},
		{Category: CategorySyntax, Name: "opcode-filter", Title: "opcode 过滤", Description: "在 ?{opcode: name} 中按 SSA 指令类型过滤，如 call、const、param、phi、return。", Example: "$v?{opcode: call}"},
		{Category: CategorySyntax, Name: "glob-name", Title: "名称 glob/正则", Description: "方法名可用 glob（get*）或正则；不要在普通字符串里用 | 表示多选。", Example: ".get*(* as $p)"},
		{Category: CategorySyntax, Name: "check-self-test", Title: "规则自检约定", Description: "正例用第二个 desc 的 'file://xxx': <<<UNSAFE ...；调用 check-syntaxflow-syntax 传 sample_code/language 直至 matched=true。", Example: "desc(\n  lang: golang\n  alert_high: 1\n  'file://vuln.go': <<<UNSAFE\n// vuln code\nUNSAFE\n)"},
	}
	for _, e := range syntaxes {
		put(h.Syntax, e)
	}
}
