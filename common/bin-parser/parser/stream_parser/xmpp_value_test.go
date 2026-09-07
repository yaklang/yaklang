package stream_parser

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const xmppTestHeader = `<stream:stream xmlns:stream="http://etherx.jabber.org/streams" xmlns="jabber:client" version="1.0" xml:lang="en">`

func TestXMPPValueFramingAndCoreFields(t *testing.T) {
	wire := `<?xml version="1.0"?>` + xmppTestHeader + `<message id="one" to="a@example.test" type="chat"><body>A&amp;B</body><body xml:lang="zh">你好</body><subject/><thread parent="prior">t1</thread><x xmlns="urn:example:extension">a<y value="v"/>b</x></message><presence><show>away</show><status>Busy</status><priority>-128</priority></presence><iq id="two" type="set"><bind xmlns="urn:ietf:params:xml:ns:xmpp-bind"><resource>desk</resource></bind></iq></stream:stream>`
	m, err := decodeXMPPText(wire, "")
	require.NoError(t, err)
	require.Len(t, m.Events, 5)
	require.False(t, m.StreamOpen)
	require.True(t, m.StreamClosed)
	require.False(t, m.CallerContextUsed)
	require.False(t, m.ConnectionStateValidated)
	s := m.Events[1].Stanza
	require.Equal(t, "chat", s.Type)
	require.Equal(t, "one", s.ID)
	require.Equal(t, "a@example.test", s.To)
	require.Equal(t, []XMPPLanguageText{{"en", "A&B"}, {"zh", "你好"}}, s.Bodies)
	require.Equal(t, []XMPPLanguageText{{"en", ""}}, s.Subjects)
	require.Equal(t, "t1", s.Thread)
	require.Equal(t, "prior", s.ThreadParent)
	require.True(t, s.ThreadPresent)
	require.Len(t, s.Payloads, 1)
	x := s.Payloads[0]
	require.Equal(t, xml.Name{Space: "urn:example:extension", Local: "x"}, x.Name)
	require.Equal(t, "ab", x.Text)
	require.Len(t, x.Content, 3)
	require.Equal(t, "a", x.Content[0].Text)
	require.Equal(t, x.Children[0], x.Content[1].Child)
	require.Equal(t, "b", x.Content[2].Text)
	require.Equal(t, []xml.Attr{{Name: xml.Name{Local: "value"}, Value: "v"}}, x.Children[0].Attributes)
	require.Equal(t, "away", m.Events[2].Stanza.Show)
	require.Equal(t, int8(-128), m.Events[2].Stanza.Priority)
	require.True(t, m.Events[2].Stanza.PriorityPresent)
	require.Equal(t, "desk", m.Events[3].Stanza.BindResource)
	var walk func(*XMPPElement)
	walk = func(n *XMPPElement) {
		require.Equal(t, wire[n.Offset:n.End], n.Raw)
		for _, child := range n.Children {
			require.GreaterOrEqual(t, child.Offset, n.Offset)
			require.LessOrEqual(t, child.End, n.End)
			walk(child)
		}
	}
	for _, event := range m.Events {
		walk(event.Element)
	}
}

func TestXMPPValueContinuationRestartAndNamespaces(t *testing.T) {
	input := `<iq type="result" id="a"/><stream:stream xmlns:stream="http://etherx.jabber.org/streams" xmlns="jabber:client" version="1.0"><message><body>new</body></message>`
	m, err := decodeXMPPText(input, xmppTestHeader)
	require.NoError(t, err)
	require.True(t, m.CallerContextUsed)
	require.True(t, m.StreamOpen)
	require.Equal(t, 1, m.Restarts)
	require.Equal(t, "en", m.Events[0].Stanza.Language)
	require.Empty(t, m.Events[2].Stanza.Language, "restart replaces inherited language")
	require.Equal(t, m.Events[1].Element.Raw, m.StreamHeader)
	closed, err := decodeXMPPText(`</stream:stream>`, m.StreamHeader)
	require.NoError(t, err)
	require.True(t, closed.StreamClosed)
	partial, err := decodeXMPPText(`</stream:stream>`, "")
	require.NoError(t, err)
	require.True(t, partial.ContextRequired)
	require.Empty(t, partial.Events[0].Element.Name)
	partial, err = decodeXMPPText(`<message><body>unknown namespace</body></message>`, "")
	require.NoError(t, err)
	require.True(t, partial.ContextRequired)
	require.Nil(t, partial.Events[0].Stanza)
	require.Empty(t, partial.Events[0].Element.Name.Space)
	require.Equal(t, "unknown namespace", partial.Events[0].Element.Children[0].Text)
	_, err = decodeXMPPText(`<p:message xmlns:p="jabber:client"><p:body>yes</p:body></p:message>`, "")
	require.Error(t, err)
	_, err = decodeXMPPText(`<message xmlns="jabber:client" xmlns:p="urn:example"><p:x>yes</p:x></message>`, "")
	require.NoError(t, err)
	old := strings.Replace(xmppTestHeader, `version="1.0"`, `version="1.0" xmlns:old="urn:old"`, 1)
	_, err = decodeXMPPText(xmppTestHeader+`<message><old:x/></message>`, old)
	require.ErrorContains(t, err, "unbound")
	// RFC 6120 4.8.2 explicitly permits the prefix-free canonical spelling.
	m, err = decodeXMPPText(`<stream xmlns="http://etherx.jabber.org/streams" version="1.0"><message xmlns="jabber:client"><body>yes</body></message></stream>`, "")
	require.NoError(t, err)
	require.Equal(t, "yes", m.Events[1].Stanza.Bodies[0].Text)
	require.True(t, m.StreamClosed)
	_, err = decodeXMPPText(`<stream xmlns="http://etherx.jabber.org/streams" version="1.0"><message/></stream>`, "")
	require.Error(t, err, "the canonical stream namespace does not imply a content namespace")
}

func TestXMPPValueNegotiationAndErrors(t *testing.T) {
	input := `<stream:features><mechanisms xmlns="urn:ietf:params:xml:ns:xmpp-sasl"><mechanism>PLAIN</mechanism><mechanism>DIGEST-MD5</mechanism></mechanisms><bind xmlns="urn:ietf:params:xml:ns:xmpp-bind"/></stream:features><auth xmlns="urn:ietf:params:xml:ns:xmpp-sasl" mechanism="EXAMPLE">YQ==</auth><challenge xmlns="urn:ietf:params:xml:ns:xmpp-sasl">Yg==</challenge><response xmlns="urn:ietf:params:xml:ns:xmpp-sasl">=</response><success xmlns="urn:ietf:params:xml:ns:xmpp-sasl"/><failure xmlns="urn:ietf:params:xml:ns:xmpp-sasl"><not-authorized/><text xml:lang="en">No</text></failure><starttls xmlns="urn:ietf:params:xml:ns:xmpp-tls"/><proceed xmlns="urn:ietf:params:xml:ns:xmpp-tls"/><iq type="error" id="a"><query xmlns="urn:example"/><error type="cancel" code="404"><item-not-found xmlns="urn:ietf:params:xml:ns:xmpp-stanzas"/><text xmlns="urn:ietf:params:xml:ns:xmpp-stanzas">Missing</text></error></iq>`
	m, err := decodeXMPPText(input, xmppTestHeader)
	require.NoError(t, err)
	require.Len(t, m.Events, 9)
	require.Equal(t, []string{"PLAIN", "DIGEST-MD5"}, m.Events[0].Mechanisms)
	require.Len(t, m.Events[0].Extensions, 1)
	require.Equal(t, "EXAMPLE", m.Events[1].Mechanism)
	require.Equal(t, []byte("a"), m.Events[1].Token)
	require.Equal(t, []byte("b"), m.Events[2].Token)
	require.Empty(t, m.Events[3].Token)
	require.Equal(t, "not-authorized", m.Events[5].Condition)
	require.Equal(t, []XMPPLanguageText{{"en", "No"}}, m.Events[5].Texts)
	require.Equal(t, "tls-starttls", m.Events[6].Kind)
	require.Equal(t, "tls-proceed", m.Events[7].Kind)
	require.Equal(t, "item-not-found", m.Events[8].Stanza.Error.Condition)
	require.Equal(t, "404", m.Events[8].Stanza.Error.LegacyCode)
	m, err = decodeXMPPText(`<s:features xmlns:s="http://etherx.jabber.org/streams"><mechanisms xmlns="urn:ietf:params:xml:ns:xmpp-sasl"><mechanism>PLAIN</mechanism><x xmlns="urn:example:hint">v</x></mechanisms></s:features>`, "")
	require.NoError(t, err)
	require.Equal(t, []string{"PLAIN"}, m.Events[0].Mechanisms)
	require.Len(t, m.Events[0].Extensions, 1)
	require.Equal(t, "v", m.Events[0].Extensions[0].Text)
	for _, value := range []string{"", "plain", "PL AIN", strings.Repeat("A", 21)} {
		_, err = decodeXMPPText(`<s:features xmlns:s="http://etherx.jabber.org/streams"><mechanisms xmlns="urn:ietf:params:xml:ns:xmpp-sasl"><mechanism>`+value+`</mechanism></mechanisms></s:features>`, "")
		require.Error(t, err)
	}
}

func TestXMPPValueAllStructuralLimitEdges(t *testing.T) {
	for _, nodes := range []int{8192, 8193} {
		wire := `<message xmlns="jabber:client">` + strings.Repeat(`<x xmlns="urn:example"/>`, nodes-1) + `</message>`
		m, err := decodeXMPPText(wire, "")
		if nodes == 8192 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "XML structure limit exceeded")
			require.Nil(t, m)
		}
	}
	for _, attrs := range []int{64, 65} {
		wire := `<message xmlns="jabber:client"`
		for i := 1; i < attrs; i++ {
			wire += fmt.Sprintf(` a%d="v"`, i)
		}
		wire += `/>`
		m, err := decodeXMPPText(wire, "")
		if attrs == 64 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "XML structure limit exceeded")
			require.Nil(t, m)
		}
	}
	for _, namespaces := range []int{64, 65} {
		wire := `<message xmlns="jabber:client"`
		for i := 2; i < namespaces; i++ {
			wire += fmt.Sprintf(` xmlns:p%d="urn:example:%d"`, i, i)
		}
		wire += `/>`
		m, err := decodeXMPPText(wire, "")
		if namespaces == 64 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "namespace limit exceeded")
			require.Nil(t, m)
		}
	}
	for _, size := range []int{64 << 10, (64 << 10) + 1} {
		prefix := strings.TrimSuffix(xmppTestHeader, ">")
		header := prefix + strings.Repeat(" ", size-len(prefix)-1) + ">"
		require.Len(t, header, size)
		m, err := decodeXMPPText(header, "")
		if size == 64<<10 {
			require.NoError(t, err)
			_, err = decodeXMPPText(`<message/>`, header)
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			require.Nil(t, m)
			m, err = decodeXMPPText(`<message/>`, header)
			require.ErrorContains(t, err, "context size")
			require.Nil(t, m)
		}
	}
}

func TestXMPPValueInvalidAndShortPrefixes(t *testing.T) {
	valid := `<message xmlns="jabber:client"><body>abc</body></message>`
	for cut := 0; cut < len(valid); cut++ {
		m, err := decodeXMPPText(valid[:cut], "")
		require.Error(t, err, "prefix %d", cut)
		require.Nil(t, m)
	}
	for _, input := range []string{
		`<x/>`, `<?xml version="1.0"?>`, `<?xml?>` + xmppTestHeader,
		` <?xml version="1.0"?>` + xmppTestHeader,
		`<?xml version="1.0" standalone="no"?>` + xmppTestHeader, `<?other a?>` + xmppTestHeader,
		`<!DOCTYPE stream [<!ENTITY a "x">]>` + xmppTestHeader, `<!--x-->` + xmppTestHeader,
		strings.Replace(xmppTestHeader, `version="1.0"`, `version="1.0" version="1.0"`, 1),
		strings.Replace(xmppTestHeader, `version="1.0"`, `version="2.0"`, 1),
		strings.TrimSuffix(xmppTestHeader, ">") + "/>",
		`<message xmlns="jabber:client" xmlns:a="urn:a" xmlns:b="urn:a" a:x="1" b:x="2"/>`,
		`<message xmlns="jabber:client" xmlns:xml="urn:wrong"/>`,
		`<message xmlns="jabber:client" bad:a="1"/>`,
		`<message xmlns="jabber:client"><body>&undefined;</body></message>`,
		`<message xmlns="jabber:client"><body>&#0;</body></message>`,
		`<message xmlns="jabber:client"><body>` + "\x01" + `</body></message>`,
		`<message xmlns="jabber:client"><body>x</body><!--x--></message>`,
		`<message xmlns="jabber:client"><body>x</subject></message>`,
		`<message xmlns="jabber:client"><body>x</body><body>y</body></message>`,
		`<message xmlns="jabber:client" type="unknown"/>`,
		`<message xmlns="jabber:client" type=""/>`,
		`<presence xmlns="jabber:client" type=""/>`,
		`<presence xmlns="jabber:client" type="available"/>`,
		`<message xmlns=""/>`,
		`<message/><message xmlns="jabber:client"/>`,
		`<message xmlns="jabber:client"/><message/>`,
		`<message/>` + xmppTestHeader,
		`<message xmlns="jabber:client"><c:body xmlns:c="jabber:client">x</c:body></message>`,
		`<presence xmlns="jabber:client"><show>invalid</show></presence>`,
		`<presence xmlns="jabber:client"><priority>128</priority></presence>`,
		`<presence xmlns="jabber:client"><priority>-129</priority></presence>`,
		`<iq xmlns="jabber:client" type="get" id="a"/>`,
		`<iq xmlns="jabber:client" type="result"/>`,
		`<iq xmlns="jabber:client" type="get" id="a"><x/><y/></iq>`,
		`<iq xmlns="jabber:server" type="result" id="a"/>`,
		`<iq xmlns="jabber:client" type="error" id="a"/>`,
		`<auth xmlns="urn:ietf:params:xml:ns:xmpp-sasl"/>`,
		`<auth xmlns="urn:ietf:params:xml:ns:xmpp-sasl" mechanism="plain"/>`,
		`<auth xmlns="urn:ietf:params:xml:ns:xmpp-sasl" mechanism="PL AIN"/>`,
		`<auth xmlns="urn:ietf:params:xml:ns:xmpp-sasl" mechanism="ABCDEFGHIJKLMNOPQRSTU"/>`,
		`<response xmlns="urn:ietf:params:xml:ns:xmpp-sasl">YR==</response>`,
		`<response xmlns="urn:ietf:params:xml:ns:xmpp-sasl">Y Q==</response>`,
		`<response xmlns="urn:ietf:params:xml:ns:xmpp-sasl"><x/></response>`,
		`<proceed xmlns="urn:ietf:params:xml:ns:xmpp-tls">x</proceed>`,
		xmppTestHeader + `</stream:stream><message/>`,
		xmppTestHeader + `</stream:stream>x`,
		xmppTestHeader + `</other:stream>`,
		"\ufeff" + xmppTestHeader, valid + "\xff",
	} {
		m, err := decodeXMPPText(input, "")
		require.Error(t, err, "%s", input)
		require.Nil(t, m)
	}
	for _, header := range []string{"bad", valid, xmppTestHeader + `<message/>`, xmppTestHeader + `</stream:stream>`, strings.Repeat(" ", (64<<10)+1)} {
		_, err := decodeXMPPText(valid, header)
		require.Error(t, err)
	}
}

func TestXMPPValueResourceAndConcurrentIsolation(t *testing.T) {
	for depth := 63; depth <= 64; depth++ {
		input := `<message xmlns="jabber:client"><x xmlns="urn:example">` + strings.Repeat("<n>", depth-1) + strings.Repeat("</n>", depth-1) + `</x></message>`
		_, err := decodeXMPPText(input, "")
		if depth == 63 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, count := range []int{4096, 4097} {
		_, err := decodeXMPPText(strings.Repeat(`<message xmlns="jabber:client"/>`, count)+" \n", "")
		if count == 4096 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	prefix, suffix := `<message xmlns="jabber:client"><body>`, `</body></message>`
	input := prefix + strings.Repeat("a", (1<<20)-len(prefix)-len(suffix)) + suffix
	_, err := decodeXMPPText(input, "")
	require.NoError(t, err)
	_, err = decodeXMPPText(input+" ", "")
	require.Error(t, err)
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			header := strings.Replace(xmppTestHeader, `xml:lang="en"`, fmt.Sprintf(`xml:lang="x-%d"`, i), 1)
			for j := 0; j < 8; j++ {
				_, err := decodeXMPPText(`<message><x></bad></message>`, header)
				require.Error(t, err)
				m, err := decodeXMPPText(`<message><body>ok</body></message>`, header)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprint("x-", i), m.Events[0].Stanza.Language)
			}
		})
	}
}
