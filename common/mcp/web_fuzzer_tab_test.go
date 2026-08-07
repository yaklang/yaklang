package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	rawmcp "github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCreateWebFuzzerTabToolRegistered(t *testing.T) {
	set, ok := globalToolSets["http_fuzzer"]
	if !ok {
		t.Fatalf("http_fuzzer tool set not registered")
	}
	for _, name := range []string{
		"create_web_fuzzer_tab",
		"create_web_fuzzer_tabs",
		"query_web_fuzzer_tabs",
		"update_web_fuzzer_tab",
		"delete_web_fuzzer_tabs",
		"manage_web_fuzzer_tab_group",
	} {
		if _, exists := set.Tools[name]; !exists {
			t.Fatalf("tool not registered: %s", name)
		}
	}
	if _, exists := set.Tools["http_fuzzer"]; !exists {
		t.Fatalf("tool not registered: http_fuzzer")
	}
}

func TestWebFuzzerGroupToolSchemaEncouragesConciseColoredGroups(t *testing.T) {
	set := globalToolSets["http_fuzzer"]
	require.NotNil(t, set)
	for _, testCase := range []struct {
		toolName, colorField string
		wantDefault          bool
	}{
		{toolName: "create_web_fuzzer_tabs", colorField: "groupColor", wantDefault: true},
		{toolName: "manage_web_fuzzer_tab_group", colorField: "color"},
	} {
		tool := set.Tools[testCase.toolName]
		require.NotNil(t, tool)

		raw, err := json.Marshal(tool.tool)
		require.NoError(t, err)
		var schema map[string]any
		require.NoError(t, json.Unmarshal(raw, &schema))
		properties := schema["inputSchema"].(map[string]any)["properties"].(map[string]any)
		groupName := properties["groupName"].(map[string]any)
		groupColor := properties[testCase.colorField].(map[string]any)

		require.EqualValues(t, yakit.WebFuzzerGroupNameMaxLength, groupName["maxLength"])
		if testCase.wantDefault {
			require.Equal(t, yakit.WebFuzzerDefaultGroupColor, groupColor["default"])
		}
		require.Equal(t, yakit.WebFuzzerGroupColorPattern(), groupColor["pattern"])
	}
}

func TestWebFuzzerTabManagementLifecycle(t *testing.T) {
	yakit.CallPostInitDatabase()
	srv, err := NewMCPServer(WithEnableAllToolSets())
	require.NoError(t, err)
	require.NoError(t, srv.BindLocalGRPCClient())

	ctx := context.Background()
	originalLiveCache, err := srv.grpcClient.GetProjectKey(ctx, &ypb.GetKeyRequest{Key: yakit.WebFuzzerCacheProjectKey})
	require.NoError(t, err)
	defer func() {
		_, _ = srv.grpcClient.SetProjectKey(ctx, &ypb.SetKeyRequest{
			Key:   yakit.WebFuzzerCacheProjectKey,
			Value: originalLiveCache.GetValue(),
		})
	}()

	groupID := uuid.NewString() + "-group"
	tabOneID := uuid.NewString()
	tabTwoID := uuid.NewString()
	manualTabID := uuid.NewString()
	allIDs := []string{groupID, tabOneID, tabTwoID, manualTabID}
	defer func() {
		_, _ = srv.grpcClient.DeleteFuzzerConfig(ctx, &ypb.DeleteFuzzerConfigRequest{PageId: allIDs})
	}()

	manualConfig, err := yakit.BuildWebFuzzerConfig(&ypb.FuzzerRequest{
		Request: "GET /manual HTTP/1.1\r\nHost: example.com\r\n\r\n",
	}, func(options *yakit.WebFuzzerPageBuildOptions) {
		options.PageID = manualTabID
		options.TabName = "manually opened tab"
	})
	require.NoError(t, err)
	_, err = srv.grpcClient.SetProjectKey(ctx, &ypb.SetKeyRequest{
		Key:   yakit.WebFuzzerCacheProjectKey,
		Value: "[" + manualConfig.GetConfig() + "]",
	})
	require.NoError(t, err)

	created := invokeWebFuzzerTool(t, ctx, srv, "create_web_fuzzer_tabs", map[string]any{
		"groupName": "MCP lifecycle group",
		"groupId":   groupID,
		"openFlag":  false,
		"tabs": []any{
			map[string]any{
				"pageId":     tabOneID,
				"tabName":    "login request",
				"request":    "GET /login HTTP/1.1\r\nHost: example.com\r\n\r\n",
				"isHttps":    true,
				"concurrent": float64(5),
			},
			map[string]any{
				"pageId":  tabTwoID,
				"tabName": "admin request",
				"request": "GET /admin HTTP/1.1\r\nHost: example.com\r\n\r\n",
				"isHttps": true,
			},
		},
	})
	require.Equal(t, "created", created["operation"])
	require.Equal(t, groupID, created["groupId"])

	queried := invokeWebFuzzerTool(t, ctx, srv, "query_web_fuzzer_tabs", map[string]any{
		"pageIds":        allIDs,
		"includeRequest": true,
	})
	require.EqualValues(t, 1, queried["totalGroups"])
	require.EqualValues(t, 3, queried["totalTabs"])
	require.Equal(t, "live-cache", queried["source"])
	require.Equal(t, []any{yakit.WebFuzzerDefaultGroupColor}, queried["usedGroupColors"])
	require.Equal(t, "blue", queried["recommendedGroupColor"])
	require.Equal(t, true, queried["colorCollisionAvoidable"])
	require.Equal(t, true, queried["customGroupColorSupported"])
	require.Equal(t, "#RRGGBB", queried["customGroupColorFormat"])
	require.NotContains(t, queried["availableGroupColors"], yakit.WebFuzzerDefaultGroupColor)

	updated := invokeWebFuzzerTool(t, ctx, srv, "update_web_fuzzer_tab", map[string]any{
		"pageId":        tabOneID,
		"tabName":       "renamed login request",
		"targetGroupId": "0",
		"proxy":         "http://127.0.0.1:8080",
		"openFlag":      false,
	})
	require.Equal(t, "updated", updated["operation"])

	groupUpdated := invokeWebFuzzerTool(t, ctx, srv, "manage_web_fuzzer_tab_group", map[string]any{
		"action":    "update",
		"groupId":   groupID,
		"groupName": "renamed lifecycle group",
		"mode":      "add",
		"tabIds":    []string{tabOneID, manualTabID},
	})
	require.Equal(t, "group-updated", groupUpdated["operation"])

	groupDeleted := invokeWebFuzzerTool(t, ctx, srv, "manage_web_fuzzer_tab_group", map[string]any{
		"action":     "delete",
		"groupId":    groupID,
		"deleteTabs": false,
	})
	require.Equal(t, "group-deleted", groupDeleted["operation"])

	response, err := srv.grpcClient.QueryFuzzerConfig(ctx, &ypb.QueryFuzzerConfigRequest{
		PageId:     []string{tabOneID, tabTwoID, manualTabID},
		Pagination: &ypb.Paging{Limit: -1},
	})
	require.NoError(t, err)
	require.Len(t, response.GetData(), 3)
	for _, config := range response.GetData() {
		item, parseErr := yakit.ParseWebFuzzerConfig(config)
		require.NoError(t, parseErr)
		require.Equal(t, "0", item.GroupID)
	}

	deleted := invokeWebFuzzerTool(t, ctx, srv, "delete_web_fuzzer_tabs", map[string]any{
		"pageIds": []string{tabOneID, tabTwoID, manualTabID},
	})
	require.ElementsMatch(t, []any{tabOneID, tabTwoID, manualTabID}, deleted["deletedPageIds"])
}

func TestWebFuzzerTabGroupMembershipModesAndCascadeDelete(t *testing.T) {
	yakit.CallPostInitDatabase()
	srv, err := NewMCPServer(WithEnableAllToolSets())
	require.NoError(t, err)
	require.NoError(t, srv.BindLocalGRPCClient())

	ctx := context.Background()
	originalLiveCache, err := srv.grpcClient.GetProjectKey(ctx, &ypb.GetKeyRequest{Key: yakit.WebFuzzerCacheProjectKey})
	require.NoError(t, err)
	_, err = srv.grpcClient.SetProjectKey(ctx, &ypb.SetKeyRequest{Key: yakit.WebFuzzerCacheProjectKey, Value: "[]"})
	require.NoError(t, err)
	defer func() {
		_, _ = srv.grpcClient.SetProjectKey(ctx, &ypb.SetKeyRequest{
			Key:   yakit.WebFuzzerCacheProjectKey,
			Value: originalLiveCache.GetValue(),
		})
	}()

	groupID := uuid.NewString() + "-group"
	tabA := uuid.NewString()
	tabB := uuid.NewString()
	tabC := uuid.NewString()
	tabD := uuid.NewString()
	allIDs := []string{groupID, tabA, tabB, tabC, tabD}
	defer func() {
		_, _ = srv.grpcClient.DeleteFuzzerConfig(ctx, &ypb.DeleteFuzzerConfigRequest{PageId: allIDs})
	}()

	tabs := make([]any, 0, 4)
	for _, pageID := range []string{tabA, tabB, tabC, tabD} {
		tabs = append(tabs, map[string]any{
			"pageId":  pageID,
			"tabName": "membership tab",
			"request": "GET /membership HTTP/1.1\r\nHost: example.com\r\n\r\n",
			"isHttps": false,
		})
	}
	invokeWebFuzzerTool(t, ctx, srv, "create_web_fuzzer_tabs", map[string]any{"tabs": tabs, "openFlag": false})
	invokeWebFuzzerTool(t, ctx, srv, "manage_web_fuzzer_tab_group", map[string]any{
		"action": "create", "groupId": groupID, "groupName": "membership", "tabIds": []string{tabA, tabB},
	})
	require.Equal(t, map[string]string{tabA: groupID, tabB: groupID, tabC: "0", tabD: "0"},
		webFuzzerTabMemberships(t, invokeWebFuzzerTool(t, ctx, srv, "query_web_fuzzer_tabs", map[string]any{"kind": "tab"})))

	invokeWebFuzzerTool(t, ctx, srv, "manage_web_fuzzer_tab_group", map[string]any{
		"action": "update", "groupId": groupID, "mode": "remove", "tabIds": []string{tabA},
	})
	require.Equal(t, map[string]string{tabA: "0", tabB: groupID, tabC: "0", tabD: "0"},
		webFuzzerTabMemberships(t, invokeWebFuzzerTool(t, ctx, srv, "query_web_fuzzer_tabs", map[string]any{"kind": "tab"})))

	invokeWebFuzzerTool(t, ctx, srv, "manage_web_fuzzer_tab_group", map[string]any{
		"action": "update", "groupId": groupID, "mode": "add", "tabIds": []string{tabC},
	})
	require.Equal(t, map[string]string{tabA: "0", tabB: groupID, tabC: groupID, tabD: "0"},
		webFuzzerTabMemberships(t, invokeWebFuzzerTool(t, ctx, srv, "query_web_fuzzer_tabs", map[string]any{"kind": "tab"})))

	invokeWebFuzzerTool(t, ctx, srv, "manage_web_fuzzer_tab_group", map[string]any{
		"action": "update", "groupId": groupID, "mode": "replace", "tabIds": []string{tabA, tabD},
	})
	require.Equal(t, map[string]string{tabA: groupID, tabB: "0", tabC: "0", tabD: groupID},
		webFuzzerTabMemberships(t, invokeWebFuzzerTool(t, ctx, srv, "query_web_fuzzer_tabs", map[string]any{"kind": "tab"})))

	deleted := invokeWebFuzzerTool(t, ctx, srv, "manage_web_fuzzer_tab_group", map[string]any{
		"action": "delete", "groupId": groupID, "deleteTabs": true,
	})
	require.ElementsMatch(t, []any{groupID, tabA, tabD}, deleted["deletedPageIds"])
	remaining := invokeWebFuzzerTool(t, ctx, srv, "query_web_fuzzer_tabs", map[string]any{})
	require.EqualValues(t, 0, remaining["totalGroups"])
	require.Equal(t, map[string]string{tabB: "0", tabC: "0"}, webFuzzerTabMemberships(t, remaining))

	invokeWebFuzzerTool(t, ctx, srv, "delete_web_fuzzer_tabs", map[string]any{"pageIds": []string{tabB, tabC}})
}

func TestWebFuzzerTabInvalidMutationsAreAtomic(t *testing.T) {
	yakit.CallPostInitDatabase()
	srv, err := NewMCPServer(WithEnableAllToolSets())
	require.NoError(t, err)
	require.NoError(t, srv.BindLocalGRPCClient())

	ctx := context.Background()
	originalLiveCache, err := srv.grpcClient.GetProjectKey(ctx, &ypb.GetKeyRequest{Key: yakit.WebFuzzerCacheProjectKey})
	require.NoError(t, err)
	_, err = srv.grpcClient.SetProjectKey(ctx, &ypb.SetKeyRequest{Key: yakit.WebFuzzerCacheProjectKey, Value: "[]"})
	require.NoError(t, err)
	defer func() {
		_, _ = srv.grpcClient.SetProjectKey(ctx, &ypb.SetKeyRequest{
			Key:   yakit.WebFuzzerCacheProjectKey,
			Value: originalLiveCache.GetValue(),
		})
	}()

	existingID := uuid.NewString()
	newID := uuid.NewString()
	missingID := uuid.NewString()
	invalidGroupID := uuid.NewString() + "-group"
	allIDs := []string{existingID, newID, invalidGroupID}
	defer func() {
		_, _ = srv.grpcClient.DeleteFuzzerConfig(ctx, &ypb.DeleteFuzzerConfigRequest{PageId: allIDs})
	}()

	invokeWebFuzzerTool(t, ctx, srv, "create_web_fuzzer_tab", map[string]any{
		"pageId": existingID, "tabName": "existing", "request": "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n", "isHttps": false,
	})

	_, err = InvokeBuiltinTool(ctx, srv, "create_web_fuzzer_tabs", map[string]any{
		"tabs": []any{
			map[string]any{"pageId": newID, "request": "GET /new HTTP/1.1\r\nHost: example.com\r\n\r\n", "isHttps": false},
			map[string]any{"pageId": existingID, "request": "GET /duplicate HTTP/1.1\r\nHost: example.com\r\n\r\n", "isHttps": false},
		},
	})
	require.ErrorContains(t, err, "already exists")

	_, err = InvokeBuiltinTool(ctx, srv, "manage_web_fuzzer_tab_group", map[string]any{
		"action": "create", "groupId": invalidGroupID, "groupName": "invalid", "tabIds": []string{existingID, missingID},
	})
	require.ErrorContains(t, err, "does not exist")

	_, err = InvokeBuiltinTool(ctx, srv, "delete_web_fuzzer_tabs", map[string]any{"pageIds": []string{existingID, missingID}})
	require.ErrorContains(t, err, "does not exist")

	_, err = InvokeBuiltinTool(ctx, srv, "update_web_fuzzer_tab", map[string]any{
		"pageId": existingID, "targetGroupId": invalidGroupID,
	})
	require.ErrorContains(t, err, "does not exist")

	state := invokeWebFuzzerTool(t, ctx, srv, "query_web_fuzzer_tabs", map[string]any{})
	require.EqualValues(t, 0, state["totalGroups"])
	require.Equal(t, map[string]string{existingID: "0"}, webFuzzerTabMemberships(t, state))

	invokeWebFuzzerTool(t, ctx, srv, "delete_web_fuzzer_tabs", map[string]any{"pageIds": []string{existingID}})
}

func TestMutateWebFuzzerConfigPreservesUnknownFields(t *testing.T) {
	config, err := yakit.BuildWebFuzzerConfig(&ypb.FuzzerRequest{
		Request: "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n",
	}, func(options *yakit.WebFuzzerPageBuildOptions) {
		options.PageID = "preserve-fields"
	})
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(config.GetConfig()), &raw))
	raw["futureTopLevelField"] = map[string]any{"enabled": true}
	raw["pageParams"].(map[string]any)["futurePageField"] = "kept"
	withUnknown, err := json.Marshal(raw)
	require.NoError(t, err)
	config.Config = string(withUnknown)

	updated, err := mutateWebFuzzerConfig(config, func(root map[string]any) error {
		root["verbose"] = "renamed"
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(updated.GetConfig()), &raw))
	require.Equal(t, map[string]any{"enabled": true}, raw["futureTopLevelField"])
	require.Equal(t, "kept", raw["pageParams"].(map[string]any)["futurePageField"])
}

func TestWebFuzzerGroupColorOverviewUsesUnusedThenLeastUsedColor(t *testing.T) {
	state := &webFuzzerState{}
	for index, color := range yakit.WebFuzzerGroupColors() {
		config, err := yakit.BuildWebFuzzerGroupConfig("group-"+color, func(options *yakit.WebFuzzerGroupBuildOptions) {
			options.GroupID = color + "-group"
			options.Color = color
			options.SortField = int64(index + 1)
		})
		require.NoError(t, err)
		item, err := yakit.ParseWebFuzzerConfig(config)
		require.NoError(t, err)
		state.Items = append(state.Items, &webFuzzerStoredItem{Config: config, Item: item, IsGroup: true})
	}

	overview := state.groupColorOverview()
	require.Empty(t, overview["availableGroupColors"])
	require.Equal(t, false, overview["presetColorCollisionAvoidable"])
	require.Equal(t, true, overview["colorCollisionAvoidable"])
	require.Equal(t, yakit.WebFuzzerDefaultGroupColor, overview["recommendedGroupColor"])

	extraPurple, err := yakit.BuildWebFuzzerGroupConfig("second-purple", func(options *yakit.WebFuzzerGroupBuildOptions) {
		options.GroupID = "second-purple-group"
	})
	require.NoError(t, err)
	extraItem, err := yakit.ParseWebFuzzerConfig(extraPurple)
	require.NoError(t, err)
	state.Items = append(state.Items, &webFuzzerStoredItem{Config: extraPurple, Item: extraItem, IsGroup: true})

	overview = state.groupColorOverview()
	require.Equal(t, "blue", overview["recommendedGroupColor"])
}

func invokeWebFuzzerTool(t *testing.T, ctx context.Context, srv *MCPServer, name string, args map[string]any) map[string]any {
	t.Helper()
	result, err := InvokeBuiltinTool(ctx, srv, name, args)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(rawmcp.TextContent)
	require.True(t, ok)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &decoded))
	return decoded
}

func webFuzzerTabMemberships(t *testing.T, result map[string]any) map[string]string {
	t.Helper()
	memberships := make(map[string]string)
	tabs, ok := result["tabs"].([]any)
	require.True(t, ok)
	for _, rawTab := range tabs {
		tab, ok := rawTab.(map[string]any)
		require.True(t, ok)
		pageID, ok := tab["pageId"].(string)
		require.True(t, ok)
		groupID, ok := tab["groupId"].(string)
		require.True(t, ok)
		memberships[pageID] = groupID
	}
	return memberships
}
