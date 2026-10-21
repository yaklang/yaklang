//go:build dameng

package damenggorm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNativeDriverRegistration(t *testing.T) {
	require.True(t, NativeEnabled())
	db, err := sql.Open(Dialect, "dm://user:password@127.0.0.1:1?schema=SSA_IR")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.True(t, nativeDriver.matches(db.Driver()))
	require.Implements(t, (*driver.DriverContext)(nil), db.Driver())
}

func TestNativeHandshakeCancellationClosesSocket(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	peerDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			peerDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		// Wait for the real native driver's handshake before cancellation.
		var first [1]byte
		if _, err := io.ReadFull(conn, first[:]); err != nil {
			peerDone <- err
			return
		}
		cancel()
		_, err = io.Copy(io.Discard, conn)
		peerDone <- err
	}()
	done := make(chan error, 1)
	go func() {
		db, err := OpenDBContext(ctx, "dm://user:password@"+listener.Addr().String()+"?schema=SSA_IR")
		if db != nil {
			_ = db.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(7 * time.Second):
		t.Fatal("canceled DM handshake did not return")
	}
	select {
	case err := <-peerDone:
		require.NoError(t, err, "the peer must observe socket closure")
	case <-time.After(time.Second):
		t.Fatal("canceled DM handshake left its socket open")
	}
}
