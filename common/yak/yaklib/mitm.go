package yaklib

import (
	"context"
	"net/http"

	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

var MitmExports = map[string]interface{}{
	"Start":  startMitm,
	"Bridge": startBridge,

	"maxContentLength":     mitmMaxContentLength,
	"isTransparent":        mitmConfigIsTransparent,
	"context":              mitmConfigContext,
	"host":                 mitmConfigHost,
	"callback":             mitmConfigCallback,
	"hijackHTTPRequest":    mitmConfigHijackHTTPRequest,
	"hijackHTTPResponse":   mitmConfigHijackHTTPResponse,
	"hijackHTTPResponseEx": mitmConfigHijackHTTPResponseEx,
	"wscallback":           mitmConfigWSCallback,
	"wsforcetext":          mitmConfigWSForceTextFrame,
	"rootCA":               mitmConfigCertAndKey,
	"useDefaultCA":         mitmConfigUseDefault,
}

// Start 启动一个 MITM (中间人)代理服务器，它的第一个参数是端口，接下来可以接收零个到多个选项函数，用于影响中间人代理服务器的行为
// 如果没有指定 CA 证书和私钥，那么将使用内置的证书和私钥
// Example:
// ```
// mitm.Start(8080, mitm.host("127.0.0.1"), mitm.callback(func(isHttps, urlStr, req, rsp) { http.dump(req); http.dump(rsp)  })) // 启动一个中间人代理服务器，并将请求和响应打印到标准输出
// ```
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
	hijackResponse           func(isHttps bool, urlStr string, rsp []byte, forward func([]byte), reject func())
	hijackResponseEx         func(isHttps bool, urlStr string, req, rsp []byte, forward func([]byte), reject func())
}

type mitmConfigOpt func(config *mitmConfig)

// isTransparent 是一个选项函数，用于指定中间人代理服务器是否开启透明劫持模式，默认为false
// 在开启透明模式下，所有流量都会被默认转发，所有的回调函数都会被忽略
// Example:
// ```
// mitm.Start(8080, mitm.isTransparent(true))
// ```
func mitmConfigIsTransparent(b bool) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.isTransparent = b
	}
}

// context 是一个选项函数，用于指定中间人代理服务器的上下文
// Example:
// ```
// mitm.Start(8080, mitm.context(context.Background()))
// ```
func mitmConfigContext(ctx context.Context) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.ctx = ctx
	}
}

// useDefaultCA 是一个选项函数，用于指定中间人代理服务器是否使用内置的证书和私钥，默认为true
// 默认的证书与私钥路径：~/yakit-projects/yak-mitm-ca.crt 和 ~/yakit-projects/yak-mitm-ca.key
// Example:
// ```
// mitm.Start(8080, mitm.useDefaultCA(true))
// ```
func mitmConfigUseDefault(t bool) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.useDefaultMitmCert = t
	}
}

// host 是一个选项函数，用于指定中间人代理服务器的监听地址，默认为空，即监听所有网卡
// Example:
// ```
// mitm.Start(8080, mitm.host("127.0.0.1"))
// ```
func mitmConfigHost(host string) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.host = host
	}
}

// callback 是一个选项函数，用于指定中间人代理服务器的回调函数，当接收到请求和响应后，会调用该回调函数
// Example:
// ```
// mitm.Start(8080, mitm.callback(func(isHttps, urlStr, req, rsp) { http.dump(req); http.dump(rsp)  }))
// ```
func mitmConfigCallback(f func(bool, string, *http.Request, *http.Response)) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.callback = f
	}
}

// hijackHTTPRequest 是一个选项函数，用于指定中间人代理服务器的请求劫持函数，当接收到请求后，会调用该回调函数
// 通过调用该回调函数的第四个参数，可以修改请求内容，通过调用该回调函数的第五个参数，可以丢弃请求
// Example:
// ```
// mitm.Start(8080, mitm.hijackHTTPRequest(func(isHttps, urlStr, req, modified, dropped) {
// // 添加一个额外的请求头
// req = poc.ReplaceHTTPPacketHeader(req, "AAA", "BBB")
// modified(req)
// }
// ))
// ```
func mitmConfigHijackHTTPRequest(h func(isHttps bool, u string, req []byte, modified func([]byte), dropped func())) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.hijackRequest = h
	}
}

// hijackHTTPResponse 是一个选项函数，用于指定中间人代理服务器的响应劫持函数，当接收到响应后，会调用该回调函数
// 通过调用该回调函数的第四个参数，可以修改响应内容，通过调用该回调函数的第五个参数，可以丢弃响应
// Example:
// ```
// mitm.Start(8080, mitm.hijackHTTPResponse(func(isHttps, urlStr, rsp, modified, dropped) {
// // 修改响应体为hijacked
// rsp = poc.ReplaceBody(rsp, b"hijacked", false)
// modified(rsp)
// }
// ))
// ```
func mitmConfigHijackHTTPResponse(h func(isHttps bool, u string, rsp []byte, modified func([]byte), dropped func())) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.hijackResponse = h
	}
}

// hijackHTTPResponseEx 是一个选项函数，用于指定中间人代理服务器的响应劫持函数，当接收到响应后，会调用该回调函数
// 通过调用该回调函数的第五个参数，可以修改响应内容，通过调用该回调函数的第六个参数，可以丢弃响应
// 它与 hijackHTTPResponse 的区别在于，它可以获取到原始请求报文
// Example:
// ```
// mitm.Start(8080, mitm.hijackHTTPResponseEx(func(isHttps, urlStr, req, rsp, modified, dropped) {
// // 修改响应体为hijacked
// rsp = poc.ReplaceBody(rsp, b"hijacked", false)
// modified(rsp)
// }
// ))
// ```
func mitmConfigHijackHTTPResponseEx(h func(bool, string, []byte, []byte, func([]byte), func())) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.hijackResponseEx = h
	}
}

// wsforcetext 是一个选项函数，用于强制指定中间人代理服务器的 websocket 劫持的数据帧转换为文本帧，默认为false
// ! 已弃用
// Example:
// ```
// mitm.Start(8080, mitm.wsforcetext(true))
// ```
func mitmConfigWSForceTextFrame(b bool) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.wsForceTextFrame = b
	}
}

// wscallback 是一个选项函数，用于指定中间人代理服务器的 websocket 劫持函数，当接收到 websocket 请求或响应后，会调用该回调函数
// 该回调函数的第一个参数是请求或响应的内容
// 第二个参数是一个布尔值，用于指示该内容是请求还是响应，true 表示请求，false 表示响应
// 通过该回调函数的返回值，可以修改请求或响应的内容
// Example:
// ```
// mitm.Start(8080, mitm.wscallback(func(data, isRequest) { println(data); return data }))
// ```
func mitmConfigWSCallback(f func([]byte, bool) interface{}) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.wscallback = f
	}
}

// rootCA 是一个选项函数，用于指定中间人代理服务器的根证书和私钥
// Example:
// ```
// mitm.Start(8080, mitm.rootCA(cert, key))
// ```
func mitmConfigCertAndKey(cert, key []byte) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.mitmCert = cert
		config.mitmPkey = key
	}
}

// maxContentLength 是一个选项函数，用于指定中间人代理服务器的最大的请求和响应内容长度，默认为10MB
// Example:
// ```
// mitm.Start(8080, mitm.maxContentLength(100 * 1000 * 1000))
// ```
func mitmMaxContentLength(i int) mitmConfigOpt {
	return func(config *mitmConfig) {
		config.maxContentLength = i
	}
}

// Bridge 启动一个 MITM (中间人)代理服务器，它的第一个参数是端口，第二个参数是下游代理服务器地址，接下来可以接收零个到多个选项函数，用于影响中间人代理服务器的行为
// Bridge 与 Start 类似，但略有不同，Bridge可以指定下游代理服务器地址，同时默认会在接收到请求和响应时打印到标准输出
// 如果没有指定 CA 证书和私钥，那么将使用内置的证书和私钥
// Example:
// ```
// mitm.Bridge(8080, "", mitm.host("127.0.0.1"), mitm.callback(func(isHttps, urlStr, req, rsp) { http.dump(req); http.dump(rsp)  })) // 启动一个中间人代理服务器，并将请求和响应打印到标准输出
// ```
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
		log.Infof("mitm proxy use the default cert and key")
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

			req = lowhttp.FixHTTPRequest(req)
			urlStrIns, _ := lowhttp.ExtractURLFromHTTPRequestRaw(req, isHttps)
			after := req
			isDropped := utils.NewBool(false)
			config.hijackRequest(isHttps, urlStrIns.String(), req, func(bytes []byte) {
				after = bytes
			}, func() {
				isDropped.Set()
			})
			if isDropped.IsSet() {
				return nil
			}
			return lowhttp.FixHTTPRequest(after)
		}),
		crep.MITM_SetHTTPResponseHijackRaw(func(isHttps bool, req *http.Request, rspInstance *http.Response, rsp []byte, remoteAddr string) []byte {
			if config.hijackResponse == nil && config.hijackResponseEx == nil {
				return rsp
			}

			if req.Method == "CONNECT" {
				return rsp
			}

			fixedResp, _, _ := lowhttp.FixHTTPResponse(rsp)
			if fixedResp == nil {
				fixedResp = rsp
			}
			urlStrIns, err := lowhttp.ExtractURLFromHTTPRequest(req, isHttps)
			if err != nil {
				log.Errorf("extract url from httpRequest failed: %s", err)
			}
			after := fixedResp
			isDropped := utils.NewBool(false)
			if config.hijackResponse != nil {
				config.hijackResponse(isHttps, urlStrIns.String(), fixedResp, func(bytes []byte) {
					after = bytes
				}, func() {
					isDropped.Set()
				})
			}

			if config.hijackResponseEx != nil {
				reqRaw := httpctx.GetRequestBytes(req)
				if reqRaw != nil {
					reqRaw, _ = utils.HttpDumpWithBody(req, true)
					if reqRaw != nil && !httpctx.GetContextBoolInfoFromRequest(req, httpctx.REQUEST_CONTEXT_KEY_RequestIsStrippedGzip) {
						reqRaw = lowhttp.DeletePacketEncoding(reqRaw)
					}
				}
				config.hijackResponseEx(isHttps, urlStrIns.String(), reqRaw, fixedResp, func(bytes []byte) {
					after = bytes
				}, func() {
					isDropped.Set()
				})
			}
			if isDropped.IsSet() {
				return nil
			}

			return after
		}),
	)
	if err != nil {
		return utils.Errorf("create mitm server failed: %s", err)
	}
	err = server.Serve(config.ctx, utils.HostPort(config.host, port))
	if err != nil {
		log.Errorf("server mitm failed: %s", err)
		return err
	}
	return nil
}
