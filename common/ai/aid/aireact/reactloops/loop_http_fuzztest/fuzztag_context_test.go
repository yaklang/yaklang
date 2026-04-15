package loop_http_fuzztest

import (
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestGetLoopHTTPFuzzFuzztagReference_LoadsSourceDocument(t *testing.T) {
	ref := getLoopHTTPFuzzFuzztagReference()
	require.NotEmpty(t, ref)
	require.Contains(t, ref, "This is the current built-in fuzztag reference.")
	require.Contains(t, ref, "## fuzztag 可用标签一览")
	require.Contains(t, ref, "`fuzz:password`")
	require.Contains(t, ref, "{{payload(pass_top25)}}")
}

func TestBuildLoopHTTPFuzzPayloadGroupsReference_UsesCurrentDBGroups(t *testing.T) {
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, db.AutoMigrate(&schema.Payload{}).Error)
	require.NoError(t, yakit.SavePayloadGroup(db, "pass_top25", []string{"admin", "root"}))
	require.NoError(t, yakit.SavePayloadGroup(db, "usernames_default", []string{"admin", "guest"}))

	ref := buildLoopHTTPFuzzPayloadGroupsReference(db)
	require.Contains(t, ref, "{{payload(group)}}")
	require.Contains(t, ref, "{{payload:nodup(group)}}")
	require.Contains(t, ref, "{{payload:full(group)}}")
	require.Contains(t, ref, "pass_top25")
	require.Contains(t, ref, "usernames_default")
	require.Contains(t, ref, "Current database payload groups (2):")
}

func TestLoopHTTPFuzzReactiveData_RendersFuzztagAndPayloadGroupContext(t *testing.T) {
	rendered, err := utils.RenderTemplate(reactiveData, map[string]any{
		"Nonce":                  "n123",
		"OriginalRequest":        "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
		"FuzztagReference":       "Source: common/mutate/fuzztag.md\n{{fuzz:password}}\n{{payload(pass_top25)}}",
		"PayloadGroupsReference": "Current database payload groups (1):\n- pass_top25",
	})
	require.NoError(t, err)
	require.Contains(t, rendered, "<|FUZZTAG_REFERENCE_n123|>")
	require.Contains(t, rendered, "{{fuzz:password}}")
	require.Contains(t, rendered, "{{payload(pass_top25)}}")
	require.Contains(t, rendered, "<|AVAILABLE_PAYLOAD_GROUPS_n123|>")
	require.Contains(t, rendered, "pass_top25")
}
