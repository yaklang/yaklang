package ssa_compile

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// SSADetectConfig 调用「SSA 项目探测」时的入参
type SSADetectConfig struct {
	Target             string
	Language           string
	CompileImmediately bool
	Params             map[string]any
}

// SSADetectResult ParseProjectWithAutoDetective 的返回
type SSADetectResult struct {
	Info    *AutoDetectInfo
	Program *ssaapi.Program
	Cleanup func()
}

// AutoDetectInfo 与「SSA 项目探测」插件约定的事件结构（含 ssaconfig.Config）
type AutoDetectInfo struct {
	*ssaconfig.Config
	FileCount          int  `json:"file_Count"`
	CompileImmediately bool `json:"compile_immediately"`
	ProjectExists      bool `json:"project_exists"`
	Error              struct {
		Kind string `json:"kind"`
		Msg  string `json:"msg"`
	} `json:"error"`
}
