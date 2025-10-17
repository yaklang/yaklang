package lowhttp

import (
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type saveHTTPFlowHandler func(*LowhttpResponse, bool)

var saveHTTPFlowFunc saveHTTPFlowHandler

func RegisterSaveHTTPFlowHandler(h saveHTTPFlowHandler) {
	m := new(sync.Mutex)

	saveHTTPFlowFunc = func(r *LowhttpResponse, saveFlowSync bool) {
		m.Lock()
		defer m.Unlock()

		defer func() {
			if err := recover(); err != nil {
				log.Errorf("call lowhttp.saveHTTPFlowFunc panic: %s", err)
			}
		}()

		h(r, saveFlowSync)
	}
}

func SaveLowHTTPResponse(r *LowhttpResponse, saveFlowSync bool) {
	if saveHTTPFlowFunc == nil {
		utils.Debug(func() {
			log.Warn("SaveResponse failed because yakit.RegisterSaveHTTPFlowHandler is not finished")
		})
		return
	}

	saveHTTPFlowFunc(r, saveFlowSync)
}

var mitmReplacerLabelingHTTPFlowFunc func(*LowhttpResponse)

func RegisterLabelingHTTPFlowFunc(h func(*LowhttpResponse)) {
	m := new(sync.Mutex)

	mitmReplacerLabelingHTTPFlowFunc = func(r *LowhttpResponse) {
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
