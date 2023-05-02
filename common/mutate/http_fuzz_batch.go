package mutate

import (
	"net/http"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type FuzzHTTPRequestBatch struct {
	originRequest    *FuzzHTTPRequest
	fallback         FuzzHTTPRequestIf
	nextFuzzRequests []FuzzHTTPRequestIf
}

func NewFuzzHTTPRequestBatch(f *FuzzHTTPRequest, reqs ...*http.Request) *FuzzHTTPRequestBatch {
	var fReqs []FuzzHTTPRequestIf
	for _, r := range reqs {
		req, err := NewFuzzHTTPRequest(r, f.Opts...)
		if err != nil {
			continue
		}
		fReqs = append(fReqs, req)
	}
	if fReqs == nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f}
	}
	return &FuzzHTTPRequestBatch{nextFuzzRequests: fReqs, originRequest: f}
}

func (f *FuzzHTTPRequestBatch) Show() FuzzHTTPRequestIf {
	reqs, err := f.Results()
	if err != nil {
		log.Errorf("fetch results failed: %s", err)
	}

	for _, req := range reqs {
		utils.HttpShow(req)
	}
	return f
}

func (f *FuzzHTTPRequestBatch) GetOriginRequest() *FuzzHTTPRequest {
	if f.originRequest != nil {
		return f.originRequest
	}
	raw, ok := f.fallback.(*FuzzHTTPRequest)
	if !ok {
		return nil
	}
	return raw
}

func (f *FuzzHTTPRequestBatch) Repeat(i int) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.Repeat(i)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.Repeat(i))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzMethod(p ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzMethod(p...)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzMethod(p...))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzPathAppend(p ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPathAppend(p...)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPathAppend(p...))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzPath(p ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPath(p...)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPath(p...))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzHTTPHeader(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzHTTPHeader(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzHTTPHeader(k, v))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzGetParamsRaw(raw ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzGetParamsRaw(raw...)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzGetParamsRaw(raw...))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzGetParams(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzGetParams(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzGetParams(k, v))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzPostRaw(body ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostRaw(body...)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostRaw(body...))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzPostParams(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostParams(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostParams(k, v))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzPostJsonParams(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostJsonParams(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostJsonParams(k, v))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzCookieRaw(value interface{}) FuzzHTTPRequestIf {
	return f.FuzzHTTPHeader("Cookie", value)
}

func (f *FuzzHTTPRequestBatch) FuzzCookie(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzCookie(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzCookie(k, v))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzFormEncoded(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzFormEncoded(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzFormEncoded(k, v))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzUploadFile(k, v interface{}, raw []byte) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzUploadFile(k, v, raw)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzUploadFile(k, v, raw))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzUploadKVPair(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzUploadKVPair(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzUploadKVPair(k, v))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) FuzzUploadFileName(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzUploadFileName(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzUploadFileName(k, v))
	}

	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			fallback:      f.fallback,
			originRequest: f.GetOriginRequest(),
		}
	}
	return &FuzzHTTPRequestBatch{
		nextFuzzRequests: reqs,
		originRequest:    f.GetOriginRequest(),
	}
}

func (f *FuzzHTTPRequestBatch) Results() ([]*http.Request, error) {
	if f.fallback != nil {
		return f.fallback.Results()
	}

	var reqs []*http.Request
	for _, tReq := range f.nextFuzzRequests {
		middleResults, err := tReq.Results()
		if err != nil {
			return nil, utils.Errorf("fuzz failed: %s", err)
		}
		reqs = append(reqs, middleResults...)
	}

	if reqs == nil {
		return nil, utils.Errorf("fuzz failed... empty fuzz result")
	}
	return reqs, nil
}

func (f *FuzzHTTPRequestBatch) ExecFirst(opts ...HttpPoolConfigOption) (*_httpResult, error) {
	return executeOne(f, opts...)
}

func executeOne(f FuzzHTTPRequestIf, opts ...HttpPoolConfigOption) (*_httpResult, error) {
	reqs, err := f.Results()
	if err != nil {
		return nil, err
	}
	if len(reqs) <= 0 {
		return nil, utils.Error("no request is rendered")
	}

	fixedOpts := append(opts, _httpPool_SetForceFuzz(false))

	switch v := f.(type) {
	case *FuzzHTTPRequestBatch:
		fixedOpts = append(opts, _httpPool_IsHttps(v.originRequest.isHttps))
	case *FuzzHTTPRequest:
		fixedOpts = append(opts, _httpPool_IsHttps(v.isHttps))
	}

	res, err := _httpPool(reqs[0], fixedOpts...)
	if err != nil {
		return nil, err
	}

	for result := range res {
		if result.Error != nil {
			return result, result.Error
		}
		return result, nil
	}
	return nil, utils.Error("empty result for FuzzHTTPRequest")
}
