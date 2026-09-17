package pcaputil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	yaklang "github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

// Exercise the same source-comment extractor used to generate completion data.
// A valid Go comment alone does not guarantee a useful Yak signature/example.
func TestProtocolAPIDocumentation(t *testing.T) {
	for _, name := range []string{
		"pcap_onProtocolMessage", "pcap_onProtocolStats", "pcap_onTCPReassemblyStats",
		"pcap_protocolDeferred", "pcap_outputFile", "NewProtocolInspector",
		"ListDevices", "CaptureContext", "StartSniff", "OpenPcapFile",
	} {
		t.Run(name, func(t *testing.T) {
			decl, err := yakdoc.FuncToFuncDecl(Exports[name], "pcapx", name)
			require.NoError(t, err)
			require.Equal(t, name, decl.MethodName)
			require.Contains(t, decl.Decl, name+"(")
			require.Contains(t, decl.VSCodeSnippets, name+"(")
			require.Contains(t, decl.Document, "返回值:")
			require.NotContains(t, decl.Document, "pcap_protocolParser")
			embedded := doc.GetDocumentFunction("pcapx", name)
			require.NotNil(t, embedded, "refresh embedded completion data after changing exports")
			require.Equal(t, decl.Decl, embedded.Decl)
			require.Equal(t, decl.Document, embedded.Document)
			require.Equal(t, decl.VSCodeSnippets, embedded.VSCodeSnippets)
			if strings.HasPrefix(name, "pcap_on") {
				require.Len(t, decl.Params, 1)
				require.Equal(t, "callback", decl.Params[0].Name)
				require.Contains(t, decl.Params[0].Type, "func(")
			}
			_, example, ok := strings.Cut(decl.Document, "Example:")
			require.True(t, ok, "completion must include a runnable Yak example")
			parts := strings.Split(example, "```")
			require.Len(t, parts, 3)
			_, err = yaklang.New().Compile(strings.TrimSpace(parts[1]))
			require.NoError(t, err, "invalid Yak example: %s", parts[1])
		})
	}
}
