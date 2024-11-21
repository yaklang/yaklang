package yaklib

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/yakxml/xml-tools"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

func TestXMLloadsAndDumps(t *testing.T) {
	s := `
<a>
	<b>
		<c>
			1
		</c>
		<d>
			2
		</d>
	</b>
</a>
`
	v := xml_tools.XmlLoads(s)
	spew.Dump(v)
	fmt.Println(string(xml_tools.XmlDumps(v)))

	require.Equal(t, v, map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": "1",
				"d": "2",
			},
		},
	})
}

func TestXMLloadsEncoding(t *testing.T) {
	t.Run("utf-8", func(t *testing.T) {
		s := `<?xml version="1.0" encoding="utf-8"?><note><to>George</to><from>John</from><heading>Reminder</heading><body>Don't forget the meeting!</body></note>`
		v := xml_tools.XmlLoads(s)
		spew.Dump(v)
		fmt.Println(string(xml_tools.XmlDumps(v)))

		require.Equal(t, v, map[string]any{
			"note": map[string]any{
				"to":      "George",
				"from":    "John",
				"heading": "Reminder",
				"body":    "Don't forget the meeting!",
			},
		})
	})
	t.Run("ISO-8859-1", func(t *testing.T) {
		s := fmt.Sprintf(`<?xml version="1.0" encoding="ISO-8859-1"?><note>%s</note>`, string([]byte{0xc4, 0xd6, 0xdc, 0xe4, 0xf6, 0xfc, 0xdf}))
		v := xml_tools.XmlLoads(s)
		spew.Dump(v)
		fmt.Println(string(xml_tools.XmlDumps(v)))

		require.Equal(t, v, map[string]any{
			"note": "ÄÖÜäöüß",
		})
	})
}

func TestXMLDumpEscape(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		s := `<a><b>'</b></a>`
		v := xml_tools.XmlLoads(s)
		spew.Dump(v)
		res := xml_tools.XmlDumps(v)
		spew.Dump(res)
		require.Contains(t, string(res), `&#39;`)
	})

	t.Run("ISO-8859-1", func(t *testing.T) {
		s := `<a><b>'</b></a>`
		v := xml_tools.XmlLoads(s)
		spew.Dump(v)
		res := xml_tools.XmlDumps(v, xml_tools.WithHTMLEscape(false))
		fmt.Println(string(res))
		require.NotContains(t, string(res), `&#39;`)
	})
}
