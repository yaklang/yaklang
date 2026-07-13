package yakgrpc

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func init() {
	yakit.RegisterMCPBuiltinToolDefaultEnableResolver(resolveBuiltinToolDefaultEnable)
}

func resolveBuiltinToolDefaultEnable(db *gorm.DB, toolName string) (bool, error) {
	setName, ok := mcp.BuiltinToolSetOf(toolName)
	if !ok {
		return false, nil
	}
	defaultSets, err := yakit.EffectiveDefaultMCPToolSetMap(db)
	if err != nil {
		return false, err
	}
	_, enabled := defaultSets[setName]
	return enabled, nil
}
