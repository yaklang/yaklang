package privileged

var IsPrivileged = false

func init() {
	IsPrivileged = isPrivileged()
}

func GetIsPrivileged() bool {
	return isPrivileged()
}

type ExecuteConfig struct {
	Title       string
	Prompt      string
	Description string
}

func DefaultExecuteConfig() *ExecuteConfig {
	return &ExecuteConfig{
		Title:       "privilege execute",
		Prompt:      "This operation requires administrator privileges",
		Description: "",
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
