package reactinit_test

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactinit"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
)

// essential-tests.yml runs aireact/... including this contract. Inspect raw
// registrations, not the production deduplicated catalog or a developer's DB.
func TestFocusActionAndBuiltinToolNames(t *testing.T) {
	sources := map[string]string{}
	var names []string
	add := func(name, source string) {
		t.Helper()
		if previous, exists := sources[name]; exists {
			t.Errorf("tool name %q is registered by both %s and %s", name, previous, source)
		}
		sources[name] = source
		names = append(names, name)
	}
	for _, tool := range buildinaitools.GetBuiltinGoTools() {
		add(tool.GetName(), "Go builtin")
	}
	require.NoError(t, fs.WalkDir(yakscripttools.GetEmbedFS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yak") {
			add(strings.TrimSuffix(entry.Name(), ".yak"), path)
		}
		return nil
	}))
	require.NotEmpty(t, names)
	metadata := reactloops.GetAllLoopMetadata()
	require.NotEmpty(t, metadata)
	for _, native := range []bool{false, true} {
		runtime, err := aireact.NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithEnableFunctionCallMode(native))
		require.NoError(t, err)
		for _, meta := range metadata {
			t.Run(fmt.Sprintf("%s/native=%t", meta.Name, native), func(t *testing.T) {
				var opts []reactloops.ReActLoopOption
				if meta.Name == coordinator.Name {
					controller := coordinator.New(context.Background(), nil, 1)
					defer controller.Close()
					opts = append(opts, coordinator.WithController(controller))
				}
				loop, err := reactloops.CreateLoopByName(meta.Name, runtime, opts...)
				require.NoError(t, err)
				defer loop.Release()
				require.Equal(t, native, loop.FunctionCallModeEnabled(), "focus modes must inherit session protocol")
				require.NoError(t, reactloops.ValidateActionToolNames(loop.GetAllActions(), names))
			})
		}
	}
}
