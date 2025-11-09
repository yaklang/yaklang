package privileged

// IsPrivilegedGlobal 全局变量，在包初始化时检测当前进程是否具有特权
// 注意：这个值在程序启动时确定，不会动态更新
// 如果需要实时检测，请使用 IsPrivileged() 函数
var IsPrivilegedGlobal = false

func init() {
	IsPrivilegedGlobal = isPrivileged()
}

// IsPrivileged 检测当前进程是否具有特权（管理员/root权限）
// 这个函数会实时检测当前进程的权限状态
//
// 返回值：
//   - true: 当前进程具有管理员/root权限
//   - false: 当前进程是普通用户权限
//
// 平台说明：
//   - Windows: 检测进程是否通过 UAC 提升为管理员权限
//   - Linux: 检测进程是否具有 root 权限或 CAP_NET_RAW 能力
//   - macOS: 检测进程的有效用户ID是否为0（root）
func IsPrivileged() bool {
	return isPrivileged()
}

// GetIsPrivileged 已废弃：请使用 IsPrivileged() 函数
// 保留此函数是为了向后兼容
//
// Deprecated: 使用 IsPrivileged() 代替
func GetIsPrivileged() bool {
	return isPrivileged()
}

// BeforeExecuteHandler 在特权进程真正启动之前被调用
// 这个回调在 osascript 获得权限并即将执行命令之前触发
type BeforeExecuteHandler func()

type ExecuteConfig struct {
	Title                          string
	Prompt                         string
	Description                    string
	DiscardStdoutStderr            bool
	SkipConfirmDialog              bool                 // 跳过第一个确认对话框，直接弹出系统管理员权限验证对话框
	BeforePrivilegedProcessExecute BeforeExecuteHandler // 在特权进程启动前的回调
}

func DefaultExecuteConfig() *ExecuteConfig {
	return &ExecuteConfig{
		Title:                          "privilege execute",
		Prompt:                         "",
		Description:                    "",
		DiscardStdoutStderr:            false,
		SkipConfirmDialog:              false,
		BeforePrivilegedProcessExecute: nil,
	}
}

type ExecuteOption func(*ExecuteConfig)

func WithTitle(title string) ExecuteOption {
	return func(c *ExecuteConfig) {
		c.Title = title
	}
}

func WithPrompt(prompt string) ExecuteOption {
	return func(c *ExecuteConfig) {
		c.Prompt = prompt
	}
}

func WithDescription(description string) ExecuteOption {
	return func(c *ExecuteConfig) {
		c.Description = description
	}
}

// WithDiscardStdoutAndStderr 丢弃 stdout 和 stderr，不收集它们
func WithDiscardStdoutAndStderr() ExecuteOption {
	return func(c *ExecuteConfig) {
		c.DiscardStdoutStderr = true
	}
}

// WithSkipConfirmDialog 跳过第一个确认对话框，直接弹出系统管理员权限验证对话框
// 这可以减少用户的点击次数，提升用户体验
func WithSkipConfirmDialog() ExecuteOption {
	return func(c *ExecuteConfig) {
		c.SkipConfirmDialog = true
	}
}

// WithBeforePrivilegedProcessExecute 设置在特权进程真正启动之前的回调
// 这个回调在 osascript 获得权限并即将执行命令之前触发
// 可以用于在进程启动后开始轮询检查某些资源（如 socket）是否创建成功
func WithBeforePrivilegedProcessExecute(handler BeforeExecuteHandler) ExecuteOption {
	return func(c *ExecuteConfig) {
		c.BeforePrivilegedProcessExecute = handler
	}
}
