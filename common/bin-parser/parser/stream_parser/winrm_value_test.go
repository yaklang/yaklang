package stream_parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWinRMIndependentSyntaxAndSpans(t *testing.T) {
	wrap := func(body string) []byte {
		return []byte(fmt.Sprintf("POST /wsman HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/soap+xml\r\nContent-Length: %d\r\n\r\n%s", len(body), body))
	}
	body := `<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:i="http://schemas.dmtf.org/wbem/wsman/identify/1/wsmanidentity.xsd"><e:Body><i:Identify/></e:Body></e:Envelope>`
	for _, text := range []string{body, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + body + "\n", strings.ReplaceAll(body, "/identify/1/", "/identity/1/")} {
		wire := wrap(text)
		fields, info, err := decodeWinRMRecord(wire)
		require.NoError(t, err)
		require.Equal(t, "Identify", info["Kind"])
		var walk func([]winrmField, int) int
		walk = func(fields []winrmField, pos int) int {
			for _, f := range fields {
				require.Equal(t, pos, f.Start, f.Name)
				require.LessOrEqual(t, f.End, len(wire))
				if f.Type == "" {
					require.Equal(t, f.End, walk(f.Children, f.Start))
				}
				pos = f.End
			}
			return pos
		}
		require.Equal(t, len(wire), walk(fields, 0))
		for cut := 0; cut < len(wire); cut++ {
			_, _, e := decodeWinRMRecord(wire[:cut])
			require.Error(t, e, "prefix %d", cut)
		}
	}
	for _, v := range []string{"PT60.000S", "P0D", "P1Y2M3DT4H5M6.7S", "PT0S", "P1MT1M"} {
		require.True(t, winrmDuration(v), v)
	}
	for _, v := range []string{"P", "PT", "P1DT", "PT.1S", "P1.2D", "PT1M1H", "PT1S1S", "PT1SS", "P-1D", "P1DT1S2M"} {
		require.False(t, winrmDuration(v), v)
	}
	for _, extension := range []string{
		`<v:x xmlns:v="urn:v" ` + strings.Repeat(`a="x" `, 65) + `/>`,
		`<v:x xmlns:v="urn:v">` + strings.Repeat(`<v:a/>`, 8193) + `</v:x>`,
	} {
		bad := strings.Replace(body, "<i:Identify/>", "<i:Identify>"+extension+"</i:Identify>", 1)
		_, _, e := decodeWinRMRecord(wrap(bad))
		require.Error(t, e)
	}
}
