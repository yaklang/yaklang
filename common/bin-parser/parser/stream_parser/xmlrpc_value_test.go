package stream_parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func xmlrpcTestCall(values string) string {
	return `<methodCall><methodName>catalog.lookup</methodName><params>` + values + `</params></methodCall>`
}

func TestDecodeXMLRPCAllValueTypesAndResponses(t *testing.T) {
	values := []struct {
		body string
		want XMLRPCValue
	}{
		{"plain &amp; &#x4e2d;", XMLRPCValue{Kind: "string", Text: "plain & 中"}},
		{"", XMLRPCValue{Kind: "string"}},
		{"<string><![CDATA[<text>]]></string>", XMLRPCValue{Kind: "string", Text: "<text>"}},
		{"<int>-2147483648</int>", XMLRPCValue{Kind: "int", Text: "-2147483648", IntValue: -2147483648}},
		{"<i4>+002147483647</i4>", XMLRPCValue{Kind: "i4", Text: "+002147483647", IntValue: 2147483647}},
		{"<boolean>1</boolean>", XMLRPCValue{Kind: "boolean", Text: "1", BoolValue: true}},
		{"<boolean>0</boolean>", XMLRPCValue{Kind: "boolean", Text: "0"}},
		{"<double>-12.125</double>", XMLRPCValue{Kind: "double", Text: "-12.125", DoubleValue: -12.125}},
		{"<dateTime.iso8601>20240229T23:59:59</dateTime.iso8601>", XMLRPCValue{Kind: "dateTime.iso8601", Text: "20240229T23:59:59"}},
		{"<base64> AAEC\n/w==\t </base64>", XMLRPCValue{Kind: "base64", Text: " AAEC\n/w==\t ", Bytes: []byte{0, 1, 2, 255}}},
		{"<array><data/></array>", XMLRPCValue{Kind: "array"}},
		{"<struct/>", XMLRPCValue{Kind: "struct"}},
	}
	var params strings.Builder
	for _, fixture := range values {
		params.WriteString("<param><value>" + fixture.body + "</value></param>")
	}
	message, err := decodeXMLRPCText("\ufeff<?xml version=\"1.0\"?><!--intro-->"+xmlrpcTestCall(params.String()), 64)
	require.NoError(t, err)
	require.Equal(t, "request", message.Kind)
	require.Equal(t, "catalog.lookup", message.Method)
	require.Len(t, message.Params, len(values))
	for i, fixture := range values {
		fixture.want.Raw = "<value>" + fixture.body + "</value>"
		require.Equal(t, fixture.want, *message.Params[i], "value %d", i)
	}
	nested := `<value><array><data><value><struct><member><name>x</name><value><int>1</int></value></member><member><value>again</value><name>x</name></member></struct></value></data></array></value>`
	message, err = decodeXMLRPCText(`<methodResponse><params><param>`+nested+`</param></params></methodResponse>`, 64)
	require.NoError(t, err)
	require.Equal(t, "success", message.Kind)
	require.Len(t, message.Params, 1)
	value := message.Params[0]
	require.Equal(t, nested, value.Raw)
	require.Equal(t, "array", value.Kind)
	require.Len(t, value.Elements, 1)
	require.Equal(t, "struct", value.Elements[0].Kind)
	require.Equal(t, []XMLRPCMember{
		{Name: "x", Value: &XMLRPCValue{Kind: "int", Raw: `<value><int>1</int></value>`, Text: "1", IntValue: 1}},
		{Name: "x", Value: &XMLRPCValue{Kind: "string", Raw: `<value>again</value>`, Text: "again"}},
	}, value.Elements[0].Members)
	message, err = decodeXMLRPCText(`<methodResponse><fault><value><struct><member><name>faultString</name><value>not found</value></member><member><name>faultCode</name><value><i4>-4</i4></value></member></struct></value></fault></methodResponse>`, 64)
	require.NoError(t, err)
	require.Equal(t, "fault", message.Kind)
	require.Len(t, message.Fault.Members, 2)
	require.Equal(t, int32(-4), message.Fault.Members[1].Value.IntValue)
}

func TestDecodeXMLRPCRejectsInvalidGrammarAndBounds(t *testing.T) {
	for i, value := range []string{
		`<int>2147483648</int>`, `<i4>-2147483649</i4>`, `<int> 1</int>`, `<int>1_0</int>`,
		`<boolean>true</boolean>`, `<boolean>01</boolean>`, `<boolean/>`,
		`<double>NaN</double>`, `<double>Inf</double>`, `<double>1e3</double>`, `<double>0x1.0p2</double>`, `<double>.</double>`, `<double> 1.0</double>`,
		`<dateTime.iso8601>20230229T00:00:00</dateTime.iso8601>`, `<dateTime.iso8601>20240101T00:00:00Z</dateTime.iso8601>`,
		`<base64>AB==</base64>`, `<base64>AQ=</base64>`, `<nil/>`, `<i8>1</i8>`,
		`<array/>`, `<array><data/><data/></array>`, `<array><data>mixed</data></array>`,
		`<struct><member><name>x</name></member></struct>`, `<struct>mixed</struct>`,
		`<string><int>1</int></string>`, `<int>1</int><int>2</int>`, `mixed<int>1</int>`, `&custom;`,
	} {
		t.Run(fmt.Sprintf("value-%d", i), func(t *testing.T) {
			_, err := decodeXMLRPCText(xmlrpcTestCall("<param><value>"+value+"</value></param>"), 64)
			require.Error(t, err)
		})
	}
	valid := `<methodCall><methodName>read</methodName></methodCall>`
	for _, declaration := range []string{`<?xml version="1.0"?>`, `<?xml version='1.0' encoding="UTF-8" standalone='yes'?>`, "<?xml\nversion = '1.0'\r\nencoding = 'utf-8' standalone=\"no\" ?>"} {
		_, err := decodeXMLRPCText(declaration+valid, 64)
		require.NoError(t, err)
	}
	for i, document := range []string{
		``, `<methodCall/>`, `<methodCall><methodName/></methodCall>`, `<methodCall><methodName>bad name</methodName></methodCall>`,
		`<methodCall><methodName><string>x</string></methodName></methodCall>`, `<methodCall><params/><methodName>x</methodName></methodCall>`,
		`<methodResponse/>`, `<methodResponse><params/></methodResponse>`, `<methodResponse><params/><fault/></methodResponse>`,
		`<methodResponse><fault><value>not a struct</value></fault></methodResponse>`,
		`<methodResponse><fault><value><struct><member><name>faultCode</name><value><int>1</int></value></member><member><name>faultCode</name><value><int>2</int></value></member></struct></value></fault></methodResponse>`,
		valid + valid, valid + "tail", "prefix" + valid, " <?xml version=\"1.0\"?>" + valid,
		`<!DOCTYPE methodCall [<!ENTITY x "ignored">]>` + valid, `<?other instruction?>` + valid,
		`<methodCall attr="x"><methodName>x</methodName></methodCall>`, `<methodCall xmlns="x"><methodName>x</methodName></methodCall>`,
		`<q:methodCall><methodName>x</methodName></q:methodCall>`, `<methodCall><methodName>x</methodName></other>`,
		"\xff" + valid, `<?xml version="1.0" encoding="iso-8859-1"?>` + valid,
		`<?xml?>` + valid, `<?xml ?>` + valid, `<?xml encoding="UTF-8"?>` + valid,
		`<?xml version="1.0" version="1.0"?>` + valid, `<?xml version="1.0" unexpected="x"?>` + valid,
		`<?xml version="1.0" standalone="maybe"?>` + valid, `<?xml version="1&#46;0"?>` + valid,
		`<?xml version="1.0"encoding="UTF-8"?>` + valid,
	} {
		t.Run(fmt.Sprintf("document-%d", i), func(t *testing.T) {
			_, err := decodeXMLRPCText(document, 64)
			require.Error(t, err)
		})
	}
	for _, limit := range []int{-1, 0, 1, 129} {
		_, err := decodeXMLRPCText(valid, limit)
		require.Error(t, err)
	}
	_, err := decodeXMLRPCText(strings.Repeat(" ", 1<<20)+valid, 64)
	require.Error(t, err)
	for cut := 0; cut < len(valid); cut++ {
		_, err := decodeXMLRPCText(valid[:cut], 64)
		require.Error(t, err)
	}
}

func FuzzDecodeXMLRPCText(f *testing.F) {
	f.Add(`<methodCall><methodName>x</methodName></methodCall>`)
	f.Add(`<methodResponse><params><param><value><array><data/></array></value></param></params></methodResponse>`)
	f.Fuzz(func(t *testing.T, text string) {
		message, err := decodeXMLRPCText(text, 64)
		if err == nil {
			require.NotNil(t, message)
			require.Contains(t, []string{"request", "success", "fault"}, message.Kind)
		}
	})
}
