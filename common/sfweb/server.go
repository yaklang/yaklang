package sfweb

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	_ "github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"

	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ServerConfig struct {
	Host               string
	ChatGLMAPIKey      string
	Port               int
	Debug              bool
	Https              bool
	ServerCrtPath      string
	ServerKeyPath      string
	WebSocketRateLimit time.Duration
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		WebSocketRateLimit: 10 * time.Millisecond,
	}
}

type ServerOpt func(*ServerConfig)

func WithHost(host string) ServerOpt {
	return func(c *ServerConfig) {
		c.Host = host
	}
}

func WithChatGLMAPIKey(apiKey string) ServerOpt {
	return func(c *ServerConfig) {
		c.ChatGLMAPIKey = apiKey
	}
}

func WithPort(port int) ServerOpt {
	return func(c *ServerConfig) {
		c.Port = port
	}
}

func WithDebug(debug bool) ServerOpt {
	return func(c *ServerConfig) {
		c.Debug = debug
	}
}

func WithHttps(https bool) ServerOpt {
	return func(c *ServerConfig) {
		c.Https = https
	}
}

func WithServerCrtPath(p string) ServerOpt {
	return func(c *ServerConfig) {
		c.ServerCrtPath = p
	}
}

func WithServerKeyPath(p string) ServerOpt {
	return func(c *ServerConfig) {
		c.ServerKeyPath = p
	}
}

type SyntaxFlowWebServer struct {
	grpcClient ypb.YakClient
	db         *gorm.DB
	router     *mux.Router
	config     *ServerConfig
}

func (s *SyntaxFlowWebServer) init() {
	s.registerTemplateRoute()
	s.registerYakURLRoute()
	s.registerScanRoute()
	s.registerAIAnalysisRoute()
	s.registerReportRoute()
}

func NewSyntaxFlowWebServer(ctx context.Context, opts ...ServerOpt) (string, error) {
	serverCfg := NewServerConfig()
	for _, opt := range opts {
		opt(serverCfg)
	}

	router := mux.NewRouter()
	router.Use(func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requestRaw, _ := utils.HttpDumpWithBody(request, true)
			if string(requestRaw) != "" {
				if len(requestRaw) > 4000 {
					header, _ := lowhttp.SplitHTTPPacketFast(requestRaw)
					requestRaw = []byte(header)
				}
			}
			var statusCodeWriter *StatusCodeResponseWriter
			var debugWriter *LogHTTPResponseWriter
			if serverCfg.Debug {
				SfWebLogger.Infof("Request:\n%s", requestRaw)
				debugWriter = NewLogHTTPResponseWriter(writer)
				writer = debugWriter
			} else {
				statusCodeWriter = NewStatusCodeResponseWriter(writer)
				writer = statusCodeWriter
			}

			writer.Header().Set("Access-Control-Allow-Origin", "*")
			writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			writer.Header().Set("Access-Control-Allow-Credentials", "true")
			writer.Header().Set("Access-Control-Max-Age", "86400")
			if request.Method == http.MethodOptions {
				writer.Header().Set("Access-Control-Allow-Headers", request.Header.Get("Access-Control-Request-Headers"))
				writer.Header().Set("Allow", "POST, GET, OPTIONS")
				writer.WriteHeader(http.StatusOK)
				return
			}

			handler.ServeHTTP(writer, request)
			if serverCfg.Debug {
				SfWebLogger.Debugf("Response:\n%s", debugWriter.Raw())
			} else {
				u := request.URL
				var (
					path       string
					query      string
					statusCode int = statusCodeWriter.StatusCode
				)
				if u != nil {
					if u.RawPath != "" {
						path = u.RawPath
					} else {
						path = u.Path
					}
					if u.RawQuery != "" {
						query = "?" + u.RawQuery
					} else {
						query = u.RawQuery
					}
				} else {
					path = "/!unknown_path"
				}
				if statusCode == 200 {
					SfWebLogger.Infof("[%d] %s%s", statusCode, path, query)
				} else {
					SfWebLogger.Errorf("[%d] %s%s", statusCode, path, query)
				}
			}
		})
	})

	server := &SyntaxFlowWebServer{router: router, config: serverCfg}
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		return "", err
	}
	server.grpcClient = client

	// route
	server.init()

	if serverCfg.Port <= 0 {
		serverCfg.Port = utils.GetRandomAvailableTCPPort()
	}
	addr := utils.HostPort(serverCfg.Host, serverCfg.Port)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	go func() {
		select {
		case <-ctx.Done():
			lis.Close()
		}
	}()
	dealTls := make(chan bool)

	go func() {
		crep.InitMITMCert()
		if !serverCfg.Https {
			dealTls <- false
			SfWebLogger.Info("start to load no tls config")
			err := http.Serve(lis, router)
			if err != nil {
				SfWebLogger.Error(err)
			}
			return
		}

		dealTls <- true
		SfWebLogger.Info("start to load tls config")
		var (
			server           *http.Server
			config           *tls.Config
			crtPath, keyPath = serverCfg.ServerCrtPath, serverCfg.ServerKeyPath
		)
		if serverCfg.ServerCrtPath == "" || serverCfg.ServerKeyPath == "" {
			crtPath, keyPath = "", ""
			ca, key, _ := crep.GetDefaultCaAndKey()
			crt, serverKey, _ := tlsutils.SignServerCrtNKeyWithParams(ca, key, "127.0.0.1", time.Now().Add(time.Hour*24*180), false)
			config, err = tlsutils.GetX509ServerTlsConfig(ca, crt, serverKey)
			if err != nil {
				SfWebLogger.Error(err)
				return
			}
		} else {
			config = &tls.Config{}
		}
		server = &http.Server{Handler: router}
		config.MinVersion = tls.VersionSSL30
		config.MaxVersion = tls.VersionTLS13
		server.TLSConfig = config
		err = server.ServeTLS(lis, crtPath, keyPath)
		if err != nil {
			SfWebLogger.Error(err)
			return
		}
	}()
	proto := "http"
	if <-dealTls {
		proto = "https"
	}
	time.Sleep(time.Second)
	url := fmt.Sprintf("%s://%s", proto, addr)
	SfWebLogger.Infof("start syntaxflow web server on: %v", url)
	return url, nil
}
