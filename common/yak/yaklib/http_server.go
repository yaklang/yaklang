package yaklib

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/glob"
	"github.com/gorilla/websocket"
	"github.com/yaklang/fastgocaptcha"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

var HttpServeExports = map[string]interface{}{
	"Serve":                  _httpServe,
	"tlsCertAndKey":          _httpServerOptCaAndKey,
	"context":                _httpServerOptContext,
	"handler":                _httpServerOptCallback,
	"routeHandler":           _httpServerOptRouteHandler,
	"wsRouteHandler":         _httpServerOptWsRouteHandler,
	"localFileSystemHandler": _httpServerOptLocalFileSystemHandler,
	"LocalFileSystemServe":   _localFileSystemServe,
	"captchaRouteHandler":    _httpServerOptCaptchaRoute,
}

var (
	HTTPServer_Serve             = _httpServe
	HTTPServer_ServeOpt_Context  = _httpServerOptContext
	HTTPServer_ServeOpt_Callback = _httpServerOptCallback
)

type _httpServerConfig struct {
	tlsConfig *tls.Config
	ctx       context.Context

	localFileSystemHandler map[string]http.Handler
	routeHandler           map[string]http.HandlerFunc
	wsRouteHandler         map[string]WebSocketHandler
	callback               http.HandlerFunc

	// _globHandler 用于存储 glob 路由处理器, is auto managed
	_globCacheMutex sync.RWMutex
	_globHandler    map[string]glob.Glob

	captchaManager *fastgocaptcha.FastGoCaptcha
}

func (c *_httpServerConfig) addGlobHandler(route string) {
	c._globCacheMutex.Lock()
	defer c._globCacheMutex.Unlock()

	if c._globHandler == nil {
		c._globHandler = make(map[string]glob.Glob)
	}
	var err error
	c._globHandler[route], err = glob.Compile(route, rune('/'))
	if err != nil {
		log.Errorf("compile glob failed: %s", err)
		return
	}
}

func (c *_httpServerConfig) getGlobHandler(route string) (glob.Glob, bool) {
	c._globCacheMutex.RLock()
	defer c._globCacheMutex.RUnlock()

	handler, ok := c._globHandler[route]
	return handler, ok
}

type HttpServerConfigOpt func(c *_httpServerConfig)

// WebSocketConn 是 WebSocket 连接的包装结构体，继承了 websocket.Conn 的所有方法
type WebSocketConn struct {
	*websocket.Conn
	rawRequest []byte
}

// GetRawRequest 返回原始的 HTTP 请求数据
func (w *WebSocketConn) GetRawRequest() []byte {
	return w.rawRequest
}

// WebSocketHandler 是 WebSocket 连接处理函数类型
type WebSocketHandler func(conn *WebSocketConn)

// localFileSystemHandler 是一个 HTTP 服务器配置选项，用于将某个 URL 前缀映射到本地文件系统目录提供静态文件服务
// 参数:
//   - prefix: URL 访问路径前缀
//   - dir: 本地文件系统目录
//
// 返回值:
//   - 一个 HTTP 服务器配置选项，作为可变参数传入 httpserver.Serve
//
// Example:
// ```
// // 提供本地静态文件服务，依赖网络，此处仅作示意
// err = httpserver.Serve("127.0.0.1", 8888, httpserver.localFileSystemHandler("/static", "/var/www/static"))
// ```
func _httpServerOptLocalFileSystemHandler(prefix, dir string) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		if c.localFileSystemHandler == nil {
			c.localFileSystemHandler = make(map[string]http.Handler)
		}
		c.localFileSystemHandler[prefix] = http.FileServer(http.Dir(dir))
	}
}

// routeHandler 用于设置 HTTP 服务器的路由处理函数，此函数会根据路由路径自动添加前缀 "/"
// 参数:
//   - route: 路由路径
//   - handler: 命中该路由时的处理函数，参数为响应写入器与请求对象
//
// 返回值:
//   - 一个 HTTP 服务器配置选项，作为可变参数传入 httpserver.Serve
//
// Example:
// ```
//
//	err = httpserver.Serve("127.0.0.1", 8888, httpserver.routeHandler("/", func(w http.ResponseWriter, r *http.Request) {
//		w.Write([]byte("Hello world"))
//	}))
//
// ```
func _httpServerOptRouteHandler(route string, handler http.HandlerFunc) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		if c.routeHandler == nil {
			c.routeHandler = make(map[string]http.HandlerFunc)
		}
		var routes = make([]string, 0, 2)
		routes = append(routes, route)
		if !strings.HasSuffix(route, "/") {
			routes = append(routes, route+"/")
		}
		for _, routeHandled := range routes {
			log.Infof("add route handler: %s", routeHandled)
			if strings.HasPrefix(routeHandled, "/") {
				c.addGlobHandler(routeHandled)
				c.routeHandler[routeHandled] = handler
			} else {
				c.addGlobHandler("/" + routeHandled)
				c.routeHandler["/"+routeHandled] = handler
			}
		}
	}
}

// wsRouteHandler 用于设置 HTTP 服务器的 WebSocket 路由处理函数，会自动处理 WebSocket 握手升级，并在连接建立后调用处理函数
// 参数:
//   - route: 路由路径
//   - handler: WebSocket 连接处理函数，参数为已升级的连接对象
//
// 返回值:
//   - 一个 HTTP 服务器配置选项，作为可变参数传入 httpserver.Serve
//
// Example:
// ```
//
//	err = httpserver.Serve("127.0.0.1", 8888, httpserver.wsRouteHandler("/ws", func(conn) {
//		rawReq := conn.GetRawRequest() // 获取原始 HTTP 请求
//		for {
//			messageType, message, err = conn.ReadMessage()
//			if err != nil {
//				return
//			}
//			conn.WriteMessage(messageType, message) // echo back
//		}
//	}))
//
// ```
func _httpServerOptWsRouteHandler(route string, handler WebSocketHandler) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		if c.wsRouteHandler == nil {
			c.wsRouteHandler = make(map[string]WebSocketHandler)
		}

		var routes = make([]string, 0, 2)
		routes = append(routes, route)
		if !strings.HasSuffix(route, "/") {
			routes = append(routes, route+"/")
		}
		for _, routeHandled := range routes {
			log.Infof("add websocket route handler: %s", routeHandled)
			if strings.HasPrefix(routeHandled, "/") {
				c.addGlobHandler(routeHandled)
				c.wsRouteHandler[routeHandled] = handler
			} else {
				c.addGlobHandler("/" + routeHandled)
				c.wsRouteHandler["/"+routeHandled] = handler
			}
		}
	}
}

// captchaRouteHandler 用于设置 HTTP 服务器的验证码保护处理函数，会根据路由路径自动添加前缀 "/"
// 参数:
//   - route: 受验证码保护的路由路径
//   - timeoutSeconds: 验证码有效期（秒），小于等于 0 时使用默认 30 秒
//   - handler: 通过验证码后的处理函数，参数为响应写入器与请求对象
//
// 返回值:
//   - 一个 HTTP 服务器配置选项，作为可变参数传入 httpserver.Serve
//
// Example:
// ```
//
//	err = httpserver.Serve("127.0.0.1", 8888, httpserver.captchaRouteHandler("/captcha", 30, func(w http.ResponseWriter, r *http.Request) {
//		w.Write([]byte("Hello world"))
//	}))
//
// ```
func _httpServerOptCaptchaRoute(route string, timeoutSeconds float64, handler http.HandlerFunc) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		if c.captchaManager == nil {
			var err error
			log.Info("start to init fastgocaptcha")
			c.captchaManager, err = fastgocaptcha.NewFastGoCaptcha()
			if err != nil {
				log.Errorf("new fastgocaptcha failed: %s", err)
				return
			}
			c.captchaManager.SetErrorf(log.Errorf)
			c.captchaManager.SetInfof(log.Infof)
			c.captchaManager.SetWarningf(log.Warnf)
			for _, i := range []string{
				"/fastgocaptcha/resources/*",
				"/fastgocaptcha/*",
				"/fastgocaptcha/session/*",
			} {
				_httpServerOptRouteHandler(i, func(w http.ResponseWriter, r *http.Request) {
					c.captchaManager.Middleware(nil).ServeHTTP(w, r)
				})(c)
			}
		}
		timeout := time.Second * time.Duration(timeoutSeconds)
		if timeoutSeconds <= 0 {
			log.Warnf("timeoutSeconds is less than 0, use default 30 seconds")
			timeout = 30 * time.Second
		}
		log.Infof("add protect matcher with timeout: %s, timeout: %s", route, timeout)
		err := c.captchaManager.AddProtectMatcherWithTimeout(route, timeout)
		if err != nil {
			log.Errorf("add captcha protect matcher failed: %s", err)
		}
		_httpServerOptRouteHandler(route, func(w http.ResponseWriter, r *http.Request) {
			log.Infof("captcha middleware hit: %s", route)
			c.captchaManager.Middleware(handler).ServeHTTP(w, r)
		})(c)
	}
}

func BuildGmTlsConfig(crt, key interface{}, cas ...interface{}) *gmtls.Config {
	crtRaw := utils.StringAsFileParams(crt)
	keyRaw := utils.StringAsFileParams(key)
	var caCrts [][]byte
	for _, i := range cas {
		caCrts = append(caCrts, utils.StringAsFileParams(i))
	}
	tlsConfig, err := tlsutils.GetX509GMMutualAuthClientTlsConfig(crtRaw, keyRaw, caCrts...)
	if err != nil {
		log.Errorf("build tls.Config failed")
		return &gmtls.Config{InsecureSkipVerify: true}
	}
	tlsConfig.InsecureSkipVerify = true
	return tlsConfig
}

func BuildTlsConfig(crt, key interface{}, cas ...interface{}) *tls.Config {
	crtRaw := utils.StringAsFileParams(crt)
	keyRaw := utils.StringAsFileParams(key)
	var caCrts [][]byte
	for _, i := range cas {
		caCrts = append(caCrts, utils.StringAsFileParams(i))
	}
	tlsConfig, err := tlsutils.GetX509MutualAuthClientTlsConfig(crtRaw, keyRaw, caCrts...)
	if err != nil {
		log.Errorf("build tls.Config failed")
		return &tls.Config{InsecureSkipVerify: true}
	}
	tlsConfig.InsecureSkipVerify = true
	return tlsConfig
}

// tlsCertAndKey 用于设置 HTTP 服务器的 TLS 证书和密钥，一般配合 tls 标准库使用
// 参数:
//   - crt: 服务器证书（PEM 内容或文件路径）
//   - key: 服务器私钥（PEM 内容或文件路径）
//   - cas: 可选的 CA 证书，用于双向认证
//
// 返回值:
//   - 一个 HTTP 服务器配置选项，作为可变参数传入 httpserver.Serve
//
// Example:
// ```
// ca, key, err = tls.GenerateRootCA("yaklang.io")
// cert, sKey, err = tls.SignServerCertAndKey(ca, key)
// err = httpserver.Serve("127.0.0.1", 8888, httpserver.tlsCertAndKey(cert, sKey))
// ```
func _httpServerOptCaAndKey(crt, key interface{}, cas ...interface{}) HttpServerConfigOpt {
	config := BuildTlsConfig(crt, key, cas...)
	return func(c *_httpServerConfig) {
		c.tlsConfig = config
	}
}

// context 用于设置 HTTP 服务器的上下文，可通过取消上下文来停止服务
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - 一个 HTTP 服务器配置选项，作为可变参数传入 httpserver.Serve
//
// Example:
// ```
// ctx = context.New()
// err = httpserver.Serve("127.0.0.1", 8888, httpserver.context(ctx))
// ```
func _httpServerOptContext(ctx context.Context) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		c.ctx = ctx
	}
}

// handler 用于设置 HTTP 服务器的默认回调函数，会在每次收到（未命中其他路由的）请求时被调用
// 参数:
//   - cb: 请求处理回调函数，第一个参数为响应写入器，第二个参数为请求对象
//
// 返回值:
//   - 一个 HTTP 服务器配置选项，作为可变参数传入 httpserver.Serve
//
// Example:
// ```
// err = httpserver.Serve("127.0.0.1", 8888, httpserver.handler(func(rspWriter, req) { rspWriter.Write("Hello world") }))
// ```
func _httpServerOptCallback(cb func(rsp http.ResponseWriter, req *http.Request)) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		c.callback = cb
	}
}

// localFileSystemHandler 用于设置 HTTP 服务器的本地文件系统处理函数，第一个参数为访问路径前缀，第二个参数为本地文件系统路径
// Example:
// ```
// err = httpserver.Serve("127.0.0.1", 8888, httpserver.localFileSystemHandler("/static", "/var/www/static"))
// ```
func _localFileSystemHandler(prefix, dir string) http.Handler {
	if prefix != "" {
		return http.StripPrefix(prefix, http.FileServer(http.Dir(dir)))
	}
	return http.FileServer(http.Dir(dir))
}

func _listen(host string, port int, opts ...HttpServerConfigOpt) (lis net.Listener, config *_httpServerConfig, err error) {
	config = &_httpServerConfig{}
	for _, opt := range opts {
		opt(config)
	}

	if config.ctx == nil {
		config.ctx = context.Background()
	}

	if config.tlsConfig != nil {
		lis, err = tls.Listen("tcp", utils.HostPort(host, port), config.tlsConfig)
	} else {
		lis, err = net.Listen("tcp", utils.HostPort(host, port))
	}
	if err != nil {
		return nil, nil, utils.Errorf("listen on %v failed: %s", utils.HostPort(host, port), err)
	}

	return lis, config, nil
}

// Serve 根据给定的 host 和 port 启动一个 HTTP 服务，可接收零个到多个选项函数用于设置上下文、回调函数等
// 参数:
//   - host: 监听主机
//   - port: 监听端口
//   - opts: 可选配置，例如 httpserver.handler、httpserver.routeHandler、httpserver.context
//
// 返回值:
//   - 错误信息，监听失败或服务异常退出时返回非空
//
// Example:
// ```
// err = httpserver.Serve("127.0.0.1", 8888, httpserver.handler(func(rspWriter, req) { rspWriter.Write("Hello world") }))
// ```
func _httpServe(host string, port int, opts ...HttpServerConfigOpt) error {
	lis, config, err := _listen(host, port, opts...)
	if err != nil {
		return err
	}

	go func() {
		select {
		case <-config.ctx.Done():
			_ = lis.Close()
		}
	}()

	return http.Serve(lis, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if config.localFileSystemHandler != nil {
			for globRoute, handler := range config.localFileSystemHandler {
				pathStr := request.URL.Path
				if matched, _ := filepath.Match(globRoute, pathStr); matched {
					if globRoute != "" {
						handler = http.StripPrefix(globRoute, handler)
					}
					handler.ServeHTTP(writer, request)
					return
				} else {
					hasPrefix := strings.HasPrefix(pathStr, globRoute)
					if hasPrefix {
						if globRoute != "" {
							handler = http.StripPrefix(globRoute, handler)
						}
						handler.ServeHTTP(writer, request)
						return
					}
				}
			}
		}

		// WebSocket 路由处理
		if config.wsRouteHandler != nil {
			for route, handler := range config.wsRouteHandler {
				matched := false
				if route == request.URL.Path {
					matched = true
				} else if globHandler, ok := config.getGlobHandler(route); ok {
					if globHandler.Match(request.URL.Path) {
						matched = true
					}
				}

				if matched {
					// 创建 WebSocket upgrader
					upgrader := websocket.Upgrader{
						CheckOrigin: func(r *http.Request) bool {
							return true // 允许所有来源，实际使用时可以根据需要限制
						},
					}

					// 序列化原始 HTTP 请求
					rawRequest, err := utils.DumpHTTPRequest(request, true)
					if err != nil {
						log.Errorf("dump http request failed: %s", err)
						rawRequest = []byte{}
					}

					// 升级 HTTP 连接为 WebSocket 连接
					conn, err := upgrader.Upgrade(writer, request, nil)
					if err != nil {
						log.Errorf("websocket upgrade failed: %s", err)
						return
					}
					defer conn.Close()

					// 创建包装的 WebSocket 连接
					wsConn := &WebSocketConn{
						Conn:       conn,
						rawRequest: rawRequest,
					}

					// 调用用户定义的 WebSocket 处理函数
					handler(wsConn)
					return
				}
			}
		}

		if config.routeHandler != nil {
			for route, handler := range config.routeHandler {
				if route == request.URL.Path {
					//log.Infof("route handler hit exactly: %s", route)
					handler.ServeHTTP(writer, request)
					return
				} else if globHandler, ok := config.getGlobHandler(route); ok {
					//log.Infof("route handler hit glob: %s", route)
					if globHandler.Match(request.URL.Path) {
						handler.ServeHTTP(writer, request)
						return
					}
				}
			}
		}

		if config.callback == nil {
			writer.WriteHeader(404)
		} else {
			if config.captchaManager == nil {
				config.callback(writer, request)
				return
			}

			config.captchaManager.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				config.callback(w, r)
			})).ServeHTTP(writer, request)
		}
	}))
}

// LocalFileSystemServe 根据给定的 host 和 port 启动一个用于访问本地文件系统的 HTTP 静态文件服务
// 参数:
//   - host: 监听主机
//   - port: 监听端口
//   - prefix: 访问路径前缀
//   - localPath: 对外提供服务的本地文件系统目录
//   - opts: 可选配置，例如 httpserver.context
//
// 返回值:
//   - 错误信息，监听失败或服务异常退出时返回非空
//
// Example:
// ```
// err = httpserver.LocalFileSystemServe("127.0.0.1", 8888, "/static", "/var/www/static")
// ```
func _localFileSystemServe(host string, port int, prefix, localPath string, opts ...HttpServerConfigOpt) error {
	lis, config, err := _listen(host, port, opts...)
	if err != nil {
		return err
	}

	go func() {
		select {
		case <-config.ctx.Done():
			_ = lis.Close()
		}
	}()

	return http.Serve(lis, _localFileSystemHandler(prefix, localPath))
}
