package yakgit

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlame(t *testing.T) {
	r := getTestGitRepo(t)
	result, err := Blame(r, filepath.Join(r, "./file1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	_ = result
	fmt.Println(result.String())
	require.True(t, strings.Contains(result.String(), "184d4e3f"))
	require.True(t, strings.Contains(result.String(), "Modified content of file1"))
}

func TestBlameLocalTest(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	r := `/Users/v1ll4n/Projects/yaklang`
	result, err := Blame(r, filepath.Join(r, "common/har/har_easyjson_patch.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range result {
		fmt.Println(line.String())
	}
}
