package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/contextmenu"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/metadata"
)

func newContextMenuServerForTest(t *testing.T) *Server {
	t.Helper()
	profileDB, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	projectDB, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = profileDB.Close()
		_ = projectDB.Close()
	})
	require.NoError(t, profileDB.AutoMigrate(&schema.YakScript{}, &schema.ContextMenuBinding{}).Error)
	require.NoError(t, projectDB.AutoMigrate(&schema.HTTPFlow{}, &schema.Risk{}).Error)
	return &Server{profileDatabase: profileDB, projectDatabase: projectDB}
}

func createContextMenuServerTestPlugin(t *testing.T, server *Server, content string, core bool) *schema.YakScript {
	t.Helper()
	script := &schema.YakScript{
		ScriptName:   "context-menu-" + uuid.NewString(),
		Type:         contextmenu.PluginType,
		Content:      content,
		Uuid:         uuid.NewString(),
		IsCorePlugin: core,
	}
	require.NoError(t, server.GetProfileDatabase().Create(script).Error)
	return script
}

func createLegacyCodecServerTestPlugin(t *testing.T, server *Server, tags string, core bool) *schema.YakScript {
	t.Helper()
	script := &schema.YakScript{
		ScriptName:   "legacy-codec-" + uuid.NewString(),
		Type:         contextmenu.LegacyPluginType,
		Content:      `result = handle(input)`,
		Tags:         tags,
		IsCorePlugin: core,
	}
	require.NoError(t, server.GetProfileDatabase().Create(script).Error)
	return script
}

func TestContextMenuQueryAndBinding(t *testing.T) {
	server := newContextMenuServerForTest(t)
	script := createContextMenuServerTestPlugin(t, server, `
handleOneHTTPFlow = func(ctx, flow) { return nil }
`, false)

	management, err := server.QueryContextMenuActions(context.Background(), &ypb.QueryContextMenuActionsRequest{IncludeDisabled: true})
	require.NoError(t, err)
	require.Len(t, management.Actions, 1)
	require.False(t, management.Actions[0].Enabled)
	require.EqualValues(t, contextmenu.MaxCustomPluginsPerScene, management.MaxCustomPluginCount)

	action, err := server.SetContextMenuActionBinding(context.Background(), &ypb.SetContextMenuActionBindingRequest{
		PluginUUID: script.Uuid,
		ActionID:   contextmenu.ActionHistorySingle,
		Enabled:    true,
		ResultMode: contextmenu.ResultModeDrawer,
		Shortcut:   "Ctrl+Alt+H",
	})
	require.NoError(t, err)
	require.True(t, action.Enabled)
	require.Equal(t, contextmenu.ResultModeDrawer, action.ResultMode)
	packetScript := createContextMenuServerTestPlugin(t, server, `
handleHTTPPacket = func(ctx, request, response) { return nil }
`, false)
	_, err = server.SetContextMenuActionBinding(context.Background(), &ypb.SetContextMenuActionBindingRequest{
		PluginUUID: packetScript.Uuid,
		ActionID:   contextmenu.ActionHTTPPacket,
		Enabled:    true,
	})
	require.NoError(t, err)

	menu, err := server.QueryContextMenuActions(context.Background(), &ypb.QueryContextMenuActionsRequest{
		Scene: contextmenu.ActionHistorySingle,
	})
	require.NoError(t, err)
	require.Len(t, menu.Actions, 1)
	require.EqualValues(t, 1, menu.EnabledCustomPluginCount)

	packetMenu, err := server.QueryContextMenuActions(context.Background(), &ypb.QueryContextMenuActionsRequest{
		Scene: contextmenu.ActionHTTPPacket,
	})
	require.NoError(t, err)
	require.Len(t, packetMenu.Actions, 1)
	require.EqualValues(t, 1, packetMenu.EnabledCustomPluginCount)
}

func TestContextMenuQueryIncludesLegacyCodecCapabilities(t *testing.T) {
	server := newContextMenuServerForTest(t)
	legacy := createLegacyCodecServerTestPlugin(t, server, strings.Join([]string{
		contextmenu.LegacyTagHistorySingle,
		contextmenu.LegacyTagPacketMutate,
	}, ","), false)

	management, err := server.QueryContextMenuActions(context.Background(), &ypb.QueryContextMenuActionsRequest{
		IncludeDisabled: true,
	})
	require.NoError(t, err)
	require.Len(t, management.Actions, 2)
	for _, action := range management.Actions {
		require.Equal(t, contextmenu.LegacyPluginType, action.PluginType)
		require.False(t, action.SupportsResultMode)
		require.False(t, action.Enabled)
		require.NotEmpty(t, action.Scene)
		require.NotEmpty(t, action.ExecutionType)
	}
	legacy.Uuid = management.Actions[0].PluginUUID
	require.NotEmpty(t, legacy.Uuid, "query should backfill UUIDs for old CODEC plugins")

	action, err := server.SetContextMenuActionBinding(context.Background(), &ypb.SetContextMenuActionBindingRequest{
		PluginUUID: legacy.Uuid,
		ActionID:   contextmenu.ActionLegacyPacketMutate,
		Enabled:    true,
		ResultMode: contextmenu.ResultModeTab,
	})
	require.NoError(t, err)
	require.True(t, action.Enabled)
	require.Equal(t, contextmenu.ResultModeAuto, action.ResultMode, "legacy execution keeps its existing result pipeline")

	packetMenu, err := server.QueryContextMenuActions(context.Background(), &ypb.QueryContextMenuActionsRequest{
		Scene: contextmenu.ActionHTTPPacket,
	})
	require.NoError(t, err)
	require.Len(t, packetMenu.Actions, 1)
	require.Equal(t, contextmenu.ActionLegacyPacketMutate, packetMenu.Actions[0].ActionID)
	require.EqualValues(t, 1, packetMenu.EnabledCustomPluginCount)

	err = server.ExecuteContextMenuAction(&ypb.ExecuteContextMenuActionRequest{
		PluginUUID: legacy.Uuid,
		ActionID:   contextmenu.ActionLegacyPacketMutate,
	}, &contextMenuEventTestStream{ctx: context.Background()})
	require.ErrorContains(t, err, "not a context-menu plugin")
}

func TestSaveContextMenuPluginValidatesHooksAndCreatesUUID(t *testing.T) {
	server := newContextMenuServerForTest(t)
	_, err := server.SaveYakScript(context.Background(), &ypb.YakScript{
		ScriptName: "invalid-context-menu",
		Type:       contextmenu.PluginType,
		Content:    `a = 1`,
	})
	require.ErrorContains(t, err, "at least one context-menu hook")

	saved, err := server.SaveYakScript(context.Background(), &ypb.YakScript{
		ScriptName: "valid-context-menu",
		Type:       contextmenu.PluginType,
		Content:    `handleOneHTTPFlow = func(ctx, flow) { return flow }`,
	})
	require.NoError(t, err)
	require.NotEmpty(t, saved.GetUUID())
	require.Equal(t, contextmenu.PluginType, saved.GetType())

	legacySaved, err := server.SaveNewYakScript(context.Background(), &ypb.SaveNewYakScriptRequest{
		ScriptName: "valid-context-menu-through-save-new",
		Type:       contextmenu.PluginType,
		Content: `handleHTTPPacket = func(ctx, request, response) {
    return context.NewPacketResult(context.ReplaceRequest(request))
}`,
	})
	require.NoError(t, err)
	require.NotEmpty(t, legacySaved.GetUUID())
}

func TestExecuteContextMenuPacketAction(t *testing.T) {
	server := newContextMenuServerForTest(t)
	script := createContextMenuServerTestPlugin(t, server, `
handleHTTPPacket = func(ctx, request, response) {
    return context.NewPacketResult(
        context.ReplaceRequest(request),
        context.RequireConfirmation(true),
    )
}
`, false)
	_, err := yakit.SetContextMenuBinding(server.GetProfileDatabase(), &schema.ContextMenuBinding{
		PluginUUID: script.Uuid,
		ActionID:   contextmenu.ActionHTTPPacket,
		Enabled:    true,
		ResultMode: contextmenu.ResultModeDialog,
	})
	require.NoError(t, err)

	stream := &contextMenuEventTestStream{ctx: context.Background()}
	err = server.ExecuteContextMenuAction(&ypb.ExecuteContextMenuActionRequest{
		PluginUUID:     script.Uuid,
		ActionID:       contextmenu.ActionHTTPPacket,
		HttpsState:     string(contextmenu.HttpsStateUnknown),
		Request:        []byte("GET / HTTP/1.1\r\n\r\n"),
		HasRequest:     true,
		PacketRevision: "revision-1",
	}, stream)
	require.NoError(t, err)
	require.NotEmpty(t, stream.events)
	require.Equal(t, "started", stream.events[0].Status)
	require.Equal(t, "completed", stream.events[len(stream.events)-1].Status)

	var packetResult *ypb.ContextMenuPacketActionResult
	for _, event := range stream.events {
		if event.PacketResult != nil {
			packetResult = event.PacketResult
			break
		}
	}
	require.NotNil(t, packetResult)
	require.True(t, packetResult.ReplaceRequest)
	require.True(t, packetResult.RequireConfirmation)
	require.Equal(t, "revision-1", packetResult.PacketRevision)
}

func TestResolveContextMenuParamsUsesDefaultsAndChecksRequired(t *testing.T) {
	paramsJSON, err := json.Marshal([]*ypb.YakScriptParam{
		{Field: "mode", DefaultValue: "safe"},
		{Field: "keyword", Required: true},
	})
	require.NoError(t, err)
	script := &schema.YakScript{Params: strconv.Quote(string(paramsJSON))}

	_, _, err = resolveContextMenuParams(script, nil)
	require.ErrorContains(t, err, "keyword")

	resolved, paramMap, err := resolveContextMenuParams(script, []*ypb.ExecParamItem{{Key: "keyword", Value: "admin"}})
	require.NoError(t, err)
	require.Len(t, resolved, 2)
	require.Equal(t, "admin", paramMap["keyword"])
	require.Equal(t, "safe", paramMap["mode"])
}

type contextMenuEventTestStream struct {
	ctx    context.Context
	events []*ypb.ContextMenuActionEvent
}

func (s *contextMenuEventTestStream) Send(event *ypb.ContextMenuActionEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *contextMenuEventTestStream) SetHeader(metadata.MD) error  { return nil }
func (s *contextMenuEventTestStream) SendHeader(metadata.MD) error { return nil }
func (s *contextMenuEventTestStream) SetTrailer(metadata.MD)       {}
func (s *contextMenuEventTestStream) Context() context.Context     { return s.ctx }
func (s *contextMenuEventTestStream) SendMsg(any) error            { return nil }
func (s *contextMenuEventTestStream) RecvMsg(any) error            { return io.EOF }
