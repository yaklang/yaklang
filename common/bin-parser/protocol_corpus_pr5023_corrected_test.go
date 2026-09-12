package bin_parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// The incomplete PR #5023 messages remain immutable negative inputs. Each
// has a separate deterministic positive companion. Inspect every capture
// record, parse every full Ethernet envelope, and validate the single message
// record against the same application rule and extraction contract.
func TestProtocolCorpusPR5023CorrectedCompanionsEveryRecordAndBoundary(t *testing.T) {
	const corpusDir = "testdata/protocol-corpus"
	var manifest protocolCorpusManifest
	readProtocolCorpusJSON(t, filepath.Join(corpusDir, "manifest.json"), &manifest)
	captures := make(map[string]protocolCorpusCapture, len(manifest.Captures))
	for _, capture := range manifest.Captures {
		captures[capture.ID] = capture
	}

	for _, pair := range []struct {
		name               string
		negativeID         string
		positiveID         string
		retainedNegativeID string
	}{
		{name: "NBT SS", negativeID: "pr5023-gen-nbt-ss", positiveID: "gen-nbt-ss-valid"},
		{name: "Java serialization", negativeID: "pr5023-gen-java-ser", positiveID: "gen-java-ser-valid"},
		{name: "NTLMSSP", negativeID: "pr5023-gen-ntlmssp", positiveID: "gen-ntlmssp-valid"},
		{name: "NTLM", negativeID: "pr5023-gen-ntlm", positiveID: "gen-ntlm-valid"},
		{name: "NetNTLMv2", negativeID: "pr5023-gen-netntlmv2", positiveID: "gen-netntlmv2-valid"},
		{name: "NTLM v1/v2", negativeID: "pr5023-gen-ntlm-v2", positiveID: "gen-ntlm-v2-valid"},
		{name: "SPNEGO", negativeID: "pr5023-gen-spnego", positiveID: "gen-spnego-valid"},
		{name: "WinRM HTTP", negativeID: "pr5023-gen-winrm-http", positiveID: "gen-winrm-identify-valid", retainedNegativeID: "gen-winrm-http-valid"},
		{name: "MSDP", negativeID: "pr5023-gen-msdp", positiveID: "gen-msdp-valid"},
		{name: "X11", negativeID: "pr5023-gen-x11", positiveID: "gen-x11-valid"},
		{name: "iSCSI", negativeID: "pr5023-gen-iscsi", positiveID: "gen-iscsi-valid"},
		{name: "UCP/EMI", negativeID: "pr5023-gen-ucp", positiveID: "gen-ucp-valid"},
	} {
		pair := pair
		t.Run(pair.name, func(t *testing.T) {
			contract := protocolCorpusParseContracts[pair.name]
			ids := []string{pair.negativeID, pair.positiveID}
			if pair.retainedNegativeID != "" {
				ids = append(ids, pair.retainedNegativeID)
			}
			for _, id := range ids {
				if id != pair.positiveID {
					spec, ok := protocolCorpusRejectionSpecs[id]
					require.True(t, ok, "missing rejection contract for %s", id)
					require.Equal(t, pair.positiveID, spec.ControlCaptureID, "companion must match the rejection contract")
					require.Equal(t, contract, spec.Contract)
					require.Equal(t, contract, spec.ControlContract)
				}
				capture, ok := captures[id]
				require.True(t, ok, "capture missing from manifest: %s", id)
				frames := protocolCorpusAuditPackets(t, filepath.Join(corpusDir, capture.CaptureFile))
				require.Len(t, frames, 4, "%s must retain all TCP records", id)
				require.Equal(t, 4, capture.RepresentativeFrame.Number)
				controls, messages := 0, 0
				for index, frame := range frames {
					t.Run(fmt.Sprintf("%s/frame-%d", id, index+1), func(t *testing.T) {
						protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
						packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
						require.Nil(t, packet.ErrorLayer())
						tcpLayer := packet.Layer(layers.LayerTypeTCP)
						require.NotNil(t, tcpLayer)
						tcp := tcpLayer.(*layers.TCP)
						if len(tcp.Payload) == 0 {
							controls++
							require.LessOrEqual(t, index, 2)
							require.Equal(t, index < 2, tcp.SYN)
							require.Equal(t, index > 0, tcp.ACK)
							return
						}
						messages++
						require.Equal(t, 3, index)
						info := ProtocolInfo{Name: pair.name, Layer: contract.Layer, RuleFile: contract.RuleFile}
						input := protocolCorpusParseInput(t, corpusDir, capture, info, contract)
						if len(contract.Base64After) == 0 {
							require.Equal(t, tcp.Payload, input)
						} else {
							encoded := append(bytes.Clone(contract.Base64After), []byte(base64.StdEncoding.EncodeToString(input))...)
							require.True(t, bytes.Contains(tcp.Payload, encoded), "%s message record does not contain its decoded input", id)
						}
						if id == pair.positiveID {
							node := protocolCorpusRequireBoundedContractParse(t, input, contract)
							protocolCorpusRequireCorrectedCompanionFields(t, pair.name, node)
							for cut := 0; cut < len(input); cut++ {
								reader := newProtocolCorpusBoundedReader(input[:cut])
								_, err := protocolCorpusParseRule(reader, contract)
								require.Errorf(t, err, "%s accepted truncated message at %d/%d", id, cut, len(input))
							}
							return
						}
						spec := protocolCorpusRejectionSpecs[id]
						reader := newProtocolCorpusBoundedReader(input)
						_, err := protocolCorpusParseRule(reader, contract)
						protocolCorpusRequireExpectedFailure(t, id, spec, err)
					})
				}
				require.Equal(t, 3, controls)
				require.Equal(t, 1, messages)
			}
		})
	}

	for _, pair := range []struct {
		name       string
		negativeID string
		positiveID string
	}{
		{name: "GTPv2", negativeID: "pr5023-gen-gtpv2", positiveID: "gen-gtpv2-valid"},
		{name: "Quake", negativeID: "pr5023-gen-quake", positiveID: "gen-quake-valid"},
		{name: "OLSR", negativeID: "pr5023-gen-olsr", positiveID: "gen-olsr-valid"},
		{name: "IPMI RMCP+", negativeID: "pr5023-gen-ipmi-rmcpplus", positiveID: "gen-ipmi-rmcpplus-valid"},
	} {
		pair := pair
		t.Run(pair.name, func(t *testing.T) {
			contract := protocolCorpusParseContracts[pair.name]
			for _, id := range []string{pair.negativeID, pair.positiveID} {
				capture, ok := captures[id]
				require.True(t, ok, "capture missing from manifest: %s", id)
				frames := protocolCorpusAuditPackets(t, filepath.Join(corpusDir, capture.CaptureFile))
				require.Len(t, frames, 1, "%s must contain exactly one datagram", id)
				require.Equal(t, 1, capture.RepresentativeFrame.Number)
				protocolCorpusRequireBoundedRuleParse(t, frames[0], "ethernet", "Ethernet")
				packet := gopacket.NewPacket(frames[0], layers.LayerTypeEthernet, gopacket.DecodeOptions{Lazy: false, NoCopy: true})
				require.Nil(t, packet.ErrorLayer())
				udpLayer := packet.Layer(layers.LayerTypeUDP)
				require.NotNil(t, udpLayer)
				payload := udpLayer.(*layers.UDP).Payload
				info := ProtocolInfo{Name: pair.name, Layer: contract.Layer, RuleFile: contract.RuleFile}
				input := protocolCorpusParseInput(t, corpusDir, capture, info, contract)
				require.Equal(t, payload, input)
				if id == pair.positiveID {
					node := protocolCorpusRequireBoundedContractParse(t, input, contract)
					protocolCorpusRequireCorrectedCompanionFields(t, pair.name, node)
					for cut := 0; cut < len(input); cut++ {
						reader := newProtocolCorpusBoundedReader(input[:cut])
						_, err := protocolCorpusParseRule(reader, contract)
						require.Errorf(t, err, "%s accepted truncated message at %d/%d", id, cut, len(input))
					}
					continue
				}
				spec := protocolCorpusRejectionSpecs[id]
				reader := newProtocolCorpusBoundedReader(input)
				_, err := protocolCorpusParseRule(reader, contract)
				protocolCorpusRequireExpectedFailure(t, id, spec, err)
			}
		})
	}
}

func protocolCorpusRequireBoundedContractParse(t *testing.T, input []byte, contract protocolCorpusParseContract) *base.Node {
	t.Helper()
	reader := newProtocolCorpusBoundedReader(input)
	node, err := protocolCorpusParseRule(reader, contract)
	require.NoError(t, err)
	require.NotNil(t, node)
	require.Zero(t, reader.Len(), "message parser left bytes unread")
	return node
}

func protocolCorpusRequireCorrectedCompanionFields(t *testing.T, name string, node *base.Node) {
	t.Helper()
	switch name {
	case "NBT SS":
		protocolCorpusRequireValue(t, node, "Type", uint64(0x81))
		protocolCorpusRequireValue(t, node, "Flags", uint64(0))
		protocolCorpusRequireValue(t, node, "Length", uint64(68))
		require.Len(t, protocolCorpusNodesNamed(node, "Encoded Name"), 2)
		require.Len(t, protocolCorpusNodesNamed(node, "Terminator"), 2)
	case "Java serialization":
		protocolCorpusRequireValue(t, node, "Magic", uint64(0xaced))
		protocolCorpusRequireValue(t, node, "Version", uint64(5))
		protocolCorpusRequireValue(t, node, "Content Type", uint64(0x70))
	case "NTLMSSP":
		protocolCorpusRequireValue(t, node, "MessageType", uint64(1))
		protocolCorpusRequireValue(t, node, "NegotiateFlags", uint64(0x00000201))
		for _, field := range []string{"DomainNameFields", "WorkstationFields"} {
			descriptor := protocolCorpusFindNode(node, field)
			protocolCorpusRequireValue(t, descriptor, "Length", uint64(0))
			protocolCorpusRequireValue(t, descriptor, "BufferOffset", uint64(32))
		}
	case "NTLM":
		protocolCorpusRequireValue(t, node, "MessageType", uint64(2))
		protocolCorpusRequireValue(t, node, "NegotiateFlags", uint64(0x00000201))
		protocolCorpusRequireValue(t, node, "ServerChallenge", []byte{1, 2, 3, 4, 5, 6, 7, 8})
	case "NetNTLMv2", "NTLM v1/v2":
		protocolCorpusRequireValue(t, node, "MessageType", uint64(3))
		protocolCorpusRequireValue(t, node, "NegotiateFlags", uint64(0x00088201))
		descriptor := protocolCorpusFindNode(node, "NtChallengeResponseFields")
		protocolCorpusRequireValue(t, descriptor, "Length", uint64(44))
		protocolCorpusRequireValue(t, descriptor, "BufferOffset", uint64(64))
		protocolCorpusRequireValue(t, node, "NTProofStr", bytes.Repeat([]byte{0x11}, 16))
		protocolCorpusRequireValue(t, node, "RespType", uint64(1))
		protocolCorpusRequireValue(t, node, "HiRespType", uint64(1))
		protocolCorpusRequireValue(t, node, "ClientChallenge", []byte{9, 8, 7, 6, 5, 4, 3, 2})
	case "SPNEGO":
		protocolCorpusRequireValue(t, node, "Tag", uint64(0x60))
		protocolCorpusRequireValue(t, node, "OID", string([]byte{0x2b, 0x06, 0x01, 0x05, 0x05, 0x02}))
		protocolCorpusRequireValue(t, node, "MechOID", string([]byte{0x2b, 0x06, 0x01, 0x04, 0x01, 0x82, 0x37, 0x02, 0x02, 0x0a}))
	case "WinRM HTTP":
		protocolCorpusRequireValue(t, node, "Method", "POST")
		protocolCorpusRequireValue(t, node, "Path", "/wsman")
		protocolCorpusRequireValue(t, node, "Content Length", uint64(190))
		identify := protocolCorpusFindNode(node, "Identify")
		require.NotNil(t, identify, "HTTP alone is not a decoded WS-Management Identify message")
		require.Equal(t, "http://schemas.dmtf.org/wbem/wsman/identify/1/wsmanidentity.xsd", identify.Cfg.GetItem("additionInfo").(map[string]any)["Namespace"])
	case "MSDP":
		protocolCorpusRequireValue(t, node, "Type", uint64(4))
		protocolCorpusRequireValue(t, node, "Length", uint64(3))
	case "X11":
		protocolCorpusRequireValue(t, node, "Byte Order", uint64('l'))
		protocolCorpusRequireValue(t, node, "Protocol Major", uint64(11))
		protocolCorpusRequireValue(t, node, "Authorization Name Length", uint64(0))
	case "iSCSI":
		protocolCorpusRequireValue(t, node, "Opcode", uint64(0x43))
		protocolCorpusRequireValue(t, node, "Data Length Low", uint64(16))
	case "UCP/EMI":
		protocolCorpusRequireValue(t, node, "Length Text", "00020")
		protocolCorpusRequireValue(t, node, "Checksum", "7E")
	case "GTPv2":
		protocolCorpusRequireValue(t, node, "Flags", uint64(0x40))
		protocolCorpusRequireValue(t, node, "Payload Length", uint64(4))
	case "Quake":
		protocolCorpusRequireValue(t, node, "Length and Flags", uint64(0x8000000a))
	case "OLSR":
		protocolCorpusRequireValue(t, node, "Packet Length", uint64(4))
		protocolCorpusRequireValue(t, node, "Packet Sequence Number", uint64(1))
	case "IPMI RMCP+":
		protocolCorpusRequireValue(t, node, "Authentication Type", uint64(6))
		protocolCorpusRequireValue(t, node, "Payload Type", uint64(0x10))
		protocolCorpusRequireValue(t, node, "Payload Length", uint64(32))
	default:
		t.Fatalf("missing companion assertions for %s", name)
	}
}
