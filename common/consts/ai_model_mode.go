package consts

import (
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var ErrInvalidSingleAIModel = errors.New("single-model mode requires a configured intelligent model with provider and model name")

func FirstIntelligentModel(models []*ypb.AIModelConfig) *ypb.AIModelConfig {
	if len(models) == 0 {
		return nil
	}
	return models[0]
}

// IsSingleAIModelMode reads the global startup setting. Existing Configs keep
// their resolved mode independently of later changes to this value.
func IsSingleAIModelMode() bool {
	tieredAIConfigLock.RLock()
	defer tieredAIConfigLock.RUnlock()
	return tieredAIConfig != nil && tieredAIConfig.SingleModelMode
}

func ValidateSingleAIModel(model *ypb.AIModelConfig) error {
	if model == nil || model.GetProvider() == nil || strings.TrimSpace(model.GetProvider().GetType()) == "" || strings.TrimSpace(model.GetModelName()) == "" {
		return ErrInvalidSingleAIModel
	}
	return nil
}
