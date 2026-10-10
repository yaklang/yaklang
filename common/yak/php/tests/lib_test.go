package tests

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed syntax/composer.lock
var composer string

func TestComposerJson(t *testing.T) {
	fs := filesys.NewVirtualFs()
	fs.AddFile("composer.lock", composer)
	// composer.lock is excluded by built-in default compile excludes (lock files
	// are not production source); this test asserts dependency parsing on it,
	// so disable the defaults for the full-tree behavior.
	ssatest.CheckWithFS(fs, t, func(programs ssaapi.Programs) error {
		result, err := programs.SyntaxFlowWithError("__dependency__.myclabs*.version as $version", ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		values := result.GetValues("version")
		require.True(t, len(values) == 1)
		return nil
	}, ssaapi.WithLanguage(ssaconfig.PHP), ssaapi.WithDisableDefaultCompileExcludes(true))
}
