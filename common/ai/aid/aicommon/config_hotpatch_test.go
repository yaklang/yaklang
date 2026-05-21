package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHotPatchConfig(t *testing.T) {
	// Setup config with epm stub
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := NewTestConfig(ctx)
	c.StartEventLoop(ctx)

	require.True(t, c.AllowRequireForUserInteract)
	require.Equal(t, c.AgreePolicy, AgreePolicyManual)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_AllowRequireForUserInteract,
		Params: &ypb.AIStartParams{
			DisallowRequireForUserPrompt: true,
		},
	})
	time.Sleep(1 * time.Second)
	require.False(t, c.AllowRequireForUserInteract)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_AgreePolicy,
		Params: &ypb.AIStartParams{
			ReviewPolicy: string(AgreePolicyYOLO),
		},
	})
	time.Sleep(1 * time.Second)
	require.Equal(t, AgreePolicyYOLO, c.AgreePolicy)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_RiskControlScore,
		Params: &ypb.AIStartParams{
			AIReviewRiskControlScore: 0.75,
		},
	})
	time.Sleep(1 * time.Second)
	require.Equal(t, 0.75, c.AgreeAIScoreMiddle)
	require.Equal(t, 0.55, c.AgreeAIScoreLow)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnablePlan,
		Params: &ypb.AIStartParams{
			EnablePlan: false,
		},
	})
	time.Sleep(1 * time.Second)
	require.False(t, c.GetEnablePlanAndExec())

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_SyncPerceptionTrigger,
		Params: &ypb.AIStartParams{
			SyncPerceptionTrigger: true,
		},
	})
	time.Sleep(1 * time.Second)
	require.True(t, c.GetSyncPerceptionTrigger())
}

func TestConfigHotpatch_PersistSessionStartParams(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessionID := "session-hotpatch-persist"
	c := NewTestConfig(ctx, WithPersistentSessionId(sessionID))
	require.NoError(t, c.GetDB().AutoMigrate(&schema.AISession{}).Error)
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(c.GetDB(), sessionID, &ypb.AIStartParams{
		EnablePlan:            false,
		SyncPerceptionTrigger: false,
		TimelineSessionID:     sessionID,
	})
	require.NoError(t, err)
	c.StartEventLoop(ctx)

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnablePlan,
		Params: &ypb.AIStartParams{
			EnablePlan: true,
		},
	})
	time.Sleep(time.Second)

	got, err := yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.True(t, got.GetEnablePlan())
	require.False(t, got.GetSyncPerceptionTrigger())

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_SyncPerceptionTrigger,
		Params: &ypb.AIStartParams{
			SyncPerceptionTrigger: true,
		},
	})
	time.Sleep(time.Second)

	got, err = yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.True(t, got.GetEnablePlan())
	require.True(t, got.GetSyncPerceptionTrigger())

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_EnablePlan,
		Params: &ypb.AIStartParams{
			EnablePlan: false,
		},
	})
	time.Sleep(time.Second)

	got, err = yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), sessionID)
	require.NoError(t, err)
	require.False(t, got.GetEnablePlan())
	require.True(t, got.GetSyncPerceptionTrigger())
}
