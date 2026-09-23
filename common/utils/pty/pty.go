// Package pty provides the PTY operations used by Yaklang's terminal.
// It is adapted from github.com/aymanbagabas/go-pty v0.2.2 (MIT licensed).
package pty

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrInvalidCommand is returned when the command is invalid.
	ErrInvalidCommand = errors.New("pty: invalid command")

	// ErrUnsupported is returned when the platform is unsupported.
	ErrUnsupported = errors.New("pty: unsupported platform")
)

// New returns a new pseudo-terminal.
func New() (Pty, error) {
	return newPty()
}

// Pty is the small portion of go-pty used by Yaklang's interactive terminal.
type Pty interface {
	io.ReadWriteCloser

	// CommandContext returns a command that can be used to start a process
	// attached to the pseudo-terminal.
	CommandContext(ctx context.Context, name string, args ...string) *Cmd

	// Resize resizes the pseudo-terminal.
	Resize(width int, height int) error
}
