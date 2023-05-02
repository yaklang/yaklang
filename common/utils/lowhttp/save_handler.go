package lowhttp

import (
	"sync"
	"yaklang.io/yaklang/common/log"
)

type saveHTTPFlowHandler func(https bool, req []byte, rsp []byte, url string, remoteAddr string, reqSource string)

var saveHTTPFlowFunc saveHTTPFlowHandler

func RegisterSaveHTTPFlowHandler(h saveHTTPFlowHandler) {
	m := new(sync.Mutex)
	saveHTTPFlowFunc = func(https bool, req []byte, rsp []byte, url string, remoteAddr string, reqSource string) {
		m.Lock()
		defer m.Unlock()

		defer func() {
			if err := recover(); err != nil {
				log.Errorf("call lowhttp.saveHTTPFlowFunc panic: %s", err)
			}
		}()

		h(https, req, rsp, url, remoteAddr, reqSource)
	}
}
func SaveResponse(r *LowhttpResponse) {
	if saveHTTPFlowFunc == nil {
		return
	}
	saveHTTPFlowFunc(r.Https, r.RawRequest, r.RawPacket, r.Url, r.RemoteAddr, r.Source)
}
