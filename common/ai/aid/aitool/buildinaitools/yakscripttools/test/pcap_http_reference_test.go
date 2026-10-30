package test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/pcapx/pcapdb"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/testutil/enterprisefixtures"
)

// TestPCAPHTTPDGAFixtureUsesStructuredWorkflow is the end-to-end regression for
// the reported tshark detour. The public fixture must be answerable by the
// built-in load_pcapfile/query_pcapdb workflow, including request/response IDs.
func TestPCAPHTTPDGAFixtureUsesStructuredWorkflow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("YAKIT_HOME", home)
	consts.ResetYakitHomeOnce()
	profile, err := gorm.Open("sqlite3", filepath.Join(home, "profile.sqlite"))
	require.NoError(t, err)
	previous, previousPath := schema.GetGormProfileDatabase(), consts.GetCurrentProfileDatabasePath()
	require.NoError(t, pcapdb.ClosePCAPDatabases())
	consts.BindProfileDatabase(profile, "")
	t.Cleanup(func() {
		require.NoError(t, pcapdb.ClosePCAPDatabases())
		consts.BindProfileDatabase(previous, previousPath)
		profile.Close()
		consts.ResetYakitHomeOnce()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tools := make(map[string]*aitool.Tool)
	for _, name := range []string{"load_pcapfile", "query_pcapdb"} {
		source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/pcap/" + name + ".yak")
		require.NoError(t, err)
		metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
		require.NotNil(t, metadata)
		converted := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
		require.Len(t, converted, 1)
		tools[name] = converted[0]
	}
	invoke := func(name string, params map[string]any) any {
		t.Helper()
		result, err := tools[name].ExecuteToolWithCapture(ctx, params, aitool.NewToolInvokeConfig())
		require.NoError(t, err)
		require.NotNil(t, result.Result)
		return result.Result
	}
	capture := enterprisefixtures.Path(t, "http-dga-c2/capture.pcapng")
	page := invoke("query_pcapdb", map[string]any{
		"pcapfile": capture, "query_type": "protocols", "protocol": "http", "limit": 20, "result_bytes": 65536,
	}).(*pcapdb.ResultPage[pcapdb.ProtocolSummary])
	require.Nil(t, page.Error)
	require.Len(t, page.Items, 8)
	require.False(t, page.HasMore)
	byID := make(map[uint]pcapdb.ProtocolSummary, len(page.Items))
	for _, item := range page.Items {
		byID[item.ID] = item
	}
	for _, id := range []uint{1, 2, 3, 4, 5, 6, 7, 8} {
		require.Contains(t, byID, id)
	}
	require.EqualValues(t, byID[1].ID, byID[2].ResponseTo)
	require.EqualValues(t, byID[3].ID, byID[4].ResponseTo)
	require.EqualValues(t, byID[5].ID, byID[6].ResponseTo)
	require.EqualValues(t, byID[7].ID, byID[8].ResponseTo)
	for _, id := range []uint{1, 3, 5, 7} {
		details := invoke("query_pcapdb", map[string]any{
			"id": page.DatasetID, "query_type": "details", "record_id": id, "result_bytes": 65536,
		}).(*pcapdb.ResultPage[pcapdb.ProtocolDetail])
		require.Nil(t, details.Error)
		require.Len(t, details.Items, 1)
		encoded, err := json.Marshal(details.Items[0].Fields)
		require.NoError(t, err)
		require.Contains(t, string(encoded), "db54a845.top")
	}
	bodyPage := invoke("query_pcapdb", map[string]any{
		"id": page.DatasetID, "query_type": "body_summary", "record_id": 7, "result_bytes": 65536,
	}).(*pcapdb.ResultPage[pcapdb.HTTPBodyProfile])
	require.Nil(t, bodyPage.Error)
	require.Len(t, bodyPage.Items, 1)
	body := bodyPage.Items[0]
	require.Equal(t, "POST", body.Method)
	require.Equal(t, "application/x-www-form-urlencoded", body.ContentType)
	require.NotNil(t, body.DeclaredBodyBytes)
	require.EqualValues(t, 4464, *body.DeclaredBodyBytes)
	require.Equal(t, 4464, body.WireBodyBytes)
	require.Equal(t, 4464, body.DecodedBodyBytes)
	require.True(t, body.TransferComplete)
	require.True(t, body.DecodedComplete)
	require.Equal(t, "eb563e11423ebbc793348c40a7bd6204682ef023912118be47dc4411b9f8ceb6", body.MessageSHA256)
	require.Equal(t, "3fa31e8d261fafbf21a297c2492c852fce2b9de6879a6965f204b99451d17179", body.WireBodySHA256)
	require.Equal(t, body.WireBodySHA256, body.DecodedBodySHA256)
	require.True(t, body.EncodedForm)
	require.True(t, body.PercentEncoded)
	require.False(t, body.Compressed)
	require.False(t, body.Binary)
	require.True(t, body.TextLike)
	require.True(t, body.Sensitive)
	require.Contains(t, body.SensitiveIndicators, "user-identifier")
	require.Contains(t, body.SensitiveIndicators, "encoded-payload")
	encoded, err := json.Marshal(bodyPage)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "IEUser")
	require.NotContains(t, string(encoded), "IAAAAE")
}
