package tlsutils

import (
	"crypto/tls"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"testing"
	"time"
)

func TestP12Auth(t *testing.T) {
	ca, key, err := GenerateSelfSignedCertKey("", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sCert, sKey, err := SignServerCrtNKeyEx(ca, key, "", true)
	if err != nil {
		t.Fatal(err)
	}
	sConfig, err := GetX509ServerTlsConfigWithAuth(ca, sCert, sKey, true)
	if err != nil {
		t.Fatal(err)
	}

	cCert, cKey, err := SignClientCrtNKeyEx(ca, key, "", true)
	if err != nil {
		t.Fatal(err)
	}

	port := utils.GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", utils.HostPort("127.0.0.1", port))
	if err != nil {
		t.Fatal(err)
	}

	token := utils.RandStringBytes(20)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			tlsConn := tls.Server(conn, sConfig)
			err = tlsConn.Handshake()
			if err != nil {
				log.Errorf("handshake to client failed: %s", err)
				continue
			}
			tlsConn.Write([]byte(token))
			tlsConn.Close()
		}
	}()
	time.Sleep(time.Second)
	clientConfig, err := GetX509MutualAuthGoClientTlsConfig(cCert, cKey, ca)
	if err != nil {
		t.Fatal()
	}
	conn, err := tls.Dial("tcp", utils.HostPort("127.0.0.1", port), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	var buf = make([]byte, 20)
	conn.Read(buf)
	if string(buf) != token {
		t.Fatal("token not match")
	}
	conn.Close()

	p12bytes, err := BuildP12(cCert, cKey, "", ca)
	if err != nil {
		t.Fatal(err)
	}
	cCert, cKey, _, err = LoadP12ToPEM(p12bytes, "")
	if err != nil {
		t.Fatal(err)
	}
	clientConfig, err = GetX509MutualAuthGoClientTlsConfig(cCert, cKey, ca)
	if err != nil {
		t.Fatal()
	}
	conn, err = tls.Dial("tcp", utils.HostPort("127.0.0.1", port), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	conn.Read(buf)
	if string(buf) != token {
		t.Fatal("token not match")
	}
	conn.Close()

	cCert2, cKey2, err := SignClientCrtNKeyEx(ca, key, "", false)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(cCert2, cKey2)
	if err != nil {
		t.Fatal(err)
	}
	clientConfig.Certificates = append(clientConfig.Certificates, cert)
	conn, err = tls.Dial("tcp", utils.HostPort("127.0.0.1", port), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	conn.Read(buf)
	if string(buf) != token {
		t.Fatal("token not match")
	}
	conn.Close()

	conn, err = tls.Dial("tcp", utils.HostPort("127.0.0.1", port), utils.NewDefaultTLSConfig())
	if err != nil {
		return
	}
	err = conn.Handshake()
	if err != nil {
		return
	}
	buf = make([]byte, 20)
	conn.Read(buf)
	if string(buf) == token {
		t.Fatal("token not match")
	}
	conn.Close()
}

func TestP12OrPFX(t *testing.T) {
	ca, key, err := GenerateSelfSignedCertKey("127.0.0.1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cert, sKey, err := SignServerCrtNKeyEx(ca, key, "", false)
	if err != nil {
		t.Fatal(err)
	}
	p12Bytes, err := BuildP12(cert, sKey, "123456", ca)
	if err != nil {
		t.Fatal(err)
	}
	certBytes, keyBytes, cas, err := LoadP12ToPEM(p12Bytes, "123456")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(certBytes, keyBytes, cas)

	p12Bytes, err = BuildP12(cert, sKey, "", ca)
	if err != nil {
		t.Fatal(err)
	}
	certBytes, keyBytes, cas, err = LoadP12ToPEM(p12Bytes, "")
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(certBytes, keyBytes, cas)
}
