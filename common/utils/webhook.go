package utils

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type WebHookServer struct {
	s    *http.Server
	addr string
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
	_ = w.s.Shutdown(TimeoutContext(1 * time.Second))
}

func (w *WebHookServer) Addr() string {
	return fmt.Sprintf("http://%v/webhook", w.addr)
}
