package cybertunnel

import (
	"context"
	"io"
	"net"
	"yaklang/common/cybertunnel/tpb"
	"yaklang/common/log"
	"yaklang/common/utils"
	"sync"
	"time"
)

type tunnelDesc struct {
	LocalPort int
	// tcp / udp
	Network string

	// for tcp
	Listener    net.Listener
	Connections *sync.Map // map[string]*connectionDesc

	// for udp
	UDPConn   *net.UDPConn
	UDPReader chan *tpb.TunnelInput
}

type connectionDesc struct {
	Connection net.Conn
	RemoteAddr string
	Reader     chan *tpb.TunnelInput
}

func (t *tunnelDesc) Close() {
	if t.Listener != nil {
		t.Listener.Close()
	}
	if t.Connections != nil {
		t.Connections.Range(func(key, value interface{}) bool {
			c := value.(*connectionDesc)
			c.Connection.Close()

			defer func() {
				recover()
			}()
			close(c.Reader)
			return true
		})
	}

	if t.UDPReader != nil {
		defer func() {
			recover()
		}()
		close(t.UDPReader)
	}

	if t.UDPConn != nil {
		t.UDPConn.Close()
	}
}

func (s *TunnelServer) CreateTunnel(server tpb.Tunnel_CreateTunnelServer) error {
	rootCtx, rootCtxCancel := context.WithCancel(server.Context())
	defer rootCtxCancel()

	first, err := server.Recv()
	if err != nil {
		return err
	}

	if len(first.GetMirrors()) == 0 {
		return utils.Errorf("first mirrors empty!")
	}

	defer func() {
		for _, m := range first.Mirrors {
			RemoveTunnel(m.GetId())
		}
	}()

	var idToTunnel = make(map[string]*tunnelDesc)
	for _, i := range first.Mirrors {
		i := i
		if i.GetId() == "" {
			return utils.Error("mirror id cannot be empty")
		}

		port := i.GetPort()
		tunnel, _ := GetTunnel(i.GetId())
		if tunnel != nil && tunnel.Port > 0 {
			port = int32(tunnel.Port)
			go func() {
				// 每秒更新一下 TTL
				for {
					GetTunnel(i.GetId())
					select {
					case <-rootCtx.Done():
						return
					default:
						time.Sleep(time.Second)
					}
				}
			}()
		}

		if port <= 0 {
			return utils.Error("mirror port cannot be empty")
		}

		proto := i.GetNetwork()
		if proto == "" {
			proto = "tcp"
		}

		if proto == "tcp" {
			log.Infof("cyber tunnel listen: %v:\\\\%v", proto, utils.HostPort("0.0.0.0", port))
			lis, err := net.Listen(proto, utils.HostPort("0.0.0.0", port))
			if err != nil {
				log.Errorf("listen failed: %s", err)
				return err
			}

			idToTunnel[i.GetId()] = &tunnelDesc{
				LocalPort:   int(port),
				Listener:    lis,
				Network:     proto,
				Connections: new(sync.Map), //make(map[string]*connectionDesc),
				//UDPReader:   make(chan *tpb.TunnelInput, 10000),
			}
		} else if proto == "udp" {
			conn, err := net.ListenUDP(proto, &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: int(port)})
			if err != nil {
				log.Errorf("listen failed: %s", err)
				return err
			}

			idToTunnel[i.GetId()] = &tunnelDesc{
				LocalPort: int(port),
				UDPConn:   conn,
				Network:   proto,
				//Connections: new(sync.Map), //make(map[string]*connectionDesc),
				UDPReader: make(chan *tpb.TunnelInput, 10000),
			}
		} else {
			return utils.Errorf("unsupported proto: %s", proto)
		}

	}

	// 按照 ID 做分流
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Error(err)
			}
		}()
		for {
			rsp, err := server.Recv()
			if err != nil {
				return
			}

			desc, ok := idToTunnel[rsp.GetToId()]
			if !ok {
				continue
			}

			switch desc.Network {
			case "tcp":
				connRaw, ok := desc.Connections.Load(rsp.GetToRemoteAddr())
				if !ok {
					continue
				}
				conn := connRaw.(*connectionDesc)

				if rsp.GetClose() {
					close(conn.Reader)
					conn.Connection.Close()
					desc.Connections.Delete(rsp.GetToRemoteAddr())
					continue
				}

				select {
				case conn.Reader <- rsp:
				case <-server.Context().Done():
					return
				}
			case "udp":
				select {
				case desc.UDPReader <- rsp:
				case <-server.Context().Done():
					return
				}
			}

		}
	}()

	go func() {
		ctx := server.Context()
		select {
		case <-ctx.Done():
			for _, i := range idToTunnel {
				i.Close()
			}
		}
	}()

	swg := new(sync.WaitGroup)
	swg.Add(len(idToTunnel))
	for id, i := range idToTunnel {
		i := i
		id := id
		go func() {
			defer swg.Done()
			switch i.Network {
			case "tcp":
				log.Infof("start to listen on %v:\\\\%s", id, i.Listener.Addr().String())
				for {
					conn, err := i.Listener.Accept()
					if err != nil {
						log.Errorf("accept conn from %s failed: %s", i.Listener.Addr().String(), err)
						return
					}
					// 为 conn 创建虚拟链接
					remoteAddr := conn.RemoteAddr().String()
					c := &connectionDesc{
						Connection: conn,
						RemoteAddr: remoteAddr,
						Reader:     make(chan *tpb.TunnelInput, 10000),
					}
					i.Connections.Store(remoteAddr, c)

					tConn := NewTunnelServerConn(id, c, server)
					go func() {
						defer i.Connections.Delete(remoteAddr)
						defer log.Infof("close tcp conn %v => %v", "tunnel", conn.RemoteAddr())
						io.Copy(conn, tConn)
						conn.Close()
						tConn.Close()
					}()
					go func() {
						defer i.Connections.Delete(remoteAddr)
						defer log.Infof("close tcp conn %v => %v", conn.RemoteAddr(), "tunnel")
						io.Copy(tConn, conn)
						conn.Close()
						tConn.Close()
					}()
				}
			case "udp":
				log.Infof("start to handle %v://%v", "udp", i.UDPConn.LocalAddr())
				wg := new(sync.WaitGroup)
				wg.Add(2)
				go func() {
					defer wg.Done()
					for {
						// 从 con 接收，然后转发
						var raw []byte
						var lastAddr *net.UDPAddr
						var buf = make([]byte, 4096)
						for {
							n, addr, err := i.UDPConn.ReadFromUDP(buf)
							if err != nil {
								break
							}
							lastAddr = addr
							// 可能还没读完
							if n >= 4096 {
								raw = append(raw, buf...)
								buf = make([]byte, 4096)
								continue
							} else {
								raw = append(raw, buf...)
								break
							}
						}
						err := server.Send(&tpb.TunnelOutput{
							FromId:     id,
							RemoteAddr: lastAddr.String(),
							Data:       raw,
						})
						if err != nil {
							log.Errorf("send to tunnel client failed: %s", err)
							return
						}
					}
				}()
				go func() {
					defer wg.Done()
					for {
						in, ok := <-i.UDPReader
						if !ok {
							log.Error("udp reader closed")
							return
						}
						host, port, _ := utils.ParseStringToHostPort(in.GetToRemoteAddr())
						_, err := i.UDPConn.WriteToUDP(in.GetData(), &net.UDPAddr{
							IP:   net.ParseIP(host),
							Port: port,
						})
						if err != nil {
							log.Error("write to udp failed")
							return
						}
					}
				}()
				wg.Wait()
			}
		}()
	}
	swg.Wait()
	return nil
}

func NewTunnelServerConn(id string, desc *connectionDesc, stream tpb.Tunnel_CreateTunnelServer) *TunnelServerConn {
	return &TunnelServerConn{
		id:     id,
		desc:   desc,
		stream: stream,
	}
}

type TunnelServerConn struct {
	io.ReadWriteCloser

	id     string
	desc   *connectionDesc
	stream tpb.Tunnel_CreateTunnelServer
	rbuf   []byte
}

func (c *TunnelServerConn) Read(b []byte) (n int, err error) {
	if len(c.rbuf) > 0 {
		n = copy(b, c.rbuf)
		c.rbuf = c.rbuf[n:]
		return n, nil
	}

	select {
	case res, ok := <-c.desc.Reader:
		if !ok {
			return 0, utils.Error("reader closed")
		}

		buf := res.GetData()
		n = copy(b, buf)
		c.rbuf = buf[n:]
		return n, nil
	case <-c.stream.Context().Done():
		return 0, utils.Error("context canceled")
	}
}

func (s *TunnelServerConn) Write(b []byte) (int, error) {
	//log.Infof("send[%d]: %s", len(b), strconv.Quote(string(b)))
	err := s.stream.Send(&tpb.TunnelOutput{
		FromId:     s.id,
		RemoteAddr: s.desc.RemoteAddr,
		Data:       b,
	})
	if err != nil {
		return 0, err
	}
	return len(b), err
}

func (s *TunnelServerConn) Close() error {
	defer func() {
		recover()
	}()
	close(s.desc.Reader)
	s.stream.Send(&tpb.TunnelOutput{
		FromId:     s.id,
		RemoteAddr: s.desc.RemoteAddr,
		Data:       nil,
		Close:      true,
	})
	return nil
}
