package netstackvm

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun/netstack"
	"net"
)

type TunSpoofingListener struct {
	ctx      context.Context
	cancel   context.CancelFunc
	tvm      *TunVirtualMachine
	connChan chan net.Conn
}

func (t *TunSpoofingListener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-t.connChan:
		if !ok {
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-t.ctx.Done():
		return nil, t.ctx.Err()
	}
}

func (t *TunSpoofingListener) Close() error {
	t.cancel()
	return nil
}

func (t *TunSpoofingListener) Addr() net.Addr {
	return &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	}
}

func NewTunSpoofingListener(ctx context.Context, c chan net.Conn) *TunSpoofingListener {
	if ctx == nil {
		ctx = context.Background()
	}
	baseCtx, cancel := context.WithCancel(ctx)
	tl := &TunSpoofingListener{
		ctx:      baseCtx,
		cancel:   cancel,
		connChan: c,
	}
	return tl
}

var _ net.Conn = netstack.TCPConn(nil)
var _ net.Listener = &TunSpoofingListener{}

func (t *TunVirtualMachine) GetListener() *TunSpoofingListener {
	ch := make(chan net.Conn, 65535)

	lis := NewTunSpoofingListener(t.ctx, ch)
	t.hijackedMutex.Lock()
	defer t.hijackedMutex.Unlock()

	if t.hijackedHandler != nil {
		origin := t.hijackedHandler
		t.hijackedHandler = func(conn netstack.TCPConn) {
			select {
			case ch <- conn:
			case <-t.ctx.Done():
				log.Error("context cancelled while sending conn")
				return
			default:
				log.Error("conn chan is full and dropping conn")
			}
			origin(conn)
		}
	} else {
		t.hijackedHandler = func(conn netstack.TCPConn) {
			select {
			case ch <- conn:
			case <-t.ctx.Done():
				log.Error("context cancelled while sending conn")
				return
			default:
				log.Error("conn chan is full and dropping conn")
			}
		}
	}

	return lis
}
