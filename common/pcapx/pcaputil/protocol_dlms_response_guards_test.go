package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func hdlcForeignResponseControls(t *testing.T) []hdlcOpCase {
	t.Helper()
	var x struct{ Cases []hdlcOpCase }
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-response-guards", "DLMS_HDLC_RESPONSE_GUARDS_TEST_ROOT", "controls.json"), &x))
	require.Len(t, x.Cases, 2)
	return x.Cases
}
func TestDLMSHDLCForeignResponseNative(t *testing.T) {
	hdlcOpNative(t, hdlcForeignResponseControls(t), 41000, 4059)
}
func TestDLMSHDLCForeignResponseOffPort(t *testing.T) {
	hdlcOpNative(t, hdlcForeignResponseControls(t)[:2], 45000, 45001)
}
func TestDLMSHDLCForeignResponseMatrix(t *testing.T) {
	for _, c := range hdlcForeignResponseControls(t) {
		for _, transport := range []string{"udp", "tcp"} {
			t.Run(c.Name+"/"+transport, func(t *testing.T) {
				var reuse map[string]string
				require.NoError(t, json.Unmarshal(finsReadBatch(t, "dlms-hdlc-response-guards", "DLMS_HDLC_RESPONSE_GUARDS_TEST_ROOT", "reuse-inputs.json"), &reuse))
				name := c.Name + "." + transport + ".pcap"
				raw := []byte(nil)
				if path, ok := reuse[name]; ok {
					parts := strings.SplitN(path, "/", 2)
					require.Len(t, parts, 2)
					raw = finsReadBatch(t, parts[0], "DLMS_HDLC_RESPONSE_GUARDS_TEST_ROOT", parts[1])
				} else {
					raw = finsReadBatch(t, "dlms-hdlc-response-guards", "DLMS_HDLC_RESPONSE_GUARDS_TEST_ROOT", name)
				}
				discoveryMatrix(t, func(t *testing.T, w int, d, o bool) {
					es, stats := hdlcOpReplay(t, c, raw, w, d, o, transport)
					count := len(c.Steps)
					if transport == "tcp" {
						for _, s := range c.Steps {
							if s.TCPOverride != nil {
								count--
							}
						}
					}
					closeCount := len(c.Close.Events)
					if transport == "tcp" && c.TCPCloseOverride != nil {
						closeCount = 0
					}
					require.Len(t, es, count+closeCount)
					hdlcOpCheck(t, c, es[:count], transport, true, d)
					hdlcOpCloseCheck(t, c, es[count:], transport, "ctx-canceled")
					require.EqualValues(t, count, stats.Messages)
					require.EqualValues(t, closeCount, stats.Incomplete)
				})
			})
		}
	}
}

// Only the exact archived orphan Action-next input receives a full new answer.
// The original ZIP remains immutable. This preserves the negative ownership
// case while allowing the newly implemented wire service to decode.
func actionDownloadHistoricalUpgrade(t *testing.T, original []byte, cases []actionCase) []actionCase {
	t.Helper()
	require.Equal(t, "db709d3bd80d4a51be2cbc40f22d0a6e74961f8e2b2f9588012e83122f574487", fmt.Sprintf("%x", sha256.Sum256(original)))
	raw := finsReadBatch(t, "dlms-response-ownership", "DLMS_RESPONSE_OWNERSHIP_TEST_ROOT", "historical-next-feature-upgrade.json")
	require.Equal(t, "00961159319c81063a7c0e3736136ba7332637cd2def69ab0857ccbb7bd51c70", fmt.Sprintf("%x", sha256.Sum256(raw)))
	var upgrade struct{ Case json.RawMessage }
	require.NoError(t, json.Unmarshal(raw, &upgrade))
	var before struct{ Cases []map[string]any }
	require.NoError(t, json.Unmarshal(original, &before))
	require.Len(t, before.Cases, 8)
	require.Len(t, cases, 8)
	var next map[string]any
	require.NoError(t, json.Unmarshal(upgrade.Case, &next))
	found := 0
	for i, c := range cases {
		if c.Name != "Action-next-is-not-Get-next" {
			continue
		}
		found++
		require.Len(t, c.Steps, 1)
		require.Equal(t, "b9f2f75d8b04301cc666c1a77379d3ebf063f399c85b4cdb12c9619b8ff6ae71", c.Steps[0].SHA)
		require.Equal(t, "UnsupportedFeature", c.Steps[0].Error)
		old := before.Cases[i]
		oldStep := old["steps"].([]any)
		newStep := next["steps"].([]any)
		require.Len(t, oldStep, 1)
		require.Len(t, newStep, 1)
		a, b := oldStep[0].(map[string]any), newStep[0].(map[string]any)
		for _, key := range []string{"Decode", "GetFields", "completeness", "error", "fields", "profile", "query_error", "session", "status"} {
			delete(a, key)
			delete(b, key)
		}
		require.Equal(t, a, b, "Every input, owner, FlowID, Raw and unknown step key remains unchanged")
		require.Equal(t, old, next, "Name, ports, budget, Close and ownership contract remain unchanged")
		var replacement actionCase
		require.NoError(t, json.Unmarshal(upgrade.Case, &replacement))
		require.Empty(t, replacement.Steps[0].Error)
		require.Equal(t, "dlms-wrapper-v1-action-download", replacement.Steps[0].Profile)
		require.Equal(t, "unmatched-block-request", replacement.Steps[0].Session["Association"])
		require.Zero(t, replacement.Steps[0].Transaction)
		require.Zero(t, replacement.Steps[0].Response)
		require.Empty(t, replacement.Close.Events)
		cases[i] = replacement
	}
	require.Equal(t, 1, found)
	return cases
}

func TestDLMSActionDownloadHistoricalUpgradeBinding(t *testing.T) {
	raw := finsReadBatch(t, "dlms-action-normal", "DLMS_ACTION_TEST_ROOT", "supplemental.json")
	var original struct{ Cases []actionCase }
	require.NoError(t, json.Unmarshal(raw, &original))
	var independent struct{ Cases []actionCase }
	require.NoError(t, json.Unmarshal(raw, &independent))
	got := actionDownloadHistoricalUpgrade(t, raw, independent.Cases)
	unchanged := 0
	for i, c := range original.Cases {
		if c.Name != "Action-next-is-not-Get-next" {
			require.Equal(t, c, got[i])
			unchanged++
		}
	}
	require.Equal(t, 7, unchanged)
}
