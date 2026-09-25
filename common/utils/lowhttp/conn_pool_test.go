package lowhttp

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"

	"golang.org/x/net/http2"
)

func TestH2FramerRejectsFramesAboveAdvertisedMaxSize(t *testing.T) {
	localConn, remoteConn := net.Pipe()
	defer localConn.Close()
	defer remoteConn.Close()

	pool := NewHttpConnPool(context.Background(), 100, 2)
	entry := &h2ConnEntry{
		conn:     localConn,
		cacheKey: &connectKey{scheme: H2},
		pool:     pool.h2Pool,
	}
	pool.h2Pool.initH2Conn(entry)
	defer entry.alt.idleTimer.Stop()
	defer entry.alt.setClose()

	frameHeader := make([]byte, 9)
	binary.BigEndian.PutUint32(frameHeader[:4], (defaultMaxFrameSize+1)<<8)
	frameHeader[4] = byte(http2.FrameData)
	binary.BigEndian.PutUint32(frameHeader[5:], 1)
	go func() {
		_, _ = io.Copy(remoteConn, bytes.NewReader(frameHeader))
	}()

	_, err := entry.alt.fr.ReadFrame()
	if !errors.Is(err, http2.ErrFrameTooLarge) {
		t.Fatalf("ReadFrame() error = %v, want %v", err, http2.ErrFrameTooLarge)
	}
}
