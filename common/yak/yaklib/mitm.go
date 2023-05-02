package yaklib

import (
	"context"
	"net/http"
	"yaklang.io/yaklang/common/crep"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
)

var (
	MitmExports = map[string]interface{}{
		"Start":  startMitm,
		"Bridge": startBridge,

		"maxContentLength":   mitmMaxContentLength,
		"isTransparent":      mitmConfigIsTransparent,
		"context":            mitmConfigContext,
		"host":               mitmConfigHost,
		"callback":           mitmConfigCallback,
		"hijackHTTPRequest":  mitmConfigHijackHTTPRequest,
		"hijackHTTPResponse": mitmConfigHijackHTTPResponse,
		"wscallback":         mitmConfigWSCallback,
		"wsforcetext":        mitmConfigWSForceTextFrame,
		"rootCA":             mitmConfigCertAndKey,
		"useDefaultCA":       mitmConfigUseDefault,
	}
)

func startMitm(
	port int,
	opts ...mitmConfigOpt,
) error {
	return startBridge(port, "", opts...)
}

type mitmConfig struct {
	ctx                context.Context
	host               string
	callback           func(isHttps bool, urlStr string, r *http.Request, rsp *http.Response)
	wsForceTextFrame   bool
	wscallback         func(data []byte, isRequest bool) interface{}
	mitmCert, mitmPkey []byte
	useDefaultMitmCert bool
	maxContentLength   int

	// 是否开启透明劫持
	isTransparent            bool
	hijackRequest            func(isHttps bool, urlStr string, req []byte, forward func([]byte), reject func())
	hijackWebsocketDataFrame func(isHttps bool, urlStr string, req []byte, forward func([]byte), reject func())
	hijackResponse           func(isHttps bool, urlStr string, req []byte, forward func([]byte), reject func())
}

type mitmConfigOpt func(config *mitmConfig)

func mitmConfigIsTransparent(b bool) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.isTransparent = b
	}
}

func mitmConfigContext(ctx context.Context) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.ctx = ctx
	}
}

func mitmConfigUseDefault(t bool) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.useDefaultMitmCert = t
	}
}

func mitmConfigHost(host string) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.host = host
	}
}

func mitmConfigCallback(f func(bool, string, *http.Request, *http.Response)) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.callback = f
	}
}

func mitmConfigHijackHTTPRequest(h func(isHttps bool, u string, req []byte, modified func([]byte), dropped func())) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.hijackRequest = h
	}
}

func mitmConfigHijackHTTPResponse(h func(isHttps bool, u string, rsp []byte, modified func([]byte), dropped func())) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.hijackResponse = h
	}
}

func mitmConfigWSForceTextFrame(b bool) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.wsForceTextFrame = b
	}
}

func mitmConfigWSCallback(f func([]byte, bool) interface{}) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.wscallback = f
	}
}

func mitmConfigCertAndKey(cert, key []byte) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.mitmCert = cert
		config.mitmPkey = key
	}
}

func mitmMaxContentLength(i int) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.maxContentLength = i
	}
}

func startBridge(
	port interface{},
	downstreamProxy string,
	opts ...mitmConfigOpt,
) error {
	config := &mitmConfig{
		ctx:                context.Background(),
		host:               "",
		callback:           nil,
		mitmCert:           nil,
		mitmPkey:           nil,
		useDefaultMitmCert: true,
		maxContentLength:   10 * 1000 * 1000,
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.host == "" {
		config.host = "127.0.0.1"
	}

	if config.mitmPkey == nil || config.mitmCert == nil {
		if !config.useDefaultMitmCert {
			return utils.Errorf("empty root CA, please use tls to generate or use mitm.useDefaultCA(true) to allow buildin ca.")
		}
		log.Warningf("mitm proxy use the default cert and key")
	}

	if config.isTransparent && downstreamProxy != "" {
		log.Errorf("mitm.Bridge cannot be 'isTransparent'")
	}

	if config.ctx == nil {
		config.ctx = context.Background()
	}

	server, err := crep.NewMITMServer(
		crep.MITM_SetWebsocketHijackMode(true),
		crep.MITM_SetForceTextFrame(config.wsForceTextFrame),
		crep.MITM_SetWebsocketRequestHijackRaw(func(req []byte, r *http.Request, rspIns *http.Response, t int64) []byte {
			var i interface{}
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()

			if config.wscallback != nil {
				i = config.wscallback(req, true)
				req = utils.InterfaceToBytes(i)
			}

			return req
		}),
		crep.MITM_SetWebsocketResponseHijackRaw(func(rsp []byte, r *http.Request, rspIns *http.Response, t int64) []byte {
			var i interface{}
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()

			if config.wscallback != nil {
				i = config.wscallback(rsp, false)
				rsp = utils.InterfaceToBytes(i)
			}

			return rsp
		}),
		crep.MITM_SetHijackedMaxContentLength(config.maxContentLength),
		crep.MITM_SetTransparentHijackMode(config.isTransparent),
		crep.MITM_SetTransparentMirrorHTTP(func(isHttps bool, r *http.Request, rsp *http.Response) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()

			urlIns, err := lowhttp.ExtractURLFromHTTPRequest(r, isHttps)
			if urlIns == nil {
				log.Errorf("parse to url instance failed...")
				return
			}

			if config.callback != nil {
				config.callback(isHttps, urlIns.String(), r, rsp)
				return
			}

			println("RECV request:", urlIns.String())
			println("REQUEST: ")
			raw, err := utils.HttpDumpWithBody(r, false)
			if err != nil {
				println("Parse Request Failed: %s")
			}
			println(string(raw))
			println("RESPONSE: ")
			raw, err = utils.HttpDumpWithBody(rsp, false)
			if err != nil {
				println("Parse Response Failed: %s")
			}
			println(string(raw))
			println("-----------------------------")
		}),
		crep.MITM_SetHTTPResponseMirror(func(isHttps bool, u string, r *http.Request, rsp *http.Response, remoteAddr string) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()

			if config.callback != nil {
				config.callback(isHttps, u, r, rsp)
				return
			}

			urlIns, _ := lowhttp.ExtractURLFromHTTPRequest(r, isHttps)
			if urlIns == nil {
				log.Errorf("parse to url instance failed...")
				return
			}

			println("RECV request:", urlIns.String())
			println("REQUEST: ")
			raw, err := utils.HttpDumpWithBody(r, false)
			if err != nil {
				println("Parse Request Failed: %s")
			}
			println(string(raw))
			println("RESPONSE: ")
			raw, err = utils.HttpDumpWithBody(rsp, false)
			if err != nil {
				println("Parse Response Failed: %s")
			}
			println(string(raw))
			println("-----------------------------")
		}),
		crep.MITM_SetDownstreamProxy(downstreamProxy),
		crep.MITM_SetCaCertAndPrivKey(config.mitmCert, config.mitmPkey),
		crep.MITM_SetHTTPRequestHijackRaw(func(isHttps bool, reqIns *http.Request, req []byte) []byte {
			if config.hijackRequest == nil {
				return req
			}

			if reqIns.Method == "CONNECT" {
				return req
			}

			req = lowhttp.FixHTTPRequestOut(req)
			urlStrIns, _ := lowhttp.ExtractURLFromHTTPRequestRaw(req, isHttps)
			var after = req
			var isDropped = utils.NewBool(false)
			config.hijackRequest(isHttps, urlStrIns.String(), req, func(bytes []byte) {
				after = bytes
			}, func() {
				isDropped.Set()
			})
			if isDropped.IsSet() {
				return nil
			}
			return lowhttp.FixHTTPRequestOut(after)
		}),
		crep.MITM_SetHTTPResponseHijackRaw(func(isHttps bool, req *http.Request, rsp []byte, remoteAddr string) []byte {
			if config.hijackResponse == nil {
				return rsp
			}

			if req.Method == "CONNECT" {
				return rsp
			}

			var fixedResp, _, _ = lowhttp.FixHTTPResponse(rsp)
			if fixedResp == nil {
				fixedResp = rsp
			}
			urlStrIns, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
			if err != nil {
				log.Errorf("extract url from httpRequest failed: %s", err)
			}
			var after = fixedResp
			var isDropped = utils.NewBool(false)
			config.hijackResponse(isHttps, urlStrIns.String(), fixedResp, func(bytes []byte) {
				after = bytes
			}, func() {
				isDropped.IsSet()
			})
			return after
		}),
	)
	if err != nil {
		return utils.Errorf("create mitm server failed: %s", err)
	}
	err = server.Serve(config.ctx, utils.HostPort(config.host, port))
	if err != nil {
		log.Errorf("server mitm failed: %s", err)
		return nil
	}
	return err
}
