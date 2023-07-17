package crep

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/martian/v3"
	"github.com/yaklang/yaklang/common/martian/v3/fifo"
	"github.com/yaklang/yaklang/common/martian/v3/header"
	"github.com/yaklang/yaklang/common/martian/v3/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
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
	HostMapping              map[string]string
	via                      string
	allowForwarded           bool
	httpTransport            *http.Transport
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

	originHttpTransport := m.httpTransport

	m.proxy.SetDownstreamProxy(m.proxyUrl)
	m.proxy.SetH2(m.http2)
	if m.proxyAuth != nil {
		m.proxy.SetAuth(m.proxyAuth.Username, m.proxyAuth.Password)
	}
	//m.proxy.SetRoundTripper(m.httpTransport)
	m.remoteAddrCache = ttlcache.NewCache()
	m.remoteAddrCache.SetTTL(10 * time.Second)

	m.proxy.SetRoundTripper(&httpTraceTransport{
		Transport: originHttpTransport, cache: m.remoteAddrCache,
	})

	m.proxy.SetGMTLS(m.gmtls)
	m.proxy.SetGMPrefer(m.gmPrefer)
	m.proxy.SetGMOnly(m.gmOnly)

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

type rawRequest struct {
	IsHttps bool
	Raw     []byte
}

func (m *MITMServer) preHandle(rootCtx context.Context) {
	group := fifo.NewGroup()

	largerThanMaxContentLength := func(res *http.Response) bool {
		length, _ := strconv.Atoi(res.Header.Get("Content-Length"))
		if length > m.hijackedMaxContentLength && m.hijackedMaxContentLength > 0 {
			log.Infof("allow rsp: %p's content-length: %v passed for limit content-length", res, length)
			return true
		}
		return false
	}

	wsModifier := &WebSocketModifier{
		websocketHijackMode:            m.websocketHijackMode,
		forceTextFrame:                 m.forceTextFrame,
		websocketRequestHijackHandler:  m.websocketRequestHijackHandler,
		websocketResponseHijackHandler: m.websocketResponseHijackHandler,
		websocketRequestMirror:         m.websocketRequestMirror,
		websocketResponseMirror:        m.websocketResponseMirror,
		TR:                             m.httpTransport,
		ProxyGetter:                    m.GetMartianProxy,
		RequestHijackCallback: func(req *http.Request) error {
			var isHttps bool
			switch req.URL.Scheme {
			case "https", "HTTPS":
				isHttps = true
			case "http", "HTTP":
				isHttps = false
			}
			hijackedRaw, err := utils.HttpDumpWithBody(req, true)
			if err != nil {
				log.Errorf("mitm-hijack marshal request to bytes failed: %s", err)
				return nil
			}
			m.requestHijackHandler(isHttps, req, hijackedRaw)
			return nil
		},
	}
	if m.proxyUrl != nil {
		wsModifier.ProxyStr = m.proxyUrl.String()
	}

	group.AddRequestModifier(NewRequestModifier(func(req *http.Request) error {
		/*
		 use buildin cert domains
		*/

		//log.Infof("hostname: %v", req.URL.Hostname())
		if utils.StringArrayContains(defaultBuildinDomains, req.URL.Hostname()) {
			ctx := martian.NewContext(req, m.GetMartianProxy())
			if ctx != nil {
				ctx.SkipRoundTrip()
			}
			return nil
		}

		reqCtx, cancel := context.WithCancel(rootCtx)
		httpctx.SetContextValueInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_RequestHijackDone, reqCtx)
		defer func() {
			defer cancel()
			if err := recover(); err != nil {
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		/*
			handle websocket
		*/
		if utils.IContains(req.Header.Get("Connection"), "upgrade") && req.Header.Get("Upgrade") == "websocket" {
			return wsModifier.ModifyRequest(req)
		}

		err := header.NewHopByHopModifier().ModifyRequest(req)
		if err != nil {
			log.Errorf("remove hop by hop header failed: %s", err)
		}

		// content-length and transfer-encoding existed
		firstLine, _, _ := strings.Cut(req.Header.Get("Content-Length"), ",")
		if firstLine != "" {
			req.Header.Set("Content-Length", firstLine)
		}
		te, _, _ := strings.Cut(req.Header.Get("Transfer-Encoding"), ",")
		if te != "" {
			req.Header.Set("Transfer-Encoding", "chunked")
			req.Header.Del("Content-Length")
		}

		/*
			handle hijack
		*/
		var isHttps bool
		switch req.URL.Scheme {
		case "https", "HTTPS":
			isHttps = true
		case "http", "HTTP":
			isHttps = false
		}

		var (
			raw       []byte
			isDropped = utils.NewBool(false)
		)
		if m.requestHijackHandler != nil {
			originUrl, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
			if err != nil {
				if strings.HasPrefix(err.Error(), "ignore connect") {
					return err
				}
				log.Errorf("parse request url failed: %s", err)
				return nil
			}

			hijackedRaw, err := utils.HttpDumpWithBody(req, true)
			if err != nil {
				log.Errorf("mitm-hijack marshal request to bytes failed: %s", err)
				return nil
			}
			raw = hijackedRaw

			/*
				ctx control
			*/
			select {
			case <-rootCtx.Done():
				reqContext := martian.NewContext(req, m.proxy)
				reqContext.SkipRoundTrip()
				return utils.Error("request hijacker error: MITM Proxy Context Canceled")
			default:
			}
			hijackedRequestRaw := m.requestHijackHandler(isHttps, req, hijackedRaw)
			select {
			case <-rootCtx.Done():
				reqContext := martian.NewContext(req, m.proxy)
				reqContext.SkipRoundTrip()
				return utils.Error("request hijacker error: MITM Proxy Context Canceled")
			default:
			}
			if hijackedRequestRaw == nil {
				isDropped.Set()
			} else {
				hijackedRaw = hijackedRequestRaw
				raw = hijackedRequestRaw
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

			}
		}

		if raw == nil {
			raw, err = httputil.DumpRequest(req, true)
			if err != nil {
				log.Errorf("dump request failed: %s", err)
				return nil
			}
		}
		m.mirrorCache.Set(fmt.Sprintf("%p", req), &rawRequest{
			IsHttps: isHttps,
			Raw:     raw,
		})
		if m.requestMirror != nil {
			m.requestMirror(isHttps, raw)
		}

		return nil
	}))

	// 劫持响应
	group.AddResponseModifier(NewResponseModifier(func(rsp *http.Response) error {
		/*
			return the ca certs
		*/
		if utils.StringArrayContains(defaultBuildinDomains, rsp.Request.URL.Hostname()) {
			body := defaultCA
			rsp.Body = ioutil.NopCloser(bytes.NewReader(body))
			rsp.ContentLength = int64(len(body))
			// rsp.Header.Set("Content-Length", strconv.Itoa(len(body)))
			rsp.Header.Set("Content-Disposition", `attachment; filename="mitm-server.crt"`)
			rsp.Header.Set("Content-Type", "octet-stream")
			return nil
		}

		val, ok := httpctx.GetContextInfoMap(rsp.Request).Load(httpctx.REQUEST_CONTEXT_KEY_RequestHijackDone)
		if !ok {
			msg := `[BUG]! ResponseModifer Cannot Fetch InfoMap Context`
			fmt.Println(msg)
			fmt.Println(msg)
			fmt.Println(msg)
			return utils.Error(msg)
		}
		cond := val.(context.Context)
		select {
		case <-cond.Done():
		}

		var (
			responseBytes    []byte
			dropped          = utils.NewBool(false)
			shouldHandleBody = true
		)

		// response hijacker
		if m.responseHijackHandler != nil {
			// max content-length
			if largerThanMaxContentLength(rsp) {
				shouldHandleBody = false
			}

			var isHttps bool
			switch rsp.Request.URL.Scheme {
			case "https", "HTTPS":
				isHttps = true
			case "http", "HTTP":
				isHttps = false
			}

			var err error
			responseBytes, err = utils.HttpDumpWithBody(rsp, shouldHandleBody)
			if err != nil {
				log.Errorf("hijack response failed: %s", err)
				return nil
			}
			if responseBytes == nil {
				return nil
			}
			result := m.responseHijackHandler(isHttps, rsp.Request, responseBytes, m.GetRemoteAddr(isHttps, rsp.Request.Host))
			if result == nil {
				dropped.Set()
			} else {
				responseBytes = result[:]
				req := rsp.Request
				resultRsp, err := http.ReadResponse(bufio.NewReader(bytes.NewBuffer(result)), req)
				if err != nil {
					log.Errorf("parse fixed response to body failed: %s", err)
					return utils.Errorf("hijacking modified response parsing failed: %s", err)
				}
				*rsp = *resultRsp
			}
		}

		defer func() {
			if dropped.IsSet() {
				log.Info("drop response cause sleep in httpflow")
				time.Sleep(3 * time.Minute)
			}
		}()

		/**
		mirror below, no modify packet!
		*/
		rspP := fmt.Sprintf("%p", rsp)
		reqP := utils.InterfaceToString(rsp.Request.Context().Value("request-id"))

		log.Debugf("request-id: [%v]   -->   response-id: [%v] ", reqP, rspP)

		if m.responseMirrorWithInstance != nil {
			if len(responseBytes) <= 0 {
				var err error
				responseBytes, err = utils.HttpDumpWithBody(rsp, shouldHandleBody)
				if err != nil {
					log.Errorf("dump response mirror failed: %s", err)
					return nil
				}
			}

			_req, ok := m.mirrorCache.Get(reqP)
			if !ok {
				log.Debugf("mirror cache cannot fetch requestP: %p", rsp.Request)
				return nil
			}
			rawRequestIns := _req.(*rawRequest)
			reqRawBytes := rawRequestIns.Raw
			if ok {
				//log.Infof("response mirror recv request len: [%v]", len(rawReq))
				remoteAddr := m.GetRemoteAddr(rawRequestIns.IsHttps, rsp.Request.Host)
				m.responseMirrorWithInstance(rawRequestIns.IsHttps, reqRawBytes, responseBytes, remoteAddr, rsp)
				m.mirrorCache.Remove(reqP)
			} else {
				log.Errorf("no such request-id: %v", reqP)
			}
			return nil
		}
		return nil
	}))

	m.proxy.SetRequestModifier(group)
	m.proxy.SetResponseModifier(group)
}

var (
	defaultBuildinDomains = []string{
		"download-mitm-ca.com",
		"download-mitm-cert.yaklang.io",
		"mitm",
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
		HostMapping:              make(map[string]string),
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
	opts.OnlyGM = server.gmOnly
	opts.PreferGM = server.gmPrefer
	opts.DnsServers = server.DNSServers
	opts.HostMapping = server.HostMapping
	opts.ClientCerts = server.clientCerts
	// Do custom transport configuration here
	// 按理说在这之后transport就不应该被改动了 除了最后传给martian做roundTripper时套了个Trace
	loadTransport, err := MITM_SetTransportByHTTPClientOptions(opts)
	if err != nil {
		return nil, err
	}
	err = loadTransport(server)
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
