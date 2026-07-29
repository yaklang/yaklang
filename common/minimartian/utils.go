package minimartian

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/proxyutil"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"golang.org/x/crypto/cryptobyte"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	AUTH_FINISH  = "authFinish"
	PROTO_S5     = "s5"
	PROTO_HTTP   = "HTTP"
	PROTO_TUNNEL = "TUNNEL"
)

type emptyProxyHandleReader struct{}

func (emptyProxyHandleReader) Read([]byte) (int, error) {
	return 0, io.EOF
}

type proxyHandleBuffers struct {
	brw              *bufio.ReadWriter
	contextIOBinding *proxyHandleContextIOBinding
}

var proxyHandleBufferPool sync.Pool

type proxyHandleContextIO struct {
	net.Conn
	ctx context.Context
}

func (c *proxyHandleContextIO) Read(packet []byte) (int, error) {
	n, err := c.Conn.Read(packet)
	if ctxErr := c.ctx.Err(); ctxErr != nil {
		return n, ctxErr
	}
	return n, err
}

func (c *proxyHandleContextIO) Write(packet []byte) (int, error) {
	n, err := c.Conn.Write(packet)
	if ctxErr := c.ctx.Err(); ctxErr != nil {
		return n, ctxErr
	}
	return n, err
}

type proxyHandleContextIOBinding struct {
	stop chan struct{}
	done chan struct{}
}

func bindProxyHandleContextIO(ctx context.Context, conn net.Conn) (*proxyHandleContextIO, *proxyHandleContextIOBinding) {
	if ctx == nil {
		ctx = context.Background()
	}
	contextIO := &proxyHandleContextIO{Conn: conn, ctx: ctx}
	if ctx.Done() == nil {
		return contextIO, nil
	}

	binding := &proxyHandleContextIOBinding{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go func() {
		defer close(binding.done)
		select {
		case <-ctx.Done():
			// release may race with cancellation while an HTTP or SOCKS
			// session is being rebuilt. A released binding must not poison the
			// connection that the next session is about to reuse.
			select {
			case <-binding.stop:
				return
			default:
			}
			if err := conn.SetDeadline(time.Now()); err != nil {
				// A net.Conn that cannot honor deadlines must be closed to
				// preserve the old context wrapper's prompt cancellation.
				_ = conn.Close()
			}
		case <-binding.stop:
		}
	}()
	return contextIO, binding
}

func (b *proxyHandleContextIOBinding) stopAndWait() {
	if b == nil {
		return
	}
	close(b.stop)
	<-b.done
}

func acquireProxyHandleBuffers(reader io.Reader, writer io.Writer) *proxyHandleBuffers {
	buffers, _ := proxyHandleBufferPool.Get().(*proxyHandleBuffers)
	if buffers == nil {
		buffers = &proxyHandleBuffers{
			brw: bufio.NewReadWriter(
				bufio.NewReader(emptyProxyHandleReader{}),
				bufio.NewWriter(io.Discard),
			),
		}
	}
	buffers.brw.Reader.Reset(reader)
	buffers.brw.Writer.Reset(writer)
	return buffers
}

func releaseProxyHandleBuffers(buffers *proxyHandleBuffers) {
	if buffers == nil || buffers.brw == nil {
		return
	}
	buffers.contextIOBinding.stopAndWait()
	buffers.contextIOBinding = nil
	// Do not retain the connection, context, or any buffered packet while the
	// pair waits in the pool. Reset also discards unread/read-ahead bytes from
	// the connection that has just finished.
	buffers.brw.Reader.Reset(emptyProxyHandleReader{})
	buffers.brw.Writer.Reset(io.Discard)
	proxyHandleBufferPool.Put(buffers)
}

func releaseProxyHandleContext(ctx *Context) {
	if ctx == nil {
		return
	}
	ctx.Session().releaseProxyHandleBuffers()
}

func CreateProxyHandleContext(ctx context.Context, conn net.Conn) (*Context, error) {
	contextIO, binding := bindProxyHandleContextIO(ctx, conn)
	buffers := acquireProxyHandleBuffers(contextIO, contextIO)
	buffers.contextIOBinding = binding
	s, err := newSession(conn, buffers.brw)
	if err != nil {
		releaseProxyHandleBuffers(buffers)
		return nil, utils.Errorf("mitm: failed to create session: %v", err)
	}
	s.proxyHandleBuffers = buffers
	proxyContext, err := withSession(s)
	if err != nil {
		s.releaseProxyHandleBuffers()
		return nil, utils.Errorf("mitm: failed to build new context: %v", err)
	}
	return proxyContext, nil
}

func IsSocks5HandleShake(conn net.Conn) (fConn net.Conn, _ bool, _ byte, _ error) {
	peekable := utils.NewPeekableNetConn(conn)

	defer func() {
		if err := recover(); err != nil {
			log.Infof("peekable failed: %s", err)
			fConn = peekable
		}
	}()

	raw, err := peekable.Peek(2)
	if err != nil {
		if err == io.EOF {
			return peekable, false, 0, nil
		}
		return nil, false, 0, utils.Errorf("peek failed: %s", err)
	}
	if len(raw) != 2 {
		return nil, false, 0, utils.Errorf("check s5 failed: %v", raw)
	}
	return peekable, raw[0] == socks5Version && raw[1] > 0, raw[0], nil
}

func IsTlsHandleShake(conn net.Conn) (fConn net.Conn, _ bool, _ error) {
	peekable := utils.NewPeekableNetConn(conn)

	defer func() {
		if err := recover(); err != nil {
			log.Infof("peekable failed: %s", err)
			fConn = peekable
		}
	}()

	raw, err := peekable.Peek(2)
	if err != nil {
		if err == io.EOF {
			return peekable, false, nil
		}
		return nil, false, utils.Errorf("peek failed: %s", err)
	}
	if len(raw) != 2 {
		return nil, false, utils.Errorf("check s5 failed: %v", raw)
	}
	return peekable, raw[0] == 0x16, nil
}

func peekTLSVersion(conn net.Conn) (fConn net.Conn, version int, _ error) {
	peekable, ok := conn.(*utils.BufferedPeekableConn)
	if !ok {
		peekable = utils.NewPeekableNetConn(conn)
	}
	defer func() {
		if err := recover(); err != nil {
			log.Infof("peekable failed: %s", err)
			fConn = peekable
		}
	}()

	headerByte, err := peekable.Peek(5)
	if err != nil {
		return nil, 0, err
	}
	header := cryptobyte.String(headerByte)

	var clientHelloLength uint16
	if !header.Skip(3) || !header.ReadUint16(&clientHelloLength) {
		return nil, 0, utils.Errorf("failed to parse TLS header")
	}

	clientHelloByte, err := peekable.Peek(5 + int(clientHelloLength))
	if err != nil {
		return nil, 0, err
	}
	clientHelloByte = clientHelloByte[5:]

	if len(clientHelloByte) < int(clientHelloLength) || clientHelloByte[0] != 0x01 {
		return nil, 0, utils.Errorf("failed to parse client hello msg")
	}
	info, err := gmtls.UnmarshalClientHello(clientHelloByte)
	if err != nil {
		return nil, 0, err
	}
	var chosenVersion uint16
	if len(info.SupportedVersions) > 0 {
		chosenVersion = lo.Max(lo.Filter(info.SupportedVersions, func(i uint16, _ int) bool {
			if i > gmtls.VersionTLS13 { // filter reserved version
				return false
			}
			return true
		}))
	}

	return &peekedConn{
		Conn: conn,
		r:    io.MultiReader(bytes.NewReader(peekable.GetBuf()), peekable.GetReader()),
	}, int(chosenVersion), nil
}

func (p *Proxy) setHTTPCtxConnectTo(req *http.Request) (string, error) {
	connectedTo, err := utils.GetConnectedToHostPortFromHTTPRequest(req)
	if err != nil {
		return "", utils.Wrap(err, "mitm: invalid host")
	}

	return connectedTo, nil
}

// connectResponse fix previous 200 CONNECT response with content-length issue
func (p *Proxy) connectResponse(req *http.Request) *http.Response {
	// "Connection Established" is the standard status for connect request. ref-link https://github.com/google/martian/issues/306
	// Content-Length  should not be set, otherwise awvs will not work ref-link https://github.com/chaitin/xray/issues/627
	resp := proxyutil.NewResponse(200, nil, req)
	resp.Header.Del("Content-Type")
	resp.Close = false
	resp.Status = fmt.Sprintf("%d %s", 200, "Connection established")
	resp.Proto = "HTTP/1.0"
	resp.ProtoMajor = 1
	resp.ProtoMinor = 0
	resp.ContentLength = -1
	return resp
}

func (p *Proxy) handshakeWithTarget(req *http.Request) (net.Conn, error) {
	var rawConn net.Conn
	var err error
	var proxyUrl string
	gmConfig := &gmtls.Config{
		InsecureSkipVerify: true,
		GMSupport:          &gmtls.GMSupport{},
		ServerName:         utils.ExtractHost(req.URL.Host),
	}

	if p.proxyURL != nil {
		proxyUrl = p.proxyURL.String()
	}
	vanillaTLS := func() {
		rawConn, err = netx.DialTLSTimeout(time.Second*10, req.URL.Host, &gmtls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
			ServerName:         utils.ExtractHost(req.URL.Host),
		}, proxyUrl)
	}
	gmTLS := func() {
		rawConn, err = netx.DialTLSTimeout(time.Second*10, req.URL.Host, gmConfig, proxyUrl)
	}
	var taskGroup []func()

	// when not enable gmTLS
	if !p.gmTLS {
		taskGroup = append(taskGroup, vanillaTLS)
	} else {
		// when enable gmTLS add another func
		if !p.gmTLSOnly {
			taskGroup = append(taskGroup, vanillaTLS)
		}
		taskGroup = append(taskGroup, gmTLS)
	}

	// handle gmPrefer option
	// we get at least one option in taskGroup
	if p.gmTLS && p.gmPrefer && !p.gmTLSOnly {
		taskGroup[0], taskGroup[1] = taskGroup[1], taskGroup[0] // vanilla TLS always be the first
	}

	for _, task := range taskGroup {
		task()
		if len(taskGroup) > 1 && err != nil {
			continue
		} else {
			break
		}
	}
	return rawConn, err
}

func sessionBindConnectTo(s *Session, proxyProtocol string, host string, port int) {
	s.Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedTo, utils.HostPort(host, port))
	s.Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToHost, host)
	s.Set(httpctx.REQUEST_CONTEXT_KEY_ConnectedToPort, port)
	s.Set(httpctx.REQUEST_CONTEXT_KEY_RequestProxyProtocol, proxyProtocol)
	s.Set(AUTH_FINISH, true)
}

// A peekedConn subverts the net.Conn.Read implementation, primarily so that
// sniffed bytes can be transparently prepended.
type peekedConn struct {
	net.Conn
	r io.Reader
}

// Read allows control over the embedded net.Conn's read data. By using an
// io.MultiReader one can read from a conn, and then replace what they read, to
// be read again.
func (c *peekedConn) Read(buf []byte) (int, error) { return c.r.Read(buf) }
