package loop_yaklangcode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
)

func TestSeedYaklangLoopFullCode_PrefersDiskInEditMode(t *testing.T) {
	dir := t.TempDir()
	yakPath := filepath.Join(dir, "demo.yak")
	fullDisk := "header\nbody\nurlDecoded, err := codec.DecodeUrl(urlEncoded)\ndie(err)\nyakit.Info(\"old\")\nfooter"
	require.NoError(t, os.WriteFile(yakPath, []byte(fullDisk), 0o644))

	selection := `{"path":"` + filepath.ToSlash(yakPath) + `","startLine":3,"endLine":5,"language":"yak","content":"urlDecoded, err := codec.DecodeUrl(urlEncoded)\ndie(err)\nyakit.Info(\"old\")"}`
	ctx := aicommon.ParseYaklangEditorContextFromAttached([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeFile, aicommon.YaklangAttachedResourceKeyEditorFile, yakPath),
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, selection),
	})

	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("test", runtime)
	require.NoError(t, err)
	loop.Set("editor_file_path", yakPath)

	seedYaklangLoopFullCode(loop, ctx, fullDisk)

	assert.Equal(t, fullDisk, loop.Get("full_code"))
	assert.Equal(t, 0, loop.GetInt(loopinfra.LoopVarCodeLineBase))
}

func TestSeedYaklangLoopFullCode_SelectionFallbackSetsLineBase(t *testing.T) {
	yakPath := filepath.Join(t.TempDir(), "unsaved.yak")
	selection := `{"path":"` + filepath.ToSlash(yakPath) + `","startLine":28,"endLine":30,"language":"yak","content":"a\nb\nc"}`
	ctx := aicommon.ParseYaklangEditorContextFromAttached([]*aicommon.AttachedResource{
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeFile, aicommon.YaklangAttachedResourceKeyEditorFile, yakPath),
		aicommon.NewAttachedResource(aicommon.AttachedResourceTypeSelected, aicommon.AttachedResourceKeyContent, selection),
	})

	runtime := mock.NewMockInvoker(context.Background())
	loop, err := reactloops.NewReActLoop("test", runtime)
	require.NoError(t, err)
	loop.Set("editor_file_path", yakPath)

	seedYaklangLoopFullCode(loop, ctx, "")

	assert.Equal(t, "a\nb\nc", loop.Get("full_code"))
	assert.Equal(t, 27, loop.GetInt(loopinfra.LoopVarCodeLineBase))
}
