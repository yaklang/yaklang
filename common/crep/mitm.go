package crep

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/cybertunnel/ctxio"
	"yaklang.io/yaklang/common/gmsm/gmtls"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/martian/v3"
	"yaklang.io/yaklang/common/martian/v3/fifo"
	"yaklang.io/yaklang/common/martian/v3/header"
	"yaklang.io/yaklang/common/martian/v3/mitm"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
	"yaklang.io/yaklang/common/utils/tlsutils"

	"github.com/ReneKroon/ttlcache"
)

var (
	initMITMCertOnce              = new(sync.Once)
	defaultCAFile, defaultKeyFile = "yak-mitm-ca.crt", "yak-mitm-ca.key"
	defaultCA, defaultKey         []byte
)

func GetDefaultCaFilePath() string {
	return defaultCAFile
}

func init() {
	homeDir := consts.GetDefaultYakitBaseDir()
	//_ = os.MkdirAll(homeDir, os.ModePerm)
	defaultCAFile = filepath.Join(homeDir, defaultCAFile)
	defaultKeyFile = filepath.Join(homeDir, defaultKeyFile)
}

func GetDefaultCAAndPrivRaw() ([]byte, []byte) {
	ca, key, err := tlsutils.GenerateSelfSignedCertKeyWithCommonName("yak-mitm", "yaklang.io", nil, nil)
	if err != nil {
		panic(fmt.Sprintf("generate mitm root ca failed: %v", err))
	}
	return ca, key
}

func GetDefaultCAAndPriv() (*x509.Certificate, *rsa.PrivateKey, error) {
	ca, key := GetDefaultCAAndPrivRaw()
	p, _ := pem.Decode(ca)
	caCert, err := x509.ParseCertificate(p.Bytes)
	if err != nil {
		return nil, nil, utils.Errorf("default ca failed: %s", err)
	}

	priv, _ := pem.Decode(key)
	privKey, err := x509.ParsePKCS1PrivateKey(priv.Bytes)
	if err != nil {
		return nil, nil, utils.Errorf("default private key failed: %s", err)
	}

	return caCert, privKey, nil
}

func InitMITMCert() {
	defaultCA, _ = ioutil.ReadFile(defaultCAFile)
	defaultKey, _ = ioutil.ReadFile(defaultKeyFile)

	if defaultCA != nil && defaultKey != nil {
		log.Info("Successfully load cert and key from default files")
		return
	}

	if defaultCA == nil || defaultKey == nil {
		var err error
		defaultCA, defaultKey, err = tlsutils.GenerateSelfSignedCertKey("mitmserver", nil, nil)
		if err != nil {
			log.Errorf("generate default ca/key failed: %s", err)
			return
		}

		_ = os.MkdirAll(consts.GetDefaultYakitBaseDir(), 0777)
		err = ioutil.WriteFile(defaultCAFile, defaultCA, 0444)
		if err != nil {
			log.Error("write default ca failed")
		}
		err = ioutil.WriteFile(defaultKeyFile, defaultKey, 0444)
		if err != nil {
			log.Error("write default key failed")
		}
	}
}

func GetDefaultCaAndKey() ([]byte, []byte, error) {
	if defaultCA == nil || defaultKey == nil {
		return nil, nil, utils.Error("cannot set ca/key for mitm")
	}
	return defaultCA, defaultKey, nil
}

type ClientCertificationPair struct {
	CrtPem []byte
	KeyPem []byte
	CaPem  [][]byte
}

func NewClientCertificationPair(crt, key []byte, cas ...[]byte) *ClientCertificationPair {
	return &ClientCertificationPair{
		CrtPem: crt,
		KeyPem: key,
		CaPem:  cas,
	}
}

type ProxyAuth struct {
	Username string
	Password string
}

type MITMServer struct {
	proxy        *martian.Proxy
	mitmConfig   *mitm.Config
	caCert       []byte
	caKey        []byte
	dnsCache     *sync.Map
	lowerHeaders []string
	http2        bool
	gmtls        bool
	gmPrefer     bool
	gmOnly       bool

	clientCerts []*ClientCertificationPair

	DNSServers               []string
	via                      string
	allowForwarded           bool
	httpTransport            *http.Transport
	httpTransportForGM       *http.Transport
	proxyUrl                 *url.URL
	hijackedMaxContentLength int

	// transparent hijack mode
	transparentHijackRequestManager *TransparentHijackManager
	transparentHijackMode           *utils.AtomicBool
	transparentHijackRequest        func(isHttps bool, req []byte) []byte
	transparentHijackResponse       func(isHttps bool, rsp []byte) []byte
	transparentOriginMirror         func(isHttps bool, req, rsp []byte)
	transparentHijackedMirror       func(isHttps bool, req, rsp []byte)

	proxyAuth *ProxyAuth

	// mirror
	mirrorCache           *ttlcache.Cache
	mirrorCacheTTL        time.Duration
	requestHijackHandler  func(isHttps bool, originReq *http.Request, req []byte) []byte
	responseHijackHandler func(isHttps bool, r *http.Request, rsp []byte, remoteAddr string) []byte

	requestMirror              func(isHttps bool, req []byte)
	responseMirror             func(isHttps bool, req, rsp []byte, remoteAddr string)
	responseMirrorWithInstance func(isHttps bool, req, rsp []byte, remoteAddr string, response *http.Response)
	// websocket
	websocketHijackMode            *utils.AtomicBool
	forceTextFrame                 *utils.AtomicBool
	websocketRequestHijackHandler  func(req []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte
	websocketResponseHijackHandler func(rsp []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte
	websocketRequestMirror         func(req []byte)
	websocketResponseMirror        func(rsp []byte)

	// 缓存 remote addr cache
	remoteAddrCache *ttlcache.Cache
}

func (m *MITMServer) Configure(options ...MITMConfig) error {
	for _, p := range options {
		err := p(m)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *MITMServer) GetMartianProxy() *martian.Proxy {
	return m.proxy
}

func (m *MITMServer) GetCaCert() []byte {
	return m.caCert
}

func (m *MITMServer) Serve(ctx context.Context, addr string) error {
	if m.mitmConfig == nil {
		return utils.Errorf("mitm config empty")
	}

	if m.httpTransport == nil {
		return utils.Errorf("mitm transport empty")
	}

	originCtx := ctx
	m.httpTransport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion:         tls.VersionTLS13,
	}
	originHttpTransport := m.httpTransport
	originHttpTransportForGM := m.httpTransportForGM

	m.proxy.SetDownstreamProxy(m.proxyUrl)
	m.proxy.SetH2(m.http2)
	if m.proxyAuth != nil {
		m.proxy.SetAuth(m.proxyAuth.Username, m.proxyAuth.Password)
	}
	//m.proxy.SetRoundTripper(m.httpTransport)
	m.remoteAddrCache = ttlcache.NewCache()
	m.remoteAddrCache.SetTTL(10 * time.Second)

	/*
		为 httpTransport 设置 TLS 证书
	*/
	pool := x509.NewCertPool()
	for _, certs := range m.clientCerts {
		for _, ca := range certs.CaPem {
			pool.AppendCertsFromPEM(ca)
		}
		pair, _, err := tlsutils.ParseCertAndPriKeyAndPool(certs.CrtPem, certs.KeyPem)
		if err != nil {
			return utils.Errorf("initial tls with client cert error (mTLS error): %s", err)
		}
		m.httpTransport.TLSClientConfig.Certificates = append(m.httpTransport.TLSClientConfig.Certificates, pair)

		pairGM, _, err := tlsutils.ParseCertAndPriKeyAndPoolForGM(certs.CrtPem, certs.KeyPem)
		if err != nil {
			return utils.Errorf("initial tls with client cert error (mTLS error) for GM: %s", err)
		}
		m.httpTransportForGM.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{}
			conn, err := gmtls.DialWithDialer(dialer, network, addr, &gmtls.Config{
				GMSupport:          &gmtls.GMSupport{},
				Certificates:       []gmtls.Certificate{pairGM},
				InsecureSkipVerify: true,
			})
			if err != nil {
				return nil, err
			}
			ctx, _ = context.WithTimeout(originCtx, 30*time.Second)
			return ctxio.NewConn(ctx, conn), nil
		}
	}

	m.proxy.SetRoundTripper(&httpTraceTransport{
		Transport: originHttpTransport, cache: m.remoteAddrCache,
	})

	if m.httpTransportForGM != nil {
		m.proxy.SetRoundTripperForGM(originHttpTransportForGM)
		m.proxy.SetGMTLS(m.gmtls)
		m.proxy.SetGMPrefer(m.gmPrefer)
		m.proxy.SetGMOnly(m.gmOnly)
	}

	m.proxy.SetMITM(m.mitmConfig)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m.preHandle(ctx)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return utils.Errorf("listen port: %v failed: %s", addr, err)
	}
	defer lis.Close()

	go func() {
		select {
		case <-ctx.Done():
			_ = lis.Close()
		}
	}()

	log.Infof("start to server mitm server: tcp://%v", addr)
	err = m.proxy.Serve(lis, ctx)
	if err != nil {
		return utils.Errorf("serve proxy server failed: %s", err)
	}

	return nil
}

func (m *MITMServer) fixHeader(req *http.Request) {
	if req.Header.Get("user-agent") == "" {
		req.Header.Set("user-agent", consts.DefaultUserAgent)
	}
}

type rawRequest struct {
	IsHttps bool
	Raw     []byte
}

func (m *MITMServer) preHandle(ctx context.Context) {
	group := fifo.NewGroup()

	largerThanMaxContentLength := func(res *http.Response) bool {
		length, _ := strconv.Atoi(res.Header.Get("Content-Length"))
		if length > m.hijackedMaxContentLength && m.hijackedMaxContentLength > 0 {
			log.Infof("allow rsp: %p's content-length: %v passed for limit content-length", res, length)
			return true
		}
		return false
	}

	requestHijackCallback := func(req *http.Request) error {
		var isHttps bool
		switch req.URL.Scheme {
		case "https", "HTTPS":
			isHttps = true
		case "http", "HTTP":
			isHttps = false
		}

		if m.requestHijackHandler != nil {
			originUrl, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
			if err != nil {
				if strings.HasPrefix(err.Error(), "ignore connect") {
					return err
				}
				log.Errorf("parse request url failed: %s", err)
				return nil
			}

			log.Debugf("start to hijack mitm request: %p", req)
			m.fixHeader(req)
			hijackedRaw, err := httputil.DumpRequest(req, true)

			if err != nil {
				log.Errorf("mitm-hijack marshal request to bytes failed: %s", err)
				return nil
			}

			hijackedRequestRaw := m.requestHijackHandler(isHttps, req, hijackedRaw)
			select {
			case <-ctx.Done():
				reqContext := martian.NewContext(req, m.proxy)
				reqContext.SkipRoundTrip()
				return utils.Error("request hijacker error: MITM Proxy Context Canceled")
			default:
			}
			if hijackedRequestRaw == nil {
				time.Sleep(5 * time.Minute)
				return nil
			}
			hijackedReq, err := lowhttp.ParseBytesToHttpRequest(hijackedRequestRaw)
			if err != nil {
				log.Errorf("mitm-hijacked request to http.Request failed: %s", err)
				return nil
			}
			if isHttps {
				hijackedReq.TLS = req.TLS
			}

			if req.ProtoMajor != 2 {
				hijackedReq, err = utils.FixHTTPRequestForHTTPDoWithHttps(hijackedReq, isHttps)
				if err != nil {
					log.Errorf("fix mitm-hijacked http.Request failed: %s", err)
					return nil
				}
			}

			*req = *hijackedReq.WithContext(req.Context())
			m.fixHeader(req)

			if req.URL.Host == "" || req.URL.Hostname() == "" {
				hostInHeader := req.Header.Get("Host")
				if hostInHeader == "" && req.Host != "" {
					hostInHeader = req.Host
				}
				if hostInHeader != "" {
					req.Host = hostInHeader
					req.URL.Host = hostInHeader
				} else {
					req.Host = originUrl.Host
					req.URL.Host = originUrl.Host
				}
			}
			if isHttps {
				req.URL.Scheme = "https"
			}
			//log.Infof("finished to hijack mitm request: %p", req)
		}
		return nil
	}

	// 过滤证书请求-直接跳过
	group.AddRequestModifier(NewRequestModifier(func(req *http.Request) error {
		ctx := martian.NewContext(req, m.GetMartianProxy())
		if ctx == nil {
			return nil
		}
		//log.Infof("hostname: %v", req.URL.Hostname())
		if utils.StringArrayContains(defaultBuildinDomains, req.URL.Hostname()) {
			ctx.SkipRoundTrip()
		}
		return nil
	}))
	wsModifier := &WebSocketModifier{
		websocketHijackMode:            m.websocketHijackMode,
		forceTextFrame:                 m.forceTextFrame,
		websocketRequestHijackHandler:  m.websocketRequestHijackHandler,
		websocketResponseHijackHandler: m.websocketResponseHijackHandler,
		websocketRequestMirror:         m.websocketRequestMirror,
		websocketResponseMirror:        m.websocketResponseMirror,
		TR:                             m.httpTransport,
		ProxyGetter:                    m.GetMartianProxy,
		RequestHijackCallback:          requestHijackCallback,
	}
	if m.proxyUrl != nil {
		wsModifier.ProxyStr = m.proxyUrl.String()
	}
	group.AddRequestModifier(wsModifier)

	group.AddRequestModifier(header.NewHopByHopModifier())

	// fix err frame
	group.AddRequestModifier(header.NewBadFramingModifier())

	// 劫持
	group.AddRequestModifier(NewRequestModifier(func(req *http.Request) error {
		return requestHijackCallback(req)
	}))

	// 镜像分流
	group.AddRequestModifier(NewRequestModifier(func(req *http.Request) error {
		var isHttps bool
		switch req.URL.Scheme {
		case "https", "HTTPS":
			isHttps = true
		case "http", "HTTP":
			isHttps = false
		}

		raw, err := httputil.DumpRequest(req, true)
		if err != nil {
			log.Errorf("dump request failed: %s", err)
			return nil
		}

		p := fmt.Sprintf("%p", req)
		log.Debugf("request-id: [%v]", p)
		m.mirrorCache.Set(p, &rawRequest{
			IsHttps: isHttps,
			Raw:     raw,
		})
		if m.requestMirror != nil {
			m.requestMirror(isHttps, raw)
		}

		return nil
	}))

	if m.allowForwarded {
		group.AddRequestModifier(header.NewForwardedModifier())
	}

	if m.via != "" {
		log.Warnf("VIA is unsupported by yak mitm (no longer)")
		//vm := header.NewViaModifier(m.via)
		//group.AddRequestModifier(vm)
		//group.AddResponseModifier(vm)
	}

	// 证书-下载证书
	group.AddResponseModifier(NewResponseModifier(func(rsp *http.Response) error {
		if utils.StringArrayContains(defaultBuildinDomains, rsp.Request.URL.Hostname()) {
			body := defaultCA
			rsp.Body = ioutil.NopCloser(bytes.NewReader(body))
			rsp.ContentLength = int64(len(body))
			// rsp.Header.Set("Content-Length", strconv.Itoa(len(body)))
			rsp.Header.Set("Content-Disposition", `attachment; filename="mitm-server.crt"`)
			rsp.Header.Set("Content-Type", "octet-stream")
			return nil
		}
		return nil
	}))

	// 劫持响应
	group.AddResponseModifier(NewResponseModifier(func(rsp *http.Response) error {
		if largerThanMaxContentLength(rsp) {
			return nil
		}

		if m.responseHijackHandler != nil {
			var isHttps bool
			switch rsp.Request.URL.Scheme {
			case "https", "HTTPS":
				isHttps = true
			case "http", "HTTP":
				isHttps = false
			}

			var rspRaw []byte
			var err error
			rspRaw, err = utils.HttpDumpWithBody(rsp, true)
			if err != nil {
				log.Errorf("hijack response failed: %s", err)
				return nil
			}
			if rspRaw == nil {
				return nil
			}
			result := m.responseHijackHandler(isHttps, rsp.Request, rspRaw, m.GetRemoteAddr(isHttps, rsp.Request.Host))
			if result == nil {
				// 解析结果为空，这个时候可能是有意 drop 掉了请求，我们不返回就行了
				time.Sleep(5 * time.Minute)
				return nil
			}
			req := rsp.Request
			resultRsp, err := http.ReadResponse(bufio.NewReader(bytes.NewBuffer(result)), req)
			if err != nil {
				log.Errorf("parse fixed response to body failed: %s", err)
				return err
			}
			*rsp = *resultRsp
			return nil
		}
		return nil
	}))

	m.proxy.SetRequestModifier(group)
	m.proxy.SetResponseModifier(group)

	// mirror response
	group.AddResponseModifier(NewResponseModifier(func(res *http.Response) error {
		// 过滤一下最大长度
		haveBody := true
		if largerThanMaxContentLength(res) {
			haveBody = false
		}

		rspP := fmt.Sprintf("%p", res)
		reqP := fmt.Sprintf("%p", res.Request)
		log.Debugf("request-id: [%v]   -->   response-id: [%v] ", reqP, rspP)

		if m.responseMirrorWithInstance != nil {
			rsp, err := httputil.DumpResponse(res, haveBody)
			if err != nil {
				log.Errorf("dump response mirror failed: %s", err)
				return nil
			}

			_req, ok := m.mirrorCache.Get(reqP)
			if !ok {
				log.Debugf("mirror cache cannot fetch requestP: %p", res.Request)
				return nil
			}
			rawRequestIns := _req.(*rawRequest)
			reqRawBytes := rawRequestIns.Raw
			if ok {
				//log.Infof("response mirror recv request len: [%v]", len(rawReq))
				remoteAddr := m.GetRemoteAddr(rawRequestIns.IsHttps, res.Request.Host)
				m.responseMirrorWithInstance(rawRequestIns.IsHttps, reqRawBytes, rsp, remoteAddr, res)
				m.mirrorCache.Remove(reqP)
			} else {
				log.Errorf("no such request-id: %v", reqP)
			}
			return nil
		}
		return nil
	}))
}

var (
	defaultBuildinDomains = []string{
		"download-mitm-ca.com",
		"download-mitm-cert.yaklang.io",
	}
)

func NewMITMServer(options ...MITMConfig) (*MITMServer, error) {
	cache := ttlcache.NewCache()

	initMITMCertOnce.Do(InitMITMCert)

	proxy := martian.NewProxy()
	server := &MITMServer{
		proxy:                    proxy,
		mirrorCache:              ttlcache.NewCache(),
		DNSServers:               []string{"8.8.8.8", "114.114.114.114"},
		dnsCache:                 new(sync.Map),
		hijackedMaxContentLength: 10 * 1000 * 1000,
		http2:                    false,
	}

	// 配置 transport
	opts := NewDefaultClientOptions()

	for _, op := range options {
		err := op(server)
		if err != nil {
			return nil, utils.Errorf("config failed: %s", err)
		}
	}

	// MITM option configured above

	// sync config with MITMServer
	opts.EnableHTTP2 = server.http2
	opts.EnableGMTLS = server.gmtls

	err := MITM_SetTransportByHTTPClientOptions(opts)(server)
	if err != nil {
		return nil, utils.Errorf("create http transport failed: %v", err)
	}

	if server.mitmConfig == nil { // currently seems it must be nil since no function is exposed to directly create
		err := MITM_SetCaCertAndPrivKey(defaultCA, defaultKey)(server)
		if err != nil {
			return nil, utils.Errorf("set ca/key failed: %s", err)
		}
	}

	if server.mirrorCacheTTL <= 0 {
		cache.SetTTL(60 * time.Second)
	}

	if server.proxyUrl != nil {
		log.Infof("server go proxy: %v", server.proxyUrl.String())
		if server.httpTransport != nil {
			server.httpTransport.Proxy = func(request *http.Request) (*url.URL, error) {
				return server.proxyUrl, nil
			}
		}
		if server.httpTransportForGM != nil {
			server.httpTransportForGM.Proxy = func(request *http.Request) (*url.URL, error) {
				return server.proxyUrl, nil
			}
		}
	}

	return server, nil
}

func (s *MITMServer) GetRemoteAddr(isHttps bool, host string) string {
	if s == nil {
		return ""
	}

	if s.remoteAddrCache == nil {
		return ""
	}
	var schema = "http"
	if isHttps {
		schema = "https"
	}
	urlRaw := fmt.Sprintf("%v://%v", schema, host)
	host, port, err := utils.ParseStringToHostPort(urlRaw)
	if err != nil {
		return ""
	}
	key := utils.HostPort(host, port)
	r, ok := s.remoteAddrCache.Get(key)
	if !ok {
		return ""
	}
	return fmt.Sprint(r)
}

func (s *MITMServer) GetRemoteAddrRaw(host string) string {
	if s == nil {
		return ""
	}
	r, ok := s.remoteAddrCache.Get(host)
	if !ok {
		return ""
	}
	return fmt.Sprint(r)
}
