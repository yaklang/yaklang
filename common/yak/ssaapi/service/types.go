package service

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// SSACompileRequest 编译请求
type SSACompileRequest struct {
	Target      string              // 目标文件/目录
	ProgramName string              // 程序名称
	Language    string              // 语言类型
	Entry       string              // 入口文件
	ReCompile   bool                // 是否重新编译
	ExcludeFile string              // 排除文件模式
	Options     []ssaconfig.Option  // 编译选项
}

// SSACompileResponse 编译响应
type SSACompileResponse struct {
	Programs ssaapi.Programs
	Error    error
}

// SSAQueryRequest 查询请求
type SSAQueryRequest struct {
	ProgramNamePattern string // 程序名称模式（正则）
	Language           string // 语言过滤
	Limit              int    // 限制数量
}

// SSAQueryResponse 查询响应
type SSAQueryResponse struct {
	Programs []*ssaapi.Program
	Error    error
}

// SSASyntaxFlowQueryRequest SyntaxFlow查询请求
type SSASyntaxFlowQueryRequest struct {
	ProgramName string // 程序名称
	Rule        string // SyntaxFlow规则
	Debug       bool   // 是否开启调试
	ShowDot     bool   // 是否显示dot图
	WithCode    bool   // 是否显示代码上下文
}

// SSASyntaxFlowQueryResponse SyntaxFlow查询响应
type SSASyntaxFlowQueryResponse struct {
	Result *ssaapi.SyntaxFlowResult
	Error  error
}

