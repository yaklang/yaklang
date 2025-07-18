package aitool

import (
	"context"
	"io"
)

type ToolCallCancelCallback func(result *ToolExecutionResult, err error) (*ToolExecutionResult, error)

type ToolInvokeConfig struct {
	ctx            context.Context
	stdout         io.Writer
	stderr         io.Writer
	errCallback    func(error) (*ToolResult, error)
	resCallback    func(result *ToolExecutionResult) (*ToolResult, error)
	cancelCallback ToolCallCancelCallback
	runtimeConfig  *ToolRuntimeConfig
}

func (i *ToolInvokeConfig) GetErrCallback() func(error) (*ToolResult, error) {
	if i == nil {
		return nil
	}
	return i.errCallback
}

func (i *ToolInvokeConfig) GetResCallback() func(result *ToolExecutionResult) (*ToolResult, error) {
	if i == nil {
		return nil
	}
	return i.resCallback
}

func (i *ToolInvokeConfig) GetCancelCallback() ToolCallCancelCallback {
	if i == nil {
		return nil
	}
	return i.cancelCallback
}

func (i *ToolInvokeConfig) GetRuntimeConfig() *ToolRuntimeConfig {
	if i == nil {
		return nil
	}
	return i.runtimeConfig
}

func (i *ToolInvokeConfig) GetStdout() io.Writer {
	if i == nil {
		return nil
	}
	return i.stdout
}

func (i *ToolInvokeConfig) GetStderr() io.Writer {
	if i == nil {
		return nil
	}
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

func WithCancelCallback(callback ToolCallCancelCallback) ToolInvokeOptions {
	return func(config *ToolInvokeConfig) {
		config.cancelCallback = callback
	}
}

func WithRuntimeConfig(config *ToolRuntimeConfig) ToolInvokeOptions {
	return func(toolConfig *ToolInvokeConfig) {
		toolConfig.runtimeConfig = config
	}
}
