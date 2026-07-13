package yakgrpc

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func init() {
	yakit.RegisterMCPBuiltinToolDefaultEnableResolver(resolveBuiltinToolDefaultEnable)
	yakit.RegisterMCPGlobalConfigValidators(mcp.ValidateToolSetNames, validateMCPResourceSetNames)
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

func validateMCPResourceSetNames(names []string) error {
	registered := mcp.GlobalResourceSetList()
	for _, name := range names {
		if name == "" {
			continue
		}
		found := false
		for _, item := range registered {
			if item == name {
				found = true
				break
			}
		}
		if !found {
			return utils.Errorf("undefined resource set: %s", name)
		}
	}
	return nil
}
