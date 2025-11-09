package yaklib

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minimartian"
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
	"gmRootCA":             mitmConfigGMCertAndKey,
	"useDefaultCA":         mitmConfigUseDefault,
	"gmtls":                mitmConfigGMTLS,
	"gmtlsPrefer":          mitmConfigGMTLSPrefer,
	"gmtlsOnly":            mitmConfigGMTLSOnly,
	"randomJA3":            mitmConfigRandomJA3,
	"extraIncomingConn":    mitmConfigExtraIncomingConn,
	"extraIncomingConnEx":  mitmConfigExtraIncomingConnEx,
}

// Start 启动一个 MITM (中间人)代理服务器，它的第一个参数是端口，接下来可以接收零个到多个选项函数，用于影响中间人代理服务器的行为
// 如果没有指定 CA 证书和私钥，那么将使用内置的证书和私钥
// Example:
// ```
// mitm.Start(8080, mitm.host("127.0.0.1"), mitm.callback(func(isHttps, urlStr, req, rsp) { http.dump(req); http.dump(rsp)  })) // 启动一个中间人代理服务器，并将请求和响应打印到标准输出
// ```
func startMitm(
	port int,
	opts ...MitmConfigOpt,
) error {
	return startBridge(port, "", opts...)
}

type mitmConfig struct {
	ctx                    context.Context
	host                   string
	callback               func(isHttps bool, urlStr string, r *http.Request, rsp *http.Response)
	wsForceTextFrame       bool
	wscallback             func(data []byte, isRequest bool) interface{}
	mitmCert, mitmPkey     []byte
	mitmGMCert, mitmGMPKey []byte
	useDefaultMitmCert     bool
	maxContentLength       int
	gmtls                  bool
	gmtlsPrefer            bool
	gmtlsOnly              bool
	randomJA3              bool
	dialer                 func(timeout time.Duration, target string) (net.Conn, error)
	tunMode                bool

	// 是否开启透明劫持
	isTransparent            bool
	hijackRequest            func(isHttps bool, urlStr string, req []byte, forward func([]byte), reject func())
	hijackWebsocketDataFrame func(isHttps bool, urlStr string, req []byte, forward func([]byte), reject func())
	hijackResponse           func(isHttps bool, urlStr string, rsp []byte, forward func([]byte), reject func())
	hijackResponseEx         func(isHttps bool, urlStr string, req, rsp []byte, forward func([]byte), reject func())

	// extra incoming connection channels (legacy, for backward compatibility)
	extraIncomingConnChans []chan net.Conn
	// extra incoming connection channels with wrapperedConn
	extraIncomingConnChansEx []chan *minimartian.WrapperedConn
}

type MitmConfigOpt func(config *mitmConfig)

// isTransparent 是一个选项函数，用于指定中间人代理服务器是否开启透明劫持模式，默认为false
// 在开启透明模式下，所有流量都会被默认转发，所有的回调函数都会被忽略
// Example:
// ```
// mitm.Start(8080, mitm.isTransparent(true))
// ```
func mitmConfigIsTransparent(b bool) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.isTransparent = b
	}
}

// gmtls 是一个选项参数，用于指定中间人代理服务器是否开启 GMTLS 劫持模式，默认为false
// 在开启 GMTLS 劫持模式下，中间人代理服务器会劫持所有的 GMTLS 流量
// Example:
// ```
// mitm.Start(8080, mitm.gmtls(true))
// ```
func mitmConfigGMTLS(b bool) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.gmtls = b
	}
}

// gmtlsPrefer 是一个选项参数，用于指定中间人代理服务器是否优先使用 GMTLS 劫持模式，默认为false
// 在开启 GMTLS 劫持模式下，中间人代理服务器会优先使用 GMTLS 劫持模式
// Example:
// ```
// mitm.Start(8080, mitm.gmtlsPrefer(true))
// ```
func mitmConfigGMTLSPrefer(b bool) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.gmtlsPrefer = b
	}
}

// gmtlsOnly 是一个选项参数，用于指定中间人代理服务器是否只使用 GMTLS 劫持模式，默认为false
// 在开启 GMTLS 劫持模式下，中间人代理服务器只会使用 GMTLS 劫持模式
// Example:
// ```
// mitm.Start(8080, mitm.gmtlsOnly(true))
// ```
func mitmConfigGMTLSOnly(b bool) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.gmtlsOnly = b
	}
}

var MitmConfigContext = mitmConfigContext

// context 是一个选项函数，用于指定中间人代理服务器的上下文
// Example:
// ```
// mitm.Start(8080, mitm.context(context.Background()))
// ```
func mitmConfigContext(ctx context.Context) MitmConfigOpt {
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
func mitmConfigUseDefault(t bool) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.useDefaultMitmCert = t
	}
}

// host 是一个选项函数，用于指定中间人代理服务器的监听地址，默认为空，即监听所有网卡
// Example:
// ```
// mitm.Start(8080, mitm.host("127.0.0.1"))
// ```
func mitmConfigHost(host string) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.host = host
	}
}

// callback 是一个选项函数，用于指定中间人代理服务器的回调函数，当接收到请求和响应后，会调用该回调函数
// Example:
// ```
// mitm.Start(8080, mitm.callback(func(isHttps, urlStr, req, rsp) { http.dump(req); http.dump(rsp)  }))
// ```
func mitmConfigCallback(f func(bool, string, *http.Request, *http.Response)) MitmConfigOpt {
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
func mitmConfigHijackHTTPRequest(h func(isHttps bool, u string, req []byte, modified func([]byte), dropped func())) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.hijackRequest = h
	}
}

var MITMConfigHijackHTTPResponse = mitmConfigHijackHTTPResponse

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
func mitmConfigHijackHTTPResponse(h func(isHttps bool, u string, rsp []byte, modified func([]byte), dropped func())) MitmConfigOpt {
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
func mitmConfigHijackHTTPResponseEx(h func(bool, string, []byte, []byte, func([]byte), func())) MitmConfigOpt {
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
func mitmConfigWSForceTextFrame(b bool) MitmConfigOpt {
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
func mitmConfigWSCallback(f func([]byte, bool) interface{}) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.wscallback = f
	}
}

// rootCA 是一个选项函数，用于指定中间人代理服务器的根证书和私钥
// Example:
// ```
// mitm.Start(8080, mitm.rootCA(cert, key))
// ```
func mitmConfigCertAndKey(cert, key []byte) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.mitmCert = cert
		config.mitmPkey = key
	}
}

// gmRootCA 是一个选项函数，用于指定中间人代理服务器的国密根证书和私钥
// Example:
// ```
// mitm.Start(8080, mitm.gmRootCA(cert, key))
// ```
func mitmConfigGMCertAndKey(cert, key []byte) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.mitmGMCert = cert
		config.mitmGMPKey = key
	}
}

// maxContentLength 是一个选项函数，用于指定中间人代理服务器的最大的请求和响应内容长度，默认为10MB
// Example:
// ```
// mitm.Start(8080, mitm.maxContentLength(100 * 1000 * 1000))
// ```
func mitmMaxContentLength(i int) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.maxContentLength = i
	}
}

// randomJA3 是一个选项函数，用于指定中间人代理服务器是否开启随机 JA3 劫持模式，默认为false
// Example:
// ```
// mitm.Start(8080, mitm.randomJA3(true))
// ```
func mitmConfigRandomJA3(b bool) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.randomJA3 = b
	}
}

// extraIncomingConn 是一个选项函数，用于指定中间人代理服务器接受外部传入的连接通道
// 通过该选项，可以将外部的 net.Conn 连接注入到 MITM 服务器中进行劫持处理
// Example:
// ```
// connChan = make(chan net.Conn)
// mitm.Start(8080, mitm.extraIncomingConn(connChan))
// ```
func mitmConfigExtraIncomingConn(ch interface{}) MitmConfigOpt {
	return func(config *mitmConfig) {
		// Handle both chan net.Conn and chan interface{} (from Yak scripts)
		switch c := ch.(type) {
		case chan net.Conn:
			config.extraIncomingConnChans = append(config.extraIncomingConnChans, c)
		case chan interface{}:
			// Create a converter goroutine for Yak script channels
			convertedChan := make(chan net.Conn)
			go func() {
				for v := range c {
					if conn, ok := v.(net.Conn); ok {
						convertedChan <- conn
					} else {
						log.Errorf("extraIncomingConn: received non-net.Conn value: %T", v)
					}
				}
				close(convertedChan)
			}()
			config.extraIncomingConnChans = append(config.extraIncomingConnChans, convertedChan)
		default:
			log.Errorf("extraIncomingConn: unsupported channel type: %T", ch)
		}
	}
}

// extraIncomingConnEx 是一个选项函数，用于指定中间人代理服务器接受外部传入的连接通道（增强版）
// 支持强主机模式和元数据信息
// Example:
// ```
// connChan = make(chan net.Conn)
// mitm.Start(8080, mitm.extraIncomingConnEx(true, connChan))  // 启用强主机模式
// mitm.Start(8080, mitm.extraIncomingConnEx(true, connChan, {"key": "value"}))  // 启用强主机模式并设置元数据
// ```
func mitmConfigExtraIncomingConnEx(mode interface{}, ch interface{}, kv ...interface{}) MitmConfigOpt {
	return func(config *mitmConfig) {
		// Convert mode to bool
		isStrongHostMode := false
		if mode != nil {
			isStrongHostMode = utils.InterfaceToBoolean(mode)
		}

		// Convert kv to map
		metaInfo := make(map[string]interface{})
		if len(kv) > 0 && kv[0] != nil {
			metaInfo = utils.InterfaceToGeneralMap(kv[0])
		}

		// Handle different channel types
		switch c := ch.(type) {
		case chan net.Conn:
			// Convert chan net.Conn to chan *wrapperedConn
			wrappedChan := make(chan *minimartian.WrapperedConn)
			go func() {
				defer close(wrappedChan)
				for conn := range c {
					wrapped := minimartian.NewWrapperedConn(conn, isStrongHostMode, metaInfo)
					wrappedChan <- wrapped
				}
			}()
			config.extraIncomingConnChansEx = append(config.extraIncomingConnChansEx, wrappedChan)
		case chan interface{}:
			// Create a converter goroutine for Yak script channels
			wrappedChan := make(chan *minimartian.WrapperedConn)
			go func() {
				defer close(wrappedChan)
				for v := range c {
					if conn, ok := v.(net.Conn); ok {
						wrapped := minimartian.NewWrapperedConn(conn, isStrongHostMode, metaInfo)
						wrappedChan <- wrapped
					} else {
						log.Errorf("extraIncomingConnEx: received non-net.Conn value: %T", v)
					}
				}
			}()
			config.extraIncomingConnChansEx = append(config.extraIncomingConnChansEx, wrappedChan)
		case chan *minimartian.WrapperedConn:
			// If already a wrapperedConn channel, merge metaInfo into each connection
			wrappedChan := make(chan *minimartian.WrapperedConn)
			go func() {
				defer close(wrappedChan)
				for wrapped := range c {
					if len(metaInfo) > 0 {
						wrapped.MergeMetaInfo(metaInfo)
					}
					if isStrongHostMode {
						// Note: we can't change strongHostMode after creation, so we need to create a new one
						newWrapped := minimartian.NewWrapperedConn(wrapped.Conn, isStrongHostMode, wrapped.GetMetaInfo())
						newWrapped.MergeMetaInfo(metaInfo)
						wrappedChan <- newWrapped
					} else {
						wrappedChan <- wrapped
					}
				}
			}()
			config.extraIncomingConnChansEx = append(config.extraIncomingConnChansEx, wrappedChan)
		default:
			log.Errorf("extraIncomingConnEx: unsupported channel type: %T", ch)
		}
	}
}

var MITMConfigTunMode = mitmConfigTunMode

// set tunmode ,not process proxy proto
func mitmConfigTunMode(b bool) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.tunMode = b
	}
}

var MITMConfigDialer = mitmConfigDialer

// setDialer for proxy
func mitmConfigDialer(dialer func(timeout time.Duration, target string) (net.Conn, error)) MitmConfigOpt {
	return func(config *mitmConfig) {
		config.dialer = dialer
	}
}

// NewMITMServer just new mitm server
func NewMITMServer(
	opts ...MitmConfigOpt,
) (*crep.MITMServer, error) {
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
	server, err := initMitmServer(nil, config)
	if err != nil {
		return nil, utils.Errorf("create mitm server failed: %s", err)
	}
	return server, nil
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
	opts ...MitmConfigOpt,
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
	downstreamProxys := strings.Split(downstreamProxy, ",")
	server, err := initMitmServer(downstreamProxys, config)
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

func initMitmServer(downstreamProxy []string, config *mitmConfig) (*crep.MITMServer, error) {

	if config.host == "" {
		config.host = "127.0.0.1"
	}

	if config.mitmPkey == nil || config.mitmCert == nil {
		if !config.useDefaultMitmCert {
			return nil, utils.Errorf("empty root CA, please use tls to generate or use mitm.useDefaultCA(true) to allow buildin ca.")
		}
		log.Infof("mitm proxy use the default cert and key")
	}

	if config.isTransparent && downstreamProxy != nil {
		log.Errorf("mitm.Bridge cannot be 'isTransparent'")
	}

	if config.ctx == nil {
		config.ctx = context.Background()
	}

	var mitmOpts []crep.MITMConfig
	mitmOpts = append(mitmOpts,
		crep.MITM_SetDialer(config.dialer),
		crep.MITM_SetTunMode(config.tunMode),
		crep.MITM_SetGM(config.gmtls),
		crep.MITM_SetGMPrefer(config.gmtlsPrefer),
		crep.MITM_SetGMOnly(config.gmtlsOnly),
		crep.MITM_RandomJA3(config.randomJA3),
	)

	// Add extra incoming connection channels (legacy)
	for _, ch := range config.extraIncomingConnChans {
		mitmOpts = append(mitmOpts, crep.MITM_SetExtraIncomingConectionChannelLegacy(ch))
	}
	// Add extra incoming connection channels (new with wrapperedConn)
	for _, ch := range config.extraIncomingConnChansEx {
		mitmOpts = append(mitmOpts, crep.MITM_SetExtraIncomingConectionChannel(ch))
	}

	mitmOpts = append(mitmOpts,
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

			fmt.Println("RECV request:", urlIns.String())
			fmt.Println("REQUEST: ")
			raw, err := utils.HttpDumpWithBody(r, false)
			if err != nil {
				log.Errorf("Parse Request Failed: %v", err)
			}
			fmt.Println(string(raw))
			fmt.Println("RESPONSE: ")
			raw, err = utils.HttpDumpWithBody(rsp, false)
			if err != nil {
				log.Errorf("Parse Request Failed: %v", err)
			}
			fmt.Println(string(raw))
			fmt.Println("-----------------------------")
		}),
		crep.MITM_SetDownstreamProxy(downstreamProxy...),
		crep.MITM_SetCaCertAndPrivKey(config.mitmCert, config.mitmPkey, config.mitmGMCert, config.mitmGMPKey),
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
				after = lowhttp.FixHTTPRequest(bytes)
				httpctx.SetRequestModified(reqIns, "user")
				httpctx.SetHijackedRequestBytes(reqIns, after)
			}, func() {
				isDropped.Set()
			})
			if isDropped.IsSet() {
				httpctx.SetContextValueInfoFromRequest(reqIns, httpctx.REQUEST_CONTEXT_KEY_IsDropped, true)
				return nil
			}

			return after
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
					after = lowhttp.FixHTTPRequest(bytes)
					httpctx.SetResponseModified(req, "user")
					httpctx.SetHijackedResponseBytes(req, after)
				}, func() {
					isDropped.Set()
				})
			}
			if isDropped.IsSet() {
				httpctx.SetContextValueInfoFromRequest(req, httpctx.RESPONSE_CONTEXT_KEY_IsDropped, true)
				return nil
			}
			return after
		}),
	)

	return crep.NewMITMServer(mitmOpts...)
}
