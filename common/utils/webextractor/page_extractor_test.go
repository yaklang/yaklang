package webextractor

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestRodExtractor_ExtractFromURL(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip("Skip test in Github Actions")
	}

	content, err := ExtractPageRod("https://cloud.tencent.com/developer/article/2233629")
	require.NoError(t, err)
	fmt.Println(content)
}
