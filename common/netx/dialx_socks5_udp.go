package netx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// socks5Address encodes an endpoint without resolving its domain name.
func socks5Address(endpoint string) ([]byte, error) {
	host, port, err := splitHostPort(endpoint)
	if err != nil {
		return nil, err
	}
	if host == "" {
		return nil, errors.New("empty SOCKS5 host")
	}
	result := make([]byte, 0, 1+len(host)+3)
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			result = append(result, 1)
			result = append(result, ip4...)
		} else {
			result = append(result, 4)
			result = append(result, ip.To16()...)
		}
	} else {
		if strings.ContainsAny(host, "\x00:") {
			return nil, errors.New("invalid SOCKS5 domain name")
		}
		if len(host) > 255 {
			return nil, errors.New("SOCKS5 domain name exceeds 255 bytes")
		}
		result = append(result, 3, byte(len(host)))
		result = append(result, host...)
	}
	return append(result, byte(port>>8), byte(port)), nil
}

func readSocks5Address(reader io.Reader, atyp byte) (string, error) {
	var size int
	switch atyp {
	case 1:
		size = net.IPv4len
	case 4:
		size = net.IPv6len
	case 3:
		var length [1]byte
		if _, err := io.ReadFull(reader, length[:]); err != nil {
			return "", err
		}
		if length[0] == 0 {
			return "", errors.New("empty SOCKS5 domain")
		}
		size = int(length[0])
	default:
		return "", errors.New("invalid SOCKS5 reply address type")
	}
	address := make([]byte, size+2)
	if _, err := io.ReadFull(reader, address); err != nil {
		return "", err
	}
	host := string(address[:size])
	if atyp != 3 {
		host = net.IP(address[:size]).String()
	}
	port := int(address[size])<<8 | int(address[size+1])
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func readSocks5Reply(reader io.Reader) (string, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return "", err
	}
	if header[0] != 5 || header[2] != 0 {
		return "", errors.New("invalid SOCKS5 reply")
	}
	if header[1] != 0 {
		return "", fmt.Errorf("SOCKS5 request failed with status %d", header[1])
	}
	return readSocks5Address(reader, header[3])
}

// Socks5UDPConn carries datagrams through a SOCKS5 UDP ASSOCIATE relay. Each
// Write sends one datagram and each Read returns one datagram. FRAG != 0 is
// rejected because SOCKS5 fragmentation is rarely implemented by relays.
type Socks5UDPConn struct {
	udp     *net.UDPConn
	control net.Conn
	target  []byte
	remote  *net.UDPAddr
	close   sync.Once
}

// DialSocks5UDPContext establishes a SOCKS5 UDP association. The SOCKS server
// resolves a domain target by default; DialX_WithResolveBeforeProxy(true) sends
// a locally resolved IP instead. DialUdpX remains a direct *net.UDPConn API.
func DialSocks5UDPContext(ctx context.Context, target, proxy string, opts ...DialXOption) (_ *Socks5UDPConn, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dialCfg := &dialXConfig{Timeout: 10 * time.Second}
	for _, opt := range opts {
		opt(dialCfg)
	}
	if dialCfg.Timeout <= 0 {
		return nil, errors.New("SOCKS5 UDP timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, dialCfg.Timeout)
	defer cancel()
	credential, err := newProxyCredential(proxy, dialCfg)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(credential.schema) {
	case "socks", "socks5", "socks5h", "s5":
	default:
		return nil, fmt.Errorf("SOCKS5 UDP requires a SOCKS5 proxy: %q", proxy)
	}
	if _, _, err := net.SplitHostPort(credential.proxyAddr); err != nil {
		return nil, err
	}
	if dialCfg.ResolveBeforeProxy {
		host, port, splitErr := net.SplitHostPort(target)
		if splitErr != nil {
			return nil, splitErr
		}
		if net.ParseIP(host) == nil {
			ip := lookupFirstWithContext(ctx, host, dialCfg.DNSOpts...)
			if ip == "" {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("cannot resolve UDP target %q locally", host)
			}
			if dialCfg.DisallowAddress != nil && dialCfg.DisallowAddress.Contains(ip) {
				return nil, fmt.Errorf("disallow UDP address %q", ip)
			}
			target = net.JoinHostPort(ip, port)
		}
	}
	address, err := socks5Address(target)
	if err != nil {
		return nil, err
	}
	if targetHost, _, _ := net.SplitHostPort(target); dialCfg.DisallowAddress != nil && dialCfg.DisallowAddress.Contains(targetHost) {
		return nil, fmt.Errorf("disallow UDP address %q", targetHost)
	}
	if address[len(address)-2] == 0 && address[len(address)-1] == 0 {
		return nil, errors.New("SOCKS5 UDP target port must be nonzero")
	}
	serverCfg := &config{Context: ctx, Host: credential.proxyAddr, ProxyDialer: credential.dialProxyTCP, Check: true}
	if credential.username != "" {
		serverCfg.Auth = &auth{credential.username, credential.password}
	}
	control, err := serverCfg.dialSocks5("")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = control.Close()
		}
	}()
	stopCancel := context.AfterFunc(ctx, func() { _ = control.Close() })
	defer stopCancel()
	if deadline, ok := ctx.Deadline(); ok {
		if err = control.SetDeadline(deadline); err != nil {
			return nil, err
		}
	}
	udpNetwork := "udp4"
	udpBind := &net.UDPAddr{IP: net.IPv4zero}
	if localTCP, ok := control.LocalAddr().(*net.TCPAddr); ok && localTCP.IP.To4() == nil {
		udpNetwork = "udp6"
		udpBind.IP = net.IPv6zero
	}
	if dialCfg.LocalAddr != nil {
		udpBind = dialCfg.LocalAddr
	}
	udp, err := net.ListenUDP(udpNetwork, udpBind)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = udp.Close()
		}
	}()
	local := udp.LocalAddr().(*net.UDPAddr)
	requestAddr := net.JoinHostPort("0.0.0.0", strconv.Itoa(local.Port))
	if udpNetwork == "udp6" {
		requestAddr = net.JoinHostPort("::", strconv.Itoa(local.Port))
	}
	requestAddress, err := socks5Address(requestAddr)
	if err != nil {
		return nil, err
	}
	request := append([]byte{5, 3, 0}, requestAddress...)
	if _, err = control.Write(request); err != nil {
		return nil, err
	}
	relay, err := readSocks5Reply(control)
	if err != nil {
		return nil, err
	}
	if !stopCancel() {
		return nil, ctx.Err()
	}
	_ = control.SetDeadline(time.Time{})
	relayHost, relayPort, err := net.SplitHostPort(relay)
	if err != nil {
		return nil, err
	}
	if relayPort == "0" {
		return nil, errors.New("SOCKS5 UDP relay port is zero")
	}
	if ip := net.ParseIP(relayHost); ip != nil && ip.IsUnspecified() {
		remoteTCP, ok := control.RemoteAddr().(*net.TCPAddr)
		if !ok {
			return nil, errors.New("SOCKS5 proxy has no usable relay address")
		}
		relayHost = remoteTCP.IP.String()
	}
	relayAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(relayHost, relayPort))
	if err != nil {
		return nil, err
	}
	if udpNetwork == "udp4" && relayAddr.IP.To4() == nil || udpNetwork == "udp6" && relayAddr.IP.To4() != nil {
		return nil, errors.New("SOCKS5 UDP relay address family differs from control connection")
	}
	association := &Socks5UDPConn{udp: udp, control: control, target: address, remote: relayAddr}
	go func() {
		_, _ = io.Copy(io.Discard, control)
		_ = association.Close()
	}()
	return association, nil
}

func (c *Socks5UDPConn) Read(p []byte) (int, error) {
	packet := make([]byte, 65535)
	n, sender, err := c.udp.ReadFromUDP(packet)
	if err != nil {
		return 0, err
	}
	if !sender.IP.Equal(c.remote.IP) || sender.Port != c.remote.Port {
		return 0, errors.New("SOCKS5 UDP packet came from an unexpected relay")
	}
	if n < 4 || packet[0] != 0 || packet[1] != 0 {
		return 0, errors.New("invalid SOCKS5 UDP header")
	}
	if packet[2] != 0 {
		return 0, errors.New("SOCKS5 UDP fragmentation is unsupported")
	}
	reader := bytes.NewReader(packet[4:n])
	if _, err := readSocks5Address(reader, packet[3]); err != nil {
		return 0, err
	}
	headerLen := n - reader.Len()
	return copy(p, packet[headerLen:n]), nil
}

func (c *Socks5UDPConn) Write(p []byte) (int, error) {
	packet := make([]byte, 3+len(c.target)+len(p))
	copy(packet[3:], c.target)
	copy(packet[3+len(c.target):], p)
	if _, err := c.udp.WriteTo(packet, c.remote); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *Socks5UDPConn) Close() error {
	var err error
	c.close.Do(func() {
		err = errors.Join(c.udp.Close(), c.control.Close())
	})
	return err
}

func (c *Socks5UDPConn) LocalAddr() net.Addr                { return c.udp.LocalAddr() }
func (c *Socks5UDPConn) RemoteAddr() net.Addr               { return c.remote }
func (c *Socks5UDPConn) SetDeadline(t time.Time) error      { return c.udp.SetDeadline(t) }
func (c *Socks5UDPConn) SetReadDeadline(t time.Time) error  { return c.udp.SetReadDeadline(t) }
func (c *Socks5UDPConn) SetWriteDeadline(t time.Time) error { return c.udp.SetWriteDeadline(t) }
