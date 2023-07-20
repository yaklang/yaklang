package lowhttp

import (
	"github.com/yaklang/yaklang/common/log"
	"sync"
)

type saveHTTPFlowHandler func(https bool, req []byte, rsp []byte, url string, remoteAddr string, reqSource string, runtimeId string, fromPlugin string)

var saveHTTPFlowFunc saveHTTPFlowHandler

func RegisterSaveHTTPFlowHandler(h saveHTTPFlowHandler) {
	m := new(sync.Mutex)
	saveHTTPFlowFunc = func(https bool, req []byte, rsp []byte, url string, remoteAddr string, reqSource string, runtimeId string, fromPlugin string) {
		m.Lock()
		defer m.Unlock()

		defer func() {
			if err := recover(); err != nil {
				log.Errorf("call lowhttp.saveHTTPFlowFunc panic: %s", err)
			}
		}()
		h(https, req, rsp, url, remoteAddr, reqSource, runtimeId, fromPlugin)
	}
}
func SaveResponse(r *LowhttpResponse) {
	if saveHTTPFlowFunc == nil {
		return
	}
	saveHTTPFlowFunc(r.Https, r.RawRequest, r.RawPacket, r.Url, r.RemoteAddr, r.Source, r.RuntimeId, r.FromPlugin)
}
