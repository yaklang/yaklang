package lowhttp

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"sync"
	"time"
)

type lowHttpConnPool struct {
	idleConnMux        sync.Mutex
	maxIdleConn        int
	maxIdleConnPerHost int
	idleConnCount      int
	idleConn           map[uint64][]*persistConn //空闲连接
	proxy              string
	IdleConnTimeout    time.Duration //连接过期时间
	gcPool             sync.Pool
	//todo conn的LRU
}

// 取出一个空闲连接
// want 检索一个可用的连接，并且把这个连接从连接池中取出来
func (l *lowHttpConnPool) getIdleConn(key connectKey) *persistConn {
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	// 检索是否有符合要求的连接
	if len(l.idleConn[key.hash()]) > 0 {
		for _, pConn := range l.idleConn[key.hash()] {
			if pConn.isAlive == true {
				return pConn
			}
		}
	}
	pConn, err := newPersistConn(key, l)
	if err == nil {
		log.Errorf("connect new tcp conn err:[%v]", err)
		return nil
	}
	return pConn
}

func (l *lowHttpConnPool) putIdleConn(conn *persistConn) {
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	cacheKeyHash := conn.cacheKey.hash()
	//如果超过池规定的单个host可以拥有的最大连接数量则直接放弃添加连接
	if len(l.idleConn[cacheKeyHash]) >= l.maxIdleConnPerHost {
		return
	}

	if l.idleConnCount >= l.maxIdleConn {
		//todo LRU 淘汰一个连接
	}
	//添加一个连接到连接池,转化连接状态,刷新空闲时间
	conn.idleAt = time.Now()
	if l.IdleConnTimeout > 0 {
		if conn.closeTimer != nil {
			conn.closeTimer.Reset(l.IdleConnTimeout)
		} else {
			conn.closeTimer = time.AfterFunc(l.IdleConnTimeout, conn.closeConnIfStillIdle)
		}
	}
	conn.isIdle = true
	l.idleConn[cacheKeyHash][len(l.idleConn[cacheKeyHash])] = conn
}

func (l *lowHttpConnPool) keepAliveCheck() {

}

// 在有写锁的环境中从池子里删除一个连接
func (l *lowHttpConnPool) removeConnLocked(pConn *persistConn) bool {
	if pConn.closeTimer != nil {
		pConn.closeTimer.Stop()
	}
	key := pConn.cacheKey.hash()
	pConns := l.idleConn[pConn.cacheKey.hash()]
	var removed bool
	switch len(pConns) {
	case 0:
		// Nothing
	case 1:
		if pConns[0] == pConn {
			l.gcPool.Put(pConn)
			delete(l.idleConn, key)
			removed = true
		}
	default:
		for i, v := range pConns {
			if v != pConn {
				continue
			}
			copy(pConns[i:], pConns[i+1:])
			l.idleConn[key] = pConns[:len(pConns)-1]
			removed = true
			break
		}
	}
	return removed
}

// 存活的连接

/*
//todo 连接存活探测
//todo 连接存取 队列/栈
//todo 连接生命周期
//todo
*/
type persistConn struct {
	mu         sync.Mutex
	conn       net.Conn         //conn本体
	p          *lowHttpConnPool //连接对应的连接池
	cacheKey   connectKey       //连接池缓存key
	isProxy    bool             //是否使用代理
	idleAt     time.Time        //进入空闲的时间
	closeTimer *time.Timer      //关闭定时器
	isIdle     bool             //是否空闲
	isAlive    bool             //是否存活
}

func newPersistConn(key connectKey, pool *lowHttpConnPool) (*persistConn, error) {
	needProxy := len(key.proxy) > 0
	newConn, err := netx.DialX(key.addr, netx.DialX_WithProxy(key.proxy...))
	if err != nil {
		return nil, err
	}
	// gc池里取出一个回收的persistConn结构体
	conn := pool.gcPool.Get().(*persistConn)
	conn.conn = newConn
	conn.p = pool
	conn.cacheKey = key
	conn.isProxy = needProxy
	conn.isIdle = false
	conn.isAlive = true
	return conn, nil
}

func (pc *persistConn) closeConnIfStillIdle() {
	l := pc.p
	l.idleConnMux.Lock()
	defer l.idleConnMux.Unlock()
	l.removeConnLocked(pc)
}

func (pc *persistConn) idle() {

}

func (c connectKey) hash() uint64 {
	data := []byte(fmt.Sprintf("%#v|%s|%s", c.proxy, c.scheme, c.addr))
	return utils.SimHash(data)
}

type connectKey struct {
	proxy        []string //可以使用的代理
	scheme, addr string   //协议和目标地址
}
