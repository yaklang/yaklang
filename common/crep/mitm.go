package crep

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	_ "embed"
	"encoding/pem"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/minimartian/mitm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
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

func InitMITMCert() {
	defaultCA, _ = ioutil.ReadFile(defaultCAFile)
	defaultKey, _ = ioutil.ReadFile(defaultKeyFile)

	if defaultCA != nil && defaultKey != nil {
		log.Debug("Successfully load cert and key from default files")
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

	clientCerts []*ClientCertificationPair

	DNSServers     []string
	HostMapping    map[string]string
	via            string
	allowForwarded bool
	// httpTransport            *http.Transport
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
}

func (m *MITMServer) GetMaxContentLength() int {
	if m == nil || m.maxContentLength <= 0 {
		return 10 * 1000 * 1000 // 10MB roughly
	}
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
	if m.mitmConfig == nil {
		return utils.Errorf("mitm config empty")
	}

	//m.proxy.SetDownstreamProxy(m.proxyUrl)
	m.proxy.SetH2(m.http2)
	if m.proxyAuth != nil {
		m.proxy.SetAuth(m.proxyAuth.Username, m.proxyAuth.Password)
	}

	var config []lowhttp.LowhttpOpt

	config = append(config, lowhttp.WithProxyGetter(func() []string {
		if m.proxyUrl == nil {
			return []string{}
		}
		return []string{m.proxyUrl.String()}
	}))

	if len(m.DNSServers) > 0 {
		config = append(config, lowhttp.WithDNSServers(m.DNSServers))
	}
	if len(m.HostMapping) > 0 {
		config = append(config, lowhttp.WithETCHosts(m.HostMapping))
	}

	m.proxy.SetLowhttpConfig(config)
	m.proxy.SetGMTLS(m.gmtls)
	m.proxy.SetGMPrefer(m.gmPrefer)
	m.proxy.SetGMOnly(m.gmOnly)
	m.proxy.SetHTTPForceClose(m.forceDisableKeepAlive)

	m.proxy.SetMITM(m.mitmConfig)
	m.proxy.SetMaxContentLength(m.GetMaxContentLength())

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	m.setHijackHandler(ctx)

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

var (
	defaultBuildinDomains = []string{
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
		proxy:                    proxy,
		DNSServers:               make([]string, 0),
		dnsCache:                 new(sync.Map),
		HostMapping:              make(map[string]string),
		hijackedMaxContentLength: 10 * 1000 * 1000,
		http2:                    false,
		maxContentLength:         10 * 1000 * 1000,
	}
	for _, op := range options {
		err := op(server)
		if err != nil {
			return nil, utils.Errorf("config failed: %s", err)
		}
	}

	// MITM option configured above
	if server.mitmConfig == nil { // currently seems it must be nil since no function is exposed to directly create
		err := MITM_SetCaCertAndPrivKey(defaultCA, defaultKey)(server)
		if err != nil {
			return nil, utils.Errorf("set ca/key failed: %s", err)
		}
	}

	return server, nil
}
