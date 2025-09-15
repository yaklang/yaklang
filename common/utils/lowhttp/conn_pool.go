package lowhttp

import (
	"bufio"
	"bytes"
	"container/list"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"

	utls "github.com/refraction-networking/utls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

var (
	H2 = "h2"
	H1 = "http/1.1"
)

var (
	DefaultLowHttpConnPool = NewHttpConnPool(context.Background(), 100, 2)
	errServerClosedIdle    = errors.New("conn pool: server closed idle connection")
)

func NewHttpConnPool(ctx context.Context, idleCount int, perHostCount int) *LowHttpConnPool {
	return &LowHttpConnPool{
		maxIdleConn:        idleCount,
		maxIdleConnPerHost: perHostCount,
		connCount:          0,
		idleConnTimeout:    90 * time.Second,
		idleConnMap:        make(map[string][]*persistConn),
		keepAliveTimeout:   30 * time.Second,
		ctx:                ctx,
	}
}

type LowHttpConnPool struct {
	idleConnMux        sync.Mutex                // 空闲连接访问锁
	maxIdleConn        int                       // 最大总连接
	maxIdleConnPerHost int                       // 单host最大连接
	connCount          int                       // 已有连接计数器
	idleConnMap        map[string][]*persistConn // 空闲连接
	idleConnTimeout    time.Duration             // 连接过期时间
	idleLRU            connLRU                   // 连接池 LRU
	keepAliveTimeout   time.Duration
	ctx                context.Context
}

func (l *LowHttpConnPool) HostConnFull(key *connectKey) bool {
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	return len(l.idleConnMap[key.hash()]) >= l.maxIdleConnPerHost
}

func (l *LowHttpConnPool) clear() {
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	if l == nil {
		return
	}
	for _, pcs := range l.idleConnMap {
		for _, pc := range pcs {
			err := l.removeConnLocked(pc, true)
			if err != nil {
				log.Warnf("lowhttp conn pool clear failed when calling removeConnLocked: %v", err)
			}
		}
	}
}

func (l *LowHttpConnPool) contextDone() bool {
	select {
	case <-l.ctx.Done():
		return true
	default:
		return false
	}
}

var (
	getIdleConnRequiredCounter = new(int64)
	getIdleConnFinishedCounter = new(int64)
)

func addGetIdleConnRequiredCounter() {
	result := atomic.AddInt64(getIdleConnRequiredCounter, 1)
	_ = result
	log.Infof("get idle conn start counter: %d", result)
}

func addGetIdleConnFinishedCounter() {
	result := atomic.AddInt64(getIdleConnFinishedCounter, 1)
	_ = result
	log.Infof("get idle conn finished counter: %d", result)
}

// 取出一个空闲连接
// want 检索一个可用的连接，并且把这个连接从连接池中取出来
func (l *LowHttpConnPool) getIdleConn(key *connectKey, opts ...netx.DialXOption) (*persistConn, error) {
	//addGetIdleConnRequiredCounter()
	if l.contextDone() {
		return nil, utils.Error("lowhttp: context done")
	}

	// 尝试获取复用连接
	if oldPc, ok := l.getFromConn(key); ok {
		//addGetIdleConnFinishedCounter()
		return oldPc, nil
	}
	// 没有复用连接则新建一个连接
	pConn, err := newPersistConn(key, l, opts...)
	if err != nil {
		// try get idle conn
		if oldPc, ok := l.getFromConn(key); ok {
			//addGetIdleConnFinishedCounter()
			return oldPc, nil
		}
		return nil, err
	}
	//addGetIdleConnFinishedCounter()
	return pConn, nil
}

func (l *LowHttpConnPool) getFromConn(key *connectKey) (oldPc *persistConn, getConn bool) {
	if l.contextDone() {
		return nil, false
	}
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()

	var oldTime time.Time
	if l.idleConnTimeout > 0 {
		oldTime = time.Now().Add(-l.idleConnTimeout)
	}

	// 从连接池中取出一个连接
	connList, ok := l.idleConnMap[key.hash()]
	if !ok { // if not get return
		return
	}

	for len(connList) > 0 {
		oldPc = connList[len(connList)-1]

		if key.scheme == H2 {
			// 检查获取的连接是否可用
			canUse := !(oldPc.alt.readGoAway || oldPc.alt.closed || oldPc.alt.full)
			if canUse {
				// h2 conn not need leave
				getConn = true
				break
			}
		} else {
			l.idleLRU.remove(oldPc)
			tooOld := !oldTime.Before(oldPc.idleAt)
			if !tooOld {
				oldPc.closeTimer.Stop()
				connList = connList[:len(connList)-1] // h1 conn need leave conn
				getConn = true
				break
			}
			oldPc.closeNetConn() // close too old conn
		}
		connList = connList[:len(connList)-1]
	}

	// clear empty list
	if len(connList) > 0 {
		l.idleConnMap[key.hash()] = connList
	} else {
		delete(l.idleConnMap, key.hash())
	}

	return
}

func (l *LowHttpConnPool) putIdleConn(pc *persistConn) error {
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()

	cacheKeyHash := pc.cacheKey.hash()
	// 如果超过池规定的单个host可以拥有的最大连接数量则直接放弃添加连接
	if len(l.idleConnMap[cacheKeyHash]) >= l.maxIdleConnPerHost || l.contextDone() {
		if !pc.IsH2Conn() {
			pc.closeNetConn() // if too many, close it
		}
		return nil
	}

	// 添加一个连接到连接池,转化连接状态,刷新空闲时间
	pc.idleAt = time.Now()
	if l.idleConnTimeout > 0 && !pc.IsH2Conn() { // 判断空闲时间,若为0则不设限
		if pc.closeTimer != nil {
			pc.closeTimer.Reset(l.idleConnTimeout)
		} else {
			pc.closeTimer = time.AfterFunc(l.idleConnTimeout, func() {
				pc.closeConn(utils.Error("lowhttp: idle timeout"))
			})
		}
	}

	if l.connCount >= l.maxIdleConn { // if conn pool is full, remove oldest
		oldPconn := l.idleLRU.removeOldest()
		err := l.removeConnLocked(oldPconn, !oldPconn.IsH2Conn())
		if err != nil {
			return err
		}
	}
	l.idleConnMap[cacheKeyHash] = append(l.idleConnMap[cacheKeyHash], pc)
	if !pc.IsH2Conn() {
		pc.markReused()
	}
	return nil
}

// 在有写锁的环境中从池子里删除一个空闲连接
func (l *LowHttpConnPool) removeConnLocked(pc *persistConn, needClose bool) error {
	if pc.closeTimer != nil {
		pc.closeTimer.Stop()
	}
	if needClose {
		pc.closeNetConn()
	}
	key := pc.cacheKey.hash()
	connList := l.idleConnMap[pc.cacheKey.hash()]
	switch len(connList) {
	case 0:
		return nil
	case 1:
		if connList[0] == pc {
			delete(l.idleConnMap, key)
		}
	default:
		for i, v := range connList {
			if v != pc {
				continue
			}
			copy(connList[i:], connList[i+1:])
			l.idleConnMap[key] = connList[:len(connList)-1]
			break
		}
	}
	return nil
}

// 长连接
type persistConn struct {
	ctx      context.Context
	cancel   context.CancelFunc
	alt      *http2ClientConn
	conn     net.Conn // conn本体
	mu       sync.Mutex
	p        *LowHttpConnPool // 连接对应的连接池
	cacheKey *connectKey      // 连接池缓存key
	isProxy  bool             // 是否使用代理
	alive    bool             // 存活判断
	sawEOF   bool             // 连接是否EOF

	idleAt          time.Time                 // 进入空闲的时间
	closeTimer      *time.Timer               // 关闭定时器
	dialOption      []netx.DialXOption        // dial 选项
	br              *bufio.Reader             // from conn
	bw              *bufio.Writer             // to conn
	h2br            *bufio.Reader             // dedicated bufio.Reader for HTTP/2
	reqCh           chan requestAndResponseCh // 读取管道
	writeCh         chan writeRequest         // 写入管道
	writeErrCh      chan error                // 写入错误信号
	serverStartTime time.Time                 // 响应时间
	//numExpectedResponses int                       // 预期的响应数量
	reused bool  // 是否复用
	closed error // 连接关闭原因

	inPool bool
	isIdle bool

	// debug info
	wPacket []packetInfo
	rPacket []packetInfo
}

type requestAndResponseCh struct {
	reqPacket   []byte
	ch          chan responseInfo
	reqInstance *http.Request
	option      *LowhttpExecConfig
	writeErrCh  chan error
	// respCh
}

type responseInfo struct {
	resp      *http.Response
	respBytes []byte
	err       error
	info      httpInfo
}

type httpInfo struct {
	ServerTime time.Duration
}

type writeRequest struct {
	reqPacket   []byte
	ch          chan error
	reqInstance *http.Request
	options     *LowhttpExecConfig
}

type packetInfo struct {
	localPort string
	packet    []byte
}

type persistConnWriter struct {
	pc *persistConn
}

func (w persistConnWriter) Write(p []byte) (n int, err error) {
	n, err = w.pc.conn.Write(p)
	return
}

func (w persistConnWriter) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = io.Copy(w.pc.conn, r)
	return
}

type bodyEOFSignal struct {
	body         io.ReadCloser
	mu           sync.Mutex        // guards following 4 fields
	closed       bool              // whether Close has been called
	rerr         error             // sticky Read error
	fn           func(error) error // err will be nil on Read io.EOF
	earlyCloseFn func() error      // optional alt Close func used if io.EOF not seen
}

var errReadOnClosedResBody = errors.New("http: read on closed response body")

func (es *bodyEOFSignal) Read(p []byte) (n int, err error) {
	es.mu.Lock()
	closed, rerr := es.closed, es.rerr
	es.mu.Unlock()
	if closed {
		return 0, errReadOnClosedResBody
	}
	if rerr != nil {
		return 0, rerr
	}

	n, err = es.body.Read(p)
	if err != nil {
		es.mu.Lock()
		defer es.mu.Unlock()
		if es.rerr == nil {
			es.rerr = err
		}
		err = es.condfn(err)
	}
	return
}

func (es *bodyEOFSignal) Close() error {
	es.mu.Lock()
	defer es.mu.Unlock()
	if es.closed {
		return nil
	}
	es.closed = true
	if es.earlyCloseFn != nil && es.rerr != io.EOF {
		return es.earlyCloseFn()
	}
	err := es.body.Close()
	return es.condfn(err)
}

// caller must hold es.mu.
func (es *bodyEOFSignal) condfn(err error) error {
	if es.fn == nil {
		return err
	}
	err = es.fn(err)
	es.fn = nil
	return err
}

func newPersistConn(key *connectKey, pool *LowHttpConnPool, opt ...netx.DialXOption) (*persistConn, error) {
	needProxy := len(key.proxy) > 0
	opt = append(opt, netx.DialX_WithKeepAlive(pool.keepAliveTimeout))
	newConn, err := netx.DialX(key.addr, opt...)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	pc := &persistConn{
		ctx:             ctx,
		cancel:          cancel,
		conn:            newConn,
		mu:              sync.Mutex{},
		p:               pool,
		cacheKey:        key,
		isProxy:         needProxy,
		sawEOF:          false,
		idleAt:          time.Time{},
		closeTimer:      nil,
		dialOption:      opt,
		reqCh:           make(chan requestAndResponseCh, 2),
		writeCh:         make(chan writeRequest, 2),
		writeErrCh:      make(chan error, 2),
		serverStartTime: time.Time{},
		wPacket:         make([]packetInfo, 0),
		rPacket:         make([]packetInfo, 0),
		//numExpectedResponses: 0,
	}

	if key.scheme == H2 {
		if key.https { // Negotiation fail downgrade
			switch conn := newConn.(type) {
			case *tls.Conn:
				key.scheme = conn.ConnectionState().NegotiatedProtocol
			case *utls.UConn:
				key.scheme = conn.ConnectionState().NegotiatedProtocol
			case *gmtls.Conn:
				key.scheme = conn.ConnectionState().NegotiatedProtocol
			}
			if key.scheme != H2 { // downgrade conn should not reuse
				key.scheme = H1
				return pc, nil
			}
		}

		pc.h2Conn()
		go pc.alt.readLoop()
		if err = pc.alt.preface(); err == nil {
			err = pool.putIdleConn(pc)
			if err != nil {
				log.Errorf("h2 conn put idle conn failed: %v", err) // not care h2 conn put idle conn failed
			}
			return pc, nil
		} else { // preface fail downgrade
			key.scheme = H1
			newH1Conn, err := netx.DialX(key.addr, append(opt, netx.DialX_WithTLSNextProto(H1))...)
			if err != nil {
				return nil, err
			}
			pc.alt = nil
			pc.conn = newH1Conn
			return pc, nil
		}
	}

	pc.br = bufio.NewReader(pc)
	pc.bw = bufio.NewWriter(persistConnWriter{pc})

	// 启动读取写入循环
	go pc.writeLoop()
	go pc.readLoop()
	return pc, nil
}

func (pc *persistConn) h2Conn() {
	// 复用或创建专用的HTTP/2 bufio.Reader
	if pc.h2br == nil {
		pc.h2br = bufio.NewReaderSize(pc.conn, 32*1024) // 使用较大的缓冲区
	} else {
		// 重置Reader以复用缓冲区
		pc.h2br.Reset(pc.conn)
	}

	newH2Conn := &http2ClientConn{
		conn:              pc.conn,
		ctx:               pc.p.ctx,
		mu:                new(sync.Mutex),
		streams:           make(map[uint32]*http2ClientStream),
		currentStreamID:   1,
		idleTimeout:       pc.p.idleConnTimeout,
		maxFrameSize:      defaultMaxFrameSize,
		initialWindowSize: defaultStreamReceiveWindowSize,
		headerListMaxSize: defaultHeaderTableSize,
		connWindowControl: newControl(defaultStreamReceiveWindowSize),
		maxStreamsCount:   defaultMaxConcurrentStreamSize,
		fr:                http2.NewFramer(pc.conn, pc.h2br), // 使用复用的Reader
		frWriteMutex:      new(sync.Mutex),
		hDec:              hpack.NewDecoder(defaultHeaderTableSize, nil),
		closeCond:         sync.NewCond(new(sync.Mutex)),
		clientPrefaceOk:   utils.NewAtomicBool(),
		resourceCleaned:   utils.NewAtomicBool(), // 初始化资源清理标记
		http2StreamPool: &sync.Pool{
			New: func() interface{} {
				return new(http2ClientStream)
			},
		},
	}

	newH2Conn.idleTimer = time.AfterFunc(newH2Conn.idleTimeout, func() {
		newH2Conn.setClose()
	})
	pc.alt = newH2Conn
}

func (pc *persistConn) readLoop() {
	var closeErr error
	defer func() {
		pc.closeConn(closeErr)
	}()

	tryPutIdleConn := func() bool {
		err := pc.p.putIdleConn(pc)
		if err != nil {
			return false
		}
		return true
	}

	var rc requestAndResponseCh

	count := 0
	alive := true
	firstAuth := true
	for alive {
		// if failed, handle it (re-conn / or abandoned)
		_ = pc.conn.SetReadDeadline(time.Time{})
		_, err := pc.br.Peek(1)

		// 检查是否有需要返回的响应,如果没有则可以直接返回,不需要往管道里返回数据（err）
		if err != nil {
			if errors.Is(err, io.EOF) {
				pc.sawEOF = true
				closeErr = errServerClosedIdle
			} else {
				closeErr = connPoolReadFromServerError{err}
			} // read error just return and close
			return
		}
		info := httpInfo{ServerTime: time.Since(pc.serverStartTime)}

		if firstAuth {
			select {
			case rc = <-pc.reqCh:
			case <-time.After(time.Second * 5):
				closeErr = utils.Error("lowhttp: readLoop reqCh timeout, met abnormal event: got response before request arrived")
				log.Error(closeErr)
				return
			}
		}

		var resp *http.Response

		stashRequest := rc.reqInstance
		if stashRequest == nil {
			stashRequest = new(http.Request)
		}
		// peek is executed, so we can read without timeout
		// for long time chunked supported

		var respBuffer bytes.Buffer
		httpResponseReader := io.TeeReader(pc.br, &respBuffer)
		_ = pc.conn.SetReadDeadline(time.Time{})
		resp, err = utils.ReadHTTPResponseFromBufioReaderConn(httpResponseReader, pc.conn, stashRequest)
		if resp != nil {
			resp.Request = nil
		}

		if firstAuth && resp != nil && resp.StatusCode == http.StatusUnauthorized {
			if authHeader := IGetHeader(resp, "WWW-Authenticate"); len(authHeader) > 0 {
				if auth := GetHttpAuth(authHeader[0], rc.option); auth != nil {
					authReq, err := auth.Authenticate(pc.conn, rc.option)
					if err == nil {
						pc.writeCh <- writeRequest{
							reqPacket:   authReq,
							ch:          rc.writeErrCh,
							reqInstance: rc.reqInstance,
							options:     rc.option,
						}
					}
					firstAuth = false
					continue
				}
			}
		}

		count++
		var responseRaw bytes.Buffer
		var respClose bool
		var respPacket = respBuffer.Bytes()
		if resp != nil {
			respClose = resp.Close
		}
		if len(respPacket) > 0 {
			responseRaw.Write(respPacket)
		}

		if err != nil || respClose {
			if responseRaw.Len() >= len(respPacket) { // 如果 TeaReader内部还有数据证明,证明有响应数据,只是解析失败
				// continue read 5 seconds, to receive rest data
				// ignore error, treat as bad conn
				timeout := 5 * time.Second
				if respClose {
					timeout = 1 * time.Second // 如果 http close 了 则只等待1秒
				}
				restBytes, _ := utils.ReadUntilStable(pc.br, pc.conn, timeout, 300*time.Millisecond)
				pc.sawEOF = true // 废弃连接
				if len(restBytes) > 0 {
					responseRaw.Write(restBytes)
					respPacket = responseRaw.Bytes()
					if len(respPacket) > 0 {
						httpctx.SetBareResponseBytesForce(stashRequest, respPacket) // 强制修改原始响应包
						err = nil
					}
				}
			}
		}

		//pc.mu.Lock()
		////pc.numExpectedResponses-- // 减少预期响应数量
		//pc.mu.Unlock()

		rc.ch <- responseInfo{resp: resp, respBytes: respPacket, info: info, err: err}
		firstAuth = true
		alive = alive &&
			!pc.sawEOF &&
			tryPutIdleConn()
	}
}

func (pc *persistConn) writeLoop() {
	var closeErr error
	defer func() {
		pc.closeConn(closeErr)
	}()
	for {
		select {
		case wr := <-pc.writeCh:
			currentRPS.Add(1)
			var err error
			if wr.options != nil && wr.options.EnableRandomChunked {
				// 随机分块写入
				sender, err := wr.options.GetOrCreateChunkSender()
				if err != nil {
					closeErr = err
					return
				}
				err = sender.Send(wr.reqPacket, pc.bw)
			} else {
				// 正常写入
				_, err = pc.bw.Write(wr.reqPacket)
			}

			if err == nil {
				err = pc.bw.Flush()
				pc.serverStartTime = time.Now()
			}
			if err != nil {
				closeErr = err
				return
			}
		case <-pc.ctx.Done():
			return
		}
	}
}

func (pc *persistConn) closeConn(err error) { // when write loop break or read loop break and ideal timeout call
	pc.mu.Lock()
	defer pc.mu.Unlock()
	select {
	case <-pc.ctx.Done(): // already closed
	default:
		pc.closed = err
		pc.cancel()
		pc.removeConn()
	}
}

func (pc *persistConn) removeConn() {
	l := pc.p
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	err := l.removeConnLocked(pc, true)
	if err != nil {
		log.Error(err)
	}
}

func (pc *persistConn) closeNetConn() error {
	if pc.IsH2Conn() && pc.alt != nil {
		pc.alt.setClose()
		// 清理HTTP/2专用的bufio.Reader
		if pc.h2br != nil {
			// 重置Reader以释放内部缓冲区引用
			pc.h2br.Reset(nil)
			pc.h2br = nil
		}
		return nil
	}
	return pc.conn.Close()
}

func (pc *persistConn) IsH2Conn() bool {
	return pc.cacheKey.scheme == H2
}

// implement net.Conn
func (pc *persistConn) Close() error {
	return pc.p.putIdleConn(pc)
}

func (pc *persistConn) Read(b []byte) (n int, err error) {
	n, err = pc.conn.Read(b)
	if err == io.EOF {
		pc.sawEOF = true
	}
	return
}

func (pc *persistConn) Write(b []byte) (n int, err error) {
	return pc.conn.Write(b)
}

func (pc *persistConn) LocalAddr() net.Addr {
	return pc.conn.LocalAddr()
}

func (pc *persistConn) RemoteAddr() net.Addr {
	return pc.conn.RemoteAddr()
}

func (pc *persistConn) SetDeadline(t time.Time) error {
	return pc.conn.SetDeadline(t)
}

func (pc *persistConn) SetReadDeadline(t time.Time) error {
	return pc.conn.SetReadDeadline(t)
}

func (pc *persistConn) SetWriteDeadline(t time.Time) error {
	return pc.conn.SetWriteDeadline(t)
}

// markReused 标识此连接已经被复用
func (pc *persistConn) markReused() {
	pc.mu.Lock()
	pc.reused = true
	pc.mu.Unlock()
}

func (pc *persistConn) isReused() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.alt != nil {
		return pc.alt.currentStreamID > 1
	}
	return pc.reused
}

func (pc *persistConn) shouldRetryRequest(err error) bool {
	if !pc.isReused() {
		// 初次连接失败，则不重试
		return false
	}
	var connPoolReadFromServerError connPoolReadFromServerError
	if errors.As(err, &connPoolReadFromServerError) {
		// 除了EOF以外的服务器错误，重试
		return true
	}
	// todo 幂等性请求
	if errors.Is(err, errServerClosedIdle) {
		// peek 到 EOF 大可能是连接池中的连接已经被服务器关闭，所以尝试重试
		return true
	}
	if errors.Is(err, errH2ConnClosed) {
		return true
	}
	return false // 保守不重试
}

type connPoolReadFromServerError struct {
	err error
}

func (e connPoolReadFromServerError) Unwrap() error { return e.err }

func (e connPoolReadFromServerError) Error() string {
	return fmt.Sprintf("lowhttp: conn pool failed to read from server: %v", e.err)
}

type connectKey struct {
	proxy           []string // 可以使用的代理
	scheme, addr    string   // 协议和目标地址
	https           bool
	gmTls           bool
	clientHelloSpec *utls.ClientHelloSpec
	sni             string
}

func (c *connectKey) hash() string {
	return utils.CalcSha1(c.proxy, c.scheme, c.addr, c.https, c.gmTls, c.clientHelloSpec, c.sni)
}

type connLRU struct {
	ll *list.List // list.Element.Value type is of *persistConn
	m  map[*persistConn]*list.Element
}

// 添加一个新的连接到LRU的双向链表中
func (cl *connLRU) add(pc *persistConn) {
	if cl.ll == nil {
		cl.ll = list.New()
		cl.m = make(map[*persistConn]*list.Element)
	}
	ele := cl.ll.PushFront(pc)
	if _, ok := cl.m[pc]; ok {
		panic("persistConn was already in LRU")
	}
	cl.m[pc] = ele
}

// 使用一个连接后移动LRU
func (cl *connLRU) use(pc *persistConn) {
	if cl.ll == nil {
		cl.ll = list.New()
		cl.m = make(map[*persistConn]*list.Element)
	}
	ele, ok := cl.m[pc]
	if !ok {
		panic("persistConn is not already in LRU")
	}
	cl.ll.MoveToFront(ele)
}

// 从LRU中取出应该删除的连接
func (cl *connLRU) removeOldest() *persistConn {
	ele := cl.ll.Back()
	pc := ele.Value.(*persistConn)
	cl.ll.Remove(ele)
	delete(cl.m, pc)
	return pc
}

// 删除一个LRU链表中的元素
func (cl *connLRU) remove(pc *persistConn) {
	if ele, ok := cl.m[pc]; ok {
		cl.ll.Remove(ele)
		delete(cl.m, pc)
	}
}

// 获取缓存的长度.
func (cl *connLRU) len() int {
	return len(cl.m)
}
