package aicommon

import (
	"hash/fnv"
	"strings"
)

// stablePromptNonceLength 是 AITag nonce 的固定长度。
//
// AITag nonce 长度需要与既有 RandStringBytes(6) 反模式产出的长度保持一致，
// 这样 prompt 模板的字面量长度也保持稳定，方便 prefix cache 比对。
//
// 关键词: stable nonce, AITag nonce, prefix cache
const stablePromptNonceLength = 6

// stablePromptNonceAlphabet 是 AITag nonce 允许使用的字符集 ([a-z0-9])。
//
// AITag 解析器只接受 word 字符；为了与历史 RandStringBytes(6) 产出兼容，这里
// 选用全小写字母 + 数字的 36 字符表。
//
// 关键词: stable nonce alphabet, AITag 兼容字符集
var stablePromptNonceAlphabet = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

// StablePromptNonce 把任意若干字符串 part 哈希成跨调用稳定的 6 字符 nonce。
//
// 使用场景:
//   1) 任何在 prompt 中以 <|TAG_<nonce>|>...<|TAG_END_<nonce>|> 形式出现的
//      AITag, 如果该段内容跨多次调用字节稳定, 都应该用 StablePromptNonce
//      派生 nonce, 替代 utils.RandStringBytes(6) 这类反模式。
//   2) 派生时把语义明确的 part (rootTaskID / planEpoch / "PARENT_TASK" 等)
//      作为参数传入, 不同 part 组合自然产出不同 nonce 而互不冲突, 不需要
//      手工维护命名空间。
//
// 同一组 part 反复调用必返回相同 nonce; part 顺序敏感, "a","b" 与 "b","a"
// 派生的 nonce 不同。
//
// 关键词: StablePromptNonce, AITag stable nonce, prefix cache, 反 RandStringBytes
func StablePromptNonce(parts ...string) string {
	if len(parts) == 0 {
		return strings.Repeat("0", stablePromptNonceLength)
	}
	h := fnv.New64a()
	for i, p := range parts {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = h.Write([]byte(p))
	}
	v := h.Sum64()
	out := make([]byte, stablePromptNonceLength)
	n := uint64(len(stablePromptNonceAlphabet))
	for i := range out {
		out[i] = stablePromptNonceAlphabet[v%n]
		v /= n
	}
	return string(out)
}

// CacheableUserInputProvider 让具体的 task 类型可以把"用户输入"再细分为
// "本 turn 真正可变的部分 (rawQuery)" 和 "整个 plan 周期内可冻结的部分
// (frozenUserContext, 例如 PARENT_TASK / CURRENT_TASK / INSTRUCTION 三联块)",
// 配合 frozen-block 段把后者搬出 dynamic 段实现 prefix cache 命中。
//
// rawQuery: 当前 turn 的用户原始输入或当前任务的轻量 query, 进入 dynamic 段
//   USER_QUERY 块。
// frozenUserContext: 跨同一 plan 周期所有子任务字节稳定的上下文块, 包装成
//   PLAN_CONTEXT 后注入 frozen-block (Tool/Forge 之后, Timeline 之前)。
//
// 老路径 (普通 ReAct loop / focus mode 等没有 plan 上下文的场景) 不实现该
// 接口, 框架自动 fallback 到原始 task.GetUserInput() 语义, 行为保持不变。
//
// 关键词: CacheableUserInputProvider, PE-TASK PLAN 产物冻结,
//        frozenUserContext, prefix cache
type CacheableUserInputProvider interface {
	GetUserInputSplitForCache() (rawQuery, frozenUserContext string)
}

// PlanScopedNonce 用 root task 的稳定标识符派生"整个 plan 周期共用"的 nonce。
//
// 语义:
//   - 同一个 plan 周期 (root task 没有改变) 内, 所有 PE-TASK 子任务共用一个
//     plan-scoped nonce, 让 PARENT_TASK / CURRENT_TASK / INSTRUCTION 等
//     标签的 nonce 跨子任务调用字节稳定, 进入 prefix cache。
//   - plan 重新生成 (root task 改变 / 新生成) 时, rootIdentifier 自然变化,
//     plan-scoped nonce 随之变化, 旧 prefix cache 自然失效, 不会污染新 plan。
//
// salt 让同一个 plan 周期内派生多个互不冲突的 nonce (如 "user_query" /
// "facts" / "document"), 不需要在调用方手工维护 nonce 命名空间。
//
// 关键词: PlanScopedNonce, plan epoch nonce, prefix cache, PE-TASK 缓存稳定
func PlanScopedNonce(rootIdentifier string, salt string) string {
	rootIdentifier = strings.TrimSpace(rootIdentifier)
	if rootIdentifier == "" {
		rootIdentifier = "anonymous-root"
	}
	return StablePromptNonce("plan-scope", rootIdentifier, salt)
}
