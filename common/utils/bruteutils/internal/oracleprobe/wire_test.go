package oracleprobe

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type memoryConn struct {
	*bytes.Reader
	written bytes.Buffer
	closed  bool
}

func (c *memoryConn) Write(p []byte) (int, error)      { return c.written.Write(p) }
func (c *memoryConn) Close() error                     { c.closed = true; return nil }
func (c *memoryConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *memoryConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *memoryConn) SetDeadline(time.Time) error      { return nil }
func (c *memoryConn) SetReadDeadline(time.Time) error  { return nil }
func (c *memoryConn) SetWriteDeadline(time.Time) error { return nil }

type dialFunc func(context.Context, string, string) (net.Conn, error)

func (f dialFunc) DialContext(c context.Context, n, a string) (net.Conn, error) { return f(c, n, a) }

func wirePacket(version uint16, typ byte, data []byte) []byte {
	b := make([]byte, 8)
	b[4] = typ
	if version >= 315 {
		binary.BigEndian.PutUint32(b, uint32(8+len(data)))
	} else {
		binary.BigEndian.PutUint16(b, uint16(8+len(data)))
	}
	return append(b, data...)
}
func wireSession(data []byte) *session {
	return &session{conn: &memoryConn{Reader: bytes.NewReader(data)}, sdu: 8192, ClrChunkSize: 64}
}
func acceptPacket() []byte {
	b := make([]byte, 24)
	binary.BigEndian.PutUint16(b, 314)
	binary.BigEndian.PutUint16(b[4:], 8192)
	return wirePacket(0, 2, b)
}

func TestPacketBounds(t *testing.T) {
	for _, v := range []uint16{314, 315} {
		for _, n := range []int{0, 7, maxField + 1} {
			if v < 315 && n > 65535 {
				continue
			}
			b := make([]byte, 8)
			if v >= 315 {
				binary.BigEndian.PutUint32(b, uint32(n))
			} else {
				binary.BigEndian.PutUint16(b, uint16(n))
			}
			s := wireSession(b)
			s.version = v
			if _, e := s.packet(); e == nil {
				t.Fatalf("accepted packet length %d", n)
			}
		}
	}
	s := wireSession(wirePacket(0, 6, []byte{0, 0, 1}))
	s.received = maxExchange - 8
	if _, e := s.packet(); e == nil {
		t.Fatal("exchange budget ignored")
	}
	s = wireSession(nil)
	s.packets = 256
	if _, e := s.packet(); e == nil {
		t.Fatal("packet budget ignored")
	}
}

func TestCLRFragmentsAndTruncation(t *testing.T) {
	// A single CLR field crosses TNS packet boundaries, including its length.
	data := append(wirePacket(0, 6, []byte{0, 0, 0xfe, 3, 'a'}), wirePacket(0, 6, []byte{0, 0, 'b', 'c', 2, 'd', 'e', 0})...)
	s := wireSession(data)
	b, e := s.GetClr()
	if e != nil || string(b) != "abcde" {
		t.Fatalf("%q %v", b, e)
	}
	for _, b := range [][]byte{{0xfe, 3, 'a'}, {9}, {0x81, 1}, {1, 4, 2, 'a', 'b'}} {
		s = wireSession(wirePacket(0, 6, append([]byte{0, 0}, b...)))
		_, e = s.GetDlc()
		if e == nil || s.err == nil {
			t.Fatalf("accepted malformed DLC %x", b)
		}
		if _, e = s.GetByte(); e == nil {
			t.Fatal("lost sticky error")
		}
	}
}

func TestConnectResendRedirectAndRefuse(t *testing.T) {
	redirect := []byte("(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.2)(PORT=1522))")
	p := binary.BigEndian.AppendUint16(nil, uint16(len(redirect)))
	first := &memoryConn{Reader: bytes.NewReader(wirePacket(0, 5, append(p, redirect...)))}
	second := &memoryConn{Reader: bytes.NewReader(append(wirePacket(0, 11, nil), acceptPacket()...))}
	calls := 0
	d := dialFunc(func(_ context.Context, _, address string) (net.Conn, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		if calls != 2 || address != "127.0.0.2:1522" {
			t.Fatalf("unexpected redirect %d %s", calls, address)
		}
		return second, nil
	})
	s, e := connect(context.Background(), d, Options{Address: "127.0.0.1:1521"}, "(DESCRIPTION=probe)", nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.conn.Close()
	if !first.closed || calls != 2 || s.version != 314 {
		t.Fatal("redirect did not transfer connection ownership")
	}
	if bytes.Count(second.written.Bytes(), []byte("(DESCRIPTION=probe)")) != 2 {
		t.Fatal("CONNECT was not resent")
	}
	msg := []byte("(DESCRIPTION=(ERR=12514))")
	b := binary.BigEndian.AppendUint16([]byte{0, 0}, uint16(len(msg)))
	c := &memoryConn{Reader: bytes.NewReader(wirePacket(0, 4, append(b, msg...)))}
	_, e = connect(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) { return c, nil }), Options{Address: "localhost:1521"}, "test", nil)
	var ora *Error
	if !errors.As(e, &ora) || !ora.ServiceUnknown() || !c.closed {
		t.Fatalf("REFUSE: %v, closed=%v", e, c.closed)
	}
}

func TestProbeCancellationClosesConnection(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := make(chan error, 1)
	go func() {
		h := make([]byte, 8)
		_, e := io.ReadFull(server, h)
		if e == nil {
			_, e = io.CopyN(io.Discard, server, int64(binary.BigEndian.Uint16(h)-8))
		}
		cancel()
		if e == nil {
			_, e = server.Read(h)
		}
		closed <- e
	}()
	e := Probe(ctx, dialFunc(func(context.Context, string, string) (net.Conn, error) { return client, nil }), Options{Address: "127.0.0.1:1521", Service: "XE", Username: "probe", Password: "test"})
	if !errors.Is(e, context.Canceled) {
		t.Fatalf("cancellation: %v", e)
	}
	select {
	case e := <-closed:
		if !errors.Is(e, io.EOF) {
			t.Fatal(e)
		}
	case <-time.After(time.Second):
		t.Fatal("owned connection leaked")
	}
}

func authResult(properties map[string]string, done bool) []byte {
	w := &session{ClrChunkSize: 64}
	w.PutBytes(8)
	w.PutInt(len(properties), 4, true, true)
	for k, v := range properties {
		w.PutKeyValString(k, v, 0)
	}
	if done {
		w.PutBytes(9)
	}
	return wirePacket(0, 6, append([]byte{0, 0}, w.out.Bytes()...))
}
func TestAuthenticationRequiresCompleteSession(t *testing.T) {
	key := bytes.Repeat([]byte{0x31}, 16)
	clear := append(bytes.Repeat([]byte{0x52}, 16), []byte("SERVER_TO_CLIENT")...)
	clear = append(clear, bytes.Repeat([]byte{16}, 16)...)
	blk, _ := aes.NewCipher(key)
	proof := make([]byte, len(clear))
	cipher.NewCBCEncrypter(blk, make([]byte, 16)).CryptBlocks(proof, clear)
	for _, test := range []struct {
		name        string
		props       map[string]string
		done, valid bool
	}{
		{"empty", nil, true, false},
		{"partial", map[string]string{"AUTH_SESSION_ID": "1"}, true, false},
		{"truncated", map[string]string{"AUTH_SESSION_ID": "1", "AUTH_SERIAL_NUM": "2"}, false, false},
		{"invalid_proof", map[string]string{"AUTH_SESSION_ID": "1", "AUTH_SERIAL_NUM": "2", "AUTH_SVR_RESPONSE": "00"}, true, false},
		{"missing_proof", map[string]string{"AUTH_SESSION_ID": "1", "AUTH_SERIAL_NUM": "2"}, true, false},
		{"complete", map[string]string{"AUTH_SESSION_ID": "1", "AUTH_SERIAL_NUM": "2", "AUTH_SVR_RESPONSE": hex.EncodeToString(proof)}, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := &Connection{session: wireSession(authResult(test.props, test.done))}
			e := c.readAuthResult(&AuthObject{KeyHash: key})
			if (e == nil) != test.valid {
				t.Fatalf("result: %v", e)
			}
		})
	}
}

func FuzzLoginWire(f *testing.F) {
	f.Add([]byte{})
	f.Add(acceptPacket())
	f.Add(wirePacket(0, 6, []byte{0, 0, 0xfe, 3, 1, 2, 3, 0}))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > maxExchange {
			t.Skip()
		}
		for op := 0; op < 7; op++ {
			s := wireSession(b)
			switch op {
			case 0:
				_, _ = s.packet()
			case 1:
				_, _ = s.GetClr()
			case 2:
				_, _, _, _ = s.GetKeyVal()
			case 3:
				c := &Connection{session: s}
				_ = c.readAuthResult(&AuthObject{KeyHash: make([]byte, 16)})
			case 4:
				_ = s.negotiateAdvanced()
			case 5:
				c := &Connection{session: s}
				_ = (&TCPNego{conn: c}).readMessage()
			case 6:
				c := &Connection{session: s, ctx: context.Background(), connOption: &loginOptions{UserID: "PROBE", Password: "dummy"}}
				_ = (&AuthObject{conn: c, tcpNego: &TCPNego{ServerCompileTimeCaps: make([]byte, 8)}}).read()
			}
		}
	})
}

func FuzzSummaryAndSideband(f *testing.F) {
	f.Add(byte(8), byte(0), make([]byte, 32))
	f.Add(byte(6), byte(1), []byte{0xff})
	f.Fuzz(func(t *testing.T, version, flags byte, b []byte) {
		if len(b) > maxField {
			t.Skip()
		}
		s := wireSession(nil)
		s.in.Write(b)
		s.TTCVersion = version
		s.HasEOSCapability = flags&1 != 0
		s.HasFSAPCapability = flags&2 != 0
		_, _ = NewSummary(s)
		s = wireSession(nil)
		s.in.Write(b)
		_ = (&Connection{session: s}).getServerNetworkInformation(flags)
	})
}

func TestProbeRejectsDescriptorInjection(t *testing.T) {
	for _, service := range []string{"XE)(CONNECT_DATA=(SERVICE_NAME=other)", "XE\x00", "XE\n"} {
		dialed := false
		err := Probe(context.Background(), dialFunc(func(context.Context, string, string) (net.Conn, error) {
			dialed = true
			return nil, errors.New("should not dial")
		}), Options{Address: "127.0.0.1:1521", Service: service, Username: "probe"})
		if err == nil || dialed {
			t.Fatal("descriptor value reached dialer")
		}
	}
}

func TestProbeConcurrentCancellation(t *testing.T) {
	var workers sync.WaitGroup
	for n := 0; n < 24; n++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			client, server := net.Pipe()
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := Probe(ctx, dialFunc(func(context.Context, string, string) (net.Conn, error) { return client, nil }), Options{Address: "127.0.0.1:1521", Service: "XE", Username: "probe"})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("cancelled probe: %v", err)
			}
		}()
	}
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent cancellation leaked workers")
	}
}
