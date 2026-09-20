package oracleprobe

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestProbeTimeoutBudget(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second, time.Hour, 40 * time.Millisecond} {
		t.Run(timeout.String(), func(t *testing.T) {
			sentinel := errors.New("dial inspected")
			err := Probe(context.Background(), dialFunc(func(ctx context.Context, _, _ string) (net.Conn, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > MaxTimeout {
					t.Fatalf("unbounded deadline: %v", deadline)
				}
				if timeout > 0 && timeout < MaxTimeout && time.Until(deadline) > timeout {
					t.Fatal("short timeout was enlarged")
				}
				return nil, sentinel
			}), Options{Address: "127.0.0.1:1521", Service: "MOCK", Username: "PROBE", Timeout: timeout})
			if !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
		})
	}
}

func TestProbeStalledHandshake(t *testing.T) {
	for _, stage := range []string{"connect", "protocol", "datatype", "challenge", "result", "tls", "write", "dribble"} {
		t.Run(stage, func(t *testing.T) {
			client, server := net.Pipe()
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer server.Close()
				switch stage {
				case "tls":
					_, _ = io.Copy(io.Discard, server)
				case "write":
					// Do not read the client's CONNECT. The write must obey the same budget.
					<-time.After(300 * time.Millisecond)
				case "dribble":
					s := &session{conn: server}
					if _, err := s.packet(); err != nil {
						return
					}
					for _, b := range acceptPacket() {
						if _, err := server.Write([]byte{b}); err != nil {
							return
						}
						time.Sleep(20 * time.Millisecond)
					}
				default:
					_ = mockLoginServer(server, "stall_"+stage)
				}
			}()
			o := Options{Address: "127.0.0.1:1521", Service: "MOCK", Username: "PROBE", Password: "Mock_密码!123", Timeout: 100 * time.Millisecond}
			if stage == "tls" {
				o.TLS = &tls.Config{ServerName: "oracle.test"}
			}
			start := time.Now()
			err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { return client, nil }), o)
			var networkError net.Error
			if !errors.Is(err, context.DeadlineExceeded) && !(errors.As(err, &networkError) && networkError.Timeout()) {
				t.Fatalf("expected deadline at %s, got %v", stage, err)
			}
			if time.Since(start) > time.Second {
				t.Fatal("probe exceeded its shared budget")
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("server connection leaked")
			}
		})
	}
}

func TestProbeRedirectSharesDeadline(t *testing.T) {
	var first time.Time
	calls := 0
	redirect := []byte("(ADDRESS=(PROTOCOL=TCP)(HOST=localhost)(PORT=1522))")
	payload := append([]byte{0, byte(len(redirect))}, redirect...)
	err := Probe(context.Background(), dialFunc(func(ctx context.Context, _, _ string) (net.Conn, error) {
		deadline, _ := ctx.Deadline()
		calls++
		if calls == 1 {
			first = deadline
			return wireSession(wirePacket(0, 5, payload)).conn, nil
		}
		if !deadline.Equal(first) {
			t.Fatal("redirect restarted timeout")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}), Options{Address: "127.0.0.1:1521", Service: "MOCK", Username: "PROBE", Timeout: 50 * time.Millisecond})
	if calls != 2 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
