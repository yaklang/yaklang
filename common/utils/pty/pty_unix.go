//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build darwin dragonfly freebsd linux netbsd openbsd solaris

package pty

import (
	"context"
	"errors"
	"os"

	creackpty "github.com/creack/pty"
)

// unixPty is a POSIX compliant Unix pseudo-terminal.
// See: https://pubs.opengroup.org/onlinepubs/9699919799/
type unixPty struct {
	master, slave *os.File
	closed        bool
}

var _ Pty = &unixPty{}

// Close implements Pty.
func (p *unixPty) Close() error {
	if p.closed {
		return nil
	}
	defer func() {
		p.closed = true
	}()
	return errors.Join(p.master.Close(), p.slave.Close())
}

// CommandContext implements Pty.
func (p *unixPty) CommandContext(ctx context.Context, name string, args ...string) *Cmd {
	if ctx == nil {
		panic("nil context")
	}
	return &Cmd{ctx: ctx, pty: p, Path: name, Args: append([]string{name}, args...)}
}

// Read implements Pty.
func (p *unixPty) Read(b []byte) (n int, err error) {
	return p.master.Read(b)
}

// Resize implements Pty.
func (p *unixPty) Resize(width int, height int) error {
	return creackpty.Setsize(p.master, &creackpty.Winsize{Rows: uint16(height), Cols: uint16(width)})
}

// Write implements Pty.
func (p *unixPty) Write(b []byte) (n int, err error) {
	return p.master.Write(b)
}

func newPty() (Pty, error) {
	master, slave, err := creackpty.Open()
	if err != nil {
		return nil, err
	}

	return &unixPty{
		master: master,
		slave:  slave,
	}, nil
}
