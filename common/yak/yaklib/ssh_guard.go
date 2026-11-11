package yaklib

import (
	"bytes"
	"context"
	"errors"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils/sshguard"
	"golang.org/x/crypto/ssh"
)

type sshCommandBinding struct {
	ctx               context.Context
	invocationContext func() context.Context
	authorize         sshguard.Guard
}

// SSHExportsForContext supplies session-owned connectors to an engine without
// changing the shared export table used by standalone scripts.
func SSHExportsForContext(ctx context.Context) map[string]interface{} {
	return SSHExportsForInvocationContext(ctx, nil)
}

// SSHExportsForInvocationContext keeps the authorization binding on the
// connector and resolves the context of the Yak frame that actually calls Run.
func SSHExportsForInvocationContext(ctx context.Context, invocationContext func() context.Context) map[string]interface{} {
	guard, required := sshguard.FromContext(ctx)
	if !required {
		return nil
	}
	binding := &sshCommandBinding{ctx: ctx, invocationContext: invocationContext, authorize: guard}
	bind := func(client *SSHClient, err error) (*SSHClient, error) {
		if err == nil {
			client.guard = binding
		}
		return client, err
	}
	exports := make(map[string]interface{}, len(SSHExports))
	for name, value := range SSHExports {
		exports[name] = value
	}
	exports["Connect"] = func(host string, opts ...SSHOption) (*SSHClient, error) {
		return bind(SSHConnect(host, opts...))
	}
	exports["ConnectWithKey"] = func(host, user, path string) (*SSHClient, error) {
		return bind(SSHConnectWithKey(host, user, path))
	}
	exports["ConnectWithPasswd"] = func(host, user, password string) (*SSHClient, error) {
		return bind(SSHConnectWithPasswd(host, user, password))
	}
	return exports
}

const sshAuditOutputLimit = 4096

type sshAuditBuffer struct {
	buffer    bytes.Buffer
	truncated bool
}

func (b *sshAuditBuffer) Write(data []byte) (int, error) {
	n := len(data)
	if available := sshAuditOutputLimit - b.buffer.Len(); n > available {
		b.truncated = true
		data = data[:available]
	}
	_, _ = b.buffer.Write(data)
	return n, nil
}

func (s *SSHClient) runGuarded(command string) (string, error) {
	ctx := s.guard.ctx
	if s.guard.invocationContext != nil {
		ctx = s.guard.invocationContext()
		if !sshguard.SameScope(s.guard.ctx, ctx) {
			return "", errors.New("SSH command has no managed invocation context")
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.guard.authorize == nil {
		return "", errors.New("SSH command guard is unavailable")
	}
	action := sshguard.Command{
		Target: s.target, Peer: s.client.RemoteAddr().String(), User: s.username, Command: command,
	}
	if !utf8.ValidString(action.Command) || !utf8.ValidString(action.Target) || !utf8.ValidString(action.User) {
		return "", errors.New("SSH action must be valid UTF-8")
	}
	sshguard.SetInvocation(ctx, &action)
	finish, err := s.guard.authorize(ctx, action)
	if err != nil {
		return "", err
	}
	if finish == nil {
		return "", errors.New("SSH command guard returned no result recorder")
	}

	result := sshguard.Result{Status: "cancelled"}
	defer func() { finish(result) }()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// Closing this connection prevents a cancelled invocation from sending any
	// later lines. A sent command may still run after the connection closes.
	stop := context.AfterFunc(ctx, func() { _ = s.client.Close() })
	defer stop()
	var stderr sshAuditBuffer
	output, err := s.client.Cmd(command).SetStdio(nil, &stderr).Output()
	var stdout sshAuditBuffer
	_, _ = stdout.Write(output)
	result = sshguard.Result{
		Status: "succeeded", Stdout: stdout.buffer.String(), Stderr: stderr.buffer.String(),
		Truncated: stdout.truncated || stderr.truncated,
	}
	code := 0
	result.ExitCode = &code
	if err != nil {
		result.Status = "unknown"
		result.ExitCode = nil
		var exit *ssh.ExitError
		if errors.As(err, &exit) {
			result.Status = "failed"
			code = exit.ExitStatus()
			result.ExitCode = &code
		}
		return "", err
	}
	return string(output), nil
}
