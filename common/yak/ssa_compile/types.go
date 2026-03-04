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
	// Config 不为空时直接使用该配置编译（不再探测）。
	Config *ssaconfig.Config
	// Options 会应用到探测结果或 Config 上，便于复用 ssaconfig.Option。
	Options []ssaconfig.Option
	// ForceProgramName 为 true 时，编译插件优先使用 Config 中的 program_name（例如 CLI 显式 -p）。
	ForceProgramName bool
	// DisableTimestampProgramName 为 true 时，禁止编译插件自动追加时间戳到 program_name。
	DisableTimestampProgramName bool
}

// SSADetectResult ParseProjectWithAutoDetective 的返回
type SSADetectResult struct {
	Info    *AutoDetectInfo
	Program *ssaapi.Program
	// Cleanup 仅为兼容保留；默认情况下无需项目级清理。
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
