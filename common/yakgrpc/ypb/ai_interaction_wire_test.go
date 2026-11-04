package ypb_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestAIClientUIAndNetworkDebugWireContract(t *testing.T) {
	// Keep the same fixtures as Yakit's aiSingleModelProto.test.js. Field 52
	// belongs to NetworkDebug; ClientUI must use 53 in both repositories.
	for _, tc := range []struct {
		name    string
		enabled bool
		wire    string
	}{
		{"disabled", false, "08011208a00300aa03027b7d"},
		{"enabled", true, "08011208a00301aa03027b7d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := hex.DecodeString(tc.wire)
			require.NoError(t, err)
			var decoded ypb.AIInputEvent
			require.NoError(t, proto.Unmarshal(wire, &decoded))
			require.True(t, decoded.IsStart)
			require.NotNil(t, decoded.Params)
			require.NotNil(t, decoded.Params.NetworkDebug)
			require.Equal(t, tc.enabled, decoded.Params.GetNetworkDebug())
			require.Equal(t, "{}", decoded.Params.ClientUI)

			encoded, err := proto.Marshal(&ypb.AIInputEvent{
				IsStart: true,
				Params:  &ypb.AIStartParams{NetworkDebug: proto.Bool(tc.enabled), ClientUI: "{}"},
			})
			require.NoError(t, err)
			require.Equal(t, wire, encoded)
		})
	}
}
