package openapi

import (
	_ "embed"
	"testing"
)

//go:embed openapi2/testdata/swagger.json
var openapi2demo string

func TestRequest_V2(t *testing.T) {
	v2Generator(openapi2demo, nil)
}

//go:embed openapi3/testdata/oai_v3_stoplight.json
var openapi3demo string

func TestRequest_V3(t *testing.T) {
	v3Generator(openapi3demo, nil)
}
