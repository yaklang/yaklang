package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/aid/liteforge/liteforgeapp"
	"github.com/yaklang/yaklang/common/ai/aiknowledge"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Exercise the embedded plugin over the actual gRPC streaming transport, through
// the Yak VM, media routing, question indexing and vector persistence.
func TestGRPCMUSTPASS_KnowledgeBaseOmniPlugin(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	yakit.GuardConfigurationForTest(t)
	oldMock := vectorstore.IsMockMode
	vectorstore.IsMockMode = true
	t.Cleanup(func() { vectorstore.IsMockMode = oldMock })
	var probes, media, indexes atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		messages, _ := json.Marshal(body["messages"])
		answer := ""
		switch {
		case bytes.Contains(messages, []byte("OMNI_READY")):
			probes.Add(1)
			answer = "OMNI_READY"
		case bytes.Contains(messages, []byte("image_url")):
			media.Add(1)
			select {
			case <-time.After(5200 * time.Millisecond):
			case <-req.Context().Done():
				return
			}
			answer = "# Redhaze recovery\nThe Redhaze recovery code is ORBIT-731.\nUse the recovery code to restore the training system."
		default:
			indexes.Add(1)
			answer = `{"@action":"object","chunk_list":[{"title":"Redhaze recovery","answer_location":{"start_line":1,"end_line":100},"question_list":["What is the Redhaze recovery code?","How is the Redhaze training system restored?"]}]}`
		}
		w.Header().Set("Content-Type", "text/event-stream")
		payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": answer}}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
	}))
	t.Cleanup(upstream.Close)
	model := &ypb.AIModelConfig{Provider: &ypb.ThirdPartyApplicationConfig{Type: "aibalance", APIKey: "fixture-key", BaseURL: upstream.URL}, ModelName: liteforgeapp.DefaultKnowledgeOmniModel}
	cfg := &ypb.AIGlobalConfig{Enabled: true, RoutingPolicy: "balance", IntelligentModels: []*ypb.AIModelConfig{model}, LightweightModels: []*ypb.AIModelConfig{model}, VisionModels: []*ypb.AIModelConfig{model}}
	_, err = yakit.SetAIGlobalConfig(consts.GetGormProfileDatabase(), cfg)
	require.NoError(t, err)
	require.NoError(t, yakit.ApplyAIGlobalConfig(consts.GetGormProfileDatabase(), cfg))
	var pngData bytes.Buffer
	require.NoError(t, png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 64, 64))))
	path := filepath.Join(t.TempDir(), "recovery.png")
	require.NoError(t, os.WriteFile(path, pngData.Bytes(), 0600))
	kbName := "omni-grpc-" + utils.RandStringBytes(10)
	code := coreplugin.GetCorePluginDataWithHook("构建知识库")
	require.NotEmpty(t, code)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{Code: string(code), PluginType: "yak", ExecParams: []*ypb.KVPair{
		{Key: "files", Value: path}, {Key: "kbName", Value: kbName}, {Key: "disableERM", Value: "true"}, {Key: "omni-mode", Value: "required"},
	}})
	require.NoError(t, err)
	var output strings.Builder
	for {
		result, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if result.GetIsMessage() {
			output.Write(result.GetMessage())
		}
	}
	require.NotContains(t, output.String(), "YakVM Panic")
	require.Contains(t, output.String(), "waiting elapsed=", "gRPC must stream progress before a slow media request completes")
	require.EqualValues(t, 1, probes.Load(), output.String())
	require.EqualValues(t, 1, media.Load(), output.String())
	require.GreaterOrEqual(t, indexes.Load(), int32(1), output.String())
	var info schema.KnowledgeBaseInfo
	db := consts.GetGormProfileDatabase()
	require.NoError(t, db.Where("knowledge_base_name = ?", kbName).First(&info).Error, output.String())
	response, err := client.SearchKnowledgeBaseEntry(ctx, &ypb.SearchKnowledgeBaseEntryRequest{
		Filter: &ypb.SearchKnowledgeBaseEntryFilter{KnowledgeBaseId: int64(info.ID), Keyword: "ORBIT-731"}, Pagination: &ypb.Paging{Page: 1, Limit: 10},
	})
	require.NoError(t, err)
	require.Greater(t, response.GetTotal(), int64(0), output.String())
	require.Contains(t, response.GetKnowledgeBaseEntries()[0].GetKnowledgeDetails(), "ORBIT-731")
	var vectors int64
	require.NoError(t, db.Model(&schema.VectorStoreDocument{}).Where("collection_id IN (?)", db.Model(&schema.VectorStoreCollection{}).Select("id").Where("name = ?", kbName).SubQuery()).Count(&vectors).Error)
	require.Greater(t, vectors, int64(0), "the plugin must persist vectors before reporting entries")

	// Run the actual native management tool over gRPC as well. Its progress
	// must reach captured stdout, where asynchronous AI inspect/wait can see it.
	toolName := kbName + "-managed"
	_, err = aiknowledge.Add(toolName)
	require.NoError(t, err)
	toolCode, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/knowledge/manage_knowledge_base.yak")
	require.NoError(t, err)
	stream, err = client.DebugPlugin(ctx, &ypb.DebugPluginRequest{Code: string(toolCode), PluginType: "yak", ExecParams: []*ypb.KVPair{
		{Key: "op", Value: "build"}, {Key: "name", Value: toolName}, {Key: "source_file", Value: path}, {Key: "build_options", Value: `{"mode":"chunks","omni_mode":"required"}`},
	}})
	require.NoError(t, err)
	output.Reset()
	for {
		result, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		require.NoError(t, recvErr)
		if result.GetIsMessage() {
			output.Write(result.GetMessage())
		}
	}
	require.NotContains(t, output.String(), "YakVM Panic")
	require.Contains(t, output.String(), "persisted_entries=0")
	require.Contains(t, output.String(), "persisted_entries=1")
	require.Contains(t, output.String(), "completed; all reported entries persisted")
}
