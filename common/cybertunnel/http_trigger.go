package cybertunnel

import (
	"bufio"
	"crypto/tls"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"io"
	"net"
	"sync"
)

type HTTPTrigger struct {
	httpListener net.Listener
	tlsListener  net.Listener
}

var defaultHTTPTrigger *HTTPTrigger

func NewHTTPTrigger() (*HTTPTrigger, error) {
	trigger := &HTTPTrigger{}
	return trigger, nil
}

func (t *HTTPTrigger) serveRequest(isTls bool, req []byte, conn io.WriteCloser) error {
	spew.Dump(req)
	return nil
}

func (t *HTTPTrigger) Serve() error {

	var httpErr, tlsErr error
	t.httpListener, httpErr = net.Listen("tcp", "0.0.0.0:80")
	t.tlsListener, tlsErr = net.Listen("tcp", "0.0.0.0:443")
	defer func() {
		if t.httpListener != nil {
			t.httpListener.Close()
		}
		if t.tlsListener != nil {
			t.tlsListener.Close()
		}
	}()
	errMsg := ""
	if httpErr != nil {
		errMsg = utils.Wrap(httpErr, "create http listener failed\n").Error()
	}
	if tlsErr != nil {
		errMsg = utils.Wrap(tlsErr, "create tls listener failed\n").Error()
	}

	if errMsg != "" {
		return utils.Error(errMsg)
	}

	caPem, caPrivateKey, err := tlsutils.GenerateSelfSignedCertKeyWithCommonName("Yak Bridge Service", "yaklang.io", nil, nil)
	if err != nil {
		return err
	}
	caCert, _, err := tlsutils.ParseCertAndPriKeyAndPool(caPem, caPrivateKey)
	if err != nil {
		return utils.Wrap(err, "parse cert and private key failed")
	}

	cert, err := tlsutils.ParsePEMCert(caPem)
	if err != nil {
		return utils.Wrap(err, "parse cert failed")
	}
	mitmConfig, err := mitm.NewConfig(cert, caCert.PrivateKey)
	if err != nil {
		return utils.Errorf("create mitm config failed: %s", err)
	}

	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()

		for {
			conn, err := t.httpListener.Accept()
			if err != nil {
				log.Errorf("accept http connection failed: %s", err)
				return
			}
			go func() {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("panic: %s", err)
					}
				}()

				defer conn.Close()
				cr := bufio.NewReader(conn)
				req, err := utils.ReadHTTPRequestFromBufioReader(cr)
				if err != nil {
					log.Errorf("read http request failed: %s", err)
					return
				}
				reqRaw := httpctx.GetRequestBytes(req)
				err = t.serveRequest(false, reqRaw, conn)
				if err != nil {
					log.Warnf("serve http request failed: %s", err)
				}
			}()
		}
	}()

	go func() {
		defer wg.Done()
		mitmConfig.SkipTLSVerify(true)

		for {
			conn, err := t.tlsListener.Accept()
			if err != nil {
				log.Errorf("accept tls connection failed: %s", err)
				return
			}
			go func() {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("panic: %s", err)
					}
				}()

				conn = tls.Server(conn, mitmConfig.TLS())

				defer conn.Close()
				cr := bufio.NewReader(conn)
				req, err := utils.ReadHTTPRequestFromBufioReader(cr)
				if err != nil {
					log.Warnf("read http request failed: %s", err)
					return
				}
				reqRaw := httpctx.GetRequestBytes(req)
				err = t.serveRequest(true, reqRaw, conn)
				if err != nil {
					log.Warnf("serve http request failed: %s", err)
				}
			}()
		}
	}()

	wg.Wait()
	return nil
}
