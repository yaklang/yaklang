package cybertunnel

import (
	"context"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/cybertunnel/tpb"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net"
	"sync"
	"time"
)

var tunnelContexts = new(sync.Map)

func outputToVerbose(o *tpb.TunnelOutput) string {
	return fmt.Sprintf("[%v]:[%v]", o.GetFromId(), o.GetRemoteAddr())
}

/*
channel context¬
*/
type channelContext struct {
	feedback io.Writer

	tunnelId        string
	remoteAddr      string
	localHost       string
	localPort       int
	mutex           *sync.Mutex
	dialed          *utils.AtomicBool
	osConn          net.Conn
	fallbackChannel chan *tpb.TunnelOutput

	runOnce *sync.Once

	//
	fs []func(remove, local string)
}

func (c *channelContext) feed(o *tpb.TunnelOutput) {
	if o == nil || c == nil {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("feed failed")
			return
		}
	}()
	select {
	case c.fallbackChannel <- o:
	default:
		_, ok := tunnelContexts.Load(outputToVerbose(o))
		if !ok {
			return
		}
		go func() {
			c.fallbackChannel <- o
		}()
	}
}

func (c *channelContext) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case output, ok := <-c.fallbackChannel:
			if !ok {
				return
			}
			if output.Close {
				close(c.fallbackChannel)
				return
			}

			if !c.dialed.IsSet() {
				c.dialed.Set()

				// dial
				// 创建一个新的 connection
				addr := utils.HostPort(c.localHost, c.localPort)
				osConn, err := netx.DialTCPTimeout(10*time.Second, addr)
				if err != nil {
					panic(fmt.Sprintf("cannot dial to %v, reason: %v", addr, err))
				}
				for _, cb := range c.fs {
					cb(c.remoteAddr, osConn.LocalAddr().String())
				}
				c.osConn = ctxio.NewConn(ctx, osConn)

				// first message
				if len(output.GetData()) > 0 {
					c.osConn.Write(output.GetData())
				}

				go func() {
					io.Copy(c.feedback, c.osConn)
				}()
			} else {
				if c.osConn == nil {
					panic("BUG: osConn cannot be found")
				}
				c.osConn.Write(output.GetData())
			}
		}
	}
}

func getClientTunnelContext(
	ctx context.Context, channelId string, tunnelId string, remoteAddr string, lhost string, lport int, client tpb.Tunnel_CreateTunnelClient,
	fs ...func(string, string)) *channelContext {
	raw, ok := tunnelContexts.Load(channelId)
	if !ok {
		tun := &channelContext{
			tunnelId:        tunnelId,
			remoteAddr:      remoteAddr,
			localHost:       lhost,
			localPort:       lport,
			mutex:           new(sync.Mutex),
			dialed:          utils.NewBool(false),
			fallbackChannel: make(chan *tpb.TunnelOutput, 10*1000*10),
			runOnce:         new(sync.Once),
			feedback:        createTunnelClientToCloseWriter(client, tunnelId, remoteAddr),
			fs:              fs,
		}
		go tun.runOnce.Do(func() {
			defer func() {
				tunnelContexts.Delete(channelId)

				if err := recover(); err != nil {
					log.Errorf("hold")
				}
			}()
			tun.run(ctx)
		})
		tunnelContexts.Store(channelId, tun)
		return tun
	}
	return raw.(*channelContext)
}

func dispatchOutput(ctx context.Context, output *tpb.TunnelOutput, localhost string, localPort int, client tpb.Tunnel_CreateTunnelClient, fs ...func(string, string)) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("recover from handle output")
		}
	}()

	/*
		来往顺序要注意，不要因为一个 conn 导致顺序炸了
	*/
	contextId := outputToVerbose(output)
	chanCtx := getClientTunnelContext(ctx, contextId, output.FromId, output.RemoteAddr, localhost, localPort, client, fs...)
	chanCtx.feed(output)
}

func HoldingCreateTunnelClient(
	client tpb.Tunnel_CreateTunnelClient,
	localhost string, localport int,
	remoteport int, id string,
	fs ...func(remote, local string)) error {
	if id == "" {
		id = uuid.NewV4().String()
	}

	ctx, cancel := context.WithCancel(client.Context())
	defer cancel()

	/*
		id => tunnel
	*/
	var idToLocalPort = make(map[string]int)
	var idToRemotePort = make(map[string]int)
	var idToLocalHost = make(map[string]string)
	idToLocalPort[id] = localport
	idToLocalHost[id] = localhost
	idToRemotePort[id] = remoteport

	var mirrors []*tpb.Mirror
	for id, port := range idToRemotePort {
		mirrors = append(mirrors, &tpb.Mirror{
			Id:      id,
			Port:    int32(port),
			Network: "tcp",
		})
	}

	err := client.Send(&tpb.TunnelInput{Mirrors: mirrors})
	if err != nil {
		return utils.Errorf("create tunnel failed: %s", err)
	}

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf(
					"panic from main loop mirror tunnel: %s => %v, reason: %s",
					fmt.Sprintf("%v - %v", id, utils.HostPort(localhost, localport)),
					remoteport, err,
				)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		for {
			output, err := client.Recv()
			if err != nil {
				panic(err)
			}
			dispatchOutput(
				ctx, output, idToLocalHost[output.GetFromId()], idToLocalPort[output.GetFromId()],
				client, fs...)
		}
	}()
	select {
	case <-ctx.Done():
		return nil
	}
}

func createTunnelClientToCloseWriter(client tpb.Tunnel_CreateTunnelClient, id, remoteAddr string) io.WriteCloser {
	return &createTunnelClientWriter{client, id, remoteAddr}
}

type createTunnelClientWriter struct {
	client     tpb.Tunnel_CreateTunnelClient
	toId       string
	remoteAddr string
}

func (c *createTunnelClientWriter) Write(buf []byte) (int, error) {
	err := c.client.Send(&tpb.TunnelInput{
		ToId:         c.toId,
		Data:         buf,
		ToRemoteAddr: c.remoteAddr,
		Close:        false,
	})
	if err != nil {
		return 0, err
	}
	return len(buf), err
}

func (c *createTunnelClientWriter) Close() error {
	err := c.client.Send(&tpb.TunnelInput{
		ToId:         c.toId,
		ToRemoteAddr: c.remoteAddr,
		Close:        true,
	})
	if err != nil {
		return err
	}
	return err
}

type clientTunnelDesc struct {
	Id          string
	Connections *sync.Map // map[string/*remote-addr*/]*clientConnectionDesc
}

type clientConnectionDesc struct {
	Connection net.Conn
	RemoteAddr string
	Reader     chan []byte
}

/*
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error

	LocalAddr() Addr
	RemoteAddr() Addr
	SetReadDeadline(t time.Time) error
*/

//type Addr interface {
//	Network() string // name of the network (for example, "tcp", "udp")
//	String() string  // string form of address (for example, "192.0.2.1:25", "[2001:db8::1]:80")
//}

func NewTunnelClientConn(id string, remoteAddr string, reader chan []byte, stream tpb.Tunnel_CreateTunnelClient) *TunnelClientConn {
	return &TunnelClientConn{
		id:         id,
		reader:     reader,
		stream:     stream,
		mirrorAddr: remoteAddr,
	}
}

type TunnelClientConn struct {
	id     string
	reader chan []byte
	stream tpb.Tunnel_CreateTunnelClient

	mirrorAddr string
	rbuf       []byte
}

func (c *TunnelClientConn) Read(b []byte) (n int, err error) {
	if len(c.rbuf) > 0 {
		n = copy(b, c.rbuf)
		c.rbuf = c.rbuf[n:]
		return n, nil
	}

	select {
	case res, ok := <-c.reader:
		if !ok {
			return 0, io.EOF
		}

		n = copy(b, res)
		c.rbuf = res[n:]
		return n, nil
	case <-c.stream.Context().Done():
		return 0, io.EOF
	}
}

func (s *TunnelClientConn) Write(b []byte) (int, error) {
	err := s.stream.Send(&tpb.TunnelInput{
		ToId:         s.id,
		Data:         b,
		ToRemoteAddr: s.mirrorAddr,
	})
	if err != nil {
		return 0, io.EOF
	}
	return len(b), nil
}

func (s *TunnelClientConn) Close() error {
	defer func() {
		recover()
	}()
	close(s.reader)
	return nil
}
