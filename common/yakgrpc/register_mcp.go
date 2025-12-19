package yakgrpc

import (
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	mcp.RegisterNewLocalClient(func(locals ...bool) (mcp.YakClientInterface, error) {
		client, err := NewLocalClient(locals...)
		if err != nil {
			return nil, err
		}
		v, ok := client.(mcp.YakClientInterface)
		if !ok {
			return nil, utils.Error("failed to cast client to mcp.YakClientInterface")
		}
		return v, nil
	})
}
