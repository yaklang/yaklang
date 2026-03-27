package yakgrpc

import (
	"net/http"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yak"
)

const mitmHotPatchPhaseContextKey = "yakgrpc.mitm.hotPatchPhaseContext"

func getMitmHotPatchPhaseContext(req *http.Request) *yak.HotPatchPhaseContext {
	if req == nil {
		return nil
	}
	raw := httpctx.GetContextAnyFromRequest(req, mitmHotPatchPhaseContextKey)
	if raw == nil {
		return nil
	}
	ctx, _ := raw.(*yak.HotPatchPhaseContext)
	return ctx
}

func setMitmHotPatchPhaseContext(req *http.Request, phaseCtx *yak.HotPatchPhaseContext) {
	if req == nil || phaseCtx == nil {
		return
	}
	httpctx.SetContextValueInfoFromRequest(req, mitmHotPatchPhaseContextKey, phaseCtx)
}

func clearMitmHotPatchPhaseContext(req *http.Request) {
	if req == nil {
		return
	}
	httpctx.SetContextValueInfoFromRequest(req, mitmHotPatchPhaseContextKey, nil)
}

func prepareMitmRequestPhaseContext(
	req *http.Request,
	source string,
	isHTTPS bool,
	u string,
	originReq []byte,
	request []byte,
) *yak.HotPatchPhaseContext {
	phaseCtx := getMitmHotPatchPhaseContext(req)
	if phaseCtx == nil {
		phaseCtx = yak.NewHotPatchRequestPhaseContext(source, isHTTPS, u, originReq, request, nil, nil)
		setMitmHotPatchPhaseContext(req, phaseCtx)
		return phaseCtx
	}
	phaseCtx.PrepareForRequestPhase(source, isHTTPS, u, originReq, request)
	return phaseCtx
}

func prepareMitmResponsePhaseContext(
	req *http.Request,
	source string,
	isHTTPS bool,
	u string,
	originReq []byte,
	request []byte,
	originRsp []byte,
	response []byte,
) *yak.HotPatchPhaseContext {
	phaseCtx := prepareMitmRequestPhaseContext(req, source, isHTTPS, u, originReq, request)
	phaseCtx.PrepareForResponsePhase(source, isHTTPS, u, originReq, request, originRsp, response)
	return phaseCtx
}

func prepareMitmArchivePhaseContext(req *http.Request, source string, flow *schema.HTTPFlow) *yak.HotPatchPhaseContext {
	phaseCtx := getMitmHotPatchPhaseContext(req)
	if phaseCtx == nil {
		return yak.NewHotPatchFlowArchiveContext(source, flow)
	}
	phaseCtx.PrepareForArchivePhase(source, flow)
	return phaseCtx
}
