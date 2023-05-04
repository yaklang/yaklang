package permutil

import (
	"context"
	"io"
	"os"
)

type SudoConfig struct {
	// 提示语，一般用于告诉用户这个 Sudo 是用来干嘛的
	Verbose string

	// CWD: 命令执行的目录是啥
	Workdir string

	// Env
	Environments map[string]string

	// 用于控制生命周期
	Ctx context.Context

	Stdout, Stderr  io.Writer
	ExitCodeHandler func(int)
}

type SudoOption func(config *SudoConfig)

func WithVerbose(i string) SudoOption {
	return func(config *SudoConfig) {
		config.Verbose = i
	}
}

func WithWorkdir(i string) SudoOption {
	return func(config *SudoConfig) {
		config.Workdir = i
	}
}

func WithContext(ctx context.Context) SudoOption {
	return func(config *SudoConfig) {
		config.Ctx = ctx
	}
}

func WithEnv(k, v string) SudoOption {
	return func(config *SudoConfig) {
		if config.Environments == nil {
			config.Environments = make(map[string]string)
		}
		config.Environments[k] = v
	}
}

func NewDefaultSudoConfig() *SudoConfig {
	return &SudoConfig{
		Verbose:      "Auth(or Password) Required",
		Workdir:      os.TempDir(),
		Environments: make(map[string]string),
		Ctx:          context.Background(),
	}
}

func WithStdout(w io.Writer) SudoOption {
	return func(config *SudoConfig) {
		config.Stdout = w
	}
}

func WithStderr(w io.Writer) SudoOption {
	return func(config *SudoConfig) {
		config.Stderr = w
	}
}

func WithExitCodeHandler(i func(int)) SudoOption {
	return func(config *SudoConfig) {
		config.ExitCodeHandler = i
	}
}
