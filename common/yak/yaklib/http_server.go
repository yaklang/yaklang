package yaklib

import (
	"context"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"net"
	"net/http"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
)

var HttpServeExports = map[string]interface{}{
	"Serve":                _httpServe,
	"tlsCertAndKey":        _httpServerOptCaAndKey,
	"context":              _httpServerOptContext,
	"handler":              _httpServerOptCallback,
	"LocalFileSystemServe": _localFileSystemServe,
}

var (
	HTTPServer_Serve             = _httpServe
	HTTPServer_ServeOpt_Context  = _httpServerOptContext
	HTTPServer_ServeOpt_Callback = _httpServerOptCallback
)

type _httpServerConfig struct {
	tlsConfig *gmtls.Config
	ctx       context.Context
	callback  http.HandlerFunc
}

type HttpServerConfigOpt func(c *_httpServerConfig)

func BuildTlsConfig(crt, key interface{}, cas ...interface{}) *gmtls.Config {
	crtRaw := utils.StringAsFileParams(crt)
	keyRaw := utils.StringAsFileParams(key)
	var caCrts [][]byte
	for _, i := range cas {
		caCrts = append(caCrts, utils.StringAsFileParams(i))
	}
	tlsConfig, err := tlsutils.GetX509MutualAuthClientTlsConfig(crtRaw, keyRaw, caCrts...)
	if err != nil {
		log.Errorf("build tls.Config failed")
		return &gmtls.Config{InsecureSkipVerify: true}
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
		lis, err = gmtls.Listen("tcp", utils.HostPort(host, port), config.tlsConfig)
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
