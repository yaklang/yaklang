package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

const etcdTestRequest = "GET /version HTTP/1.1\r\nHost: etcd.example:2379\r\nAccept: application/json\r\n\r\n"
const etcdTestBody = `{"etcdserver":"3.6.0","etcdcluster":"3.6.0","storage":"3.5.2"}`
const etcdTestResponse = "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 62\r\n\r\n" + etcdTestBody

func etcdTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.etcd", entry)
	require.Equal(t, wire, NodeToBytes(n))
	h225TestTree(t, n, wire, 0)
	return n
}

func etcdTestWhole(t *testing.T, record []byte, entry string) *base.Node {
	t.Helper()
	p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
	ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
	start := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
	size := len(tcp.Payload)
	tail := len(record) - start - size
	require.Equal(t, tcp.Payload, record[start:start+size])
	require.GreaterOrEqual(t, tail, 0)
	source := fmt.Sprintf("endian: little\nunit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Envelope: raw,%d\n    Record: \"import:application-layer/etcd.yaml;node:%s\"\n    Tail: raw,%d\n", size, tail, start, entry, tail)
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
	root, err := base.NewNodeTree(doc)
	require.NoError(t, err)
	root.Cfg.SetItem(base.CfgLength, uint64(len(record))*8)
	reader := base.NewBitReader(bytes.NewReader(record))
	require.NoError(t, root.ParseSubNode(reader, "Capture"))
	n := base.GetNodeByPath(root, "@Capture")
	require.Equal(t, record, NodeToBytes(n))
	h225TestTree(t, n, record, 0)
	require.ErrorContains(t, reader.Recovery(), "no backup")
	return protocolCorpusFindNode(n, "Record")
}

func TestProtocolCorpusEtcdAllOriginalRecords(t *testing.T) {
	for _, tc := range []struct{ dir, id, sha string }{
		{"generated-local", "gen-etcd", "47bab7dadd4b36abf12e0e07424208394475b6f4e1819ea1ecadcde9be4d3ecd"},
		{"generated-pr5023", "pr5023-gen-etcd", "dab35469fb572397ab9cef181eb2123282c6b21fd7406a92f1ec83d71d2b631e"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/" + tc.dir + "/" + tc.id + ".pcap"
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tc.sha, fmt.Sprintf("%x", sha256.Sum256(data)))
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, 4)
			for i, record := range records {
				packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
				require.Nil(t, packet.ErrorLayer())
				tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
				if i < 3 {
					require.Empty(t, tcp.Payload)
					continue
				}
				require.Equal(t, []byte("GET /v3/version HTTP/1.1\r\nHost: 127.0.0.1:2379\r\nUser-Agent: etcdctl\r\n\r\n"), tcp.Payload)
				require.Len(t, tcp.Payload, 71)
				http, err := parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), "application-layer.http", "HTTPExact")
				require.NoError(t, err)
				protocolCorpusRequireValue(t, http, "Path", "/v3/version")
				_, err = parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), "application-layer.etcd", "EtcdVersionHTTP")
				require.ErrorContains(t, err, "version endpoint path must be /version, not /v3/version")
				n := etcdTestWhole(t, record, "EtcdVersionCarrier")
				protocolCorpusRequireValue(t, n, "Unparsed Etcd Version Record", tcp.Payload)
				require.Nil(t, protocolCorpusFindNode(n, "Method"))
				require.Nil(t, protocolCorpusFindNode(n, "Versions"))
			}
		})
	}
}

func TestProtocolCorpusEtcdIndependentVersionFields(t *testing.T) {
	// Literal request and response independently follow etcd's registered route
	// and v3.6.0 version_test.go. This does not execute any request.
	require.Len(t, etcdTestRequest, 76)
	require.Len(t, etcdTestBody, 62)
	require.Len(t, etcdTestResponse, 133)
	n := etcdTestParse(t, []byte(etcdTestRequest), "EtcdVersionHTTP")
	protocolCorpusRequireValue(t, n, "Method", "GET")
	protocolCorpusRequireValue(t, n, "Path", "/version")
	require.Equal(t, "Version Request", n.Cfg.GetItem("additionInfo").(map[string]any)["Message"])
	n = etcdTestParse(t, []byte(etcdTestResponse), "EtcdVersionHTTP")
	protocolCorpusRequireValue(t, n, "Status", uint64(200))
	protocolCorpusRequireValue(t, n, "Content Length", uint64(62))
	protocolCorpusRequireValue(t, n, "etcdserver", "3.6.0")
	protocolCorpusRequireValue(t, n, "etcdcluster", "3.6.0")
	protocolCorpusRequireValue(t, n, "storage", "3.5.2")
	require.Equal(t, "No request-response correlation", n.Cfg.GetItem("additionInfo").(map[string]any)["Session"])
	for _, body := range []string{
		`{"etcdserver":"3.5.21","etcdcluster":"not_decided"}`,
		`{"storage":"unknown","etcdcluster":"not_decided","etcdserver":"3.6.0-rc.0"}`,
		` { "etcd\u0073erver" : "3.5.21" , "etcdcluster" : "not_decided" } `,
	} {
		wire := []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json; charset=UTF-8\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
		n := etcdTestParse(t, wire, "EtcdVersionHTTP")
		protocolCorpusRequireValue(t, n, "etcdcluster", "not_decided")
	}
}

func TestProtocolCorpusEtcdAllCompanionRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-etcd-version-valid.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "a093c7e086dea766449cb50e1769314d60ad1e5bf9a472037cd5762ed6f04d79", fmt.Sprintf("%x", sha256.Sum256(data)))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 5)
	for i, record := range records {
		packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if i < 3 {
			require.Empty(t, tcp.Payload)
			continue
		}
		if i == 3 {
			require.Equal(t, []byte(etcdTestRequest), tcp.Payload)
			require.Equal(t, layers.TCPPort(2379), tcp.DstPort)
		} else {
			require.Equal(t, []byte(etcdTestResponse), tcp.Payload)
			require.Equal(t, layers.TCPPort(2379), tcp.SrcPort)
			require.Equal(t, uint32(1001), tcp.Seq)
			require.Equal(t, uint32(78), tcp.Ack)
		}
		n := etcdTestWhole(t, record, "EtcdVersionHTTP")
		if i == 3 {
			protocolCorpusRequireValue(t, n, "Path", "/version")
		} else {
			protocolCorpusRequireValue(t, n, "etcdserver", "3.6.0")
			protocolCorpusRequireValue(t, n, "etcdcluster", "3.6.0")
			protocolCorpusRequireValue(t, n, "storage", "3.5.2")
		}
	}
}

func TestProtocolCorpusEtcdNegativeFramingAndLimits(t *testing.T) {
	valid := []byte(etcdTestResponse)
	for _, wire := range [][]byte{
		append(bytes.Clone(valid), 0), append(bytes.Clone(valid), valid...),
		bytes.Replace(valid, []byte("Content-Length: 62"), []byte("Content-Length: 61"), 1),
		bytes.Replace(valid, []byte("Content-Length: 62"), []byte("Content-Length: 63"), 1),
		bytes.Replace(valid, []byte("Content-Length: 62"), []byte("Content-Length: 999999999999999999999999"), 1),
		bytes.Replace(valid, []byte("Content-Length: 62\r\n"), nil, 1),
		bytes.Replace(valid, []byte("Content-Length: 62"), []byte("Content-Length: 62\r\nContent-Length: 62"), 1),
		bytes.Replace(valid, []byte("Content-Length: 62"), []byte("Transfer-Encoding: chunked\r\nContent-Length: 62"), 1),
		bytes.Replace(valid, []byte("application/json"), []byte("text/plain"), 1),
		bytes.Replace(valid, []byte("application/json"), []byte("application/json; charset=utf-16"), 1),
		bytes.Replace(valid, []byte("Content-Type:"), []byte("Content-Encoding: gzip\r\nContent-Type:"), 1),
		bytes.Replace(valid, []byte("200"), []byte("404"), 1),
		bytes.Replace(valid, []byte("etcdserver"), []byte("notaserver"), 1),
		[]byte(strings.Replace(etcdTestRequest, "GET", "HEAD", 1)),
		[]byte(strings.Replace(etcdTestRequest, "/version", "/version/", 1)),
		[]byte(strings.Replace(etcdTestRequest, "/version", "/version?x=y", 1)),
		[]byte(strings.Replace(etcdTestRequest, "Host:", "Host :", 1)),
		[]byte(strings.Replace(etcdTestRequest, "Host:", "Content-Length: 1\r\nHost:", 1)),
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.etcd", "EtcdVersionHTTP")
		require.Error(t, err)
		n := etcdTestParse(t, wire, "EtcdVersionCarrier")
		protocolCorpusRequireValue(t, n, "Unparsed Etcd Version Record", wire)
		require.Nil(t, protocolCorpusFindNode(n, "etcdserver"))
	}
	for _, complete := range [][]byte{[]byte(etcdTestRequest), valid} {
		for cut := 0; cut < len(complete); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(complete[:cut]), "application-layer.etcd", "EtcdVersionHTTP")
			require.Error(t, err, "prefix %d", cut)
		}
	}
	for _, entry := range []string{"EtcdVersionHTTP", "EtcdVersionCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(valid), "application-layer.etcd", entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, "application-layer.etcd", entry)
		require.Error(t, err)
	}
}

func TestProtocolCorpusEtcdOffsetsTransactionsAndIsolation(t *testing.T) {
	valid := []byte(etcdTestResponse)
	for offset := uint64(0); offset < 8; offset++ {
		for _, good := range []bool{true, false} {
			t.Run(fmt.Sprintf("%d/%t", offset, good), func(t *testing.T) {
				wire := bytes.Clone(valid)
				if !good {
					wire[len(wire)-1] = 0
				}
				var packed bytes.Buffer
				writer := base.NewBitWriter(&packed)
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, writer.WriteBits(wire, uint64(len(wire))*8))
				require.NoError(t, writer.WriteBits([]byte{0xd3}, 8))
				if offset > 0 {
					require.NoError(t, writer.WriteBits([]byte{0}, 8-offset))
				}
				source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Record: \"import:application-layer/etcd.yaml;node:EtcdVersionCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
				var doc yaml.MapSlice
				require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
				root, err := base.NewNodeTree(doc)
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("body_length", 123)
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				n := base.GetNodeByPath(root, "@Wrapped")
				record := protocolCorpusFindNode(n, "Record")
				h225TestTree(t, record, wire, offset)
				if good {
					protocolCorpusRequireValue(t, record, "storage", "3.5.2")
				} else {
					protocolCorpusRequireValue(t, record, "Unparsed Etcd Version Record", wire)
					require.Nil(t, protocolCorpusFindNode(record, "Versions"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, packed.Bytes(), NodeToBytes(n))
				require.Equal(t, 123, root.Ctx.GetItem("body_length"))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				_, err = reader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
	for _, cut := range []int{0, 1, len(valid) - 1} {
		root, err := base.ParseRule("application-layer/etcd.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(valid))*8)
		reader := base.NewBitReader(bytes.NewReader(valid[:cut]))
		require.Error(t, root.ParseSubNode(reader, "EtcdVersionHTTP"))
		require.Nil(t, protocolCorpusFindNode(base.GetNodeByPath(root, "@EtcdVersionHTTP"), "Status"))
		require.ErrorContains(t, reader.Recovery(), "no backup")
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint("isolated-", worker), func(t *testing.T) {
			t.Parallel()
			host := fmt.Sprint("unrelated-", worker, ".example")
			wire := []byte(strings.Replace(etcdTestRequest, "etcd.example:2379", host, 1))
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.etcd", "EtcdVersionHTTP", cfg)
			protocolCorpusRequireValue(t, n, "HTTP host", host)
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
