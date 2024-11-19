package yakgrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"time"
)

func (s *Server) Echo(ctx context.Context, req *ypb.EchoRequest) (*ypb.EchoResposne, error) {
	return &ypb.EchoResposne{Result: req.GetText()}, nil
}

var verifyFunction = verifySystemCertificate

var VerifySystemCertificateCD = utils.NewCoolDown(10 * time.Second)
var resp *ypb.VerifySystemCertificateResponse

func (s *Server) VerifySystemCertificate(ctx context.Context, _ *ypb.Empty) (*ypb.VerifySystemCertificateResponse, error) {
	var err error
	VerifySystemCertificateCD.DoOr(func() {
		resp = nil
		resp, err = verifyFunction()
	}, func() {
		_ = utils.Spinlock(10, func() bool {
			// 拿到结果，解除自旋
			return resp != nil
		})
	})
	if resp == nil {
		return &ypb.VerifySystemCertificateResponse{Valid: false, Reason: "Timeout"}, nil
	}
	//return verifySystemCertificateByURL()
	return resp, err
}

func verifySystemCertificateByURL() (*ypb.VerifySystemCertificateResponse, error) {
	err := verify(nil, nil, "www.example.com")
	if err != nil {
		return &ypb.VerifySystemCertificateResponse{Valid: false, Reason: err.Error()}, nil
	}
	return &ypb.VerifySystemCertificateResponse{Valid: true}, nil
}

func verify(serConfig, cliConfig *tls.Config, domain string) error {
	crep.InitMITMCert()
	caCert, caKey, _ := crep.GetDefaultMITMCAAndPriv()
	fakeCert, err := crep.FakeCertificateByHost(caCert, caKey, domain)
	if err != nil {
		return err
	}
	port := utils.GetRandomAvailableTCPPort()

	log.Infof("Starting HTTPS server on port %d", port)

	if serConfig == nil {
		serConfig = &tls.Config{
			Certificates: []tls.Certificate{fakeCert},
		}
	}
	server := &http.Server{
		Addr:      fmt.Sprintf("127.0.0.1:%d", port), // HTTPS 默认端口
		TLSConfig: serConfig,
	}

	serverReady := make(chan struct{})

	go func() {
		close(serverReady)
		err := server.ListenAndServeTLS("", "")
		if err != nil {
			log.Errorf("server.ListenAndServeTLS() failed: %v", err)
		}
	}()

	<-serverReady

	defer server.Shutdown(context.Background())

	if cliConfig == nil {
		cliConfig = &tls.Config{
			ServerName: domain,
		}
	}
	conn, err := netx.DialX(fmt.Sprintf("127.0.0.1:%d", port),
		netx.DialX_WithTimeout(3*time.Second),
	)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(conn, cliConfig)
	err = tlsConn.Handshake()
	if err != nil {
		return err
	}
	defer tlsConn.Close()

	return nil
}

func verifySystemCertificate() (*ypb.VerifySystemCertificateResponse, error) {
	err := crep.VerifySystemCertificate()
	if err != nil {
		return &ypb.VerifySystemCertificateResponse{Valid: false, Reason: err.Error()}, nil
	}
	return &ypb.VerifySystemCertificateResponse{Valid: true}, nil
}
