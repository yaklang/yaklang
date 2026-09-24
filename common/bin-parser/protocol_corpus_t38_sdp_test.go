package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

func TestProtocolCorpusT38SDPImportedHeldReader(t *testing.T) {
	for _, entry := range []string{"T38SDPAdvertisement", "T38H248SDPAdvertisement", "T38SDPCarrier", "T38H248SDPCarrier"} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				if !valid && strings.HasSuffix(entry, "Advertisement") {
					continue // direct failures and exact staging rollback are tested natively
				}
				wire := []byte(t38SDPTestFixtures[0].wire)
				if !valid {
					wire = append(wire, 'x')
				}
				t.Run(fmt.Sprintf("%s/%d/%t", entry, offset, valid), func(t *testing.T) {
					var packed bytes.Buffer
					w := base.NewBitWriter(&packed)
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0x55}, offset))
					}
					require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
					require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
					}
					root := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(%d)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/t38_sdp.yaml;node:%s"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, entry, 8-offset))
					root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
					root.Ctx.SetItem("t38-caller", "preserved")
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					require.NoError(t, r.Backup())
					require.NoError(t, root.ParseSubNode(r, "Envelope"))
					n := base.GetNodeByPath(root, "@Envelope")
					if valid {
						t38SDPTestTree(t, n, wire, offset)
					} else {
						latTestField(t, n, "Unparsed T38 SDP Advertisement", "raw", offset, offset+uint64(len(wire))*8, wire)
						require.Nil(t, protocolCorpusFindNode(n, "Field Type"))
					}
					protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
					require.Equal(t, "preserved", root.Ctx.GetItem("t38-caller"))
					require.Equal(t, packed.Bytes(), NodeToBytes(n))
					require.NoError(t, r.Recovery())
					got, err := r.ReadBits(uint64(packed.Len()) * 8)
					require.NoError(t, err)
					require.Equal(t, packed.Bytes(), got)
					require.ErrorContains(t, r.PopBackup(), "no backup")
				})
			}
		}
	}
}

const t38SDPTestRule = "application-layer.t38_sdp"
const t38SDPTestFull = "v=0\r\no=- 1 1 IN IP4 192.0.2.1\r\ns=example\r\nc=IN IP4 192.0.2.1\r\nt=0 0\r\nm=image 4000 udptl t38\r\n"

// Independent literal advertisements; these are not transmitted. The first
// full session also states the required rate-management parameter explicitly.
var t38SDPTestFixtures = []struct {
	name, wire                  string
	h248                        bool
	media, capability, sessions int
}{
	{"full-udptl", t38SDPTestFull + "a=T38FaxRateManagement:transferredTCF\r\n", false, 1, 0, 1},
	{"full-capability", "v=0\r\no=- 2 2 IN IP4 192.0.2.2\r\ns=example\r\nc=IN IP4 192.0.2.2\r\nt=0 0\r\nm=audio 4002 RTP/AVP 8\r\na=sqn: 7\r\na=cdsc: 1 audio RTP/AVP 8\r\na=cdsc: 9 image udptl t38\r\n", false, 0, 1, 1},
	{"h248-choose-alternatives", "v=0\r\nc=IN IP4 $\r\nm=audio $ RTP/AVP 8\r\nv=0\r\nc=IN IP4 $\r\nm=image $ udptl t38\r\n", true, 1, 0, 2},
	{"h248-reply-port-zero", " \r\nv=0\r\no=- 0 0 IN IP4 -\r\ns=-\r\nc=IN IP4 192.0.2.3\r\nt=0 0\r\nm=image 0 UDPTL t38\r\n ", true, 1, 0, 1},
	{"full-tcp-attributes", "v=0\r\no=- 3 3 IN IP6 2001:db8::1\r\ns=example\r\nc=IN IP6 2001:db8::1\r\nt=0 0\r\nm=image 4004 tcp t38\r\na=T38FaxVersion:4\r\na=T38MaxBitRate:14400\r\na=T38FaxFillBitRemoval\r\na=T38FaxTranscodingMMR\r\na=T38FaxTranscodingJBIG\r\na=T38FaxRateManagement:localTCF\r\na=T38FaxMaxBuffer:1800\r\na=T38VendorInfo:0 0 37\r\n", false, 1, 0, 1},
	{"full-udptl-attributes", t38SDPTestFull + "a=T38FaxRateManagement:transferredTCF\r\na=T38FaxMaxDatagram:400\r\na=T38FaxMaxIFP:200\r\na=T38FaxUdpEC:t38UDPRedundancy\r\na=T38FaxUdpECDepth:1 3\r\na=T38FaxUdpFECMaxSpan:4\r\na=x-extension:preserved\r\n", false, 1, 0, 1},
}

func t38SDPTestEntry(h248, carrier bool) string {
	if h248 {
		if carrier {
			return "T38H248SDPCarrier"
		}
		return "T38H248SDPAdvertisement"
	}
	if carrier {
		return "T38SDPCarrier"
	}
	return "T38SDPAdvertisement"
}

func t38SDPTestInfo(t *testing.T, n *base.Node) map[string]any {
	t.Helper()
	return gssapiTestInfo(t, n, "T38 Media Advertisement Count")
}

func t38SDPTestTree(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	version := protocolCorpusFindNode(n, "Field Type")
	require.NotNil(t, version)
	message := version.Cfg.GetItem(base.CfgParent).(*base.Node).Cfg.GetItem(base.CfgParent).(*base.Node).Cfg.GetItem(base.CfgParent).(*base.Node)
	megacoTestTree(t, message, offset, offset+uint64(len(wire))*8)
	info := t38SDPTestInfo(t, n)
	for _, key := range []string{"Media Payload Decoded", "Negotiation Outcome Validated", "Endpoint Address Validated", "Capability Set Completeness Validated", "T38 Required Parameter Set Validated", "H248 Direction Validated"} {
		require.Equal(t, false, info[key], key)
	}
}

func TestProtocolCorpusT38SDPFields(t *testing.T) {
	for _, f := range t38SDPTestFixtures {
		for _, carrier := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", f.name, carrier), func(t *testing.T) {
				wire := []byte(f.wire)
				n := protocolCorpusRequireBoundedRuleParse(t, wire, t38SDPTestRule, t38SDPTestEntry(f.h248, carrier))
				require.Equal(t, wire, NodeToBytes(n))
				t38SDPTestTree(t, n, wire, 0)
				info := t38SDPTestInfo(t, n)
				require.Equal(t, f.media, info["T38 Media Advertisement Count"])
				require.Equal(t, f.capability, info["T38 Capability Advertisement Count"])
				require.Equal(t, f.sessions, info["SDP Session Count"])
				if f.media > 0 && f.sessions == 1 {
					protocolCorpusRequireValue(t, n, "Media Format 0", "t38")
				} else if f.media == 0 {
					protocolCorpusRequireValue(t, n, "Capability Format 0", "8")
				} else {
					protocolCorpusRequireValue(t, n, "Media Format 0", "8")
				}
			})
		}
	}
	wire := []byte(t38SDPTestFixtures[0].wire)
	n := protocolCorpusRequireBoundedRuleParse(t, wire, t38SDPTestRule, "T38SDPAdvertisement")
	for _, f := range []struct{ name, value, anchor string }{{"SDP Version", "0", "v="}, {"Origin Username", "-", "o="}, {"Session ID", "1", "o=- "}, {"Session Name", "example", "s="}, {"Connection Address", "192.0.2.1", "c="}, {"Start Time", "0", "t="}, {"Media Type", "image", "m="}, {"Media Port", "4000", "m="}, {"Media Transport", "udptl", "m="}, {"Media Format 0", "t38", "m="}, {"Attribute Name", "T38FaxRateManagement", "a="}, {"Attribute Value", "transferredTCF", "a="}} {
		start := bytes.Index(wire, []byte(f.anchor))
		require.GreaterOrEqual(t, start, 0)
		at := start + bytes.Index(wire[start:], []byte(f.value))
		latTestField(t, n, f.name, "string", uint64(at)*8, uint64(at+len(f.value))*8, f.value)
	}
	// The normative receiver compatibility rule does not interpret ':0' as false.
	compat := t38SDPTestFull + "a=T38FaxTranscodingJBIG:0\r\n"
	n = protocolCorpusRequireBoundedRuleParse(t, []byte(compat), t38SDPTestRule, "T38SDPAdvertisement")
	info := gssapiTestInfo(t, n, "Boolean Present")
	require.Equal(t, true, info["Boolean Present"])
	require.Equal(t, true, info["Noncanonical Boolean Value Present"])
	for _, wire := range []string{"c=IN IP4 $\r\nm=image $ udptl t38\r\n", strings.Replace(t38SDPTestFull, "udptl", "UDPTL", 1), strings.Replace(t38SDPTestFull, "4000", "4000/2", 1)} {
		h248 := strings.HasPrefix(wire, "c=")
		n := protocolCorpusRequireBoundedRuleParse(t, []byte(wire), t38SDPTestRule, t38SDPTestEntry(h248, false))
		require.Equal(t, []byte(wire), NodeToBytes(n))
	}
}

func t38SDPTestReject(t *testing.T, wire []byte, h248 bool, diagnostic string) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), t38SDPTestRule, t38SDPTestEntry(h248, false))
	require.ErrorContains(t, err, diagnostic)
	if len(wire) == 0 {
		return
	}
	n := protocolCorpusRequireBoundedRuleParse(t, wire, t38SDPTestRule, t38SDPTestEntry(h248, true))
	require.Equal(t, wire, NodeToBytes(n))
	latTestField(t, n, "Unparsed T38 SDP Advertisement", "raw", 0, uint64(len(wire))*8, wire)
	require.Nil(t, protocolCorpusFindNode(n, "Field Type"))
}

func TestProtocolCorpusT38SDPPrefixesAndNegatives(t *testing.T) {
	// SDP has no terminal marker: cutting optional complete lines can produce a
	// valid shorter advertisement. Every other short prefix must fail, and any
	// successful line-boundary prefix must still round-trip completely.
	for _, f := range t38SDPTestFixtures {
		for cut := 0; cut < len(f.wire); cut++ {
			wire := []byte(f.wire[:cut])
			n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), t38SDPTestRule, t38SDPTestEntry(f.h248, false))
			if err == nil {
				require.True(t, strings.HasSuffix(strings.TrimRight(string(wire), " \t"), "\r\n"), "%s cut %d", f.name, cut)
				require.Equal(t, wire, NodeToBytes(n))
				t38SDPTestTree(t, n, wire, 0)
			}
		}
	}
	for _, pair := range [][2]string{{"v=0", "v=1"}, {"v=0\r\n", ""}, {"o=- 1 1 IN IP4 192.0.2.1\r\n", ""}, {"s=example\r\n", ""}, {"t=0 0\r\n", ""}, {"c=IN IP4 192.0.2.1\r\n", ""}, {"m=image 4000 udptl t38", "m=image 4000 udptl"}, {"4000", "65536"}, {"4000", "-1"}, {"4000", "4000/0"}, {"4000", "$"}, {"udptl", "UDP"}, {"t38", "other"}, {"o=- 1 1", "o=- x 1"}, {"t=0 0", "t=123 0"}, {"s=example", "s="}, {"m=image", "m= image"}, {"m=image", "q=image"}, {"\r\n", "\n"}} {
		t38SDPTestReject(t, []byte(strings.Replace(t38SDPTestFull, pair[0], pair[1], 1)), false, "t38-sdp:")
	}
	for _, tail := range []string{"x", "\x00", "\r\n", "v=0\r\n", "o=- 1 1 IN IP4 x\r\n", "a=sqn:256\r\n", "a=sqn:0\r\n", "a=cdsc:1 image udptl t38\r\n", "a=sqn:0\r\na=x-other\r\na=cdsc:1 image udptl t38\r\n", "a=sqn:0\r\na=cdsc:0 image udptl t38\r\n", "a=T38FaxVersion:-1\r\n", "a=T38FaxRateManagement:other\r\n", "a=T38FaxUdpEC:other\r\n", "a=T38VendorInfo:256 0 37\r\n", "a=T38FaxUdpECDepth:1 2 3\r\n", "a=T38FaxVersion:0\r\na=T38FaxVersion:0\r\n"} {
		t38SDPTestReject(t, []byte(t38SDPTestFull+tail), false, "t38-sdp:")
	}
	t38SDPTestReject(t, []byte(t38SDPTestFull+"a=x:\r\n"), false, "empty attribute value")
	t38SDPTestReject(t, []byte(t38SDPTestFull+"m=image 4002 udptl t38\r\n"), true, "one media")
	t38SDPTestReject(t, []byte("c=IN IP4 $\r\nm=image $ udptl t38\r\nv=0\r\nc=IN IP4 $\r\nm=image $ udptl t38\r\n"), true, "v delimiters")
	t38SDPTestReject(t, []byte("v=0\r\nc=IN IP4 $\r\nm=image $ udptl t38\r\nv=0\r\n"), true, "no media")
}

// Complete original-record inspection, without rerunning the entire carrier
// protocol suite: all 1552 records have checked IPv4/UDP boundaries; only the
// 14 Megaco records containing SDP and 28 exact SIP bodies need SDP parsing.
func TestProtocolCorpusT38SDPOriginalRecords(t *testing.T) {
	const path = "testdata/protocol-corpus/captures/ndpi/ndpi-t38.pcap"
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "b402d3ee4ccb32fba3c75a90fdb8ae50ab9a58f9f5e51a72d9d873b18a68d99e", fmt.Sprintf("%x", sha256.Sum256(data)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, 1552)
	megacoCount, sipCount, rtpCount, sdpMegaco, sdpSIP, ads, nonAds, empty := 0, 0, 0, 0, 0, 0, 0, 0
	verify := func(frameNo int, wire []byte, h248 bool, originalFrame []byte, offset int) {
		n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), t38SDPTestRule, t38SDPTestEntry(h248, false))
		if len(wire) == 0 {
			require.True(t, h248)
			require.ErrorContains(t, err, "boundary")
			empty++
			return
		}
		if err != nil {
			require.ErrorContains(t, err, "no image/t38 advertisement", fmt.Sprintf("frame %d: %q", frameNo, wire))
			nonAds++
			return
		}
		ads++
		require.Equal(t, wire, NodeToBytes(n))
		t38SDPTestTree(t, n, wire, 0)
		require.Equal(t, wire, originalFrame[offset:offset+len(wire)])
		// Reparse in an explicit physical carrier boundary. All derived leaves
		// now refer to the original PCAP frame's actual bit positions.
		suffixLength := len(originalFrame) - offset - len(wire)
		root := latTestInline(t, fmt.Sprintf("Package:\n  Envelope:\n    operator: |\n      this.ProcessSubNode(\"Prefix\")\n      this.GetSubNode(\"Advertisement\").SetMaxLength(%d)\n      this.ProcessSubNode(\"Advertisement\")\n      if %d > 0 { this.ProcessSubNode(\"Suffix\") }\n    Prefix: raw,%d\n    Advertisement: \"import:application-layer/t38_sdp.yaml;node:%s\"\n    Suffix: raw,%d\n", len(wire), suffixLength, offset, t38SDPTestEntry(h248, false), suffixLength))
		root.Cfg.SetItem(base.CfgLength, uint64(len(originalFrame))*8)
		require.NoError(t, root.ParseSubNode(base.NewBitReader(bytes.NewReader(originalFrame)), "Envelope"))
		container := base.GetNodeByPath(root, "@Envelope")
		require.Equal(t, originalFrame, NodeToBytes(container))
		t38SDPTestTree(t, container, wire, uint64(offset)*8)
	}
	for i, frame := range frames {
		require.GreaterOrEqual(t, len(frame), 42)
		require.Equal(t, uint16(0x800), binary.BigEndian.Uint16(frame[12:14]))
		require.Equal(t, byte(17), frame[23])
		require.Zero(t, binary.BigEndian.Uint16(frame[20:22])&0x3fff)
		ihl := int(frame[14]&15) * 4
		require.GreaterOrEqual(t, ihl, 20)
		udp := 14 + ihl
		require.GreaterOrEqual(t, len(frame), udp+8)
		length := int(binary.BigEndian.Uint16(frame[udp+4 : udp+6]))
		require.GreaterOrEqual(t, length, 8)
		require.Equal(t, ihl+length, int(binary.BigEndian.Uint16(frame[16:18])))
		require.LessOrEqual(t, udp+length, len(frame))
		wire := frame[udp+8 : udp+length]
		src, dst := binary.BigEndian.Uint16(frame[udp:udp+2]), binary.BigEndian.Uint16(frame[udp+2:udp+4])
		if src == 2944 || dst == 2944 {
			megacoCount++
			if !bytes.Contains(wire, []byte("v=0\r\n")) {
				continue
			}
			sdpMegaco++
			n := megacoTestParse(t, wire, "Megaco")
			require.Equal(t, wire, NodeToBytes(n))
			var walk func(*base.Node)
			walk = func(n *base.Node) {
				if n.Name == "SDP Text" {
					span := n.Cfg.GetItem(base.CfgNodeResult).([2]uint64)
					require.Zero(t, span[0]%8)
					require.Zero(t, span[1]%8)
					raw := wire[span[0]/8 : span[1]/8]
					protocolCorpusRequireValue(t, n, "SDP Text", raw)
					if i == 20 {
						t.Logf("representative frame21: UDP offset=%d fullframe offset=%d length=%d", span[0]/8, udp+8+int(span[0]/8), len(raw))
					}
					verify(i+1, raw, true, frame, udp+8+int(span[0]/8))
				}
				for _, c := range n.Children {
					walk(c)
				}
			}
			walk(n)
		} else if src == 5060 || dst == 5060 {
			sipCount++
			split := bytes.Index(wire, []byte("\r\n\r\n"))
			require.GreaterOrEqual(t, split, 0)
			contentLength := -1
			contentType := ""
			for _, line := range strings.Split(string(wire[:split]), "\r\n")[1:] {
				parts := strings.SplitN(line, ":", 2)
				require.Len(t, parts, 2)
				switch strings.ToLower(parts[0]) {
				case "content-length":
					require.Equal(t, -1, contentLength)
					contentLength, err = strconv.Atoi(strings.TrimSpace(parts[1]))
					require.NoError(t, err)
				case "content-type":
					require.Empty(t, contentType)
					contentType = strings.TrimSpace(parts[1])
				}
			}
			if contentLength == -1 {
				require.Equal(t, split+4, len(wire), "bodyless UDP SIP without Content-Length, frame %d", i+1)
				contentLength = 0
			}
			require.Equal(t, len(wire)-split-4, contentLength)
			if contentLength > 0 {
				require.Equal(t, "application/sdp", contentType)
				sdpSIP++
				verify(i+1, wire[split+4:], false, frame, udp+8+split+4)
			}
		} else {
			rtpCount++
			require.GreaterOrEqual(t, len(wire), 12)
			require.Equal(t, byte(2), wire[0]>>6)
		}
	}
	require.Equal(t, 130, megacoCount)
	require.Equal(t, 92, sipCount)
	require.Equal(t, 1330, rtpCount)
	require.Equal(t, 14, sdpMegaco)
	require.Equal(t, 28, sdpSIP)
	require.Equal(t, 27, ads)
	require.Equal(t, 21, nonAds)
	require.Equal(t, 1, empty)
	t.Logf("SDP descriptors/bodies: advertisements=%d non-T38=%d empty=%d", ads, nonAds, empty)
	// All 27 dynamic-tshark 't38' frames are independently continuous RTP PCMA
	// headers, not proof of UDPTL. Include the immediately preceding RTP frame.
	previous := frames[1429-1][42:]
	require.Equal(t, uint16(0x733), binary.BigEndian.Uint16(previous[2:4]))
	require.Equal(t, uint32(0x67d396e8), binary.BigEndian.Uint32(previous[4:8]))
	for i := 0; i < 27; i++ {
		frame := frames[1436+2*i-1]
		wire := frame[42 : 42+172]
		require.Equal(t, []byte{0x80, 0x08}, wire[:2])
		require.Equal(t, uint16(0x734+i), binary.BigEndian.Uint16(wire[2:4]))
		require.Equal(t, uint32(0x67d39788+i*160), binary.BigEndian.Uint32(wire[4:8]))
		require.Equal(t, uint32(0x0eaf0eaf), binary.BigEndian.Uint32(wire[8:12]))
		require.Equal(t, uint16(180), binary.BigEndian.Uint16(frame[38:40]))
		require.Equal(t, []byte{10, 35, 60, 100}, frame[26:30])
		require.Equal(t, []byte{10, 23, 1, 52}, frame[30:34])
		t38SDPTestReject(t, wire, false, "t38-sdp:")
	}
}

func TestProtocolCorpusT38SDPResourcesAndIsolation(t *testing.T) {
	for _, entry := range []string{"T38SDPAdvertisement", "T38H248SDPAdvertisement", "T38SDPCarrier", "T38H248SDPCarrier"} {
		_, err := parser.ParseBinary(bytes.NewReader([]byte(t38SDPTestFull)), t38SDPTestRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, t38SDPTestRule, entry)
		require.Error(t, err)
		for bits := uint64(1); bits < 8; bits++ {
			r := &giopCarrierTestBitReader{Reader: bytes.NewReader([]byte(t38SDPTestFull + "x")), bits: uint64(len(t38SDPTestFull))*8 + bits}
			_, err = parser.ParseBinary(r, t38SDPTestRule, entry)
			require.ErrorContains(t, err, "byte")
			require.Equal(t, len(t38SDPTestFull)+1, r.Len())
		}
	}
	for _, lines := range []int{1024, 1025} {
		wire := t38SDPTestFull + strings.Repeat("a=x\r\n", lines-6)
		if lines == 1024 {
			n := protocolCorpusRequireBoundedRuleParse(t, []byte(wire), t38SDPTestRule, "T38SDPAdvertisement")
			require.Equal(t, []byte(wire), NodeToBytes(n))
		} else {
			t38SDPTestReject(t, []byte(wire), false, "1024-line")
		}
	}
	for _, size := range []int{4096, 4097} {
		wire := t38SDPTestFull + "a=x:" + strings.Repeat("x", size-6) + "\r\n"
		if size == 4096 {
			n := protocolCorpusRequireBoundedRuleParse(t, []byte(wire), t38SDPTestRule, "T38SDPAdvertisement")
			require.Equal(t, []byte(wire), NodeToBytes(n))
		} else {
			t38SDPTestReject(t, []byte(wire), false, "4096-byte")
		}
	}
	oversize := &giopCarrierTestBitReader{Reader: bytes.NewReader(make([]byte, 65537)), bits: 65537 * 8}
	_, err := parser.ParseBinary(oversize, t38SDPTestRule, "T38SDPAdvertisement")
	require.ErrorContains(t, err, "boundary")
	require.Equal(t, 65537, oversize.Len())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f := t38SDPTestFixtures[i%len(t38SDPTestFixtures)]
			wire := []byte(f.wire)
			n := protocolCorpusRequireBoundedRuleParse(t, wire, t38SDPTestRule, t38SDPTestEntry(f.h248, false))
			require.Equal(t, wire, NodeToBytes(n))
			t38SDPTestTree(t, n, wire, 0)
		}(i)
	}
	wg.Wait()
}
