package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeConcreteEvidenceMarkdown(t *testing.T) {
	t.Run("accepts concrete evidence", func(t *testing.T) {
		content, err := NormalizeConcreteEvidenceMarkdown("## 攻击面\n- SQL 注入端点: /user/id, /user/post/name\n- XSS 端点: /xss/echo, /xss/js/in-str")
		require.NoError(t, err)
		require.Contains(t, content, "/user/id")
	})

	t.Run("rejects vague evidence wording", func(t *testing.T) {
		_, err := NormalizeConcreteEvidenceMarkdown("## 攻击面\n- SQL 注入端点: /user/id, /user/post/name 等")
		require.Error(t, err)
	})
}