package syntaxflowdoc_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/syntaxflowdoc"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	_ "github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestBuildDocumentHelper_WithLiveNativeCalls(t *testing.T) {
	require.NotEmpty(t, ssaapi.NativeCallDocuments)

	items := make([]syntaxflowdoc.NativeCallInfo, 0, len(ssaapi.NativeCallDocuments))
	for name, doc := range ssaapi.NativeCallDocuments {
		if name == "" || doc == nil {
			continue
		}
		items = append(items, syntaxflowdoc.NativeCallInfo{Name: name, Description: doc.Description})
	}

	h := syntaxflowdoc.BuildDocumentHelper(syntaxflowdoc.BuildOptions{NativeCalls: items})
	require.Greater(t, len(h.NativeCalls), 20)
	require.NotNil(t, h.GetNativeCall("include"))
}
