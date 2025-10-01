package aireact

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

func getReSelectTool(
	allowAskForClarification bool,
) string {
	var opts []any

	actionEnum := []string{
		"require-tool", "abandon",
	}
	if allowAskForClarification {
		actionEnum = []string{"require-tool", "ask-for-clarification", "abandon"}
	}
	rules := []string{
		"如果你决定使用某个工具，请选择require-tool，设置tool为你选择的工具名称",
		"如果你认为当前任务不需要工具，请选择abandon，设置tool为空字符串",
	}
	if allowAskForClarification {
		rules = append(rules, "如果你不确定用户的意图，请选择ask-for-clarification，设置tool为空字符串")
	}

	opts = append(opts, aitool.WithStringParam(
		"@action",
		aitool.WithParam_Description("You MUST choose one of the following action types."+
			" What you choose will determine the next-step behavior.",
		),
		aitool.WithParam_EnumString(actionEnum...),
		aitool.WithParam_Required(true),
		aitool.WithParam_Raw("x-rules", rules),
	))
	opts = append(opts, aitool.WithStringParam("tool", aitool.WithParam_Description("Your tool that will be used.")))
	opts = append(opts, aitool.WithStringParam("abandon_reason", aitool.WithParam_Description("If you choose 'abandon', please provide a brief reason for abandoning the tool usage.")))

	if allowAskForClarification {
		opts = append(opts, aitool.WithStructParam(
			"clarification_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Required(false),
			},
			aitool.WithStringParam(
				"question",
				aitool.WithParam_Description("A clear, concise question to ask the user for more information. This should help clarify their intent or provide necessary details."), aitool.WithParam_Required(true),
			),
			aitool.WithStringArrayParam(
				"options",
				aitool.WithParam_Description("A list of options for helping user option or suggestions. This can include examples or explanations of why the clarification is necessary."),
			),
		))
	}
	return aitool.NewObjectSchema(opts...)
}

func getDirectlyAnswer() string {
	var opts []any

	actionFields := []aitool.ToolOption{
		aitool.WithStringParam(
			"type",
			aitool.WithParam_Description("You MUST use 'directly_answer' as the action type."),
			aitool.WithParam_Enum(ActionDirectlyAnswer),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam("answer_payload", aitool.WithParam_Description("Provide the final, complete answer for the user here. The content should be self-contained and ready to be displayed."), aitool.WithParam_Required(true)),
	}

	opts = append(opts, aitool.WithStructParam(
		"next_action",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("Contains the direct answer action."),
		},
		actionFields...,
	), aitool.WithStringParam(
		"cumulative_summary",
		aitool.WithParam_Required(true),
		aitool.WithParam_Description("An evolving summary of the conversation. Update this field to include key information from the current interaction that should be remembered for future responses. Include topics discussed, user preferences, important context, and relevant details. If this is the first interaction, create a new summary. If there's existing context, build upon it."),
	))
	return aitool.NewObjectSchemaWithAction(opts...)
}

func getLoopSchema(
	disallowAskForClarification,
	disableKnowledgeEnhanceAnswer,
	disallowPlanAndExecution,
	disallowWriteYaklangCode bool,
	haveAIForgeList bool,
) string {
	var opts []any
	mode := []any{
		ActionDirectlyAnswer, ActionRequireTool,
	}
	if !disallowPlanAndExecution {
		mode = append(mode, ActionRequestPlanExecution)
		if haveAIForgeList {
			mode = append(mode, ActionRequireAIBlueprintForge)
		}
	}
	if !disallowAskForClarification {
		mode = append(mode, ActionAskForClarification)
	}

	if !disableKnowledgeEnhanceAnswer {
		mode = append(mode, ActionKnowledgeEnhanceAnswer)
	}

	if !disallowWriteYaklangCode {
		mode = append(mode, ActionWriteYaklangCode)
	}

	actionFields := []aitool.ToolOption{
		aitool.WithStringParam(
			"type",
			aitool.WithParam_Description("You MUST choose one of following action types. The value you select here determines which of the other fields in this 'action' object you should populate."),
			aitool.WithParam_Enum(mode...),
			aitool.WithParam_Required(true),
		),
	}
	actionFields = append(
		actionFields,
		aitool.WithStringParam("answer_payload", aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'directly_answer'. Provide the final, complete answer for the user here. The content should be self-contained and ready to be displayed.")),
		aitool.WithStringParam("tool_require_payload", aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_tool'. Provide the exact name of the tool you need to use (e.g., 'web_search', 'database_query'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here.")),
		aitool.WithBoolParam(
			"middle_step",
			aitool.WithParam_Description("CRUCIAL for multi-tool tasks. Use ONLY with 'tool_require_payload'. Set to 'true' if this tool call is an intermediate step in a sequence to solve a complex task. Set to 'false' if this is the FINAL tool call needed before you can provide the complete answer. Before starting, you should outline your multi-step plan in 'cumulative_summary'."),
			aitool.WithParam_Required(true),
		),
	)
	if !disallowPlanAndExecution {
		actionFields = append(
			actionFields,
			aitool.WithStringParam(
				"plan_request_payload",
				aitool.WithParam_Description(
					"USE THIS FIELD ONLY IF type is 'request_plan_and_execution'. Provide a one-sentence summary of the complex task that needs a multi-step plan. This summary will trigger a more advanced planning system. Example: 'Create a marketing plan for a new product launch.'",
				),
			),
		)
		if haveAIForgeList {
			aitool.WithStringParam(
				"blueprint_payload",
				aitool.WithParam_Description(
					"USE THIS FIELD ONLY IF type is 'require_ai_blueprint'. Provide a forge blueprint ID from the available AI blueprint list to execute the task.",
				),
			)
		}
	}
	if !disallowAskForClarification {
		actionFields = append(actionFields, aitool.WithStructParam(
			"ask_for_clarification_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Use this action when user's intent is ambiguous or incomplete."),
			},
			aitool.WithStringParam("question", aitool.WithParam_Required(true), aitool.WithParam_Description("A clear, concise question to ask the user for more information. This should help clarify their intent or provide necessary details.")),
			aitool.WithStringArrayParam(
				"options",
				aitool.WithParam_Description(
					`Optional additional context that may help the user understand what information is needed. This can include examples or explanations of why the clarification is necessary.`,
				),
			),
		))
	}

	if !disallowWriteYaklangCode {
		actionFields = append(actionFields, aitool.WithStringParam(
			"write_yaklang_code_approach",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'write_yaklang_code' 编写Yaklang代码的具体思路和策略。应该包含：1) 代码的核心逻辑设计思路和主要执行流程；2) 需要使用的Yaklang内置库和关键函数（如poc、synscan、servicescan、crawler、等安全库）；3) 数据处理方式和结果输出策略；4) 并发控制方案（是否使用go关键字、SizedWaitGroup限制并发数等）；5) 错误处理策略（优先使用~操作符进行简洁的错误处理）；6) 用户交互设计（如cli参数接收、进度显示等）。重点阐述如何充分利用Yaklang的安全特性、语法糖和内置能力来高效实现目标功能，确保代码既简洁又强大。"),
		))
	}

	opts = append(opts, aitool.WithStringParam(
		"human_readable_thought",
		aitool.WithParam_Required(false),
		aitool.WithParam_Description("[Not a must-being] Provide a brief, user-friendly status message here, explaining what you are currently doing. This will be shown to the user in real-time. Examples: 'Okay, I understand. Searching for the requested information now...', 'I need to use a tool to get the current stock price.', 'This is a complex request, I will try to execute tool step by step.' if direct-answer mode, no need to fill this field"),
	), aitool.WithStructParam(
		"next_action",
		[]aitool.PropertyOption{
			aitool.WithParam_Description("Contains the specific action the AI has decided to take. You must choose one action type and provide its corresponding payload."),
		},
		actionFields...,
	), aitool.WithStringParam(
		"cumulative_summary",
		aitool.WithParam_Required(true),
		aitool.WithParam_Description("An evolving summary of the conversation. Update this field to include key information from the current interaction that should be remembered for future responses. Include topics discussed, user preferences, important context, and relevant details. If this is the first interaction, create a new summary. If there's existing context, build upon it."),
	))
	return aitool.NewObjectSchemaWithAction(opts...)
}

func getYaklangCodeLoopSchema(allowAskForClarification bool, haveFinished bool) string {
	actionEnums := []string{
		"query_document",
		"write_code",
		"modify_code",
		"require_tool",
	}
	if allowAskForClarification {
		actionEnums = append(actionEnums, "ask_for_clarification")
	}
	if haveFinished {
		actionEnums = append(actionEnums, "finish")
	}

	description := "You MUST choose one of the following action types for the Yaklang code generation loop. What you choose will determine the next-step behavior in the code generation process.\n" +
		"⚠️ CRITICAL: When using 'write_code' or 'modify_code', you MUST provide PURE Yaklang code in <|GEN_CODE_...|> tags WITHOUT any line numbers or prefixes. The code must be directly executable. Check examples after schema.\n\n" +
		"Action descriptions:\n" +
		"- 'query_document': Search for specific Yaklang functions or patterns in documentation\n" +
		"- 'write_code': Generate new Yaklang code from scratch\n" +
		"- 'modify_code': Modify existing code by replacing specific line ranges\n" +
		"- 'require_tool': Request additional tools to help complete the task\n" +
		"- 'ask_for_clarification': Ask user for more information when intent is unclear\n"

	if haveFinished {
		description += "- 'finish': Complete the task when code is fully functional and error-free"
		description += "\n\n⚠️ IMPORTANT: Since 'finish' action is available, you MUST ensure that all errors in the error section (<|ERR/LINT_WARNING|>) are thoroughly addressed. No syntax errors, logic errors, or functionality-impairing issues should remain. The code must be production-ready before choosing 'finish'."
	}

	opts := []any{
		aitool.WithStringParam(
			"@action",
			aitool.WithParam_Description(description),
			aitool.WithParam_EnumString(actionEnums...),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"human_readable_thought",
			aitool.WithParam_Description("Provide a brief, user-friendly status message here, explaining what you are currently doing. This will be shown to the user in real-time. Examples: 'Okay, I understand. Searching for the requested information now...', 'I need to use a tool to get the current stock price.', 'This is a complex request, I will try to execute tool step by step.'"),
		),
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_tool'. Provide the exact name of the tool you need to use (e.g., 'check-yaklang-syntax', 'yak-document'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here."),
		),
		aitool.WithNumberParam(
			"modify_start_line",
			aitool.WithParam_Description("⚠️ ONLY for 'modify_code': Specify the starting line number (1-based) of code to replace. IMPORTANT: These numbers are ONLY for identifying the replacement range - the generated code in <|GEN_CODE_...|> must NOT include any line numbers or '|' separators. Generate pure, clean Yaklang code only."),
		),
		aitool.WithNumberParam(
			"modify_end_line",
			aitool.WithParam_Description("⚠️ ONLY for 'modify_code': Specify the ending line number (1-based) of code to replace. Lines from modify_start_line to modify_end_line will be replaced by your generated code. CRITICAL: Your code in <|GEN_CODE_...|> must be pure Yaklang without line numbers - no '18 |', '19 |' prefixes! Just raw executable code."),
		),
		aitool.WithStructParam(
			"query_document_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'query_document'. Provide the exact search pattern of the document you need to query (e.g., 'json.dump', 'servicescan.Scan', 'file.ReadFile', '端口扫描', '打开文件'). Another system will handle the parameter generation based on this name."),
			},
			aitool.WithBoolParam(
				"case_sensitive",
				aitool.WithParam_Description("Indicates whether the search should be case-sensitive. If true, the search will differentiate between uppercase and lowercase letters. If false, the search will be case-insensitive."),
			),
			aitool.WithStringArrayParam(
				"keywords",
				aitool.WithParam_Description(`Keywords or phrases to search in Yaklang documentation (supports both Chinese and English). Common patterns:

**High-Frequency Functions (use exact names)**:
• Network: 'poc.HTTP', 'poc.HTTPEx', 'poc.Get', 'poc.Post', 'servicescan.Scan', 'synscan.Scan'
• File: 'file.ReadFile', 'file.Save', 'filesys.Recursive', 'zip.CompressRaw', 'zip.Recursive'
• String: 'str.Split', 'str.Join', 'str.Contains', 'str.Replace', 'str.TrimPrefix'
• Codec: 'codec.DecodeBase64', 'codec.EncodeBase64', 'json.dumps', 'json.loads'
• Database: 'db.Query', 'db.Exec', 'risk.NewRisk'

**Function Options (exact option names)**:
• HTTP: 'poc.timeout', 'poc.json', 'poc.header', 'poc.cookie', 'poc.body', 'poc.retry'
• Scan: 'servicescan.concurrent', 'servicescan.active', 'servicescan.web', 'servicescan.all'
• File: 'filesys.onFileStat', 'file.IsDir', 'file.IsFile'

**Feature Keywords (Chinese or English)**:
• Chinese: 'HTTP发包', 'HTTP请求', '端口扫描', '服务扫描', '文件读取', '文件写入', '字符串处理', 'JSON解析', '并发编程', '错误处理', '正则匹配'
• English: 'send request', 'port scan', 'file operation', 'string processing', 'error handling', 'concurrent', 'goroutine', 'channel'

**Common Patterns**:
• Error handling: 'die(err)', '~', 'try-catch', 'defer-recover'
• Concurrency: 'go func', 'sync.NewWaitGroup', 'sync.NewSizedWaitGroup', 'channel'
• Fuzzing: 'fuzz.HTTPRequest', 'fuzztag', '{{参数}}'

**Example combinations**:
- For HTTP: ["poc.HTTP", "HTTP发包", "poc.timeout", "发送请求"]
- For scanning: ["servicescan.Scan", "端口扫描", "servicescan.concurrent", "指纹识别"]
- For files: ["file.ReadFile", "文件读取", "filesys.Recursive", "文件遍历"]`),
			),
			aitool.WithStringArrayParam(
				"regexp",
				aitool.WithParam_Description(`Regular expressions to match specific code patterns in Yaklang documentation. Use for precise structural matching:

**Function Call Patterns**:
• Library functions: '\w+\.\w+\(' - matches any library.function() calls
• Specific library: 'poc\.\w+\(' - matches all poc.* functions
• HTTP methods: 'poc\.(HTTP|HTTPEx|Get|Post|Do)\(' - matches HTTP-related functions
• File operations: 'file\.(ReadFile|Save|WriteFile)\(' - matches file functions
• String utils: 'str\.(Split|Join|Contains|Replace)\(' - matches string functions

**Configuration Options**:
• HTTP options: 'poc\.(timeout|json|header|cookie|body|query|postParams)\(' - matches HTTP config
• Scan options: 'servicescan\.(concurrent|timeout|active|web|all)\(' - matches scan config
• Context options: '\.(https|port|host|redirectTimes|retryTimes)\(' - matches connection config

**Control Flow & Error Handling**:
• Error handling: 'die\(|~\s*$|try\s*\{|defer.*recover\(' - matches error patterns
• Concurrency: 'go\s+func|sync\.New\w+WaitGroup|make\(chan\s+' - matches concurrent code
• Loops: 'for\s+\w+\s+in\s+|for\s+\w+\s*:?=?\s*range\s+' - matches for-in/range loops

**Code Structure**:
• Function definition: '(func|fn|def)\s+\w+\s*\(' - matches function declarations
• Variable assignment: '\w+\s*:?=\s*\w+\.\w+\(' - matches var = lib.func() pattern
• Method chaining: '\)\s*\.\s*\w+\(' - matches chained method calls

**Example patterns**:
- HTTP workflow: ['poc\.(HTTP|Get|Post)\(', 'poc\.(timeout|json|header)\(', '~\s*$']
- File processing: ['file\.\w+\(', 'filesys\.Recursive\(', 'for.*range.*']
- Error handling: ['die\(|~', 'try\s*\{.*\}\s*catch', 'defer.*recover\(']
- Concurrency: ['go\s+func', 'sync\.New.*WaitGroup', '<-.*chan|chan\s*<-']

**Note**: Patterns are case-sensitive. Use '\s+' for whitespace, '\w+' for identifiers, '.*' for wildcards.`),
			),
		),
	}
	if allowAskForClarification {
		opts = append(opts, aitool.WithStructParam(
			"ask_for_clarification_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Use this action when user's intent is ambiguous or incomplete."),
			},
			aitool.WithStringParam("question", aitool.WithParam_Required(true), aitool.WithParam_Description("A clear, concise question to ask the user for more information. This should help clarify their intent or provide necessary details.")),
			aitool.WithStringArrayParam(
				"options",
				aitool.WithParam_Description(
					`Optional additional context that may help the user understand what information is needed. This can include examples or explanations of why the clarification is necessary.`,
				),
			),
		))
	}
	return aitool.NewObjectSchema(opts...)
}
