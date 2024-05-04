package twofa

import (
	"bufio"
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strings"
)

type OTPServer struct {
	config              *OTPConfig
	localPort           int
	to                  string
	enableSelfSignedTLS bool
}

func (o *OTPServer) SetLocalPort(port int) {
	o.localPort = port
}

func (o *OTPServer) SetForwardTo(to string) {
	o.to = to
}

func (o *OTPServer) handle(conn net.Conn) error {
	br := bufio.NewReader(conn)
	req, err := utils.ReadHTTPRequestFromBufioReader(br)
	if err != nil {
		return err
	}
	raw := httpctx.GetRequestBytes(req)
	results := lowhttp.GetHTTPPacketHeader(raw, "Y-T-Verify-Code")
	pathStr := lowhttp.GetHTTPRequestPath(raw)
	log.Infof("request path: %#v need to verify code: %#v", pathStr, results)
	if codec.Atoi(results) != o.config.GetToptUTCCode() {

		return utils.Error("y-t-verify-code not match")
	}
	isHttps := strings.HasPrefix(strings.ToLower(o.to), "https://")
	raw = lowhttp.DeleteHTTPPacketHeader(raw, "Y-T-Verify-Code")
	host, port, err := utils.ParseStringToHostPort(o.to)
	if err != nil {
		return err
	}
	var requestHost string
	if isHttps && port == 443 {
		requestHost = host
	} else if !isHttps && port == 80 {
		requestHost = host
	} else {
		requestHost = utils.HostPort(host, port)
	}
	raw = lowhttp.ReplaceHTTPPacketHeader(raw, "Host", requestHost)
	rsp, _, err := poc.HTTP(raw, poc.WithForceHTTPS(isHttps), poc.WithHost(host), poc.WithPort(port))
	if err != nil {
		return err
	}
	_, err = conn.Write(rsp)
	return nil
}

func (o *OTPServer) Serve() error {
	return o.serveContext(context.Background())
}

func (o *OTPServer) ServeContext(ctx context.Context) error {
	return o.serveContext(ctx)
}

func (o *OTPServer) serveContext(ctx context.Context) (retErr error) {
	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
			retErr = utils.Wrap(utils.Error(err), "panic ... in otp server ...")
		}
	}()

	log.Infof("start to listen on: [::]:%v for %v", o.localPort, o.to)
	lis, err := net.Listen("tcp", utils.HostPort("0.0.0.0", o.localPort))
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		lis.Close()
	}()

	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Errorf("failed to accept connection: %v", err)
			break
		}
		log.Infof("recv tcp from: %v", conn.RemoteAddr().String())
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("panic: %v", err)
				}
			}()

			defer func() {
				conn.Close()
			}()
			log.Infof("start to serve: %v for checking totp token", conn.RemoteAddr().String())
			err := o.handle(conn)
			if err != nil {
				log.Errorf("failed to handle connection: %v", err)
			}
		}()
	}
	return nil
}

func NewOTPServer(secret string, localPort int, forwardTo string) *OTPServer {
	return &OTPServer{
		config:    NewTOTPConfig(secret),
		localPort: localPort,
		to:        forwardTo,
	}
}
