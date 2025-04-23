package aitool

import (
	"context"
	"io"
)

type ToolInvokeConfig struct {
	invokeCtx *ToolInvokeCtx
	stdout    io.Writer
	stderr    io.Writer
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
