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

const winrmTestIdentifyBody = `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:i="http://schemas.dmtf.org/wbem/wsman/identify/1/wsmanidentity.xsd"><s:Header/><s:Body><i:Identify/></s:Body></s:Envelope>`
const winrmTestNamespaces = ` xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:w="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd"`

func winrmTestHTTP(body string, response bool) []byte {
	first := "POST /wsman HTTP/1.1\r\nHost: wsman.example\r\n"
	if response {
		first = "HTTP/1.1 200 OK\r\n"
	}
	return []byte(fmt.Sprintf("%sContent-Type: application/soap+xml;charset=UTF-8\r\nContent-Length: %d\r\n\r\n%s", first, len(body), body))
}
func winrmTestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	n := protocolCorpusRequireBoundedRuleParse(t, wire, "application-layer.winrm", entry)
	require.Equal(t, wire, NodeToBytes(n))
	h225TestTree(t, n, wire, 0)
	return n
}
func winrmTestWhole(t *testing.T, record []byte, entry string) *base.Node {
	t.Helper()
	p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
	ip := p.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
	start := 14 + int(ip.IHL)*4 + int(tcp.DataOffset)*4
	size := len(tcp.Payload)
	tail := len(record) - start - size
	require.Equal(t, tcp.Payload, record[start:start+size])
	require.GreaterOrEqual(t, tail, 0)
	source := fmt.Sprintf("endian: little\nunit: byte\nPackage:\n  Capture:\n    operator: |\n      this.ProcessSubNode(\"Envelope\")\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n      if %d > 0 { this.ProcessSubNode(\"Tail\") }\n    Envelope: raw,%d\n    Record: \"import:application-layer/winrm.yaml;node:%s\"\n    Tail: raw,%d\n", size, tail, start, entry, tail)
	var doc yaml.MapSlice
	require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
	root, e := base.NewNodeTree(doc)
	require.NoError(t, e)
	root.Cfg.SetItem(base.CfgLength, uint64(len(record))*8)
	reader := base.NewBitReader(bytes.NewReader(record))
	require.NoError(t, root.ParseSubNode(reader, "Capture"))
	n := base.GetNodeByPath(root, "@Capture")
	require.Equal(t, record, NodeToBytes(n))
	h225TestTree(t, n, record, 0)
	require.ErrorContains(t, reader.Recovery(), "no backup")
	return protocolCorpusFindNode(n, "Record")
}

func TestProtocolCorpusWinRMAllOriginalRecordsAndLayers(t *testing.T) {
	for _, tc := range []struct {
		id, dir, sha, needle string
		length               int
		httpValid            bool
	}{
		{"pr5023-gen-winrm-http", "generated-pr5023", "fc1aba9652986dce07a23771c59432a2ddab1560d3d0787e9378f888c5d49bae", "Content-Length does not match", 7, false},
		{"gen-winrm-http-valid", "generated-validated", "cc35f7e9774b2df709c0d8315682c7f597c12202b913e11d82aaa61a214f5127", "unbound namespace prefix", 8, true},
	} {
		t.Run(tc.id, func(t *testing.T) {
			path := "testdata/protocol-corpus/captures/" + tc.dir + "/" + tc.id + ".pcap"
			data, e := os.ReadFile(path)
			require.NoError(t, e)
			if tc.sha != "" {
				require.Equal(t, tc.sha, fmt.Sprintf("%x", sha256.Sum256(data)))
			}
			records := protocolCorpusAuditPackets(t, path)
			require.Len(t, records, 4)
			for i, record := range records {
				p := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
				tcp := p.Layer(layers.LayerTypeTCP).(*layers.TCP)
				if i < 3 {
					require.Empty(t, tcp.Payload)
					continue
				}
				literal := fmt.Sprintf("POST /wsman HTTP/1.1\r\nHost: winrm.lab\r\nContent-Type: application/soap+xml\r\nContent-Length: %d\r\n\r\n<s:Env/>", tc.length)
				require.Equal(t, []byte(literal), tcp.Payload)
				http, e := parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), "application-layer.http", "HTTPExact")
				if tc.httpValid {
					require.NoError(t, e)
					protocolCorpusRequireValue(t, http, "Octets", "<s:Env/>")
				} else {
					require.Error(t, e)
				}
				_, e = parser.ParseBinary(newProtocolCorpusBoundedReader(tcp.Payload), "application-layer.winrm", "WinRMHTTP")
				require.ErrorContains(t, e, tc.needle)
				n := winrmTestWhole(t, record, "WinRMHTTPCarrier")
				protocolCorpusRequireValue(t, n, "Unparsed WinRM Record", tcp.Payload)
				require.Nil(t, protocolCorpusFindNode(n, "Method"))
				require.Nil(t, protocolCorpusFindNode(n, "SOAP"))
			}
		})
	}
	companionPath := "testdata/protocol-corpus/captures/generated-validated/gen-winrm-identify-valid.pcap"
	data, err := os.ReadFile(companionPath)
	require.NoError(t, err)
	require.Equal(t, "778ae8689f877ad75837cff8f1be541eff98db755e6a8dbb8ca0ef0c2425156b", fmt.Sprintf("%x", sha256.Sum256(data)))
	records := protocolCorpusAuditPackets(t, companionPath)
	require.Len(t, records, 4)
	for i, record := range records {
		packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
		tcp := packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
		if i < 3 {
			require.Empty(t, tcp.Payload)
			continue
		}
		require.Equal(t, winrmTestHTTP(winrmTestIdentifyBody, false), tcp.Payload)
		require.Len(t, tcp.Payload, 306)
		n := winrmTestWhole(t, record, "WinRMHTTP")
		protocolCorpusRequireValue(t, n, "Method", "POST")
		protocolCorpusRequireValue(t, n, "Content Length", uint64(190))
		require.NotNil(t, protocolCorpusFindNode(n, "Identify"))
	}
}

func TestProtocolCorpusWinRMIndependentIdentifyAndGet(t *testing.T) {
	// Fixed Identify is independently serialized from MS-WSMV 2.2.1 and
	// DSP0226 1.1.1 section 11, not from the production decoder.
	wire := winrmTestHTTP(winrmTestIdentifyBody, false)
	require.Len(t, wire, 306)
	n := winrmTestParse(t, wire, "WinRMHTTP")
	protocolCorpusRequireValue(t, n, "Method", "POST")
	protocolCorpusRequireValue(t, n, "Path", "/wsman")
	protocolCorpusRequireValue(t, n, "Content Length", uint64(190))
	require.NotNil(t, protocolCorpusFindNode(n, "Identify"))
	i := protocolCorpusFindNode(n, "Identify")
	require.Equal(t, "http://schemas.dmtf.org/wbem/wsman/identify/1/wsmanidentity.xsd", i.Cfg.GetItem("additionInfo").(map[string]any)["Namespace"])
	for _, body := range []string{
		strings.ReplaceAll(winrmTestIdentifyBody, "/identify/1/", "/identity/1/"),
		strings.NewReplacer("<s:", "<q:", "</s:", "</q:", "xmlns:s=", "xmlns:q=").Replace(winrmTestIdentifyBody),
		strings.ReplaceAll(winrmTestIdentifyBody, "<s:Header/>", ""),
		strings.ReplaceAll(winrmTestIdentifyBody, "<i:Identify/>", `<i:Identify><v:Hint xmlns:v="urn:example:extension">literal &amp; text</v:Hint></i:Identify>`),
	} {
		winrmTestParse(t, winrmTestHTTP(body, false), "WinRMHTTP")
	}
	response := strings.Replace(winrmTestIdentifyBody, "<i:Identify/>", `<i:IdentifyResponse><i:ProtocolVersion>http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd</i:ProtocolVersion><i:ProductVendor>Example &amp; Co</i:ProductVendor><i:ProductVersion>1.2</i:ProductVersion></i:IdentifyResponse>`, 1)
	n = winrmTestParse(t, winrmTestHTTP(response, true), "WinRMHTTP")
	protocolCorpusRequireValue(t, n, "Status", uint64(200))
	protocolCorpusRequireValue(t, n, "ProtocolVersion", "http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd")
	protocolCorpusRequireValue(t, n, "ProductVendor", "Example & Co")
	// Fields and layout from MS-WSMV 4.1.1, with independently chosen literals.
	header := `<s:Header><a:To>http://server.example/wsman</a:To><w:ResourceURI s:mustUnderstand="true">urn:example:resource</w:ResourceURI><a:ReplyTo><a:Address>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo><a:Action s:mustUnderstand="1">http://schemas.xmlsoap.org/ws/2004/09/transfer/Get</a:Action><w:MaxEnvelopeSize>51200</w:MaxEnvelopeSize><a:MessageID>uuid:00000000-0000-0000-0000-000000000001</a:MessageID><w:SelectorSet><w:Selector Name="id">1</w:Selector></w:SelectorSet><w:OperationTimeout>PT60.000S</w:OperationTimeout></s:Header>`
	get := `<s:Envelope` + winrmTestNamespaces + `>` + header + `<s:Body/></s:Envelope>`
	n = winrmTestParse(t, winrmTestHTTP(get, false), "WinRMHTTP")
	protocolCorpusRequireValue(t, n, "Action", "http://schemas.xmlsoap.org/ws/2004/09/transfer/Get")
	protocolCorpusRequireValue(t, n, "ResourceURI", "urn:example:resource")
	protocolCorpusRequireValue(t, n, "Selector id", "1")
	protocolCorpusRequireValue(t, n, "MaxEnvelopeSize", uint64(51200))
	protocolCorpusRequireValue(t, n, "OperationTimeout", "PT60.000S")
	reply := `<s:Envelope` + winrmTestNamespaces + `><s:Header><a:Action>http://schemas.xmlsoap.org/ws/2004/09/transfer/GetResponse</a:Action><a:MessageID>urn:uuid:2</a:MessageID><a:To>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:To><a:RelatesTo>urn:uuid:1</a:RelatesTo></s:Header><s:Body><p:myclass xmlns:p="urn:example:resource"><p:Data1>Hello World</p:Data1><p:id>1</p:id></p:myclass></s:Body></s:Envelope>`
	n = winrmTestParse(t, winrmTestHTTP(reply, true), "WinRMHTTP")
	protocolCorpusRequireValue(t, n, "Resource Data1", "Hello World")
	protocolCorpusRequireValue(t, n, "Resource id", "1")
	for _, bad := range []string{
		strings.Replace(get, "PT60.000S", "PTH", 1), strings.Replace(get, "51200", "4294967296", 1),
		strings.Replace(get, `Name="id"`, `bad="id"`, 1), strings.Replace(get, "<s:Body/>", "<s:Body><w:wrong/></s:Body>", 1),
		strings.Replace(get, "s:mustUnderstand=\"true\"", "s:mustUnderstand=\"yes\"", 1),
		strings.Replace(get, "<a:To>", "<a:To/><a:To>", 1), strings.Replace(get, "urn:example:resource", "relative", 1),
	} {
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(winrmTestHTTP(bad, false)), "application-layer.winrm", "WinRMHTTP")
		require.Error(t, e)
	}
}

func TestProtocolCorpusWinRMNegativeXMLFramingAndLimits(t *testing.T) {
	valid := winrmTestHTTP(winrmTestIdentifyBody, false)
	for _, body := range []string{
		"<s:Env/>", `<s:Env xmlns:s="http://www.w3.org/2003/05/soap-envelope"/>`,
		strings.Replace(winrmTestIdentifyBody, "<s:Body>", "<s:Body/><s:Body>", 1),
		strings.Replace(winrmTestIdentifyBody, "<s:Body><i:Identify/></s:Body>", "", 1),
		strings.Replace(winrmTestIdentifyBody, "<s:Header/>", "<s:Header><x/></s:Header>", 1),
		strings.Replace(winrmTestIdentifyBody, "<i:Identify/>", "<i:Identify>text</i:Identify>", 1),
		strings.Replace(winrmTestIdentifyBody, "<i:Identify/>", "<i:Identify><i:Unknown/></i:Identify>", 1),
		strings.Replace(winrmTestIdentifyBody, "http://schemas.dmtf.org/", "https://schemas.dmtf.org/", 1),
		strings.Replace(winrmTestIdentifyBody, "<i:Identify/>", "<i:IdentifyResponse/>", 1),
		strings.Replace(winrmTestIdentifyBody, "<s:Header/>", "<?x literal?>", 1),
		"<!DOCTYPE s:Envelope [<!ENTITY x 'value'>]>" + winrmTestIdentifyBody,
		"<!--outside-->" + winrmTestIdentifyBody,
		strings.Replace(winrmTestIdentifyBody, "<i:Identify/>", `<i:Identify xmlns:v="urn:example">`+strings.Repeat("<v:x>", 50)+strings.Repeat("</v:x>", 50)+`</i:Identify>`, 1),
		strings.Replace(winrmTestIdentifyBody, "<i:Identify/>", `<i:Identify><v:x xmlns:v="urn:example">`+string([]byte{0xff})+`</v:x></i:Identify>`, 1),
	} {
		wire := winrmTestHTTP(body, false)
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.winrm", "WinRMHTTP")
		require.Error(t, e)
		n := winrmTestParse(t, wire, "WinRMHTTPCarrier")
		protocolCorpusRequireValue(t, n, "Unparsed WinRM Record", wire)
	}
	for _, wire := range [][]byte{
		append(bytes.Clone(valid), 0), bytes.Replace(valid, []byte("Content-Length: 190"), []byte("Content-Length: 189"), 1),
		bytes.Replace(valid, []byte("Content-Length: 190"), []byte("Content-Length: 99999999999999999999999"), 1),
		bytes.Replace(valid, []byte("Content-Length: 190"), []byte("Content-Length: 190\r\nContent-Length: 190"), 1),
		bytes.Replace(valid, []byte("Content-Length: 190"), []byte("Transfer-Encoding: chunked\r\nContent-Length: 190"), 1),
		bytes.Replace(valid, []byte("application/soap+xml;charset=UTF-8"), []byte("application/soap+xml;charset=UTF-16"), 1),
		bytes.Replace(valid, []byte("application/soap+xml;charset=UTF-8"), []byte("application/soap+xml;action=\"urn:wrong\""), 1),
		bytes.Replace(valid, []byte("POST"), []byte("GET"), 1), bytes.Replace(valid, []byte("Host:"), []byte("Host :"), 1),
		bytes.Replace(valid, []byte("Content-Type:"), []byte("Content-Encoding: gzip\r\nContent-Type:"), 1),
		bytes.Replace(valid, []byte("Host: wsman.example"), []byte("Host: wsman.example\r\n"+strings.Repeat("X-A: a\r\n", 257)), 1),
	} {
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), "application-layer.winrm", "WinRMHTTP")
		require.Error(t, e)
	}
	for cut := 0; cut < len(valid); cut++ {
		_, e := parser.ParseBinary(newProtocolCorpusBoundedReader(valid[:cut]), "application-layer.winrm", "WinRMHTTP")
		require.Error(t, e, "prefix %d", cut)
	}
	for _, entry := range []string{"WinRMHTTP", "WinRMHTTPCarrier"} {
		_, e := parser.ParseBinary(bytes.NewReader(valid), "application-layer.winrm", entry)
		require.ErrorContains(t, e, "explicit")
		_, e = parser.GenerateBinary(map[string]any{}, "application-layer.winrm", entry)
		require.Error(t, e)
	}
}

func TestProtocolCorpusWinRMOffsetsTransactionsAndIsolation(t *testing.T) {
	valid := winrmTestHTTP(winrmTestIdentifyBody, false)
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
				source := fmt.Sprintf("endian: little\nPackage:\n  Wrapped:\n    operator: |\n      if %d > 0 { this.ProcessSubNode(\"Prefix\") }\n      this.GetSubNode(\"Record\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Record\")\n      this.ProcessSubNode(\"Sentinel\")\n      if %d > 0 { this.ProcessSubNode(\"Padding\") }\n    Prefix: uint8,%dbit\n    Record: \"import:application-layer/winrm.yaml;node:WinRMHTTPCarrier\"\n    Sentinel: uint8\n    Padding: uint8,%dbit\n", offset, len(wire), offset, offset, 8-offset)
				var doc yaml.MapSlice
				require.NoError(t, yaml.Unmarshal([]byte(source), &doc))
				root, e := base.NewNodeTree(doc)
				require.NoError(t, e)
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				root.Ctx.SetItem("body_length", 123)
				root.Ctx.SetItem("httpHasLength", false)
				reader := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.NoError(t, root.ParseSubNode(reader, "Wrapped"))
				n := base.GetNodeByPath(root, "@Wrapped")
				record := protocolCorpusFindNode(n, "Record")
				h225TestTree(t, record, wire, offset)
				if good {
					require.NotNil(t, protocolCorpusFindNode(record, "Identify"))
					protocolCorpusRequireValue(t, record, "Content Length", uint64(190))
				} else {
					protocolCorpusRequireValue(t, record, "Unparsed WinRM Record", wire)
					require.Nil(t, protocolCorpusFindNode(record, "SOAP"))
				}
				protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
				require.Equal(t, packed.Bytes(), NodeToBytes(n))
				require.Equal(t, 123, root.Ctx.GetItem("body_length"))
				require.Equal(t, false, root.Ctx.GetItem("httpHasLength"))
				require.ErrorContains(t, reader.Recovery(), "no backup")
				_, e = reader.ReadBits(8)
				require.ErrorIs(t, e, io.EOF)
			})
		}
	}
	for _, cut := range []int{0, 1, 100, len(valid) - 1} {
		root, e := base.ParseRule("application-layer/winrm.yaml")
		require.NoError(t, e)
		root.Cfg.SetItem(base.CfgLength, uint64(len(valid))*8)
		reader := base.NewBitReader(bytes.NewReader(valid[:cut]))
		require.Error(t, root.ParseSubNode(reader, "WinRMHTTP"))
		require.Nil(t, protocolCorpusFindNode(base.GetNodeByPath(root, "@WinRMHTTP"), "Method"))
		require.ErrorContains(t, reader.Recovery(), "no backup")
	}
	for worker := 0; worker < 4; worker++ {
		worker := worker
		t.Run(fmt.Sprint("isolated-", worker), func(t *testing.T) {
			t.Parallel()
			wire := bytes.Replace(valid, []byte("wsman.example"), []byte(fmt.Sprint("wsma", worker, ".example")), 1)
			cfg := map[string]any{"marker": worker}
			n := protocolCorpusRequireBoundedRuleParseWithConfig(t, wire, "application-layer.winrm", "WinRMHTTP", cfg)
			protocolCorpusRequireValue(t, n, "HTTP host", fmt.Sprint("wsma", worker, ".example"))
			require.Equal(t, map[string]any{"marker": worker}, cfg)
		})
	}
}
