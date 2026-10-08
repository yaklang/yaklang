package bin_parser

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func nhrpNativeTestBody(count int, mixed bool) []byte {
	var body []byte
	for i := 0; i < count; i++ {
		lengths := [3]byte{}
		if mixed {
			switch i % 4 {
			case 0:
				lengths = [3]byte{0x40, 0x40, 0}
			case 1:
				lengths = [3]byte{0x43, 2, 5}
			case 2:
				lengths = [3]byte{0, 0x41, 3}
			case 3:
				lengths = [3]byte{4, 0, 0}
			}
		}
		body = append(body, byte(i%16), byte(17+i%16), 0xa1, 0x23, 5, 0xcd, 1, 0x9d, lengths[0], lengths[1], lengths[2], byte(i+7))
		for j := 0; j < int(lengths[0]&63)+int(lengths[1]&63)+int(lengths[2]); j++ {
			body = append(body, byte(i*11+j*13+7))
		}
	}
	return body
}

func nhrpNativeTestMessage(count int, mixed bool) []byte {
	return nhrpTestSeal(append(nhrpTestMessage(2, nil)[:40], nhrpNativeTestBody(count, mixed)...))
}

func nhrpNativeInfos(n *base.Node) []any {
	values := []any{n.Cfg.GetItem("additionInfo")}
	for _, child := range n.Children {
		values = append(values, nhrpNativeInfos(child)...)
	}
	return values
}

func TestProtocolCorpusNHRPNativeDifferential(t *testing.T) {
	for _, count := range []int{1, 2, 8, 128} {
		for _, extension := range []bool{false, true} {
			for _, imported := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/extension-%t/import-%t", count, extension, imported), func(t *testing.T) {
					wire := nhrpNativeTestMessage(count, true)
					if extension {
						ext := nhrpTestExtension(4, nhrpNativeTestBody(count, true))
						ext = append(ext, 0x80, 0, 0, 0)
						wire = nhrpTestMessage(2, ext)
					}
					input, rule, entry := wire, "nhrp", "NHRP"
					if imported {
						input, rule, entry = nhrpTestEthernet(wire), "ethernet", "Ethernet"
					}
					var roots [2]*base.Node
					for i, legacy := range []bool{true, false} {
						root, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(input), rule, map[string]any{"nhrpClientsLegacy": legacy}, entry)
						require.NoError(t, err)
						roots[i] = root
					}
					oldTree, oldResult := dicomNativeSnapshot(t, roots[0], input)
					newTree, newResult := dicomNativeSnapshot(t, roots[1], input)
					require.Equal(t, oldTree, newTree)
					require.Equal(t, oldResult, newResult)
					require.Equal(t, nhrpNativeInfos(roots[0]), nhrpNativeInfos(roots[1]))
				})
			}
		}
	}
}

func TestProtocolCorpusNHRPNativeRejectionsAndTransactions(t *testing.T) {
	valid := nhrpNativeTestMessage(2, true)
	var bad [][]byte
	// Every nonempty short prefix, including incomplete second CIE/address.
	for cut := 1; cut < len(valid); cut++ {
		// Re-sealing exactly at a complete CIE boundary makes a shorter valid
		// packet, not a truncation. The zero-CIE case is checked by the rule.
		if cut == 40 || cut == 52 {
			continue
		}
		v := bytes.Clone(valid[:cut])
		if cut >= 14 {
			nhrpTestSeal(v)
		}
		bad = append(bad, v)
	}
	for _, field := range []int{48, 49, 60, 61} {
		v := bytes.Clone(valid)
		v[field] |= 128
		bad = append(bad, nhrpTestSeal(v))
	}
	bad = append(bad, nhrpTestSeal(append(bytes.Clone(valid), 0)))
	for index, wire := range bad {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			var roots []*base.Node
			frame := nhrpTestEthernet(wire)
			for _, legacy := range []bool{true, false} {
				_, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "nhrp", map[string]any{"nhrpClientsLegacy": legacy}, "NHRP")
				require.Error(t, err)
				root, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(frame), "ethernet", map[string]any{"nhrpClientsLegacy": legacy}, "Ethernet")
				require.NoError(t, err)
				roots = append(roots, root)
				protocolCorpusRequireValue(t, root, "NHRP Payload", wire)
				require.Equal(t, frame, NodeToBytes(root))
			}
			oldTree, oldResult := dicomNativeSnapshot(t, roots[0], frame)
			newTree, newResult := dicomNativeSnapshot(t, roots[1], frame)
			require.Equal(t, oldTree, newTree)
			require.Equal(t, oldResult, newResult)
			require.Equal(t, nhrpNativeInfos(roots[0]), nhrpNativeInfos(roots[1]))
		})
	}
	for _, wire := range [][]byte{valid, bad[len(bad)-1]} {
		for _, imported := range []bool{false, true} {
			for _, legacy := range []bool{true, false} {
				input, rule, entry := wire, "nhrp.yaml", "NHRPCarrier"
				if imported {
					input, rule, entry = nhrpTestEthernet(wire), "ethernet.yaml", "Ethernet"
				}
				root, err := base.ParseRule(rule)
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(len(input))*8)
				root.Ctx.SetItem("nhrpClientsLegacy", legacy)
				root.Ctx.SetItem(base.CtxInputConfig, map[string]any{"nhrpClientsLegacy": legacy})
				reader := base.NewBitReader(bytes.NewReader(input))
				require.NoError(t, root.ParseSubNode(reader, entry))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				require.ErrorContains(t, reader.PopBackup(), "no backup")
				_, err = reader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			}
		}
	}
}

// Same binary, same wire and public entry; the caller flag selects the original
// YAML oracle. Use -benchtime=1x for the expensive 4096-entry legacy case.
func BenchmarkNHRPClientList(b *testing.B) {
	for _, count := range []int{1, 32, 128, 4096} {
		for _, legacy := range []bool{true, false} {
			b.Run(fmt.Sprintf("%d/legacy-%t", count, legacy), func(b *testing.B) {
				wire := nhrpNativeTestMessage(count, false)
				config := map[string]any{"nhrpClientsLegacy": legacy}
				// Warm rule/VM definitions with one CIE, not another full expensive list.
				if _, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(nhrpNativeTestMessage(1, false)), "nhrp", config, "NHRP"); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(wire)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					n, err := parser.ParseBinaryWithConfig(newProtocolCorpusBoundedReader(wire), "nhrp", config, "NHRP")
					if err != nil {
						b.Fatal(err)
					}
					clients := protocolCorpusFindNode(n, "Clients")
					if clients == nil || len(clients.Children) != count {
						b.Fatal("incomplete CIE tree")
					}
				}
			})
		}
	}
}
