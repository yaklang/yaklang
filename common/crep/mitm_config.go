package crep

import (
	"bufio"
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/url"
	"time"

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

func MITM_SetCaCertAndPrivKey(ca []byte, key []byte) MITMConfig {
	return func(server *MITMServer) error {
		if ca == nil || key == nil {
			return MITM_SetCaCertAndPrivKey(defaultCA, defaultKey)(server)
		}

		c, err := tls.X509KeyPair(ca, key)
		if err != nil {
			// if not pem blocks
			// try to parse as der
			caDer, err := x509.ParseCertificate(ca)
			if err != nil {
				return utils.Errorf("parse ca[pem/der] failed: %s", err)
			}
			ca = pem.EncodeToMemory(&pem.Block{Type: `CERTIFICATE`, Bytes: caDer.Raw})
			keyDer, err := x509.ParsePKCS8PrivateKey(key)
			if err != nil {
				log.Warnf("parse key[pem/der] pkcs8 pkey failed: %s", err)
				keyDer, err = x509.ParsePKCS1PrivateKey(key)
				if err != nil {
					return utils.Errorf("parse key[pem/der] pkcs1/pkcs8 pkey failed: %s", err)
				}
				// pkcs1
				key = pem.EncodeToMemory(&pem.Block{Type: `RSA PRIVATE KEY`, Bytes: x509.MarshalPKCS1PrivateKey(keyDer.(*rsa.PrivateKey))})
			} else {
				// pkcs8
				keyRawBytes, err := x509.MarshalPKCS8PrivateKey(keyDer)
				if err != nil {
					return utils.Errorf("marshal key[pem/der] pkcs8 pkey failed: %s", err)
				}
				key = pem.EncodeToMemory(&pem.Block{Type: `PRIVATE KEY`, Bytes: keyRawBytes})
			}
			c, err = tls.X509KeyPair(ca, key)
			if err != nil {
				return utils.Errorf("parse ca and privKey (DER) failed: %s", err)
			}
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
			defaultH2Config.EnableDebugLogs = true // for DEBUG and DEV only
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

func MITM_SetDownstreamProxy(s string) MITMConfig {
	return func(server *MITMServer) error {
		if s == "" {
			server.proxyUrl = nil
			return nil
		}
		urlRaw, err := url.Parse(s)
		if err != nil {
			return utils.Errorf("parse proxy url: %v failed: %s", s, err)
		}
		log.Infof("set downstream proxy as %v", urlRaw.String())
		server.proxyUrl = urlRaw
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
