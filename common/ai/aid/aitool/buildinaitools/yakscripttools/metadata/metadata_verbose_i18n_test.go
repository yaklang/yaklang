package metadata_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
)

func TestParseYakScriptMetadata_VerboseNameBilingual(t *testing.T) {
	code := `
__DESC__ = "demo tool"
__VERBOSE_NAME__ = "Text Grep Tool"
__VERBOSE_NAME_ZH__ = "文本查找工具"
__KEYWORDS__ = "grep,search"
`
	meta, err := metadata.ParseYakScriptMetadata("grep", code)
	require.NoError(t, err)
	require.Equal(t, "Text Grep Tool", meta.VerboseName)
	require.Equal(t, "文本查找工具", meta.VerboseNameZh)
	require.Equal(t, "demo tool", meta.Description)
}

func TestParseYakScriptMetadata_VerboseNameOnlyEnglish(t *testing.T) {
	code := `
__DESC__ = "demo"
__VERBOSE_NAME__ = "Unified File Reader"
__KEYWORDS__ = "read"
`
	meta, err := metadata.ParseYakScriptMetadata("read_file", code)
	require.NoError(t, err)
	require.Equal(t, "Unified File Reader", meta.VerboseName)
	require.Equal(t, "", meta.VerboseNameZh)
}
