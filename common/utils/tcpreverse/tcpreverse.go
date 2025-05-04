package tcpreverse

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/cybertunnel/ctxio"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

type TCPReverseTarget struct {
	ForceTLS bool
	Address  string
}

type TCPReverse struct {
	port      int
	tlsConfig *tls.Config
	rwm       sync.RWMutex
	target    map[string]*TCPReverseTarget
}

func NewTCPReverse(port int) (*TCPReverse, error) {
	log.Infof("Creating new TCP reverse proxy on port %d", port)

	ca, key, err := tlsutils.GenerateSelfSignedCertKey(strings.ToLower(utils.RandStringBytes(12)+".com"), nil, nil)
	if err != nil {
		return nil, err
	}

	crt, spriv, err := tlsutils.SignServerCrtNKey(ca, key)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := tlsutils.GetX509ServerTlsConfig(ca, crt, spriv)
	if err != nil {
		return nil, err
	}
	crts := tlsConfig.Certificates
	if len(crts) == 0 {
		return nil, fmt.Errorf("no certs in tls config(BUG)")
	}
	tlsConfig.GetCertificate = func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &crts[0], nil
	}

	return &TCPReverse{
		port:      port,
		tlsConfig: tlsConfig,
		rwm:       sync.RWMutex{},
		target:    make(map[string]*TCPReverseTarget),
	}, nil
}

func (t *TCPReverse) RegisterSNIForward(sni string, target *TCPReverseTarget) {
	t.rwm.Lock()
	defer t.rwm.Unlock()
	if t.target == nil {
		t.target = make(map[string]*TCPReverseTarget)
	}
	log.Infof("Registering SNI forward: %s -> %s (TLS: %v)", sni, target.Address, target.ForceTLS)
	t.target[sni] = target
}

func (t *TCPReverse) Run() error {
	log.Infof("Starting TCP reverse proxy on port %d", t.port)
	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", t.port))
	if err != nil {
		log.Errorf("Failed to listen on port %d: %v", t.port, err)
		return fmt.Errorf("listen error: %w", err)
	}
	log.Infof("TCP reverse proxy listening on 0.0.0.0:%d", t.port)
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Errorf("Failed to accept connection: %v", err)
			return fmt.Errorf("accept error: %w", err)
		}
		log.Debugf("Accepted new connection from %s", conn.RemoteAddr())
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("close conn error: %v", utils.ErrorStack(err))
				}
			}()
			defer conn.Close()
			err := t.servePlain(conn)
			if err != nil {
				log.Errorf("serve plain error: %v", err)
			}
		}()
	}
}

func (t *TCPReverse) servePlain(conn net.Conn) error {
	var err error
	var sni string
	log.Debugf("Setting up TLS server for connection from %s", conn.RemoteAddr())
	//conn = tls.Server(conn, t.tlsConfig)
	tlsConn := tls.Server(conn, t.tlsConfig)
	err = tlsConn.Handshake()
	if err != nil {
		return err
	}
	state := tlsConn.ConnectionState()
	sni = state.ServerName
	conn = tlsConn
	log.Infof("Detected SNI: %#v", sni)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var targetConn net.Conn

	t.rwm.RLock()
	target, ok := t.target[sni]
	t.rwm.RUnlock()

	var requestHost string
	enableTls := target.ForceTLS
	host, port, err := utils.ParseStringToHostPort(target.Address)
	if err != nil {
		host = target.Address
		if enableTls {
			port = 443
		} else {
			port = 80
		}
		target.Address = utils.HostPort(host, port)
	}
	if enableTls && port == 443 {
		requestHost = host
	} else {
		requestHost = target.Address
	}

	if ok {
		log.Infof("Forwarding connection with SNI %s to %s (TLS: %v)", sni, target.Address, enableTls)
		targetConn, err = netx.DialX(
			target.Address, netx.DialX_WithTLS(enableTls), netx.DialX_WithTimeout(10*time.Second),
		)
		if err != nil {
			log.Errorf("Failed to connect to target %s: %v", target.Address, err)
			return err
		}
		log.Debugf("Successfully connected to target %s", target.Address)
	} else {
		if len(t.target) == 1 {
			var targetIns *TCPReverseTarget
			t.rwm.Lock()
			for _, v := range t.target {
				targetIns = v
			}
			t.rwm.Unlock()
			if targetIns == nil {
				log.Errorf("No target found for SNI %s, or no targets registered", sni)
				return utils.Errorf("no target found for sni %s, or not set target yet", sni)
			}
			log.Infof("Using default target %s (TLS: %v) as only one target is registered", targetIns.Address, targetIns.ForceTLS)
			targetConn, err = netx.DialX(targetIns.Address, netx.DialX_WithTLS(targetIns.ForceTLS), netx.DialX_WithTimeout(10*time.Second))
			if err != nil {
				log.Errorf("Failed to connect to default target %s: %v", targetIns.Address, err)
				return err
			}
			log.Debugf("Successfully connected to default target %s", targetIns.Address)
		} else {
			log.Errorf("No target found for SNI %s and multiple targets are registered", sni)
			return utils.Errorf("no target found for sni %s", sni)
		}
	}

	if targetConn == nil {
		log.Errorf("Target connection is nil, this should not happen")
		return utils.Errorf("target conn is nil")
	}

	log.Infof("Starting bidirectional proxy between %s and %s", conn.RemoteAddr(), targetConn.RemoteAddr())
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer cancel()
		defer targetConn.Close()

		for {
			reader := ctxio.NewReader(ctx, conn)
			request, err := utils.ReadHTTPRequestFromBufioReader(bufio.NewReader(reader))
			if err != nil {
				log.Debugf("Error reading request from client: %v", err)
				return
			}
			raw, err := utils.DumpHTTPRequest(request, true)
			if err != nil {
				log.Debugf("Error dumping request: %v", err)
				return
			}
			raw = lowhttp.ReplaceHTTPPacketHeader(raw, "Host", requestHost)
			_, err = targetConn.Write(raw)
			if err != nil {
				log.Warnf("Error writing request: %v", err)
				return
			}
		}
		//bytes, err := io.Copy(targetConn, io.TeeReader(ctxio.NewReader(ctx, conn), os.Stdout))
		//if err != nil {
		//	log.Debugf("Error copying data from client to target: %v", err)
		//}
		//log.Debugf("Copied %d bytes from client to target", bytes)
	}()
	go func() {
		defer wg.Done()
		defer cancel()
		defer conn.Close()
		bytes, err := io.Copy(conn, ctxio.NewReader(ctx, targetConn))
		//bytes, err := io.Copy(conn, io.TeeReader(ctxio.NewReader(ctx, targetConn), os.Stdout))
		if err != nil {
			log.Debugf("Error copying data from target to client: %v", err)
		}
		log.Debugf("Copied %d bytes from target to client", bytes)
	}()
	wg.Wait()
	log.Infof("Connection between %s and %s closed", conn.RemoteAddr(), targetConn.RemoteAddr())
	return nil
}
