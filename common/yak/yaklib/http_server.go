package yaklib

import (
	"context"
	"crypto/tls"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net"
	"net/http"
)

var HttpServeExports = map[string]interface{}{
	"Serve":                _httpServe,
	"tlsCertAndKey":        _httpServerOptCaAndKey,
	"context":              _httpServerOptContext,
	"handler":              _httpServerOptCallback,
	"LocalFileSystemServe": _localFileSystemServe,
}

var HTTPServer_Serve = _httpServe
var HTTPServer_ServeOpt_Context = _httpServerOptContext
var HTTPServer_ServeOpt_Callback = _httpServerOptCallback

type _httpServerConfig struct {
	tlsConfig *tls.Config
	ctx       context.Context
	callback  http.HandlerFunc
}

type _httpServerConfigOpt func(c *_httpServerConfig)

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

func _httpServerOptCaAndKey(crt, key interface{}, cas ...interface{}) _httpServerConfigOpt {
	config := BuildTlsConfig(crt, key, cas...)
	return func(c *_httpServerConfig) {
		c.tlsConfig = config
	}
}

func _httpServerOptContext(ctx context.Context) _httpServerConfigOpt {
	return func(c *_httpServerConfig) {
		c.ctx = ctx
	}
}

func _httpServerOptCallback(cb func(rsp http.ResponseWriter, req *http.Request)) _httpServerConfigOpt {
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

func _listen(host string, port int, opts ..._httpServerConfigOpt) (lis net.Listener, config *_httpServerConfig, err error) {
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

func _httpServe(host string, port int, opts ..._httpServerConfigOpt) error {
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

func _localFileSystemServe(host string, port int, prefix, localPath string, opts ..._httpServerConfigOpt) error {
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
