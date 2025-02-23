package yaklib

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"path/filepath"
	"strings"

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
	"localFileSystemHandler": _httpServerOptLocalFileSystemHandler,
	"LocalFileSystemServe":   _localFileSystemServe,
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
	callback               http.HandlerFunc
}

type HttpServerConfigOpt func(c *_httpServerConfig)

func _httpServerOptLocalFileSystemHandler(prefix, dir string) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		if c.localFileSystemHandler == nil {
			c.localFileSystemHandler = make(map[string]http.Handler)
		}
		c.localFileSystemHandler[prefix] = http.FileServer(http.Dir(dir))
	}
}

// routeHandler 用于设置 HTTP 服务器的路由处理函数，第一个参数为路由路径，第二个参数为处理函数
// 此函数会根据路由路径自动添加前缀 "/"
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
		if strings.HasPrefix(route, "/") {
			c.routeHandler[route] = handler
		} else {
			c.routeHandler["/"+route] = handler
		}
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

// tlsCertAndKey 用于设置 HTTP服务器的 TLS 证书和密钥，第一个参数为证书，第二个参数为密钥，第三个参数为可选的 CA 证书
// 一般配合tls标准库使用
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

// context 用于设置 HTTP 服务器的上下文
// Example:
// ```
// ctx = context.New()
// err = httpserver.Serve("127.0.0.1", httpserver, http.context(ctx))
// ```
func _httpServerOptContext(ctx context.Context) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		c.ctx = ctx
	}
}

// handler 用于设置 HTTP 服务器的回调函数，此函数会在每次收到请求时被调用
// 此函数的第一个参数为响应回复者结构体，第二个参数为 请求结构体，你可以调用第一个参数中的方法来设置响应头，响应体等
// Example:
// ```
// err = httpserver.Serve("127.0.0.1", 8888, httpserver.handler(func(rspWriter, req) { rspWriter.Write("Hello world") }))
// ```
func _httpServerOptCallback(cb func(rsp http.ResponseWriter, req *http.Request)) HttpServerConfigOpt {
	return func(c *_httpServerConfig) {
		c.callback = cb
	}
}

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

// Serve 根据给定的 host 和 port 启动一个 http 服务，第一个参数为监听主机，第二个参数为监听端口，接下来可以接收零个到多个选项函数，用于设置上下文，回调函数等
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
					handler.ServeHTTP(writer, request)
					return
				} else {
					hasPrefix := strings.HasPrefix(pathStr, globRoute)
					if hasPrefix {
						handler.ServeHTTP(writer, request)
						return
					}
				}
			}
		}

		if config.routeHandler != nil {
			for route, handler := range config.routeHandler {
				if route == request.URL.Path {
					handler.ServeHTTP(writer, request)
					return
				} else if strings.HasPrefix(request.URL.Path, route) {
					handler.ServeHTTP(writer, request)
					return
				} else if utils.MatchAnyOfGlob(request.URL.Path, route) {
					handler.ServeHTTP(writer, request)
					return
				}
			}
		}

		if config.callback == nil {
			_, _ = writer.Write([]byte("not implemented yak http server handler"))
			writer.WriteHeader(200)
		} else {
			config.callback(writer, request)
		}
	}))
}

// LocalFileSystemServe 根据给定的 host 和 port 启动一个 http 服务用于访问本地文件系统
// 第一个参数为监听主机，第二个参数为监听端口，第三个参数为访问路径前缀，第四个参数为本地文件系统路径，接下来可以接收零个到多个选项函数，用于设置上下文，回调函数等
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
