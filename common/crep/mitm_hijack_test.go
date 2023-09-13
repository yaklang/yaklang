package crep

import (
	"bytes"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"net/http"
	"testing"
	"time"
)

func TestMITM_SetTransparentHijackResponse(t *testing.T) {
	test := assert.New(t)

	rs, err := NewMITMServer(
		//MITM_SetHTTPRequestHijackRaw(func(isHttps bool, reqIns *http.Request, req []byte) []byte {
		//	*reqIns = *reqIns.WithContext(httptrace.WithClientTrace(reqIns.Context(), &httptrace.ClientTrace{
		//		GotConn: func(info httptrace.GotConnInfo) {
		//			log.Infof("fetch context: %v => %v", info.Conn.RemoteAddr(), info.Conn.LocalAddr())
		//		}}))
		//	return nil
		//}),
		MITM_SetHTTPResponseHijackRaw(func(isHttps bool, req *http.Request, rsp []byte, remoteAddr string) []byte {
			//log.Infof("remote addr: %v", remoteAddr)

			if req.Method == "CONNECT" {
				return rsp
			}
			flag := "www.iana.org/domains/example"
			newFlag := "xxxxxxxxxxxxxxxxxxxx"
			var err error
			rsp, _, err = lowhttp.FixHTTPResponse(rsp)
			if err != nil {
				return rsp
			}
			newResp := bytes.ReplaceAll(rsp, []byte(flag), []byte(newFlag))
			newResp, _, err = lowhttp.FixHTTPResponse(newResp)
			if err != nil {
				return rsp
			}
			return newResp
		}))
	if err != nil {
		test.FailNow(err.Error())
		return
	}

	addr := "127.0.0.1:55342"

	go func() {
		err := rs.Serve(context.Background(), addr)
		if err != nil {
			test.FailNow(err.Error())
		}
	}()
	time.Sleep(1 * time.Second)

	client := netx.NewDefaultHTTPClient()
	client.Transport = netx.NewDefaultHTTPTransport()
	req, err := http.NewRequest("GET", "https://www.example.com", nil)
	if err != nil {
		test.FailNow(err.Error())
		return
	}
	req.Header["lower-header"] = []string{"value"}
	rsp, err := client.Do(req)
	if err != nil {
		test.FailNow(err.Error())
	}
	utils.HttpShow(rsp)
}
