package netx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type requestBuilder struct {
	bytes.Buffer
}

func (b *requestBuilder) add(data ...byte) {
	_, _ = b.Write(data)
}

func (c *config) sendReceive(conn net.Conn, req []byte) (resp []byte, err error) {
	defer conn.SetDeadline(time.Time{})
	if c.Context != nil {
		ddl, ok := c.Context.Deadline()
		if ok {
			if err := conn.SetDeadline(ddl); err != nil {
				return nil, err
			}
		}
	} else if c.Timeout > 0 {
		if err := conn.SetDeadline(time.Now().Add(c.Timeout)); err != nil {
			return nil, err
		}
	}
	_, err = conn.Write(req)
	if err != nil {
		return
	}
	resp, err = c.readAll(conn)
	return
}

func (c *config) readAll(conn net.Conn) (resp []byte, err error) {
	resp = make([]byte, 1024)
	if c.Timeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(c.Timeout)); err != nil {
			return nil, err
		}
	}
	n, err := conn.Read(resp)
	resp = resp[:n]
	return
}

func lookupIPv4(host string) (net.IP, error) {
	ipStr := LookupFirst(host)
	if ipStr == "" {
		return nil, fmt.Errorf("host not found: %s", host)
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("(%v)invalid IP address: %s", host, ipStr)
	}
	return ip, nil
}

func splitHostPort(addr string) (host string, port uint16, err error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	portInt, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return "", 0, err
	}
	port = uint16(portInt)
	return
}

// Constants to choose which version of SOCKS protocol to use.
const (
	SOCKS4 = iota
	SOCKS4A
	SOCKS5
)

type (
	config struct {
		Context     context.Context
		Proto       int
		Host        string
		Auth        *auth
		Timeout     time.Duration
		Check       bool
		ProxyDialer func(ctx context.Context, target string) (net.Conn, error)
	}
	auth struct {
		Username string
		Password string
	}
)

func parse(proxyURI string) (*config, error) {
	uri, err := url.Parse(proxyURI)
	if err != nil {
		return nil, err
	}
	cfg := &config{}
	switch uri.Scheme {
	case "socks4":
		cfg.Proto = SOCKS4
	case "socks4a":
		cfg.Proto = SOCKS4A
	case "socks5", "socks5h":
		cfg.Proto = SOCKS5
	default:
		return nil, fmt.Errorf("unknown SOCKS protocol %s", uri.Scheme)
	}
	cfg.Host = uri.Host
	user := uri.User.Username()
	password, _ := uri.User.Password()
	if user != "" || password != "" {
		if user == "" || password == "" || len(user) > 255 || len(password) > 255 {
			return nil, errors.New("invalid user name or password")
		}
		cfg.Auth = &auth{
			Username: user,
			Password: password,
		}
	}
	query := uri.Query()
	timeout := query.Get("timeout")
	if timeout != "" {
		var err error
		cfg.Timeout, err = time.ParseDuration(timeout)
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func (cfg *config) dialSocks5(targetAddr string) (_ net.Conn, err error) {
	ctx := cfg.Context

	// dial TCP
	conn, err := cfg.ProxyDialer(ctx, cfg.Host)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	stopCancel := func() bool { return true }
	if ctx != nil {
		stopCancel = context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stopCancel()
		if deadline, ok := ctx.Deadline(); ok {
			if err = conn.SetDeadline(deadline); err != nil {
				return nil, err
			}
		}
	} else if cfg.Timeout > 0 {
		if err = conn.SetDeadline(time.Now().Add(cfg.Timeout)); err != nil {
			return nil, err
		}
	}
	defer conn.SetDeadline(time.Time{})
	writeRead := func(req []byte, size int) ([]byte, error) {
		if _, err := io.Copy(conn, bytes.NewReader(req)); err != nil {
			return nil, err
		}
		resp := make([]byte, size)
		_, err := io.ReadFull(conn, resp)
		return resp, err
	}

	var req requestBuilder

	version := byte(5) // socks version 5
	// Offer both methods when credentials are configured. Some proxies allow
	// unauthenticated connections even when credentials were supplied.
	req.add(version, 1, 0)
	if cfg.Auth != nil {
		req.Reset()
		req.add(version, 2, 0, 2)
	}

	// version identifier/method selection request
	resp, err := writeRead(req.Bytes(), 2)
	if err != nil {
		return nil, err
	} else if resp[0] != 5 {
		return nil, errors.New("server does not support Socks 5")
	} else if resp[1] != 0 && (cfg.Auth == nil || resp[1] != 2) {
		return nil, errors.New("socks method negotiation failed")
	}
	if resp[1] == 2 {
		version := byte(1) // user/password version 1
		req.Reset()
		req.add(
			version,                      // user/password version
			byte(len(cfg.Auth.Username)), // length of username
		)
		req.add([]byte(cfg.Auth.Username)...)
		req.add(byte(len(cfg.Auth.Password)))
		req.add([]byte(cfg.Auth.Password)...)
		resp, err := writeRead(req.Bytes(), 2)
		if err != nil {
			return nil, err
		} else if resp[0] != version {
			return nil, errors.New("server does not support user/password version 1")
		} else if resp[1] != 0 { // not success
			return nil, errors.New("user/password login failed")
		}
	}

	if cfg.Check { // s5 just auth ok
		if !stopCancel() {
			return nil, ctx.Err()
		}
		return conn, nil
	}
	if targetAddr == "" {
		targetAddr = cfg.Host
	}
	// detail request
	host, port, err := splitHostPort(targetAddr)
	if err != nil {
		return nil, err
	}
	aType := 0x3
	if utils.IsIPv4(host) {
		aType = 0x1
	} else if utils.IsIPv6(host) {
		aType = 0x4
	}

	req.Reset()
	req.add(
		5,           // version number
		1,           // connect command
		0,           // reserved, must be zero
		byte(aType), // address type, 3 means domain name
	)
	if aType == 0x1 {
		req.add(net.ParseIP(host).To4()...)
	} else if aType == 0x4 {
		req.add(net.ParseIP(host).To16()...)
	} else {
		if len(host) > 255 {
			return nil, errors.New("SOCKS5 domain name exceeds 255 bytes")
		}
		req.add(byte(len(host))) // length of domain name
		req.add([]byte(host)...)
	}

	req.add(
		byte(port>>8), // higher byte of destination port
		byte(port),    // lower byte of destination port (big endian)
	)
	resp, err = writeRead(req.Bytes(), 4)
	if err != nil {
		return nil, err
	} else if resp[0] != 5 || resp[2] != 0 {
		return nil, errors.New("invalid SOCKS5 connect reply")
	} else if resp[1] != 0 {
		return nil, errors.New("can't complete SOCKS5 connection")
	}
	var addressSize int
	switch resp[3] {
	case 1:
		addressSize = net.IPv4len
	case 4:
		addressSize = net.IPv6len
	case 3:
		length := make([]byte, 1)
		if _, err = io.ReadFull(conn, length); err != nil {
			return nil, err
		}
		addressSize = int(length[0])
	default:
		return nil, errors.New("invalid SOCKS5 reply address type")
	}
	if _, err = io.CopyN(io.Discard, conn, int64(addressSize+2)); err != nil {
		return nil, err
	}
	if !stopCancel() {
		return nil, ctx.Err()
	}

	return conn, nil
}

//// DialSocksProxy returns the dial function to be used in http.Transport object.
//// Argument socksType should be one of SOCKS4, SOCKS4A and SOCKS5.
//// Argument proxy should be in this format "127.0.0.1:1080".
//func DialSocksProxy(ctx context.Context, socksType int, proxy string, username string, password string) func(string, string) (net.Conn, error) {
//	cfg := &config{Context: ctx, Proto: socksType, Host: proxy}
//	if username != "" {
//		cfg.Auth = &auth{username, password}
//	}
//	return cfg.dialFunc()
//}

func dialSocksProxyCheckConfig(ctx context.Context, socksType int, proxy string, timeout time.Duration, username string, password string) *config {
	cfg := &config{Context: ctx, Proto: socksType, Host: proxy, Check: true, Timeout: timeout}
	if username != "" {
		cfg.Auth = &auth{username, password}
	}
	return cfg
}

func (c *config) dialFunc() func(string) (net.Conn, error) {
	switch c.Proto {
	case SOCKS5:
		return c.dialSocks5
	case SOCKS4, SOCKS4A:
		return c.dialSocks4
	}
	return dialError(fmt.Errorf("unknown SOCKS protocol %v", c.Proto))
}

func dialError(err error) func(string) (net.Conn, error) {
	return func(_ string) (net.Conn, error) {
		return nil, err
	}
}

func (cfg *config) dialSocks4(targetAddr string) (_ net.Conn, err error) {
	socksType := cfg.Proto
	ctx := cfg.Context

	// dial TCP
	conn, err := cfg.ProxyDialer(ctx, cfg.Host)
	if err != nil {
		return nil, err
	}
	if cfg.Check { // s4 just dial ok
		return conn, nil
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	// connection request
	host, port, err := splitHostPort(targetAddr)
	if err != nil {
		return nil, err
	}
	ip := net.IPv4(0, 0, 0, 1).To4()
	if socksType == SOCKS4 {
		ip, err = lookupIPv4(host)
		if err != nil {
			return nil, err
		}
	}
	req := []byte{
		4,                          // version number
		1,                          // command CONNECT
		byte(port >> 8),            // higher byte of destination port
		byte(port),                 // lower byte of destination port (big endian)
		ip[0], ip[1], ip[2], ip[3], // special invalid IP address to indicate the host name is provided
		0, // user id is empty, anonymous proxy only
	}
	if socksType == SOCKS4A {
		req = append(req, []byte(host+"\x00")...)
	}

	resp, err := cfg.sendReceive(conn, req)
	if err != nil {
		return nil, err
	}
	switch resp[1] {
	case 90:
		// request granted
	case 91:
		return nil, errors.New("socks connection request rejected or failed")
	case 92:
		return nil, errors.New("socks connection request rejected because SOCKS server cannot connect to identd on the client")
	case 93:
		return nil, errors.New("socks connection request rejected because the client program and identd report different user-ids")
	default:
		return nil, errors.New("socks connection request failed, unknown error")
	}
	// clear the deadline before returning
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	return conn, nil
}
