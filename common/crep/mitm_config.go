package crep

import (
	"bufio"
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	gmx509 "github.com/yaklang/yaklang/common/gmsm/x509"

	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/minimartian/h2"
	"github.com/yaklang/yaklang/common/minimartian/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// request modifier
func NewRequestModifier(f minimartian.RequestModifierFunc) minimartian.RequestModifier {
	return &requestModifierFunc{f: f}
}

type requestModifierFunc struct {
	f minimartian.RequestModifierFunc
}

func (r *requestModifierFunc) ModifyRequest(req *http.Request) error {
	return r.f(req)
}

// response modifier
func NewResponseModifier(f minimartian.ResponseModifierFunc) minimartian.ResponseModifier {
	return &responseModifierFunc{f: f}
}

type responseModifierFunc struct {
	f minimartian.ResponseModifierFunc
}

func (r *responseModifierFunc) ModifyResponse(req *http.Response) error {
	return r.f(req)
}

// config
type MITMConfig func(server *MITMServer) error

func MITM_EnableMITMCACertPage(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.enableMITMCACertPage = b
		return nil
	}
}

func MITM_EnableWebsocketCompression(b bool) MITMConfig {
	return func(server *MITMServer) error {
		if server.enableWebsocketCompression == nil {
			server.enableWebsocketCompression = utils.NewAtomicBool()
		}
		server.enableWebsocketCompression.SetTo(b)
		return nil
	}
}

func MITM_RandomJA3(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.randomJA3 = b
		return nil
	}
}

func MITM_SetHijackedMaxContentLength(i int) MITMConfig {
	return func(server *MITMServer) error {
		server.hijackedMaxContentLength = i
		if i <= 0 {
			server.hijackedMaxContentLength = 10 * 1000 * 1000
		}
		return nil
	}
}

func MITM_MutualTLSClient(crt, key []byte, cas ...[]byte) MITMConfig {
	return func(server *MITMServer) error {
		server.clientCerts = append(server.clientCerts, NewClientCertificationPair(crt, key, cas...))
		return nil
	}
}

// parseGMCertificate 解析国密证书，返回证书和私钥，如果解析失败返回nil
func parseGMCertificate(gmCA, gmKey []byte) (*gmx509.Certificate, interface{}) {
	if gmCA == nil || gmKey == nil {
		return nil, nil
	}

	// 尝试直接解析PEM格式
	gmC, err := gmtls.X509KeyPair(gmCA, gmKey)
	if err == nil {
		return extractGMCertAndKey(gmC)
	}

	// 尝试解析DER格式
	return parseGMCertificateFromDER(gmCA, gmKey)
}

// extractGMCertAndKey 从gmtls.Certificate中提取证书和私钥
func extractGMCertAndKey(gmC gmtls.Certificate) (*gmx509.Certificate, interface{}) {
	gmCert, err := gmx509.ParseCertificate(gmC.Certificate[0])
	if err != nil {
		log.Warnf("parse gmx509 cert failed: %s", err)
		return nil, nil
	}
	return gmCert, gmC.PrivateKey
}

// parseGMCertificateFromDER 从DER格式解析国密证书
func parseGMCertificateFromDER(gmCA, gmKey []byte) (*gmx509.Certificate, interface{}) {
	caDer, err := gmx509.ParseCertificate(gmCA)
	if err != nil {
		log.Warnf("parse GM ca as [der] format failed: %s", err)
		return nil, nil
	}

	keyDer, err := gmx509.ParsePKCS8PrivateKey(gmKey, nil)
	if err != nil {
		log.Warnf("parse GM key as [der] format pkcs8 pkey failed: %s", err)
		return nil, nil
	}

	keyRawBytes, err := gmx509.MarshalSm2PrivateKey(keyDer, nil)
	if err != nil {
		log.Warnf("marshal GM key as [der] format pkey failed: %s", err)
		return nil, nil
	}

	gmCAPem := pem.EncodeToMemory(&pem.Block{Type: `CERTIFICATE`, Bytes: caDer.Raw})
	gmKeyPem := pem.EncodeToMemory(&pem.Block{Type: `PRIVATE KEY`, Bytes: keyRawBytes})

	gmC, err := gmtls.X509KeyPair(gmCAPem, gmKeyPem)
	if err != nil {
		log.Warnf("parse GM ca and privKey (DER) failed: %s", err)
		return nil, nil
	}

	return extractGMCertAndKey(gmC)
}

func MITM_SetCaCertAndPrivKey(ca []byte, key []byte, gmCA []byte, gmKey []byte) MITMConfig {
	return func(server *MITMServer) error {
		if (ca == nil || key == nil) && (defaultCA != nil && defaultKey != nil) {
			return MITM_SetCaCertAndPrivKey(defaultCA, defaultKey, gmCA, gmKey)(server)
		}

		if (gmCA == nil || gmKey == nil) && (defaultGMCA != nil && defaultGMKey != nil) {
			return MITM_SetCaCertAndPrivKey(ca, key, defaultGMCA, defaultGMKey)(server)
		}

		// 解析普通证书
		c, err := tls.X509KeyPair(ca, key)
		if err != nil {
			c, err = parseRegularCertificateFromDER(ca, key)
			if err != nil {
				return err
			}
		}

		// 解析国密证书（允许为nil或解析失败） 这里其实有点问题 这里国密库对证书解析做了兼容性处理 只要公私钥类型一致就没问题 但是应该验证是否为国密证书
		gmCert, gmPrivateKey := parseGMCertificate(gmCA, gmKey)

		// 提取普通证书
		cert, err := x509.ParseCertificate(c.Certificate[0])
		if err != nil {
			return utils.Errorf("extract x509 cert failed: %s", err)
		}

		gmx509FormCert, err := gmx509.ParseCertificate(c.Certificate[0])
		if err != nil {
			return utils.Errorf("extract x509 cert failed: %s", err)
		}

		// 配置mitm选项
		opts := []mitm.ConfigOption{
			mitm.WithObsoleteTLS(gmx509FormCert, gmCert, c.PrivateKey, gmPrivateKey),
		}

		mc, err := mitm.NewConfig(cert, c.PrivateKey, opts...)
		if err != nil {
			return utils.Errorf("build private key failed: %s", err)
		}

		mc.SkipTLSVerify(true)
		mc.SetOrganization("MITMServer")
		mc.SetValidity(time.Hour * 24 * 90)

		// 配置H2支持
		setupH2Config(mc, cert)

		server.caCert = ca
		server.caKey = key
		server.mitmConfig = mc

		return nil
	}
}

// parseRegularCertificateFromDER 从DER格式解析普通证书
func parseRegularCertificateFromDER(ca, key []byte) (tls.Certificate, error) {
	caDer, err := x509.ParseCertificate(ca)
	if err != nil {
		return tls.Certificate{}, utils.Errorf("parse ca as [der] format failed: %s", err)
	}

	caPem := pem.EncodeToMemory(&pem.Block{Type: `CERTIFICATE`, Bytes: caDer.Raw})
	keyPem, err := parseRegularPrivateKeyFromDER(key)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(caPem, keyPem)
}

// parseRegularPrivateKeyFromDER 从DER格式解析普通私钥
func parseRegularPrivateKeyFromDER(key []byte) ([]byte, error) {
	keyDer, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		// 尝试PKCS1格式
		keyDer, err = x509.ParsePKCS1PrivateKey(key)
		if err != nil {
			return nil, utils.Errorf("parse key as [der] format pkcs1/pkcs8 pkey failed: %s", err)
		}
		// PKCS1格式
		return pem.EncodeToMemory(&pem.Block{
			Type:  `RSA PRIVATE KEY`,
			Bytes: x509.MarshalPKCS1PrivateKey(keyDer.(*rsa.PrivateKey)),
		}), nil
	}

	// PKCS8格式
	keyRawBytes, err := x509.MarshalPKCS8PrivateKey(keyDer)
	if err != nil {
		return nil, utils.Errorf("marshal key as [der] format pkcs8 pkey failed: %s", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: `PRIVATE KEY`, Bytes: keyRawBytes}), nil
}

// setupH2Config 配置H2支持
func setupH2Config(mc *mitm.Config, cert *x509.Certificate) {
	defaultH2Config := new(h2.Config)
	if log.GetLevel() == log.DebugLevel {
		defaultH2Config.EnableDebugLogs = true
	}

	certPool, err := x509.SystemCertPool()
	if err != nil {
		log.Fatal("Failed to retrieve system certificates pool")
	}
	certPool.AddCert(cert)

	defaultH2Config.RootCAs = certPool
	defaultH2Config.AllowedHostsFilter = func(_ string) bool { return true }

	mc.SetH2Config(defaultH2Config)
}

func MITM_SetVia(s string) MITMConfig {
	return func(server *MITMServer) error {
		server.via = s
		return nil
	}
}

func MITM_SetXForwarded(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.allowForwarded = b
		return nil
	}
}

type PKCS12Config struct {
	Path     string
	Password string
}

func MITM_SetHTTP2(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.http2 = b
		return nil
	}
}

func MITM_SetGM(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.gmtls = b
		return nil
	}
}

func MITM_SetGMPrefer(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.gmPrefer = b
		return nil
	}
}

func MITM_SetGMOnly(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.gmOnly = b
		return nil
	}
}

func MITM_SetHTTPForceClose(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.forceDisableKeepAlive = b
		return nil
	}
}

func MITM_SetFindProcessName(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.findProcessName = b
		return nil
	}
}

func MITM_MergeOptions(b ...MITMConfig) MITMConfig {
	return func(server *MITMServer) error {
		for _, c := range b {
			err := c(server)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func MITM_SetDownstreamProxy(proxys ...string) MITMConfig {
	return func(server *MITMServer) error {
		server.proxyUrls = make([]*url.URL, 0)
		if len(proxys) == 0 || proxys == nil {
			server.proxyUrls = nil
			return nil
		}
		for _, proxy := range proxys {
			urlRaw, err := url.Parse(proxy)
			if err != nil {
				return utils.Errorf("parse proxy url: %v failed: %s", proxy, err)
			}
			log.Infof("set downstream proxy as %v", urlRaw.String())
			server.proxyUrls = append(server.proxyUrls, urlRaw)
		}
		return nil
	}
}

func MITM_SetHTTPRequestHijackRaw(c func(isHttps bool, reqIns *http.Request, req []byte) []byte) MITMConfig {
	return func(server *MITMServer) error {
		server.requestHijackHandler = c
		return nil
	}
}

func MITM_SetHTTPResponseHijackRaw(c func(isHttps bool, req *http.Request, rspInstance *http.Response, rsp []byte, remoteAddr string) []byte) MITMConfig {
	return func(server *MITMServer) error {
		server.responseHijackHandler = c
		return nil
	}
}

func MITM_ProxyAuth(username string, password string) MITMConfig {
	return func(server *MITMServer) error {
		if username == "" || password == "" {
			return nil
		}
		server.proxyAuth = &ProxyAuth{
			Username: username,
			Password: password,
		}
		return nil
	}
}

func MITM_SetHTTPRequestHijack(c func(isHttps bool, req *http.Request) *http.Request) MITMConfig {
	return func(server *MITMServer) error {
		server.requestHijackHandler = func(isHttps bool, reqOrigin *http.Request, req []byte) []byte {
			reqIns, err := lowhttp.ParseBytesToHttpRequest(req)
			if err != nil {
				log.Errorf("unmarshal requests bytes to http.Request failed: %s", err)
				return req
			}

			hijackedReq := c(isHttps, reqIns)
			hijackedReq, err = utils.FixHTTPRequestForHTTPDoWithHttps(hijackedReq, isHttps)
			if err != nil {
				log.Errorf("fix hijacked req: http.Request failed: %v", err)
				return req
			}
			raw, err := utils.HttpDumpWithBody(hijackedReq, true)
			if err != nil {
				log.Errorf("dump/marshal hijacked http.Request to bytes failed: %s", err)
				return req
			}
			return raw
		}
		return nil
	}
}

func MITM_SetHTTPResponseMirrorInstance(f func(isHttps bool, req, rsp []byte, remoteAddr string, response *http.Response)) MITMConfig {
	return func(server *MITMServer) error {
		server.httpFlowMirror = func(isHttps bool, r *http.Request, rsp *http.Response, startTs int64) {
			f(isHttps, httpctx.GetPlainRequestBytes(r), httpctx.GetPlainResponseBytes(rsp.Request), r.RemoteAddr, rsp)
		}
		return nil
	}
}

func MITM_SetHTTPResponseMirror(f func(bool, string, *http.Request, *http.Response, string)) MITMConfig {
	return MITM_SetHTTPResponseMirrorInstance(func(isHttps bool, req, rsp []byte, remoteAddr string, response *http.Response) {
		urlStr := httpctx.GetRequestURL(response.Request)
		if urlStr == "" {
			u, _ := lowhttp.ExtractURLFromHTTPRequest(response.Request, isHttps)
			if u != nil {
				urlStr = u.String()
			}
			httpctx.SetRequestURL(response.Request, urlStr)
		}
		f(isHttps, urlStr, response.Request, response, remoteAddr)
	})
}

func MITM_SetTransparentHijackMode(t bool) MITMConfig {
	return func(server *MITMServer) error {
		if server.transparentHijackMode == nil {
			server.transparentHijackMode = utils.NewAtomicBool()
			server.transparentHijackMode.SetTo(t)
		}
		return nil
	}
}

type (
	MITMTransparentHijackFunc             func(isHttps bool, data []byte) []byte
	MITMTransparentHijackHTTPRequestFunc  func(isHttps bool, data *http.Request) *http.Request
	MITMTransparentHijackHTTPResponseFunc func(isHttps bool, data *http.Response) *http.Response
	MITMTransparentMirrorFunc             func(isHttps bool, req []byte, rsp []byte)
	MITMTransparentMirrorHTTPFunc         func(isHttps bool, req *http.Request, rsp *http.Response)
)

func MITM_SetTransparentHijackRequest(f MITMTransparentHijackFunc) MITMConfig {
	return func(server *MITMServer) error {
		if server.transparentHijackRequestManager != nil {
			return utils.Errorf("hijacked request manager have been set")
		}
		server.transparentHijackRequest = f
		return nil
	}
}

func MITM_SetTransparentHijackHTTPRequest(f MITMTransparentHijackHTTPRequestFunc) MITMConfig {
	return MITM_SetTransparentHijackRequest(func(isHttps bool, req []byte) []byte {
		rp, err := utils.ReadHTTPRequestFromBytes(req)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse request to *http.Request failed: %s", err)
			return nil
		}
		if isHttps {
			rp.URL.Scheme = "https"
		} else {
			rp.URL.Scheme = "http"
		}

		reqInstance := f(isHttps, rp)
		raw, err := utils.HttpDumpWithBody(reqInstance, true)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse *http.Request to []byte failed: %s", err)
			return nil
		}
		return raw
	})
}

func MITM_SetTransparentHijackResponse(f MITMTransparentHijackFunc) MITMConfig {
	return func(server *MITMServer) error {
		server.transparentHijackResponse = f
		return nil
	}
}

func MITM_SetTransparentHijackHTTPResponse(f MITMTransparentHijackHTTPResponseFunc) MITMConfig {
	return MITM_SetTransparentHijackResponse(func(isHttps bool, rsp []byte) []byte {
		rp, err := utils.ReadHTTPResponseFromBufioReader(bufio.NewReader(bytes.NewReader(rsp)), nil)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse response to *http.Response failed: %s", err)
			return nil
		}

		rspInstance := f(isHttps, rp)
		raw, err := utils.DumpHTTPResponse(rspInstance, true)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse *http.Response to []byte failed: %s", err)
			return nil
		}

		return raw
	})
}

func MITM_SetTransparentHijackedMirror(f MITMTransparentMirrorFunc) MITMConfig {
	return func(server *MITMServer) error {
		server.transparentHijackedMirror = f
		return nil
	}
}

func MITM_SetTransparentHijackedMirrorHTTP(f MITMTransparentMirrorHTTPFunc) MITMConfig {
	return MITM_SetTransparentHijackedMirror(func(isHttps bool, req []byte, rsp []byte) {
		rq, err := utils.ReadHTTPRequestFromBytes(req)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse request to *http.Request failed: %s", err)
			return
		}

		if isHttps {
			rq.URL.Scheme = "https"
		} else {
			rq.URL.Scheme = "http"
		}

		rp, err := utils.ReadHTTPResponseFromBufioReader(bufio.NewReader(bytes.NewReader(rsp)), rq)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse response to *http.Response failed: %s", err)
			return
		}

		f(isHttps, rq, rp)
	})
}

func MITM_SetTransparentMirror(f MITMTransparentMirrorFunc) MITMConfig {
	return func(server *MITMServer) error {
		server.transparentOriginMirror = f
		return nil
	}
}

func MITM_SetTransparentMirrorHTTP(f MITMTransparentMirrorHTTPFunc) MITMConfig {
	return MITM_SetTransparentMirror(func(isHttps bool, req []byte, rsp []byte) {
		rq, err := utils.ReadHTTPRequestFromBytes(req)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse request to *http.Request failed: %s", err)
			return
		}
		if isHttps {
			rq.URL.Scheme = "https"
		} else {
			rq.URL.Scheme = "http"
		}
		rp, err := utils.ReadHTTPResponseFromBufioReader(bufio.NewReader(bytes.NewReader(rsp)), rq)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse response to *http.Response failed: %s", err)
			return
		}

		f(isHttps, rq, rp)
	})
}

func MITM_SetDNSServers(servers ...string) MITMConfig {
	return func(server *MITMServer) error {
		server.DNSServers = servers
		return nil
	}
}

func MITM_SetHostMapping(m map[string]string) MITMConfig {
	return func(server *MITMServer) error {
		server.HostMapping = m
		return nil
	}
}

func MITM_SetMaxContentLength(m int64) MITMConfig {
	return func(server *MITMServer) error {
		server.maxContentLength = int(m)
		return nil
	}
}

func MITM_SetMaxReadWaitTime(m time.Duration) MITMConfig {
	return func(server *MITMServer) error {
		server.maxReadWaitTime = m
		return nil
	}
}

func MITM_AppendDNSServers(servers ...string) MITMConfig {
	return func(server *MITMServer) error {
		server.DNSServers = utils.RemoveRepeatStringSlice(append(server.DNSServers, servers...))
		return nil
	}
}

func MITM_SetTransparentHijackRequestManager(m *TransparentHijackManager) MITMConfig {
	return func(server *MITMServer) error {
		if server.transparentHijackRequest != nil {
			return utils.Errorf("transparent hijack request (basic) have been set")
		}
		server.transparentHijackRequestManager = m
		return nil
	}
}

// websocket

func MITM_SetWebsocketHijackMode(t bool) MITMConfig {
	return func(server *MITMServer) error {
		if server.websocketHijackMode == nil {
			server.websocketHijackMode = utils.NewAtomicBool()
		}
		server.websocketHijackMode.SetTo(t)
		return nil
	}
}

func MITM_SetForceTextFrame(t bool) MITMConfig {
	return func(server *MITMServer) error {
		if server.forceTextFrame == nil {
			server.forceTextFrame = utils.NewAtomicBool()
		}
		server.forceTextFrame.SetTo(t)
		return nil
	}
}

func MITM_SetWebsocketRequestHijackRaw(c func(req []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte) MITMConfig {
	return func(server *MITMServer) error {
		server.websocketRequestHijackHandler = c
		return nil
	}
}

func MITM_SetWebsocketResponseHijackRaw(c func(rsp []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte) MITMConfig {
	return func(server *MITMServer) error {
		server.websocketResponseHijackHandler = c
		return nil
	}
}

func MITM_SetWebsocketRequestMirrorRaw(f func(req []byte)) MITMConfig {
	return func(server *MITMServer) error {
		server.websocketRequestMirror = f
		return nil
	}
}

func MITM_SetWebsocketResponseMirrorRaw(f func(req []byte)) MITMConfig {
	return func(server *MITMServer) error {
		server.websocketResponseMirror = f
		return nil
	}
}

func MITM_SetTunMode(b bool) MITMConfig {
	return func(server *MITMServer) error {
		server.tunMode = b
		return nil
	}
}

func MITM_SetDialer(dialer func(duration time.Duration, target string) (net.Conn, error)) MITMConfig {
	return func(server *MITMServer) error {
		server.dialer = dialer
		return nil
	}
}

func MITM_SetExtraIncomingConectionChannel(ch chan net.Conn) MITMConfig {
	return func(server *MITMServer) error {
		server.extraIncomingConnChans = append(server.extraIncomingConnChans, ch)
		return nil
	}
}
