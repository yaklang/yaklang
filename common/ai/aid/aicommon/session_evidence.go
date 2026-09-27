package aicommon

import (
	"fmt"
	"strings"
)

func RenderSessionEvidencePromptBlock(nonce string, evidence string) string {
	evidence = strings.ReplaceAll(strings.TrimSpace(evidence), "<|", "&lt;|")
	if evidence == "" {
		return ""
	}
	nonce = strings.TrimSpace(nonce)
	if nonce == "" {
		nonce = StablePromptNonce("session-evidence-open")
	}
	return fmt.Sprintf(
		"<|SESSION_EVIDENCE_%s|>\n## 已知观测（Evidence）\n\n以下是从真实工具执行中积累的观测结果。它们代表可能性空间中已确认的边界——哪些路径已被排除，哪些路径产生了有价值的信号。\n\n在选择下一步行动前：\n- 检查你的计划是否与已知观测矛盾（不要重复已被排除的路径）\n- 优先沿产生过非基线响应的方向迭代（信息量最大的方向）\n- 如果当前工具的控制能力不足以缩小目标差，考虑换工具或做共轭变换\n\n%s\n<|SESSION_EVIDENCE_END_%s|>",
		nonce, evidence, nonce,
	)
}
