package reactloops

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// 此文件提供 Yak 专注模式的两阶段注册入口：
//
//  1. boot 期：从 *.ai-focus.yak 主入口 + 同级 sidekick *.yak 拼接成 bundle，
//     创建一个临时 caller 提取 metadata（__VERBOSE_NAME__ 等），把 metadata
//     注册到全局 loopMetadata 表，把名字 → bundle 映射保存到 yakFocusBundles。
//
//  2. run 期：每次 CreateLoopByName 触发 LoopFactory，工厂内部再创建一个新的
//     caller（独立 yak 引擎），调用 CollectFocusMode* 系列函数生成 ReActLoopOption
//     列表，构造 ReActLoop 实例，并通过 onRelease 回收 caller。
//
// 关键词: yak focus mode register, two-phase registration, bundle to factory

// FocusModeBundle 描述一个完整的 Yak 专注模式：主入口代码 + sidekick 列表 +
// 元数据来源（文件名、自定义名）。
//
// 关键词: focus mode bundle definition
type FocusModeBundle struct {
	// Name 显式指定专注模式名（不填则取 EntryFile 的 basename 作 fallback）
	Name string

	// EntryFile 主入口文件相对路径（仅做调试 / log 使用）
	EntryFile string

	// EntryCode 主入口 yak 代码内容
	EntryCode string

	// Sidekicks 同级目录中其它 yak 文件的内容（除 .ai-focus.yak 主入口）。
	// 这些代码会被拼接到 EntryCode 之前，作为可复用的工具函数。
	Sidekicks []FocusModeSidekick

	// ToolLookup 用于把 __ACTIONS_FROM_TOOLS__ 中的工具名解析为 *aitool.Tool
	// 实例。boot 期可空（仅 metadata），run 期建议提供，否则该 dunder 失效。
	ToolLookup func(name string) *aitool.Tool

	// CallTimeout 单次 hook 调用超时时间（默认 30s）
	CallTimeout time.Duration
}

// FocusModeSidekick 一个 sidekick yak 文件
type FocusModeSidekick struct {
	Path    string // 相对路径，仅用于 log
	Content string // yak 代码
}

// yakFocusBundles 保存所有已通过 RegisterYakFocusModeFromBundle 注册的 bundle，
// run 期 CreateLoopByName 时会回查它生成新引擎。
var yakFocusBundles = map[string]*FocusModeBundle{}

// GetYakFocusModeBundle 通过名字查询已注册的 bundle，主要供测试使用。
func GetYakFocusModeBundle(name string) (*FocusModeBundle, bool) {
	b, ok := yakFocusBundles[name]
	return b, ok
}

// RegisterYakFocusModeFromBundle 注册一个 Yak 专注模式 bundle。
// 内部步骤：
//  1. 创建 boot caller（临时引擎），执行 bundle 提取 metadata + 名字
//  2. 把 metadata 注册到全局 loopMetadata 表
//  3. 把 bundle 缓存到 yakFocusBundles
//  4. 注册一个 LoopFactory：每次创建 loop 时新建 caller，收集 options 后构造 ReActLoop
//
// 关键词: register yak focus mode, boot phase metadata, run phase factory
func RegisterYakFocusModeFromBundle(bundle *FocusModeBundle) error {
	if bundle == nil {
		return utils.Error("yak focus mode: bundle is nil")
	}
	if bundle.EntryCode == "" {
		return utils.Error("yak focus mode: bundle entry code is empty")
	}

	defaultName := bundle.Name
	if defaultName == "" {
		defaultName = deriveFocusModeNameFromFile(bundle.EntryFile)
	}
	if defaultName == "" {
		return utils.Error("yak focus mode: cannot derive name (no explicit Name + no EntryFile)")
	}

	bundleCode := bundleSidekicksFromList(bundle.EntryCode, bundle.Sidekicks)

	// ---- boot 期：创建临时 caller 提取 metadata + 解析最终名字 ----
	bootCallTimeout := bundle.CallTimeout
	if bootCallTimeout <= 0 {
		bootCallTimeout = 30 * time.Second
	}
	bootCaller, err := NewFocusModeYakHookCaller(
		bundle.EntryFile,
		bundleCode,
		WithFocusModeCallerCallTimeout(bootCallTimeout),
	)
	if err != nil {
		return utils.Wrapf(err, "yak focus mode: boot eval failed for %s", defaultName)
	}
	defer bootCaller.Close()

	resolvedName, metadataOpts := CollectFocusModeMetadataOptions(bootCaller, defaultName)
	if resolvedName == "" {
		resolvedName = defaultName
	}
	bundle.Name = resolvedName

	// 缓存 bundle 给 run 期使用（在尝试注册 LoopFactory 之前），方便测试通过名称回查
	yakFocusBundles[resolvedName] = bundle

	// ---- run 期 LoopFactory ----
	factory := func(invoker aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
		runCallTimeout := bundle.CallTimeout
		if runCallTimeout <= 0 {
			runCallTimeout = 30 * time.Second
		}

		callerOpts := []FocusModeCallerOption{
			WithFocusModeCallerCallTimeout(runCallTimeout),
		}
		if invoker != nil {
			if cfg := invoker.GetConfig(); cfg != nil {
				if ctx := cfg.GetContext(); ctx != nil {
					callerOpts = append(callerOpts, WithFocusModeCallerParentContext(ctx))
				}
			}
		}

		caller, err := NewFocusModeYakHookCaller(bundle.EntryFile, bundleCode, callerOpts...)
		if err != nil {
			return nil, utils.Wrapf(err, "yak focus mode[%v]: create run-phase caller failed", resolvedName)
		}

		// 拼装 options：静态 < 动态 < actions（外部 opts 优先级最高，最后追加）
		var allOpts []ReActLoopOption
		allOpts = append(allOpts, CollectFocusModeStaticOptions(caller)...)
		allOpts = append(allOpts, CollectFocusModeDynamicOptions(caller)...)
		allOpts = append(allOpts, CollectFocusModeActionOptions(caller, bundle.ToolLookup)...)
		allOpts = append(allOpts, WithOnLoopRelease(caller.Close))
		allOpts = append(allOpts, opts...)

		loop, err := NewReActLoop(resolvedName, invoker, allOpts...)
		if err != nil {
			caller.Close()
			return nil, utils.Wrapf(err, "yak focus mode[%v]: build ReActLoop failed", resolvedName)
		}
		return loop, nil
	}

	if err := RegisterLoopFactory(resolvedName, factory, metadataOpts...); err != nil {
		// 注册失败时回滚 bundle 缓存，避免 GetYakFocusModeBundle 给出脏数据
		delete(yakFocusBundles, resolvedName)
		return utils.Wrapf(err, "yak focus mode[%v]: register loop factory failed", resolvedName)
	}

	log.Infof("yak focus mode[%v] registered (entry=%v, sidekicks=%d)", resolvedName, bundle.EntryFile, len(bundle.Sidekicks))
	return nil
}

// RegisterYakFocusMode 是 RegisterYakFocusModeFromBundle 的便捷封装，
// 专门用于"只有一段主代码 + 没有 sidekick"的简单场景。
//
// 关键词: register yak focus mode simple
func RegisterYakFocusMode(name string, entryCode string) error {
	return RegisterYakFocusModeFromBundle(&FocusModeBundle{
		Name:      name,
		EntryFile: name + FocusModeFileSuffix,
		EntryCode: entryCode,
	})
}

// deriveFocusModeNameFromFile 从文件路径中提取专注模式名：取 basename 并去掉
// 后缀 .ai-focus.yak / .yak。
func deriveFocusModeNameFromFile(path string) string {
	if path == "" {
		return ""
	}
	base := filepath.Base(path)
	for _, suffix := range []string{FocusModeFileSuffix, FocusModeYakFileSuffix} {
		if strings.HasSuffix(base, suffix) {
			return strings.TrimSuffix(base, suffix)
		}
	}
	return base
}

// bundleSidekicksFromList 把 FocusModeSidekick 列表与主入口代码拼接，
// 复用 BundleSidekicks 的格式约定但支持额外的路径 banner。
func bundleSidekicksFromList(entry string, sidekicks []FocusModeSidekick) string {
	if len(sidekicks) == 0 {
		return entry
	}
	var b strings.Builder
	for _, sk := range sidekicks {
		if sk.Content == "" {
			continue
		}
		b.WriteString("// ===== sidekick: ")
		if sk.Path != "" {
			b.WriteString(sk.Path)
		} else {
			b.WriteString("(unnamed)")
		}
		b.WriteString(" ===== //\n")
		b.WriteString(sk.Content)
		b.WriteString("\n\n")
	}
	b.WriteString("// ===== main focus entry ===== //\n")
	b.WriteString(entry)
	return b.String()
}
