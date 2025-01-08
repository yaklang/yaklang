package ssa_bootstrapping

import (
	"embed"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"io/fs"
	"os"
	"strings"
	"testing"
)

//go:embed ci_rule/***
var ruleFS embed.FS

func TestSSACI(t *testing.T) {
	if !utils.InGithubActions() {
		t.Skip()
	}
	repos := os.Getenv("GIT_DIR")
	log.Infof("repos: %s", repos)
	fsInstance := filesys.NewEmbedFS(ruleFS)
	sfRules := []*schema.SyntaxFlowRule{}
	err := filesys.Recursive(".", filesys.WithFileSystem(fsInstance), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
		_, name := fsInstance.PathSplit(s)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}

		raw, err := fsInstance.ReadFile(s)
		require.NoError(t, err)

		content := string(raw)

		sfRule, err := sfdb.CheckSyntaxFlowRuleContent(content)
		require.NoError(t, err) // import builtin rule

		sfRules = append(sfRules, sfRule)
		return nil
	}))
	require.NoError(t, err)
	baseCommit, currentCommit, err := yakgit.GetHeadCommitRange(repos)
	require.NoError(t, err)

	gitFs, err := yakgit.FromCommitRange(repos, baseCommit.Hash.String(), currentCommit.Hash.String())
	require.NoError(t, err)
	progs, err := ssaapi.ParseProjectWithFS(gitFs, ssaapi.WithLanguage(ssaapi.GO))
	require.NoError(t, err)
	var errs error
	for _, rule := range sfRules {
		result, err2 := progs.SyntaxFlowRule(rule, ssaapi.QueryWithEnableDebug())
		if err2 != nil {
			errs = errors.Wrapf(errs, "syntaxflow rule execute error: %s", err2)
			continue
		}
		risks := result.GetValues("risk")
		if risks.Len() != 0 {
			log.Errorf("this pr have risk. check it. rule[%s]", rule.RuleName)
			risks.ShowWithSource(true)
			errs = errors.Wrapf(errs, "this pr have risk. rule[%s],", rule.RuleName)
		}
	}
	require.NoError(t, errs)
}
