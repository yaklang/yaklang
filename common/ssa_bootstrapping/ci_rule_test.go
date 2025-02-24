package ssa_bootstrapping

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
)

//go:embed ci_rule/**
var ciRules embed.FS

func TestCiRule(t *testing.T) {
	var checkDir func()
	var checkFile func(fs.DirEntry)
	path := []string{"ci_rule"}

	checkDir = func() {
		name := strings.Join(path, "/")
		dir, err := ciRules.ReadDir(name)
		require.NoError(t, err)
		for _, entry := range dir {
			checkFile(entry)
		}
	}

	checkFile = func(entry fs.DirEntry) {
		name := strings.Join(path, "/")
		raw, err2 := ciRules.ReadFile(fmt.Sprintf("%s/%s", name, entry.Name()))
		if err2 != nil {
			path = append(path, entry.Name())
			checkDir()
			path = path[:len(path)-1]
		} else {
			// fmt.Printf("%s\n%s\n\n", name, raw)
			_, err2 = sfdb.CheckSyntaxFlowRuleContent(string(raw))
			require.NoError(t, err2)
		}
	}

	checkDir()

}
