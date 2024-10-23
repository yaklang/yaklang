package sfweb

import (
	"context"
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
	Host          string
	ChatGLMAPIKey string
	Port          int
	Debug         bool
	Https         bool
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{}
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
	s.registerReportRoute()
}

func NewSyntaxFlowWebServer(ctx context.Context, opts ...ServerOpt) (string, error) {
	config := NewServerConfig()
	for _, opt := range opts {
		opt(config)
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
			SfWebLogger.Infof("Request:\n%s", requestRaw)
			var debugWriter *LogHTTPResponseWriter
			if config.Debug {
				debugWriter = NewLogHTTPResponseWriter(writer)
				writer = debugWriter
			}
			writer.Header().Set("Access-Control-Allow-Origin", "*")
			writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			writer.Header().Set("Access-Control-Allow-Credentials", "true")

			handler.ServeHTTP(writer, request)
			if config.Debug {
				SfWebLogger.Debugf("Response:\n%s", debugWriter.Raw())
			}
		})
	})

	server := &SyntaxFlowWebServer{router: router, config: config}
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		return "", err
	}
	server.grpcClient = client

	// route
	server.init()

	if config.Port <= 0 {
		config.Port = utils.GetRandomAvailableTCPPort()
	}
	addr := utils.HostPort(config.Host, config.Port)

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
		ca, key, _ := crep.GetDefaultCaAndKey()
		if ca == nil || !config.Https {
			dealTls <- false
			SfWebLogger.Info("start to load no tls config")
			err := http.Serve(lis, router)
			if err != nil {
				SfWebLogger.Error(err)
			}
		} else {
			dealTls <- true
			SfWebLogger.Info("start to load tls config")
			crt, serverKey, _ := tlsutils.SignServerCrtNKeyWithParams(ca, key, "127.0.0.1", time.Now().Add(time.Hour*24*180), false)
			config, err := tlsutils.GetX509ServerTlsConfig(ca, crt, serverKey)
			if err != nil {
				SfWebLogger.Error(err)
				return
			}
			server := &http.Server{Handler: router}
			server.TLSConfig = config
			err = server.ServeTLS(lis, "", "")
			if err != nil {
				SfWebLogger.Error(err)
			}
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
