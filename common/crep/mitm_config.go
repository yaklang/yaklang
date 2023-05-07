package crep

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	log "github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/martian/v3"
	"github.com/yaklang/yaklang/common/martian/v3/h2"
	"github.com/yaklang/yaklang/common/martian/v3/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils/go-pkcs12"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"golang.org/x/net/http2"
)

// round trip
type HTTPRoundTripHandler func(req *http.Request) (*http.Response, error)

func NewRoundTripHandler(f HTTPRoundTripHandler) *mitmRoundTrip {
	return &mitmRoundTrip{f: f}
}

type mitmRoundTrip struct {
	f HTTPRoundTripHandler
}

func (r *mitmRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.f(req)
}

// request modifier
func NewRequestModifier(f martian.RequestModifierFunc) martian.RequestModifier {
	return &requestModifierFunc{f: f}
}

type requestModifierFunc struct {
	f martian.RequestModifierFunc
}

func (r *requestModifierFunc) ModifyRequest(req *http.Request) error {
	return r.f(req)
}

// response modifier
func NewResponseModifier(f martian.ResponseModifierFunc) martian.ResponseModifier {
	return &responseModifierFunc{f: f}
}

type responseModifierFunc struct {
	f martian.ResponseModifierFunc
}

func (r *responseModifierFunc) ModifyResponse(req *http.Response) error {
	return r.f(req)
}

// config
type MITMConfig func(server *MITMServer) error

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

func MITM_SetCaCertAndPrivKey(ca []byte, key []byte) MITMConfig {
	return func(server *MITMServer) error {
		if ca == nil || key == nil {
			return MITM_SetCaCertAndPrivKey(defaultCA, defaultKey)(server)
		}

		c, err := tls.X509KeyPair(ca, key)
		if err != nil {
			return utils.Errorf("parse ca and privKey failed: %s", err)
		}

		cert, err := x509.ParseCertificate(c.Certificate[0])
		if err != nil {
			return utils.Errorf("extract x509 cert failed: %s", err)
		}

		mc, err := mitm.NewConfig(cert, c.PrivateKey)
		if err != nil {
			return utils.Errorf("build private key failed: %s", err)
		}

		mc.SkipTLSVerify(true)
		mc.SetOrganization("MITMServer")
		mc.SetValidity(time.Hour * 24 * 90)

		// add default config for H2 support
		defaultH2Config := new(h2.Config)
		if log.GetLevel() == log.DebugLevel {
			defaultH2Config.EnableDebugLogs = true //for DEBUG and DEV only
		}

		certPool, err := x509.SystemCertPool()
		if err != nil {
			log.Fatal("Failed to retrieve system certificates pool")
		}
		certPool.AddCert(cert) // even though user may not add yak certificate yet, we add it manually

		defaultH2Config.RootCAs = certPool
		defaultH2Config.AllowedHostsFilter = func(_ string) bool { return true }

		mc.SetH2Config(defaultH2Config)

		server.caCert = ca
		server.caKey = key
		server.mitmConfig = mc

		return nil
	}
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

func NewDefaultClientOptions() *HTTPClientOptions {
	return &HTTPClientOptions{
		DialTimeout:         120,
		TLSHandshakeTimeout: 120,
		ReadTimeout:         120,
		IdleConnTimeout:     120,
		MaxConnsPerHost:     10,
		MaxIdleConns:        50,
		TLSSkipVerify:       true,
		TLSMinVersion:       tls.VersionSSL30, // nolint[:staticcheck]
		TLSMaxVersion:       tls.VersionTLS13,
	}
}

type HTTPClientOptions struct {
	Proxy               string       `json:"proxy" yaml:"proxy"`               // HTTP 代理
	DialTimeout         int          `json:"dial_timeout" yaml:"dial_timeout"` // tcp connect timeout
	TLSHandshakeTimeout int          `json:"tls_handshake_timeout" yaml:"tls_handshake_timeout"`
	ReadTimeout         int          `json:"read_timeout" yaml:"read_timeout"` // http read timeout
	IdleConnTimeout     int          `json:"idle_conn_timeout" yaml:"idle_conn_timeout"`
	MaxConnsPerHost     int          `json:"max_conns_per_host" yaml:"max_conns_per_host"`
	MaxIdleConns        int          `json:"max_idle_conns" yaml:"max_idle_conns"`
	TLSSkipVerify       bool         `json:"tls_skip_verify" yaml:"tls_skip_verify"` // 是否验证证书
	TLSMinVersion       uint16       `json:"tls_min_version" yaml:"tls_min_version"` // ssl / tls 版本号
	TLSMaxVersion       uint16       `json:"tls_max_version" yaml:"tls_max_version"`
	PKCS12              PKCS12Config `json:"pkcs12" yaml:"pkcs12"`
	EnableHTTP2         bool         `json:"enable_http2" yaml:"enable_http2"`
	EnableGMTLS         bool
	PreferGM            bool
	// 下面的需要自己实现
	//FailRetries     int               `json:"fail_retries" yaml:"fail_retries"`
	//MaxRedirect     int               `json:"max_redirect" yaml:"max_redirect"`
	//MaxRespBodySize int64             `json:"max_resp_body_size" yaml:"max_resp_body_size"`
	//MaxQPS          rate.Limit        `json:"max_qps" yaml:"max_qps"` // 全局最大每秒请求数
	//Headers         Header            `json:"headers" yaml:"headers"`
	//Cookies         map[string]string `json:"cookies" yaml:"cookies"`
	//AllowMethods    []string          `json:"allow_methods" yaml:"allow_methods"`
	//ExemptPathRegex string            `json:"exempt_path_regex" yaml:"exempt_path_regex"`
}

func NewTransport(opts *HTTPClientOptions) (*http.Transport, *http.Transport) {
	proxyFunc := http.ProxyFromEnvironment
	if opts.Proxy != "" {
		parsedURL, err := url.Parse(opts.Proxy)
		if err != nil {
			log.Error("incorrect proxy url", opts.Proxy)
		} else {
			proxyFunc = http.ProxyURL(parsedURL)
		}
	}

	certificates := make([]tls.Certificate, 0, 1)
	if opts.PKCS12.Path != "" {
		clientCertificate, err := ParsePKCS12FromFile(opts.PKCS12)
		if err != nil {
			log.Fatal(err)
		}
		certificates = append(certificates, *clientCertificate)
	}

	t := &http.Transport{
		Proxy: proxyFunc,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(opts.DialTimeout) * time.Second,
		}).DialContext,
		DisableCompression:    true,
		DisableKeepAlives:     false,
		MaxIdleConns:          opts.MaxIdleConns,
		MaxConnsPerHost:       opts.MaxConnsPerHost,
		IdleConnTimeout:       time.Duration(opts.IdleConnTimeout) * time.Second,
		TLSHandshakeTimeout:   time.Duration(opts.TLSHandshakeTimeout) * time.Second,
		ResponseHeaderTimeout: time.Duration(opts.ReadTimeout) * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: opts.TLSSkipVerify,
			MinVersion:         opts.TLSMinVersion,
			MaxVersion:         opts.TLSMaxVersion,
			Certificates:       certificates,
		},
	}
	if opts.EnableHTTP2 {
		err := http2.ConfigureTransport(t)
		if err != nil {
			log.Errorf("http2 config failed: %s", err)
		} else {
			log.Info("http2 config success")
		}
	}
	if !opts.EnableGMTLS {
		return t, nil
	} else {
		gmtr := &http.Transport{
			Proxy: proxyFunc,
			DialContext: (&net.Dialer{
				Timeout: time.Duration(opts.DialTimeout) * time.Second,
			}).DialContext,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) { /* this field may be re-set
				if mTLS in yakit enabled */
				dialer := &net.Dialer{}
				conn, err := gmtls.DialWithDialer(dialer, network, addr, &gmtls.Config{
					GMSupport:          &gmtls.GMSupport{},
					InsecureSkipVerify: true,
				})
				if err != nil {
					return nil, err
				}
				return conn, nil
			},
			DisableCompression:    true,
			DisableKeepAlives:     false,
			MaxIdleConns:          opts.MaxIdleConns,
			MaxConnsPerHost:       opts.MaxConnsPerHost,
			IdleConnTimeout:       time.Duration(opts.IdleConnTimeout) * time.Second,
			TLSHandshakeTimeout:   time.Duration(opts.TLSHandshakeTimeout) * time.Second,
			ResponseHeaderTimeout: time.Duration(opts.ReadTimeout) * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: opts.TLSSkipVerify,
				MinVersion:         opts.TLSMinVersion,
				MaxVersion:         opts.TLSMaxVersion,
				Certificates:       certificates,
			},
		}
		return t, gmtr
	}

}

func ParsePKCS12FromFile(c PKCS12Config) (*tls.Certificate, error) {
	data, err := ioutil.ReadFile(c.Path)
	if err != nil {
		return nil, err
	}

	privateKey, certificate, _, err := pkcs12.DecodeChain(data, c.Password)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{certificate.Raw},
		PrivateKey:  privateKey,
		Leaf:        certificate,
	}, nil
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

func MITM_SetTransport(tr *http.Transport, gmtr *http.Transport) MITMConfig {
	return func(server *MITMServer) error {
		server.httpTransport = tr
		server.httpTransportForGM = gmtr
		return nil
	}
}

func MITM_SetTransportByHTTPClientOptions(client *HTTPClientOptions) MITMConfig {
	return MITM_SetTransport(NewTransport(client))
}

func MITM_SetDownstreamProxy(s string) MITMConfig {
	return func(server *MITMServer) error {
		if s == "" {
			return nil
		}
		urlRaw, err := url.Parse(s)
		if err != nil {
			return utils.Errorf("parse proxy url: %v failed: %s", s, err)
		}
		log.Infof("set downstream proxy as %v", urlRaw.String())
		server.proxyUrl = urlRaw

		if server.httpTransport != nil {
			server.httpTransport.Proxy = func(request *http.Request) (u *url.URL, err error) {
				ur, err := lowhttp.ExtractURLFromHTTPRequest(request, true)
				if err != nil {
					log.Errorf("url: %s cannot use proxy: %s", ur, urlRaw.String())
					return nil, utils.Errorf("invalid http.Request: %v", err)
				}
				log.Infof("url: %s use proxy: %s", ur, urlRaw.String())
				return urlRaw, nil
			}
		}
		return nil
	}
}

func MITM_SetHTTPRequestMirrorRaw(f func(isHttps bool, raw []byte)) MITMConfig {
	return func(server *MITMServer) error {
		server.requestMirror = f
		return nil
	}
}

func MITM_SetHTTPRequestHijackRaw(c func(isHttps bool, reqIns *http.Request, req []byte) []byte) MITMConfig {
	return func(server *MITMServer) error {
		server.requestHijackHandler = c
		return nil
	}
}

func MITM_SetLowerHeader(c ...string) MITMConfig {
	return func(server *MITMServer) error {
		//server.lowerHeaders = c
		return nil
	}
}

func MITM_SetHTTPResponseHijackRaw(c func(isHttps bool, req *http.Request, rsp []byte, remoteAddr string) []byte) MITMConfig {
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
			//raw, err := httputil.DumpRequestOut(hijackedReq, true)
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
		server.responseMirrorWithInstance = f
		server.responseMirror = func(isHttps bool, req, rsp []byte, remoteAddr string) {
			f(isHttps, req, rsp, remoteAddr, nil)
		}
		return nil
	}
}

func MITM_SetHTTPResponseMirrorRaw(f func(isHttps bool, req, rsp []byte, remoteAddr string)) MITMConfig {
	return func(server *MITMServer) error {
		server.responseMirror = f
		return nil
	}
}

func MITM_SetHTTPRequestMirror(f func(isHttps bool, req *http.Request)) MITMConfig {
	return MITM_SetHTTPRequestMirrorRaw(func(isHttps bool, raw []byte) {
		req, err := lowhttp.ReadHTTPRequest(bufio.NewReader(bytes.NewBuffer(raw)))
		if err != nil {
			log.Errorf("parse raw to http.Request failed: %s", err)
			return
		}
		if isHttps {
			req.URL.Scheme = "https"
		} else {
			req.URL.Scheme = "http"
		}

		f(isHttps, req)
	})
}

func MITM_SetHTTPResponseMirror(f func(isHttps bool, reqUrl string, _ *http.Request, _ *http.Response, remoteAddr string)) MITMConfig {
	return MITM_SetHTTPResponseMirrorInstance(func(isHttps bool, req, rsp []byte, remoteAddr string, response *http.Response) {
		var schema = "http"
		if isHttps {
			schema = "https"
		}

		reqObj, err := lowhttp.ParseBytesToHttpRequest(req)
		if response != nil && response.Request != nil {
			*reqObj = *reqObj.WithContext(response.Request.Context())
		}
		//reqObj, err := http.ReadRequest(bufio.NewReader(bytes.NewBuffer(req)))
		if err != nil {
			log.Errorf("parse raw to http.Request failed: %s", err)
			return
		}
		reqObj.URL.Scheme = schema

		urlObj, err := lowhttp.ExtractURLFromHTTPRequest(reqObj, isHttps)
		if err != nil {
			log.Debugf("extract url from httpRequest: %v", err)
			urlObj = reqObj.URL
		}
		reqObj.URL.Host = urlObj.Host

		rspObj, err := http.ReadResponse(bufio.NewReader(bytes.NewBuffer(rsp)), reqObj)
		if err != nil {
			log.Errorf("parse raw to http.Response failed: %s", err)
			return
		}
		//log.Infof("recv request len:%-6d ==> response len:%-6d", len(req), len(rsp))
		if urlObj == nil {
			f(isHttps, "", reqObj, rspObj, remoteAddr)
		} else {
			f(isHttps, urlObj.String(), reqObj, rspObj, remoteAddr)
		}
	})
}

func MITM_SetMirrorRequestCacheTTL(ttl time.Duration) MITMConfig {
	return func(server *MITMServer) error {
		if server.mirrorCache != nil {
			server.mirrorCache.SetTTL(ttl)
		}
		return nil
	}
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

type MITMTransparentHijackFunc func(isHttps bool, data []byte) []byte
type MITMTransparentHijackHTTPRequestFunc func(isHttps bool, data *http.Request) *http.Request
type MITMTransparentHijackHTTPResponseFunc func(isHttps bool, data *http.Response) *http.Response
type MITMTransparentMirrorFunc func(isHttps bool, req []byte, rsp []byte)
type MITMTransparentMirrorHTTPFunc func(isHttps bool, req *http.Request, rsp *http.Response)

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
		rp, err := lowhttp.ReadHTTPRequest(bufio.NewReader(bytes.NewReader(req)))
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
		raw, err := httputil.DumpRequestOut(reqInstance, true)
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
		rp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(rsp)), nil)
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse response to *http.Response failed: %s", err)
			return nil
		}

		rspInstance := f(isHttps, rp)
		raw, err := httputil.DumpResponse(rspInstance, true)
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
		rq, err := lowhttp.ReadHTTPRequest(bufio.NewReader(bytes.NewReader(req)))
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse request to *http.Request failed: %s", err)
			return
		}

		if isHttps {
			rq.URL.Scheme = "https"
		} else {
			rq.URL.Scheme = "http"
		}

		rp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(rsp)), rq)
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
		rq, err := lowhttp.ReadHTTPRequest(bufio.NewReader(bytes.NewReader(req)))
		if err != nil {
			log.Errorf("[MITM-transparent CONFIG] parse request to *http.Request failed: %s", err)
			return
		}
		if isHttps {
			rq.URL.Scheme = "https"
		} else {
			rq.URL.Scheme = "http"
		}
		rp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(rsp)), rq)
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
