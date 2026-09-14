package bin_parser

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

// Original nDPI capture, unchanged. The independent Wireshark/sequence ledger
// below separates missing stream context, transport-only records and unknown
// port-5223 binary data. XML syntax alone is not an XMPP semantic positive.
const xmppCorpusRule = "application-layer.xmpp"
const xmppCorpusHeader = `<stream:stream xmlns:stream="http://etherx.jabber.org/streams" xmlns="jabber:client" version="1.0">`

type xmppCorpusRecord struct {
	number         int
	frame, payload []byte
	tcp            *layers.TCP
	flow           string
}

func xmppCorpusRecords(t *testing.T) []xmppCorpusRecord {
	t.Helper()
	frames := protocolCorpusAuditPackets(t, "testdata/protocol-corpus/captures/ndpi/ndpi-xmpp-jabber.pcap")
	require.Len(t, frames, 376)
	var result []xmppCorpusRecord
	counts := map[string]int{}
	ports := map[int]int{57094: 0, 57122: 1, 57126: 2, 57129: 3, 57147: 4, 57149: 5, 53460: 6, 34218: 7, 37614: 8, 58388: 9, 41420: 10, 34070: 11}
	for i, frame := range frames {
		packet := gopacket.NewPacket(frame, layers.LayerTypeEthernet, gopacket.Default)
		require.Nil(t, packet.ErrorLayer(), "frame %d", i+1)
		r := xmppCorpusRecord{number: i + 1, frame: frame}
		if packet.Layer(layers.LayerTypeARP) != nil {
			counts["arp"]++
		} else {
			r.tcp = packet.Layer(layers.LayerTypeTCP).(*layers.TCP)
			r.payload = r.tcp.Payload
			port, direction := int(r.tcp.SrcPort), "C"
			if port == 5222 || port == 5223 {
				port, direction = int(r.tcp.DstPort), "S"
			}
			flow, ok := ports[port]
			require.True(t, ok)
			r.flow = fmt.Sprint(flow, direction)
			if len(r.payload) == 0 {
				counts["tcp-controls"]++
			} else if flow < 7 {
				counts["plaintext"]++
			} else {
				counts["unknown-binary"]++
			}
		}
		result = append(result, r)
	}
	require.Equal(t, map[string]int{"arp": 18, "tcp-controls": 182, "plaintext": 137, "unknown-binary": 39}, counts)
	return result
}

func xmppCorpusMessage(t *testing.T, n *base.Node, fragment bool) *stream_parser.XMPPMessage {
	t.Helper()
	entry, key := "XMPP", "XMPP Message"
	if fragment {
		entry, key = "XMPPFragment", "XMPP Fragment"
	}
	if nested := protocolCorpusFindNode(n, entry); nested != nil {
		n = nested
	}
	info, ok := n.Cfg.GetItem("additionInfo").(map[string]any)
	require.True(t, ok, "missing fields on %s", n.Name)
	m, ok := info[key].(*stream_parser.XMPPMessage)
	require.True(t, ok, "raw XML is not parsed XMPP fields")
	require.Equal(t, fragment, m.ContextRequired)
	require.False(t, m.ConnectionStateValidated)
	return m
}

func xmppCorpusParse(t *testing.T, wire, header string, fragment bool) *stream_parser.XMPPMessage {
	t.Helper()
	entry := "XMPP"
	if fragment {
		entry = "XMPPFragment"
	}
	n := protocolCorpusRequireBoundedRuleParseWithConfig(t, []byte(wire), xmppCorpusRule, entry, map[string]any{"xmppStreamHeader": header})
	return xmppCorpusMessage(t, n, fragment)
}

func TestProtocolCorpusXMPPCapturedBinaryDisposition(t *testing.T) {
	seen := map[uint32][]byte{}
	binaryCount, duplicates := 0, 0
	for _, r := range xmppCorpusRecords(t) {
		if r.tcp == nil || len(r.payload) == 0 || r.tcp.SrcPort != 5223 && r.tcp.DstPort != 5223 {
			continue
		}
		binaryCount++
		n := protocolCorpusRequireBoundedRuleParse(t, r.frame, "ethernet", "Ethernet")
		require.Nil(t, protocolCorpusFindNode(n, "XMPP"))
		protocolCorpusRequireValue(t, n, "Remaining Payload", r.payload)
		tail := protocolCorpusFindNode(n, "Remaining Payload")
		require.Equal(t, [2]uint64{uint64(len(r.frame)-len(r.payload)) * 8, uint64(len(r.frame)) * 8}, stream_parser.GetNodeResultPos(tail))
		if r.flow == "10C" || r.flow == "10S" {
			if previous, ok := seen[r.tcp.Seq]; ok {
				require.Equal(t, previous, r.payload)
				require.Contains(t, []int{353, 354}, r.number)
				duplicates++
			} else {
				seen[r.tcp.Seq] = r.payload
			}
		}
	}
	require.Equal(t, 39, binaryCount)
	require.Equal(t, 2, duplicates)
}

// frame(s)/kind/wire byte length, independently extracted with Wireshark and a
// standard XML token oracle, including the four cross-segment IQ elements.
var xmppCorpusLedger = map[string]string{
	"0C": "6/d/22 8/o/116 14/auth/162 17/response/338 21/response/52 25/o/116 31/iq/123 35/iq/94 39/iq/112 41/iq/111 45/iq/122 49/iq/118 53/iq/67 53/iq/73 53/iq/155 53/iq/78 53/iq/120 71/presence/181 73/iq/217 81/iq/168 85/iq/240",
	"0S": "10/d/21 10/o/158 12/features/285 15/challenge/160 19/challenge/120 23/success/51 27/d/21 27/o/158 29/features/379 33/iq/132 37/iq/106 43/iq/236 46+47+48/iq/3379 55/iq/732 56+57+58/iq/3005 63/iq/137 64/iq/143 65/iq/260 66/iq/148 75/iq/387 77/presence/257 78/iq/252 83/iq/214 87/iq/286",
	"1C": "92/d/22 94/o/116 100/auth/162 103/response/338 107/response/52 111/o/116 117/iq/123 121/iq/94 125/iq/112 127/iq/111 137/iq/122 139/iq/118 139/iq/67 139/iq/73 139/iq/155 139/iq/78 139/iq/120 157/presence/181 159/iq/217 167/iq/168 171/iq/240",
	"1S": "96/d/21 96/o/156 98/features/285 101/challenge/160 105/challenge/120 109/success/51 113/d/21 113/o/158 115/features/379 119/iq/132 123/iq/106 129/iq/236 130+131+132/iq/3379 141/iq/732 142+143+144/iq/3005 145/iq/137 146/iq/143 149/iq/260 154/iq/148 161/iq/387 163/presence/257 164/iq/253 169/iq/214 173/iq/286",
	"2C": "175/close/16",
	"3C": "182/iq/140 186/iq/153 189/iq/196 194/iq/160 197/iq/235 202/iq/69 209/iq/703 216/iq/154",
	"3S": "184/iq/257 187/iq/199 190/iq/242 195/iq/415 198/iq/252 203/iq/106 210/iq/106 217/iq/102",
	"4C": "222/d/22 224/o/116 230/auth/162 233/response/338 237/close/16",
	"4S": "226/d/21 226/o/158 228/features/285 231/challenge/160 235/failure/132 243/close/16",
	"5C": "249/presence/239 254/iq/182 257/iq/182 262/iq/150 265/presence/54 269/iq/48 271/iq/48 275/iq/48 281/message/135 283/message/148 285/message/132",
	"5S": "250/presence/408 251/message/120 255/iq/463 258/iq/127 263/iq/204 264/iq/106 267/iq/220 273/iq/222 276/presence/101 279/presence/105 287/message/216",
	"6C": "292/d/22 294/o/119 298/close/16",
}

type xmppCorpusChunk struct {
	records []xmppCorpusRecord
	wire    string
}

func xmppCorpusChunks(t *testing.T) map[string][]xmppCorpusChunk {
	t.Helper()
	flows := map[string][]xmppCorpusChunk{}
	gaps := map[int]uint32{}
	for _, r := range xmppCorpusRecords(t) {
		if r.tcp == nil || len(r.payload) == 0 || r.tcp.SrcPort != 5222 && r.tcp.DstPort != 5222 {
			continue
		}
		chunks := flows[r.flow]
		if len(chunks) > 0 {
			last := &chunks[len(chunks)-1]
			previous := last.records[len(last.records)-1]
			end := previous.tcp.Seq + uint32(len(previous.payload))
			if r.tcp.Seq == end {
				last.records = append(last.records, r)
				last.wire += string(r.payload)
				flows[r.flow] = chunks
				continue
			}
			require.Greater(t, r.tcp.Seq, end, "no invented overlap/retransmission in plaintext frame %d", r.number)
			gaps[r.number] = r.tcp.Seq - end
		}
		flows[r.flow] = append(chunks, xmppCorpusChunk{records: []xmppCorpusRecord{r}, wire: string(r.payload)})
	}
	require.Equal(t, map[int]uint32{194: 138, 216: 138, 195: 212, 217: 212, 262: 1150, 281: 69, 263: 2009, 287: 106}, gaps)
	require.Len(t, flows, 12, "direction flows with plaintext, not the 12 TCP conversations")
	return flows
}

func xmppCorpusSpanFrames(t *testing.T, chunk xmppCorpusChunk, start, end int) string {
	t.Helper()
	var frames []string
	offset := 0
	for _, r := range chunk.records {
		if start < offset+len(r.payload) && end > offset {
			frames = append(frames, strconv.Itoa(r.number))
		}
		offset += len(r.payload)
	}
	require.NotEmpty(t, frames)
	return strings.Join(frames, "+")
}

func TestProtocolCorpusXMPPAllReassembledEvents(t *testing.T) {
	counts := map[string]int{}
	semantic, partial, framing := 0, 0, 0
	for flow, chunks := range xmppCorpusChunks(t) {
		var actual []string
		fragment := strings.HasPrefix(flow, "2") || strings.HasPrefix(flow, "3") || strings.HasPrefix(flow, "5")
		for _, chunk := range chunks {
			m := xmppCorpusParse(t, chunk.wire, "", fragment)
			if !fragment {
				require.Equal(t, strings.HasPrefix(flow, "0") || strings.HasPrefix(flow, "1"), m.StreamOpen)
				require.Equal(t, strings.HasPrefix(flow, "0") || strings.HasPrefix(flow, "1"), m.Restarts == 1)
			}
			type span struct {
				start, end int
				kind       string
			}
			var spans []span
			search := 0
			for _, decl := range m.Declarations {
				begin := strings.Index(chunk.wire[search:], decl) + search
				require.GreaterOrEqual(t, begin, search)
				end := begin + len(decl)
				// The independent ledger includes the declaration's trailing LF.
				if end < len(chunk.wire) && chunk.wire[end] == '\n' {
					end++
				}
				spans = append(spans, span{begin, end, "d"})
				search = end
				counts["d"]++
			}
			for _, event := range m.Events {
				n := event.Element
				kind := n.Name.Local
				switch event.Kind {
				case "stream-open":
					kind = "o"
					framing++
				case "stream-close", "stream-close-context-required":
					kind = "close"
					framing++
				default:
					xmppCorpusXMLOracle(t, n, chunk.wire)
					if fragment {
						partial++
						require.Nil(t, event.Stanza)
						require.Empty(t, n.Name.Space)
					} else {
						semantic++
						if kind == "iq" || kind == "message" || kind == "presence" {
							require.NotNil(t, event.Stanza)
						}
					}
				}
				counts[kind]++
				spans = append(spans, span{n.Offset, n.End, kind})
				xmppCorpusCapturedFieldOracle(t, event, chunk.wire)
			}
			sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
			previousEnd := 0
			for _, s := range spans {
				require.GreaterOrEqual(t, s.start, previousEnd, "framing spans must not overlap")
				require.Empty(t, strings.Trim(chunk.wire[previousEnd:s.start], " \t\r\n"), "only XML whitespace may separate events")
				actual = append(actual, fmt.Sprintf("%s/%s/%d", xmppCorpusSpanFrames(t, chunk, s.start, s.end), s.kind, s.end-s.start))
				previousEnd = s.end
			}
			require.Empty(t, strings.Trim(chunk.wire[previousEnd:], " \t\r\n"), "no skipped trailing event")
		}
		require.Equal(t, strings.Fields(xmppCorpusLedger[flow]), actual, "flow %s: no joining across sequence gaps", flow)
	}
	require.Equal(t, map[string]int{"d": 9, "o": 11, "close": 4, "iq": 84, "presence": 9, "message": 5, "features": 5, "auth": 3, "response": 5, "challenge": 5, "success": 2, "failure": 1}, counts)
	require.Equal(t, 81, semantic)
	require.Equal(t, 38, partial)
	require.Equal(t, 15, framing)
}

// The independent RawToken scan finds the latest actual wire opening before
// the event. In particular it never consumes the tested decoder's namespace map.
func xmppCorpusObservedHeader(t *testing.T, wire string, before int) (string, xml.Name) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(wire[:before]))
	header := ""
	var name xml.Name
	for {
		begin := int(d.InputOffset())
		token, err := d.RawToken()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Local != "stream" {
				continue
			}
			for _, attr := range token.Attr {
				if attr.Value == "http://etherx.jabber.org/streams" && (attr.Name.Space == "xmlns" && attr.Name.Local == token.Name.Space || token.Name.Space == "" && attr.Name == (xml.Name{Local: "xmlns"})) {
					header = wire[begin:int(d.InputOffset())]
					name = token.Name
				}
			}
		case xml.EndElement:
			if token.Name == name {
				header = ""
				name = xml.Name{}
			}
		}
	}
	return header, name
}

// A separate standard Token decoder supplies expanded names, ordered attrs,
// text and children under the original captured header. No header is supplied
// for the 38 missing-context fragments, including after every capture gap.
func xmppCorpusXMLOracle(t *testing.T, root *stream_parser.XMPPElement, wire string) {
	t.Helper()
	header, name := xmppCorpusObservedHeader(t, wire, root.Offset)
	closing := "</oracle>"
	if header == "" {
		header = "<oracle>"
	} else if name.Space == "" {
		closing = "</" + name.Local + ">"
	} else {
		closing = "</" + name.Space + ":" + name.Local + ">"
	}
	d := xml.NewDecoder(strings.NewReader(header + root.Raw + closing))
	first, err := d.Token()
	require.NoError(t, err)
	namespaceAttrs := func(inherited map[string]string, attrs []xml.Attr) map[string]string {
		ns := map[string]string{}
		for k, v := range inherited {
			ns[k] = v
		}
		for _, a := range attrs {
			if a.Name.Space == "xmlns" {
				ns[a.Name.Local] = a.Value
			} else if a.Name == (xml.Name{Local: "xmlns"}) {
				ns[""] = a.Value
			}
		}
		return ns
	}
	baseNS := namespaceAttrs(map[string]string{"xml": "http://www.w3.org/XML/1998/namespace"}, first.(xml.StartElement).Attr)
	baseLang := ""
	for _, attr := range first.(xml.StartElement).Attr {
		if attr.Name == (xml.Name{Space: "http://www.w3.org/XML/1998/namespace", Local: "lang"}) {
			baseLang = attr.Value
		}
	}
	var walk func(*stream_parser.XMPPElement, map[string]string, string)
	walk = func(n *stream_parser.XMPPElement, inherited map[string]string, lang string) {
		token, err := d.Token()
		require.NoError(t, err)
		start, ok := token.(xml.StartElement)
		require.True(t, ok)
		require.Equal(t, n.Name, start.Name)
		ns := namespaceAttrs(inherited, start.Attr)
		require.Equal(t, ns, n.Namespaces)
		var attrs []xml.Attr
		for _, a := range start.Attr {
			if a.Name.Space != "xmlns" && !(a.Name.Space == "" && a.Name.Local == "xmlns") {
				attrs = append(attrs, a)
			}
			if a.Name == (xml.Name{Space: "http://www.w3.org/XML/1998/namespace", Local: "lang"}) {
				lang = a.Value
			}
		}
		require.Equal(t, n.Attributes, attrs)
		require.Equal(t, lang, n.Language)
		for _, content := range n.Content {
			if content.Child != nil {
				walk(content.Child, ns, lang)
			} else {
				token, err = d.Token()
				require.NoError(t, err)
				text, ok := token.(xml.CharData)
				require.True(t, ok)
				require.Equal(t, content.Text, string(text))
			}
		}
		token, err = d.Token()
		require.NoError(t, err)
		end, ok := token.(xml.EndElement)
		require.True(t, ok)
		require.Equal(t, n.Name, end.Name)
	}
	walk(root, baseNS, baseLang)
	_, err = d.Token()
	require.NoError(t, err)
	_, err = d.Token()
	require.ErrorIs(t, err, io.EOF)
}

func xmppCorpusCapturedFieldOracle(t *testing.T, event stream_parser.XMPPEvent, wire string) {
	t.Helper()
	n := event.Element
	require.Equal(t, wire[n.Offset:n.End], n.Raw)
	switch event.Kind {
	case "sasl-auth", "sasl-challenge", "sasl-response", "sasl-success":
		// XML Text has already been compared to an independent Token oracle.
		// Decode the captured spelling separately; mechanism payload semantics
		// and session acceptance remain outside this parser's contract.
		var expected []byte
		if n.Text != "" && n.Text != "=" {
			var err error
			expected, err = base64.StdEncoding.DecodeString(n.Text)
			require.NoError(t, err)
		}
		require.Equal(t, expected, event.Token, "captured %s token bytes", event.Kind)
	}
	if event.Kind == "features" && len(event.Mechanisms) > 0 {
		require.Equal(t, []string{"PLAIN", "DIGEST-MD5", "X-OAUTH2", "SCRAM-SHA-1"}, event.Mechanisms)
	}
	if event.Kind == "sasl-auth" {
		require.Equal(t, "DIGEST-MD5", event.Mechanism)
		require.Empty(t, event.Token, "captured auth selects a mechanism without an initial response")
	}
	if event.Kind == "sasl-failure" {
		require.Equal(t, "not-authorized", event.Condition)
		require.Equal(t, []stream_parser.XMPPLanguageText{{Language: "en", Text: "Invalid username or password"}}, event.Texts)
	}
	if event.Stanza != nil {
		s := event.Stanza
		attrs := map[string]string{}
		for _, a := range n.Attributes {
			if a.Name.Space == "" {
				attrs[a.Name.Local] = a.Value
			}
		}
		typeValue, present := attrs["type"]
		if !present && s.Kind == "message" {
			typeValue = "normal"
		}
		if !present && s.Kind == "presence" {
			typeValue = "available"
		}
		require.Equal(t, n.Name.Local, s.Kind)
		require.Equal(t, typeValue, s.Type)
		require.Equal(t, attrs["id"], s.ID)
		require.Equal(t, attrs["from"], s.From)
		require.Equal(t, attrs["to"], s.To)
		require.Equal(t, n.Language, s.Language)
		if s.BindPresent {
			if s.Type == "set" {
				require.Equal(t, "darkstar", s.BindResource)
			} else {
				require.Equal(t, "tom@cs-xmpp.lan/darkstar", s.BindJID)
			}
		}
		if len(n.Raw) == 3379 || len(n.Raw) == 3005 {
			require.Len(t, s.Payloads, 1)
			query := s.Payloads[0]
			require.Equal(t, xml.Name{Space: "http://jabber.org/protocol/disco#info", Local: "query"}, query.Name)
			identities, features := 0, 0
			for _, child := range query.Children {
				if child.Name.Local == "identity" {
					identities++
				} else if child.Name.Local == "feature" {
					features++
				} else if child.Name == (xml.Name{Space: "jabber:x:data", Local: "x"}) {
					require.NotEmpty(t, child.Children)
				} else {
					t.Fatalf("unexpected discovery field %s", child.Name.Local)
				}
			}
			if len(n.Raw) == 3379 {
				require.Equal(t, 2, identities)
				require.Equal(t, 52, features)
				require.Contains(t, []string{"purplef8b14dc7", "purple6dd1b122"}, s.ID)
			} else {
				require.Equal(t, 1, identities)
				require.Equal(t, 41, features)
				require.Contains(t, []string{"purplef8b14dc9", "purple6dd1b124"}, s.ID)
			}
		}
	}
}

func TestProtocolCorpusXMPPPublicEveryRecordAndCallerContext(t *testing.T) {
	independent := map[int]bool{}
	for _, frame := range []int{8, 10, 14, 15, 17, 19, 21, 23, 25, 27, 94, 96, 100, 101, 103, 105, 107, 109, 111, 113, 224, 226, 230, 231, 233, 235, 294} {
		independent[frame] = true
	}
	fragments := map[int]bool{}
	for _, frame := range []int{46, 47, 48, 56, 57, 58, 130, 131, 132, 142, 143, 144} {
		fragments[frame] = true
	}
	headers := map[string]string{}
	plain, standalone, contextual := 0, 0, 0
	for _, r := range xmppCorpusRecords(t) {
		n := protocolCorpusRequireBoundedRuleParse(t, r.frame, "ethernet", "Ethernet")
		if r.tcp == nil || len(r.payload) == 0 || r.tcp.SrcPort != 5222 && r.tcp.DstPort != 5222 {
			require.Nil(t, protocolCorpusFindNode(n, "XMPP"), "frame %d", r.number)
			continue
		}
		plain++
		if independent[r.number] {
			standalone++
			xmppCorpusMessage(t, n, false)
		} else {
			require.Nil(t, protocolCorpusFindNode(n, "XMPP"), "frame %d must not infer a stream namespace", r.number)
			protocolCorpusRequireValue(t, n, "Remaining Payload", r.payload)
		}
		cfg := map[string]any{"xmppStreamHeader": headers[r.flow]}
		public := protocolCorpusRequireBoundedRuleParseWithConfig(t, r.frame, "ethernet", "Ethernet", cfg)
		xmpp := protocolCorpusFindNode(public, "XMPP")
		missing := strings.HasPrefix(r.flow, "2") || strings.HasPrefix(r.flow, "3") || strings.HasPrefix(r.flow, "5")
		prolog := r.number == 6 || r.number == 92 || r.number == 222 || r.number == 292
		if missing || prolog || fragments[r.number] {
			require.Nil(t, xmpp, "frame %d is unresolved or only part of an event", r.number)
			protocolCorpusRequireValue(t, public, "Remaining Payload", r.payload)
			tail := protocolCorpusFindNode(public, "Remaining Payload")
			require.Equal(t, [2]uint64{uint64(len(r.frame)-len(r.payload)) * 8, uint64(len(r.frame)) * 8}, stream_parser.GetNodeResultPos(tail))
			continue
		}
		contextual++
		m := xmppCorpusMessage(t, public, false)
		direct := xmppCorpusParse(t, string(r.payload), headers[r.flow], false)
		require.Equal(t, direct, m, "same bounded text and explicitly observed directional header, frame %d", r.number)
		headers[r.flow] = m.StreamHeader
		txt := protocolCorpusFindNode(xmpp, "XML Text")
		require.Equal(t, [2]uint64{uint64(len(r.frame)-len(r.payload)) * 8, uint64(len(r.frame)) * 8}, stream_parser.GetNodeResultPos(txt))
	}
	require.Equal(t, 137, plain)
	require.Equal(t, 27, standalone)
	require.Equal(t, 82, contextual)
}

func TestProtocolCorpusXMPPExplicitFragmentsAndCapturedFields(t *testing.T) {
	count := 0
	for _, r := range xmppCorpusRecords(t) {
		if len(r.payload) == 0 || !(strings.HasPrefix(r.flow, "2") || strings.HasPrefix(r.flow, "3") || strings.HasPrefix(r.flow, "5")) {
			continue
		}
		count++
		m := xmppCorpusParse(t, string(r.payload), "", true)
		require.Len(t, m.Events, 1)
		event := m.Events[0]
		require.Nil(t, event.Stanza)
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(r.payload), xmppCorpusRule, "XMPP")
		require.Error(t, err, "frame %d must not become semantic merely because the root name is familiar", r.number)
		if r.number == 175 {
			require.Equal(t, "stream-close-context-required", event.Kind)
			continue
		}
		n := event.Element
		require.Empty(t, n.Name.Space)
		// Fully explicit external context is a separate positive control. It
		// is not counted as an observed namespace in this original capture.
		external := xmppCorpusParse(t, string(r.payload), xmppCorpusHeader, false)
		require.True(t, external.CallerContextUsed)
		s := external.Events[0].Stanza
		require.NotNil(t, s)
		switch r.number {
		case 209:
			vcard := xmppCorpusFindElement(n, "vcard-temp", "vCard")
			require.NotNil(t, vcard)
			for name, value := range map[string]string{"FN": "Thomas", "FAMILY": "Peterson", "GIVEN": "Tom", "NICKNAME": "tom-nick", "ORGNAME": "CloudShark"} {
				require.Equal(t, value, xmppCorpusFindElement(vcard, "vcard-temp", name).Text)
			}
			desc := xmppCorpusFindElement(vcard, "vcard-temp", "DESC")
			require.Contains(t, desc.Raw, "I&apos;m Tom")
			require.Contains(t, desc.Text, "I'm Tom")
		case 250:
			x := xmppCorpusFindElement(n, "http://jabber.org/protocol/muc#user", "x")
			require.NotNil(t, x)
			item := xmppCorpusFindElement(x, x.Name.Space, "item")
			for key, value := range map[string]string{"jid": "tom@cs-xmpp.lan/darkstar", "role": "moderator", "affiliation": "owner"} {
				require.Equal(t, value, xmppCorpusAttr(item, key))
			}
			var codes []string
			for _, child := range x.Children {
				if child.Name.Local == "status" {
					codes = append(codes, xmppCorpusAttr(child, "code"))
				}
			}
			require.Equal(t, []string{"201", "110"}, codes)
		case 251:
			require.Equal(t, "groupchat", s.Type)
			require.Len(t, s.Subjects, 1)
			require.Empty(t, s.Subjects[0].Text)
		case 263, 267, 273:
			item := xmppCorpusFindElement(n, "jabber:iq:roster", "item")
			require.Equal(t, "chatback@cs-xmpp.lan", xmppCorpusAttr(item, "jid"))
			require.Equal(t, "cs-xmpp", xmppCorpusFindElement(item, "jabber:iq:roster", "group").Text)
			ask, subscription := "", ""
			if r.number == 267 {
				ask = "subscribe"
			}
			if r.number == 273 {
				subscription = "to"
			}
			require.Equal(t, ask, xmppCorpusAttr(item, "ask"))
			require.Equal(t, subscription, xmppCorpusAttr(item, "subscription"))
		case 265:
			require.Equal(t, "subscribe", s.Type)
		case 276:
			require.Equal(t, "subscribed", s.Type)
		case 279:
			require.Equal(t, "chatback@cs-xmpp.lan/104588072251790900698", s.From)
		case 281:
			require.NotNil(t, xmppCorpusFindElement(n, "http://jabber.org/protocol/chatstates", "composing"))
			require.Empty(t, s.Bodies)
		case 283, 287:
			require.NotNil(t, xmppCorpusFindElement(n, "http://jabber.org/protocol/chatstates", "active"))
			require.Len(t, s.Bodies, 1)
			require.Equal(t, "hey", s.Bodies[0].Text)
		case 285:
			require.NotNil(t, xmppCorpusFindElement(n, "http://jabber.org/protocol/chatstates", "active"))
			require.Empty(t, s.Bodies)
		}
	}
	require.Equal(t, 39, count, "38 complete XML elements plus one unresolved close")
}

func xmppCorpusFindElement(n *stream_parser.XMPPElement, space, local string) *stream_parser.XMPPElement {
	if n.Name == (xml.Name{Space: space, Local: local}) {
		return n
	}
	for _, child := range n.Children {
		if found := xmppCorpusFindElement(child, space, local); found != nil {
			return found
		}
	}
	return nil
}

func xmppCorpusAttr(n *stream_parser.XMPPElement, key string) string {
	for _, a := range n.Attributes {
		if a.Name == (xml.Name{Local: key}) {
			return a.Value
		}
	}
	return ""
}

func TestProtocolCorpusXMPPBoundariesFallbackAndNonzeroValues(t *testing.T) {
	wires := []string{`<message xmlns="jabber:client" id="m-23" from="a@b" to="c@d" type="chat" xml:lang="fr"><body>salut &amp; bye</body><thread parent="p1">t7</thread></message>`, `<presence xmlns="jabber:client"><show>dnd</show><priority>127</priority></presence>`, `<iq xmlns="jabber:client" type="result" id="r7"><bind xmlns="urn:ietf:params:xml:ns:xmpp-bind"><jid>a@b/desk</jid></bind></iq>`, `<response xmlns="urn:ietf:params:xml:ns:xmpp-sasl">AQIDBA==</response>`}
	for _, wire := range wires {
		for cut := 0; cut < len(wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(wire[:cut])), xmppCorpusRule, "XMPP")
			require.Error(t, err, "strict short prefix %d/%d", cut, len(wire))
		}
		m := xmppCorpusParse(t, wire, "", false)
		require.Len(t, m.Events, 1)
		frame := ipv4TCPFrame(t, 53000, 5222, []byte(wire))
		public := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		require.Equal(t, m, xmppCorpusMessage(t, public, false))
	}
	m := xmppCorpusParse(t, strings.Join(wires, ""), "", false)
	require.Len(t, m.Events, 4)
	require.Equal(t, "m-23", m.Events[0].Stanza.ID)
	require.Equal(t, "a@b", m.Events[0].Stanza.From)
	require.Equal(t, "c@d", m.Events[0].Stanza.To)
	require.Equal(t, []stream_parser.XMPPLanguageText{{Language: "fr", Text: "salut & bye"}}, m.Events[0].Stanza.Bodies)
	require.Equal(t, "p1", m.Events[0].Stanza.ThreadParent)
	require.Equal(t, int8(127), m.Events[1].Stanza.Priority)
	require.Equal(t, "a@b/desk", m.Events[2].Stanza.BindJID)
	require.Equal(t, []byte{1, 2, 3, 4}, m.Events[3].Token)
	for _, bad := range []string{`<message/>`, `<other/>`, `<message xmlns="jabber:client"><body>x</message>`, wires[0] + "x", wires[0] + `<message`, xmppCorpusHeader + `</stream:stream><message/>`, `<?xml version="1.0"?>`, strings.Repeat("x", (1<<20)+1)} {
		_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(bad)), xmppCorpusRule, "XMPP")
		require.Error(t, err)
		if len(bad) < 65500 {
			frame := ipv4TCPFrame(t, 53000, 5222, []byte(bad))
			public := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			require.Nil(t, protocolCorpusFindNode(public, "XMPP"))
			protocolCorpusRequireValue(t, public, "Remaining Payload", []byte(bad))
		}
	}
	_, err := parser.ParseBinary(bytes.NewReader([]byte(wires[0])), xmppCorpusRule, "XMPP")
	require.ErrorContains(t, err, "boundary")
	_, err = parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(wires[0])), xmppCorpusRule, "XMPPFragment")
	require.Error(t, err)
	tls := []byte{23, 3, 3, 0, 3, 'a', 'b', 'c'}
	for _, port := range []int{5223, 8443} {
		frame := ipv4TCPFrame(t, 53000, layers.TCPPort(port), tls)
		public := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
		record := protocolCorpusFindNode(public, "Record Layer")
		require.NotNil(t, record, "TLS port %d; terminals %v", port, protocolCorpusProcessedTerminalNames(public))
		protocolCorpusRequireValue(t, record, "ContentType", uint64(23))
		protocolCorpusRequireValue(t, record, "Version", uint64(0x0303))
		protocolCorpusRequireValue(t, record, "Length", uint64(3))
	}
	unknown := []byte{0x25, 0, 0x45, 0x14, 4, 0x11, 0x8a}
	for _, wire := range [][]byte{unknown, tls} {
		for cut := 1; cut < len(wire); cut++ {
			frame := ipv4TCPFrame(t, 53000, 5223, wire[:cut])
			n := protocolCorpusRequireBoundedRuleParse(t, frame, "ethernet", "Ethernet")
			protocolCorpusRequireValue(t, n, "Remaining Payload", wire[:cut])
			require.Equal(t, [2]uint64{54 * 8, uint64(54+cut) * 8}, stream_parser.GetNodeResultPos(protocolCorpusFindNode(n, "Remaining Payload")), "Ethernet minimum-frame padding is outside the IP/TCP payload")
			require.Nil(t, protocolCorpusFindNode(n, "XMPPTLSRecordHeader"), "the guard probe is always rolled back")
		}
	}
}
