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

const kubernetesAPITestOriginal = "GET /api/v1/namespaces HTTP/1.1\r\nHost: kubernetes.lab\r\nAuthorization: Bearer lab\r\n\r\n"
const kubernetesAPITestCompanion = "GET /api/v1/namespaces?limit=2&timeoutSeconds=30&labelSelector=env%3Ddemo&fieldSelector=metadata.name%3Dsample&resourceVersion=0&allowWatchBookmarks=false HTTP/1.1\r\nHost: api.example\r\nAccept: application/json\r\n\r\n"

func kubernetesAPITestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.kubernetes_api", entry)
	require.Equal(t, wire, NodeToBytes(n))
	h225TestTree(t, n, wire, 0)
	return n
}

func kubernetesAPITestValue(t *testing.T, n *base.Node, name string, want any) {
	t.Helper()
	f := protocolCorpusFindNode(n, name)
	require.NotNil(t, f, name)
	v, err := f.Result()
	require.NoError(t, err)
	require.Equal(t, want, v.Value, name)
	require.Same(t, f, v.Origin)
}

func kubernetesAPITestWhole(t *testing.T, record []byte, entry string) *base.Node {
	t.Helper()
	p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
	ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
	start := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
	size := len(tcp.Payload)
	tail := len(record) - start - size
	require.Equal(t, tcp.Payload, record[start:start+size])
	require.GreaterOrEqual(t, tail, 0)
	source := fmt.Sprintf("endian: little\nunit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Request\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Request\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Envelope: raw,%d\n    Request: \"import:application-layer/kubernetes_api.yaml;node:%s\"\n    Tail: raw,%d\n", size, tail, start, entry, tail)
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
	return protocolCorpusFindNode(n, "Request")
}

func TestProtocolCorpusKubernetesAPIAllOriginalRecords(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-pr5023/pr5023-gen-k8s-api.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "ce28554eae15736aa207bf1472c6489a954fc51983805ff5f3bf82f4696773ad", fmt.Sprintf("%x", sha256.Sum256(data)))
	records := protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 4)
	for i, record := range records {
		packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if i < 3 {
			require.Empty(t, tcp.Payload)
			require.Len(t, record, 54)
			continue
		}
		require.Equal(t, []byte(kubernetesAPITestOriginal), tcp.Payload)
		require.Len(t, tcp.Payload, 84)
		n := kubernetesAPITestWhole(t, record, "KubernetesAPIRequest")
		protocolCorpusRequireValue(t, n, "Method", "GET")
		protocolCorpusRequireValue(t, n, "API Prefix", "api")
		protocolCorpusRequireValue(t, n, "API Version", "v1")
		protocolCorpusRequireValue(t, n, "Resource", "namespaces")
		protocolCorpusRequireValue(t, n, "HTTP host", "kubernetes.lab")
		protocolCorpusRequireValue(t, n, "Authentication Scheme", "Bearer")
		protocolCorpusRequireValue(t, n, "Credential", []byte("lab"))
		require.Equal(t, "Not verified", n.Cfg.GetItem("additionInfo").(map[string]any)["Authentication"])
	}
	path = "testdata/protocol-corpus/captures/generated-validated/gen-k8s-list-options-valid.pcap"
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "e5c57ba28f3f62dd8695f10b047d78646312a69fbf758202a535ee6384036aa1", fmt.Sprintf("%x", sha256.Sum256(data)))
	records = protocolCorpusAuditPackets(t, path)
	require.Len(t, records, 4)
	for i, record := range records {
		packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer())
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if i < 3 {
			require.Empty(t, tcp.Payload)
			continue
		}
		require.Equal(t, []byte(kubernetesAPITestCompanion), tcp.Payload)
		n := kubernetesAPITestWhole(t, record, "KubernetesAPIRequest")
		kubernetesAPITestValue(t, n, "limit", int64(2))
		kubernetesAPITestValue(t, n, "timeoutSeconds", int64(30))
		kubernetesAPITestValue(t, n, "allowWatchBookmarks", false)
		protocolCorpusRequireValue(t, n, "labelSelector", "env=demo")
		protocolCorpusRequireValue(t, n, "fieldSelector", "metadata.name=sample")
	}
}

func TestProtocolCorpusKubernetesAPITypedListOptions(t *testing.T) {
	// Independently serialized from the pinned public Namespace client and
	// ListOptions definition; no production serializer supplies these literals.
	n := kubernetesAPITestParse(t, []byte(kubernetesAPITestCompanion), "KubernetesAPIRequest")
	kubernetesAPITestValue(t, n, "limit", int64(2))
	kubernetesAPITestValue(t, n, "timeoutSeconds", int64(30))
	kubernetesAPITestValue(t, n, "allowWatchBookmarks", false)
	protocolCorpusRequireValue(t, n, "labelSelector", "env=demo")
	protocolCorpusRequireValue(t, n, "fieldSelector", "metadata.name=sample")
	protocolCorpusRequireValue(t, n, "resourceVersion", "0")
	require.Nil(t, protocolCorpusFindNode(n, "Credential"))
	info := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.Equal(t, "ListNamespace", info["Operation"])
	require.Equal(t, "cluster", info["Scope"])
	require.Equal(t, "", info["API Group"])
	for _, query := range []string{
		"?watch&allowWatchBookmarks=0&sendInitialEvents=TRUE&resourceVersionMatch=NotOlderThan&resourceVersion=opaque",
		"?watch=arbitrary&watch=false&limit=-1&limit=bad&unknown=text&&continue=opaque%2Ftoken&",
	} {
		wire := []byte("GET /api/v1/namespaces" + query + " HTTP/1.1\r\nHost: unrelated.example\r\nContent-Length: 0\r\n\r\n")
		n := kubernetesAPITestParse(t, wire, "KubernetesAPIRequest")
		kubernetesAPITestValue(t, n, "watch", true)
		protocolCorpusRequireValue(t, n, "Content Length", uint64(0))
		require.Equal(t, "WatchNamespace", n.Cfg.GetItem("additionInfo").(map[string]any)["Operation"])
	}
	// All duplicate occurrences remain distinct source spans. Only the first
	// typed field is named limit; the ignored invalid value remains plain text.
	wire := []byte("GET /api/v1/namespaces?limit=2&limit=bad HTTP/1.1\r\nHost: example\r\n\r\n")
	n = kubernetesAPITestParse(t, wire, "KubernetesAPIRequest")
	query := protocolCorpusFindNode(n, "Query")
	require.Len(t, query.Children, 4)
	kubernetesAPITestValue(t, n, "limit", int64(2))
	protocolCorpusRequireValue(t, n, "Parameter Value", "bad")
	require.Equal(t, false, query.Children[3].Cfg.GetItem("additionInfo").(map[string]any)["Effective First Value"])
}

func TestProtocolCorpusKubernetesAPINegativeAndBounds(t *testing.T) {
	valid := []byte(kubernetesAPITestOriginal)
	for _, wire := range [][]byte{
		append(bytes.Clone(valid), 0), append(bytes.Clone(valid), valid...),
		bytes.Replace(valid, []byte("GET "), []byte("POST "), 1),
		bytes.Replace(valid, []byte("/api/v1/namespaces"), []byte("/api/v1/pods"), 1),
		bytes.Replace(valid, []byte("/api/v1/namespaces"), []byte("/api/v1/namespaces/sample"), 1),
		bytes.Replace(valid, []byte("/api/v1/namespaces"), []byte("http://kubernetes.lab/api/v1/namespaces"), 1),
		bytes.Replace(valid, []byte("/api/v1/namespaces"), []byte("/api/v1/namespaces#x"), 1),
		bytes.Replace(valid, []byte("HTTP/1.1"), []byte("HTTP/2.0"), 1),
		bytes.Replace(valid, []byte("Host: kubernetes.lab\r\n"), nil, 1),
		bytes.Replace(valid, []byte("Host:"), []byte("Host :"), 1),
		bytes.Replace(valid, []byte("Host: kubernetes.lab"), []byte("Host: a\r\nHost: b"), 1),
		bytes.Replace(valid, []byte("Bearer lab"), []byte("Bearer lab\r\nAuthorization: Other x"), 1),
		bytes.Replace(valid, []byte("Host:"), []byte("Content-Length: 1\r\nHost:"), 1),
		bytes.Replace(valid, []byte("Host:"), []byte("Content-Length: 0\r\nContent-Length: 0\r\nHost:"), 1),
		bytes.Replace(valid, []byte("Host:"), []byte("Transfer-Encoding: chunked\r\nHost:"), 1),
		bytes.Replace(valid, []byte("Host:"), []byte("X-Value: x\x00\r\nHost:"), 1),
		bytes.Replace(valid, []byte("namespaces"), []byte("namespaces?limit=9223372036854775808"), 1),
		bytes.Replace(valid, []byte("namespaces"), []byte("namespaces?watch=%zz"), 1),
	} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.kubernetes_api", "KubernetesAPIRequest")
		require.Error(t, err)
		n := kubernetesAPITestParse(t, wire, "KubernetesAPICarrier")
		protocolCorpusRequireValue(t, n, "Unparsed Kubernetes API Request", wire)
		require.Nil(t, protocolCorpusFindNode(n, "Method"))
		require.Nil(t, protocolCorpusFindNode(n, "Credential"))
	}
	for cut := 0; cut < len(valid); cut++ {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(valid[:cut]), "application-layer.kubernetes_api", "KubernetesAPIRequest")
		require.Error(t, err, "prefix %d", cut)
	}
	for _, entry := range []string{"KubernetesAPIRequest", "KubernetesAPICarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader(valid), "application-layer.kubernetes_api", entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, "application-layer.kubernetes_api", entry)
		require.Error(t, err)
	}
}

func TestProtocolCorpusKubernetesAPIOffsetsTransactionsAndIsolation(t *testing.T) {
	valid := []byte(kubernetesAPITestOriginal)
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
				source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Request\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Request\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Request: \"import:application-layer/kubernetes_api.yaml;node:KubernetesAPICarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
				var doc yaml.MapSlice
				require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
				root, err := base.NewNodeTree(doc)
				require.NoError(t, err)
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("body_length", 123)
				root.Ctx.SetItem("httpHasLength", false)
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				n := base.GetNodeByPath(root, "@Wrapped")
				request := protocolCorpusFindNode(n, "Request")
				h225TestTree(t, request, wire, offset)
				if good {
					protocolCorpusRequireValue(t, request, "Resource", "namespaces")
				} else {
					protocolCorpusRequireValue(t, request, "Unparsed Kubernetes API Request", wire)
					require.Nil(t, protocolCorpusFindNode(request, "Resource"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, packed.Bytes(), NodeToBytes(n))
				require.Equal(t, 123, root.Ctx.GetItem("body_length"))
				require.Equal(t, false, root.Ctx.GetItem("httpHasLength"))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				_, err = reader.ReadBits(8)
				require.ErrorIs(t, err, io.EOF)
			})
		}
	}
	for _, cut := range []int{0, 1, len(valid) - 1} {
		root, err := base.ParseRule("application-layer/kubernetes_api.yaml")
		require.NoError(t, err)
		root.Cfg.SetItem(base.CfgLength, uint64(len(valid))*8)
		reader := base.NewBitReader(bytes.NewReader(valid[:cut]))
		require.Error(t, root.ParseSubNode(reader, "KubernetesAPIRequest"))
		require.Nil(t, protocolCorpusFindNode(base.GetNodeByPath(root, "@KubernetesAPIRequest"), "Method"))
		require.ErrorContains(t, reader.Recovery(), "no backup")
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint("isolated-", worker), func(t *testing.T) {
			t.Parallel()
			host := fmt.Sprint("arbitrary-", worker, ".example")
			wire := []byte(strings.Replace(kubernetesAPITestOriginal, "kubernetes.lab", host, 1))
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.kubernetes_api", "KubernetesAPIRequest", cfg)
			protocolCorpusRequireValue(t, n, "HTTP host", host)
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
