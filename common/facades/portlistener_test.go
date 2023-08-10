package facades

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net/http"
	"testing"
	"time"
)

func TestPortListener(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("abc"))
	})
	_ = host
	go func() {
		_, _, local, _ := netutil.GetPublicRoute()
		time.Sleep(3 * time.Second)
		u := "http://" + utils.HostPort(local.String(), port)
		log.Infof("start to send: %v", u)
		rsp, err := http.Get(u)
		if err != nil {
			panic(err)
		}
		raw, _ := utils.HttpDumpWithBody(rsp, true)
		//spew.Dump(raw)
		_ = raw
	}()
	err := (&PortListener{}).handleFromPcap(utils.TimeoutContextSeconds(100), "", port)
	if err != nil {
		log.Error(err)
	}
}
