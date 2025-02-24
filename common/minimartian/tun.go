package minimartian

import (
	"context"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/nosigpipe"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
	"net"
)

func (p *Proxy) TunInit(ctx context.Context) error {
	s, err := netstackvm.NewTunVirtualMachine(ctx)
	if err != nil {
		return err
	}
	p.tunVM = s
	vm, err := netstackvm.GetDefaultNetStackVirtualMachine()
	if err != nil {
		return err
	}
	p.dial = vm.DialTCP
	return nil
}

func (p *Proxy) TunStart(ctx context.Context) error {
	tunSever := p.tunVM
	if tunSever == nil {
		return utils.Errorf("tun vm is nil")
	}
	l, err := tunSever.ListenTCP()
	if err != nil {
		return err
	}

	statusContext, cancel := context.WithCancel(ctx)
	defer cancel()
	cacheConns, removeConns := p.startConnLog(statusContext)

	for {
		if p.Closing() {
			l.Close()
			return nil
		}

		conn, err := l.Accept()
		if err != nil {
			log.Errorf("mitm: failed to accept: %v", err)
			return err
		}
		if conn == nil {
			continue
		}

		// generate ksuid
		uid := ksuid.New().String()

		nosigpipe.IgnoreSIGPIPE(conn)
		select {
		case <-ctx.Done():
			log.Info("closing martian proxying...")
			conn.Close()
			l.Close()
			return nil
		default:
			cacheConns(uid, conn)
		}

		log.Debugf("mitm: accepted connection from %s", conn.RemoteAddr())
		go func(uidStr string, originConn net.Conn) {
			subCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("tun handle mitm proxy loop failed: %s", err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
				if originConn != nil {
					originConn.Close()
				}
			}()
			defer removeConns(uidStr, originConn)
			var handledConnection net.Conn
			handledConnection, isTls, err := IsTlsHandleShake(originConn)
			if err != nil {
				log.Errorf("check socks5 handle shake failed: %s", err)
				return
			}

			handleContext, err := CreateProxyHandleContext(subCtx, handledConnection)
			if err != nil {
				log.Errorf("create proxy handle context failed: %s", err)
				return
			}

			p.handleLoop(isTls, handledConnection, handleContext)
		}(uid, conn)
	}
}
