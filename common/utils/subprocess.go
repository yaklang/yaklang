package utils

import (
	"context"
	"io"
	"os/exec"
	"time"
)

type SubProcess struct {
	Ctx                                context.Context
	Cmd                                *exec.Cmd
	StderrTickReader, StdoutTickReader io.Reader
	stderrTickCallback                 func(interface{})
	stdoutTickCallback                 func(interface{})

	stdout, stderr []byte
}

func (s *SubProcess) Start() error {
	return s.Cmd.Start()
}

func (s *SubProcess) Run() error {
	return s.Cmd.Run()
}

func (s *SubProcess) CombinedOutput() ([]byte, error) {
	return s.Cmd.CombinedOutput()
}

func NewSubProcess(ctx context.Context, name string, args ...string) *SubProcess {
	process := &SubProcess{
		Ctx: ctx,
		Cmd: exec.CommandContext(ctx, name, args...),
	}

	// 设置 StdoutPipe
	reader, err := process.Cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	process.StdoutTickReader = reader
	go ReadWithContextTickCallback(process.Ctx, process.StdoutTickReader, func(i []byte) bool {
		if process.stdoutTickCallback != nil {
			process.stdoutTickCallback(i)
		}
		process.stdout = i
		return true
	}, 2*time.Second)

	reader, err = process.Cmd.StderrPipe()
	if err != nil {
		panic(err)
	}
	process.StderrTickReader = reader
	go ReadWithContextTickCallback(process.Ctx, process.StderrTickReader, func(i []byte) bool {
		if process.stderrTickCallback != nil {
			process.stderrTickCallback(i)
		}
		process.stderr = i
		return true
	}, 2*time.Second)

	return process
}

func (s *SubProcess) SetStdoutTickCallback(cb func(raws interface{})) {
	s.stdoutTickCallback = cb
}

func (s *SubProcess) SetStderrTickCallback(cb func(raws interface{})) {
	s.stderrTickCallback = cb
}

func (s *SubProcess) GetStdout() []byte {
	return s.stdout
}

func (s *SubProcess) GetStderr() []byte {
	return s.stderr
}
