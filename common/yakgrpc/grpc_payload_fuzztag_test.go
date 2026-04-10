package yakgrpc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func createQuotedPayloadFileGroup(t *testing.T, group string, lines ...string) {
	t.Helper()

	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	filePath := filepath.Join(t.TempDir(), "payload.txt")
	var builder strings.Builder
	for i, line := range lines {
		builder.WriteString(strconv.Quote(line))
		if i < len(lines)-1 {
			builder.WriteByte('\n')
		}
	}

	require.NoError(t, os.WriteFile(filePath, []byte(builder.String()), 0o600))
	require.NoError(t, yakit.CreatePayload(db, filePath, group, "", 0, true))
	t.Cleanup(func() {
		require.NoError(t, yakit.DeletePayloadByGroup(db, group))
	})
}

func createRawPayloadGroup(t *testing.T, group, content string) {
	t.Helper()

	db := consts.GetGormProfileDatabase()
	require.NotNil(t, db)

	require.NoError(t, yakit.CreatePayload(db, strconv.Quote(content), group, "", 0, false))
	t.Cleanup(func() {
		require.NoError(t, yakit.DeletePayloadByGroup(db, group))
	})
}

func TestPayloadTag_LegacyPayloadStillDedupsLines(t *testing.T) {
	group := "payload-dedup-" + uuid.NewString()
	createQuotedPayloadFileGroup(t, group, "alpha", "alpha", "beta")

	results, err := mutate.FuzzTagExec(fmt.Sprintf("{{payload(%s)}}", group))
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "beta"}, results)
}

func TestPayloadTag_NoDupPreservesDuplicateLines(t *testing.T) {
	group := "payload-nodup-" + uuid.NewString()
	createQuotedPayloadFileGroup(t, group, "alpha", "alpha", "beta")

	results, err := mutate.FuzzTagExec(fmt.Sprintf("{{payload:nodup(%s)}}", group))
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "alpha", "beta"}, results)
}

func TestPayloadTag_FullReturnsWholePayloadGroup(t *testing.T) {
	group := "payload-full-file-" + uuid.NewString()
	createQuotedPayloadFileGroup(t, group, "alpha", "", "alpha")

	results, err := mutate.FuzzTagExec(fmt.Sprintf("{{payload:full(%s)}}", group))
	require.NoError(t, err)
	require.Equal(t, []string{"alpha\n\nalpha"}, results)
}

func TestPayloadTag_FullPreservesNonFileTrailingNewline(t *testing.T) {
	group := "payload-full-raw-" + uuid.NewString()
	content := "alpha\r\nbeta\r\n"
	createRawPayloadGroup(t, group, content)

	results, err := mutate.FuzzTagExec(fmt.Sprintf("{{payload:full(%s)}}", group))
	require.NoError(t, err)
	require.Equal(t, []string{content}, results)

	legacyResults, err := mutate.FuzzTagExec(fmt.Sprintf("{{payload(%s)}}", group))
	require.NoError(t, err)
	require.Equal(t, []string{"alpha\r\nbeta"}, legacyResults)
}

func TestPayloadTag_NoDupSupportsSyncLabels(t *testing.T) {
	groupLeft := "payload-sync-left-" + uuid.NewString()
	groupRight := "payload-sync-right-" + uuid.NewString()
	createQuotedPayloadFileGroup(t, groupLeft, "alpha", "beta")
	createQuotedPayloadFileGroup(t, groupRight, "one", "two")

	results, err := mutate.FuzzTagExec(
		fmt.Sprintf("{{payload:nodup::1(%s)}}---{{payload:nodup::1(%s)}}", groupLeft, groupRight),
		mutate.Fuzz_SyncTag(true),
	)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha---one", "beta---two"}, results)
}
