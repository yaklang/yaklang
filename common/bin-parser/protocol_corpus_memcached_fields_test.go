package bin_parser

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"os"
	"strings"
	"testing"
)

const memcachedFieldsTestRule = "application-layer.memcached_fields"

func TestProtocolCorpusMemcachedFieldsPackedCarrier(t *testing.T) {
	for entry, wire := range memcachedFieldsTestMessages(t) {
		for offset := uint64(0); offset < 8; offset++ {
			for _, good := range []bool{true, false} {
				w := bytes.Clone(wire)
				if !good {
					w = append(w, 0)
				}
				var packed bytes.Buffer
				bw := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, bw.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, bw.WriteBits(w, uint64(len(w))*8))
				require.NoError(t, bw.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, bw.WriteBits([]byte{0}, 8-offset))
				}
				root := latTestInline(t, fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Message\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Message\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Message: \"import:application-layer/memcached_fields.yaml;node:%sCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(w), offset, offset, entry, 8-offset))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("marker", 123)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, r.Backup())
				require.NoError(t, root.ParseSubNode(r, "Wrapped"))
				n := protocolCorpusFindNode(root, "Message")
				tlsCertificateTestTree(t, n, w, offset)
				if good {
					require.Nil(t, protocolCorpusFindNode(n, "Unparsed Memcached Wire"))
				} else {
					protocolCorpusRequireValue(t, n, "Unparsed Memcached Wire", w)
					require.Nil(t, protocolCorpusFindNode(n, "Magic"))
					require.Nil(t, protocolCorpusFindNode(n, "Statistic"))
				}
				protocolCorpusRequireValue(t, root, "Sentinel", uint64(0xd3))
				require.Equal(t, 123, root.Ctx.GetItem("marker"))
				require.Equal(t, packed.Bytes(), NodeToBytes(base.GetNodeByPath(root, "@Wrapped")))
				require.NoError(t, r.Recovery())
				got, e := r.ReadBits(uint64(packed.Len()) * 8)
				require.NoError(t, e)
				require.Equal(t, packed.Bytes(), got)
				require.ErrorContains(t, r.PopBackup(), "no backup")
			}
		}
	}
	w := append([]byte{0x80, 0, 0, 3, 0, 0, 0x12, 0x34, 0, 0, 0, 3, 1, 2, 3, 4, 1, 2, 3, 4, 5, 6, 7, 8}, []byte{0, 255, 32}...)
	n := protocolCorpusRequireBoundedRuleParse(t, w, memcachedFieldsTestRule, "MemcachedBinaryGetRequestFields")
	tlsCertificateTestTree(t, n, w, 0)
	protocolCorpusRequireValue(t, n, "CAS", uint64(0x0102030405060708))
	protocolCorpusRequireValue(t, n, "VBucket ID", uint64(0x1234))
	protocolCorpusRequireValue(t, n, "Key", []byte{0, 255, 32})
	for _, w := range [][]byte{[]byte("END\r\n"), []byte("STAT x 001\r\nSTAT x value with spaces\r\nEND\r\n")} {
		n := protocolCorpusRequireBoundedRuleParse(t, w, memcachedFieldsTestRule, "MemcachedStatsResponseFields")
		tlsCertificateTestTree(t, n, w, 0)
		if len(w) > 5 {
			require.Len(t, protocolCorpusNodesNamed(n, "Statistic"), 2)
		} else {
			require.Equal(t, []any{}, NodeToMap(protocolCorpusFindNode(n, "Statistics")))
		}
	}
}

var memcachedFieldsTestCaptures = []struct {
	path, sha string
	records   int
}{
	{"ndpi/ndpi-memcached.cap", "3a74dd7c9f97e5a7ff75d201014accb76b673d5e13579ddf5faa8bf3844d17bd", 10},
	{"generated-local/gen-memcache-bin.pcap", "8ef5179f84123ec6a6fe435fd11a5b3291b5c6f60c8c441edf7980bead85e514", 4},
	{"generated-pr5023/pr5023-gen-memcache-bin.pcap", "a08ee0943de03aad2624f017cf876a2091ea07e776c67f90dba70184042479e2", 4},
}

func memcachedFieldsTestMessages(t *testing.T) map[string][]byte {
	r := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-memcached.cap")
	binary := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-memcache-bin.pcap")
	return map[string][]byte{"MemcachedStatsRequestFields": r[3][66:], "MemcachedStatsResponseFields": r[5][66:], "MemcachedBinaryGetRequestFields": binary[3][54:]}
}

func TestProtocolCorpusMemcachedFieldsOriginalRecords(t *testing.T) {
	total, controls, payloads, wireBytes := 0, 0, 0, 0
	for ci, c := range memcachedFieldsTestCaptures {
		path := "testdata/protocol-corpus/captures/" + c.path
		raw, e := os.ReadFile(path)
		require.NoError(t, e)
		require.Equal(t, c.sha, tlsCertificateTestSHA(raw))
		records := protocolCorpusAuditPackets(t, path)
		require.Len(t, records, c.records)
		seqs := []uint32{1, 1000, 2, 2}
		acks := []uint32{0, 2, 1001, 1001}
		lengths := []int{0, 0, 0, 27}
		if ci == 0 {
			seqs = []uint32{747757264, 3408297082, 747757265, 747757265, 3408297083, 3408297083, 747757272, 747757272, 3408298111, 747757273}
			acks = []uint32{0, 747757265, 3408297083, 3408297083, 747757272, 747757272, 3408298111, 3408298111, 747757273, 3408298112}
			lengths = []int{0, 0, 0, 7, 0, 1028, 0, 0, 0, 0}
		}
		for i, record := range records {
			total++
			p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
			require.Nil(t, p.ErrorLayer())
			ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
			tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
			require.Equal(t, seqs[i], tcp.Seq)
			require.Equal(t, acks[i], tcp.Ack)
			require.Equal(t, i < 2, tcp.SYN)
			require.Equal(t, i != 0, tcp.ACK)
			require.False(t, tcp.RST)
			require.Equal(t, ci == 0 && (i == 7 || i == 8), tcp.FIN)
			w := tcp.Payload
			require.Len(t, w, lengths[i])
			require.Equal(t, len(w) > 0, tcp.PSH)
			offset := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
			require.Equal(t, record[offset:14+int(ip.Length)], w)
			if len(w) == 0 {
				controls++
				continue
			}
			payloads++
			wireBytes += len(w)
			entry := "MemcachedBinaryGetRequestFields"
			if ci == 0 {
				if i == 3 {
					entry = "MemcachedStatsRequestFields"
				} else {
					entry = "MemcachedStatsResponseFields"
				}
			}
			t.Run(fmt.Sprintf("%s/frame-%d", c.path, i+1), func(t *testing.T) {
				n := protocolCorpusRequireBoundedRuleParse(t, w, memcachedFieldsTestRule, entry)
				tlsCertificateTestTree(t, n, w, 0)
				smtpFieldsTestWhole(t, record, offset, len(w), "application-layer/memcached_fields.yaml", entry)
				require.NotNil(t, NodeToMap(n))
				m := n.Cfg.GetItem("additionInfo").(map[string]any)
				require.Equal(t, false, m["Command Executed"])
				require.Equal(t, false, m["Session State Validated"])
				cassandraFieldsTestMetadataBytes(t, w, m)
				switch entry {
				case "MemcachedStatsRequestFields":
					protocolCorpusRequireValue(t, n, "Command", "stats")
					require.Equal(t, []byte("stats\r\n"), w)
				case "MemcachedBinaryGetRequestFields":
					want, e := hex.DecodeString("800000030000000000000003000000010000000000000000666f6f")
					require.NoError(t, e)
					require.Equal(t, want, w)
					for name, v := range map[string]uint64{"Magic": 128, "Opcode": 0, "Key Length": 3, "Extras Length": 0, "Data Type": 0, "VBucket ID": 0, "Total Body Length": 3, "Opaque": 1, "CAS": 0} {
						protocolCorpusRequireValue(t, n, name, v)
					}
					protocolCorpusRequireValue(t, n, "Key", []byte("foo"))
				case "MemcachedStatsResponseFields":
					want := strings.Split("pid=8837 uptime=193 time=1534343744 version=1.4.15 libevent=2.0.21-stable pointer_size=64 rusage_user=0.005382 rusage_system=0.006280 curr_connections=10 total_connections=13 connection_structures=11 reserved_fds=20 cmd_get=0 cmd_set=0 cmd_flush=0 cmd_touch=0 get_hits=0 get_misses=0 delete_misses=0 delete_hits=0 incr_misses=0 incr_hits=0 decr_misses=0 decr_hits=0 cas_misses=0 cas_hits=0 cas_badval=0 touch_hits=0 touch_misses=0 auth_cmds=0 auth_errors=0 bytes_read=21 bytes_written=2051 limit_maxbytes=67108864 accepting_conns=1 listen_disabled_num=0 threads=4 conn_yields=0 hash_power_level=16 hash_bytes=524288 hash_is_expanding=0 bytes=0 curr_items=0 total_items=0 expired_unfetched=0 evicted_unfetched=0 evictions=0 reclaimed=0", " ")
					require.Len(t, want, 48)
					var got []string
					stats := m["Statistics"].([]map[string]any)
					names, values := protocolCorpusNodesNamed(n, "Statistic Name"), protocolCorpusNodesNamed(n, "Statistic Value")
					require.Len(t, names, 48)
					require.Len(t, values, 48)
					for j, item := range stats {
						name := item["Name"].(map[string]any)["Text"].(string)
						value := item["Value"].(map[string]any)["Text"].(string)
						got = append(got, name+"="+value)
						nv, e := names[j].Result()
						require.NoError(t, e)
						require.Equal(t, name, nv.Value)
						vv, e := values[j].Result()
						require.NoError(t, e)
						require.Equal(t, value, vv.Value)
					}
					require.Equal(t, want, got)
					require.Equal(t, uint64(48), m["Statistic Count"])
					require.Equal(t, true, m["Response Complete"])
					protocolCorpusRequireValue(t, n, "Response Terminator", "END")
				}
			})
		}
	}
	require.Equal(t, 18, total)
	require.Equal(t, 14, controls)
	require.Equal(t, 4, payloads)
	require.Equal(t, 1089, wireBytes)
}

func TestProtocolCorpusMemcachedFieldsBoundariesAndFallback(t *testing.T) {
	for entry, w := range memcachedFieldsTestMessages(t) {
		for _, name := range []string{entry, entry + "Carrier"} {
			_, e := parser.ParseBinary(bytes.NewReader(w), memcachedFieldsTestRule, name)
			require.ErrorContains(t, e, "explicit")
			_, e = parser.GenerateBinary(map[string]any{}, memcachedFieldsTestRule, name)
			require.Error(t, e)
			for _, bits := range []uint64{0, 1, 7, (1<<20)*8 + 1, (1<<20 + 1) * 8} {
				r := &tlsSHTestHeldReader{bits: bits}
				_, e := parser.ParseBinary(r, memcachedFieldsTestRule, name)
				require.Error(t, e)
				require.Zero(t, r.reads)
			}
		}
		for cut := 0; cut < len(w); cut++ {
			_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(w[:cut]), memcachedFieldsTestRule, entry)
			require.Error(t, e, "entry %s cut %d", entry, cut)
		}
		joined := append(bytes.Clone(w), w...)
		smtpFieldsTestWhole(t, joined, 0, len(w), "application-layer/memcached_fields.yaml", entry)
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(joined), memcachedFieldsTestRule, entry)
		require.Error(t, e)
		for _, bad := range [][]byte{append(bytes.Clone(w), 0), append([]byte{255}, w[1:]...)} {
			n := protocolCorpusRequireBoundedRuleParse(t, bad, memcachedFieldsTestRule, entry+"Carrier")
			tlsCertificateTestTree(t, n, bad, 0)
			protocolCorpusRequireValue(t, n, "Unparsed Memcached Wire", bad)
			require.Nil(t, protocolCorpusFindNode(n, "Statistic"))
			require.Nil(t, protocolCorpusFindNode(n, "Magic"))
			require.Nil(t, protocolCorpusFindNode(n, "Command"))
		}
	}
	for worker := 0; worker < 4; worker++ {
		t.Run(fmt.Sprintf("cache-%d", worker), func(t *testing.T) {
			t.Parallel()
			for entry, w := range memcachedFieldsTestMessages(t) {
				for i := 0; i < 5; i++ {
					n := protocolCorpusRequireBoundedRuleParse(t, w, memcachedFieldsTestRule, entry)
					tlsCertificateTestTree(t, n, w, 0)
				}
			}
		})
	}
}
