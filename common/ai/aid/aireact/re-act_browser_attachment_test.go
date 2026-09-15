package aireact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func browserAttachment(deviceID, reference string) *ypb.AttachedResourceInfo {
	return &ypb.AttachedResourceInfo{
		Type:  aicommon.AttachedResourceTypeBrowser,
		Key:   aicommon.AttachedResourceKeyBrowserDevice,
		Value: `{"deviceId":"` + deviceID + `","reference":"` + reference + `"}`,
	}
}

func attachedBrowserReferences(t *testing.T, task aicommon.AIStatefulTask) []string {
	t.Helper()
	var references []string
	for _, attached := range task.GetAttachedDatas() {
		if !attached.HasType(aicommon.AttachedResourceTypeBrowser) {
			continue
		}
		parsed, err := aicommon.ParseAttachedResourceData(attached)
		require.NoError(t, err)
		references = append(references, parsed.(*aicommon.AttachedBrowserResourceData).Reference)
	}
	require.NotEmpty(t, references)
	return references
}

func TestReActBrowserAttachmentsPersistAcrossTurns(t *testing.T) {
	react := &ReAct{config: &aicommon.Config{Ctx: context.Background()}}

	first := react.buildReTaskFromEvent(&ypb.AIInputEvent{
		FreeInput: "open bilibili",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			browserAttachment("device-a", "A"),
			{Type: aicommon.AttachedResourceTypeSelected, Key: aicommon.AttachedResourceKeyContent, Value: "first-turn-only"},
		},
	})
	require.Equal(t, []string{"A"}, attachedBrowserReferences(t, first))

	followUp := react.buildReTaskFromEvent(&ypb.AIInputEvent{
		FreeInput: "tell me my profile",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			{Type: aicommon.AttachedResourceTypeSelected, Key: aicommon.AttachedResourceKeyContent, Value: "second-turn-only"},
		},
	})
	require.Equal(t, []string{"A"}, attachedBrowserReferences(t, followUp))
	require.Len(t, followUp.GetAttachedDatas(), 2)
	require.Equal(t, "second-turn-only", followUp.GetAttachedDatas()[0].Value)
	allowed, feedback := aicommon.CheckAttachedBrowserToolRoute(followUp, "browser.capability.call")
	require.True(t, allowed, feedback)
	params, err := aicommon.BindAttachedBrowserToolParams(followUp, "browser.capability.call", nil)
	require.NoError(t, err)
	require.Equal(t, "device-a", params.GetString("device_id"))

	switched := react.buildReTaskFromEvent(&ypb.AIInputEvent{
		FreeInput: "compare both accounts",
		AttachedResourceInfo: []*ypb.AttachedResourceInfo{
			browserAttachment("device-a", "A"),
			browserAttachment("device-b", "B"),
		},
	})
	require.Equal(t, []string{"A", "B"}, attachedBrowserReferences(t, switched))

	nextFollowUp := react.buildReTaskFromEvent(&ypb.AIInputEvent{FreeInput: "continue"})
	require.Equal(t, []string{"A", "B"}, attachedBrowserReferences(t, nextFollowUp))
}
