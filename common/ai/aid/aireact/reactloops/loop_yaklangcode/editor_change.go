package loop_yaklangcode

import "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"

const defaultYaklangCodeChangeSource = "yaklang_code"

func BuildYaklangPatchChangeEvent(path string, patch *loopinfra.CodeDeliveryPatch, version int, sourceAction, reason string) loopinfra.CodeChangeEvent {
	return loopinfra.BuildCodePatchChangeEvent(path, patch, version, sourceAction, reason, defaultYaklangCodeChangeSource)
}

func BuildYaklangFullChangeEvent(op, path, content string, version int, sourceAction, reason string) loopinfra.CodeChangeEvent {
	return loopinfra.BuildCodeFullChangeEvent(op, path, content, version, sourceAction, reason, defaultYaklangCodeChangeSource)
}
