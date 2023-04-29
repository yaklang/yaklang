package tlsutils

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"yaklang/common/log"
	"sync"
	"time"
)

import (
	"net"
	"testing"
)

func TestRevokeCert(t *testing.T) {
	log.Infof("generating ca/key")
	crt, key, err := GenerateSelfSignedCertKey("127.0.0.1", []net.IP{}, []string{})
	if err != nil {
		log.Errorf("signed cert error: %s", err)
		t.Fail()
	}

	log.Infof("CACERT: \n%v", string(crt))

	serverCert, serverKey, err := SignServerCrtNKey(crt, key)
	if err != nil {
		log.Errorf("sign server crt error: %s", err)
		t.Log(err)
		t.Fail()
		return
	}

	log.Infof("ServerCert: \n%v", string(serverCert))
	log.Infof("ServerKey: \n%v", string(serverKey))

	log.Infof("sign-ing client key")
	clientCert, clientKey, err := SignClientCrtNKey(crt, key)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}

	log.Infof("Client1Cert: \n%v", string(clientCert))
	log.Infof("Client1Key: \n%v", string(clientKey))

	clientCert2, clientKey2, err := SignClientCrtNKey(crt, key)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	log.Infof("Client2Cert: \n%v", string(clientCert2))
	log.Infof("Client2Key: \n%v", string(clientKey2))

	config, err := GetX509MutualAuthServerTlsConfig(crt, serverCert, serverKey)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	_ = config
	_ = clientKey

	// 检测生成的 CRL
	crl, err := GenerateCRL(crt, key, clientCert)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	log.Infof("CRL for Client1: \n%v", string(crl))
	l, err := ParsePEMCRLRaw(crl)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}

	cert, err := ParsePEMCert(clientCert)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	cert2, err := ParsePEMCert(clientCert2)
	if err != nil {
		t.Log(err)
		t.FailNow()
		return
	}

	caCert, err := ParsePEMCert(crt)
	if err != nil {
		t.Log(err)
		t.FailNow()
		return
	}

	_ = cert
	_ = cert2
	err = caCert.CheckCRLSignature(l)
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	for _, rCrt := range l.TBSCertList.RevokedCertificates {
		if cert.SerialNumber.String() != rCrt.SerialNumber.String() {
			t.Log("failed for sid: ", rCrt.SerialNumber.String())
			t.FailNow()
		} else {
			t.Log("cert 1 verify finished")
		}

		if cert2.SerialNumber.String() == rCrt.SerialNumber.String() {
			t.Log("failed for sid(2): ", rCrt.SerialNumber.String())
			t.FailNow()
		} else {
			t.Log("cert 2 verify finished")
		}
	}

	// 验证 TLS CERT CRL
	//roots := x509.NewCertPool()
	//roots.AppendCertsFromPEM(crt)
	//cfg, err := GetX509MutualAuthServerTlsConfig(crt, serverCert, serverKey)
	//if err != nil {
	//	t.Log(err)
	//	t.FailNow()
	//	return
	//}
	//cfg.GetClientCertificate
	//var lis, err = tls.Listen(
	//	"tcp", utils.HostPort("127.0.0.1", utils.GetRandomAvailableTCPPort()),
	//	cfg,
	//)
	//if err != nil {
	//	t.Log(err)
	//	t.FailNow()
	//	return
	//}

	//err = cert.CheckCRLSignature(l)
	//if err != nil {
	//	t.Log(err)
	//	t.FailNow()
	//	return
	//}
	//
	//err = cert2.CheckCRLSignature(l)
	//if err != nil {
	//	t.Log(err)
	//	t.FailNow()
	//	return
	//}
}

func TestGenerateSelfSignedCertKey(t *testing.T) {
	crt, key, err := GenerateSelfSignedCertKey("127.0.0.1", []net.IP{}, []string{})
	if err != nil {
		log.Errorf("signed cert error: %s", err)
		t.Fail()
	}

	log.Info("generate ca success")

	serverCert, serverKey, err := SignServerCrtNKey(crt, key)
	if err != nil {
		log.Errorf("sign server crt error: %s", err)
		t.Log(err)
		t.Fail()
		return
	}

	odinaryCert, odKey, err := SignServerCrtNKeyWithParams(
		crt, key, "baidu.com", time.Now().Add(24*365*10*time.Hour), false,
	)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}
	log.Infof("\nODINARY CERT/KEY\n%v\n%v", string(odinaryCert), string(odKey))

	clientCert, clientKey, err := SignClientCrtNKey(crt, key)
	if err != nil {
		t.Log(err)
		t.Fail()
		return
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	serverRun := func() {
		config, err := GetX509MutualAuthServerTlsConfig(crt, serverCert, serverKey)
		sock, err := tls.Listen("tcp", "127.0.0.1:11112", config)
		if err != nil {
			log.Errorf("cannot listened: %s", err)
		}

		wg.Done()

		conn, err := sock.Accept()
		if err != nil {
			log.Errorf("accept error: %s", err)
			t.Fail()
		}

		//switch c := conn.(type) {
		//case *tls.Conn:
		//	c.
		//}

		_, _ = conn.Write([]byte("hello\n"))
		_ = conn.Close()
	}

	clientRun := func() {
		//pool := x509.NewCertPool()
		//if !pool.AppendCertsFromPEM(crt) {
		//	t.Fail()
		//}

		config, err := GetX509MutualAuthClientTlsConfig(clientCert, clientKey)
		if err != nil {
			log.Errorf("build client tls config error: %s", err)
			t.Fail()
			return
		}

		conn, err := tls.Dial("tcp", "127.0.0.1:11112", config)
		if err != nil {
			log.Error(err)
			t.Fail()
		}

		reader := bufio.NewReader(conn)
		line, _, _ := reader.ReadLine()
		if !bytes.HasPrefix(line, []byte("hello")) {
			t.Fail()
		}
	}

	go serverRun()

	wg.Wait()

	clientRun()

}
