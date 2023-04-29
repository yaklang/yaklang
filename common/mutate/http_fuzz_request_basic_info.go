package mutate

import (
	"mime"
	"net/url"
	"yaklang/common/go-funk"
	"yaklang/common/log"
	"yaklang/common/utils/lowhttp"
)

func (f *FuzzHTTPRequest) GetBytes() []byte {
	return f.originRequest
}

func (f *FuzzHTTPRequest) GetHeader(key string) string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return ""
	}
	return req.Header.Get(key)
}

func (f *FuzzHTTPRequest) GetHeaderKeys() []string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return []string{}
	}
	var headers []string
	for k := range req.Header {
		headers = append(headers, k)
	}
	return headers
}

func (f *FuzzHTTPRequest) GetHeaderValues() []string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return []string{}
	}
	var item []string
	for _, v := range req.Header {
		for _, value := range v {
			item = append(item, value)
		}
	}
	return item
}

func (f *FuzzHTTPRequest) GetRequestURI() string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return ""
	}
	return req.RequestURI
}

func (f *FuzzHTTPRequest) GetUrl() string {
	u, err := lowhttp.ExtractURLFromHTTPRequestRaw(f.GetBytes(), f.isHttps)
	if err != nil {
		log.Errorf("extract url from request raw failed: %s", err)
		return ""
	}
	return u.String()
}

func (f *FuzzHTTPRequest) GetPathWithQuery() string {
	return f.GetRequestURI()
}

func (f *FuzzHTTPRequest) GetPath() string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return ""
	}
	return req.URL.Path
}

func (f *FuzzHTTPRequest) GetQueryRaw() string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return ""
	}
	return req.URL.RawQuery
}

func (f *FuzzHTTPRequest) GetQueryKeys() []string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return []string{}
	}
	q := req.URL.Query()
	if q == nil {
		return []string{}
	}
	var values []string
	for k := range q {
		values = append(values, k)
	}
	return values
}

func (f *FuzzHTTPRequest) GetQueryValues() []string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return []string{}
	}
	q := req.URL.Query()
	if q == nil {
		return []string{}
	}
	var values []string
	for _, v := range q {
		for _, k := range v {
			values = append(values, k)
		}
	}
	return values
}

func (f *FuzzHTTPRequest) GetQueryValue(key string) string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return ""
	}
	q := req.URL.Query()
	if q == nil {
		return ""
	}
	return q.Get(key)
}

func (f *FuzzHTTPRequest) GetBody() []byte {
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(f.originRequest)
	return body
}

func (f *FuzzHTTPRequest) getPostQuery() url.Values {
	q, err := url.ParseQuery(string(f.GetBody()))
	if err != nil {
		return make(url.Values)
	}
	return q
}

func (f *FuzzHTTPRequest) GetPostQuery() string {
	return f.getPostQuery().Encode()
}

func (f *FuzzHTTPRequest) GetPostQueryKeys() []string {
	return funk.Keys(f.getPostQuery()).([]string)
}

func (f *FuzzHTTPRequest) GetPostQueryValues() []string {
	var vals []string
	for _, vs := range f.getPostQuery() {
		for _, v := range vs {
			vals = append(vals, v)
		}
	}
	return vals
}

func (f *FuzzHTTPRequest) GetPostQueryValue(key string) string {
	return f.getPostQuery().Get(key)
}

func (f *FuzzHTTPRequest) GetMethod() string {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return ""
	}
	return req.Method
}

func (f *FuzzHTTPRequest) GetContentType() string {
	ct := f.GetHeader("content-type")
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return ct
	}
	_ = params
	return mt
}
