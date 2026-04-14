package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeConcreteEvidenceMarkdown(t *testing.T) {
	t.Run("normalizes line endings and trims", func(t *testing.T) {
		content := NormalizeConcreteEvidenceMarkdown("\r\n## 攻击面\r\n- SQL 注入端点: /user/id, /user/post/name\r\n")
		require.Contains(t, content, "/user/id")
		require.NotContains(t, content, "\r")
	})

	t.Run("preserves original evidence wording", func(t *testing.T) {
		content := NormalizeConcreteEvidenceMarkdown("## 攻击面\n- SQL 注入端点: /user/id, /user/post/name 等")
		require.Contains(t, content, "等")
	})
}