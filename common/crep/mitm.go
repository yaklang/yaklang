package crep

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/gmsm/gmtls"

	"github.com/yaklang/yaklang/common/gmsm/sm2"
	gmx509 "github.com/yaklang/yaklang/common/gmsm/x509"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/minimartian/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

var (
	initMITMCertOnce                  = new(sync.Once)
	defaultCAFile, defaultKeyFile     = "yak-mitm-ca.crt", "yak-mitm-ca.key"
	defaultGMCAFile, defaultGMKeyFile = "yak-mitm-gm-ca.crt", "yak-mitm-gm-ca.key"
	defaultCA, defaultKey             []byte
	defaultGMCA, defaultGMKey         []byte
)

func GetDefaultCaFilePath() string {
	return defaultCAFile
}

func GetDefaultGMCaFilePath() string {
	return defaultGMCAFile
}

func init() {
	homeDir := consts.GetDefaultYakitBaseDir()
	//_ = os.MkdirAll(homeDir, os.ModePerm)
	defaultCAFile = filepath.Join(homeDir, defaultCAFile)
	defaultKeyFile = filepath.Join(homeDir, defaultKeyFile)

	defaultGMCAFile = filepath.Join(homeDir, defaultGMCAFile)
	defaultGMKeyFile = filepath.Join(homeDir, defaultGMKeyFile)
}

// DebugSetDefaultGMCAFileAndKey is used for test purpose. To simulate GM cert generation error
// which can spawn malformed GM certs or can be used to test edge case where gmCA and gmKey are nil
func DebugSetDefaultGMCAFileAndKey(ca, key []byte) {
	initMITMCertOnce.Do(func() {})
	defaultGMCA = ca
	defaultGMKey = key
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

func GetDefaultMITMCAAndPriv() (*x509.Certificate, *rsa.PrivateKey, error) {
	ca, key, err := GetDefaultCaAndKey()
	if err != nil {
		return nil, nil, err
	}
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

func GetDefaultMITMCAAndPrivForGM() (*gmx509.Certificate, *sm2.PrivateKey, error) {
	ca, key, err := GetDefaultGMCaAndKey()
	if err != nil {
		return nil, nil, err
	}
	p, _ := pem.Decode(ca)
	caCert, err := gmx509.ParseCertificate(p.Bytes)
	if err != nil {
		return nil, nil, utils.Errorf("default ca failed: %s", err)
	}

	priv, _ := pem.Decode(key)
	privKey, err := gmx509.ParseSm2PrivateKey(priv.Bytes)
	if err == nil {
		return caCert, privKey, nil
	}
	privKey, err = gmx509.ParsePKCS8UnecryptedPrivateKey(priv.Bytes)
	if err != nil {
		return nil, nil, utils.Errorf("default private key failed: %s", err)
	}
	return caCert, privKey, nil
}

func InitMITMCert() {
	defaultCA, _ = ioutil.ReadFile(defaultCAFile)
	defaultKey, _ = ioutil.ReadFile(defaultKeyFile)

	defaultGMCA, _ = ioutil.ReadFile(defaultGMCAFile)
	defaultGMKey, _ = ioutil.ReadFile(defaultGMKeyFile)

	if defaultCA != nil && defaultKey != nil {
		log.Debug("Successfully load cert and key from default files")
	}

	if defaultGMCA != nil && defaultGMKey != nil {
		// just check if the cert is valid
		if _, err := gmtls.X509KeyPair(defaultGMCA, defaultGMKey); err == nil {
			log.Debug("Successfully load GM cert and key from default files")
			return
		} else {
			log.Infof("detect gm ca certs n key err for parsing, re-generate it, reason: %v", err)
			_ = os.RemoveAll(defaultGMCAFile)
			_ = os.RemoveAll(defaultGMKeyFile)
			defaultGMCA = nil
			defaultGMKey = nil
		}
	}

	if defaultCA == nil || defaultKey == nil {
		var err error
		defaultCA, defaultKey, err = tlsutils.GenerateSelfSignedCertKey("mitmserver", nil, nil)
		if err != nil {
			log.Errorf("generate default ca/key failed: %s", err)
			return
		}

		_ = os.MkdirAll(consts.GetDefaultYakitBaseDir(), 0o777)
		err = ioutil.WriteFile(defaultCAFile, defaultCA, 0o444)
		if err != nil {
			log.Error("write default ca failed")
		}
		err = ioutil.WriteFile(defaultKeyFile, defaultKey, 0o444)
		if err != nil {
			log.Error("write default key failed")
		}
	}

	if defaultGMCA == nil || defaultGMKey == nil {
		var err error
		defaultGMCA, defaultGMKey, err = tlsutils.GenerateGMSelfSignedCertKey("Yakit MITM GM Root CA")
		if err != nil {
			log.Errorf("generate GM default ca/key failed: %s", err)
			return
		}

		_ = os.MkdirAll(consts.GetDefaultYakitBaseDir(), 0o777)
		err = ioutil.WriteFile(defaultGMCAFile, defaultGMCA, 0o444)
		if err != nil {
			log.Error("write default GM ca failed")
		}
		err = ioutil.WriteFile(defaultGMKeyFile, defaultGMKey, 0o444)
		if err != nil {
			log.Error("write default GM key failed")
		}
	}
}

func FakeCertificateByHost(caCert *x509.Certificate, caKey *rsa.PrivateKey, domain string) (tls.Certificate, error) {
	keys, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   domain,
			Country:      []string{"Yak"},
			Province:     []string{"Yak"},
			Locality:     []string{"Yak"},
			Organization: []string{"yaklang.io Project"},
			OrganizationalUnit: []string{
				"https://yaklang.com/",
			},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              []string{domain},
	}

	certBytes, _ := x509.CreateCertificate(rand.Reader, &template, caCert, &keys.PublicKey, caKey)

	x509c, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return tls.Certificate{}, err
	}

	tlsc := tls.Certificate{
		Certificate: [][]byte{certBytes, caCert.Raw},
		PrivateKey:  keys,
		Leaf:        x509c,
	}
	return tlsc, nil
}

func VerifySystemCertificate() error {
	InitMITMCert()
	caCert, caKey, _ := GetDefaultMITMCAAndPriv()
	fakeCert, err := FakeCertificateByHost(caCert, caKey, "yaklang.com")
	if err != nil {
		return err
	}

	// 解析证书
	cert, err := x509.ParseCertificate(fakeCert.Certificate[0])
	if err != nil {
		return err
	}

	// 创建系统根证书池
	pool, err := x509.SystemCertPool()
	if err != nil {
		return err
	}

	// 验证证书是否在系统根证书池中
	opts := x509.VerifyOptions{
		Roots: pool,
	}
	_, err = cert.Verify(opts)
	if err != nil {
		return err
	}
	return nil
}

func GetDefaultCaAndKey() ([]byte, []byte, error) {
	if defaultCA == nil || defaultKey == nil {
		return nil, nil, utils.Error("cannot set ca/key for mitm")
	}
	return defaultCA, defaultKey, nil
}

func GetDefaultGMCaAndKey() ([]byte, []byte, error) {
	if defaultGMCA == nil || defaultGMKey == nil {
		return nil, nil, utils.Error("cannot set GM ca/key for mitm")
	}
	return defaultGMCA, defaultGMKey, nil
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
	proxy                 *minimartian.Proxy
	mitmConfig            *mitm.Config
	caCert                []byte
	caKey                 []byte
	dnsCache              *sync.Map
	lowerHeaders          []string
	http2                 bool
	gmtls                 bool
	gmPrefer              bool
	gmOnly                bool
	forceDisableKeepAlive bool
	findProcessName       bool
	dialer                func(timeout time.Duration, addr string) (net.Conn, error)

	clientCerts []*ClientCertificationPair

	DNSServers     []string
	HostMapping    map[string]string
	via            string
	allowForwarded bool
	// httpTransport            *http.Transport
	proxyUrl                 *url.URL
	proxyUrls                []*url.URL
	hijackedMaxContentLength int

	// transparent hijack mode
	transparentHijackRequestManager *TransparentHijackManager
	transparentHijackMode           *utils.AtomicBool
	transparentHijackRequest        func(isHttps bool, req []byte) []byte
	transparentHijackResponse       func(isHttps bool, rsp []byte) []byte
	transparentOriginMirror         func(isHttps bool, req, rsp []byte)
	transparentHijackedMirror       func(isHttps bool, req, rsp []byte)

	proxyAuth *ProxyAuth

	requestHijackHandler  func(isHttps bool, originReq *http.Request, req []byte) []byte
	responseHijackHandler func(isHttps bool, r *http.Request, rspIns *http.Response, rsp []byte, remoteAddr string) []byte
	httpFlowMirror        func(isHttps bool, r *http.Request, rsp *http.Response, startTs int64)

	// websocket
	websocketHijackMode            *utils.AtomicBool
	forceTextFrame                 *utils.AtomicBool
	websocketRequestHijackHandler  func(req []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte
	websocketResponseHijackHandler func(rsp []byte, r *http.Request, rspIns *http.Response, startTs int64) []byte
	websocketRequestMirror         func(req []byte)
	websocketResponseMirror        func(rsp []byte)

	maxContentLength int
	maxReadWaitTime  time.Duration

	// disable mitm ca cert page
	enableMITMCACertPage bool
	// disable websocket compression
	enableWebsocketCompression *utils.AtomicBool

	// random JA3 fingerprint
	randomJA3 bool

	tunMode bool

	// connection pool for remote server connections
	connPool       *lowhttp.LowHttpConnPool
	connPoolCtx    context.Context
	connPoolCancel context.CancelFunc
}

func (m *MITMServer) GetMaxContentLength() int {
	return m.maxContentLength
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

func (m *MITMServer) GetMartianProxy() *minimartian.Proxy {
	return m.proxy
}

func (m *MITMServer) GetCaCert() []byte {
	return m.caCert
}

func (m *MITMServer) Serve(ctx context.Context, addr string) error {
	return m.ServeWithListenedCallback(ctx, addr, func() {
		log.Info("mitm server started")
	})
}

func (m *MITMServer) initConfig() error {
	if m.mitmConfig == nil {
		return utils.Errorf("mitm config empty")
	}
	// m.proxy.SetDownstreamProxy(m.proxyUrl)
	m.proxy.SetH2(m.http2)
	if m.proxyAuth != nil {
		m.proxy.SetAuth(m.proxyAuth.Username, m.proxyAuth.Password)
	}

	var config []lowhttp.LowhttpOpt

	config = append(config, lowhttp.WithProxyGetter(func() []string {
		if m.proxyUrls == nil {
			return []string{}
		}
		var proxys []string
		for _, proxyUrl := range m.proxyUrls {
			proxys = append(proxys, proxyUrl.String())
		}
		return proxys
	}))

	if len(m.DNSServers) > 0 {
		config = append(config, lowhttp.WithDNSServers(m.DNSServers))
	}
	if len(m.HostMapping) > 0 {
		config = append(config, lowhttp.WithETCHosts(m.HostMapping))
	}
	if m.randomJA3 {
		config = append(config, lowhttp.WithRandomJA3FingerPrint(true))
	}
	if m.GetMaxContentLength() != 0 && m.GetMaxContentLength() < 10*1024*1024 {
		m.proxy.SetMaxContentLength(m.GetMaxContentLength())
	}

	m.proxy.SetMaxReadWaitTime(m.maxReadWaitTime)
	m.proxy.SetLowhttpConfig(config)
	m.proxy.SetGMTLS(m.gmtls)
	m.proxy.SetGMPrefer(m.gmPrefer)
	m.proxy.SetGMOnly(m.gmOnly)
	m.proxy.SetHTTPForceClose(m.forceDisableKeepAlive)
	m.proxy.SetFindProcessName(m.findProcessName)
	m.proxy.SetDialer(m.dialer)

	m.proxy.SetMITM(m.mitmConfig)
	return nil
}

// handleListenError 处理端口监听失败的错误，提供端口建议
func handleListenError(addr string, originalErr error) error {
	host, port, err := utils.ParseStringToHostPort(addr)
	if err != nil {
		return utils.Errorf("listen port: %v failed: %s", addr, originalErr)
	}

	availablePort := utils.FindNearestAvailablePortWithTimeout(host, port, 3*time.Second)
	var suggestionMsg string
	if availablePort == 0 {
		suggestionMsg = "端口被占用，3秒内未找到可用端口，建议尝试重启电脑或使用管理员权限运行"
	} else {
		// 判断是附近端口还是系统分配端口
		if availablePort >= port-10 && availablePort <= port+10 {
			suggestionMsg = fmt.Sprintf("端口被占用，建议尝试附近可用端口：%d", availablePort)
		} else {
			suggestionMsg = fmt.Sprintf("端口被占用，建议尝试系统分配的可用端口：%d", availablePort)
		}
	}
	return utils.Errorf("listen port: %v failed: %s\n\n%s", addr, originalErr, suggestionMsg)
}

func (m *MITMServer) ServeWithListenedCallback(ctx context.Context, addr string, callback func()) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return handleListenError(addr, err)
	}
	defer lis.Close()

	if callback != nil {
		callback()
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = lis.Close()
		}
	}()

	return m.ServerListener(ctx, lis)
}
func (m *MITMServer) ServerListener(ctx context.Context, lis net.Listener) error {
	if err := m.initConfig(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create connection pool with cancellable context
	m.connPoolCtx, m.connPoolCancel = context.WithCancel(ctx)
	m.connPool = lowhttp.NewHttpConnPool(m.connPoolCtx, 100, 2)
	m.proxy.SetConnPool(m.connPool)

	// Clean up connection pool when done
	defer func() {
		if m.connPoolCancel != nil {
			log.Infof("mitm: cancelling connection pool context")
			m.connPoolCancel()
		}
		if m.connPool != nil {
			log.Infof("mitm: clearing connection pool in ServerListener")
			m.connPool.Clear()
		}
	}()

	m.setHijackHandler(ctx)
	err := m.proxy.Serve(lis, ctx)
	if err != nil {
		return utils.Errorf("serve proxy server failed: %s", err)
	}
	return nil
}

var (
	defaultBuiltinDomains = []string{
		"download-mitm-ca.com",
		"download-mitm-cert.yaklang.io",
		"mitm",
		// 某些手机浏览器没办法访问非域名格式的地址，比如 mitm
		"mitm.cert",
	}
	//go:embed static/navtab.html
	// 返回HTML页面内容
	htmlContent []byte
	//go:embed static/*
	staticFS embed.FS
)

func NewMITMServer(options ...MITMConfig) (*MITMServer, error) {
	initMITMCertOnce.Do(InitMITMCert)

	proxy := minimartian.NewProxy()
	server := &MITMServer{
		proxy:                      proxy,
		DNSServers:                 make([]string, 0),
		dnsCache:                   new(sync.Map),
		HostMapping:                make(map[string]string),
		hijackedMaxContentLength:   10 * 1024 * 1024,
		http2:                      false,
		maxContentLength:           10 * 1024 * 1024,
		enableWebsocketCompression: utils.NewAtomicBool(),
		websocketHijackMode:        utils.NewAtomicBool(),
		forceTextFrame:             utils.NewAtomicBool(),
	}
	for _, op := range options {
		err := op(server)
		if err != nil {
			return nil, utils.Errorf("config failed: %s", err)
		}
	}

	// MITM option configured above
	if server.mitmConfig == nil { // currently seems it must be nil since no function is exposed to directly create
		err := MITM_SetCaCertAndPrivKey(defaultCA, defaultKey, defaultGMCA, defaultGMKey)(server)
		if err != nil {
			return nil, utils.Errorf("set ca/key failed: %s", err)
		}
	}

	return server, nil
}
