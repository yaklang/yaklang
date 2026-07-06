package mcp

import (
	"context"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("fingerprint",
		WithTool(mcp.NewTool("query_fingerprint",
			mcp.WithDescription("Query service fingerprints"),
			mcp.WithPaging("pagination", []string{"id", "created_at", "updated_at", "name", "type"},
				mcp.Description("Pagination settings")),
			mcp.WithStruct("filter", []mcp.PropertyOption{mcp.Description("Fingerprint filter")}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.QueryFingerprintRequest) (any, error) {
			return s.grpcClient.QueryFingerprint(ctx, req)
		}, "failed to query fingerprint")),

		WithTool(mcp.NewTool("create_fingerprint",
			mcp.WithDescription("Create a new service fingerprint rule"),
			mcp.WithStruct("fingerprint", []mcp.PropertyOption{
				mcp.Description("Fingerprint rule data"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.CreateFingerprintRequest) (any, error) {
			return s.grpcClient.CreateFingerprint(ctx, req)
		}, "failed to create fingerprint")),

		WithTool(mcp.NewTool("update_fingerprint",
			mcp.WithDescription("Update an existing fingerprint rule"),
			mcp.WithStruct("fingerprint", []mcp.PropertyOption{
				mcp.Description("Fingerprint update data"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.UpdateFingerprintRequest) (any, error) {
			return s.grpcClient.UpdateFingerprint(ctx, req)
		}, "failed to update fingerprint")),

		WithTool(mcp.NewTool("delete_fingerprint",
			mcp.WithDescription("Delete fingerprint rules by filter"),
			mcp.WithStruct("filter", []mcp.PropertyOption{
				mcp.Description("Fingerprint filter"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteFingerprintRequest) (any, error) {
			return s.grpcClient.DeleteFingerprint(ctx, req)
		}, "failed to delete fingerprint")),

		WithTool(mcp.NewTool("get_all_fingerprint_group",
			mcp.WithDescription("List all fingerprint groups"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.GetAllFingerprintGroup(ctx, &ypb.Empty{})
		}, "failed to get fingerprint groups")),

		WithTool(mcp.NewTool("create_fingerprint_group",
			mcp.WithDescription("Create a fingerprint group"),
			mcp.WithStruct("group", []mcp.PropertyOption{
				mcp.Description("Fingerprint group data"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.FingerprintGroup) (any, error) {
			return s.grpcClient.CreateFingerprintGroup(ctx, req)
		}, "failed to create fingerprint group")),

		WithTool(mcp.NewTool("rename_fingerprint_group",
			mcp.WithDescription("Rename a fingerprint group"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Rename request with old and new names"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.RenameFingerprintGroupRequest) (any, error) {
			return s.grpcClient.RenameFingerprintGroup(ctx, req)
		}, "failed to rename fingerprint group")),

		WithTool(mcp.NewTool("delete_fingerprint_group",
			mcp.WithDescription("Delete a fingerprint group"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Delete group request"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.DeleteFingerprintGroupRequest) (any, error) {
			return s.grpcClient.DeleteFingerprintGroup(ctx, req)
		}, "failed to delete fingerprint group")),

		WithTool(mcp.NewTool("batch_update_fingerprint_to_group",
			mcp.WithDescription("Batch assign fingerprints to a group"),
			mcp.WithStruct("request", []mcp.PropertyOption{
				mcp.Description("Batch update request"),
				mcp.Required(),
			}),
		), unaryToolHandler(func(ctx context.Context, s *MCPServer, req *ypb.BatchUpdateFingerprintToGroupRequest) (any, error) {
			return s.grpcClient.BatchUpdateFingerprintToGroup(ctx, req)
		}, "failed to batch update fingerprint group")),

		WithTool(mcp.NewTool("recover_builtin_fingerprint",
			mcp.WithDescription("Recover built-in fingerprint rules"),
		), unaryEmptyToolHandler(func(ctx context.Context, s *MCPServer) (any, error) {
			return s.grpcClient.RecoverBuiltinFingerprint(ctx, &ypb.Empty{})
		}, "failed to recover builtin fingerprint")),
	)
}
