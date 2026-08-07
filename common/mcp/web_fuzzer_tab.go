package mcp

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type webFuzzerTabCreateInput struct {
	Request      string `mapstructure:"request"`
	IsHTTPS      bool   `mapstructure:"isHttps"`
	TabName      string `mapstructure:"tabName"`
	PageID       string `mapstructure:"pageId"`
	Proxy        string `mapstructure:"proxy"`
	Concurrent   int64  `mapstructure:"concurrent"`
	HotPatchCode string `mapstructure:"hotPatchCode"`
	ActualAddr   string `mapstructure:"actualAddr"`
}

type webFuzzerStoredItem struct {
	Config   *ypb.FuzzerConfig
	Item     *yakit.WebFuzzerPageCacheItem
	ParseErr error
	IsGroup  bool
}

type webFuzzerState struct {
	Items         []*webFuzzerStoredItem
	ByID          map[string]*webFuzzerStoredItem
	Errors        []map[string]any
	UsesLiveCache bool
}

func decodeWebFuzzerArguments(arguments map[string]any, target any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decodeHook,
		Result:     target,
	})
	if err != nil {
		return utils.Wrap(err, "BUG: new map structure decoder error")
	}
	if err := decoder.Decode(arguments); err != nil {
		return utils.Wrap(err, "invalid argument")
	}
	return nil
}

func (input webFuzzerTabCreateInput) build(groupID string, sortField int64, fallbackName string) (*ypb.FuzzerConfig, error) {
	req := &ypb.FuzzerRequest{
		Request:      input.Request,
		IsHTTPS:      input.IsHTTPS,
		Proxy:        input.Proxy,
		Concurrent:   input.Concurrent,
		HotPatchCode: input.HotPatchCode,
		ActualAddr:   input.ActualAddr,
	}
	return yakit.BuildWebFuzzerConfig(req, func(opts *yakit.WebFuzzerPageBuildOptions) {
		if pageID := strings.TrimSpace(input.PageID); pageID != "" {
			opts.PageID = pageID
		}
		if tabName := strings.TrimSpace(input.TabName); tabName != "" {
			opts.TabName = tabName
		} else if fallbackName != "" {
			opts.TabName = fallbackName
		}
		opts.GroupID = groupID
		opts.SortField = sortField
	})
}

func handleCreateWebFuzzerTab(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := s.ensureLocalClient(); err != nil {
			return nil, err
		}

		var args struct {
			webFuzzerTabCreateInput `mapstructure:",squash"`
			OpenFlag                *bool `mapstructure:"openFlag"`
		}
		if err := decodeWebFuzzerArguments(request.Params.Arguments, &args); err != nil {
			return nil, err
		}
		state, err := loadWebFuzzerState(ctx, s)
		if err != nil {
			return nil, err
		}
		fuzzerConfig, err := args.webFuzzerTabCreateInput.build("0", state.nextTopLevelSort(), "MCP Web Fuzzer")
		if err != nil {
			return nil, err
		}
		if _, exists := state.ByID[fuzzerConfig.GetPageId()]; exists {
			return nil, utils.Errorf("web fuzzer page id %q already exists", fuzzerConfig.GetPageId())
		}
		if err := saveWebFuzzerConfigs(ctx, s, []*ypb.FuzzerConfig{fuzzerConfig}); err != nil {
			return nil, err
		}
		if err := syncWebFuzzerLiveCache(ctx, s, state, []*ypb.FuzzerConfig{fuzzerConfig}, nil); err != nil {
			return nil, err
		}

		openFlag := boolArgumentDefault(args.OpenFlag, true)
		yakit.BroadcastWebFuzzerTab(openFlag, fuzzerConfig)
		result := webFuzzerMutationResult("created", openFlag, []*ypb.FuzzerConfig{fuzzerConfig}, nil)
		result["pageId"] = fuzzerConfig.GetPageId()
		result["type"] = fuzzerConfig.GetType()
		result["config"] = fuzzerConfig.GetConfig()
		return NewCommonCallToolResult(result)
	}
}

func handleCreateWebFuzzerTabs(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := s.ensureLocalClient(); err != nil {
			return nil, err
		}
		var args struct {
			Tabs       []webFuzzerTabCreateInput `mapstructure:"tabs"`
			GroupName  string                    `mapstructure:"groupName"`
			GroupID    string                    `mapstructure:"groupId"`
			GroupColor string                    `mapstructure:"groupColor"`
			OpenFlag   *bool                     `mapstructure:"openFlag"`
		}
		if err := decodeWebFuzzerArguments(request.Params.Arguments, &args); err != nil {
			return nil, err
		}
		if len(args.Tabs) == 0 {
			return nil, utils.Error("tabs is required and must not be empty")
		}

		state, err := loadWebFuzzerState(ctx, s)
		if err != nil {
			return nil, err
		}
		groupID := "0"
		configs := make([]*ypb.FuzzerConfig, 0, len(args.Tabs)+1)
		if strings.TrimSpace(args.GroupName) != "" {
			groupConfig, buildErr := yakit.BuildWebFuzzerGroupConfig(args.GroupName, func(opts *yakit.WebFuzzerGroupBuildOptions) {
				opts.GroupID = strings.TrimSpace(args.GroupID)
				opts.Color = args.GroupColor
				opts.SortField = state.nextTopLevelSort()
			})
			if buildErr != nil {
				return nil, buildErr
			}
			groupID = groupConfig.GetPageId()
			configs = append(configs, groupConfig)
		} else if strings.TrimSpace(args.GroupID) != "" {
			return nil, utils.Error("groupId is only valid when groupName creates a new group")
		}

		nextTopSort := state.nextTopLevelSort()
		seen := make(map[string]struct{}, len(configs)+len(args.Tabs))
		if groupID != "0" {
			seen[groupID] = struct{}{}
		}
		for index, input := range args.Tabs {
			sortField := nextTopSort + int64(index)
			if groupID != "0" {
				sortField = int64(index + 1)
			}
			config, buildErr := input.build(groupID, sortField, "MCP Web Fuzzer "+utils.InterfaceToString(index+1))
			if buildErr != nil {
				return nil, utils.Wrapf(buildErr, "invalid tabs[%d]", index)
			}
			pageID := config.GetPageId()
			if _, exists := state.ByID[pageID]; exists {
				return nil, utils.Errorf("web fuzzer page id %q already exists", pageID)
			}
			if _, exists := seen[pageID]; exists {
				return nil, utils.Errorf("duplicate web fuzzer page id %q in batch", pageID)
			}
			seen[pageID] = struct{}{}
			configs = append(configs, config)
		}

		if err := saveWebFuzzerConfigs(ctx, s, configs); err != nil {
			return nil, err
		}
		if err := syncWebFuzzerLiveCache(ctx, s, state, configs, nil); err != nil {
			return nil, err
		}
		openFlag := boolArgumentDefault(args.OpenFlag, true)
		yakit.BroadcastWebFuzzerTab(openFlag, configs...)
		result := webFuzzerMutationResult("created", openFlag, configs, nil)
		if groupID != "0" {
			result["groupId"] = groupID
			result["groupName"] = strings.TrimSpace(args.GroupName)
		}
		return NewCommonCallToolResult(result)
	}
}

func handleQueryWebFuzzerTabs(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := s.ensureLocalClient(); err != nil {
			return nil, err
		}
		var args struct {
			PageIDs        []string `mapstructure:"pageIds"`
			Kind           string   `mapstructure:"kind"`
			NameContains   string   `mapstructure:"nameContains"`
			GroupID        string   `mapstructure:"groupId"`
			IncludeRequest bool     `mapstructure:"includeRequest"`
		}
		if err := decodeWebFuzzerArguments(request.Params.Arguments, &args); err != nil {
			return nil, err
		}
		if args.Kind != "" && args.Kind != "all" && args.Kind != "tab" && args.Kind != "group" {
			return nil, utils.Error("kind must be all, tab, or group")
		}
		state, err := loadWebFuzzerState(ctx, s)
		if err != nil {
			return nil, err
		}
		return NewCommonCallToolResult(state.queryResult(args.PageIDs, args.Kind, args.NameContains, args.GroupID, args.IncludeRequest))
	}
}

func handleUpdateWebFuzzerTab(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := s.ensureLocalClient(); err != nil {
			return nil, err
		}
		arguments := request.Params.Arguments
		var args struct {
			PageID        string `mapstructure:"pageId"`
			TabName       string `mapstructure:"tabName"`
			Request       string `mapstructure:"request"`
			IsHTTPS       bool   `mapstructure:"isHttps"`
			Proxy         string `mapstructure:"proxy"`
			Concurrent    int64  `mapstructure:"concurrent"`
			HotPatchCode  string `mapstructure:"hotPatchCode"`
			ActualAddr    string `mapstructure:"actualAddr"`
			TargetGroupID string `mapstructure:"targetGroupId"`
			Sort          int64  `mapstructure:"sort"`
			OpenFlag      *bool  `mapstructure:"openFlag"`
		}
		if err := decodeWebFuzzerArguments(arguments, &args); err != nil {
			return nil, err
		}
		args.PageID = strings.TrimSpace(args.PageID)
		state, err := loadWebFuzzerState(ctx, s)
		if err != nil {
			return nil, err
		}
		record, err := state.requireTab(args.PageID)
		if err != nil {
			return nil, err
		}

		targetGroupID := record.Item.GroupID
		if _, exists := arguments["targetGroupId"]; exists {
			targetGroupID = strings.TrimSpace(args.TargetGroupID)
			if targetGroupID == "" {
				return nil, utils.Error("targetGroupId must be a group id or 0")
			}
			if targetGroupID != "0" {
				if _, requireErr := state.requireGroup(targetGroupID); requireErr != nil {
					return nil, requireErr
				}
			}
		}

		updated, err := mutateWebFuzzerConfig(record.Config, func(root map[string]any) error {
			if _, exists := arguments["tabName"]; exists {
				name := strings.TrimSpace(args.TabName)
				if name == "" {
					return utils.Error("tabName must not be empty")
				}
				root["verbose"] = name
			}
			if _, exists := arguments["targetGroupId"]; exists {
				root["groupId"] = targetGroupID
			}
			if _, exists := arguments["sort"]; exists {
				root["sortFieId"] = args.Sort
			} else if targetGroupID != record.Item.GroupID {
				if targetGroupID == "0" {
					root["sortFieId"] = state.nextTopLevelSort()
				} else {
					root["sortFieId"] = state.nextGroupSort(targetGroupID)
				}
			}

			params := ensureWebFuzzerPageParams(root)
			if _, exists := arguments["request"]; exists {
				if args.Request == "" {
					return utils.Error("request must not be empty")
				}
				params["request"] = args.Request
			}
			if _, exists := arguments["isHttps"]; exists {
				params["isHttps"] = args.IsHTTPS
			}
			if _, exists := arguments["proxy"]; exists {
				params["proxy"] = splitWebFuzzerProxy(args.Proxy)
			}
			if _, exists := arguments["concurrent"]; exists {
				if args.Concurrent < 1 {
					return utils.Error("concurrent must be at least 1")
				}
				params["concurrent"] = args.Concurrent
			}
			if _, exists := arguments["hotPatchCode"]; exists {
				params["hotPatchCode"] = args.HotPatchCode
			}
			if _, exists := arguments["actualAddr"]; exists {
				params["actualHost"] = args.ActualAddr
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if err := saveWebFuzzerConfigs(ctx, s, []*ypb.FuzzerConfig{updated}); err != nil {
			return nil, err
		}

		deletedGroupIDs := state.emptyGroupsAfter(map[string]string{args.PageID: targetGroupID}, nil)
		if err := deleteWebFuzzerConfigs(ctx, s, deletedGroupIDs); err != nil {
			return nil, err
		}
		if err := syncWebFuzzerLiveCache(ctx, s, state, []*ypb.FuzzerConfig{updated}, deletedGroupIDs); err != nil {
			return nil, err
		}
		openFlag := boolArgumentDefault(args.OpenFlag, false)
		yakit.BroadcastWebFuzzerTabChanged(yakit.WebFuzzerTabPushActionUpdate, openFlag, []*ypb.FuzzerConfig{updated}, nil)
		if len(deletedGroupIDs) > 0 {
			yakit.BroadcastWebFuzzerTabChanged(yakit.WebFuzzerTabPushActionDelete, false, nil, deletedGroupIDs)
		}
		return NewCommonCallToolResult(webFuzzerMutationResult("updated", openFlag, []*ypb.FuzzerConfig{updated}, deletedGroupIDs))
	}
}

func handleDeleteWebFuzzerTabs(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := s.ensureLocalClient(); err != nil {
			return nil, err
		}
		var args struct {
			PageIDs []string `mapstructure:"pageIds"`
		}
		if err := decodeWebFuzzerArguments(request.Params.Arguments, &args); err != nil {
			return nil, err
		}
		pageIDs := uniqueNonEmptyStrings(args.PageIDs)
		if len(pageIDs) == 0 {
			return nil, utils.Error("pageIds is required and must not be empty")
		}
		state, err := loadWebFuzzerState(ctx, s)
		if err != nil {
			return nil, err
		}
		for _, pageID := range pageIDs {
			if _, err := state.requireTab(pageID); err != nil {
				return nil, err
			}
		}
		deletedSet := make(map[string]struct{}, len(pageIDs))
		for _, pageID := range pageIDs {
			deletedSet[pageID] = struct{}{}
		}
		emptyGroupIDs := state.emptyGroupsAfter(nil, deletedSet)
		allDeleted := uniqueNonEmptyStrings(append(pageIDs, emptyGroupIDs...))
		if err := deleteWebFuzzerConfigs(ctx, s, allDeleted); err != nil {
			return nil, err
		}
		if err := syncWebFuzzerLiveCache(ctx, s, state, nil, allDeleted); err != nil {
			return nil, err
		}
		yakit.BroadcastWebFuzzerTabChanged(yakit.WebFuzzerTabPushActionDelete, false, nil, allDeleted)
		return NewCommonCallToolResult(map[string]any{
			"status":             "ok",
			"deletedPageIds":     pageIDs,
			"deletedEmptyGroups": emptyGroupIDs,
		})
	}
}

func handleManageWebFuzzerTabGroup(s *MCPServer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := s.ensureLocalClient(); err != nil {
			return nil, err
		}
		arguments := request.Params.Arguments
		var args struct {
			Action     string   `mapstructure:"action"`
			GroupID    string   `mapstructure:"groupId"`
			GroupName  string   `mapstructure:"groupName"`
			TabIDs     []string `mapstructure:"tabIds"`
			Mode       string   `mapstructure:"mode"`
			Color      string   `mapstructure:"color"`
			Expand     bool     `mapstructure:"expand"`
			DeleteTabs bool     `mapstructure:"deleteTabs"`
			OpenFlag   *bool    `mapstructure:"openFlag"`
		}
		if err := decodeWebFuzzerArguments(arguments, &args); err != nil {
			return nil, err
		}
		args.Action = strings.TrimSpace(args.Action)
		args.GroupID = strings.TrimSpace(args.GroupID)
		args.GroupName = strings.TrimSpace(args.GroupName)
		args.TabIDs = uniqueNonEmptyStrings(args.TabIDs)
		if args.Mode == "" {
			args.Mode = "add"
		}
		state, err := loadWebFuzzerState(ctx, s)
		if err != nil {
			return nil, err
		}
		openFlag := boolArgumentDefault(args.OpenFlag, false)

		switch args.Action {
		case "create":
			return createWebFuzzerGroup(ctx, s, state, args.GroupID, args.GroupName, args.Color, args.Expand, args.TabIDs, arguments, openFlag)
		case "update":
			return updateWebFuzzerGroup(ctx, s, state, args.GroupID, args.GroupName, args.Mode, args.Color, args.Expand, args.TabIDs, arguments, openFlag)
		case "delete":
			return deleteWebFuzzerGroup(ctx, s, state, args.GroupID, args.DeleteTabs, openFlag)
		default:
			return nil, utils.Error("action must be create, update, or delete")
		}
	}
}

func createWebFuzzerGroup(
	ctx context.Context,
	s *MCPServer,
	state *webFuzzerState,
	groupID, groupName, color string,
	expand bool,
	tabIDs []string,
	arguments map[string]any,
	openFlag bool,
) (*mcp.CallToolResult, error) {
	if groupName == "" {
		return nil, utils.Error("groupName is required for create")
	}
	if len(tabIDs) == 0 {
		return nil, utils.Error("tabIds is required and must not be empty for create")
	}
	for _, tabID := range tabIDs {
		if _, err := state.requireTab(tabID); err != nil {
			return nil, err
		}
	}
	groupConfig, err := yakit.BuildWebFuzzerGroupConfig(groupName, func(opts *yakit.WebFuzzerGroupBuildOptions) {
		opts.GroupID = groupID
		opts.Color = color
		opts.SortField = state.nextTopLevelSort()
		if _, exists := arguments["expand"]; exists {
			opts.Expand = expand
		}
	})
	if err != nil {
		return nil, err
	}
	groupID = groupConfig.GetPageId()
	if _, exists := state.ByID[groupID]; exists {
		return nil, utils.Errorf("web fuzzer group id %q already exists", groupID)
	}

	changed := []*ypb.FuzzerConfig{groupConfig}
	membership := make(map[string]string, len(tabIDs))
	for index, tabID := range tabIDs {
		record, _ := state.requireTab(tabID)
		updated, mutateErr := setWebFuzzerMembership(record.Config, groupID, int64(index+1))
		if mutateErr != nil {
			return nil, mutateErr
		}
		changed = append(changed, updated)
		membership[tabID] = groupID
	}
	emptyGroupIDs := state.emptyGroupsAfter(membership, nil)
	if err := saveWebFuzzerConfigs(ctx, s, changed); err != nil {
		return nil, err
	}
	if err := deleteWebFuzzerConfigs(ctx, s, emptyGroupIDs); err != nil {
		return nil, err
	}
	if err := syncWebFuzzerLiveCache(ctx, s, state, changed, emptyGroupIDs); err != nil {
		return nil, err
	}
	broadcastWebFuzzerGroupMutation(openFlag, changed, emptyGroupIDs)
	result := webFuzzerMutationResult("group-created", openFlag, changed, emptyGroupIDs)
	result["groupId"] = groupID
	result["groupName"] = groupName
	return NewCommonCallToolResult(result)
}

func updateWebFuzzerGroup(
	ctx context.Context,
	s *MCPServer,
	state *webFuzzerState,
	groupID, groupName, mode, color string,
	expand bool,
	tabIDs []string,
	arguments map[string]any,
	openFlag bool,
) (*mcp.CallToolResult, error) {
	group, err := state.requireGroup(groupID)
	if err != nil {
		return nil, err
	}
	if _, exists := arguments["groupName"]; exists {
		groupName, err = yakit.NormalizeWebFuzzerGroupName(groupName)
		if err != nil {
			return nil, err
		}
	}
	if _, exists := arguments["color"]; exists {
		color, err = yakit.NormalizeWebFuzzerGroupColor(color)
		if err != nil {
			return nil, err
		}
	}
	for _, tabID := range tabIDs {
		if _, err := state.requireTab(tabID); err != nil {
			return nil, err
		}
	}
	if mode != "add" && mode != "remove" && mode != "replace" {
		return nil, utils.Error("mode must be add, remove, or replace")
	}
	if mode == "replace" {
		if _, exists := arguments["tabIds"]; !exists || len(tabIDs) == 0 {
			return nil, utils.Error("replace mode requires a non-empty tabIds list")
		}
	}

	changedByID := make(map[string]*ypb.FuzzerConfig)
	groupUpdated, err := mutateWebFuzzerConfig(group.Config, func(root map[string]any) error {
		if _, exists := arguments["groupName"]; exists {
			if groupName == "" {
				return utils.Error("groupName must not be empty")
			}
			root["verbose"] = groupName
		}
		if _, exists := arguments["color"]; exists {
			root["color"] = color
		}
		if _, exists := arguments["expand"]; exists {
			root["expand"] = expand
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	changedByID[groupID] = groupUpdated

	currentMembers := state.tabsInGroup(groupID)
	currentSet := make(map[string]struct{}, len(currentMembers))
	for _, member := range currentMembers {
		currentSet[member.Config.GetPageId()] = struct{}{}
	}
	requestedSet := make(map[string]struct{}, len(tabIDs))
	for _, tabID := range tabIDs {
		requestedSet[tabID] = struct{}{}
	}
	membership := make(map[string]string)
	nextTopSort := state.nextTopLevelSort()
	nextGroupSort := state.nextGroupSort(groupID)

	switch mode {
	case "add":
		for _, tabID := range tabIDs {
			if _, exists := currentSet[tabID]; exists {
				continue
			}
			record, _ := state.requireTab(tabID)
			updated, mutateErr := setWebFuzzerMembership(record.Config, groupID, nextGroupSort)
			if mutateErr != nil {
				return nil, mutateErr
			}
			nextGroupSort++
			changedByID[tabID] = updated
			membership[tabID] = groupID
		}
	case "remove":
		for _, tabID := range tabIDs {
			if _, exists := currentSet[tabID]; !exists {
				return nil, utils.Errorf("tab %q is not in group %q", tabID, groupID)
			}
			record, _ := state.requireTab(tabID)
			updated, mutateErr := setWebFuzzerMembership(record.Config, "0", nextTopSort)
			if mutateErr != nil {
				return nil, mutateErr
			}
			nextTopSort++
			changedByID[tabID] = updated
			membership[tabID] = "0"
		}
	case "replace":
		for _, member := range currentMembers {
			tabID := member.Config.GetPageId()
			if _, keep := requestedSet[tabID]; keep {
				continue
			}
			updated, mutateErr := setWebFuzzerMembership(member.Config, "0", nextTopSort)
			if mutateErr != nil {
				return nil, mutateErr
			}
			nextTopSort++
			changedByID[tabID] = updated
			membership[tabID] = "0"
		}
		for index, tabID := range tabIDs {
			record, _ := state.requireTab(tabID)
			updated, mutateErr := setWebFuzzerMembership(record.Config, groupID, int64(index+1))
			if mutateErr != nil {
				return nil, mutateErr
			}
			changedByID[tabID] = updated
			membership[tabID] = groupID
		}
	}

	emptyGroupIDs := state.emptyGroupsAfter(membership, nil)
	changed := sortedWebFuzzerConfigValues(changedByID)
	if err := saveWebFuzzerConfigs(ctx, s, changed); err != nil {
		return nil, err
	}
	if err := deleteWebFuzzerConfigs(ctx, s, emptyGroupIDs); err != nil {
		return nil, err
	}
	if err := syncWebFuzzerLiveCache(ctx, s, state, changed, emptyGroupIDs); err != nil {
		return nil, err
	}
	broadcastWebFuzzerGroupMutation(openFlag, changed, emptyGroupIDs)
	result := webFuzzerMutationResult("group-updated", openFlag, changed, emptyGroupIDs)
	result["groupId"] = groupID
	return NewCommonCallToolResult(result)
}

func deleteWebFuzzerGroup(
	ctx context.Context,
	s *MCPServer,
	state *webFuzzerState,
	groupID string,
	deleteTabs, openFlag bool,
) (*mcp.CallToolResult, error) {
	if _, err := state.requireGroup(groupID); err != nil {
		return nil, err
	}
	members := state.tabsInGroup(groupID)
	deletedIDs := []string{groupID}
	changed := make([]*ypb.FuzzerConfig, 0, len(members))
	if deleteTabs {
		for _, member := range members {
			deletedIDs = append(deletedIDs, member.Config.GetPageId())
		}
	} else {
		nextTopSort := state.nextTopLevelSort()
		for _, member := range members {
			updated, err := setWebFuzzerMembership(member.Config, "0", nextTopSort)
			if err != nil {
				return nil, err
			}
			nextTopSort++
			changed = append(changed, updated)
		}
		if err := saveWebFuzzerConfigs(ctx, s, changed); err != nil {
			return nil, err
		}
	}
	if err := deleteWebFuzzerConfigs(ctx, s, deletedIDs); err != nil {
		return nil, err
	}
	if err := syncWebFuzzerLiveCache(ctx, s, state, changed, deletedIDs); err != nil {
		return nil, err
	}
	if len(changed) > 0 {
		yakit.BroadcastWebFuzzerTabChanged(yakit.WebFuzzerTabPushActionUpdate, openFlag, changed, nil)
	}
	yakit.BroadcastWebFuzzerTabChanged(yakit.WebFuzzerTabPushActionDelete, false, nil, deletedIDs)
	return NewCommonCallToolResult(map[string]any{
		"status":         "ok",
		"operation":      "group-deleted",
		"groupId":        groupID,
		"deletedPageIds": deletedIDs,
		"ungroupedTabIds": func() []string {
			if deleteTabs {
				return []string{}
			}
			return webFuzzerConfigIDs(changed)
		}(),
	})
}

func loadWebFuzzerState(ctx context.Context, s *MCPServer) (*webFuzzerState, error) {
	var configs []*ypb.FuzzerConfig
	state := &webFuzzerState{ByID: make(map[string]*webFuzzerStoredItem)}
	liveCache, err := s.grpcClient.GetProjectKey(ctx, &ypb.GetKeyRequest{Key: yakit.WebFuzzerCacheProjectKey})
	if err != nil {
		return nil, utils.Wrap(err, "failed to query live web fuzzer cache")
	}
	if rawCache := strings.TrimSpace(liveCache.GetValue()); rawCache != "" {
		configs, err = webFuzzerConfigsFromLiveCache(rawCache)
		if err != nil {
			return nil, err
		}
		state.UsesLiveCache = true
	} else {
		response, queryErr := s.grpcClient.QueryFuzzerConfig(ctx, &ypb.QueryFuzzerConfigRequest{
			Pagination: &ypb.Paging{Limit: -1},
		})
		if queryErr != nil {
			return nil, utils.Wrap(queryErr, "failed to query web fuzzer configs")
		}
		configs = response.GetData()
	}
	for _, config := range configs {
		record := &webFuzzerStoredItem{Config: config}
		item, parseErr := yakit.ParseWebFuzzerConfig(config)
		record.Item = item
		record.ParseErr = parseErr
		record.IsGroup = config.GetType() == yakit.WebFuzzerConfigTypePageGroup || strings.HasSuffix(config.GetPageId(), "group")
		state.Items = append(state.Items, record)
		state.ByID[config.GetPageId()] = record
		if parseErr != nil {
			state.Errors = append(state.Errors, map[string]any{
				"pageId": config.GetPageId(),
				"type":   config.GetType(),
				"error":  parseErr.Error(),
			})
		}
	}
	return state, nil
}

func webFuzzerConfigsFromLiveCache(rawCache string) ([]*ypb.FuzzerConfig, error) {
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(rawCache), &items); err != nil {
		return nil, utils.Wrap(err, "unmarshal live web fuzzer cache failed")
	}
	configs := make([]*ypb.FuzzerConfig, 0, len(items))
	for index, raw := range items {
		var identity struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &identity); err != nil {
			return nil, utils.Wrapf(err, "unmarshal live web fuzzer cache item %d failed", index)
		}
		identity.ID = strings.TrimSpace(identity.ID)
		if identity.ID == "" {
			return nil, utils.Errorf("live web fuzzer cache item %d has no id", index)
		}
		configType := yakit.WebFuzzerConfigTypePage
		if strings.HasSuffix(identity.ID, "group") {
			configType = yakit.WebFuzzerConfigTypePageGroup
		}
		configs = append(configs, &ypb.FuzzerConfig{
			PageId: identity.ID,
			Type:   configType,
			Config: string(raw),
		})
	}
	return configs, nil
}

func (state *webFuzzerState) requireTab(pageID string) (*webFuzzerStoredItem, error) {
	record, err := state.require(pageID)
	if err != nil {
		return nil, err
	}
	if record.IsGroup {
		return nil, utils.Errorf("web fuzzer page id %q is a group; use manage_web_fuzzer_tab_group", pageID)
	}
	return record, nil
}

func (state *webFuzzerState) requireGroup(groupID string) (*webFuzzerStoredItem, error) {
	record, err := state.require(strings.TrimSpace(groupID))
	if err != nil {
		return nil, err
	}
	if !record.IsGroup {
		return nil, utils.Errorf("web fuzzer page id %q is not a group", groupID)
	}
	return record, nil
}

func (state *webFuzzerState) require(pageID string) (*webFuzzerStoredItem, error) {
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return nil, utils.Error("pageId or groupId is required")
	}
	record, exists := state.ByID[pageID]
	if !exists {
		return nil, utils.Errorf("web fuzzer item %q does not exist; call query_web_fuzzer_tabs first", pageID)
	}
	if record.ParseErr != nil {
		return nil, utils.Wrapf(record.ParseErr, "web fuzzer item %q is invalid", pageID)
	}
	return record, nil
}

func (state *webFuzzerState) nextTopLevelSort() int64 {
	var maximum int64
	for _, record := range state.Items {
		if record.ParseErr == nil && record.Item.GroupID == "0" && record.Item.SortField > maximum {
			maximum = record.Item.SortField
		}
	}
	return maximum + 1
}

func (state *webFuzzerState) nextGroupSort(groupID string) int64 {
	var maximum int64
	for _, record := range state.Items {
		if record.ParseErr == nil && !record.IsGroup && record.Item.GroupID == groupID && record.Item.SortField > maximum {
			maximum = record.Item.SortField
		}
	}
	return maximum + 1
}

func (state *webFuzzerState) tabsInGroup(groupID string) []*webFuzzerStoredItem {
	var result []*webFuzzerStoredItem
	for _, record := range state.Items {
		if record.ParseErr == nil && !record.IsGroup && record.Item.GroupID == groupID {
			result = append(result, record)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Item.SortField < result[j].Item.SortField
	})
	return result
}

// emptyGroupsAfter returns groups that have no members after applying the
// supplied page membership overrides and page deletions.
func (state *webFuzzerState) emptyGroupsAfter(membership map[string]string, deleted map[string]struct{}) []string {
	counts := make(map[string]int)
	candidates := make(map[string]struct{})
	for _, record := range state.Items {
		if record.ParseErr != nil || record.IsGroup {
			continue
		}
		pageID := record.Config.GetPageId()
		originalGroupID := record.Item.GroupID
		if _, isDeleted := deleted[pageID]; isDeleted {
			if originalGroupID != "0" {
				candidates[originalGroupID] = struct{}{}
			}
			continue
		}
		groupID := originalGroupID
		if changedGroupID, changed := membership[pageID]; changed {
			groupID = changedGroupID
			if originalGroupID != "0" && originalGroupID != changedGroupID {
				candidates[originalGroupID] = struct{}{}
			}
		}
		if groupID != "0" {
			counts[groupID]++
		}
	}
	var result []string
	for _, record := range state.Items {
		_, affected := candidates[record.Config.GetPageId()]
		if record.ParseErr == nil && record.IsGroup && affected && counts[record.Config.GetPageId()] == 0 {
			result = append(result, record.Config.GetPageId())
		}
	}
	sort.Strings(result)
	return result
}

func (state *webFuzzerState) queryResult(pageIDs []string, kind, nameContains, groupID string, includeRequest bool) map[string]any {
	pageIDFilter := make(map[string]struct{})
	for _, pageID := range uniqueNonEmptyStrings(pageIDs) {
		pageIDFilter[pageID] = struct{}{}
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "all"
	}
	nameContains = strings.ToLower(strings.TrimSpace(nameContains))
	groupID = strings.TrimSpace(groupID)

	groups := make([]map[string]any, 0)
	tabs := make([]map[string]any, 0)
	for _, record := range state.Items {
		if record.ParseErr != nil {
			continue
		}
		pageID := record.Config.GetPageId()
		if len(pageIDFilter) > 0 {
			if _, matched := pageIDFilter[pageID]; !matched {
				continue
			}
		}
		if nameContains != "" && !strings.Contains(strings.ToLower(record.Item.Verbose), nameContains) {
			continue
		}
		if record.IsGroup {
			if kind == "tab" || (groupID != "" && pageID != groupID) {
				continue
			}
			summary := webFuzzerRecordSummary(record, includeRequest)
			summary["tabIds"] = webFuzzerRecordIDs(state.tabsInGroup(pageID))
			groups = append(groups, summary)
			continue
		}
		if kind == "group" || (groupID != "" && record.Item.GroupID != groupID) {
			continue
		}
		summary := webFuzzerRecordSummary(record, includeRequest)
		if group, exists := state.ByID[record.Item.GroupID]; exists && group.ParseErr == nil && group.IsGroup {
			summary["groupName"] = group.Item.Verbose
		}
		tabs = append(tabs, summary)
	}
	sort.SliceStable(groups, func(i, j int) bool { return numericMapValue(groups[i], "sort") < numericMapValue(groups[j], "sort") })
	sort.SliceStable(tabs, func(i, j int) bool {
		leftGroup := utils.InterfaceToString(tabs[i]["groupId"])
		rightGroup := utils.InterfaceToString(tabs[j]["groupId"])
		if leftGroup == rightGroup {
			return numericMapValue(tabs[i], "sort") < numericMapValue(tabs[j], "sort")
		}
		return leftGroup < rightGroup
	})
	result := map[string]any{
		"source": func() string {
			if state.UsesLiveCache {
				return "live-cache"
			}
			return "saved-configs"
		}(),
		"totalGroups":  len(groups),
		"totalTabs":    len(tabs),
		"groups":       groups,
		"tabs":         tabs,
		"invalidItems": state.Errors,
	}
	for key, value := range state.groupColorOverview() {
		result[key] = value
	}
	return result
}

func (state *webFuzzerState) groupColorOverview() map[string]any {
	palette := yakit.WebFuzzerGroupColors()
	groupsByColor := make(map[string][]map[string]any)
	for _, record := range state.Items {
		if record.ParseErr != nil || !record.IsGroup {
			continue
		}
		color := strings.TrimSpace(record.Item.Color)
		if normalized, normalizeErr := yakit.NormalizeWebFuzzerGroupColor(color); normalizeErr == nil {
			color = normalized
		}
		groupsByColor[color] = append(groupsByColor[color], map[string]any{
			"groupId":   record.Config.GetPageId(),
			"groupName": record.Item.Verbose,
		})
	}

	usedColors := make([]string, 0, len(palette))
	availableColors := make([]string, 0, len(palette))
	usage := make([]map[string]any, 0, len(groupsByColor))
	recommendedColor := ""
	leastUsedColor := ""
	leastUsedCount := len(state.Items) + 1
	for _, color := range palette {
		groups := groupsByColor[color]
		if len(groups) == 0 {
			availableColors = append(availableColors, color)
			if recommendedColor == "" {
				recommendedColor = color
			}
			continue
		}
		usedColors = append(usedColors, color)
		usage = append(usage, map[string]any{
			"color":  color,
			"count":  len(groups),
			"groups": groups,
		})
		if len(groups) < leastUsedCount {
			leastUsedColor = color
			leastUsedCount = len(groups)
		}
		delete(groupsByColor, color)
	}
	if recommendedColor == "" {
		recommendedColor = leastUsedColor
	}
	customColors := make([]string, 0, len(groupsByColor))
	for color := range groupsByColor {
		customColors = append(customColors, color)
	}
	sort.Strings(customColors)
	for _, color := range customColors {
		groups := groupsByColor[color]
		usedColors = append(usedColors, color)
		usage = append(usage, map[string]any{
			"color":  color,
			"count":  len(groups),
			"groups": groups,
		})
	}

	return map[string]any{
		"groupColorPalette":             palette,
		"customGroupColorFormat":        "#RRGGBB",
		"customGroupColorSupported":     true,
		"groupColorUsage":               usage,
		"usedGroupColors":               usedColors,
		"availableGroupColors":          availableColors,
		"recommendedGroupColor":         recommendedColor,
		"presetColorCollisionAvoidable": len(availableColors) > 0,
		"colorCollisionAvoidable":       true,
	}
}

func webFuzzerRecordSummary(record *webFuzzerStoredItem, includeRequest bool) map[string]any {
	result := map[string]any{
		"pageId":  record.Config.GetPageId(),
		"type":    record.Config.GetType(),
		"name":    record.Item.Verbose,
		"groupId": record.Item.GroupID,
		"sort":    record.Item.SortField,
	}
	if record.IsGroup {
		result["expand"] = record.Item.Expand
		color := strings.TrimSpace(record.Item.Color)
		if normalized, normalizeErr := yakit.NormalizeWebFuzzerGroupColor(color); normalizeErr == nil {
			color = normalized
		}
		result["color"] = color
		return result
	}
	if record.Item.PageParams == nil {
		return result
	}
	requestLine := record.Item.PageParams.Request
	if index := strings.IndexAny(requestLine, "\r\n"); index >= 0 {
		requestLine = requestLine[:index]
	}
	result["requestLine"] = requestLine
	result["isHttps"] = record.Item.PageParams.IsHttps
	if includeRequest {
		result["request"] = record.Item.PageParams.Request
		result["actualAddr"] = record.Item.PageParams.ActualHost
		result["proxy"] = record.Item.PageParams.Proxy
		result["concurrent"] = record.Item.PageParams.Concurrent
		result["hotPatchCode"] = record.Item.PageParams.HotPatchCode
	}
	return result
}

func mutateWebFuzzerConfig(config *ypb.FuzzerConfig, mutate func(map[string]any) error) (*ypb.FuzzerConfig, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(config.GetConfig()), &root); err != nil {
		return nil, utils.Wrap(err, "unmarshal web fuzzer config failed")
	}
	if err := mutate(root); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, utils.Wrap(err, "marshal web fuzzer config failed")
	}
	updated := &ypb.FuzzerConfig{PageId: config.GetPageId(), Type: config.GetType(), Config: string(raw)}
	if _, err := yakit.ParseWebFuzzerConfig(updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func setWebFuzzerMembership(config *ypb.FuzzerConfig, groupID string, sortField int64) (*ypb.FuzzerConfig, error) {
	return mutateWebFuzzerConfig(config, func(root map[string]any) error {
		root["groupId"] = groupID
		root["sortFieId"] = sortField
		if params, ok := root["pageParams"].(map[string]any); ok {
			params["groupId"] = groupID
		}
		return nil
	})
}

func ensureWebFuzzerPageParams(root map[string]any) map[string]any {
	if params, ok := root["pageParams"].(map[string]any); ok {
		return params
	}
	params := make(map[string]any)
	root["pageParams"] = params
	return params
}

func saveWebFuzzerConfigs(ctx context.Context, s *MCPServer, configs []*ypb.FuzzerConfig) error {
	if len(configs) == 0 {
		return nil
	}
	if _, err := s.grpcClient.SaveFuzzerConfig(ctx, &ypb.SaveFuzzerConfigRequest{Data: configs}); err != nil {
		return utils.Wrap(err, "failed to save web fuzzer configs")
	}
	return nil
}

func deleteWebFuzzerConfigs(ctx context.Context, s *MCPServer, pageIDs []string) error {
	pageIDs = uniqueNonEmptyStrings(pageIDs)
	if len(pageIDs) == 0 {
		return nil
	}
	if _, err := s.grpcClient.DeleteFuzzerConfig(ctx, &ypb.DeleteFuzzerConfigRequest{PageId: pageIDs}); err != nil {
		return utils.Wrap(err, "failed to delete web fuzzer configs")
	}
	return nil
}

func syncWebFuzzerLiveCache(
	ctx context.Context,
	s *MCPServer,
	state *webFuzzerState,
	changed []*ypb.FuzzerConfig,
	deletedIDs []string,
) error {
	if state == nil || !state.UsesLiveCache {
		return nil
	}
	deleted := make(map[string]struct{})
	for _, pageID := range uniqueNonEmptyStrings(deletedIDs) {
		deleted[pageID] = struct{}{}
	}
	changedByID := make(map[string]*ypb.FuzzerConfig)
	for _, config := range changed {
		if config != nil && config.GetPageId() != "" {
			changedByID[config.GetPageId()] = config
		}
	}

	ordered := make([]*ypb.FuzzerConfig, 0, len(state.Items)+len(changedByID))
	seen := make(map[string]struct{})
	for _, record := range state.Items {
		pageID := record.Config.GetPageId()
		if _, removed := deleted[pageID]; removed {
			continue
		}
		if _, duplicate := seen[pageID]; duplicate {
			continue
		}
		seen[pageID] = struct{}{}
		if updated, exists := changedByID[pageID]; exists {
			ordered = append(ordered, updated)
			delete(changedByID, pageID)
		} else {
			ordered = append(ordered, record.Config)
		}
	}
	for _, config := range changed {
		if config == nil {
			continue
		}
		if _, exists := changedByID[config.GetPageId()]; !exists {
			continue
		}
		ordered = append(ordered, config)
		delete(changedByID, config.GetPageId())
	}

	cacheItems := make([]json.RawMessage, 0, len(ordered))
	for _, config := range ordered {
		if !json.Valid([]byte(config.GetConfig())) {
			return utils.Errorf("web fuzzer config %q contains invalid JSON", config.GetPageId())
		}
		cacheItems = append(cacheItems, json.RawMessage(config.GetConfig()))
	}
	rawCache, err := json.Marshal(cacheItems)
	if err != nil {
		return utils.Wrap(err, "marshal live web fuzzer cache failed")
	}
	if _, err := s.grpcClient.SetProjectKey(ctx, &ypb.SetKeyRequest{
		Key:   yakit.WebFuzzerCacheProjectKey,
		Value: string(rawCache),
	}); err != nil {
		return utils.Wrap(err, "failed to update live web fuzzer cache")
	}
	return nil
}

func broadcastWebFuzzerGroupMutation(openFlag bool, changed []*ypb.FuzzerConfig, deletedGroupIDs []string) {
	if len(changed) > 0 {
		yakit.BroadcastWebFuzzerTabChanged(yakit.WebFuzzerTabPushActionUpdate, openFlag, changed, nil)
	}
	if len(deletedGroupIDs) > 0 {
		yakit.BroadcastWebFuzzerTabChanged(yakit.WebFuzzerTabPushActionDelete, false, nil, deletedGroupIDs)
	}
}

func webFuzzerMutationResult(operation string, openFlag bool, changed []*ypb.FuzzerConfig, deletedIDs []string) map[string]any {
	return map[string]any{
		"status":         "ok",
		"operation":      operation,
		"openFlag":       openFlag,
		"changedPageIds": webFuzzerConfigIDs(changed),
		"deletedPageIds": uniqueNonEmptyStrings(deletedIDs),
	}
}

func webFuzzerConfigIDs(configs []*ypb.FuzzerConfig) []string {
	result := make([]string, 0, len(configs))
	for _, config := range configs {
		if config != nil && config.GetPageId() != "" {
			result = append(result, config.GetPageId())
		}
	}
	return result
}

func webFuzzerRecordIDs(records []*webFuzzerStoredItem) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		result = append(result, record.Config.GetPageId())
	}
	return result
}

func sortedWebFuzzerConfigValues(configs map[string]*ypb.FuzzerConfig) []*ypb.FuzzerConfig {
	keys := make([]string, 0, len(configs))
	for key := range configs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]*ypb.FuzzerConfig, 0, len(keys))
	for _, key := range keys {
		result = append(result, configs[key])
	}
	return result
}

func splitWebFuzzerProxy(proxy string) []string {
	parts := strings.Split(proxy, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func uniqueNonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func boolArgumentDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func numericMapValue(value map[string]any, key string) int64 {
	switch number := value[key].(type) {
	case int64:
		return number
	case int:
		return int64(number)
	case float64:
		return int64(number)
	default:
		return 0
	}
}
