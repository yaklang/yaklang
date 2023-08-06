package netx

import (
	"bytes"
	"errors"
	"github.com/yaklang/yaklang/common/log"
	"net"
	"net/url"
	"strconv"
	"time"
)

import (
	"fmt"
)

type requestBuilder struct {
	bytes.Buffer
}

func (b *requestBuilder) add(data ...byte) {
	_, _ = b.Write(data)
}

func (c *config) sendReceive(conn net.Conn, req []byte) (resp []byte, err error) {
	if c.Timeout > 0 {
		if err := conn.SetWriteDeadline(time.Now().Add(c.Timeout)); err != nil {
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
		Proto   int
		Host    string
		Auth    *auth
		Timeout time.Duration
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
	case "socks5":
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
RECON:
	proxy := cfg.Host

	// dial TCP
	conn, err := DialTimeoutWithoutProxy(cfg.Timeout, "tcp", proxy)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	var req requestBuilder

	version := byte(5) // socks version 5
	method := byte(0)  // method 0: no authentication (only anonymous access supported for now)
	if cfg.Auth != nil {
		method = 2 // method 2: username/password
	}

	// version identifier/method selection request
	req.add(
		version, // socks version
		1,       // number of methods
		method,
	)

	resp, err := cfg.sendReceive(conn, req.Bytes())
	if err != nil {
		return nil, err
	} else if len(resp) != 2 {
		return nil, errors.New("server does not respond properly")
	} else if resp[0] != 5 {
		return nil, errors.New("server does not support Socks 5")
	} else if resp[1] != method {
		if cfg.Auth != nil {
			log.Warn("remote socks5 proxy do not have authentication, try fall back using no authentication")
			cfg.Auth = nil
			goto RECON
		}
		return nil, errors.New("socks method negotiation failed")
	}
	if cfg.Auth != nil {
		version := byte(1) // user/password version 1
		req.Reset()
		req.add(
			version,                      // user/password version
			byte(len(cfg.Auth.Username)), // length of username
		)
		req.add([]byte(cfg.Auth.Username)...)
		req.add(byte(len(cfg.Auth.Password)))
		req.add([]byte(cfg.Auth.Password)...)
		resp, err := cfg.sendReceive(conn, req.Bytes())
		if err != nil {
			return nil, err
		} else if len(resp) != 2 {
			return nil, errors.New("server does not respond properly")
		} else if resp[0] != version {
			return nil, errors.New("server does not support user/password version 1")
		} else if resp[1] != 0 { // not success
			return nil, errors.New("user/password login failed")
		}
	}

	// detail request
	host, port, err := splitHostPort(targetAddr)
	if err != nil {
		return nil, err
	}
	req.Reset()
	req.add(
		5,               // version number
		1,               // connect command
		0,               // reserved, must be zero
		3,               // address type, 3 means domain name
		byte(len(host)), // address length
	)
	req.add([]byte(host)...)
	req.add(
		byte(port>>8), // higher byte of destination port
		byte(port),    // lower byte of destination port (big endian)
	)
	resp, err = cfg.sendReceive(conn, req.Bytes())
	if err != nil {
		return
	} else if len(resp) != 10 {
		return nil, errors.New("server does not respond properly")
	} else if resp[1] != 0 {
		return nil, errors.New("can't complete SOCKS5 connection")
	}

	return conn, nil
}

// DialSocksProxy returns the dial function to be used in http.Transport object.
// Argument socksType should be one of SOCKS4, SOCKS4A and SOCKS5.
// Argument proxy should be in this format "127.0.0.1:1080".
func DialSocksProxy(socksType int, proxy string, username string, password string) func(string, string) (net.Conn, error) {
	if username != "" {
		return (&config{Proto: socksType, Host: proxy, Auth: &auth{username, password}}).dialFunc()
	} else {
		return (&config{Proto: socksType, Host: proxy}).dialFunc()
	}

}

func (c *config) dialFunc() func(string, string) (net.Conn, error) {
	switch c.Proto {
	case SOCKS5:
		return func(_, targetAddr string) (conn net.Conn, err error) {
			return c.dialSocks5(targetAddr)
		}
	case SOCKS4, SOCKS4A:
		return func(_, targetAddr string) (conn net.Conn, err error) {
			return c.dialSocks4(targetAddr)
		}
	}
	return dialError(fmt.Errorf("unknown SOCKS protocol %v", c.Proto))
}

func dialError(err error) func(string, string) (net.Conn, error) {
	return func(_, _ string) (net.Conn, error) {
		return nil, err
	}
}

func (cfg *config) dialSocks4(targetAddr string) (_ net.Conn, err error) {
	socksType := cfg.Proto
	proxy := cfg.Host

	// dial TCP
	conn, err := DialTimeoutWithoutProxy(cfg.Timeout, "tcp", proxy)
	if err != nil {
		return nil, err
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
	} else if len(resp) != 8 {
		return nil, errors.New("server does not respond properly")
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
