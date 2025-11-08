package privileged

var IsPrivileged = false

func init() {
	IsPrivileged = isPrivileged()
}

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
