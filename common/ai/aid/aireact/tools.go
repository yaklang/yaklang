package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
)

func NewTestReAct(opts ...aicommon.ConfigOption) (*ReAct, error) {
	basicOption := []aicommon.ConfigOption{
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithPeriodicVerificationInterval(0),
	}
	basicOption = append(basicOption, opts...)
	ins, err := NewReAct(
		basicOption...,
	)
	if err != nil {
		return nil, err
	}
	ins.memoryTriage.SetInvoker(ins)
	ins.config.SetConfig("test_yaklang_aikb_rag", true)
	return ins, nil
}
