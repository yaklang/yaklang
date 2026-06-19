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

// GenerateHTTPFlows 根据 OpenAPI 2.0/3.0 文档生成对应的 HTTP 请求流
// 自动尝试按 OpenAPI 2 与 OpenAPI 3 解析，通过 openapi.flowHandler 接收每个生成的流
// 参数:
//   - doc: OpenAPI 文档内容（JSON 或 YAML）
//   - opt: 可选项，如 openapi.flowHandler / openapi.domain / openapi.https
//
// 返回值:
//   - 错误信息
//
// Example:
// ```
// // 示意性示例，需提供真实 OpenAPI 文档
// doc = file.ReadFile("openapi.yaml")~
//
//	err = openapi.GenerateHTTPFlows(string(doc), openapi.flowHandler(func(flow) {
//	    println(flow.Url)
//	}))
//
// if err != nil { die(err) }
// ```
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

// ExtractOpenAPI3Scheme 从数据库中已记录的 HTTP 流量提取指定域名的 OpenAPI3 文档
// 依赖本地项目数据库中已抓取的流量数据
// 参数:
//   - domain: 目标域名
//
// 返回值:
//   - 提取出的 OpenAPI3 文档对象
//   - 错误信息
//
// Example:
// ```
// // 示意性示例，需要本地项目数据库中已有该域名的流量
// scheme = openapi.ExtractOpenAPI3Scheme("example.com")~
// schemeJSON = scheme.MarshalJSON()~
// println(string(schemeJSON))
// ```
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
