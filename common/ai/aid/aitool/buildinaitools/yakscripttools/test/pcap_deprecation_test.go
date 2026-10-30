package test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
)

func TestDeprecatedPCAPReadFileRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.pcap")
	require.NoError(t, os.WriteFile(path, pcapAnalysisCapture(t, "routing.example"), 0600))
	for _, name := range []string{"read_file", "read_file_lines"} {
		t.Run(name, func(t *testing.T) {
			source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/fs/" + name + ".yak")
			require.NoError(t, err)
			metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
			require.NotNil(t, metadata)
			tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
			require.Len(t, tools, 1)
			result, err := tools[0].ExecuteToolWithCapture(context.Background(), map[string]any{"file": path}, aitool.NewToolInvokeConfig())
			require.NoError(t, err)
			for _, expected := range []string{"pcapdb", "load_pcapfile", "pcapfile", "query_pcapdb", "dataset_id", "manage_pcapdb"} {
				require.Contains(t, result.Stdout, expected)
			}
			for _, retired := range schema.DeprecatedAIToolNames() {
				require.NotContains(t, result.Stdout, retired)
			}
		})
	}
}
