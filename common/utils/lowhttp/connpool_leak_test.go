package lowhttp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func TestConnPool_NoDeadlockOnTimeout(t *testing.T) {
	var port int
	var lis net.Listener
	var err error
	for i := 0; i < 10; i++ {
		port = utils.GetRandomAvailableTCPPort()
		lis, err = net.Listen("tcp", utils.HostPort("127.0.0.1", port))
		if err != nil {
			t.Error(err)
			continue
		}
		break
	}
	if lis == nil {
		t.Fatal("listener is nil")
	}
	defer lis.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			conn, acceptErr := lis.Accept()
			if acceptErr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// Keep the connection open without responding.
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
				time.Sleep(2 * time.Second)
			}(conn)
		}
	}()

	pool := NewHttpConnPool(context.Background(), 1, 1)
	reqBytes := []byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort("127.0.0.1", port)))

	assertNoGoroutineLeak(t, "connpool timeout deadlock", func() {
		const reqCount = 3
		var wg sync.WaitGroup
		errs := make(chan error, reqCount)
		for i := 0; i < reqCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := HTTP(
					WithPacketBytes(reqBytes),
					WithHost("127.0.0.1"),
					WithPort(port),
					WithTimeout(200*time.Millisecond),
					WithConnPool(true),
					ConnPool(pool),
				)
				errs <- err
			}()
		}
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for conn pool requests")
		}
		close(errs)
		for err := range errs {
			if err == nil {
				t.Fatal("expected timeout error")
			}
		}
	})
}
