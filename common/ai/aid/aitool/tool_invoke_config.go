package aitool

import (
	"context"
	"io"
)

type toolCallCancelCallback func(result *ToolExecutionResult, err error) (*ToolExecutionResult, error)

type ToolInvokeConfig struct {
	ctx            context.Context
	stdout         io.Writer
	stderr         io.Writer
	errCallback    func(error) (*ToolResult, error)
	resCallback    func(result *ToolExecutionResult) (*ToolResult, error)
	cancelCallback toolCallCancelCallback
}

func (i ToolInvokeConfig) GetStdout() io.Writer {
	return i.stdout
}

func (i ToolInvokeConfig) GetStderr() io.Writer {
	return i.stderr
}

func NewToolInvokeConfig() *ToolInvokeConfig {
	return &ToolInvokeConfig{
		ctx: context.Background(),
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

func WithContext(ctx context.Context) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.ctx = ctx
	}
}

func WithErrorCallback(callback func(error) (*ToolResult, error)) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.errCallback = callback
	}
}

func WithResultCallback(callback func(result *ToolExecutionResult) (*ToolResult, error)) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.resCallback = callback
	}
}

func WithCancelCallback(callback toolCallCancelCallback) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.cancelCallback = callback
	}
}
