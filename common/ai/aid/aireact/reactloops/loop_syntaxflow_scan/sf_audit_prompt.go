package loop_syntaxflow_scan

// SFAuditCodeSearchHint is appended to SyntaxFlow code-audit rule prompts so the model greps the tree before writing rules.
func SFAuditCodeSearchHint() string {
	return `

【代码搜索 / 专注阅读】在编写或修改 SyntaxFlow 规则前，请使用 grep、read_file、find_file 在已探索的项目路径内缩小 Source/Sink 与框架入口，避免仅凭猜测写规则；优先在可疑目录（handler、controller、router）上缩小范围后再 grep。`
}
