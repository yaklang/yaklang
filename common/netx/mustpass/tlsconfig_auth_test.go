package mustpass

import (
	"crypto/tls"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

func TestTLSConfigAuth(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Errorf("dial " + addr + " failed: " + err.Error())
		t.FailNow()
	}
	defer lis.Close()
	token := utils.RandStringBytes(40)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			tlsConn := tlsutils.NewDefaultTLSServer(conn)
			if err != nil {
				log.Error(err)
				conn.Close()
				continue
			}
			tlsConn.Write([]byte(token))
			conn.Close()
		}
	}()
	time.Sleep(time.Second)
	conn, err := netx.DialX(addr, netx.DialX_WithTLS(true))
	if err != nil {
		t.Fatal(err)
	}
	var buf = make([]byte, 40)
	conn.Read(buf)
	if string(buf) != token {
		t.Fatal("token not match")
	}
}

func TestTLSConfigAuth2_WithoutAuth(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Errorf("dial " + addr + " failed: " + err.Error())
		t.FailNow()
	}
	defer lis.Close()
	token := utils.RandStringBytes(40)

	ca, cert, err := tlsutils.GenerateSelfSignedCertKey("", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sCert, sKey, err := tlsutils.SignServerCrtNKeyEx(ca, cert, "", false)
	if err != nil {
		t.Fatal(err)
	}
	config, err := tlsutils.GetX509ServerTlsConfig(ca, sCert, sKey)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			tlsConn := tls.Server(conn, config)
			if err != nil {
				log.Error(err)
				conn.Close()
				continue
			}
			err = tlsConn.Handshake()
			if err != nil {
				log.Error(err)
				conn.Close()
				continue
			}
			tlsConn.Write([]byte(token))
			conn.Close()
		}
	}()
	time.Sleep(time.Second)
	conn, err := netx.DialX(addr, netx.DialX_WithTLS(true))
	if err != nil {
		t.Fatal(err)
	}
	var buf = make([]byte, 40)
	conn.Read(buf)
	if string(buf) != token {
		t.Fatal("token not match")
	}
}

func TestTLSConfigAuth2(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	addr := utils.HostPort("127.0.0.1", port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Errorf("dial " + addr + " failed: " + err.Error())
		t.FailNow()
	}
	defer lis.Close()
	token := utils.RandStringBytes(40)

	ca, cert, err := tlsutils.GenerateSelfSignedCertKey("", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sCert, sKey, err := tlsutils.SignServerCrtNKeyEx(ca, cert, "", true)
	if err != nil {
		t.Fatal(err)
	}
	config, err := tlsutils.GetX509MutualAuthServerTlsConfig(ca, sCert, sKey)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			log.Infof("recv from: %v start serve tls", conn.RemoteAddr())
			tlsConn := tls.Server(conn, config)
			if err != nil {
				log.Error(err)
				log.Infof("close %v", conn.RemoteAddr())
				conn.Close()
				continue
			}
			log.Infof("recv from: %v start to handshake", conn.RemoteAddr())
			err = tlsConn.HandshakeContext(utils.TimeoutContext(10 * time.Second))
			if err != nil {
				if err != io.EOF {
					log.Error(err)
				}
				log.Infof("close %v", conn.RemoteAddr())
				conn.Close()
				continue
			}
			tlsConn.Write([]byte(token))
			conn.Close()
			log.Infof("close %v", conn.RemoteAddr())
		}
	}()
	time.Sleep(time.Second)
	conn, err := netx.DialX(addr, netx.DialX_WithTLS(true))
	if err != nil {
		t.Fatal("cannot connect without cert")
	}

	tlsConn := tls.Client(conn, &tls.Config{
		//Renegotiation:      tls.RenegotiateFreelyAsClient,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionSSL30,
		MaxVersion:         tls.VersionTLS13,
		ServerName:         "127.0.0.1",
	})

	err = tlsConn.Handshake()
	if !strings.Contains(err.Error(), "tls: bad certificate") && !strings.Contains(err.Error(), "tls: certificate required") && !strings.Contains(err.Error(), "tls: alert(116)") {
		t.Fatalf("cannot connect without cert:%v", err)
	}

	tokenSecret := utils.RandStringBytes(10)
	cCert, cKey, err := tlsutils.SignClientCrtNKeyEx(ca, cert, "", true)
	if err != nil {
		t.Fatal(err)
	}
	p12Bytes, _ := tlsutils.BuildP12(cCert, cKey, tokenSecret, ca)
	err = netx.LoadP12Bytes(p12Bytes, tokenSecret, "")
	if err != nil {
		t.Fatal(err)
	}

	log.Info("start to dial right config")
	conn, err = netx.DialX(addr, netx.DialX_WithTLS(true), netx.DialX_WithTLSTimeout(100*time.Second), netx.DialX_Debug(true))
	if err != nil {
		t.Fatal(err)
	}
	var buf = make([]byte, 40)
	_, err = conn.Read(buf)
	require.NoError(t, err)
	if string(buf) != token {
		t.Fatal("token not match")
	}
}

func TestDialxGm(t *testing.T) {
	t.Skip("skip")
	addr := "180.163.248.139:443"
	conn, err := netx.DialX(addr, netx.DialX_WithTLS(true), netx.DialX_WithGMTLSSupport(true), netx.DialX_WithGMTLSOnly(true), netx.DialX_WithSNI("sm2test.ovssl.cn"), netx.DialX_WithTLSTimeout(100*time.Second), netx.DialX_Debug(true))
	require.NoError(t, err)
	spew.Dump(conn)
}
