package openapi

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"testing"
)

//go:embed openapi2/testdata/swagger.json
var openapi2demo string

func TestRequest_V2(t *testing.T) {
	config := NewDefaultOpenAPIConfig()
	db := consts.GetGormProjectDatabase()
	config.FlowHandler = func(flow *yakit.HTTPFlow) {
		flow.SourceType = "crawler"
		yakit.SaveHTTPFlow(db, flow)
	}
	v2Generator(openapi2demo, config)
}

//go:embed openapi3/testdata/oai_v3_stoplight.json
var openapi3demo string

func TestRequest_V3(t *testing.T) {
	config := NewDefaultOpenAPIConfig()
	db := consts.GetGormProjectDatabase()
	_ = db
	config.FlowHandler = func(flow *yakit.HTTPFlow) {
		flow.SourceType = "crawler"
		yakit.SaveHTTPFlow(db, flow)
	}
	v3Generator(openapi3demo, config)
}
