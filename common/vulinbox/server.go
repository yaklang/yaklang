package vulinbox

import (
	"context"
	"fmt"
	"github.com/gorilla/mux"
	"net"
	"net/http"
	"yaklang/common/log"
	"yaklang/common/utils"
	"time"
)

type VulinServer struct {
	database *dbm
	router   *mux.Router
}

func NewVulinServer(ctx context.Context, ports ...int) (string, error) {
	var router = mux.NewRouter()

	var port int
	if len(ports) > 0 {
		port = ports[0]
	}

	var m, err = newDBM()
	if err != nil {
		return "", err
	}
	server := &VulinServer{database: m, router: router}
	server.init()

	var host = "127.0.0.1"
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
	go func() {
		err := http.Serve(lis, router)
		if err != nil {
			log.Error(err)
		}
	}()
	time.Sleep(time.Second)
	addr := fmt.Sprintf("http://%v", utils.HostPort(host, port))
	log.Infof("start vulinbox on: %v", addr)
	return addr, nil
}
