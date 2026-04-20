package yakgrpc

import (
	"archive/zip"
	"context"
	"errors"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func queryForge(ctx context.Context, client ypb.YakClient, filter *ypb.AIForgeFilter) ([]*ypb.AIForge, error) {
	resp, err := client.QueryAIForge(ctx, &ypb.QueryAIForgeRequest{
		Pagination: &ypb.Paging{},
		Filter:     filter,
	})
	return resp.GetData(), err
}

func waitAIForgeExportDone(t *testing.T, stream ypb.Yak_ExportAIForgeClient) {
	t.Helper()

	for {
		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		require.NoError(t, err)
	}
}

func TestGRPCMUSTPASS_AIForge_BaseCRUD(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	name := uuid.New().String()
	content := uuid.New().String()

	_, err = client.CreateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: content,
	})
	require.NoError(t, err)

	forge, err := queryForge(ctx, client, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.Len(t, forge, 1)
	require.Equal(t, name, forge[0].ForgeName)
	require.Equal(t, content, forge[0].ForgeContent)
	require.Greater(t, forge[0].GetCreatedAt(), int64(0))
	require.Greater(t, forge[0].GetUpdatedAt(), int64(0))

	forge, err = queryForge(ctx, client, &ypb.AIForgeFilter{
		Keyword: content,
	})
	require.NoError(t, err)
	require.Len(t, forge, 1)
	require.Equal(t, name, forge[0].ForgeName)
	require.Equal(t, content, forge[0].ForgeContent)
	require.Greater(t, forge[0].GetCreatedAt(), int64(0))
	require.Greater(t, forge[0].GetUpdatedAt(), int64(0))

	newContent := uuid.New().String()
	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: newContent,
	})
	require.NoError(t, err)

	forge, err = queryForge(ctx, client, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.Len(t, forge, 1)
	require.Equal(t, name, forge[0].ForgeName)
	require.Equal(t, newContent, forge[0].ForgeContent)
	require.Greater(t, forge[0].GetCreatedAt(), int64(0))
	require.Greater(t, forge[0].GetUpdatedAt(), int64(0))

	newContent = uuid.New().String()
	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		Id:           forge[0].GetId(),
		ForgeName:    name,
		ForgeContent: newContent,
	})
	require.NoError(t, err)

	forge, err = queryForge(ctx, client, &ypb.AIForgeFilter{
		Id: forge[0].GetId(),
	})
	require.NoError(t, err)
	require.Len(t, forge, 1)
	require.Equal(t, name, forge[0].ForgeName)
	require.Equal(t, newContent, forge[0].ForgeContent)
	require.Greater(t, forge[0].GetCreatedAt(), int64(0))
	require.Greater(t, forge[0].GetUpdatedAt(), int64(0))

	_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)

	forge, err = queryForge(ctx, client, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.Len(t, forge, 0)
}

func TestGRPCMUSTPASS_AIForge_AuthorAndTimeFields(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := uuid.New().String()
	content := uuid.New().String()
	author := "alice"

	_, err = client.CreateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeType:    "yak",
		ForgeContent: content,
		Author:       author,
	})
	require.NoError(t, err)
	defer func() {
		_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{ForgeName: name})
		require.NoError(t, err)
	}()

	created, err := client.GetAIForge(ctx, &ypb.GetAIForgeRequest{ForgeName: name})
	require.NoError(t, err)
	require.Equal(t, author, created.GetAuthor())
	require.Greater(t, created.GetCreatedAt(), int64(0))
	require.Greater(t, created.GetUpdatedAt(), int64(0))

	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeType:    "yak",
		ForgeContent: "updated-content",
	})
	require.NoError(t, err)

	updated, err := client.GetAIForge(ctx, &ypb.GetAIForgeRequest{ForgeName: name})
	require.NoError(t, err)
	require.Equal(t, author, updated.GetAuthor())
	require.Equal(t, created.GetCreatedAt(), updated.GetCreatedAt())
	require.GreaterOrEqual(t, updated.GetUpdatedAt(), created.GetUpdatedAt())

	list, err := queryForge(ctx, client, &ypb.AIForgeFilter{ForgeName: name})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, author, list[0].GetAuthor())
	require.Equal(t, updated.GetCreatedAt(), list[0].GetCreatedAt())
	require.Equal(t, updated.GetUpdatedAt(), list[0].GetUpdatedAt())
}

func TestGRPCMUSTPASS_AIForge_GetByName(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := uuid.New().String()
	content := uuid.New().String()

	_, err = client.CreateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: content,
	})
	require.NoError(t, err)

	// Test GetAIForge by name
	forge, err := client.GetAIForge(ctx, &ypb.GetAIForgeRequest{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.NotNil(t, forge)
	require.Equal(t, name, forge.ForgeName)
	require.Equal(t, content, forge.ForgeContent)

	// Clean up
	_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{
		ForgeName: name,
	})
	require.NoError(t, err)
}

func TestGRPCMUSTPASS_AIForge_UpdateWithZeroField(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := uuid.New().String()
	content := uuid.New().String()

	forgeIns := &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: content,
	}
	_, err = client.CreateAIForge(ctx, forgeIns)
	require.NoError(t, err)
	defer func() {
		_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{
			ForgeName: name,
		})
		require.NoError(t, err)
	}()
	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeContent: "",
	})
	require.NoError(t, err)

	forge, err := client.GetAIForge(ctx, &ypb.GetAIForgeRequest{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.NotNil(t, forge)
	require.Equal(t, name, forge.ForgeName)
	require.Equal(t, "", forge.ForgeContent)
}

func TestGRPCMUSTPASS_AIForge_UpdateEmptyFieldsOverrideMetadata(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := uuid.New().String()
	content := `__DESC__ = "meta desc"
__KEYWORDS__ = "meta1,meta2"
__VERBOSE_NAME__ = "Meta Verbose"
query = cli.String("query", cli.setRequired(true))`

	_, err = client.CreateAIForge(ctx, &ypb.AIForge{
		ForgeName:        name,
		ForgeType:        "yak",
		ForgeContent:     content,
		Description:      "explicit desc",
		ForgeVerboseName: "Explicit Verbose",
		ToolKeywords:     []string{"explicit"},
		Tag:              []string{"explicit-tag"},
	})
	require.NoError(t, err)
	defer func() {
		_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{
			ForgeName: name,
		})
		require.NoError(t, err)
	}()

	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		ForgeName:        name,
		ForgeType:        "yak",
		ForgeContent:     content,
		Description:      "",
		ForgeVerboseName: "",
		ToolKeywords:     nil,
		Tag:              nil,
	})
	require.NoError(t, err)

	forge, err := client.GetAIForge(ctx, &ypb.GetAIForgeRequest{
		ForgeName: name,
	})
	require.NoError(t, err)
	require.NotNil(t, forge)
	require.Equal(t, "", forge.Description)
	require.Equal(t, "", forge.ForgeVerboseName)
	require.Len(t, forge.ToolKeywords, 0)
	require.Len(t, forge.Tag, 0)
}

func TestGRPCMUSTPASS_AIForge_SkillPathRoundTrip(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := uuid.New().String()
	skillDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "scripts", "helper.py"), []byte("print('hello')"), 0o644))

	_, err = client.CreateAIForge(ctx, &ypb.AIForge{
		ForgeName: name,
		ForgeType: "skillmd",
		SkillPath: skillDir,
	})
	require.NoError(t, err)
	defer func() {
		_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{ForgeName: name})
		require.NoError(t, err)
	}()

	forge, err := client.GetAIForge(ctx, &ypb.GetAIForgeRequest{ForgeName: name})
	require.NoError(t, err)
	require.Equal(t, skillDir, forge.GetSkillPath())

	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "scripts", "helper.py"), []byte("print('stale')"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "stale.txt"), []byte("stale"), 0o644))

	forge, err = client.GetAIForge(ctx, &ypb.GetAIForgeRequest{ForgeName: name, InflateSkillPath: true})
	require.NoError(t, err)
	require.Equal(t, skillDir, forge.GetSkillPath())
	content, err := os.ReadFile(filepath.Join(forge.GetSkillPath(), "scripts", "helper.py"))
	require.NoError(t, err)
	require.Equal(t, "print('hello')", string(content))
	_, err = os.Stat(filepath.Join(forge.GetSkillPath(), "stale.txt"))
	require.True(t, os.IsNotExist(err))

	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		ForgeName:    name,
		ForgeType:    "skillmd",
		Description:  "updated",
		ForgeContent: "",
	})
	require.NoError(t, err)

	forge, err = client.GetAIForge(ctx, &ypb.GetAIForgeRequest{ForgeName: name})
	require.NoError(t, err)
	require.Equal(t, skillDir, forge.GetSkillPath())

	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "scripts", "helper.py"), []byte("print('stale again')"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "stale.txt"), []byte("stale-again"), 0o644))

	forge, err = client.GetAIForge(ctx, &ypb.GetAIForgeRequest{ForgeName: name, InflateSkillPath: true})
	require.NoError(t, err)
	require.Equal(t, skillDir, forge.GetSkillPath())
	content, err = os.ReadFile(filepath.Join(forge.GetSkillPath(), "scripts", "helper.py"))
	require.NoError(t, err)
	require.Equal(t, "print('hello')", string(content))
	_, err = os.Stat(filepath.Join(forge.GetSkillPath(), "stale.txt"))
	require.True(t, os.IsNotExist(err))
}

func TestGRPCMUSTPASS_AIForge_SkillPathSaveSyncsSkillMD(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	name := uuid.New().String()
	skillDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "scripts", "helper.py"), []byte("print('hello')"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: stale-skill
description: stale description
---
stale body
`), 0o644))

	_, err = client.CreateAIForge(ctx, &ypb.AIForge{
		ForgeName:   name,
		ForgeType:   "skillmd",
		SkillPath:   skillDir,
		Description: "Generated description",
		Tag:         []string{"category:automation", "owner:platform"},
		InitPrompt:  "Generated body",
	})
	require.NoError(t, err)
	defer func() {
		_, err = client.DeleteAIForge(ctx, &ypb.AIForgeFilter{ForgeName: name})
		require.NoError(t, err)
	}()

	skillMDContent, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	require.NoError(t, err)
	meta, err := aiskillloader.ParseSkillMeta(string(skillMDContent))
	require.NoError(t, err)
	require.Equal(t, name, meta.Name)
	require.Equal(t, "Generated description", meta.Description)
	require.Equal(t, "Generated body", meta.Body)
	require.Equal(t, map[string]string{
		"category": "automation",
		"owner":    "platform",
	}, meta.Metadata)

	_, err = client.UpdateAIForge(ctx, &ypb.AIForge{
		ForgeName:   name,
		ForgeType:   "skillmd",
		Description: "Updated description",
		Tag:         []string{"category:analysis"},
		InitPrompt:  "Updated body",
	})
	require.NoError(t, err)

	skillMDContent, err = os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	require.NoError(t, err)
	meta, err = aiskillloader.ParseSkillMeta(string(skillMDContent))
	require.NoError(t, err)
	require.Equal(t, name, meta.Name)
	require.Equal(t, "Updated description", meta.Description)
	require.Equal(t, "Updated body", meta.Body)
	require.Equal(t, map[string]string{
		"category": "analysis",
	}, meta.Metadata)
}

func TestGRPCMUSTPASS_AIForge_ExportUsesMergedForgeNamesAndFilter(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	matchName := uuid.NewString()
	otherMatchName := uuid.NewString()
	forgeNames := []string{matchName, otherMatchName}
	for _, name := range forgeNames {
		_, err = client.CreateAIForge(ctx, &ypb.AIForge{
			ForgeName:    name,
			ForgeType:    "yak",
			ForgeContent: "println('hello')",
		})
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		yakit.DeleteAIForge(consts.GetGormProfileDatabase(), &ypb.AIForgeFilter{
			ForgeNames: forgeNames,
		})
	})

	exportPath := filepath.Join(t.TempDir(), "forge-export.zip")
	stream, err := client.ExportAIForge(ctx, &ypb.ExportAIForgeRequest{
		ForgeNames: forgeNames,
		TargetPath: exportPath,
		Filter: &ypb.AIForgeFilter{
			ForgeName: matchName,
		},
	})
	require.NoError(t, err)
	waitAIForgeExportDone(t, stream)

	t.Cleanup(func() {
		_ = os.Remove(exportPath)
	})

	reader, err := zip.OpenReader(exportPath)
	require.NoError(t, err)
	defer reader.Close()

	exportedRoots := make(map[string]struct{})
	for _, file := range reader.File {
		root := strings.SplitN(file.Name, "/", 2)[0]
		if root == "" {
			continue
		}
		exportedRoots[root] = struct{}{}
	}

	require.Contains(t, exportedRoots, matchName)
	require.Contains(t, exportedRoots, otherMatchName)
}
