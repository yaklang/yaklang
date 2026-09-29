package aicommon

// 关键词: aiprojection, init wiring, 发送前 hook 自动注册
//
// 通过空白 import 触发 aiprojection 包的 init()，
// 让它把 ProjectAndObserve 注册到 aispec.ChatBase 的发送前 hijack hook。
// aicommon 是所有 aid 业务的通用基础，几乎所有 ai 调用链都会加载它，
// 因此只在这里挂一次 import 就能保证 aiprojection 的注册时机覆盖整个 aid 系统。
// aiprojection 自身不依赖 aicommon 主包（只依赖 aicommon/aitag 子包），不会形成循环。
import _ "github.com/yaklang/yaklang/common/ai/aid/aiprojection"
