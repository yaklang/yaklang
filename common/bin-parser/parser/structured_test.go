package parser

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestStructuredRuleEligibility(t *testing.T) {
	const source = "endian: big\nunit: byte\nPackage:\n  Message:\n    operator: |\n      err = parseMemcachedFields(\"stats-request\")\n      if err != nil { panic(err) }\n"
	for _, text := range []string{source, strings.ReplaceAll(source, "\n", "\r\n")} {
		var document yaml.MapSlice
		require.NoError(t, yaml.Unmarshal([]byte(text), &document))
		decoders := compileStructuredEntries(document)
		require.NotNil(t, decoders["Message"])
		_, _, err := decoders["Message"]([]byte("stats\r\n"))
		require.NoError(t, err)
	}
	for name, text := range map[string]string{
		"root operator":   "operator: panic(1)\n" + source,
		"custom parser":   "parser: custom\n" + source,
		"root length":     "length: 1\n" + source,
		"invalid order":   strings.Replace(source, "endian: big", "endian: invalid", 1),
		"missing unit":    strings.Replace(source, "unit: byte\n", "", 1),
		"duplicate root":  "unit: byte\n" + source,
		"package logic":   strings.Replace(source, "Package:\n", "Package:\n  operator: panic(1)\n", 1),
		"entry output":    source + "    out: 1\n",
		"entry length":    source + "    length: 1\n",
		"entry child":     source + "    Extra: uint8\n",
		"duplicate entry": source + "  Message: uint8\n",
		"changed program": source + "      panic(1)\n",
		"unknown profile": strings.Replace(source, "stats-request", "unknown", 1),
	} {
		t.Run(name, func(t *testing.T) {
			var document yaml.MapSlice
			require.NoError(t, yaml.Unmarshal([]byte(text), &document))
			require.Nil(t, compileStructuredEntries(document)["Message"])
		})
	}
}

type structuredCountingParser struct {
	stream_parser.DefParser
	calls int
}

func (p *structuredCountingParser) OnRoot(node *base.Node) error {
	p.calls++
	return p.DefParser.OnRoot(node)
}

func TestStructuredCustomParserRegistration(t *testing.T) {
	original := base.ParserRegistration("default")
	defer base.RegisterParser("default", original)
	custom := &structuredCountingParser{}
	base.RegisterParser("default", custom)
	result, err := ParseStructured([]byte("stats\r\n"), "application-layer.memcached_fields", "MemcachedStatsRequestFields")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Positive(t, custom.calls, "registered parsers must not be bypassed")
}

func TestStructuredFastEntryCoverage(t *testing.T) {
	for _, tc := range []struct {
		path string
		want int
	}{
		{"application-layer/memcached_fields.yaml", 3},
		{"application-layer/cassandra_fields.yaml", 7},
	} {
		root, err := base.ParseRule(tc.path)
		require.NoError(t, err)
		entries := compileStructuredEntries(root.Origin.(yaml.MapSlice))
		require.Len(t, entries, tc.want)
		for name := range entries {
			require.NotNil(t, structuredDecoder(tc.path, name))
			require.Nil(t, structuredDecoder(tc.path, name+"Carrier"))
		}
	}
	require.Nil(t, structuredDecoder("missing.yaml", "missing"))
}

func TestStructuredPlanRegistrationAndBounds(t *testing.T) {
	plan, err := PrepareStructured("application-layer.memcached_fields", "MemcachedStatsRequestFields")
	require.NoError(t, err)
	result, err := plan.Parse([]byte("stats\r\n"))
	require.NoError(t, err)
	require.NotNil(t, result)
	result, err = plan.Parse([]byte("bad\r\n"))
	require.Error(t, err)
	require.Nil(t, result)
	require.NotContains(t, err.Error(), "YakVM", "hot rejection must not replay in the VM")
	_, err = PrepareStructured("missing", "Missing")
	require.Error(t, err)
	_, err = PrepareStructured("application-layer.memcached_fields", "MemcachedStatsRequestFieldsCarrier")
	require.Error(t, err, "carrier programs retain their trial and recovery semantics")
	var zero StructuredPlan
	_, err = zero.Parse(nil)
	require.Error(t, err)
	var absent *StructuredPlan
	_, err = absent.Parse(nil)
	require.Error(t, err)

	// A plan acquired before a registration change must not bypass the new
	// parser. Neither preparation nor a flow cache locks in the old factory.
	original := base.ParserRegistration("default")
	defer base.RegisterParser("default", original)
	custom := &structuredCountingParser{}
	base.RegisterParser("default", custom)
	result, err = plan.Parse([]byte("stats\r\n"))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Positive(t, custom.calls)
}

func TestStructuredEmbeddedInventory(t *testing.T) {
	files, entries := 0, 0
	for name := range structuredRules() {
		_ = structuredDecoder(name, "")
		count := len(structuredRules()[name].decoders)
		if count > 0 {
			files++
			entries += count
			t.Logf("%s: %d", name, count)
		}
	}
	t.Logf("native structured coverage: %d files, %d exact entries", files, entries)
	require.GreaterOrEqual(t, files, 13)
	before := len(structuredRules())
	for i := 0; i < 1000; i++ {
		require.Nil(t, structuredDecoder(strings.Repeat("x", i), "Entry"))
	}
	require.Len(t, structuredRules(), before, "unknown names must not occupy permanent cache slots")
}
