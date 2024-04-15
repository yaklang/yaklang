package yakgrpc

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) Echo(ctx context.Context, req *ypb.EchoRequest) (*ypb.EchoResposne, error) {
	return &ypb.EchoResposne{Result: req.GetText()}, nil
}

func (s *Server) VerifySystemCertificate(ctx context.Context, req *ypb.VerifySystemCertificateRequest) (*ypb.VerifySystemCertificateResponse, error) {

	//return verifySystemCertificateByURL(req.GetUrl())
	return verifySystemCertificate()
}

func verifySystemCertificateByURL(u string) (*ypb.VerifySystemCertificateResponse, error) {
	testUrl := "https://www.example.com"
	if u != "" {
		testUrl = u
		u, err := url.Parse(testUrl)
		if err != nil {
			return nil, utils.Wrap(err, "failed to parse url")
		}
		if u.Scheme != "https" {
			return nil, utils.Error("only support https url")
		}
	}

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(20))
	defer cancel()
	client, err := NewLocalClient()

	stream, err := client.MITM(ctx)
	if err != nil {
		return nil, utils.Wrap(err, "failed to create MITM stream")
	}
	mitmPort := utils.GetRandomAvailableTCPPort()

	host, port := "127.0.0.1", mitmPort
	stream.Send(&ypb.MITMRequest{
		Host: host,
		Port: uint32(port),
	})

	request := func() error {
		proxyURL, err := url.Parse(fmt.Sprintf("http://%s:%d", host, port))
		if err != nil {
			return err
		}
		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}
		resp, err := client.Get(testUrl)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return nil
	}

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp.GetHaveMessage() {
			msg := rsp.GetMessage().GetMessage()
			if strings.Contains(string(msg), `starting mitm server`) {
				err := request()
				if err != nil {
					return &ypb.VerifySystemCertificateResponse{
						Valid: false, Reason: err.Error(),
					}, nil
				}
				return &ypb.VerifySystemCertificateResponse{Valid: true}, nil
			}
		}
	}
	return &ypb.VerifySystemCertificateResponse{Valid: false}, nil
}

func verifySystemCertificate() (*ypb.VerifySystemCertificateResponse, error) {
	crep.InitMITMCert()
	certPEM, _, err := crep.GetDefaultCaAndKey()
	if err != nil {
		return nil, err
	}

	// 解码 PEM 格式的证书
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, err
	}

	// 解析证书
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	// 创建系统根证书池
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}

	// 验证证书是否在系统根证书池中
	opts := x509.VerifyOptions{
		Roots: pool,
	}
	_, err = cert.Verify(opts)
	if err == nil {
		return &ypb.VerifySystemCertificateResponse{Valid: true}, nil
	}

	return &ypb.VerifySystemCertificateResponse{Valid: false, Reason: err.Error()}, nil
}
