package lowhttp

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

type WebHookServer struct {
	s    *http.Server
	addr string
}

func NewWebHookServerEx(port int, cb func(data interface{})) *WebHookServer {
	addr := fmt.Sprintf("127.0.0.1:%v", port)

	//r := mux.NewRouter()
	//r.HandleFunc("/webhook", func(writer http.ResponseWriter, request *http.Request) {
	//	defer writer.WriteHeader(200)
	//	target := fmt.Sprintf("http://%v/webhook", addr)
	//	request.URL, _ = url.Parse(target)
	//	if request.Body == nil {
	//		request.Body = http.NoBody
	//		request.GetBody = func() (io.ReadCloser, error) {
	//			return http.NoBody, nil
	//		}
	//	}
	//	cb(request)
	//})
	//r.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
	//	defer writer.WriteHeader(200)
	//	target := fmt.Sprintf("http://%v/", addr)
	//	request.URL, _ = url.Parse(target)
	//	if request.Body == nil {
	//		request.Body = http.NoBody
	//		request.GetBody = func() (io.ReadCloser, error) {
	//			return http.NoBody, nil
	//		}
	//	}
	//	cb(request)
	//})
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			defer writer.WriteHeader(200)

			log.Infof("webhook met req: %v<-%v", addr, request.RemoteAddr)

			reqBytes, err := utils.HttpDumpWithBody(request, true)
			if err != nil {
				log.Errorf("webhook read request failed: %s", err)
				return
			}

			requestIns, err := utils.ReadHTTPRequestFromBytes(reqBytes)
			if err != nil {
				log.Errorf("re-build webhook request failed: %s", err)
				return
			}

			originUrl := requestIns.URL
			target := fmt.Sprintf("http://%v%v", addr, requestIns.URL.Path)
			requestIns.URL, _ = url.Parse(target)
			if requestIns.URL != nil && requestIns.Body == nil {
				requestIns.Body = http.NoBody
				requestIns.GetBody = func() (io.ReadCloser, error) {
					return http.NoBody, nil
				}
			}

			if requestIns.URL == nil {
				requestIns.URL = originUrl
			}
			cb(requestIns)
		}),
		TLSConfig:         nil,
		ReadTimeout:       30 * time.Second,
		ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	server.SetKeepAlivesEnabled(true)
	return &WebHookServer{
		s:    server,
		addr: addr,
	}
}

func NewWebHookServer(port int, cb func(data interface{})) *WebHookServer {
	addr := fmt.Sprintf("127.0.0.1:%v", port)
	server := &http.Server{
		Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			defer writer.WriteHeader(200)

			if request.Body != nil {
				raw, err := ioutil.ReadAll(request.Body)
				if err != nil {
					return
				}
				cb(raw)
			}
		}),
		Addr:         addr,
		WriteTimeout: 10 * time.Second,
		ReadTimeout:  10 * time.Second,
	}
	server.SetKeepAlivesEnabled(true)
	return &WebHookServer{
		s:    server,
		addr: addr,
	}

}

func (w *WebHookServer) Start() {
	go func() {
		err := w.s.ListenAndServe()
		if err != nil {
			//log.Errorf("serve failed: %s", err)
			//panic(err)
		}
	}()
}

func (w *WebHookServer) Shutdown() {
	_ = w.s.Shutdown(utils.TimeoutContext(1 * time.Second))
}

func (w *WebHookServer) Addr() string {
	return fmt.Sprintf("http://%v/webhook", w.addr)
}
