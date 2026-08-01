package loop_asset_intake

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

type recordingAssetSink struct {
	assets []aicommon.AssetResult
}

func (s *recordingAssetSink) SubmitRisk(context.Context, *schema.Risk) (aicommon.ResultReceipt, error) {
	return aicommon.ResultReceipt{}, nil
}

func (s *recordingAssetSink) SubmitAsset(_ context.Context, asset aicommon.AssetResult) (aicommon.ResultReceipt, error) {
	s.assets = append(s.assets, asset)
	return aicommon.ResultReceipt{ResultID: "asset-accepted"}, nil
}

func TestAssetIntakeLoopIsRegistered(t *testing.T) {
	_, ok := reactloops.GetLoopFactory(schema.AI_REACT_LOOP_NAME_ASSET_INTAKE)
	require.True(t, ok)
	meta, ok := reactloops.GetLoopMetadata(schema.AI_REACT_LOOP_NAME_ASSET_INTAKE)
	require.True(t, ok)
	require.Equal(t, "SaaS Asset Intake", meta.VerboseName)
}

func TestSubmitDeclaredHTTPAssetPublishesMetadataWithoutVerificationClaim(t *testing.T) {
	sink := &recordingAssetSink{}
	receipt, asset, err := submitDeclaredHTTPAsset(
		context.Background(),
		sink,
		" HTTP://LOCAL-SERVICE:8080/path?token=redacted#section ",
		"Local sample service",
	)
	require.NoError(t, err)
	require.Equal(t, "asset-accepted", receipt.ResultID)
	require.Equal(t, "http://local-service:8080/path", asset.Target)
	require.Equal(t, "http_endpoint:declared:http://local-service:8080/path", asset.IdentityKey)
	require.Len(t, sink.assets, 1)

	var payload declaredHTTPAssetPayload
	require.NoError(t, json.Unmarshal(asset.Payload, &payload))
	require.Equal(t, "operator_declared", payload.Source)
	require.Equal(t, "declared", payload.VerificationState)
	require.False(t, payload.NetworkAccessPerformed)
	require.Equal(t, "local-service", payload.Host)
	require.Equal(t, "8080", payload.Port)
}

func TestSubmitDeclaredHTTPAssetRejectsCredentialsAndMissingSink(t *testing.T) {
	_, _, err := submitDeclaredHTTPAsset(context.Background(), &recordingAssetSink{}, "http://user:secret@local-service/", "")
	require.ErrorContains(t, err, "must not contain credentials")

	_, _, err = submitDeclaredHTTPAsset(context.Background(), nil, "http://local-service/", "")
	require.ErrorContains(t, err, "result sink is unavailable")

	_, _, err = submitDeclaredHTTPAsset(context.Background(), &recordingAssetSink{}, "http://local-service/", string(make([]byte, maxDisplayNameBytes+1)))
	require.ErrorContains(t, err, "display_name exceeds")
}
