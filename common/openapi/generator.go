package openapi

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/openapi/openapigen"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strconv"
)

// GenerateHTTPFlows means generate yakit.HTTPFlow via openapi2/3 scheme
// use WithFlowHandler to recv and handle it
// Example:
//
//	openapi.Generate(fileName, openapi.flowHandler(flow => {
//		dump(flow.Url)
//	}))
func GenerateHTTPFlows(doc string, opt ...Option) error {
	defer func() {
		if err := recover(); err != nil {
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	config := NewDefaultOpenAPIConfig()
	for _, p := range opt {
		p(config)
	}
	err1 := v2Generator(doc, config)
	if err1 != nil {
		err2 := v3Generator(doc, config)
		if err2 != nil {
			return utils.Errorf("generate openapi2/3 failed, reason: openapi2.0[%v], openapi3.0[%v]", err1, err2)
		}
	}
	return nil
}

// ExtractOpenAPI3Scheme fetch openapi3 scheme from yakit.HTTPFlows
// Example:
//
// scheme := openapi.ExtractOpenAPI3Scheme(domain)~
// schemeJSON = scheme.MarshalJSON()~
func ExtractOpenAPI3Scheme(domain string) (*openapi3.T, error) {
	var err error
	db := consts.GetGormProjectDatabase()
	db = db.Where("(url GLOB ?) or (url GLOB ?)", `http://`+domain+`/*`, `https://`+domain+`/*`)
	// db = db.Debug()
	var c = make(chan *openapigen.BasicHTTPFlow)
	go func() {
		defer func() {
			close(c)
		}()

		for result := range yakit.YieldHTTPFlows(db, context.Background()) {
			req, _ := strconv.Unquote(result.Request)
			rsp, _ := strconv.Unquote(result.Response)
			if len(req) <= 0 {
				continue
			}
			c <- &openapigen.BasicHTTPFlow{
				Request:  []byte(req),
				Response: []byte(rsp),
			}
		}
	}()
	scheme, err := openapigen.GenerateV3Scheme(c)
	if err != nil {
		return nil, err
	}
	return scheme, nil
}
