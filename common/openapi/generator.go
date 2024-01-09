package openapi

import "github.com/yaklang/yaklang/common/utils"

// Generate means generate yakit.HTTPFlow via openapi2/3 scheme
// use WithFlowHandler to recv and handle it
// Example:
//
//	openapi.Generate(fileName, openapi.flowHandler(flow => {
//		dump(flow.Url)
//	}))
func Generate(doc string, opt ...Option) error {
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
