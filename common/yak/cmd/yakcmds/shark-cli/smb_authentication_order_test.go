package sharkcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Use the same real Type3 envelope as the existing conversation regression.
// A retained summary alone cannot prove that identity meaning remains ahead
// of the complete raw tree, or that token bytes still have their own source.
func TestSMBAuthenticationMeaningPrecedesRawTreeAcrossWorkers(t *testing.T) {
	packets, _ := top30Replay(t, top30Cases()[16])
	for _, packet := range packets {
		if !strings.Contains(summarize(packet).Info, "AUTHENTICATE") {
			continue
		}
		events := packet.protocolEvents()
		var wire, decoded, mechanism []byte
		for _, event := range events {
			if event == nil || event.Protocol != "smb2" {
				continue
			}
			auth, _ := event.Session["Authentication"].(map[string]any)
			child, _ := auth["Mechanism"].(map[string]any)
			if child["Message Name"] == "AUTHENTICATE" && child["User Name"] == "root" {
				wire = bytes.Clone(event.Raw)
				decoded, _ = auth["Decoded Bytes"].([]byte)
				mechanism, _ = auth["Mechanism Token"].([]byte)
				require.NotEmpty(t, auth["Decoded Fields"], "the full raw authentication tree remains part of the input")
				break
			}
		}
		require.NotEmpty(t, wire, "the real SMB SESSION_SETUP PDU is retained")
		require.NotEmpty(t, decoded, "the exact decoded GSS token is retained")
		require.NotEmpty(t, mechanism, "the exact NTLM mechanism token is retained")
		require.False(t, bytes.Equal(wire, decoded), "SMB framing and authentication token are separate owned byte sources")
		require.False(t, bytes.Equal(decoded, mechanism), "GSS wrapper and mechanism token are distinct source scopes")
		before, err := json.Marshal(events)
		require.NoError(t, err)
		localFields, localSources := protocolEventDetail(events)
		views := []struct {
			name   string
			detail packetDetail
		}{
			{"local", packetDetail{fields: localFields, sources: localSources}},
			{"packet-worker", workerMetadataDetail(t, events, false)},
			{"stream-worker", workerMetadataDetail(t, events, true)},
		}
		for _, view := range views {
			t.Run(view.name, func(t *testing.T) {
				username, raw, rawBranches := -1, -1, 0
				decodedSource, mechanismSource := 0, 0
				for i, item := range view.detail.fields {
					if strings.HasPrefix(item.text, "User Name: root") {
						username = i
						require.False(t, item.hasRange, "semantic identity must not borrow an outer SMB range")
						require.Zero(t, item.source)
					}
					if strings.HasPrefix(item.text, "Decoded Fields[") {
						if raw < 0 {
							raw = i
						}
						rawBranches++
					}
					var expected []byte
					switch {
					case strings.HasPrefix(item.text, "Decoded Bytes:"):
						expected = decoded
						decodedSource = item.source
					case strings.HasPrefix(item.text, "Mechanism Token:"):
						expected = mechanism
						mechanismSource = item.source
					}
					if expected != nil {
						require.True(t, item.hasRange, "owned token bytes remain selectable")
						require.Positive(t, item.source)
						require.Zero(t, item.start)
						require.Equal(t, len(expected), item.end)
						require.Equal(t, expected, view.detail.sources[item.source-1].Data)
						require.Equal(t, "Authentication token · decoded bytes", view.detail.sources[item.source-1].Name)
					}
				}
				require.NotEqual(t, -1, username)
				require.NotEqual(t, -1, raw)
				require.Less(t, username, raw, "the existing meaning-before-raw contract applies inside SMB authentication maps")
				require.Positive(t, rawBranches, "the full raw tree is shown, not dropped to satisfy order")
				require.Positive(t, decodedSource)
				require.Positive(t, mechanismSource)
				require.NotEqual(t, decodedSource, mechanismSource)
				foundWire := false
				for _, source := range view.detail.sources {
					if bytes.Equal(source.Data, wire) {
						foundWire = true
						require.Contains(t, source.Name, "SMB2 wire PDU (authentication)")
					}
				}
				require.True(t, foundWire, "the complete SMB wire PDU is preserved alongside both token sources")
				require.Contains(t, detailLines(view.detail.fields), "Authentication Verified: false")
				require.Contains(t, detailLines(view.detail.fields), "MIC Verified: false")
			})
		}
		after, err := json.Marshal(events)
		require.NoError(t, err)
		require.Equal(t, before, after, "presentation and worker IPC must not rewrite SDK events, authentication maps or raw bytes")
		return
	}
	t.Fatal("missing real Type 3 authentication envelope")
}

func TestSMBDetailMeaningPriorityKeepsOwnedWireOrder(t *testing.T) {
	for _, semantic := range []string{"Protocol", "Message Name", "Mechanism", "User Name", "Domain", "Identity Evidence", "Authentication Verified", "MIC Verified"} {
		for _, raw := range []string{"Decoded Bytes", "Mechanism Token", "Decoded Fields"} {
			require.True(t, smbDetailFieldLess(semantic, raw, nil), semantic+" must precede "+raw)
			require.False(t, smbDetailFieldLess(raw, semantic, nil))
		}
	}
	ranges := map[string][]int{"Protocol ID": {4, 8}, "Structure Size": {8, 10}, "Command": {16, 18}, "User ID": {32, 34}}
	for _, pair := range [][2]string{{"Protocol ID", "Structure Size"}, {"Structure Size", "Command"}, {"Command", "User ID"}} {
		require.True(t, smbDetailFieldLess(pair[0], pair[1], ranges), "physical SMB Header fields keep their owned wire order")
	}
}
