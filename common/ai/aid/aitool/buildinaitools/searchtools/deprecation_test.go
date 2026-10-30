package searchtools

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestDeprecatedAIToolsHiddenFromSearchIndex(t *testing.T) {
	active := aitool.NewWithoutCallback("query_pcapdb", aitool.WithDescription("pcapdb analysis"))
	stale := []*aitool.Tool{active}
	for _, name := range schema.DeprecatedAIToolNames() {
		stale = append(stale, aitool.NewWithoutCallback(name))
	}
	searcher := func(string, []*aitool.Tool) ([]*aitool.Tool, error) { return stale, nil }
	getter := func() []*aitool.Tool { return []*aitool.Tool{active} }
	output := searchTools(&SearchCapabilitiesConfig{ToolSearcher: searcher, ToolsGetter: getter}, "pcap")
	require.Contains(t, output, "query_pcapdb")
	for _, name := range schema.DeprecatedAIToolNames() {
		require.NotContains(t, output, name)
	}
	tools, err := CreateAISearchTools(searcher, getter, SearchToolName, false)
	require.NoError(t, err)
	result, err := tools[0].Callback(context.Background(), aitool.InvokeParams{"query": "pcap"}, nil, io.Discard, io.Discard)
	require.NoError(t, err)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "query_pcapdb")
	for _, name := range schema.DeprecatedAIToolNames() {
		require.NotContains(t, string(encoded), name)
	}
}
