package loop_yaklangcode

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

// 与 AIInputEvent.AttachedResourceInfo 约定（Yaklang 代码生成 / 编辑器上下文）：
//
// 通用编辑器上下文（与 code_security_audit 等不同，后者使用领域专用 key）：
//   - Type=file, Key=directory_path, Value=当前打开的工作区目录绝对路径
//   - Type=file, Key=file_path, Value=当前打开文件绝对路径
//
// 代码选区（Yak Runner chip）：
//   - Type=selected, Key=content, Value=AttachedCodeSelection JSON（含 path/content/行号）
//
// 参考其他 loop：
//   - code_security_audit: Type=file, Key=code_audit_target_path（扫描根目录，领域专用）
//   - knowledge_enhance:   Type=file, Value=任意引用文件（不区分 key）
//   - default:               Type=file, Key=file_path（图片等文件路径）
//
// 前端应与 aicommon 常量保持字符串一致，勿手写魔法串。

const (
	AttachedResourceTypeFile = aicommon.CONTEXT_PROVIDER_TYPE_FILE

	// AttachedResourceKeyWorkspaceDirectory 当前工作区目录。
	AttachedResourceKeyWorkspaceDirectory = aicommon.CONTEXT_PROVIDER_KEY_DIRECTORY_PATH

	// AttachedResourceKeyEditorFile 当前打开文件。
	AttachedResourceKeyEditorFile = aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH
)
