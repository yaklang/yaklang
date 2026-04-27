package loop_code_security_audit

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// 与 AIInputEvent.AttachedResourceInfo 约定（代码安全审计专用）：
// - Type 必须为 AttachedResourceTypeFile（与 aicommon.CONTEXT_PROVIDER_TYPE_FILE 一致，即 "file"）
// - Key 必须为 AttachedResourceKeyCodeAuditTargetPath
// - Value 为待扫描项目根目录的绝对路径（须存在且为目录）
//
// 前端应与下列 const 保持字符串一致，勿手写魔法串。

// AttachedResourceTypeFile 对应 AttachedResourceInfo.Type，须为内置 file 类型。
const AttachedResourceTypeFile = aicommon.CONTEXT_PROVIDER_TYPE_FILE

// AttachedResourceKeyCodeAuditTargetPath 对应 AttachedResourceInfo.Key；Value 为项目根目录绝对路径。
const AttachedResourceKeyCodeAuditTargetPath = "code_audit_target_path"

// scanTargetPathFromTask 从主任务附带的 AttachedResource 中解析扫描根目录；无匹配时返回 ""。
func scanTargetPathFromTask(task aicommon.AIStatefulTask) string {
	if task == nil {
		return ""
	}
	for _, a := range task.GetAttachedDatas() {
		if a == nil {
			continue
		}
		if a.Type != AttachedResourceTypeFile {
			continue
		}
		if a.Key != AttachedResourceKeyCodeAuditTargetPath {
			continue
		}
		p := strings.TrimSpace(a.Value)
		if p == "" {
			continue
		}
		return filepath.Clean(p)
	}
	return ""
}
