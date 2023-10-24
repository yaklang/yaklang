package lowhttp

import (
	"bufio"
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	DefaultLowHttpConnPool = &lowHttpConnPool{
		maxIdleConn:        100,
		maxIdleConnPerHost: 2,
		connCount:          0,
		idleConnTimeout:    90 * time.Second,
		idleConn:           make(map[uint64][]*persistConn),
		keepAliveTimeout:   30 * time.Second,
	}
	errServerClosedIdle = errors.New("conn pool: server closed idle connection")
)

type lowHttpConnPool struct {
	idleConnMux        sync.RWMutex              //空闲连接访问锁
	maxIdleConn        int                       //最大总连接
	maxIdleConnPerHost int                       //单host最大连接
	connCount          int                       //已有连接计数器
	idleConn           map[uint64][]*persistConn //空闲连接
	idleConnTimeout    time.Duration             //连接过期时间
	idleLRU            connLRU                   //连接池 LRU
	keepAliveTimeout   time.Duration
}

// 取出一个空闲连接
// want 检索一个可用的连接，并且把这个连接从连接池中取出来
func (l *lowHttpConnPool) getIdleConn(key connectKey, opts ...netx.DialXOption) (*persistConn, error) {
	//尝试获取复用连接
	if oldPc, ok := l.getFromConn(key); ok {
		//log.Infof("use old conn")
		return oldPc, nil
	}
	//没有复用连接则新建一个连接
	pConn, err := newPersistConn(key, l, opts...)
	if err != nil {
		return nil, err
	}
	return pConn, nil
}

func (l *lowHttpConnPool) getFromConn(key connectKey) (oldPc *persistConn, getConn bool) {
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	getConn = false
	var oldTime time.Time
	if l.idleConnTimeout > 0 {
		oldTime = time.Now().Add(-l.idleConnTimeout)
	}

	if connList, ok := l.idleConn[key.hash()]; ok {
		stop := false
		for len(connList) > 0 && !stop {
			oldPc = connList[len(connList)-1]

			//检查获取的连接是否空闲超时，若超时再取下一个
			tooOld := !oldTime.Before(oldPc.idleAt)
			if tooOld {
				oldPc.Conn.Close()
				connList = connList[:len(connList)-1]
				continue
			}

			l.idleLRU.remove(oldPc)
			connList = connList[:len(connList)-1]
			getConn = true
			stop = true
		}
		if len(connList) > 0 {
			l.idleConn[key.hash()] = connList
		} else {
			delete(l.idleConn, key.hash())
		}
	}
	return
}

func (l *lowHttpConnPool) putIdleConn(conn *persistConn) error {
	cacheKeyHash := conn.cacheKey.hash()
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	//如果超过池规定的单个host可以拥有的最大连接数量则直接放弃添加连接
	if len(l.idleConn[cacheKeyHash]) >= l.maxIdleConnPerHost {
		return nil
	}

	//添加一个连接到连接池,转化连接状态,刷新空闲时间
	conn.idleAt = time.Now()
	if l.idleConnTimeout > 0 { //判断空闲时间,若为0则不设限
		if conn.closeTimer != nil {
			conn.closeTimer.Reset(l.idleConnTimeout)
		} else {
			conn.closeTimer = time.AfterFunc(l.idleConnTimeout, conn.removeConn)
		}
	}

	if l.connCount >= l.maxIdleConn {
		oldPconn := l.idleLRU.removeOldest()
		err := l.removeConnLocked(oldPconn)
		if err != nil {
			return err
		}
	}
	l.idleConn[cacheKeyHash] = append(l.idleConn[cacheKeyHash], conn)
	conn.markReused()
	return nil
}

// 在有写锁的环境中从池子里删除一个空闲连接
func (l *lowHttpConnPool) removeConnLocked(pConn *persistConn) error {
	if pConn.closeTimer != nil {
		pConn.closeTimer.Stop()
	}
	key := pConn.cacheKey.hash()
	connList := l.idleConn[pConn.cacheKey.hash()]
	pConn.Conn.Close()
	switch len(connList) {
	case 0:
		log.Warn("remove Conn warning : [not find this Conn from the Conn pool]")
		return nil
	case 1:
		if connList[0] == pConn {
			delete(l.idleConn, key)
		}
	default:
		for i, v := range connList {
			if v != pConn {
				continue
			}
			copy(connList[i:], connList[i+1:])
			l.idleConn[key] = connList[:len(connList)-1]
			break
		}
	}
	return nil
}

// 长连接
type persistConn struct {
	net.Conn //conn本体
	mu       sync.Mutex
	p        *lowHttpConnPool //连接对应的连接池
	cacheKey connectKey       //连接池缓存key
	isProxy  bool             //是否使用代理
	alive    bool             //存活判断
	sawEOF   bool             //连接是否EOF

	idleAt               time.Time                 //进入空闲的时间
	closeTimer           *time.Timer               //关闭定时器
	dialOption           []netx.DialXOption        //dial 选项
	br                   *bufio.Reader             // from conn
	bw                   *bufio.Writer             // to conn
	reqCh                chan requestAndResponseCh //读取管道
	writeCh              chan writeRequest         //写入管道
	closeCh              chan struct{}             //关闭信号
	writeErrCh           chan error                //写入错误信号
	serverStartTime      time.Time                 //响应时间
	numExpectedResponses int                       //预期的响应数量
	reused               bool                      //是否复用
	closed               error                     //连接关闭原因

	inPool bool
	isIdle bool

	//debug info
	wPacket []packetInfo
	rPacket []packetInfo
}

type requestAndResponseCh struct {
	reqPacket []byte
	ch        chan responseInfo
	//respCh
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
	reqPacket []byte
	ch        chan error
}

type packetInfo struct {
	localPort string
	packet    []byte
}

type persistConnWriter struct {
	pc *persistConn
}

func (w persistConnWriter) Write(p []byte) (n int, err error) {
	n, err = w.pc.Conn.Write(p)
	return
}

func (w persistConnWriter) ReadFrom(r io.Reader) (n int64, err error) {
	n, err = io.Copy(w.pc.Conn, r)
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

func newPersistConn(key connectKey, pool *lowHttpConnPool, opt ...netx.DialXOption) (*persistConn, error) {
	needProxy := len(key.proxy) > 0
	opt = append(opt, netx.DialX_WithKeepAlive(pool.keepAliveTimeout))
	newConn, err := netx.DialX(key.addr, opt...)
	if err != nil {
		return nil, err
	}
	// 初始化连接
	pc := &persistConn{
		Conn:                 newConn,
		mu:                   sync.Mutex{},
		p:                    pool,
		cacheKey:             key,
		isProxy:              needProxy,
		sawEOF:               false,
		idleAt:               time.Time{},
		closeTimer:           nil,
		dialOption:           opt,
		reqCh:                make(chan requestAndResponseCh, 1),
		writeCh:              make(chan writeRequest, 1),
		closeCh:              make(chan struct{}, 1),
		writeErrCh:           make(chan error, 1),
		serverStartTime:      time.Time{},
		wPacket:              make([]packetInfo, 0),
		rPacket:              make([]packetInfo, 0),
		numExpectedResponses: 0,
	}
	pc.br = bufio.NewReader(pc)
	pc.bw = bufio.NewWriter(persistConnWriter{pc})

	//启动读取写入循环
	go pc.writeLoop()
	go pc.readLoop()
	return pc, nil
}

func (pc *persistConn) readLoop() {
	defer func() {
		if pc.reused {
			pc.removeConn()
		}
	}()

	tryPutIdleConn := func() bool {
		err := pc.p.putIdleConn(pc)
		if err != nil {
			return false
		}
		return true
	}

	eofc := make(chan struct{})
	defer close(eofc) // unblock reader on errors

	count := 0
	alive := true
	for alive {
		// 尝试peek error
		_, err := pc.br.Peek(1)

		// 检查是否有需要返回的响应,如果没有则可以直接返回,不需要往管道里返回数据（err）
		if pc.numExpectedResponses == 0 {
			if err == io.EOF {
				pc.closeConn(errServerClosedIdle)
			} else {
				pc.closeConn(err)
			}
			return
		}
		info := httpInfo{ServerTime: time.Since(pc.serverStartTime)}

		rc := <-pc.reqCh

		if err != nil { //需要向主进程返回一个带标识的错误,主进程用于判断是否重试
			if errors.Is(err, io.EOF) {
				pc.sawEOF = true
			}
			rc.ch <- responseInfo{err: connPoolReadFromServerError{err: err}}
			return
		}

		var responseRaw bytes.Buffer
		var respPacket []byte
		var resp *http.Response

		httpResponseReader := bufio.NewReader(io.TeeReader(pc.br, &responseRaw))
		resp, err = utils.ReadHTTPResponseFromBufioReader(httpResponseReader, nil)
		count++

		if err != nil {
			if len(responseRaw.Bytes()) > 0 { // 如果 TeaReader内部还有数据证明,证明有响应数据,只是解析失败
				_, err = io.ReadAll(httpResponseReader) // 尝试读取所有数据,主要是超过缓冲区的问题
				if errors.Is(err, io.EOF) {
					pc.sawEOF = true
				}
				respPacket = responseRaw.Bytes()
			}
		} else {
			respPacket, err = utils.DumpHTTPResponse(resp, true)
		}

		pc.mu.Lock()
		pc.numExpectedResponses-- //减少预期响应数量
		pc.mu.Unlock()

		rc.ch <- responseInfo{resp: resp, respBytes: respPacket, info: info}
		alive = alive &&
			!pc.sawEOF &&
			tryPutIdleConn()

	}
}

func (pc *persistConn) writeLoop() {
	count := 0
	for {
		select {
		case wr := <-pc.writeCh:
			count++
			_, err := pc.bw.Write(wr.reqPacket)
			if err == nil {
				err = pc.bw.Flush()
				pc.serverStartTime = time.Now()
			}
			wr.ch <- err //to exec.go
			if err != nil {
				pc.writeErrCh <- err
				//log.Infof("!!!conn [%v] connect [%v] has be writed [%d]", pc.Conn.LocalAddr(), pc.cacheKey.addr, count)
				return
			}
			pc.mu.Lock()
			pc.numExpectedResponses++
			pc.mu.Unlock()
		case <-pc.closeCh:
			//log.Infof("!!!conn [%v] connect [%v] has be writed [%d]", pc.Conn.LocalAddr(), pc.cacheKey.addr, count)
			return
		}
	}
}

func (pc *persistConn) closeConn(err error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.closed != nil {
		return
	}
	if err == nil {
		err = errors.New("lowhttp: conn pool unknown error")
	}
	pc.Conn.Close()
	pc.closed = err
	close(pc.closeCh)
}

func (pc *persistConn) Close() error {
	return pc.p.putIdleConn(pc)
}

func (pc *persistConn) removeConn() {
	l := pc.p
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	err := l.removeConnLocked(pc)
	if err != nil {
		log.Error(err)
	}
}

func (pc *persistConn) Read(b []byte) (n int, err error) {
	n, err = pc.Conn.Read(b)
	if err == io.EOF {
		pc.sawEOF = true
	}
	return
}

// markReused 标识此连接已经被复用
func (pc *persistConn) markReused() {
	pc.mu.Lock()
	pc.reused = true
	pc.mu.Unlock()
}

func (pc *persistConn) shouldRetryRequest(err error) bool {
	//todo H2处理
	if !pc.reused {
		//初次连接失败，则不重试
		return false
	}
	var connPoolReadFromServerError connPoolReadFromServerError
	if errors.As(err, &connPoolReadFromServerError) {
		//除了EOF以外的服务器错误，重试
		return true
	}
	//todo 幂等性请求
	if errors.Is(err, errServerClosedIdle) {
		// peek 到 EOF 大可能是连接池中的连接已经被服务器关闭，所以尝试重试
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
	proxy        []string //可以使用的代理
	scheme, addr string   //协议和目标地址
	https        bool
	gmTls        bool
}

func (c connectKey) hash() uint64 {
	data := []byte(fmt.Sprintf("%#v|%s|%s|%v|%v", c.proxy, c.scheme, c.addr, c.https, c.gmTls))
	return utils.SimHash(data)
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
