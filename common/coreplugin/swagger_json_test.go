package coreplugin_test

import (
	"fmt"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc"
)

func TestGRPCMUSTPASS_SwaggerJson(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		panic(err)
	}

	pluginName := "Swagger JSON 泄漏"
	vul := VulInfo{
		Path: []string{"/", "/sensitive"},
		ExpectedResult: map[string]int{
			fmt.Sprintf("Swagger(OpenAPI 3) on: %s/swagger/v2/swagger.json", vulAddr):   1,
			fmt.Sprintf("Swagger(OpenAPI 2) on: %s/swagger/v1/swagger.json", vulAddr):   1,
			fmt.Sprintf("Swagger(OpenAPI 3) on: %s/sensitive/v2/swagger.json", vulAddr): 1,
			fmt.Sprintf("Swagger(OpenAPI 2) on: %s/sensitive/v1/swagger.json", vulAddr): 1,
		},
		StrictMode: true,
	}

	Must(CoreMitmPlugTest(pluginName, server, vul, client, t), " ")
}
