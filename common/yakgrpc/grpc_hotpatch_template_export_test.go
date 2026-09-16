package yakgrpc

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func createHotPatchTemplateForExport(t *testing.T, client ypb.YakClient, ctx context.Context, name, content, typ string, tags []string) {
	t.Helper()
	_, err := client.CreateHotPatchTemplate(ctx, &ypb.HotPatchTemplate{
		Name:    name,
		Content: content,
		Type:    typ,
		Tags:    tags,
	})
	require.NoError(t, err)
}

func drainHotPatchExportStream(t *testing.T, stream ypb.Yak_ExportHotPatchTemplateStreamClient) {
	t.Helper()
	for {
		_, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Logf("export stream error: %v", err)
			}
			break
		}
	}
}

func drainHotPatchImportStream(t *testing.T, stream ypb.Yak_ImportHotPatchTemplateStreamClient) {
	t.Helper()
	for {
		_, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Logf("import stream error: %v", err)
			}
			break
		}
	}
}

func TestGRPCMUSTPASS_HotPatchTemplate_Export_And_Import(t *testing.T) {
	client, _, err := NewLocalClientAndServerWithTempDatabase(t)
	require.NoError(t, err)

	ctx := utils.TimeoutContextSeconds(15)
	typ := "export-test-" + uuid.NewString()

	// create a few templates with full info (name / content / type / tags)
	type tpl struct {
		name    string
		content string
		typ     string
		tags    []string
	}
	templates := []tpl{
		{name: uuid.NewString(), content: "content-1-" + uuid.NewString(), typ: typ, tags: []string{"alpha", "shared"}},
		{name: uuid.NewString(), content: "content-2-" + uuid.NewString(), typ: typ, tags: []string{"beta", "shared"}},
		{name: uuid.NewString(), content: "content-3-" + uuid.NewString(), typ: typ, tags: []string{"gamma"}},
	}
	for _, tp := range templates {
		createHotPatchTemplateForExport(t, client, ctx, tp.name, tp.content, tp.typ, tp.tags)
	}
	t.Cleanup(func() {
		names := make([]string, 0, len(templates))
		for _, tp := range templates {
			names = append(names, tp.name)
		}
		_, _ = client.DeleteHotPatchTemplate(ctx, &ypb.DeleteHotPatchTemplateRequest{
			Condition: &ypb.HotPatchTemplateRequest{
				Name: names,
			},
		})
	})

	checkTemplatesRestored := func(t *testing.T) {
		queryResp, err := client.QueryHotPatchTemplate(ctx, &ypb.HotPatchTemplateRequest{
			Type: typ,
		})
		require.NoError(t, err)
		gots := queryResp.GetData()
		require.Len(t, gots, len(templates))
		gotByName := make(map[string]*ypb.HotPatchTemplate, len(gots))
		for _, g := range gots {
			gotByName[g.GetName()] = g
		}
		for _, tp := range templates {
			g, ok := gotByName[tp.name]
			require.True(t, ok, "template %s should exist after import", tp.name)
			require.Equal(t, tp.name, g.GetName())
			require.Equal(t, tp.content, g.GetContent())
			require.Equal(t, tp.typ, g.GetType())
			require.ElementsMatch(t, tp.tags, g.GetTags())
		}
	}

	exportAndImport := func(t *testing.T, password string) {
		outputDir := t.TempDir()
		exportFilename := "hotpatch_export.zip"
		exportReq := &ypb.ExportHotPatchTemplateStreamRequest{
			Filter: &ypb.HotPatchTemplateRequest{
				Type: typ,
			},
			OutputFilename:  exportFilename,
			OutputPluginDir: outputDir,
			Password:        password,
		}

		exportStream, err := client.ExportHotPatchTemplateStream(ctx, exportReq)
		require.NoError(t, err)
		drainHotPatchExportStream(t, exportStream)

		// the exported file (with optional .enc suffix)
		// OutputFilename already ends with .zip so the server won't append another .zip
		finalName := exportFilename
		if password != "" {
			finalName += ".enc"
		}
		exportedPath := filepath.Join(outputDir, finalName)
		require.FileExists(t, exportedPath)

		// delete all templates to simulate a clean import
		_, err = client.DeleteHotPatchTemplate(ctx, &ypb.DeleteHotPatchTemplateRequest{
			Condition: &ypb.HotPatchTemplateRequest{
				Type: typ,
			},
		})
		require.NoError(t, err)

		// import from the file on disk
		importStream, err := client.ImportHotPatchTemplateStream(ctx, &ypb.ImportHotPatchTemplateStreamRequest{
			Filename: exportedPath,
			Password: password,
		})
		require.NoError(t, err)
		drainHotPatchImportStream(t, importStream)

		// verify all templates (with all fields) are restored
		checkTemplatesRestored(t)
	}

	t.Run("no password", func(t *testing.T) {
		exportAndImport(t, "")
	})

	t.Run("password", func(t *testing.T) {
		// SM4 key must be 16 bytes; PKCS7Padding pads the password to 16 bytes
		// only when it is <= 16 bytes, so keep the password short.
		exportAndImport(t, "yak-test-pwd-16")
	})
}
