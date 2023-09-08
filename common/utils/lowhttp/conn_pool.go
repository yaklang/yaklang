package lowhttp

import (
	"container/list"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"sync"
	"time"
)

var DefaultLowHttpConnPool = &lowHttpConnPool{
	maxIdleConn:        100,
	maxIdleConnPerHost: 2,
	connCount:          0,
	idleConnTimeout:    90 * time.Second,
	gcPool: sync.Pool{
		New: func() interface{} {
			return new(persistConn)
		},
	},
	idleConn:         make(map[uint64][]*persistConn),
	keepAliveTimeout: 30 * time.Second,
}

type lowHttpConnPool struct {
	idleConnMux        sync.RWMutex              //空闲连接访问锁
	maxIdleConn        int                       //最大总连接
	maxIdleConnPerHost int                       //单host最大连接
	connCount          int                       //已有连接计数器
	idleConn           map[uint64][]*persistConn //空闲连接
	idleConnTimeout    time.Duration             //连接过期时间
	gcPool             sync.Pool                 //回收池，用于回收conn结构体避免频繁创建销毁结构体
	idleLRU            connLRU                   //连接池 LRU
	keepAliveTimeout   time.Duration
}

// 取出一个空闲连接
// want 检索一个可用的连接，并且把这个连接从连接池中取出来
func (l *lowHttpConnPool) getIdleConn(key connectKey, opts ...netx.DialXOption) (*persistConn, error) {
	l.idleConnMux.RLock()
	// 检索是否有符合要求的连接
	if len(l.idleConn[key.hash()]) > 0 {
		for _, pConn := range l.idleConn[key.hash()] {
			if pConn.isAlive && pConn.isIdle {
				l.idleConnMux.RUnlock()
				l.idleConnMux.Lock()
				pConn.isIdle = false
				l.idleConnMux.Unlock()
				return pConn, nil
			}
		}
	}
	l.idleConnMux.RUnlock()
	pConn, err := newPersistConn(key, l, opts...)
	if err != nil {
		return nil, err
	}
	return pConn, nil
}

func (l *lowHttpConnPool) putIdleConn(conn *persistConn) error {
	cacheKeyHash := conn.cacheKey.hash()
	l.idleConnMux.RLock()
	//如果超过池规定的单个host可以拥有的最大连接数量则直接放弃添加连接
	if len(l.idleConn[cacheKeyHash]) >= l.maxIdleConnPerHost {
		l.idleConnMux.RUnlock()
		return nil
	}
	l.idleConnMux.RUnlock()

	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()

	if conn.inPool {
		conn.isIdle = true
		return nil
	}

	if l.connCount >= l.maxIdleConn {
		oldPconn := l.idleLRU.removeOldest()
		err := l.removeConnLocked(oldPconn)
		if err != nil {
			return err
		}
	}
	//添加一个连接到连接池,转化连接状态,刷新空闲时间
	conn.idleAt = time.Now()
	if l.idleConnTimeout > 0 {
		if conn.closeTimer != nil {
			conn.closeTimer.Reset(l.idleConnTimeout)
		} else {
			conn.closeTimer = time.AfterFunc(l.idleConnTimeout, conn.closeConnIfStillIdle)
		}
	}
	l.idleConn[cacheKeyHash] = append(l.idleConn[cacheKeyHash], conn)
	conn.inPool = true
	conn.isIdle = true
	return nil
}

// todo keepalive
func (l *lowHttpConnPool) keepAliveCheck() {

}

// 在有写锁的环境中从池子里删除一个连接
func (l *lowHttpConnPool) removeConnLocked(pConn *persistConn) error {
	if pConn.closeTimer != nil {
		pConn.closeTimer.Stop()
	}
	key := pConn.cacheKey.hash()
	pConns := l.idleConn[pConn.cacheKey.hash()]
	switch len(pConns) {
	case 0:
		return utils.Errorf("remove Conn err : [not find this Conn from the Conn pool]")
	case 1:
		if pConns[0] == pConn {
			l.gcPool.Put(pConn)
			delete(l.idleConn, key)
		}
	default:
		for i, v := range pConns {
			if v != pConn {
				continue
			}
			copy(pConns[i:], pConns[i+1:])
			l.idleConn[key] = pConns[:len(pConns)-1]
			break
		}
	}
	return nil
}

// 长连接
type persistConn struct {
	net.Conn   //conn本体
	mu         sync.Mutex
	p          *lowHttpConnPool //连接对应的连接池
	cacheKey   connectKey       //连接池缓存key
	isProxy    bool             //是否使用代理
	idleAt     time.Time        //进入空闲的时间
	closeTimer *time.Timer      //关闭定时器
	isIdle     bool             //是否空闲
	isAlive    bool             //是否存活
	inPool     bool             //是否入池
}

func newPersistConn(key connectKey, pool *lowHttpConnPool, opt ...netx.DialXOption) (*persistConn, error) {
	needProxy := len(key.proxy) > 0
	opt = append(opt, netx.DialX_WithProxy(key.proxy...), netx.DialX_WithKeepAlive(pool.keepAliveTimeout))
	newConn, err := netx.DialX(key.addr, opt...)
	if err != nil {
		return nil, err
	}
	// gc池里取出一个回收的persistConn结构体
	conn := pool.gcPool.Get().(*persistConn)
	conn.Conn = newConn
	conn.p = pool
	conn.cacheKey = key
	conn.isProxy = needProxy
	conn.isAlive = true
	return conn, nil
}

func (pc *persistConn) Close() error {
	return pc.p.putIdleConn(pc)
}

func (pc *persistConn) closeConnIfStillIdle() {
	l := pc.p
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	err := l.removeConnLocked(pc)
	if err != nil {
		log.Error(err)
	}
}

func (pc *persistConn) idle() {

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
