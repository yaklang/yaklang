package sfcompletion

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"os"
	"testing"
)

func TestAAA(t *testing.T) {
	fileName := `D:\GoProject\yaklang\common\syntaxflow\sfbuildin\buildin\java\cwe-22-path-travel\java-path-travel.sf`
	content, err := os.ReadFile(fileName)
	require.NoError(t, err)

	aiOpts := []aispec.AIConfigOption{
		aispec.WithContext(context.Background()),
		aispec.WithType("siliconflow"),
		aispec.WithAPIKey("sk-djmgfawmiwkrhpsjeclicniyisjwqwvgageymmkgkramynmp"),
		aispec.WithModel("deepseek-ai/DeepSeek-R1"),
	}
	result, err := CompleteInfoDesc(fileName, string(content), aiOpts...)
	require.NoError(t, err)
	t.Log(result)
}
