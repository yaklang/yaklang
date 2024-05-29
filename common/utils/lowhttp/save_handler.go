package lowhttp

import (
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type saveHTTPFlowHandler func(*LowhttpResponse)

var saveHTTPFlowFunc saveHTTPFlowHandler

func RegisterSaveHTTPFlowHandler(h saveHTTPFlowHandler) {
	m := new(sync.Mutex)

	saveHTTPFlowFunc = func(r *LowhttpResponse) {
		m.Lock()
		defer m.Unlock()

		defer func() {
			if err := recover(); err != nil {
				log.Errorf("call lowhttp.saveHTTPFlowFunc panic: %s", err)
			}
		}()

		h(r)
	}
}

func SaveLowHTTPResponse(r *LowhttpResponse) {
	if saveHTTPFlowFunc == nil {
		utils.Debug(func() {
			log.Warn("SaveResponse failed because yakit.RegisterSaveHTTPFlowHandler is not finished")
		})
		return
	}

	saveHTTPFlowFunc(r)
}
