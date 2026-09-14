package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTCPStreamBufferLateAndConcurrentReader(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		s := newTCPStreamBuffer()
		payload := make([]byte, 4<<20+17)
		for i := range payload {
			payload[i] = byte(i*31 + i/65537)
		}
		var got bytes.Buffer
		var readErr error
		var wg sync.WaitGroup
		read := func() { defer wg.Done(); _, readErr = io.CopyBuffer(&got, s, make([]byte, 1777)) }
		wg.Add(1)
		if concurrent {
			go read()
		}
		for offset := 0; offset < len(payload); {
			n := 1 + offset%100003
			if n > len(payload)-offset {
				n = len(payload) - offset
			}
			wrote, err := s.Write(payload[offset : offset+n])
			require.NoError(t, err)
			require.Equal(t, n, wrote)
			offset += n
		}
		require.NoError(t, s.Close())
		if !concurrent {
			go read()
		}
		wg.Wait()
		require.NoError(t, readErr)
		require.Equal(t, len(payload), s.Count())
		require.Equal(t, sha256.Sum256(payload), sha256.Sum256(got.Bytes()))
		_, err := s.Write([]byte("closed"))
		require.ErrorIs(t, err, io.ErrClosedPipe)
	}
}

func TestTCPStreamBufferWakeAndReuse(t *testing.T) {
	s := newTCPStreamBuffer()
	payload := bytes.Repeat([]byte{42}, 1024)
	buf := make([]byte, len(payload))
	for i := 0; i < 1000; i++ {
		_, err := s.Write(payload)
		require.NoError(t, err)
		_, err = io.ReadFull(s, buf)
		require.NoError(t, err)
		require.Equal(t, payload, buf)
	}
	require.Same(t, s.head, s.tail)
	require.Equal(t, 1024, cap(s.head.data))
	done := make(chan error, 1)
	go func() { _, err := s.Read(buf); done <- err }()
	require.Eventually(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.waiters > 0 }, time.Second, time.Millisecond)
	require.NoError(t, s.Close())
	select {
	case err := <-done:
		require.ErrorIs(t, err, io.EOF)
	case <-time.After(time.Second):
		t.Fatal("Close did not wake the reader")
	}
	n, err := s.Read(nil)
	require.Zero(t, n)
	require.NoError(t, err)
}
