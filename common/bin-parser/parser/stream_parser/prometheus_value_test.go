package stream_parser

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodePrometheusTextEveryField(t *testing.T) {
	wire := "\t# display note\n\n# HELP room_value Room\\nvalue\\\\raw\n# TYPE room_value gauge\n" +
		`room_value {zone="a\"b",path="C:\\x",empty="",} -2.5e1 -9223372036854775808` + "\n" +
		`{"室温.值", "区域"="一楼"} 1.25 +9223372036854775807` + "\n" +
		"without_type 0.2\npositive +Inf\nnegative -Inf\nundefined NaN\n"
	lines, err := decodePrometheusText(wire, 65536)
	require.NoError(t, err)
	require.Len(t, lines, 10)
	for i, text := range strings.Split(strings.TrimSuffix(wire, "\n"), "\n") {
		require.Equal(t, text, lines[i].Text)
	}
	require.Equal(t, "comment", lines[0].Kind)
	require.Equal(t, "blank", lines[1].Kind)
	require.Equal(t, "Room\nvalue\\raw", lines[2].Help)
	require.Equal(t, "gauge", lines[3].MetricType)
	require.Equal(t, PrometheusLine{
		Text: lines[4].Text, Kind: "sample", Name: "room_value",
		Labels:    []PrometheusLabel{{Name: "zone", Value: `a"b`}, {Name: "path", Value: `C:\x`}, {Name: "empty"}},
		ValueText: "-2.5e1", Value: -25, HasTimestamp: true, Timestamp: math.MinInt64, TimestampText: "-9223372036854775808",
	}, *lines[4])
	require.Equal(t, PrometheusLine{
		Text: lines[5].Text, Kind: "sample", Name: "室温.值", Labels: []PrometheusLabel{{Name: "区域", Value: "一楼"}},
		ValueText: "1.25", Value: 1.25, HasTimestamp: true, Timestamp: math.MaxInt64, TimestampText: "+9223372036854775807",
	}, *lines[5])
	require.False(t, lines[6].HasTimestamp)
	require.True(t, math.IsInf(lines[7].Value, 1))
	require.True(t, math.IsInf(lines[8].Value, -1))
	require.True(t, math.IsNaN(lines[9].Value))
	for _, kind := range []string{"counter", "gauge", "histogram", "summary", "untyped"} {
		_, err := decodePrometheusText("# TYPE x "+kind+"\nx 1\n", 65536)
		require.NoError(t, err)
	}
	for _, text := range []string{"", "\n", "# HELP x\n", "a{}1\n", "{\"a\",} 1\n", "# HELP \"a.b\" doc\n# TYPE \"a.b\" gauge\n{\"a.b\"} 1\n"} {
		_, err := decodePrometheusText(text, 65536)
		require.NoError(t, err, text)
	}
}

func TestDecodePrometheusTextRejectsAmbiguousOrIncompleteFields(t *testing.T) {
	for i, text := range []string{
		"a 1", "a 1\r\n", "a\n", "1a 1\n", "a.b 1\n", "a 1x\n", "a 1e400\n", "a 1 1.5\n", "a 1 9223372036854775808\n", "a 1 0 extra\n",
		"# TYPE\n", "# TYPE a\n", "# TYPE a other\n", "# TYPE a gauge extra\n", "# HELP\n", "# HELP a bad\\t\n", "# HELP a bad\\\n",
		"a 1\n# TYPE a gauge\n", "# TYPE a gauge\n# TYPE a gauge\n", "# HELP a first\n# HELP a second\n", "a 1\n# HELP a late\n",
		"a 1\na 2\n", `a{x="1",y="2"} 1` + "\n" + `a{y="2",x="1"} 2` + "\n",
		`a{x="1",x="2"} 1` + "\n", `a{__name__="b"} 1` + "\n", `a{"x"="1",x="2"} 1` + "\n",
		`a{x="bad\t"} 1` + "\n", `a{x="cut} 1` + "\n", `a{x="1" y="2"} 1` + "\n", `a{x="1",,} 1` + "\n",
		`a{x:="1"} 1` + "\n", `a{="1"} 1` + "\n", `a{x=1} 1` + "\n", `a{x="1"` + "\n",
		`{"",x="1"} 1` + "\n", `{"a" "x"="1"} 1` + "\n", `a{x="1"} 1 # exemplar` + "\n", "\xff 1\n",
	} {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			_, err := decodePrometheusText(text, 65536)
			require.Error(t, err, text)
		})
	}
	for _, limit := range []int{0, -1, 3, 1<<20 + 1} {
		_, err := decodePrometheusText("a 1\n", limit)
		require.Error(t, err)
	}
	_, err := decodePrometheusText(strings.Repeat(" ", 1<<20)+"\n", 1<<20)
	require.Error(t, err)
}

func FuzzDecodePrometheusText(f *testing.F) {
	f.Add("# TYPE cpu_usage gauge\ncpu_usage 0.2\n")
	f.Add("m{x=\"a\\nb\"} +Inf -5\n")
	f.Fuzz(func(t *testing.T, text string) {
		lines, err := decodePrometheusText(text, 65536)
		if err == nil {
			var recovered strings.Builder
			for _, line := range lines {
				recovered.WriteString(line.Text)
				recovered.WriteByte('\n')
			}
			require.Equal(t, text, recovered.String())
		}
	})
}
