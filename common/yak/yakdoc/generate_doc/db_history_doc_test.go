package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/webdoc"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// TestDBHistoryGeneratedDocumentation follows the complete generation and use
// path, rather than accepting nonempty comments as evidence of usable Yak docs.
func TestDBHistoryGeneratedDocumentation(t *testing.T) {
	// 1. Isolate project discovery and all executable examples from installed apps.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("YAKIT_HOME", home)
	t.Setenv("MEMFITAI_HOME", "")
	t.Setenv("TMPDIR", home)
	t.Setenv("TEMP", home)
	t.Setenv("TMP", home)
	t.Setenv("SKIP_SYNC_BUILD_IN_AI_TOOL", "true")
	require.NoError(t, consts.InitializeYakitDatabase(filepath.Join(home, "project.db"), filepath.Join(home, "profile.db"), filepath.Join(home, "ssa.db")))

	// 2. Use the same reflection/AST, overview injection, Gob and Zstd path as
	// generate_doc, then inspect the decoded artifact consumed by the engine.
	helper := yak.EngineToDocumentHelperWithVerboseInfo(yaklang.New())
	injectOverviewShort(helper)
	var buf bytes.Buffer
	require.NoError(t, gob.NewEncoder(&buf).Encode(helper))
	compressed, err := utils.ZstdCompress(buf.Bytes())
	require.NoError(t, err)
	decoded, err := utils.ZstdDeCompress(compressed)
	require.NoError(t, err)
	var artifact *yakdoc.DocumentHelper
	require.NoError(t, gob.NewDecoder(bytes.NewReader(decoded)).Decode(&artifact))

	// 3. Assert exported names, actual parameter names and result counts. An
	// export alias or optional-argument change must not silently lose its docs.
	lib := artifact.Libs["db"]
	require.NotNil(t, lib)
	require.NotEmpty(t, lib.OverviewShort)
	selected := &yakdoc.ScriptLib{Name: "db", Functions: map[string]*yakdoc.FuncDecl{}}
	shapes := []struct {
		name    string
		params  []string
		results int
	}{
		{"ListYakProjects", []string{"opts"}, 2},
		{"QueryHTTPFlows", []string{"opts"}, 2},
		{"QueryHTTPFlowByID", []string{"id", "opts"}, 2},
		{"projectID", []string{"id"}, 1},
		{"keyword", []string{"s"}, 1},
		{"url", []string{"s"}, 1},
		{"methods", []string{"s"}, 1},
		{"statusCode", []string{"s"}, 1},
		{"sourceType", []string{"s"}, 1},
		{"limit", []string{"n"}, 1},
		{"offset", []string{"n"}, 1},
		{"packetLimit", []string{"n"}, 1},
		{"afterID", []string{"n"}, 1},
		{"beforeID", []string{"n"}, 1},
	}
	for _, shape := range shapes {
		decl := lib.Functions[shape.name]
		require.NotNil(t, decl, shape.name)
		require.Equal(t, shape.name, decl.MethodName)
		require.NotEmpty(t, decl.Decl, shape.name)
		require.NotEmpty(t, decl.VSCodeSnippets, shape.name)
		require.Len(t, decl.Params, len(shape.params), shape.name)
		for i, name := range shape.params {
			require.Equal(t, name, decl.Params[i].Name, shape.name)
		}
		require.Len(t, decl.Results, shape.results, shape.name)
		selected.Functions[shape.name] = decl
	}
	require.Contains(t, selected.Functions["QueryHTTPFlowByID"].Decl, "...")
	require.Equal(t, "*schema.HTTPFlow", selected.Functions["QueryHTTPFlowByID"].Results[0].Type)

	// 4. Check every new result method too: the generator follows exported
	// return types, and these methods are how scripts display/export packets.
	libs := map[string]*yakdoc.ScriptLib{"db": selected}
	for name, methods := range map[string][]string{
		"YakProject": {"Dump"}, "HTTPHistoryPage": {"Dump"},
		"HTTPHistoryItem": {"Dump", "ExportPackets"}, "HTTPPacketFile": {"Dump"},
	} {
		key := "github.com/yaklang/yaklang/common/yak/yaklib." + name
		structLib := artifact.StructMethods[key]
		require.NotNil(t, structLib, key)
		subset := &yakdoc.ScriptLib{Name: name, Functions: map[string]*yakdoc.FuncDecl{}}
		for _, method := range methods {
			require.NotNil(t, structLib.Functions[method], key+"."+method)
			subset.Functions[method] = structLib.Functions[method]
		}
		libs[name] = subset
	}
	coverage := webdoc.CollectDocCoverage(libs)
	require.Equal(t, 19, coverage.Total)
	require.Empty(t, coverage.Gaps, "all descriptions, parameters, results and examples must be documented")

	// 5. Render the real web docs and execute every extracted example. Require
	// both old and new ID examples, so multi-example parsing cannot discard the
	// previously published save -> find ID -> exact lookup demonstration.
	options := &yakdoc.ScriptLib{Name: "db-options", Functions: map[string]*yakdoc.FuncDecl{}}
	for _, shape := range shapes[3:] {
		options.Functions[shape.name] = selected.Functions[shape.name]
	}
	libs["db-options"] = options
	examples := 0
	for name, subset := range libs {
		md := webdoc.RenderLibMarkdown(subset, "", nil)
		require.NoError(t, webdoc.CheckMarkdownInvariants(md), name)
		codes := webdoc.ExtractYakExamples(md)
		if name == "db" {
			require.Len(t, codes, 6)
			require.Contains(t, md, "doc-demo-byid.example.com")
			require.Contains(t, md, "doc-http-export.example.test")
		}
		for _, code := range codes {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			err := yaklang.New().SafeEval(ctx, code)
			cancel()
			require.NoError(t, err, "%s generated example:\n%s", name, code)
			examples++
		}
	}
	require.Equal(t, 22, examples)
	t.Logf("generated documentation: %d exports/methods; zero coverage gaps; %d real Yak examples passed", coverage.Total, examples)
}
