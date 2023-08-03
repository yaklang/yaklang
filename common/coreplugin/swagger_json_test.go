package coreplugin

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"testing"
)

func TestGRPCMUSTPASS_SwaggerJson(t *testing.T) {
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		panic(err)
	}

	vulAddr, err := vulinbox.NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}

	pluginName := "Swagger JSON 泄漏"
	server := VulServerInfo{
		VulServerAddr: vulAddr,
		IsHttps:       true,
	}
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

	Must(TestCoreMitmPlug(pluginName, server, vul, client, t), " ")
}
