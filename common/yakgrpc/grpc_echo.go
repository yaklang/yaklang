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
)

func (s *Server) Echo(ctx context.Context, req *ypb.EchoRequest) (*ypb.EchoResposne, error) {
	return &ypb.EchoResposne{Result: req.GetText()}, nil
}

func (s *Server) VerifySystemCertificate(ctx context.Context, _ *ypb.Empty) (*ypb.VerifySystemCertificateResponse, error) {

	//return verifySystemCertificateByURL()
	return verifySystemCertificate()
}

func verifySystemCertificateByURL() (*ypb.VerifySystemCertificateResponse, error) {
	crep.InitMITMCert()
	caCert, caKey, _ := crep.GetDefaultCaAndKey()
	port := utils.GetRandomAvailableTCPPort()

	cert, err := tls.X509KeyPair(caCert, caKey)
	if err != nil {
		return nil, err
	}

	log.Infof("Starting HTTPS server on port %d", port)
	// 创建 HTTPS 服务器
	server := &http.Server{
		Addr: fmt.Sprintf("127.0.0.1:%d", port), // HTTPS 默认端口
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
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

	tlsConfig := &tls.Config{
		ServerName: "mitmserver",
		MinVersion: tls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion: tls.VersionTLS13,
	}
	conn, err := netx.DialX(fmt.Sprintf("127.0.0.1:%d", port),
		netx.DialX_WithTLS(true),
		netx.DialX_WithTLSConfig(tlsConfig),
		netx.DialX_WithTimeout(3),
	)

	if err != nil {
		return &ypb.VerifySystemCertificateResponse{Valid: false, Reason: err.Error()}, nil
	}
	defer conn.Close()

	return &ypb.VerifySystemCertificateResponse{Valid: true}, nil
}

func verifySystemCertificate() (*ypb.VerifySystemCertificateResponse, error) {
	err := crep.VerifySystemCertificate()
	if err != nil {
		return &ypb.VerifySystemCertificateResponse{Valid: false, Reason: err.Error()}, nil
	}
	return &ypb.VerifySystemCertificateResponse{Valid: true}, nil
}
