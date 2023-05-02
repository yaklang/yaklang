package mutate

import "yaklang.io/yaklang/common/utils"

func (s *FuzzHTTPRequest) GetFirstFuzzHTTPRequest() (*FuzzHTTPRequest, error) {
	return NewFuzzHTTPRequest(s.originRequest, s.Opts...)
}

func (s *FuzzHTTPRequestBatch) GetFirstFuzzHTTPRequest() (*FuzzHTTPRequest, error) {
	reqs, err := s.Results()
	if err != nil {
		return nil, err
	}
	if len(reqs) <= 0 {
		return nil, utils.Error("empty result ... for GetFirstFuzzHTTPRequest")
	}
	raw := reqs[0]
	return NewFuzzHTTPRequest(raw, reqToOpts(raw)...)
}
