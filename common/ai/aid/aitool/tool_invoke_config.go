package aitool

import (
	"context"
	"io"
)

type toolInvokeHook func(t *Tool, params map[string]any, config *ToolInvokeConfig) (*ToolResult, error)

type ToolInvokeConfig struct {
	invokeCtx  *ToolInvokeCtx
	stdout     io.Writer
	stderr     io.Writer
	invokeHook toolInvokeHook // hook toolCall
}

func (i ToolInvokeConfig) GetStdout() io.Writer {
	return i.stdout
}

func (i ToolInvokeConfig) GetStderr() io.Writer {
	return i.stderr
}

func NewToolInvokeConfig(ctx context.Context) *ToolInvokeConfig {
	return &ToolInvokeConfig{
		invokeCtx: &ToolInvokeCtx{
			Ctx: ctx,
		},
	}
}

type ToolInvokeOptions func(*ToolInvokeConfig)

func WithStdout(stdout io.Writer) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.stdout = stdout
	}
}

func WithStderr(stderr io.Writer) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.stderr = stderr
	}
}

func WithChatToAiFunc(chatToAiFunc ChatToAiFuncType) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.invokeCtx.ChatToAiFunc = chatToAiFunc
	}
}

func WithInvokeHook(hook toolInvokeHook) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.invokeHook = hook
	}
}
