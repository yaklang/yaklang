package browserauthorization

import (
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Config struct {
	WorkspaceID string
	Query       string
}

func normalizedParamKey(value string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(
		strings.ToLower(strings.TrimSpace(value)),
	)
}

func ParseConfig(items []*ypb.ExecParamItem) (Config, error) {
	var config Config
	for _, item := range items {
		if item == nil {
			continue
		}
		switch normalizedParamKey(item.GetKey()) {
		case "workspaceid":
			config.WorkspaceID = strings.TrimSpace(item.GetValue())
		case "query":
			config.Query = strings.TrimSpace(item.GetValue())
		}
	}
	if config.WorkspaceID == "" {
		return config, errors.New("browser authorization analysis requires workspace_id")
	}
	if config.Query == "" {
		config.Query = "Inspect the bound dual-identity authorization workspace, validate its deterministic plan, execute it only when appropriate, and explain the resulting evidence."
	}
	return config, nil
}
