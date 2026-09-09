// legion-platform-assistant is the resident platform-only Yaklang AI runtime.
package main

import (
	"context"
	"flag"
	"github.com/yaklang/yaklang/common/ai/platformassistant"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8094", "HTTP listen address")
	legion := flag.String("legion-url", "", "fixed Legion API origin")
	concurrency := flag.Int("max-concurrent", 16, "global active turn limit")
	perUser := flag.Int("max-per-user", 2, "per-user active turn limit")
	timeout := flag.Duration("timeout", 3*time.Minute, "turn deadline, up to five minutes")
	flag.Parse()
	handler, err := platformassistant.New(platformassistant.Config{LegionURL: *legion, ServiceSecret: os.Getenv("LEGION_ASSISTANT_SERVICE_SECRET"), MaxConcurrent: *concurrency, MaxPerUser: *perUser, TurnTimeout: *timeout})
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if server.Shutdown(shutdown) != nil {
			_ = server.Close()
		}
	}()
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
