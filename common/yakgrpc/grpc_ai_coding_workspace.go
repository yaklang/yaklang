package yakgrpc

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/coding"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func codingWorkspaceProto(item *schema.AICodingWorkspace) *ypb.AICodingWorkspace {
	roots := item.RootPaths
	if len(roots) == 0 {
		roots = []string{item.RootPath}
	}
	return &ypb.AICodingWorkspace{ID: item.WorkspaceID, Name: item.Name, RootPath: item.RootPath, RootPaths: roots, IsGit: item.IsGit}
}

func (s *Server) codingWorktreeProto(ctx context.Context, item *schema.AICodingWorktree) (*ypb.AICodingWorktree, error) {
	result := &ypb.AICodingWorktree{ID: item.WorktreeID, WorkspaceID: item.WorkspaceID, OwnerSessionID: item.OwnerSessionID,
		Path: item.Path, SourceDirectory: item.SourceDirectory, RepositoryRoot: item.RepositoryRoot, GitCommonDir: item.GitCommonDir,
		WorkingDirectory: item.WorkingDirectory, BaseRef: item.BaseRef, BaseCommit: item.BaseCommit, State: item.State, LastError: item.LastError}
	var sessions []*schema.AICodingSession
	if err := s.GetProfileDatabase().Where("worktree_id = ?", item.WorktreeID).Order("id asc").Find(&sessions).Error; err != nil {
		return nil, err
	}
	for _, session := range sessions {
		result.SessionIDs = append(result.SessionIDs, session.SessionID)
	}
	if item.State == "ready" {
		live, err := coding.InspectWorktree(ctx, item.Path, item.GitCommonDir, item.WorkingDirectory)
		if err != nil {
			result.State, result.LastError = "unavailable", err.Error()
		} else {
			result.Head, result.Branch, result.Detached = live.Head, live.Branch, live.Detached
			result.Locked, result.Prunable, result.Bare = live.Locked, live.Prunable, live.Bare
		}
	}
	return result, nil
}

func (s *Server) CreateAICodingWorkspace(ctx context.Context, req *ypb.CreateAICodingWorkspaceRequest) (*ypb.AICodingWorkspace, error) {
	roots := req.GetRootPaths()
	if len(roots) == 0 {
		roots = []string{req.GetRootPath()}
	}
	item, err := yakit.CreateAICodingWorkspace(ctx, s.GetProfileDatabase(), req.GetName(), roots...)
	if err != nil {
		return nil, err
	}
	return codingWorkspaceProto(item), nil
}

func (s *Server) UpdateAICodingWorkspace(ctx context.Context, req *ypb.UpdateAICodingWorkspaceRequest) (*ypb.AICodingWorkspace, error) {
	item, err := yakit.UpdateAICodingWorkspace(ctx, s.GetProfileDatabase(), req.GetWorkspaceID(), req.GetName(), req.GetRootPaths())
	if err != nil {
		return nil, err
	}
	return codingWorkspaceProto(item), nil
}

func (s *Server) GetAICodingWorkspace(ctx context.Context, req *ypb.AICodingWorkspaceRequest) (*ypb.AICodingWorkspace, error) {
	item, err := yakit.GetAICodingWorkspace(s.GetProfileDatabase(), req.GetWorkspaceID())
	if err != nil {
		return nil, err
	}
	return codingWorkspaceProto(item), nil
}

func (s *Server) QueryAICodingWorkspaces(ctx context.Context, req *ypb.Empty) (*ypb.QueryAICodingWorkspacesResponse, error) {
	var items []*schema.AICodingWorkspace
	if err := s.GetProfileDatabase().Order("id desc").Find(&items).Error; err != nil {
		return nil, err
	}
	result := &ypb.QueryAICodingWorkspacesResponse{}
	for _, item := range items {
		result.Data = append(result.Data, codingWorkspaceProto(item))
	}
	return result, nil
}

func (s *Server) ListAICodingWorktrees(ctx context.Context, req *ypb.AICodingWorkspaceRequest) (*ypb.ListAICodingWorktreesResponse, error) {
	if _, err := yakit.GetAICodingWorkspace(s.GetProfileDatabase(), req.GetWorkspaceID()); err != nil {
		return nil, err
	}
	trees, err := yakit.ListAICodingWorktrees(s.GetProfileDatabase(), req.GetWorkspaceID())
	if err != nil {
		return nil, err
	}
	result := &ypb.ListAICodingWorktreesResponse{}
	for _, tree := range trees {
		item, err := s.codingWorktreeProto(ctx, tree)
		if err != nil {
			return nil, err
		}
		result.Data = append(result.Data, item)
	}
	return result, nil
}

func (s *Server) GetAICodingWorktree(ctx context.Context, req *ypb.GetAICodingWorktreeRequest) (*ypb.AICodingWorktree, error) {
	item, err := yakit.GetAICodingWorktree(s.GetProfileDatabase(), req.GetWorktreeID(), req.GetPath())
	if err != nil {
		return nil, err
	}
	return s.codingWorktreeProto(ctx, item)
}

func (s *Server) ListAICodingSessions(ctx context.Context, req *ypb.AICodingWorkspaceRequest) (*ypb.ListAICodingSessionsResponse, error) {
	if _, err := yakit.GetAICodingWorkspace(s.GetProfileDatabase(), req.GetWorkspaceID()); err != nil {
		return nil, err
	}
	items, err := yakit.ListAICodingSessions(s.GetProfileDatabase(), req.GetWorkspaceID())
	if err != nil {
		return nil, err
	}
	result := &ypb.ListAICodingSessionsResponse{}
	for _, item := range items {
		result.Data = append(result.Data, &ypb.AICodingSession{SessionID: item.SessionID, WorkspaceID: item.WorkspaceID,
			WorktreeID: item.WorktreeID, WorkingDirectory: item.WorkingDirectory, SourceRoots: item.SourceRoots, WorkspaceRoots: item.WorkspaceRoots})
	}
	return result, nil
}

func (s *Server) CreateAICodingWorktree(ctx context.Context, req *ypb.AICodingWorkspaceRequest) (*ypb.AICodingWorktree, error) {
	return nil, grpcstatus.Error(codes.FailedPrecondition, "worktrees are created at session startup; set AIStartParams.CodingCreateWorktree")
}

func (s *Server) ListAICodingBranches(ctx context.Context, req *ypb.AICodingWorkspaceRequest) (*ypb.ListAICodingBranchesResponse, error) {
	workspace, err := yakit.GetAICodingWorkspace(s.GetProfileDatabase(), req.GetWorkspaceID())
	if err != nil {
		return nil, err
	}
	branches, err := coding.ListBranches(ctx, workspace.RootPath)
	if err != nil {
		return nil, err
	}
	result := &ypb.ListAICodingBranchesResponse{CurrentBranch: branches.CurrentBranch,
		CurrentCommit: branches.CurrentCommit, DefaultRef: branches.DefaultRef, HasLocalChanges: branches.HasLocalChanges}
	for _, branch := range branches.Data {
		result.Data = append(result.Data, &ypb.AICodingBranch{Name: branch.Name, Ref: branch.Ref, Commit: branch.Commit, Remote: branch.Remote})
	}
	return result, nil
}
