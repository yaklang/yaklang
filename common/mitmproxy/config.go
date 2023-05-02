package mitmproxy

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"time"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/mitmproxy/mitm"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/tlsutils"
)

type Config struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	Ca              []byte        `json:"ca"`
	Key             []byte        `json:"key"`
	TransparentMode bool          `json:"transparent_mode"`
	Timeout         time.Duration `json:"timeout"`
	DownstreamProxy []string      `json:"downstream_proxy"`

	mitmConfig *mitm.Config

	webhookCallback        func(req *http.Request) []byte
	mirrorRequestCallback  func(req *http.Request)
	hijackRequestCallback  func(isHttps bool, req *http.Request, raw []byte) []byte
	hijackResponseCallback func(isHttps bool, req *http.Request, rspRaw []byte, remoteAddr string) []byte
	mirrorResponseCallback func(isHttps bool, req *http.Request, rsp *http.Response, remoteAddr string)
}

type Option func(config *Config)

func WithDownstreamProxy(c ...string) Option {
	return func(config *Config) {
		config.DownstreamProxy = c
	}
}

func WithHijackRequest(c func(isHttps bool, req *http.Request, raw []byte) []byte) Option {
	return func(config *Config) {
		config.hijackRequestCallback = c
	}
}

func WithHijackResponse(c func(isHttps bool, req *http.Request, rspRaw []byte, remoteAddr string) []byte) Option {
	return func(config *Config) {
		config.hijackResponseCallback = c
	}
}

func WithMirrorRequest(cb func(req *http.Request)) Option {
	return func(config *Config) {
		config.mirrorRequestCallback = func(req *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			cb(req)
		}
	}
}

func WithMirrorResponse(cb func(isHttps bool, req *http.Request, rsp *http.Response, remoteAddr string)) Option {
	return func(config *Config) {
		config.mirrorResponseCallback = func(isHttps bool, req *http.Request, rsp *http.Response, remoteAddr string) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			cb(isHttps, req, rsp, remoteAddr)
		}
	}
}

func WithWebHook(cb func(req *http.Request) []byte) Option {
	return func(config *Config) {
		config.webhookCallback = func(req *http.Request) []byte {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			return cb(req)
		}
	}
}

func WithHost(host string) Option {
	return func(config *Config) {
		config.Host = host
	}
}

func WithDefaultTimeout(d float64) Option {
	return func(config *Config) {
		config.Timeout = utils.FloatSecondDuration(d)
	}
}

func WithPort(port int) Option {
	return func(config *Config) {
		config.Port = port
	}
}

func WithCaCert(ca []byte, key []byte) Option {
	return func(config *Config) {
		config.Ca = ca
		config.Key = key
	}
}

func WithAutoCa() Option {
	return func(config *Config) {
		var err error
		config.Ca, config.Key, err = tlsutils.GenerateSelfSignedCertKeyWithCommonName("CA-for-MITM", "", nil, nil)
		if err != nil {
			log.Errorf("generate self signed cert failed: %s", err)
		}
	}
}

func WithTransparentMode(b bool) Option {
	return func(config *Config) {
		config.TransparentMode = b
	}
}

func NewConfig(opts ...Option) (*Config, error) {
	config := &Config{
		Host: "0.0.0.0", Port: 8088,
	}
	for _, opt := range opts {
		opt(config)
	}

	var err error
	if config.Ca == nil || config.Key == nil {
		config.Ca, config.Key, err = GetMITMCACert()
		if err != nil {
			return nil, utils.Errorf("config ca/key mitm failed: %s", err)
		}
	}

	ca, key := config.Ca, config.Key
	if ca == nil || key == nil {
		return nil, utils.Error("empty ca-cert or key...")
	}

	c, err := tls.X509KeyPair(ca, key)
	if err != nil {
		return nil, utils.Errorf("parse ca and privKey failed: %s", err)
	}

	cert, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return nil, utils.Errorf("extract x509 cert failed: %s", err)
	}

	mc, err := mitm.NewConfig(cert, c.PrivateKey)
	if err != nil {
		return nil, utils.Errorf("build private key failed: %s", err)
	}
	mc.SkipTLSVerify(true)
	mc.SetOrganization("MITMServer")
	mc.SetValidity(time.Hour * 24 * 365)
	config.mitmConfig = mc
	return config, nil
}
