package sfbuildin

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

type compiledBuiltinFixture struct {
	path string
	rule *schema.SyntaxFlowRule
}

// Compile the complete, immutable embedded set once for read-only metadata
// checks. The cold database sync tests deliberately do not use this snapshot.
var compiledBuiltinFixtures = sync.OnceValues(func() ([]compiledBuiltinFixture, error) {
	var fixtures []compiledBuiltinFixture
	var failures []error
	err := filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if !strings.HasSuffix(info.Name(), ".sf") {
			return nil
		}
		raw, err := ruleFSWithHash.ReadFile(path)
		if err != nil {
			return err
		}
		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: compile: %w", path, err))
			return nil
		}
		fixtures = append(fixtures, compiledBuiltinFixture{path: path, rule: rule})
		return nil
	}))
	return fixtures, errors.Join(append(failures, err)...)
})

func builtinFixtures(t *testing.T) []compiledBuiltinFixture {
	t.Helper()
	fixtures, err := compiledBuiltinFixtures()
	require.NoError(t, err, "every embedded rule must compile")
	require.NotEmpty(t, fixtures)
	return fixtures
}
