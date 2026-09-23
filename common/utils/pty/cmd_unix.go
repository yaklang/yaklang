//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build darwin dragonfly freebsd linux netbsd openbsd solaris

package pty

import (
	"errors"
	"os/exec"

	"golang.org/x/sys/unix"
)

func (c *Cmd) start() error {
	if c.Process != nil {
		return errors.New("exec: already started")
	}

	pty, ok := c.pty.(*unixPty)
	if !ok {
		return ErrInvalidCommand
	}

	cmd := exec.Command(c.Path, c.Args[1:]...)
	if c.ctx != nil {
		cmd = exec.CommandContext(c.ctx, c.Path, c.Args[1:]...)
		if c.Cancel == nil {
			c.Cancel = func() error {
				return cmd.Process.Kill()
			}
		}
	}
	c.sys = cmd

	cmd.Dir = c.Dir
	cmd.Env = c.Env
	cmd.Cancel = c.Cancel
	cmd.SysProcAttr = c.SysProcAttr
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &unix.SysProcAttr{}
	}

	cmd.Stdin = pty.slave
	cmd.Stdout = pty.slave
	cmd.Stderr = pty.slave
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Setctty = true
	if err := cmd.Start(); err != nil {
		return err
	}

	c.Process = cmd.Process
	return nil
}

func (c *Cmd) wait() error {
	if c.Process == nil {
		return errors.New("exec: not started")
	}
	if c.ProcessState != nil {
		return errors.New("exec: Wait was already called")
	}

	cmd, ok := c.sys.(*exec.Cmd)
	if !ok {
		return ErrInvalidCommand
	}
	err := cmd.Wait()
	c.ProcessState = cmd.ProcessState
	return err
}
