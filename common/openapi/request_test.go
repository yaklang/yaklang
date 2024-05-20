package openapi

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/schema"
	"testing"
)

//go:embed openapi2/testdata/swagger.json
var openapi2demo string

func TestRequest_V2(t *testing.T) {
	var count = 0
	err := GenerateHTTPFlows(openapi2demo, WithFlowHandler(func(flow *schema.HTTPFlow) {
		count++
		flow.SourceType = "mitm"
		//yakit.SaveHTTPFlow(consts.GetGormProjectDatabase(), flow)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if count < 36 {
		t.Fatal("generated flows toooooooo less")
	}
}

//go:embed openapi3/testdata/oai_v3_stoplight.json
var openapi3demo string

func TestRequest_V3(t *testing.T) {
	var count = 0
	err := GenerateHTTPFlows(openapi3demo, WithFlowHandler(func(flow *schema.HTTPFlow) {
		count++
		flow.SourceType = "mitm"
		//yakit.SaveHTTPFlow(consts.GetGormProjectDatabase(), flow)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if count < 918 {
		t.Fatal("generated flows toooooooo less")
	}
}
