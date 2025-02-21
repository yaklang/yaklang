package minimartian

import (
	"context"
	"encoding/binary"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/netx/dns_lookup"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

type S5Config struct {
	HandshakeTimeout    time.Duration
	S5RequestTimeout    time.Duration
	DialDstTimeout      time.Duration
	Debug               bool
	DownstreamHTTPProxy string
	ProxyUsername       string
	ProxyPassword       string
	Addr                string
	UdpChangeCache      *utils.Cache[*UDPExchange]
	UdpSrcCache         *utils.Cache[struct{}]
	UdpConn             *net.UDPConn
}

func NewSocks5Config() *S5Config {
	return &S5Config{
		HandshakeTimeout: 30 * time.Second,
		S5RequestTimeout: 30 * time.Second,
		DialDstTimeout:   30 * time.Second,
		UdpChangeCache:   utils.NewTTLCache[*UDPExchange](time.Minute * 5),
		UdpSrcCache:      utils.NewTTLCache[struct{}](time.Minute * 5),
	}
}

var (
	ErrS5Version  = errors.New("invalid s5 version")
	ErrBadRequest = errors.New("bad s5 request")
)

const (
	socks5Version = 0x05

	authNone                = 0x00
	authWithUsernameAndPass = 0x02
	authNoAcceptable        = 0xFF

	UsernameAndPassVersion     = 0x01
	UsernameAndPassAuthSuccess = 0x00
	UsernameAndPassAuthFail    = 0x01

	commandConnect      = 0x01
	commandBind         = 0x02
	commandUDPAssociate = 0x03

	addrTypeIPv4 = 0x01
	addrTypeFQDN = 0x03
	addrTypeIPv6 = 0x04

	replySuccess = 0x00
)

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

func (c *S5Config) Handshake(conn net.Conn) error {
	conn.SetReadDeadline(time.Now().Add(c.HandshakeTimeout))
	defer conn.SetReadDeadline(time.Time{})

	meta, err := utils.ReadN(conn, 2)
	if err != nil || len(meta) < 2 {
		return utils.Errorf("read negotiation header failed: %s", err)
	}

	version := meta[0]
	nMethods := meta[1]
	if version != socks5Version {
		return utils.Errorf("negotiation socks5 version failed: %v", strconv.Quote(string(meta)))
	}

	var methods = make([]byte, nMethods)
	if _, err := io.ReadFull(conn, methods); err != nil {
		return utils.Errorf("negotiation shake auth methods failed: %s", err)
	}

	supportMethods := []byte{authWithUsernameAndPass}
	needAuth := c.ProxyPassword != "" || c.ProxyUsername != ""
	if !needAuth {
		supportMethods = append(supportMethods, authNone)
	}

	authFlag := byte(authNoAcceptable)
	for _, method := range methods {
		if utils.ContainsAny(supportMethods, method) {
			authFlag = method
		}
	}

	_, err = conn.Write([]byte{socks5Version, authFlag})
	if err != nil {
		return err
	}

	if needAuth {
		authVersion, err := utils.ReadN(conn, 1)
		if err != nil {
			return err
		}

		if len(authVersion) <= 0 {
			return utils.Errorf("read auth version get empty")
		}

		if authVersion[0] != byte(UsernameAndPassVersion) {
			return utils.Errorf("auth version failed: %v", strconv.Quote(string(authVersion[0])))
		}

		getDate := func() (string, error) {
			lengthByte, err := utils.ReadN(conn, 1)
			if err != nil {
				return "", err
			}
			if len(lengthByte) <= 0 {
				return "", utils.Errorf("read data length get empty")
			}
			data, err := utils.ReadN(conn, int(lengthByte[0]))
			if err != nil {
				return "", err
			}
			return string(data), nil
		}

		username, err := getDate()
		if err != nil {
			return err
		}
		password, err := getDate()
		if err != nil {
			return err
		}

		if !(c.ProxyUsername == username && c.ProxyPassword == password) {
			conn.Write([]byte{UsernameAndPassVersion, UsernameAndPassAuthFail})
			return utils.Errorf("auth failed")
		}
		conn.Write([]byte{UsernameAndPassVersion, UsernameAndPassAuthSuccess})
	}

	return nil
}

func ParseAddress(address string) (a byte, addr []byte, port []byte, err error) {
	var h, p string
	h, p, err = net.SplitHostPort(address)
	if err != nil {
		return
	}
	ip := net.ParseIP(h)
	if ip4 := ip.To4(); ip4 != nil {
		a = addrTypeIPv4
		addr = ip4
	} else if ip6 := ip.To16(); ip6 != nil {
		a = addrTypeFQDN
		addr = ip6
	} else {
		a = addrTypeFQDN
		addr = []byte{byte(len(h))}
		addr = append(addr, []byte(h)...)
	}
	i, _ := strconv.Atoi(p)
	port = make([]byte, 2)
	binary.BigEndian.PutUint16(port, uint16(i))
	return
}

type S5Request struct {
	Ver     byte
	Cmd     byte
	Rsv     byte // 0x00
	Atyp    byte
	DstHost []byte
	DstPort []byte // 2 bytes
}

func (r *S5Request) GetDstHost() string {
	switch r.Atyp {
	case addrTypeIPv4:
		return net.IP(r.DstHost).String()
	case addrTypeIPv6:
		return net.IP(r.DstHost).String()
	case addrTypeFQDN:
		return string(r.DstHost)
	default:
		return ""
	}
}

func (r *S5Request) GetDstPort() int {
	return int(binary.BigEndian.Uint16(r.DstPort))
}

func (c *S5Config) HandleS5RequestHeader(conn net.Conn) (*S5Request, error) {
	conn.SetDeadline(time.Now().Add(c.S5RequestTimeout))
	defer conn.SetDeadline(time.Time{})
	bb := make([]byte, 4)
	if _, err := io.ReadFull(conn, bb); err != nil {
		return nil, err
	}
	if bb[0] != socks5Version {
		return nil, ErrS5Version
	}
	var addr []byte
	addrType := bb[3]
	switch addrType {
	case addrTypeIPv4:
		addr = make(net.IP, net.IPv4len)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return nil, err
		}
	case addrTypeIPv6:
		addr = make(net.IP, net.IPv6len)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return nil, err
		}
	case addrTypeFQDN:
		var buf []byte
		buf, err := utils.ReadN(conn, 1)
		if err != nil || len(buf) < 1 {
			return nil, utils.Errorf("read fqdn length failed: %s", err)
		}
		fqdnLen := buf[0]
		addr = make([]byte, fqdnLen)
		if _, err = io.ReadFull(conn, addr); err != nil {
			return nil, err
		}
	default:
		return nil, ErrBadRequest
	}

	port := make([]byte, 2)
	if _, err := io.ReadFull(conn, port); err != nil {
		return nil, err
	}
	return &S5Request{
		Ver:     bb[0],
		Cmd:     bb[1],
		Rsv:     bb[2],
		Atyp:    bb[3],
		DstHost: addr,
		DstPort: port,
	}, nil
}

func NewReply(host net.IP, port int) []byte {
	var portsBytes = make([]byte, 2)
	binary.BigEndian.PutUint16(portsBytes, uint16(port))
	reply := []byte{socks5Version, replySuccess, 0x00}
	if ipIns := host.To4(); ipIns != nil {
		reply = append(reply, addrTypeIPv4)
		reply = append(reply, ipIns...)
		reply = append(reply, portsBytes...)
	} else {
		reply = append(reply, addrTypeIPv4)
		reply = append(reply, 0x00, 0x00, 0x00, 0x00)
		reply = append(reply, portsBytes...)
	}
	return reply
}

func (c *S5Config) HandleConnect(conn net.Conn, req *S5Request) error {
	dstConn, err := netx.DialTCPTimeout(c.DialDstTimeout, utils.HostPort(string(req.DstHost), binary.BigEndian.Uint16(req.DstPort)))
	if err != nil {
		return utils.Errorf("dial target failed: %s", err)
	}
	if dstConn == nil {
		return utils.Error("BUG: act conn is nil")
	}

	// 告诉客户端已成功连接到目标服务器
	host, port, _ := utils.ParseStringToHostPort(dstConn.LocalAddr().String())
	conn.Write(NewReply(net.ParseIP(host), port))
	err = c.ConnectionFallback(dstConn, conn)
	if err != nil {
		return err
	}
	return nil
}

func (c *S5Config) HandleBind(conn net.Conn, req *S5Request) error {
	target := utils.HostPort(string(req.DstHost), binary.BigEndian.Uint16(req.DstPort))
	bindPort := utils.GetRandomAvailableTCPPort()
	targetHost, targetPort := string(req.DstHost), binary.BigEndian.Uint16(req.DstPort)
	if !utils.IsIPv4(targetHost) && !utils.IsIPv6(targetHost) {
		targetHost = dns_lookup.LookupFirst(targetHost, dns_lookup.WithTimeout(3*time.Second))
		if targetHost == "" {
			return errors.Errorf("cannot found domain[%s]'s ip address", target)
		}
	}
	_, _, bindHost, err := netutil.Route(time.Second*5, targetHost)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", utils.HostPort(bindHost.String(), bindPort))
	if err != nil {
		return err
	}

	// 告诉客户端已开始监听
	_, err = conn.Write(NewReply(bindHost, bindPort))
	if err != nil {
		return err
	}

	defer listener.Close()
	for {
		dstConn, err := listener.Accept()
		if err != nil {
			return utils.Errorf("socks5 server bind mode accept err %v", err)
		}
		if dstConn.RemoteAddr().String() == utils.HostPort(targetHost, targetPort) {
			// 第二次 reply
			_, err = conn.Write(NewReply(net.ParseIP(targetHost), int(targetPort)))
			if err != nil {
				return err
			}
			err := c.ConnectionFallback(dstConn, conn)
			if err != nil {
				return utils.Errorf("socks5 server bind mode connectFallback err %v", err)
			}
			break
		}
	}

	return nil
}

func (c *S5Config) HandleUDPAssociate(conn net.Conn, req *S5Request) error {
	srcAddr := conn.RemoteAddr().String()
	clientHost, _, _ := utils.ParseStringToHostPort(srcAddr)
	if clientHost == string(req.DstHost) {
		srcAddr = utils.HostPort(string(req.DstHost), binary.BigEndian.Uint16(req.DstPort))
	}
	host, port, _ := utils.ParseStringToHostPort(c.Addr)
	if _, err := conn.Write(NewReply(net.ParseIP(host), port)); err != nil {
		return err
	}
	c.UdpSrcCache.Set(srcAddr, struct{}{})
	return nil
}

type Datagram struct {
	Rsv     []byte // 0x00 0x00
	Frag    byte
	Atyp    byte
	DstHost []byte
	DstPort []byte // 2 bytes
	Data    []byte
}

func NewDatagramFromBytes(bb []byte) (*Datagram, error) {
	n := len(bb)
	minl := 4
	if n < minl {
		return nil, ErrBadRequest
	}
	var host []byte
	addrType := bb[3]
	switch addrType {
	case addrTypeIPv4:
		minl += 4
		if n < minl {
			return nil, ErrBadRequest
		}
		host = bb[minl-4 : minl]
	case addrTypeIPv6:
		minl += 16
		if n < minl {
			return nil, ErrBadRequest
		}
		host = bb[minl-16 : minl]
	case addrTypeFQDN:
		minl += 1
		if n < minl {
			return nil, ErrBadRequest
		}
		l := bb[4]
		if l == 0 {
			return nil, ErrBadRequest
		}
		minl += int(l)
		if n < minl {
			return nil, ErrBadRequest
		}
		host = bb[minl-int(l) : minl]
		host = append([]byte{l}, host...)
	default:
		return nil, ErrBadRequest
	}
	minl += 2
	if n <= minl {
		return nil, ErrBadRequest
	}
	port := bb[minl-2 : minl]
	data := bb[minl:]
	d := &Datagram{
		Rsv:     bb[0:2],
		Frag:    bb[2],
		Atyp:    bb[3],
		DstHost: host,
		DstPort: port,
		Data:    data,
	}
	return d, nil
}

func NewDatagram(atyp byte, dstaHost []byte, dstport []byte, data []byte) *Datagram {
	if atyp == addrTypeFQDN {
		dstaHost = append([]byte{byte(len(dstaHost))}, dstaHost...)
	}
	return &Datagram{
		Rsv:     []byte{0x00, 0x00},
		Frag:    0x00,
		Atyp:    atyp,
		DstHost: dstaHost,
		DstPort: dstport,
		Data:    data,
	}
}

func (d *Datagram) Bytes() []byte {
	b := make([]byte, 0)
	b = append(b, d.Rsv...)
	b = append(b, d.Frag)
	b = append(b, d.Atyp)
	b = append(b, d.DstHost...)
	b = append(b, d.DstPort...)
	b = append(b, d.Data...)
	return b
}

type UDPExchange struct {
	ClientAddr *net.UDPAddr
	RemoteConn net.Conn
}

func (c *S5Config) UDPHandle(addr *net.UDPAddr, d *Datagram) error {
	src := addr.String()
	send := func(ue *UDPExchange, data []byte) error {
		_, err := ue.RemoteConn.Write(data)
		if err != nil {
			return err
		}
		return nil
	}

	dst := utils.HostPort(string(d.DstHost), d.DstPort)
	var ue *UDPExchange
	ue, ok := c.UdpChangeCache.Get(src + dst)
	if ok {
		return send(ue, d.Data)
	}

	_, ok = c.UdpSrcCache.Get(src)
	if !ok {
		log.Warnf("UDP src not found: %s", src)
		return nil
	}

	var err error
	remoteAddr, err := net.ResolveUDPAddr("udp", utils.HostPort(string(d.DstHost), d.DstPort))
	if err != nil {
		return err
	}
	localAddr, err := net.ResolveUDPAddr("udp", c.UdpConn.LocalAddr().String())
	rc, err := net.DialUDP("udp", localAddr, remoteAddr)
	ue = &UDPExchange{
		ClientAddr: addr,
		RemoteConn: rc,
	}
	if err := send(ue, d.Data); err != nil {
		ue.RemoteConn.Close()
		return err
	}
	c.UdpChangeCache.Set(src+remoteAddr.String(), ue)
	go func(ue *UDPExchange, dst string) {
		defer func() {
			ue.RemoteConn.Close()
			c.UdpChangeCache.Remove(ue.ClientAddr.String() + dst)
		}()
		var b [65507]byte
		for {
			n, err := ue.RemoteConn.Read(b[:])
			if err != nil {
				return
			}
			a, host, port, err := ParseAddress(dst)
			if err != nil {
				log.Println(err)
				return
			}
			if a == addrTypeFQDN {
				host = host[1:]
			}
			d1 := NewDatagram(a, host, port, b[0:n])
			if _, err := c.UdpConn.WriteToUDP(d1.Bytes(), ue.ClientAddr); err != nil {
				return
			}
		}
	}(ue, dst)
	return nil
}

func (c *S5Config) Serve(Addr string) error {
	listener, err := net.Listen("tcp", Addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	c.Addr = Addr
	udpAddr, err := net.ResolveUDPAddr("udp", Addr)
	if err != nil {
		return utils.Errorf("resolve udp addr failed: %s", err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return utils.Errorf("listen udp failed: %s", err)
	}
	c.UdpConn = udpConn

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				b := make([]byte, 65507)
				n, addr, err := udpConn.ReadFromUDP(b)
				if err != nil {
					log.Errorf("read udp failed: %v", err)
					cancel()
					return
				}
				go func(addr *net.UDPAddr, b []byte) {
					d, err := NewDatagramFromBytes(b)
					if err != nil {
						log.Println(err)
						return
					}
					if d.Frag != 0x00 {
						log.Println("Ignore frag", d.Frag)
						return
					}
					if err := c.UDPHandle(addr, d); err != nil {
						log.Println(err)
						return
					}
				}(addr, b[0:n])
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			break
		default:
			conn, err := listener.Accept()
			if err != nil {
				log.Errorf("accept failed: %v", err)
				continue
			}
			go func() {
				defer func() {
					if conn != nil {
						conn.Close()
					}
					if err := recover(); err != nil {
						log.Errorf("serve socks5 connection panic: %v", err)
					}
				}()
				err := c.ServeConn(conn)
				if err != nil {
					log.Errorf("serve socks5 connection failed: %v", err)
				}
			}()
		}
	}
}

func (c *S5Config) ServeConn(conn net.Conn) error {
	defer func() {
		if conn != nil {
			conn.Close()
		}
		if err := recover(); err != nil {
			log.Errorf("serve socks5 connection panic: %v", err)
		}
	}()

	err := c.Handshake(conn)
	if err != nil {
		return err
	}

	req, err := c.HandleS5RequestHeader(conn)
	if err != nil {
		return err
	}

	switch req.Cmd {
	case commandConnect:
		err := c.HandleConnect(conn, req)
		if err != nil {
			return err
		}
	case commandBind:
		err := c.HandleBind(conn, req)
		if err != nil {
			return err
		}
	case commandUDPAssociate:
		err := c.HandleUDPAssociate(conn, req)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *S5Config) ConnectionFallback(src, proxiedConn net.Conn) error {
	// fullback
	wg := new(sync.WaitGroup)
	wg.Add(2)
	dst := proxiedConn
	wCtx, cancel := context.WithCancel(context.Background())
	ctxSrc := ctxio.NewReaderWriter(wCtx, src)
	ctxDst := ctxio.NewReaderWriter(wCtx, dst)

	// 从 src 读取数据，写入 dst
	go func() {
		defer func() {
			cancel()
			wg.Done()
		}()
		var err error
		if c.Debug {
			_, err = io.Copy(ctxDst, io.TeeReader(ctxSrc, os.Stdout))
		} else {
			_, err = io.Copy(ctxDst, ctxSrc)
		}
		if err != nil && c.Debug && err != io.EOF {
			log.Warnf("bridge %v -> %v failed: %s", src.RemoteAddr().String(), dst.RemoteAddr().String(), err)
		}
	}()

	// 从 dst 读取数据，写入 src
	go func() {
		defer func() {
			cancel()
			wg.Done()
		}()

		var err error
		if c.Debug {
			_, err = io.Copy(ctxSrc, io.TeeReader(ctxDst, os.Stdout))
		} else {
			_, err = io.Copy(ctxSrc, ctxDst)
		}
		if err != nil && c.Debug && err != io.EOF {
			log.Warnf("bridge %v -> %v failed: %s", src.RemoteAddr().String(), dst.RemoteAddr().String(), err)
		}
	}()
	wg.Wait()
	return nil
}

func (c *S5Config) HijackSource(src, proxiedConn net.Conn) (net.Conn, net.Conn, bool, error) {
	// martian s5 is no need to hijack
	return src, proxiedConn, true, nil
}
