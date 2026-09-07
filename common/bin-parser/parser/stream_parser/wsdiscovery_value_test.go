package stream_parser

import (
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

const wsDiscoveryTestEPR = `<a:EndpointReference><a:Address>urn:uuid:example-17</a:Address></a:EndpointReference>`
const wsDiscoveryTestMetadata = `<d:Types>p:Printer p:Scanner</d:Types><d:Scopes MatchBy="http://schemas.xmlsoap.org/ws/2005/04/discovery/ldap">ldap:///ou=office urn:floor:7</d:Scopes><d:XAddrs>http://printer.example:8080/service soap.udp://192.0.2.7:3702</d:XAddrs><d:MetadataVersion>75965</d:MetadataVersion>`

func wsDiscoveryTestDocument(kind, body, extraHeaders string) string {
	header := `<a:Action>` + wsDiscoveryNamespace + `/` + kind + `</a:Action><a:MessageID>urn:uuid:message-23</a:MessageID><a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>`
	if kind == "Hello" || kind == "Bye" || strings.HasSuffix(kind, "Matches") {
		header += `<d:AppSequence InstanceId="1077004800" SequenceId="urn:sequence:4" MessageNumber="23"/>`
	}
	if strings.HasSuffix(kind, "Matches") {
		header += `<a:RelatesTo>urn:uuid:request-19</a:RelatesTo>`
	}
	return `<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="` + wsDiscoverySOAP12 + `" xmlns:a="` + wsDiscoveryAddressing + `" xmlns:d="` + wsDiscoveryNamespace + `" xmlns:p="urn:example:printing" xmlns:e="urn:example:extension"><s:Header>` + header + extraHeaders + `</s:Header><s:Body><d:` + kind + `>` + body + `</d:` + kind + `></s:Body></s:Envelope>`
}

func TestWSDiscoveryFieldsAndMessageKinds(t *testing.T) {
	for _, item := range []struct {
		kind, body string
		endpoints  int
	}{
		{"Hello", wsDiscoveryTestEPR + wsDiscoveryTestMetadata, 1},
		{"Bye", wsDiscoveryTestEPR, 1},
		{"Probe", `<d:Types>p:Printer</d:Types><d:Scopes>urn:floor:7</d:Scopes>`, 0},
		{"ProbeMatches", `<d:ProbeMatch>` + wsDiscoveryTestEPR + wsDiscoveryTestMetadata + `</d:ProbeMatch><d:ProbeMatch>` + wsDiscoveryTestEPR + `<d:MetadataVersion>0</d:MetadataVersion></d:ProbeMatch>`, 2},
		{"Resolve", wsDiscoveryTestEPR, 1},
		{"ResolveMatches", `<d:ResolveMatch>` + wsDiscoveryTestEPR + wsDiscoveryTestMetadata + `</d:ResolveMatch>`, 1},
	} {
		t.Run(item.kind, func(t *testing.T) {
			document := wsDiscoveryTestDocument(item.kind, item.body, `<a:ReplyTo><a:Address>http://example.test/reply</a:Address></a:ReplyTo>`)
			message, err := decodeWSDiscoveryText(document, 64)
			require.NoError(t, err)
			require.Equal(t, "2005/04", message.Version)
			require.Equal(t, wsDiscoveryAddressing, message.AddressingNamespace)
			require.Equal(t, wsDiscoverySOAP12, message.SOAPNamespace)
			require.Equal(t, item.kind, message.Kind)
			require.Equal(t, wsDiscoveryNamespace+"/"+item.kind, message.Header.Action)
			require.Equal(t, "urn:uuid:message-23", message.Header.MessageID)
			require.Equal(t, "urn:schemas-xmlsoap-org:ws:2005:04:discovery", message.Header.To)
			require.Equal(t, "http://example.test/reply", message.Header.ReplyTo.Address)
			require.Len(t, message.Endpoints, item.endpoints)
			if item.endpoints > 0 {
				require.Equal(t, "urn:uuid:example-17", message.Endpoints[0].EPR.Address)
			}
			if item.kind == "Hello" || strings.HasSuffix(item.kind, "Matches") {
				require.Equal(t, &WSDiscoverySequence{1077004800, 23, "urn:sequence:4"}, message.Header.AppSequence)
				endpoint := message.Endpoints[0]
				require.True(t, endpoint.TypesPresent)
				require.True(t, endpoint.XAddrsPresent)
				require.Equal(t, []xml.Name{{Space: "urn:example:printing", Local: "Printer"}, {Space: "urn:example:printing", Local: "Scanner"}}, endpoint.Types)
				require.True(t, endpoint.ScopesPresent)
				require.Equal(t, []string{"ldap:///ou=office", "urn:floor:7"}, endpoint.Scopes)
				require.Equal(t, wsDiscoveryNamespace+"/ldap", endpoint.MatchBy)
				require.Equal(t, []string{"http://printer.example:8080/service", "soap.udp://192.0.2.7:3702"}, endpoint.XAddrs)
				require.Equal(t, uint32(75965), *endpoint.MetadataVersion)
			}
			if item.kind == "Probe" {
				require.Equal(t, []xml.Name{{Space: "urn:example:printing", Local: "Printer"}}, message.Probe.Types)
				require.Equal(t, []string{"urn:floor:7"}, message.Probe.Scopes)
				require.Equal(t, wsDiscoveryNamespace+"/rfc2396", message.Probe.MatchBy)
			}
			if strings.HasSuffix(item.kind, "Matches") {
				require.Equal(t, []WSDiscoveryRelationship{{"urn:uuid:request-19", xml.Name{Space: wsDiscoveryAddressing, Local: "Reply"}}}, message.Header.RelatesTo)
			}
			soap11, err := decodeWSDiscoveryText(strings.ReplaceAll(document, wsDiscoverySOAP12, wsDiscoverySOAP11), 64)
			require.NoError(t, err)
			require.Equal(t, wsDiscoverySOAP11, soap11.SOAPNamespace)
			for cut := 0; cut < len(document); cut++ {
				_, err := decodeWSDiscoveryText(document[:cut], 64)
				require.Errorf(t, err, "accepted cut %d/%d", cut, len(document))
			}
		})
	}
	for _, kind := range []string{"Probe", "ProbeMatches", "ResolveMatches"} {
		message, err := decodeWSDiscoveryText(wsDiscoveryTestDocument(kind, "", ""), 64)
		require.NoError(t, err)
		require.Empty(t, message.Endpoints)
	}
}

func TestWSDiscoveryExtensionsAndNamespaceSemantics(t *testing.T) {
	epr := `<a:EndpointReference e:label="sample"><a:Address>urn:uuid:example-17</a:Address><a:ReferenceProperties><e:Queue>7</e:Queue></a:ReferenceProperties><a:ReferenceParameters><e:Region>east</e:Region></a:ReferenceParameters><a:PortType>p:Printer</a:PortType><a:ServiceName PortName="PrintPort">p:PrintService</a:ServiceName><e:Extra>original &amp; value</e:Extra></a:EndpointReference>`
	extension := `<e:Duration e:unit="seconds"><e:Value>10</e:Value></e:Duration>`
	header := `<a:RelatesTo RelationshipType="d:Suppression">urn:uuid:request-19</a:RelatesTo><a:From><a:Address>urn:sender:1</a:Address></a:From><a:FaultTo><a:Address>urn:reply:2</a:Address></a:FaultTo><e:Header>bounded</e:Header>`
	message, err := decodeWSDiscoveryText(wsDiscoveryTestDocument("Hello", epr+wsDiscoveryTestMetadata+extension, header), 64)
	require.NoError(t, err)
	require.Len(t, message.Extensions, 5)
	require.Equal(t, extension, message.Extensions[4].XML)
	require.Equal(t, "urn:example:extension", message.Extensions[4].Namespaces["e"])
	require.Len(t, message.ExtensionAttributes, 1)
	require.Equal(t, "urn:example:extension", message.ExtensionAttributes[0].Namespaces["e"])
	require.Equal(t, []xml.Attr{{Name: xml.Name{Space: "urn:example:extension", Local: "label"}, Value: "sample"}}, message.ExtensionAttributes[0].Attributes)
	require.Equal(t, &xml.Name{Space: "urn:example:printing", Local: "Printer"}, message.Endpoints[0].EPR.PortType)
	require.Equal(t, &xml.Name{Space: "urn:example:printing", Local: "PrintService"}, message.Endpoints[0].EPR.ServiceName)
	require.Equal(t, "PrintPort", message.Endpoints[0].EPR.PortName)
	require.Equal(t, "urn:sender:1", message.Header.From.Address)
	require.Equal(t, "urn:reply:2", message.Header.FaultTo.Address)
	require.Equal(t, xml.Name{Space: wsDiscoveryNamespace, Local: "Suppression"}, message.Header.RelatesTo[0].Type)
	// QName default namespaces and local prefix shadowing are resolved at the
	// lexical value's element, not guessed from conventional prefix names.
	probe := `<d:Types xmlns="urn:default:types" xmlns:p="urn:local:types">Printer p:Scanner</d:Types><d:Scopes/>`
	message, err = decodeWSDiscoveryText(wsDiscoveryTestDocument("Probe", probe, ""), 64)
	require.NoError(t, err)
	require.Equal(t, []xml.Name{{Space: "urn:default:types", Local: "Printer"}, {Space: "urn:local:types", Local: "Scanner"}}, message.Probe.Types)
	require.True(t, message.Probe.ScopesPresent)
	require.Empty(t, message.Probe.Scopes)
	message, err = decodeWSDiscoveryText("\ufeff"+wsDiscoveryTestDocument("Probe", `<d:Types>p:Prínter</d:Types><!-- sample -->`, "")+"\n", 64)
	require.NoError(t, err)
	require.Equal(t, "Prínter", message.Probe.Types[0].Local)
	defaultSOAP := strings.NewReplacer("xmlns:s=", "xmlns=", "<s:", "<", "</s:", "</").Replace(wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR, ""))
	message, err = decodeWSDiscoveryText(defaultSOAP, 64)
	require.NoError(t, err)
	require.Equal(t, wsDiscoverySOAP12, message.SOAPNamespace)
	match := `<d:ProbeMatch>` + wsDiscoveryTestEPR + `<d:MetadataVersion>-0</d:MetadataVersion><e:Extra/></d:ProbeMatch>`
	message, err = decodeWSDiscoveryText(wsDiscoveryTestDocument("ProbeMatches", match+match, ""), 64)
	require.NoError(t, err)
	require.Equal(t, uint32(0), *message.Endpoints[0].MetadataVersion)
	require.Len(t, message.Extensions, 2)
	require.NotEqual(t, message.Extensions[0].Path, message.Extensions[1].Path)
	for _, version := range []string{"+0", "000", "-000", "4294967295"} {
		_, err := decodeWSDiscoveryText(wsDiscoveryTestDocument("Hello", wsDiscoveryTestEPR+`<d:MetadataVersion>`+version+`</d:MetadataVersion>`, ""), 64)
		require.NoError(t, err)
	}
	message, err = decodeWSDiscoveryText(wsDiscoveryTestDocument("ResolveMatches", `<d:ResolveMatch>`+wsDiscoveryTestEPR+`<d:Types/><d:XAddrs/><d:MetadataVersion>0</d:MetadataVersion></d:ResolveMatch>`, ""), 64)
	require.NoError(t, err)
	require.True(t, message.Endpoints[0].TypesPresent)
	require.True(t, message.Endpoints[0].XAddrsPresent)
	require.False(t, message.Endpoints[0].ScopesPresent)
	require.Empty(t, message.Endpoints[0].Types)
	require.Empty(t, message.Endpoints[0].XAddrs)
}

func TestWSDiscoverySOAPDocumentInfoset(t *testing.T) {
	document := wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR, "")
	for _, wire := range []string{
		strings.Replace(document, `<s:Envelope`, `<!--before--><s:Envelope`, 1),
		document + `<!--after-->`,
		strings.Replace(document, `encoding="utf-8"`, `encoding="utf-8" standalone = 'no'`, 1),
		strings.Replace(document, `encoding="utf-8"`, `encoding="utf-8" standalone="no"`, 1),
	} {
		_, err := decodeWSDiscoveryText(wire, 64)
		require.ErrorContains(t, err, "SOAP 1.2 disallows")
		_, err = decodeWSDiscoveryText(strings.ReplaceAll(wire, wsDiscoverySOAP12, wsDiscoverySOAP11), 64)
		require.NoError(t, err, "SOAP 1.2 restrictions must not be invented for SOAP 1.1")
	}
	for _, wire := range []string{
		strings.Replace(document, `<s:Body>`, `<s:Body><!--inside-->`, 1),
		strings.Replace(document, `encoding="utf-8"`, `encoding="utf-8" standalone = "yes"`, 1),
	} {
		_, err := decodeWSDiscoveryText(wire, 64)
		require.NoError(t, err)
	}
}

func TestWSDiscoveryRejectsMalformedStructure(t *testing.T) {
	resolve := wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR, "")
	hello := wsDiscoveryTestDocument("Hello", wsDiscoveryTestEPR+wsDiscoveryTestMetadata, "")
	matches := wsDiscoveryTestDocument("ResolveMatches", `<d:ResolveMatch>`+wsDiscoveryTestEPR+wsDiscoveryTestMetadata+`</d:ResolveMatch>`, "")
	for name, document := range map[string]string{
		"empty":                        "",
		"encoding":                     resolve + "\xff",
		"nul":                          resolve + "\x00",
		"second-root":                  resolve + `<extra/>`,
		"outside-text":                 resolve + "other",
		"doctype":                      strings.Replace(resolve, `<s:Envelope`, `<!DOCTYPE s:Envelope SYSTEM "file:///not-read"><s:Envelope`, 1),
		"entity":                       strings.Replace(resolve, "urn:uuid:example-17", "&missing;", 1),
		"interior-pi":                  strings.Replace(resolve, `<s:Body>`, `<s:Body><?xml version="1.0"?>`, 1),
		"late-declaration":             " " + resolve,
		"duplicate-declaration":        `<?xml version="1.0"?>` + resolve,
		"wrong-declaration":            strings.Replace(resolve, `version="1.0"`, `version="1.1"`, 1),
		"foreign-pi":                   strings.Replace(resolve, `<?xml version="1.0" encoding="utf-8"?>`, `<?other x?>`, 1),
		"wrong-soap":                   strings.ReplaceAll(resolve, wsDiscoverySOAP12, "urn:wrong-soap"),
		"wrong-discovery":              strings.ReplaceAll(resolve, wsDiscoveryNamespace, "http://docs.oasis-open.org/ws-dd/ns/discovery/2009/01"),
		"wrong-addressing":             strings.ReplaceAll(resolve, wsDiscoveryAddressing, "http://www.w3.org/2005/08/addressing"),
		"unbound-prefix":               strings.ReplaceAll(resolve, "a:Address", "unbound:Address"),
		"unbound-extension":            strings.Replace(resolve, `</d:Resolve>`, `<unbound:Extension/></d:Resolve>`, 1),
		"duplicate-xmlns":              strings.Replace(resolve, `<s:Envelope `, `<s:Envelope xmlns:e="urn:duplicate" `, 1),
		"reserved-xmlns":               strings.Replace(resolve, `<s:Envelope `, `<s:Envelope xmlns:xml="urn:wrong" `, 1),
		"undeclare-prefix":             strings.Replace(resolve, `<s:Body>`, `<s:Body xmlns:a="">`, 1),
		"duplicate-attribute":          strings.Replace(resolve, `<d:Resolve>`, `<d:Resolve e:x="1" e:x="2">`, 1),
		"duplicate-expanded-attribute": strings.Replace(resolve, `<d:Resolve>`, `<d:Resolve xmlns:f="urn:example:extension" e:x="1" f:x="2">`, 1),
		"end-prefix":                   strings.Replace(resolve, `</a:Address>`, `</e:Address>`, 1),
		"missing-header":               strings.Replace(resolve, `<s:Header>`, `<s:NotHeader>`, 1),
		"duplicate-header":             strings.Replace(resolve, `<s:Body>`, `<s:Header/><s:Body>`, 1),
		"duplicate-body":               strings.Replace(resolve, `</s:Body>`, `</s:Body><s:Body/>`, 1),
		"two-messages":                 strings.Replace(resolve, `</s:Body>`, `<d:Resolve>`+wsDiscoveryTestEPR+`</d:Resolve></s:Body>`, 1),
		"missing-action":               strings.Replace(resolve, `<a:Action>`+wsDiscoveryNamespace+`/Resolve</a:Action>`, "", 1),
		"action-mismatch":              strings.Replace(resolve, `/Resolve</a:Action>`, `/Probe</a:Action>`, 1),
		"duplicate-id":                 strings.Replace(resolve, `</s:Header>`, `<a:MessageID>urn:duplicate</a:MessageID></s:Header>`, 1),
		"relative-id":                  strings.Replace(resolve, "urn:uuid:message-23", "relative-id", 1),
		"nested-leaf":                  strings.Replace(resolve, "urn:uuid:example-17", `<e:Value>urn:uuid:example-17</e:Value>`, 1),
		"mixed-content":                strings.Replace(resolve, `<s:Body>`, `<s:Body>text`, 1),
		"missing-epr":                  wsDiscoveryTestDocument("Resolve", "", ""),
		"missing-address":              wsDiscoveryTestDocument("Resolve", `<a:EndpointReference/>`, ""),
		"duplicate-epr":                wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR+wsDiscoveryTestEPR, ""),
		"duplicate-address":            strings.Replace(resolve, `</a:EndpointReference>`, `<a:Address>urn:other</a:Address></a:EndpointReference>`, 1),
		"types-in-resolve":             wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR+`<d:Types/>`, ""),
		"epr-in-probe":                 wsDiscoveryTestDocument("Probe", wsDiscoveryTestEPR, ""),
		"unknown-discovery":            wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR+`<d:Unknown/>`, ""),
		"unqualified-extension":        wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR+`<Unknown/>`, ""),
		"unknown-header":               wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR, `<a:Unknown/>`),
		"unknown-discovery-header":     wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR, `<d:Unknown/>`),
		"missing-sequence":             strings.Replace(hello, `<d:AppSequence InstanceId="1077004800" SequenceId="urn:sequence:4" MessageNumber="23"/>`, "", 1),
		"bad-sequence":                 strings.Replace(hello, `MessageNumber="23"`, `MessageNumber="4294967296"`, 1),
		"missing-sequence-number":      strings.Replace(hello, ` MessageNumber="23"`, "", 1),
		"missing-version":              strings.Replace(hello, `<d:MetadataVersion>75965</d:MetadataVersion>`, "", 1),
		"negative-version":             strings.Replace(hello, ">75965<", ">-1<", 1),
		"overflow-version":             strings.Replace(hello, ">75965<", ">4294967296<", 1),
		"simple-attribute":             strings.Replace(hello, `<d:Types>`, `<d:Types e:extra="1">`, 1),
		"unbound-qname":                strings.Replace(hello, "p:Printer", "x:Printer", 1),
		"bad-qname":                    strings.Replace(hello, "p:Printer", "p:1Printer", 1),
		"relative-scope":               strings.Replace(hello, "urn:floor:7", "floor7", 1),
		"bad-matchby":                  strings.Replace(hello, `MatchBy="`+wsDiscoveryNamespace+`/ldap"`, `MatchBy="relative"`, 1),
		"field-order":                  wsDiscoveryTestDocument("Hello", wsDiscoveryTestMetadata+wsDiscoveryTestEPR, ""),
		"extension-order":              wsDiscoveryTestDocument("Hello", wsDiscoveryTestEPR+`<e:Extra/>`+wsDiscoveryTestMetadata, ""),
		"duplicate-types":              strings.Replace(hello, `</d:Types>`, `</d:Types><d:Types/>`, 1),
		"missing-relatesto":            strings.Replace(matches, `<a:RelatesTo>urn:uuid:request-19</a:RelatesTo>`, "", 1),
		"wrong-relationship":           strings.Replace(matches, `<a:RelatesTo>`, `<a:RelatesTo RelationshipType="e:Other">`, 1),
		"missing-xaddrs":               strings.Replace(matches, `<d:XAddrs>http://printer.example:8080/service soap.udp://192.0.2.7:3702</d:XAddrs>`, "", 1),
		"duplicate-resolve-match":      strings.Replace(matches, `</d:ResolveMatches>`, `<d:ResolveMatch>`+wsDiscoveryTestEPR+wsDiscoveryTestMetadata+`</d:ResolveMatch></d:ResolveMatches>`, 1),
		"unknown-kind":                 wsDiscoveryTestDocument("Other", "", ""),
	} {
		t.Run(name, func(t *testing.T) { _, err := decodeWSDiscoveryText(document, 64); require.Error(t, err) })
	}
}

func TestWSDiscoveryLimitsAndConcurrency(t *testing.T) {
	document := wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR, "")
	for _, limit := range []int{0, 1, 4, 129} {
		_, err := decodeWSDiscoveryText(document, limit)
		require.Error(t, err)
	}
	_, err := decodeWSDiscoveryText(document, 5)
	require.NoError(t, err)
	for _, body := range []string{
		wsDiscoveryTestEPR + strings.Repeat(`<e:N>`, 64) + strings.Repeat(`</e:N>`, 64),
		wsDiscoveryTestEPR + strings.Repeat(`<e:N/>`, 8192),
	} {
		_, err := decodeWSDiscoveryText(wsDiscoveryTestDocument("Resolve", body, ""), 64)
		require.ErrorContains(t, err, "structure limit")
	}
	var attrs, namespaces strings.Builder
	for i := 0; i < 65; i++ {
		fmt.Fprintf(&attrs, ` e:a%d="value"`, i)
		fmt.Fprintf(&namespaces, ` xmlns:n%d="urn:n:%d"`, i, i)
	}
	for _, extra := range []string{attrs.String(), namespaces.String()} {
		_, err := decodeWSDiscoveryText(strings.Replace(document, `<d:Resolve>`, `<d:Resolve`+extra+`>`, 1), 64)
		require.Error(t, err)
	}
	// A near-boundary extension is processed only a few times, not once per
	// prefix, so ordinary regression runs stay fast.
	large := wsDiscoveryTestDocument("Resolve", wsDiscoveryTestEPR+`<e:Text></e:Text>`, "")
	large = strings.Replace(large, `</e:Text>`, strings.Repeat("x", (1<<20)-len(large))+`</e:Text>`, 1)
	require.Len(t, large, 1<<20)
	_, err = decodeWSDiscoveryText(large, 64)
	require.NoError(t, err)
	_, err = decodeWSDiscoveryText(large+" ", 64)
	require.ErrorContains(t, err, "text size")
	var wg sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for iteration := 0; iteration < 16; iteration++ {
				value := fmt.Sprintf("urn:uuid:worker-%d-%d", worker, iteration)
				message, err := decodeWSDiscoveryText(strings.Replace(document, "urn:uuid:example-17", value, 1), 64)
				if err != nil {
					t.Errorf("decode: %v", err)
					return
				}
				if message.Endpoints[0].EPR.Address != value {
					t.Errorf("cross-call state: %s", message.Endpoints[0].EPR.Address)
				}
			}
		}(worker)
	}
	wg.Wait()
}
