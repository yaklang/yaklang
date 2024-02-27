package mutate

import (
	"net/http"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type FuzzHTTPRequestBatch struct {
	noAutoEncode     bool
	originRequest    *FuzzHTTPRequest
	fallback         FuzzHTTPRequestIf
	nextFuzzRequests []FuzzHTTPRequestIf
}

func NewFuzzHTTPRequestBatch(f *FuzzHTTPRequest, reqs ...*http.Request) *FuzzHTTPRequestBatch {
	var fReqs []FuzzHTTPRequestIf
	for _, r := range reqs {
		req, err := NewFuzzHTTPRequest(r, f.GetCurrentOptions()...)
		if err != nil {
			continue
		}
		fReqs = append(fReqs, req)
	}
	if fReqs == nil {
		return &FuzzHTTPRequestBatch{fallback: f, originRequest: f, noAutoEncode: f.noAutoEncode}
	}
	return &FuzzHTTPRequestBatch{nextFuzzRequests: fReqs, originRequest: f, noAutoEncode: f.noAutoEncode}
}

func (r *FuzzHTTPRequestBatch) DisableAutoEncode(b bool) FuzzHTTPRequestIf {
	if r != nil {
		r.noAutoEncode = b
		if r.fallback != nil && r.fallback != r {
			r.fallback.DisableAutoEncode(b)
		}
		if r.originRequest != nil {
			r.originRequest.DisableAutoEncode(b)
		}
		for _, nreq := range r.nextFuzzRequests {
			if nreq != nil {
				nreq.DisableAutoEncode(b)
			}
		}

	}
	return r
}

func (f *FuzzHTTPRequestBatch) NoAutoEncode() bool {
	if f == nil {
		return false
	}
	return f.noAutoEncode
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

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzMethod(p ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzMethod(p...)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzMethod(p...))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) toFuzzHTTPRequestIf(reqs []FuzzHTTPRequestIf) FuzzHTTPRequestIf {
	origin := f.GetOriginRequest()
	if origin != nil {
		origin.DisableAutoEncode(f.noAutoEncode)
	}
	if len(reqs) <= 0 {
		return &FuzzHTTPRequestBatch{
			noAutoEncode:  f.noAutoEncode,
			fallback:      f.fallback,
			originRequest: origin,
		}
	}
	return &FuzzHTTPRequestBatch{
		noAutoEncode:     f.noAutoEncode,
		nextFuzzRequests: reqs,
		originRequest:    origin,
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

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzPath(p ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPath(p...)
	}

	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPath(p...))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzHTTPHeader(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzHTTPHeader(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzHTTPHeader(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzPostJsonPathParams(key any, jp string, value any) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostJsonPathParams(key, jp, value)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostJsonPathParams(key, jp, value))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzGetJsonPathParams(key any, jp string, value any) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzGetJsonPathParams(key, jp, value)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzGetJsonPathParams(key, jp, value))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzGetParamsRaw(raw ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzGetParamsRaw(raw...)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzGetParamsRaw(raw...))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzGetParams(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzGetParams(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzGetParams(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzGetBase64Params(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzGetBase64Params(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzGetBase64Params(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzPostRaw(body ...string) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostRaw(body...)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostRaw(body...))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzPostParams(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostParams(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostParams(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzPostBase64Params(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostBase64Params(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostBase64Params(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzPostJsonParams(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostJsonParams(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostJsonParams(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzPostXMLParams(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzPostXMLParams(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzPostXMLParams(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
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

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzCookieBase64(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzCookieBase64(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzCookieBase64(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzFormEncoded(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzFormEncoded(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzFormEncoded(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzUploadFile(k, v interface{}, raw []byte) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzUploadFile(k, v, raw)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzUploadFile(k, v, raw))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzUploadKVPair(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzUploadKVPair(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzUploadKVPair(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
}

func (f *FuzzHTTPRequestBatch) FuzzUploadFileName(k, v interface{}) FuzzHTTPRequestIf {
	if len(f.nextFuzzRequests) <= 0 {
		return f.fallback.FuzzUploadFileName(k, v)
	}
	var reqs []FuzzHTTPRequestIf
	for _, req := range f.nextFuzzRequests {
		reqs = append(reqs, req.FuzzUploadFileName(k, v))
	}

	return f.toFuzzHTTPRequestIf(reqs)
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

func (f *FuzzHTTPRequestBatch) ExecFirst(opts ...HttpPoolConfigOption) (*HttpResult, error) {
	opts = append(opts, WithPoolOpt_RequestCountLimiter(1))
	resultCh, err := f.Exec(opts...)
	if err != nil {
		return nil, err
	}

	var result *HttpResult
	for i := range resultCh {
		result = i
	}
	if result == nil {
		return nil, utils.Error("empty result for FuzzHTTPRequest")
	}
	if result.Error != nil {
		return result, result.Error
	}

	return result, nil
}
