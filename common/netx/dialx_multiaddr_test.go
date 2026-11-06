package netx

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestInterleaveTCPAddrs(t *testing.T) {
	for _, tc := range []struct{ in, want []string }{
		{[]string{"::1", "::2", "127.0.0.1", "127.0.0.2"}, []string{"::1", "127.0.0.1", "::2", "127.0.0.2"}},
		{[]string{"127.0.0.1", "127.0.0.2", "::1"}, []string{"127.0.0.1", "::1", "127.0.0.2"}},
	} {
		if got := interleaveTCPAddrs(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%v -> %v, want %v", tc.in, got, tc.want)
		}
	}
}

// A legacy custom dialer has no context parameter and may finish after the
// winning address. It must neither hold up the winner nor leak its connection.
func TestDialTCPAddrsReturnsWinnerAndClosesLateLoser(t *testing.T) {
	late, latePeer := net.Pipe()
	winner, winnerPeer := net.Pipe()
	defer latePeer.Close()
	defer winnerPeer.Close()
	defer winner.Close()
	release := make(chan struct{})
	started := make(chan struct{})
	returned := make(chan struct{})
	cfg := &dialXConfig{Timeout: 2 * time.Second, Dialer: func(_ time.Duration, target string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		if host == "::1" {
			close(started)
			<-release
			defer close(returned)
			return late, nil
		}
		if host == "127.0.0.1" {
			return winner, nil
		}
		return nil, errors.New("unexpected address: " + host)
	}}
	type result struct {
		conn net.Conn
		err  error
	}
	results := make(chan result, 1)
	go func() {
		c, e := dialTCPAddrs(context.Background(), []string{"::1", "127.0.0.1"}, "80", cfg, nil)
		results <- result{c, e}
	}()
	<-started
	select {
	case got := <-results:
		close(release)
		if got.err != nil || got.conn != winner {
			t.Fatalf("winner %v err=%v", got.conn, got.err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("winner waited for blocked loser")
	}
	<-returned
	if err := latePeer.SetReadDeadline(time.Now().Add(time.Second)); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if _, err := latePeer.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("loser not closed: %v", err)
	}
}

func TestDialTCPAddrsCancellationClosesLateConnection(t *testing.T) {
	late, peer := net.Pipe()
	defer peer.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	cfg := &dialXConfig{Timeout: time.Second, Dialer: func(_ time.Duration, target string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			return nil, err
		}
		if host != "::1" {
			return nil, errors.New("unused fallback")
		}
		close(started)
		<-release
		return late, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		c, e := dialTCPAddrs(ctx, []string{"::1", "127.0.0.1"}, "80", cfg, nil)
		if c != nil {
			c.Close()
		}
		result <- e
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		close(release)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error: %v", err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("cancellation waited for blocked dialer")
	}
	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if _, err := peer.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("canceled connection not closed: %v", err)
	}
}

func TestCandidateAddrsPreservesDNSPolicy(t *testing.T) {
	cfg := &dialXConfig{DNSOpts: []DNSOption{WithTemporaryHosts(map[string]string{"dial-address-policy.invalid": "127.0.0.7"})}}
	got := candidateAddrs(context.Background(), "dial-address-policy.invalid", cfg)
	if !reflect.DeepEqual(got, []string{"127.0.0.7"}) {
		t.Fatalf("hosts override: %v", got)
	}
	cfg.DNSOpts = append(cfg.DNSOpts, WithDNSDisabledDomain("dial-address-policy.invalid"))
	if got := candidateAddrs(context.Background(), "dial-address-policy.invalid", cfg); len(got) != 0 {
		t.Fatalf("disabled domain resolved: %v", got)
	}
}
