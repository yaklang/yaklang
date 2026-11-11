package yaklib

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/sshguard"
	"golang.org/x/crypto/ssh"
)

func TestSSHGuardRejectsUnrelatedOrCancelledInvocation(t *testing.T) {
	ctx := sshguard.WithGuard(context.Background(), nil)
	frame := context.Background()
	client := &SSHClient{guard: &sshCommandBinding{
		ctx: ctx, invocationContext: func() context.Context { return frame },
	}}
	_, err := client.Run("id")
	require.ErrorContains(t, err, "no managed invocation context")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	frame = cancelled
	_, err = client.Run("id")
	require.ErrorIs(t, err, context.Canceled)
}

func TestSSHGuardAuthorizesEntireRequestBeforeExec(t *testing.T) {
	addr, commands := sshGuardTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	authorize := make(chan sshguard.Command, 1)
	approve := make(chan struct{})
	finished := make(chan sshguard.Result, 1)
	guard := sshguard.WithGuard(ctx, func(ctx context.Context, cmd sshguard.Command) (func(sshguard.Result), error) {
		authorize <- cmd
		select {
		case <-approve:
			return func(result sshguard.Result) { finished <- result }, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	exports := SSHExportsForContext(sshguard.WithInvocation(guard, "call-1", "task-1"))
	client, err := exports["ConnectWithPasswd"].(func(string, string, string) (*SSHClient, error))(addr, "tester", "test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	done := make(chan error, 1)
	go func() {
		out, err := client.Run("id\nhostname")
		if err == nil && out != "okok" {
			err = errors.New("unexpected stdout")
		}
		done <- err
	}()
	select {
	case cmd := <-authorize:
		require.Equal(t, sshguard.Command{
			Target: addr, Peer: addr, User: "tester", Command: "id\nhostname",
			ToolCallID: "call-1", TaskID: "task-1",
		}, cmd)
	case <-ctx.Done():
		t.Fatal("guard did not receive request")
	}
	select {
	case command := <-commands:
		t.Fatalf("command sent before authorization: %q", command)
	case <-time.After(75 * time.Millisecond):
	}
	close(approve)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("execution did not finish")
	}
	require.Equal(t, "id\n", <-commands)
	require.Equal(t, "hostname\n", <-commands)
	require.Equal(t, "succeeded", (<-finished).Status)

	denied := SSHExportsForContext(sshguard.WithGuard(ctx, nil))
	client, err = denied["Connect"].(func(string, ...SSHOption) (*SSHClient, error))(addr, WithSSHPassword("test"))
	require.NoError(t, err)
	defer client.Close()
	_, err = client.Run("id")
	require.ErrorContains(t, err, "unavailable")
	require.Empty(t, commands)

	// The standalone export has no session guard, as used by desktop clients.
	require.Nil(t, SSHExportsForContext(context.Background()))
	standalone, err := SSHConnectWithPasswd(addr, "tester", "test")
	require.NoError(t, err)
	defer standalone.Close()
	out, err := standalone.Run("id")
	require.NoError(t, err)
	require.Equal(t, "ok", out)
	require.Equal(t, "id\n", <-commands)
}

func sshGuardTestServer(t *testing.T) (string, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	commands := make(chan string, 8)
	go func() {
		for {
			socket, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer socket.Close()
				conn, channels, requests, err := ssh.NewServerConn(socket, cfg)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					channel, requests, err := incoming.Accept()
					if err != nil {
						continue
					}
					for req := range requests {
						var payload struct{ Command string }
						if req.Type != "exec" || ssh.Unmarshal(req.Payload, &payload) != nil {
							_ = req.Reply(false, nil)
							continue
						}
						commands <- payload.Command
						_ = req.Reply(true, nil)
						_, _ = channel.Write([]byte("ok"))
						_, _ = channel.Stderr().Write([]byte(strings.TrimSpace(payload.Command)))
						_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{}))
						_ = channel.Close()
						break
					}
				}
			}()
		}
	}()
	return listener.Addr().String(), commands
}
