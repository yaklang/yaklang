package aitool

import "io"

type ToolInvokeConfig struct {
	stdout io.Writer
	stderr io.Writer
}

func NewToolInvokeConfig() *ToolInvokeConfig {
	return &ToolInvokeConfig{}
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
