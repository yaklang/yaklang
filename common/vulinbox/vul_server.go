package vulinbox

import (
	"context"
	"embed"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net"
	"net/http"
	"time"
)

type VulinServer struct {
	database *dbm
	router   *mux.Router

	wsAgent wsAgent
	matcher matcher

	safeMode           bool
	groupedRoutesCache []*GroupedRoutes
}

func NewVulinServer(ctx context.Context, port ...int) (string, error) {
	return NewVulinServerEx(ctx, false, false, "127.0.0.1", port...)
}

func NewVulinboxAgent(ctx context.Context, port ...int) (string, error) {
	return NewVulinServerEx(ctx, true, true, "0.0.0.0", port...)
}

//go:embed static/*
var staticFS embed.FS

func NewVulinServerEx(ctx context.Context, noHttps, safeMode bool, host string, ports ...int) (string, error) {
	var router = mux.NewRouter()
	router.Use(func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			log.Infof("VULINBOX: %s %s", request.Method, request.URL)
			raw, _ := utils.HttpDumpWithBody(request, true)
			if string(raw) != "" {
				if len(raw) > 4000 {
					header, _ := lowhttp.SplitHTTPPacketFast(raw)
					println(header)
				} else {
					log.Println(string(raw))
				}
			}
			handler.ServeHTTP(writer, request)
		})
	})

	var port int
	if len(ports) > 0 {
		port = ports[0]
	}

	var m, err = newDBM()
	if err != nil {
		return "", err
	}
	server := &VulinServer{database: m, router: router, safeMode: safeMode}
	server.init()

	if port <= 0 {
		port = utils.GetRandomAvailableTCPPort()
	}

	lis, err := net.Listen("tcp", "0.0.0.0:"+fmt.Sprint(port))
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
		if ca == nil || noHttps {
			dealTls <- false
			log.Info("start to load no tls config")
			err := http.Serve(lis, router)
			if err != nil {
				log.Error(err)
			}
		} else {
			dealTls <- true
			log.Info("start to load tls config")
			crt, serverKey, _ := tlsutils.SignServerCrtNKeyWithParams(ca, key, "127.0.0.1", time.Now().Add(time.Hour*24*180), false)
			config, err := tlsutils.GetX509ServerTlsConfig(ca, crt, serverKey)
			if err != nil {
				log.Error(err)
				return
			}
			server := &http.Server{Handler: router}
			server.TLSConfig = config
			err = server.ServeTLS(lis, "", "")
			//err := http.ServeTLS(lis, router, "server.crt", "server.key")
			if err != nil {
				log.Error(err)
			}
		}
	}()
	var proto = "http"
	if <-dealTls {
		proto = "https"
	}
	time.Sleep(time.Second)
	addr := fmt.Sprintf("%s://%v", proto, utils.HostPort(host, port))
	log.Infof("start vulinbox on: %v", addr)
	return addr, nil
}
